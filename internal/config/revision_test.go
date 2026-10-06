package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProviderRevisionsPersistWithoutDerivingFromSecrets(t *testing.T) {
	original := []ProviderConfig{{ID: "p", Name: "Before", Type: "openai", BaseURL: "https://old.invalid/v1", APIKey: "fixture-secret", Enabled: true, ProbeEnabled: true}}
	first := ReconcileProviderRevisions(nil, original)
	if first[0].ConnectionRevision == "" || original[0].ConnectionRevision != "" {
		t.Fatal("missing generation or input mutated")
	}
	data, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var restored []ProviderConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	restored[0].Name = "After"
	restored[0].BaseURL += "/"
	same := ReconcileProviderRevisions(first, restored)
	if same[0].ConnectionRevision != first[0].ConnectionRevision {
		t.Fatal("restart, rename or trailing slash invalidated unchanged connection")
	}
	other := ReconcileProviderRevisions(nil, original)
	if other[0].ConnectionRevision == first[0].ConnectionRevision {
		t.Fatal("generation reused for identical credentials")
	}
	for _, mutate := range []func(*ProviderConfig){
		func(p *ProviderConfig) { p.BaseURL = "https://new.invalid/v1" },
		func(p *ProviderConfig) { p.APIKey = "different" },
		func(p *ProviderConfig) { p.Type = "anthropic" },
		func(p *ProviderConfig) { p.Enabled = false },
		func(p *ProviderConfig) { p.ProbeEnabled = false },
	} {
		next := append([]ProviderConfig(nil), first...)
		mutate(&next[0])
		changed := ReconcileProviderRevisions(first, next)
		reverted := ReconcileProviderRevisions(changed, first)
		if changed[0].ConnectionRevision == first[0].ConnectionRevision || reverted[0].ConnectionRevision == first[0].ConnectionRevision {
			t.Fatal("changed/reverted connection retained the old generation")
		}
	}
	safe, err := json.Marshal(SafeProviders(first))
	if err != nil || strings.Contains(string(safe), "fixture-secret") || strings.Contains(string(safe), first[0].ConnectionRevision) {
		t.Fatalf("export exposed internal state: %s, %v", safe, err)
	}
}

func TestProviderUpdateRequiresExplicitKeyForNewEndpoint(t *testing.T) {
	existing := ProviderConfig{ID: "p", BaseURL: "https://old.invalid/v1", APIKey: "fixture-secret"}
	for _, tc := range []struct {
		name   string
		update ProviderUpdate
		key    string
		fail   bool
	}{
		{"same", ProviderUpdate{BaseURL: existing.BaseURL + "/"}, existing.APIKey, false},
		{"changed", ProviderUpdate{BaseURL: "https://new.invalid/v1"}, "", true},
		{"explicit", ProviderUpdate{BaseURL: "https://new.invalid/v1", APIKey: "new-key"}, "new-key", false},
		{"cleared", ProviderUpdate{BaseURL: "https://new.invalid/v1", ClearAPIKey: true}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ApplyProviderUpdate(existing, tc.update)
			if (err != nil) != tc.fail || !tc.fail && got.APIKey != tc.key {
				t.Fatalf("unexpected update result: err=%v", err)
			}
		})
	}
}
