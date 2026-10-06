package report

import "cg/internal/probe"

// MergeModels only replaces observed model rows. Provider-level discovery errors
// and check timestamps require a full provider check to be cleared/refreshed.
func MergeModels(base, update Report) Report {
	merged := update
	merged.Providers = append([]ProviderReport{}, base.Providers...)
	merged.ProviderErrors = append(append([]probe.ProviderError{}, base.ProviderErrors...), update.ProviderErrors...)
	for _, replacement := range update.Providers {
		index := -1
		for i, item := range merged.Providers {
			if item.ProviderID == replacement.ProviderID {
				index = i
				break
			}
		}
		if index < 0 {
			replacement.CheckedAt = ""
			merged.Providers = append(merged.Providers, replacement)
			continue
		}
		group := &merged.Providers[index]
		group.Results = append([]ModelResult{}, group.Results...)
		for _, model := range replacement.Results {
			found := false
			for j := range group.Results {
				if group.Results[j].Model == model.Model {
					group.Results[j], found = model, true
					break
				}
			}
			if !found {
				group.Results = append(group.Results, model)
			}
		}
	}
	return merged
}
