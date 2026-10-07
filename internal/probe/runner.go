package probe

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"cg/internal/config"
	"cg/internal/httpclient"
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
	UsageKnown           bool                    `json:"usage_known"`
	Capability           string                  `json:"capability,omitempty"`
	CapabilityStatus     string                  `json:"capability_status,omitempty"`
	Diagnostics          *httpclient.Diagnostics `json:"diagnostics,omitempty"`
	FirstTokenMS         int                     `json:"first_token_ms,omitempty"`
	Completed            bool                    `json:"-"`
	ProviderID           string                  `json:"provider_id"`
	ProviderGroupID      string                  `json:"provider_group_id"`
	ProviderType         string                  `json:"provider_type"`
	ProviderName         string                  `json:"provider_name"`
	ProviderLogo         string                  `json:"provider_logo"`
	ProviderInstanceID   string                  `json:"provider_instance_id"`
	ProviderInstanceName string                  `json:"provider_instance_name"`
	CurrentModel         string                  `json:"current_model"`
	Model                string                  `json:"model"`
	IsCurrent            bool                    `json:"is_current"`
	Status               string                  `json:"status"`
	LatencyMS            int                     `json:"latency_ms"`
	ResponsePreview      string                  `json:"response_preview"`
	Error                string                  `json:"error"`
	HistoryKey           string                  `json:"history_key"`
	PromptTokens         int                     `json:"prompt_tokens"`
	CompletionTokens     int                     `json:"completion_tokens"`
	TotalTokens          int                     `json:"total_tokens"`
	CheckedAt            string                  `json:"checked_at"`
}

type ProviderError struct {
	ProviderID   string `json:"provider_id"`
	ProviderType string `json:"provider_type"`
	Error        string `json:"error"`
}

type Runner struct {
	cfg            config.Config
	providers      []provider.Provider
	mu             sync.Mutex
	progress       Progress
	observer       func(Progress)
	guard          func(context.Context) error
	runErr         error
	catalog        func(context.Context, config.ProviderConfig, []string) ([]string, error)
	slowThresholds map[string]int
}

func (r *Runner) SetSlowThresholds(schedules []config.ProviderSchedule) {
	r.slowThresholds = map[string]int{}
	for _, schedule := range schedules {
		if schedule.SlowThresholdMS > 0 {
			r.slowThresholds[schedule.ProviderID] = schedule.SlowThresholdMS
		}
	}
}

func (r *Runner) SetCatalogObserver(observer func(context.Context, config.ProviderConfig, []string) ([]string, error)) {
	r.catalog = observer
}

func NewRunner(cfg config.Config) *Runner {
	providers := []provider.Provider{}
	for _, providerCfg := range cfg.Providers {
		if providerCfg.Enabled && providerCfg.ProbeEnabled {
			providers = append(providers, provider.New(providerCfg))
		}
	}
	return &Runner{cfg: cfg, providers: providers}
}

func (r *Runner) Run(ctx context.Context) ([]Result, []ProviderError, error) {
	r.mu.Lock()
	r.progress, r.runErr = Progress{Active: []config.ModelTarget{}}, nil
	r.mu.Unlock()
	for _, item := range r.providers {
		if closer, ok := item.(interface{ CloseIdleConnections() }); ok {
			defer closer.CloseIdleConnections()
		}
	}
	targets, providerErrors := r.collectTargets(ctx)
	if err := ctx.Err(); err != nil {
		return nil, providerErrors, err
	}
	if len(targets) == 0 {
		return nil, providerErrors, r.runErr
	}
	r.updateProgress(func(p *Progress) { p.Phase = "probing"; p.ProviderID = ""; p.Total = len(targets) })
	results := r.probeTargets(ctx, targets)
	if err := ctx.Err(); err != nil {
		return results, providerErrors, err
	}
	return results, providerErrors, r.runErr
}

func (r *Runner) collectTargets(ctx context.Context) ([]Target, []ProviderError) {
	targets := []Target{}
	providerErrors := []ProviderError{}
	seen := map[string]bool{}

	for _, item := range r.providers {
		if ctx.Err() != nil {
			break
		}
		r.updateProgress(func(p *Progress) { p.Phase = "discovering"; p.ProviderID = item.ID() })
		automatic := true
		for _, configured := range r.cfg.Providers {
			if configured.ID == item.ID() {
				automatic = len(configured.Models) == 0
				break
			}
		}
		if automatic && r.beforeRequest(ctx) != nil {
			break
		}
		modelsCtx, cancel := context.WithTimeout(ctx, durationSeconds(r.cfg.ModelListTimeoutSeconds))
		models, err := item.Models(modelsCtx)
		cancel()
		if err != nil {
			providerErrors = append(providerErrors, ProviderError{ProviderID: item.ID(), ProviderType: item.Type(), Error: shortError(err)})
			continue
		}
		models = dedupe(models)
		if automatic && r.cfg.DiscoveryModelLimit > 0 && len(models) > r.cfg.DiscoveryModelLimit {
			providerErrors = append(providerErrors, ProviderError{ProviderID: item.ID(), ProviderType: item.Type(),
				Error: fmt.Sprintf("自动发现 %d 个模型，超过确认阈值 %d；请在 Provider 中选择并保存模型", len(models), r.cfg.DiscoveryModelLimit)})
			continue
		}
		if len(models) == 0 {
			providerErrors = append(providerErrors, ProviderError{ProviderID: item.ID(), ProviderType: item.Type(), Error: "no models returned"})
			continue
		}
		if automatic && r.catalog != nil {
			for _, configured := range r.cfg.Providers {
				if configured.ID != item.ID() {
					continue
				}
				models, err = r.catalog(ctx, configured, models)
				if err != nil {
					r.runErr = err
					return targets, providerErrors
				}
				if len(models) == 0 {
					providerErrors = append(providerErrors, ProviderError{ProviderID: item.ID(), ProviderType: item.Type(), Error: "model catalog requires approval"})
				}
				break
			}
		}
		models = SelectModels(models, r.cfg.SkipModels, item.ID(), item.Name(), r.cfg.MaxModelsPerProvider)
		current := ""
		if len(models) > 0 {
			current = models[0]
		}
		logo := provider.IconFor(item.ID(), item.Type(), item.Name())
		for _, model := range models {
			key := provider.ModelKey(item.ID(), model)
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
			case providerLimits[target.ProviderID] <- struct{}{}:
				defer func() { <-providerLimits[target.ProviderID] }()
			case <-ctx.Done():
				results[index] = resultPayload(target, "error", 0, "", shortError(ctx.Err()), provider.Usage{})
				return
			}
			select {
			case globalLimit <- struct{}{}:
				defer func() { <-globalLimit }()
			case <-ctx.Done():
				results[index] = resultPayload(target, "error", 0, "", shortError(ctx.Err()), provider.Usage{})
				return
			}
			results[index] = r.probeOne(ctx, target)
		}(i, target)
	}
	wg.Wait()
	return results
}

func (r *Runner) probeOne(ctx context.Context, target Target) (result Result) {
	if r.beforeRequest(ctx) != nil {
		return Result{}
	}
	r.updateProgress(func(p *Progress) {
		p.Active = append(p.Active, config.ModelTarget{ProviderID: target.ProviderID, Model: target.Model})
	})
	defer r.updateProgress(func(p *Progress) {
		p.Completed++
		for i, item := range p.Active {
			if item.ProviderID == target.ProviderID && item.Model == target.Model {
				p.Active = append(p.Active[:i], p.Active[i+1:]...)
				break
			}
		}
	})
	// Keep confirmed responses even when another probe cancels the batch.
	defer func() {
		result.Completed = ctx.Err() == nil || result.Status == "ok" || result.Status == "slow" ||
			result.PromptTokens > 0 || result.CompletionTokens > 0 || result.TotalTokens > 0
	}()
	started := time.Now()
	timeout := r.cfg.TimeoutSeconds
	options := config.ProbeOptions{}
	for _, item := range r.cfg.Providers {
		if item.ID == target.ProviderID {
			options = item.Probe
			if item.Probe.TimeoutSeconds > 0 {
				timeout = item.Probe.TimeoutSeconds
			}
			break
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, durationSeconds(timeout))
	defer cancel()
	probeCtx, trace := httpclient.TraceContext(probeCtx)
	defer func() {
		secret := ""
		for _, item := range r.cfg.Providers {
			if item.ID == target.ProviderID {
				secret = item.APIKey
				break
			}
		}
		result.Diagnostics = trace.Snapshot(secret)
		result.Capability = options.Capability
		if result.Capability == "" {
			result.Capability = "text"
		}
		switch {
		case result.Status == "ok" || result.Status == "slow":
			result.CapabilityStatus = "passed"
		case strings.Contains(result.Error, provider.ErrAssertion.Error()):
			result.CapabilityStatus = "assertion_failed"
		default:
			result.CapabilityStatus = "request_failed"
		}
	}()
	text, usage, err := target.Provider.Chat(probeCtx, target.Model, r.cfg.ProbeSystemPrompt, r.cfg.ProbePrompt)
	if err == nil && options.Capability != "embedding" && options.Capability != "tools" {
		err = provider.AssertText(text, options)
	}
	latency := int(time.Since(started).Milliseconds())
	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return resultPayload(target, "error", latency, "", fmt.Sprintf("timeout after %gs", timeout), usage)
		}
		return resultPayload(target, "error", latency, "", shortError(err), usage)
	}
	status := "ok"
	threshold := r.cfg.SlowThresholdMS
	if configured := r.slowThresholds[target.ProviderID]; configured > 0 {
		threshold = configured
	}
	if latency >= threshold {
		status = "slow"
	}
	return resultPayload(target, status, latency, truncate(text, 80), "", usage)
}

func resultPayload(target Target, status string, latency int, preview, errText string, usage provider.Usage) Result {
	return Result{
		UsageKnown:           usage.Known,
		FirstTokenMS:         usage.FirstTokenMS,
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
		HistoryKey:           provider.ModelKey(target.ProviderID, target.Model),
		PromptTokens:         usage.PromptTokens,
		CompletionTokens:     usage.CompletionTokens,
		TotalTokens:          usage.TotalTokens,
		CheckedAt:            time.Now().UTC().Format(time.RFC3339),
	}
}

// SelectModels applies exclusions before spending the per-provider probe budget.
func SelectModels(models, exclusions []string, providerID, providerName string, limit int) []string {
	skip := skipSet(exclusions)
	selected := []string{}
	for _, model := range dedupe(models) {
		if isSkipped(skip, providerID, providerName, model) {
			continue
		}
		selected = append(selected, model)
		if limit > 0 && len(selected) >= limit {
			break
		}
	}
	return selected
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
