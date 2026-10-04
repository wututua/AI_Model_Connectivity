package report

import (
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
)

func TestMergeProviderPreservesOtherProviders(t *testing.T) {
	base := Report{
		GeneratedAt: "2026-09-05 10:00:00",
		Providers: []ProviderReport{
			{ProviderID: "p1", Results: []ModelResult{{Result: probeResult("p1", "m1", "error")}}, ErrorCount: 1, ModelCount: 1, Status: "error"},
			{ProviderID: "p2", Results: []ModelResult{{Result: probeResult("p2", "m2", "ok")}}, OKCount: 1, ModelCount: 1, Status: "ok"},
		},
		ProviderErrors: []probe.ProviderError{{ProviderID: "p3", Error: "models unavailable"}},
	}
	update := Report{
		GeneratedAt: "2026-09-05 10:01:00",
		Providers: []ProviderReport{
			{ProviderID: "p1", Results: []ModelResult{{Result: probeResult("p1", "m1", "ok")}}, OKCount: 1, ModelCount: 1, Status: "ok"},
		},
	}

	merged := MergeProvider(base, update, "p1")
	if len(merged.Providers) != 2 {
		t.Fatalf("expected two providers, got %d", len(merged.Providers))
	}
	if merged.Providers[0].ProviderID != "p1" || merged.Providers[0].OKCount != 1 {
		t.Fatalf("provider rerun did not replace p1: %+v", merged.Providers)
	}
	if merged.Providers[1].ProviderID != "p2" || merged.Providers[1].OKCount != 1 {
		t.Fatalf("provider p2 was not preserved: %+v", merged.Providers)
	}
	if merged.Total != 2 || merged.OKCount != 2 || merged.ErrorCount != 0 || merged.OverallStatus != "DEGRADED" {
		t.Fatalf("unexpected merged counters: %+v", merged)
	}
}

func TestReportTimestampsAndProviderFreshness(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	cfg := config.Config{HistorySize: 10, MaxHistoryRecords: 100, StatsWindowDays: 7, AutoCheckIntervalMaxHours: 12}
	value, history := Build(cfg, []probe.Result{
		{ProviderID: "p1", Model: "m1", HistoryKey: "p1::m1", Status: "ok", CheckedAt: now.Format(time.RFC3339)},
		{ProviderID: "p1", Model: "m2", HistoryKey: "p1::m2", Status: "slow", CheckedAt: now.Add(-time.Minute).Format(time.RFC3339)},
	}, nil, nil, now)
	if _, err := time.Parse(time.RFC3339, value.GeneratedAt); err != nil {
		t.Fatalf("ambiguous generated_at: %q", value.GeneratedAt)
	}
	if value.Providers[0].CheckedAt != now.Format(time.RFC3339) || history["p1::m2"][0].CheckedAt != now.Add(-time.Minute).Format(time.RFC3339) {
		t.Fatal("report discarded actual probe timestamps")
	}
	if value.StaleAfterSeconds < 12*3600 {
		t.Fatal("report becomes stale before next scheduled check")
	}
	update, _ := Build(cfg, []probe.Result{{ProviderID: "p2", Model: "m3", Status: "ok"}}, nil, nil, now)
	merged := MergeProvider(value, update, "p2")
	if merged.Providers[0].CheckedAt != value.Providers[0].CheckedAt {
		t.Fatal("partial rerun refreshed untouched provider")
	}
}

func TestErrorVisibilityDoesNotMutateSource(t *testing.T) {
	original := Report{
		Providers:      []ProviderReport{{Results: []ModelResult{{Result: probe.Result{Error: "private-model"}}}}},
		ProviderErrors: []probe.ProviderError{{Error: "private-provider"}},
	}
	hidden := WithErrorVisibility(original, false)
	if hidden.ProviderErrors[0].Error != "" || hidden.Providers[0].Results[0].Error != "" {
		t.Fatal("details were not hidden")
	}
	if original.ProviderErrors[0].Error == "" || original.Providers[0].Results[0].Error == "" {
		t.Fatal("redaction mutated shared report")
	}
}

func TestDiscoveryGapsRespectConfiguredSelection(t *testing.T) {
	cfg := config.Config{
		Providers:  []config.ProviderConfig{{ID: "p1", Enabled: true, ProbeEnabled: true}},
		SkipModels: []string{"skip"}, MaxModelsPerProvider: 1,
	}
	previous := Report{Providers: []ProviderReport{
		{ProviderID: "p1", Results: []ModelResult{{Result: probe.Result{Model: "skip"}}, {Result: probe.Result{Model: "keep"}}, {Result: probe.Result{Model: "other"}}}},
		{ProviderID: "removed", Results: []ModelResult{{Result: probe.Result{Model: "m1"}}}},
	}}
	failures := []probe.ProviderError{{ProviderID: "p1", Error: "discovery failed"}, {ProviderID: "removed"}}
	results := WithDiscoveryGaps(cfg, nil, failures, previous)
	if len(results) != 1 || results[0].Model != "keep" || results[0].Status != "unknown" || results[0].HistoryKey != "p1::keep" || !results[0].IsCurrent {
		t.Fatalf("wrong discovery gaps: %+v", results)
	}
	if len(WithDiscoveryGaps(cfg, nil, failures, Report{})) != 0 {
		t.Fatal("first failure invented model history")
	}
}

func TestFailedAndPausedProvidersHaveEmptyResultArrays(t *testing.T) {
	cfg := config.Config{Providers: []config.ProviderConfig{
		{ID: "failed", Enabled: true, ProbeEnabled: true},
		{ID: "paused", Enabled: true},
	}}
	value, _ := Build(cfg, nil, []probe.ProviderError{{ProviderID: "failed", Error: "unavailable"}}, nil, time.Now())
	if value.OverallStatus != "DEGRADED" || len(value.Providers) != 2 {
		t.Fatalf("missing provider states: %+v", value)
	}
	for _, group := range value.Providers {
		if group.Results == nil {
			t.Errorf("%s results serialize as null", group.ProviderID)
		}
		if group.ProviderID == "failed" && group.Status != "error" {
			t.Error("failed provider is not in error state")
		}
		if group.ProviderID == "paused" && group.Status != "paused" {
			t.Error("paused provider is not paused")
		}
	}
}

func TestMergeProviderFailurePreservesProviderWithoutDuplicatingErrors(t *testing.T) {
	base := Report{GeneratedAt: "before", Providers: []ProviderReport{
		{ProviderID: "p1", ProviderName: "First", Results: []ModelResult{{Result: probeResult("p1", "m1", "ok")}}, OKCount: 1},
		{ProviderID: "p2", Results: []ModelResult{{Result: probeResult("p2", "m2", "ok")}}, OKCount: 1},
	}, ProviderErrors: []probe.ProviderError{{ProviderID: "p1", Error: "old"}}}
	update := Report{GeneratedAt: "after", ProviderErrors: []probe.ProviderError{{ProviderID: "p1", Error: "new"}}}
	merged := MergeProvider(base, update, "p1")
	if len(merged.Providers) != 2 || merged.Providers[0].Status != "error" || merged.Providers[0].ProviderName != "First" {
		t.Fatalf("provider disappeared: %+v", merged.Providers)
	}
	if len(merged.ProviderErrors) != 1 || merged.ProviderErrors[0].Error != "new" {
		t.Fatalf("duplicate or stale errors: %+v", merged.ProviderErrors)
	}
	if merged.Total != 1 || merged.OKCount != 1 || merged.OverallStatus != "DEGRADED" {
		t.Fatalf("stale counters: %+v", merged)
	}
}

func probeResult(providerID, model, status string) probe.Result {
	return probe.Result{ProviderID: providerID, Model: model, Status: status}
}

func TestProviderErrorsRespectErrorVisibilityWithoutMutatingInput(t *testing.T) {
	failures := []probe.ProviderError{{ProviderID: "test", Error: "sensitive upstream response"}}
	value, _ := Build(config.Config{}, nil, failures, nil, time.Now())
	if value.ProviderErrors[0].Error != "" || failures[0].Error == "" || value.OverallStatus != "DEGRADED" {
		t.Fatal("provider errors were leaked or input was modified")
	}
}
