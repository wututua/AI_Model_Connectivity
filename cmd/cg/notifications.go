package main

import (
	"context"
	"errors"

	"cg/internal/config"
	"cg/internal/notify"
	"cg/internal/storage"
	"cg/internal/web"
)

func (a *application) notificationClient(cfg config.Config) *notify.Client {
	client := notify.New(cfg, storage.SQLiteNotifyStateStore{Store: a.store})
	client.SetHistory(a.store)
	return client
}

func (a *application) SendNotification(ctx context.Context, retryID int64) (notify.Delivery, error) {
	if !a.notificationMu.TryLock() {
		return notify.Delivery{}, web.ErrNotificationBusy
	}
	defer a.notificationMu.Unlock()
	a.mu.RLock()
	stopping := a.shuttingDown
	a.mu.RUnlock()
	if stopping {
		return notify.Delivery{}, web.ErrShuttingDown
	}
	client := a.notificationClient(a.currentConfig())
	var value notify.Delivery
	var err error
	if retryID == 0 {
		value, err = client.SendTest(ctx)
	} else {
		var previous notify.Delivery
		previous, err = a.store.GetDelivery(ctx, retryID)
		if err != nil {
			return notify.Delivery{}, err
		}
		if previous.RuleID != "" {
			settings, loadErr := a.store.MonitoringSettings(ctx)
			if loadErr != nil {
				return notify.Delivery{}, loadErr
			}
			found := false
			for _, rule := range settings.Rules {
				if rule.ID == previous.RuleID && rule.Enabled {
					client = a.notificationClient(rule.Apply(a.currentConfig()))
					client.SetRuleID(rule.ID)
					found = true
					break
				}
			}
			if !found {
				return notify.Delivery{}, notify.ErrNotConfigured
			}
		}
		value, err = client.Retry(ctx, previous)
	}
	// A completed delivery is a valid API result even when the platform rejected it.
	if value.ID != 0 && !errors.Is(err, notify.ErrResultNotSaved) {
		return value, nil
	}
	return value, err
}
