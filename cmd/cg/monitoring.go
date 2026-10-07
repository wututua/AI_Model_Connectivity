package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"cg/internal/config"
	"cg/internal/notify"
	"cg/internal/report"
	"cg/internal/storage"
	"cg/internal/web"
)

func (a *application) sendRuleAlerts(ctx context.Context, observed report.Report, checked config.Config, providerID string, settings config.MonitoringSettings) {
	a.notificationMu.Lock()
	defer a.notificationMu.Unlock()
	currentSettings, err := a.store.MonitoringSettings(ctx)
	if err != nil || currentSettings.Version != settings.Version {
		return
	}
	current := a.currentConfig()
	for _, rule := range settings.Rules {
		if !rule.Enabled || providerID != "" && rule.ProviderID != providerID {
			continue
		}
		cfg := rule.Apply(current)
		for _, schedule := range settings.Schedules {
			if schedule.ProviderID == rule.ProviderID && (config.OperationsSettings{MaintenanceStart: schedule.MaintenanceStart, MaintenanceEnd: schedule.MaintenanceEnd}).InMaintenance(time.Now()) {
				cfg.MaintenanceStart, cfg.MaintenanceEnd = schedule.MaintenanceStart, schedule.MaintenanceEnd
			}
		}
		scope := []string{rule.Generation}
		for _, p := range current.Providers {
			if rule.ProviderID == "" || rule.ProviderID == p.ID {
				scope = append(scope, p.ID, p.ConnectionRevision)
			}
		}
		encoded, _ := json.Marshal(scope)
		hash := sha256.Sum256(encoded)
		client := notify.New(cfg, storage.RuleStateStore{Store: a.store, Key: rule.ID + ":" + hex.EncodeToString(hash[:])})
		client.SetHistory(a.store)
		client.SetRuleID(rule.ID)
		if err := client.SendCheckIfNeeded(ctx, report.WithConfig(observed, config.AdminConfigFromConfig(cfg)), rule.Apply(checked)); err != nil {
			slog.Warn("rule notification failed", "rule", rule.ID, "err", err)
		}
	}
}

func (a *application) runProviderSchedules(ctx context.Context, cfg config.Config, settings config.MonitoringSettings) {
	minHours, maxHours, enabled := intervalRange(cfg)
	fallback := 0
	if enabled {
		fallback = max(1, int((minHours+maxHours)*30))
	}
	for _, provider := range cfg.Providers {
		currentSettings, err := a.store.MonitoringSettings(ctx)
		if err != nil || currentSettings.Version != settings.Version {
			return
		}
		if !provider.Enabled || !provider.ProbeEnabled || ctx.Err() != nil {
			continue
		}
		minutes := fallback
		for _, schedule := range settings.Schedules {
			if schedule.ProviderID == provider.ID && schedule.IntervalMinutes > 0 {
				minutes = schedule.IntervalMinutes
			}
		}
		due, err := a.store.ScheduleDue(ctx, provider.ID, provider.ConnectionRevision, minutes, time.Now())
		if err != nil || !due {
			continue
		}
		current, ok := filterProvider(a.currentConfig(), provider.ID)
		if !ok || current.Providers[0].ConnectionRevision != provider.ConnectionRevision {
			continue
		}
		jobCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		_, err = a.checkWithOptions(jobCtx, checkOptions{Kind: "scheduled-provider", ProviderID: provider.ID, SaveLatest: true})
		cancel()
		if errors.Is(err, web.ErrCheckAlreadyRunning) || errors.Is(err, storage.ErrBudgetExceeded) || errors.Is(err, web.ErrShuttingDown) {
			return
		}
		if err != nil {
			slog.Warn("provider schedule failed", "provider", provider.ID, "err", err)
		}
		if ctx.Err() == nil {
			_ = a.store.CompleteSchedule(ctx, provider.ID, provider.ConnectionRevision, minutes, time.Now())
		}
	}
}

func (a *application) backupScheduler(ctx context.Context) {
	timer := time.NewTicker(time.Minute)
	defer timer.Stop()
	nextAttempt := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		a.sendOperationalNotices(ctx)
		settings, err := a.store.MonitoringSettings(ctx)
		if err != nil || settings.BackupIntervalHours == 0 || time.Now().Before(nextAttempt) {
			continue
		}
		backups, err := a.store.Backups(ctx)
		if err != nil {
			continue
		}
		if len(backups) > 0 {
			last, _ := time.Parse(time.RFC3339, backups[0].CreatedAt)
			if time.Since(last) < time.Duration(settings.BackupIntervalHours)*time.Hour {
				continue
			}
		}
		nextAttempt = time.Now().Add(15 * time.Minute)
		jobCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		_, err = a.store.CreateBackup(jobCtx, settings.BackupKeep)
		cancel()
		kind, detail := "backup", "scheduled backup completed"
		if err != nil {
			kind, detail = "backup_error", "scheduled backup failed; inspect disk space and database access"
		}
		_ = a.store.AddMonitorEvent(ctx, kind, detail)
	}
}
