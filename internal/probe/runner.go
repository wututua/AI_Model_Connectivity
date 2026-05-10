package probe

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"cg/internal/config"
	"cg/internal/provider"
)

type Target struct {
	Provider     provider.Provider
	ProviderID   string
	ProviderType string
	ProviderName string
	ProviderLogo string
	CurrentModel string
	Model        string
}

type Result struct {
	ProviderID           string `json:"provider_id"`
	ProviderGroupID      string `json:"provider_group_id"`
	ProviderType         string `json:"provider_type"`
	ProviderName         string `json:"provider_name"`
	ProviderLogo         string `json:"provider_logo"`
	ProviderInstanceID   string `json:"provider_instance_id"`
	ProviderInstanceName string `json:"provider_instance_name"`
	CurrentModel         string `json:"current_model"`
	Model                string `json:"model"`
	IsCurrent            bool   `json:"is_current"`
	Status               string `json:"status"`
	LatencyMS            int    `json:"latency_ms"`
	ResponsePreview      string `json:"response_preview"`
	Error                string `json:"error"`
	HistoryKey           string `json:"history_key"`
}

type ProviderError struct {
	ProviderID   string `json:"provider_id"`
	ProviderType string `json:"provider_type"`
	Error        string `json:"error"`
}

type Runner struct {
	cfg       config.Config
	providers []provider.Provider
}

func NewRunner(cfg config.Config) *Runner {
	providers := []provider.Provider{}
	for _, providerCfg := range cfg.Providers {
		if providerCfg.Enabled {
			providers = append(providers, provider.New(providerCfg))
		}
	}
	return &Runner{cfg: cfg, providers: providers}
}

func (r *Runner) Run(ctx context.Context) ([]Result, []ProviderError, error) {
	targets, providerErrors := r.collectTargets(ctx)
	if len(targets) == 0 {
		if len(providerErrors) > 0 {
			return nil, providerErrors, errors.New("no probe targets collected")
		}
		return nil, nil, errors.New("no enabled providers or models")
	}
	return r.probeTargets(ctx, targets), providerErrors, nil
}

func (r *Runner) collectTargets(ctx context.Context) ([]Target, []ProviderError) {
	targets := []Target{}
	providerErrors := []ProviderError{}
	seen := map[string]bool{}
	skip := skipSet(r.cfg.SkipModels)

	for _, item := range r.providers {
		modelsCtx, cancel := context.WithTimeout(ctx, durationSeconds(r.cfg.ModelListTimeoutSeconds))
		models, err := item.Models(modelsCtx)
		cancel()
		if err != nil {
			providerErrors = append(providerErrors, ProviderError{ProviderID: item.ID(), ProviderType: item.Type(), Error: shortError(err)})
			continue
		}
		models = dedupe(models)
		if r.cfg.MaxModelsPerProvider > 0 && len(models) > r.cfg.MaxModelsPerProvider {
			models = models[:r.cfg.MaxModelsPerProvider]
		}
		current := ""
		if len(models) > 0 {
			current = models[0]
		}
		logo := provider.IconFor(item.ID(), item.Type(), item.Name())
		for _, model := range models {
			if isSkipped(skip, item.ID(), item.Name(), model) {
				continue
			}
			key := item.ID() + "::" + model
			if seen[key] {
				continue
			}
			seen[key] = true
			targets = append(targets, Target{
				Provider:     item,
				ProviderID:   item.ID(),
				ProviderType: item.Type(),
				ProviderName: item.Name(),
				ProviderLogo: logo,
				CurrentModel: current,
				Model:        model,
			})
		}
	}
	return targets, providerErrors
}

func (r *Runner) probeTargets(ctx context.Context, targets []Target) []Result {
	globalLimit := make(chan struct{}, max(1, r.cfg.Concurrency))
	providerLimits := map[string]chan struct{}{}
	for _, target := range targets {
		if providerLimits[target.ProviderID] == nil {
			providerLimits[target.ProviderID] = make(chan struct{}, max(1, r.cfg.ProviderConcurrency))
		}
	}

	results := make([]Result, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(index int, target Target) {
			defer wg.Done()
			select {
			case globalLimit <- struct{}{}:
				defer func() { <-globalLimit }()
			case <-ctx.Done():
				results[index] = resultPayload(target, "error", 0, "", shortError(ctx.Err()))
				return
			}
			select {
			case providerLimits[target.ProviderID] <- struct{}{}:
				defer func() { <-providerLimits[target.ProviderID] }()
			case <-ctx.Done():
				results[index] = resultPayload(target, "error", 0, "", shortError(ctx.Err()))
				return
			}
			results[index] = r.probeOne(ctx, target)
		}(i, target)
	}
	wg.Wait()
	return results
}

func (r *Runner) probeOne(ctx context.Context, target Target) Result {
	started := time.Now()
	probeCtx, cancel := context.WithTimeout(ctx, durationSeconds(r.cfg.TimeoutSeconds))
	defer cancel()
	text, err := target.Provider.Chat(probeCtx, target.Model, r.cfg.ProbeSystemPrompt, r.cfg.ProbePrompt)
	latency := int(time.Since(started).Milliseconds())
	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return resultPayload(target, "error", latency, "", fmt.Sprintf("timeout after %gs", r.cfg.TimeoutSeconds))
		}
		return resultPayload(target, "error", latency, "", shortError(err))
	}
	status := "ok"
	if latency >= r.cfg.SlowThresholdMS {
		status = "slow"
	}
	return resultPayload(target, status, latency, truncate(text, 80), "")
}

func resultPayload(target Target, status string, latency int, preview, errText string) Result {
	return Result{
		ProviderID:           target.ProviderID,
		ProviderGroupID:      target.ProviderID,
		ProviderType:         target.ProviderType,
		ProviderName:         target.ProviderName,
		ProviderLogo:         target.ProviderLogo,
		ProviderInstanceID:   target.ProviderID,
		ProviderInstanceName: target.ProviderName,
		CurrentModel:         target.CurrentModel,
		Model:                target.Model,
		IsCurrent:            target.Model == target.CurrentModel,
		Status:               status,
		LatencyMS:            latency,
		ResponsePreview:      preview,
		Error:                errText,
		HistoryKey:           target.ProviderID + "::" + target.Model,
	}
}

func skipSet(items []string) map[string]bool {
	set := map[string]bool{}
	for _, item := range items {
		value := strings.ToLower(strings.TrimSpace(item))
		if value != "" {
			set[value] = true
		}
	}
	return set
}

func isSkipped(skip map[string]bool, providerID, providerName, model string) bool {
	modelKey := strings.ToLower(model)
	providerID = strings.ToLower(providerID)
	providerName = strings.ToLower(providerName)
	candidates := []string{modelKey, providerID + "/" + modelKey, providerID + "::" + modelKey, providerName + "/" + modelKey, providerName + "::" + modelKey}
	for _, candidate := range candidates {
		if skip[candidate] {
			return true
		}
	}
	return false
}

func dedupe(items []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func durationSeconds(value float64) time.Duration {
	if value <= 0 {
		value = 1
	}
	return time.Duration(value * float64(time.Second))
}

func shortError(err error) string {
	text := sanitizeErrorText(strings.TrimSpace(err.Error()))
	return truncate(text, 300)
}

func sanitizeErrorText(text string) string {
	if pos := strings.Index(text, `/v1/`); pos >= 0 {
		if end := strings.Index(text[pos:], `"`); end >= 0 {
			text = `Post "` + text[pos:pos+end] + `"` + text[pos+end+1:]
		}
	}
	if idx := strings.Index(text, `lookup `); idx >= 0 {
		if end := strings.Index(text[idx+len(`lookup `):], `: no such host`); end >= 0 {
			text = text[:idx+len(`lookup `)] + text[idx+len(`lookup `)+end:]
		}
	}
	return text
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}


