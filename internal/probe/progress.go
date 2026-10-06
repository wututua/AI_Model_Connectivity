package probe

import (
	"context"

	"cg/internal/config"
)

type Progress struct {
	Phase      string               `json:"phase"`
	ProviderID string               `json:"provider_id"`
	Total      int                  `json:"total"`
	Completed  int                  `json:"completed"`
	Active     []config.ModelTarget `json:"active"`
}

func (r *Runner) SetObserver(callback func(Progress))                  { r.observer = callback }
func (r *Runner) SetRequestGuard(callback func(context.Context) error) { r.guard = callback }

func (r *Runner) updateProgress(update func(*Progress)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	update(&r.progress)
	if r.observer != nil {
		value := r.progress
		value.Active = append([]config.ModelTarget{}, value.Active...)
		r.observer(value)
	}
}

func (r *Runner) beforeRequest(ctx context.Context) error {
	r.mu.Lock()
	err := r.runErr
	r.mu.Unlock()
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && r.guard != nil {
		err = r.guard(ctx)
	}
	if err != nil {
		r.mu.Lock()
		if r.runErr == nil {
			r.runErr = err
		}
		r.mu.Unlock()
	}
	return err
}
