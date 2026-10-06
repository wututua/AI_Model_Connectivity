package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
	"cg/internal/report"
)

func TestUnobservedProviderDoesNotSendRecovery(t *testing.T) {
	value := report.WithConfig(report.Report{GeneratedAt: time.Now().UTC().Format(time.RFC3339)}, config.AdminConfig{
		Providers: []config.SafeProviderConfig{{ID: "p1", Name: "Primary", Enabled: true, ProbeEnabled: true}},
	})
	for _, scope := range []config.Config{
		{}, {NotifyProviders: []string{"Primary"}}, {NotifyModels: []string{"p1/m"}},
	} {
		var sent atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { sent.Add(1) }))
		scope.NotifyWebhookURL, scope.NotifyOnRecovery = server.URL, true
		before := State{Status: "error", SentAt: time.Now().Add(-time.Hour)}
		store := &memoryState{state: before}
		err := New(scope, store).SendIfNeeded(context.Background(), value)
		server.Close()
		if err != nil || sent.Load() != 0 || store.state != before {
			t.Fatalf("unobserved scope sent recovery or consumed state: sent=%d state=%+v err=%v", sent.Load(), store.state, err)
		}
	}
}

func TestModelScopeExcludesUnrelatedDiscoveryFailures(t *testing.T) {
	value := report.Report{
		Providers: []report.ProviderReport{
			{ProviderID: "p1", ProviderName: "Primary", Status: "ok", Results: []report.ModelResult{{
				Result: probe.Result{ProviderID: "p1", ProviderName: "Primary", Model: "m", Status: "ok"},
			}}},
			{ProviderID: "p2", ProviderName: "Other", Status: "error"},
		},
		ProviderErrors: []probe.ProviderError{{ProviderID: "p2", Error: "discovery failed"}},
	}
	for _, models := range [][]string{{"p1/m"}, {"Primary::m"}, {"m"}} {
		filtered := filterReport(value, nil, models)
		if len(filtered.ProviderErrors) != 0 || filtered.OKCount != 1 || alertState(filtered) != "ok" {
			t.Errorf("unrelated discovery failure leaked into %v: %+v", models, filtered)
		}
	}
	if len(value.ProviderErrors) != 1 || len(value.Providers) != 2 {
		t.Fatal("filter mutated the original report")
	}
}

func TestModelScopeRetainsItsOwnDiscoveryFailure(t *testing.T) {
	value := report.Report{
		Providers:      []report.ProviderReport{{ProviderID: "p1", ProviderName: "Primary", Status: "error"}},
		ProviderErrors: []probe.ProviderError{{ProviderID: "p1", Error: "discovery failed"}},
	}
	for _, selector := range []string{"p1/m", "PRIMARY::m"} {
		filtered := filterReport(value, nil, []string{selector})
		if !reflect.DeepEqual(filtered.ProviderErrors, value.ProviderErrors) || alertState(filtered) != "error" {
			t.Errorf("qualified scope lost its own discovery failure: %+v", filtered)
		}
	}
	filtered := filterReport(value, []string{"different"}, []string{"p1/m"})
	if len(filtered.ProviderErrors) != 0 {
		t.Fatal("model scope bypassed the provider filter")
	}
	value.Providers[0].Results = []report.ModelResult{{Result: probe.Result{
		ProviderID: "p1", Model: "m", Status: "unknown",
	}}}
	filtered = filterReport(value, nil, []string{"m"})
	if len(filtered.ProviderErrors) != 1 || filtered.UnknownCount != 1 || alertState(filtered) != "error" {
		t.Fatal("bare model scope lost its known discovery failure")
	}
}

func TestRecoveryRequiresObservedHealthyScope(t *testing.T) {
	var sent atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { sent.Add(1) }))
	defer server.Close()
	before := State{Status: "error", SentAt: time.Now().Add(-time.Hour)}
	store := &memoryState{state: before}
	client := New(config.Config{NotifyWebhookURL: server.URL, NotifyOnRecovery: true}, store)
	for _, value := range []report.Report{
		{},
		{Providers: []report.ProviderReport{{Status: "paused"}}},
		{OKCount: 1, Providers: []report.ProviderReport{{Status: "ok"}, {Status: "unknown"}}},
	} {
		if err := client.SendIfNeeded(context.Background(), value); err != nil || sent.Load() != 0 || store.state != before {
			t.Fatalf("incomplete scope consumed recovery: state=%+v, err=%v", store.state, err)
		}
	}
	healthy := report.Report{OKCount: 1, Total: 1, Providers: []report.ProviderReport{{Status: "ok"}}}
	for i := 0; i < 2; i++ {
		if err := client.SendIfNeeded(context.Background(), healthy); err != nil {
			t.Fatal(err)
		}
	}
	if sent.Load() != 1 || store.state.Status != "ok" || !store.state.SentAt.After(before.SentAt) {
		t.Fatalf("healthy recovery not delivered exactly once: sent=%d state=%+v", sent.Load(), store.state)
	}
}
