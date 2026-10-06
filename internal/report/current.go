package report

import (
	"cg/internal/config"
	"cg/internal/probe"
	providerpkg "cg/internal/provider"
)

// WithConfig projects a snapshot onto the current configuration without inventing
// a new check timestamp or changing historical measurements.
func WithConfig(value Report, current config.AdminConfig) Report {
	value.Title = current.Settings.DashboardTitle
	if value.Title == "" {
		value.Title = "模型连通性"
	}
	configured := make(map[string]config.SafeProviderConfig, len(current.Providers))
	for _, provider := range current.Providers {
		if provider.Enabled {
			configured[provider.ID] = provider
		}
	}
	errors := make([]probe.ProviderError, 0, len(value.ProviderErrors))
	failed := map[string]bool{}
	revisions := make(map[string]string, len(value.Providers))
	for _, group := range value.Providers {
		revisions[group.ProviderID] = group.ConnectionRevision
	}
	for _, failure := range value.ProviderErrors {
		if provider, ok := configured[failure.ProviderID]; ok && provider.ProbeEnabled &&
			revisions[failure.ProviderID] == provider.ConnectionRevision {
			errors = append(errors, failure)
			failed[failure.ProviderID] = true
		}
	}
	groups := make([]ProviderReport, 0, len(configured))
	seen := map[string]bool{}
	for _, group := range value.Providers {
		provider, ok := configured[group.ProviderID]
		if !ok {
			continue
		}
		seen[group.ProviderID] = true
		group.ProviderName, group.ProviderType = provider.Name, provider.Type
		group.ProviderLogo = providerpkg.IconFor(provider.ID, provider.Type, provider.Name)
		stale := group.ConnectionRevision != provider.ConnectionRevision
		group.Results = currentModels(group.Results, provider, current.Settings, stale)
		group.ConnectionRevision = provider.ConnectionRevision
		if stale {
			group.CheckedAt = ""
		}
		if !provider.ProbeEnabled {
			group.Results = []ModelResult{}
			group.Status, group.StatusLabel = "paused", "已暂停"
		} else if group.Status == "paused" {
			group.Status, group.StatusLabel = "unknown", "未检测"
		}
		groups = append(groups, group)
	}
	for _, provider := range current.Providers {
		if !provider.Enabled || seen[provider.ID] {
			continue
		}
		status, label := "unknown", "未检测"
		if !provider.ProbeEnabled {
			status, label = "paused", "已暂停"
		}
		groups = append(groups, ProviderReport{
			ConnectionRevision: provider.ConnectionRevision,
			ProviderID:         provider.ID, ProviderName: provider.Name, ProviderType: provider.Type,
			ProviderLogo: providerpkg.IconFor(provider.ID, provider.Type, provider.Name),
			Results:      currentModels(nil, provider, current.Settings, false), Status: status, StatusLabel: label,
		})
	}
	value.Providers, value.ProviderErrors = groups, errors
	value.Total, value.OKCount, value.SlowCount, value.ErrorCount, value.UnknownCount = 0, 0, 0, 0, 0
	for i := range value.Providers {
		group := &value.Providers[i]
		group.OKCount, group.SlowCount, group.ErrorCount, group.UnknownCount = 0, 0, 0, 0
		for _, model := range group.Results {
			switch model.Status {
			case "ok":
				group.OKCount++
			case "slow":
				group.SlowCount++
			case "error":
				group.ErrorCount++
			case "unknown":
				group.UnknownCount++
			}
		}
		group.ModelCount = len(group.Results)
		group.CurrentModel = ""
		if group.ModelCount > 0 {
			group.CurrentModel = group.Results[0].Model
		}
		for j := range group.Results {
			group.Results[j].CurrentModel = group.CurrentModel
			group.Results[j].IsCurrent = j == 0
		}
		if group.Status != "paused" {
			switch {
			case failed[group.ProviderID] || group.ErrorCount > 0:
				group.Status, group.StatusLabel = "error", "异常"
			case group.UnknownCount > 0 || group.ModelCount == 0:
				group.Status, group.StatusLabel = "unknown", "未检测"
			case group.SlowCount > 0:
				group.Status, group.StatusLabel = "slow", "较慢"
			default:
				group.Status, group.StatusLabel = "ok", "正常"
			}
		}
		value.Total += group.ModelCount
		value.OKCount += group.OKCount
		value.SlowCount += group.SlowCount
		value.ErrorCount += group.ErrorCount
		value.UnknownCount += group.UnknownCount
	}
	value.ProviderCount = len(groups)
	value.OverallStatus, value.OverallClass = "OPERATIONAL", "ok"
	if value.ErrorCount > 0 || value.UnknownCount > 0 || len(errors) > 0 {
		value.OverallStatus, value.OverallClass = "DEGRADED", "error"
	}
	for _, group := range groups {
		if group.Status == "unknown" {
			value.OverallStatus, value.OverallClass = "DEGRADED", "error"
		}
	}
	value.State = "ready"
	if len(configured) == 0 {
		value.State = "unconfigured"
	} else if value.GeneratedAt == "" {
		value.State = "pending"
	}
	return WithErrorVisibility(value, current.Settings.ShowErrorDetail)
}

func currentModels(previous []ModelResult, provider config.SafeProviderConfig, settings config.RuntimeSettings, stale bool) []ModelResult {
	results := []ModelResult{}
	if !provider.ProbeEnabled {
		return results
	}
	models := append([]string{}, provider.Models...)
	known := make(map[string]ModelResult, len(previous))
	for _, model := range previous {
		known[model.Model] = model
		if len(provider.Models) == 0 {
			models = append(models, model.Model)
		}
	}
	models = probe.SelectModels(models, settings.SkipModels, provider.ID, provider.Name, settings.MaxModelsPerProvider)
	for _, name := range models {
		model, ok := known[name]
		if !ok {
			model = ModelResult{
				Result: probe.Result{
					ProviderID: provider.ID, ProviderGroupID: provider.ID, ProviderInstanceID: provider.ID,
					Model: name, HistoryKey: providerpkg.ModelKey(provider.ID, name), Status: "unknown",
				},
				StatusLabel: "未检测", StatusClass: "unknown", Availability: "N/A",
				History: []string{}, TimeLabels: []string{},
				AvgLatency24h: "N/A", P50Latency24h: "N/A", P95Latency24h: "N/A", P99Latency24h: "N/A",
			}
		}
		if stale {
			// Retain historical aggregates, but remove all current-check measurements.
			model.Result = probe.Result{
				ProviderID: provider.ID, ProviderGroupID: provider.ID, ProviderInstanceID: provider.ID,
				Model: name, HistoryKey: providerpkg.ModelKey(provider.ID, name), Status: "unknown",
			}
			model.StatusLabel, model.StatusClass = "未检测", "unknown"
		}
		model.HistoryKey = providerpkg.ModelKey(provider.ID, name)
		model.ProviderName, model.ProviderInstanceName, model.ProviderType = provider.Name, provider.Name, provider.Type
		model.ProviderLogo = providerpkg.IconFor(provider.ID, provider.Type, provider.Name)
		model.ShowCurveChart = settings.ShowCurveChart
		results = append(results, model)
	}
	return results
}
