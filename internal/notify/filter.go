package notify

import (
	"strings"

	"cg/internal/report"
)

func filterReport(value report.Report, providers, models []string) report.Report {
	providerSet := lowerSet(providers)
	modelSet := lowerSet(models)
	if len(providerSet) == 0 && len(modelSet) == 0 {
		return value
	}

	filtered := value
	filtered.Providers = nil
	filtered.ProviderErrors = nil
	filtered.OKCount = 0
	filtered.SlowCount = 0
	filtered.ErrorCount = 0
	filtered.Total = 0

	for _, provider := range value.Providers {
		providerMatched := matchesProvider(providerSet, provider.ProviderID, provider.ProviderName)
		if len(providerSet) > 0 && !providerMatched {
			continue
		}
		copyProvider := provider
		copyProvider.Results = nil
		copyProvider.OKCount = 0
		copyProvider.SlowCount = 0
		copyProvider.ErrorCount = 0
		for _, result := range provider.Results {
			if len(modelSet) > 0 && !matchesModel(modelSet, result.Model, result.ProviderID, result.ProviderName) {
				continue
			}
			copyProvider.Results = append(copyProvider.Results, result)
			switch result.Status {
			case "ok":
				copyProvider.OKCount++
			case "slow":
				copyProvider.SlowCount++
			case "error":
				copyProvider.ErrorCount++
			}
		}
		if len(copyProvider.Results) == 0 {
			continue
		}
		copyProvider.ModelCount = len(copyProvider.Results)
		copyProvider.Status = "ok"
		copyProvider.StatusLabel = "正常"
		if copyProvider.ErrorCount > 0 {
			copyProvider.Status = "error"
			copyProvider.StatusLabel = "异常"
		} else if copyProvider.SlowCount > 0 {
			copyProvider.Status = "slow"
			copyProvider.StatusLabel = "较慢"
		}
		filtered.Providers = append(filtered.Providers, copyProvider)
		filtered.OKCount += copyProvider.OKCount
		filtered.SlowCount += copyProvider.SlowCount
		filtered.ErrorCount += copyProvider.ErrorCount
		filtered.Total += len(copyProvider.Results)
	}

	for _, item := range value.ProviderErrors {
		if len(providerSet) == 0 || matchesProvider(providerSet, item.ProviderID, "") {
			filtered.ProviderErrors = append(filtered.ProviderErrors, item)
		}
	}

	filtered.ProviderCount = len(filtered.Providers)
	filtered.OverallStatus = "OPERATIONAL"
	filtered.OverallClass = "ok"
	if filtered.ErrorCount > 0 || len(filtered.ProviderErrors) > 0 {
		filtered.OverallStatus = "DEGRADED"
		filtered.OverallClass = "error"
	}
	return filtered
}

func lowerSet(items []string) map[string]bool {
	set := map[string]bool{}
	for _, item := range items {
		value := strings.ToLower(strings.TrimSpace(item))
		if value != "" {
			set[value] = true
		}
	}
	return set
}

func matchesProvider(set map[string]bool, providerID, providerName string) bool {
	if len(set) == 0 {
		return true
	}
	return set[strings.ToLower(providerID)] || set[strings.ToLower(providerName)]
}

func matchesModel(set map[string]bool, model, providerID, providerName string) bool {
	if len(set) == 0 {
		return true
	}
	model = strings.ToLower(model)
	providerID = strings.ToLower(providerID)
	providerName = strings.ToLower(providerName)
	return set[model] || set[providerID+"/"+model] || set[providerID+"::"+model] || set[providerName+"/"+model] || set[providerName+"::"+model]
}
