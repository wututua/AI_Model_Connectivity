package notify

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"cg/internal/config"
)

func evidenceScope(cfg config.Config) string {
	type providerScope struct {
		ID, Name, Revision    string
		Models                []string
		Enabled, ProbeEnabled bool
	}
	scopes := make([]providerScope, 0, len(cfg.Providers))
	for _, provider := range cfg.Providers {
		scopes = append(scopes, providerScope{
			ID: provider.ID, Name: provider.Name, Revision: provider.ConnectionRevision,
			Models: provider.Models, Enabled: provider.Enabled, ProbeEnabled: provider.ProbeEnabled,
		})
	}
	// Only non-secret scope settings and opaque connection revisions enter this key.
	body, _ := json.Marshal(struct {
		Providers, Models, SkipModels  []string
		Scopes                         []providerScope
		MaxModels, DiscoveryModelLimit int
		Failure, Recovery              int
		Start, End                     string
	}{
		Providers: cfg.NotifyProviders, Models: cfg.NotifyModels, SkipModels: cfg.SkipModels,
		Scopes: scopes, MaxModels: cfg.MaxModelsPerProvider, DiscoveryModelLimit: cfg.DiscoveryModelLimit,
		Failure: cfg.NotifyFailureThreshold, Recovery: cfg.NotifyRecoveryThreshold,
		Start: cfg.MaintenanceStart, End: cfg.MaintenanceEnd,
	})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
