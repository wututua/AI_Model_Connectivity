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
