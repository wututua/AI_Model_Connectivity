package config

import (
	"crypto/rand"
	"errors"
	"strings"
)

var ErrProviderExists = errors.New("provider id already exists")
var ErrProviderNotFound = errors.New("provider not found")

// ReconcileProviderRevisions uses opaque generations, never hashes of credentials.
// Persisting them prevents old snapshots and in-flight checks from surviving an
// endpoint change, including change-back and delete/recreate operations.
func ReconcileProviderRevisions(previous, next []ProviderConfig) []ProviderConfig {
	known := make(map[string]ProviderConfig, len(previous))
	for _, provider := range previous {
		known[provider.ID] = provider
	}
	result := append([]ProviderConfig(nil), next...)
	for i := range result {
		provider := &result[i]
		old, exists := known[provider.ID]
		if exists && old.ConnectionRevision != "" &&
			normalizeBaseURL(old.BaseURL) == normalizeBaseURL(provider.BaseURL) &&
			old.Type == provider.Type && old.APIKey == provider.APIKey &&
			old.Enabled == provider.Enabled && old.ProbeEnabled == provider.ProbeEnabled {
			provider.ConnectionRevision = old.ConnectionRevision
		} else {
			provider.ConnectionRevision = rand.Text()
		}
	}
	return result
}

func normalizeBaseURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}
