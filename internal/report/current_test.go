package report

import (
	"reflect"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
)

func TestCurrentConfigProjection(t *testing.T) {
	cfg := config.Config{ShowErrorDetail: true, Providers: []config.ProviderConfig{
		{ID: "one", Name: "Before", Enabled: true, ProbeEnabled: true},
		{ID: "two", Name: "Removed", Enabled: true, ProbeEnabled: true},
	}}
	original, _ := Build(cfg, []probe.Result{
		{ProviderID: "one", Model: "old", Status: "error", Error: "secret"},
		{ProviderID: "one", Model: "keep", Status: "ok"},
		{ProviderID: "two", Model: "other", Status: "error"},
	}, []probe.ProviderError{{ProviderID: "two", Error: "secret"}}, nil, time.Now())
	current := config.AdminConfigFromConfig(cfg)
	current.Providers = current.Providers[:1]
	current.Providers[0].Name = "After"
	current.Providers[0].Models = []string{"keep"}
	current.Settings.ShowErrorDetail = false
	got := WithConfig(original, current)
	if got.Total != 1 || got.OKCount != 1 || got.ErrorCount != 0 || got.ProviderCount != 1 || len(got.ProviderErrors) != 0 {
		t.Fatalf("stale counts/providers: %+v", got)
	}
	if got.Providers[0].ProviderName != "After" || got.Providers[0].CurrentModel != "keep" || got.GeneratedAt != original.GeneratedAt {
		t.Fatalf("wrong metadata: %+v", got)
	}
	if original.Total != 3 || original.Providers[0].Results[0].Error != "secret" || original.Providers[0].ProviderName == "After" {
		t.Fatal("projection modified shared original")
	}
	if repeated := WithConfig(got, current); !reflect.DeepEqual(repeated, got) {
		t.Fatal("projection must be idempotent")
	}
	current.Providers[0].Models = []string{"keep", "new", "skipped"}
	current.Settings.SkipModels = []string{"skipped"}
	got = WithConfig(original, current)
	if got.UnknownCount != 1 || got.Total != 2 || got.Providers[0].Results[1].CheckedAt != "" {
		t.Fatalf("new model is missing or falsely checked: %+v", got)
	}
	current.Providers[0].ProbeEnabled = false
	got = WithConfig(original, current)
	if got.Providers[0].Status != "paused" || got.Total != 0 {
		t.Fatal("paused provider retained live results")
	}
	current.Providers[0].Enabled = false
	if got = WithConfig(original, current); got.State != "unconfigured" || got.Total != 0 {
		t.Fatal("disabled provider still visible")
	}
	current.Providers[0].Enabled, current.Providers[0].ProbeEnabled = true, true
	got = WithConfig(Report{}, current)
	if got.State != "pending" || got.GeneratedAt != "" || got.Providers[0].Status != "unknown" {
		t.Fatalf("first run is not pending: %+v", got)
	}
}

func TestStaleConnectionDropsFailuresButKeepsHistoricalMeasurements(t *testing.T) {
	cfg := config.Config{ShowErrorDetail: true, Providers: []config.ProviderConfig{{
		ID: "p", Type: "openai", ConnectionRevision: "old", Enabled: true, ProbeEnabled: true,
	}}}
	original, _ := Build(cfg, []probe.Result{{
		ProviderID: "p", Model: "discovered", Status: "error", Error: "old failure", LatencyMS: 50,
		HistoryKey: "p::discovered",
	}}, []probe.ProviderError{{ProviderID: "p", Error: "old listing failure"}}, nil, time.Now())
	current := config.AdminConfigFromConfig(cfg)
	current.Providers[0].ConnectionRevision = "new"
	got := WithConfig(original, current)
	if len(got.ProviderErrors) != 0 || got.ErrorCount != 0 || got.UnknownCount != 1 ||
		got.Providers[0].CheckedAt != "" || got.Providers[0].Results[0].Error != "" {
		t.Fatalf("old connection failure survived: %+v", got)
	}
	if !reflect.DeepEqual(got.Providers[0].Results[0].History, original.Providers[0].Results[0].History) ||
		original.Providers[0].Results[0].Error == "" || original.Providers[0].ConnectionRevision != "old" {
		t.Fatal("historical data or shared source mutated")
	}
	if !reflect.DeepEqual(got, WithConfig(got, current)) {
		t.Fatal("invalidation not idempotent")
	}
	current.Providers[0].ConnectionRevision = ""
	if WithConfig(got, current).UnknownCount != 1 {
		t.Fatal("generation downgrade recovered old measurements")
	}
}
