package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
	"cg/internal/provider"
	"cg/internal/report"
	"cg/internal/storage"
	"cg/internal/web"
)

func (a *application) reserveRequest(ctx context.Context) error {
	return a.store.ReserveRequest(ctx, a.currentConfig().DailyRequestLimit)
}

func (a *application) setPhase(phase string) {
	a.mu.Lock()
	a.progress.Phase = phase
	a.mu.Unlock()
}

func (a *application) StartSelectedCheck(ctx context.Context, selection config.CheckSelection) (storage.CheckTask, error) {
	if err := ctx.Err(); err != nil {
		return storage.CheckTask{}, err
	}
	cfg := a.currentConfig()
	previous, err := a.store.LatestReport(ctx)
	if err != nil {
		return storage.CheckTask{}, err
	}
	previous = report.WithConfig(previous, config.AdminConfigFromConfig(cfg))
	if selection.FailedOnly && len(selection.Targets) > 0 {
		return storage.CheckTask{}, fmt.Errorf("%w: failed_only 与 targets 不能同时使用", web.ErrInvalidSelection)
	}
	targets := selection.Targets
	if selection.FailedOnly {
		for _, group := range previous.Providers {
			if selection.ProviderID != "" && group.ProviderID != selection.ProviderID {
				continue
			}
			for _, model := range group.Results {
				if model.Status == "error" {
					targets = append(targets, config.ModelTarget{ProviderID: group.ProviderID, Model: model.Model})
				}
			}
		}
	}
	if len(targets) == 0 || len(targets) > 1000 {
		return storage.CheckTask{}, fmt.Errorf("%w: 请选择 1 到 1000 个模型；发现失败请重测整个 Provider", web.ErrInvalidSelection)
	}
	allowed := allowedModelTargets(cfg, previous)
	unique := make([]config.ModelTarget, 0, len(targets))
	seen := map[string]bool{}
	for _, target := range targets {
		key := provider.ModelKey(target.ProviderID, target.Model)
		if !allowed[key] || selection.ProviderID != "" && selection.ProviderID != target.ProviderID {
			return storage.CheckTask{}, fmt.Errorf("%w: 模型不存在、已暂停或不在当前检测范围", web.ErrInvalidSelection)
		}
		if !seen[key] {
			unique = append(unique, target)
			seen[key] = true
		}
	}
	options := checkOptions{Kind: "models", SaveLatest: true, Targets: unique, ProviderID: selection.ProviderID}
	if selection.FailedOnly {
		options.Kind = "failed"
	}
	background, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	run, err := a.beginCheck(background, options)
	if err != nil {
		cancel()
		return storage.CheckTask{}, err
	}
	go func() {
		defer cancel()
		_, _ = a.executeCheck(run, options)
	}()
	return run.task, nil
}

func allowedModelTargets(cfg config.Config, previous report.Report) map[string]bool {
	allowed := map[string]bool{}
	for _, item := range cfg.Providers {
		if !item.Enabled || !item.ProbeEnabled {
			continue
		}
		models := append([]string{}, item.Models...)
		if len(models) == 0 {
			for _, group := range previous.Providers {
				if group.ProviderID == item.ID {
					for _, model := range group.Results {
						models = append(models, model.Model)
					}
				}
			}
		}
		for _, model := range probe.SelectModels(models, cfg.SkipModels, item.ID, item.Name, cfg.MaxModelsPerProvider) {
			allowed[provider.ModelKey(item.ID, model)] = true
		}
	}
	return allowed
}

func selectedConfig(cfg config.Config, targets []config.ModelTarget) config.Config {
	selected := map[string][]string{}
	for _, target := range targets {
		selected[target.ProviderID] = append(selected[target.ProviderID], target.Model)
	}
	providers := []config.ProviderConfig{}
	for _, item := range cfg.Providers {
		if models := selected[item.ID]; len(models) > 0 {
			item.Models = models
			providers = append(providers, item)
		}
	}
	cfg.Providers = providers
	return cfg
}

func (a *application) BatchProviders(ctx context.Context, batch config.ProviderBatch) (config.AdminConfig, error) {
	if len(batch.IDs) == 0 || len(batch.IDs) > 1000 {
		return config.AdminConfig{}, errors.New("请选择 1 到 1000 个 Provider")
	}
	switch batch.Action {
	case "enable", "disable", "pause", "resume", "group":
	default:
		return config.AdminConfig{}, errors.New("不支持的批量操作")
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	cfg := a.currentConfig()
	providers := append([]config.ProviderConfig{}, cfg.Providers...)
	selected := map[string]bool{}
	for _, id := range batch.IDs {
		selected[id] = true
	}
	for i := range providers {
		if !selected[providers[i].ID] {
			continue
		}
		delete(selected, providers[i].ID)
		switch batch.Action {
		case "enable":
			providers[i].Enabled = true
		case "disable":
			providers[i].Enabled = false
		case "pause":
			providers[i].ProbeEnabled = false
		case "resume":
			providers[i].ProbeEnabled = true
		case "group":
			providers[i].Group = batch.Group
		}
	}
	if len(selected) > 0 {
		return config.AdminConfig{}, config.ErrProviderNotFound
	}
	err := a.replaceRuntimeConfig(ctx, config.RuntimeConfig{Settings: config.SettingsFromConfig(cfg), Providers: providers})
	return config.AdminConfigFromConfig(a.currentConfig()), err
}
