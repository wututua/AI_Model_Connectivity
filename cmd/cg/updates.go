package main

import (
	"context"

	"cg/internal/update"
)

func (a *application) Status() (update.Status, error) { return a.updater.Status() }
func (a *application) Resolve(requestID string) (update.Status, error) {
	return a.updater.Resolve(requestID)
}
func (a *application) Check(ctx context.Context, channel string) (update.Check, error) {
	return a.updater.Check(ctx, channel)
}

func (a *application) Start(ctx context.Context, channel, version, requestID string) (update.Job, error) {
	// Fetch before taking the application lock. Start reuses the checked release.
	if _, err := a.updater.Check(ctx, channel); err != nil {
		return update.Job{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running || a.shuttingDown {
		return update.Job{}, update.ErrBusy
	}
	return a.updater.Start(ctx, channel, version, requestID)
}
