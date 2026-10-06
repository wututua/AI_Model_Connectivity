package config

import (
	"encoding/json"
	"testing"
)

func TestProbeValidationAndRevisions(t *testing.T) {
	for _, value := range []ProbeOptions{{Protocol: "invalid"}, {MaxTokens: -1}, {Temperature: 3}, {TimeoutSeconds: -1}, {TokenLimitField: "injected"}} {
		if value.Validate() == nil {
			t.Fatalf("invalid options accepted: %+v", value)
		}
	}
	base := ProviderConfig{ID: "p", BaseURL: "https://example.test/v1", ConnectionRevision: "original", Enabled: true, ProbeEnabled: true}
	changed := base
	changed.Probe.Protocol = "responses"
	if ReconcileProviderRevisions([]ProviderConfig{base}, []ProviderConfig{changed})[0].ConnectionRevision == "original" {
		t.Fatal("protocol change kept old result revision")
	}
	changed = base
	changed.Group = "new"
	changed.Tags = []string{"prod"}
	if ReconcileProviderRevisions([]ProviderConfig{base}, []ProviderConfig{changed})[0].ConnectionRevision != "original" {
		t.Fatal("metadata invalidated result")
	}
}

func TestNewSettingsLegacyDefaultsAndRoundTrip(t *testing.T) {
	var legacy RuntimeConfig
	if err := json.Unmarshal([]byte(`{"settings":{"timeout_seconds":30},"providers":[{"id":"p"}]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Settings.OperationsSettings != (OperationsSettings{}) || legacy.Providers[0].Probe != (ProbeOptions{}) {
		t.Fatal("legacy defaults not safe")
	}
	cfg := defaults()
	cfg.DailyRequestLimit = 123
	cfg.NotifyFailureThreshold = 3
	cfg.Providers = []ProviderConfig{{ID: "p", Probe: ProbeOptions{Protocol: "responses", Stream: true}, Tags: []string{"test"}, Group: "demo"}}
	encoded, _ := json.Marshal(RuntimeConfigFromConfig(cfg))
	var saved RuntimeConfig
	if err := json.Unmarshal(encoded, &saved); err != nil {
		t.Fatal(err)
	}
	got := ApplyRuntimeConfig(defaults(), saved)
	if got.DailyRequestLimit != 123 || got.NotifyFailureThreshold != 3 || !got.Providers[0].Probe.Stream || got.Providers[0].Group != "demo" {
		t.Fatal("configuration lost")
	}
	for _, value := range []OperationsSettings{{DailyRequestLimit: -1}, {NotifyFailureThreshold: 101}, {MaintenanceStart: "invalid"}, {MaintenanceStart: "2026-10-06T00:00:00Z", MaintenanceEnd: "2026-10-05T00:00:00Z"}} {
		if value.Validate() == nil {
			t.Fatal("invalid operations settings accepted", value)
		}
	}
}
