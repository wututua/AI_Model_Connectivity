package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
	"cg/internal/report"
	"cg/internal/storage"
	"cg/internal/web"
)

func awaitTask(t *testing.T, app *application, id int64) storage.CheckTask {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		task, err := app.GetTask(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status != "running" && !app.RunningState().Running {
			return task
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("task did not finish")
	return storage.CheckTask{}
}

func TestSelectedCheckPreservesUnselectedHistoryAndAlerts(t *testing.T) {
	app := testApplication(t)
	ctx := context.Background()
	requests := make(chan string, 10)
	var notifications atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/notify" {
			notifications.Add(1)
			return
		}
		var payload struct {
			Model string `json:"model"`
		}
		json.NewDecoder(r.Body).Decode(&payload)
		requests <- payload.Model
		fmt.Fprint(w, `{"choices":[{"message":{"content":"pang"}}],"usage":{"total_tokens":9}}`)
	}))
	defer server.Close()
	app.cfg.Providers = []config.ProviderConfig{{ID: "p1", Name: "first", BaseURL: server.URL, Models: []string{"m1", "m2"}, Enabled: true, ProbeEnabled: true}}
	app.cfg.NotifyWebhookURL = server.URL + "/notify"
	app.cfg.NotifyOnRecovery = true
	before := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	previous, _ := report.Build(app.cfg, []probe.Result{
		{ProviderID: "p1", Model: "m1", Status: "error", CheckedAt: before},
		{ProviderID: "p1", Model: "m2", Status: "ok", CheckedAt: before, LatencyMS: 123},
	}, nil, nil, time.Now())
	previous = report.WithConfig(previous, config.AdminConfigFromConfig(app.cfg))
	if err := app.store.SaveLatestReport(ctx, previous); err != nil {
		t.Fatal(err)
	}
	old := previous.Providers[0].Results[1]
	task, err := app.StartSelectedCheck(ctx, config.CheckSelection{FailedOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if done := awaitTask(t, app, task.ID); done.Status != "success" || done.Total != 1 || done.Kind != "failed" {
		t.Fatal(done)
	}
	if got := <-requests; got != "m1" {
		t.Fatal("wrong model called", got)
	}
	if len(requests) != 0 {
		t.Fatal("extra model request")
	}
	latest, err := app.store.LatestReport(ctx)
	if err != nil || latest.OKCount != 2 || latest.ErrorCount != 0 {
		t.Fatal(latest, err)
	}
	if latest.Providers[0].Results[0].CheckedAt == before {
		t.Fatal("selected result not refreshed")
	}
	if got := latest.Providers[0].Results[1]; !reflect.DeepEqual(got, old) {
		t.Fatalf("unselected model altered: %+v vs %+v", got, old)
	}
	if notifications.Load() != 0 {
		t.Fatal("partial check changed alert evidence")
	}
	history, _ := app.store.LoadHistory(ctx, 100, 7)
	if len(history) != 1 {
		t.Fatal("unselected history reinserted", history)
	}
	if _, err := app.StartSelectedCheck(ctx, config.CheckSelection{FailedOnly: true}); !errors.Is(err, web.ErrInvalidSelection) {
		t.Fatal("empty retry accepted", err)
	}
	if _, err := app.StartSelectedCheck(ctx, config.CheckSelection{Targets: []config.ModelTarget{{ProviderID: "p1", Model: "unknown"}}}); !errors.Is(err, web.ErrInvalidSelection) {
		t.Fatal("unknown model accepted", err)
	}
}

func TestBudgetLimitsRequestsAndPreservesLastReport(t *testing.T) {
	app := testApplication(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"pang"}}],"usage":{"total_tokens":3}}`)
	}))
	defer server.Close()
	app.cfg.DailyRequestLimit = 1
	app.cfg.Concurrency = 4
	app.cfg.ProviderConcurrency = 4
	app.cfg.Providers = []config.ProviderConfig{{ID: "p", BaseURL: server.URL, Models: []string{"a", "b", "c"}, Enabled: true, ProbeEnabled: true}}
	ctx := context.Background()
	if err := app.store.SaveLatestReport(ctx, report.Report{GeneratedAt: "previous"}); err != nil {
		t.Fatal(err)
	}
	_, err := app.check(ctx)
	if !errors.Is(err, storage.ErrBudgetExceeded) || calls.Load() != 1 {
		t.Fatal(calls.Load(), err)
	}
	latest, _ := app.store.LatestReport(ctx)
	if latest.GeneratedAt != "previous" {
		t.Fatal("budget interruption replaced report")
	}
	usage, _ := app.store.LoadBillingSummary(ctx, 1)
	if usage.TotalTokens != 3 || usage.TotalProbeCount != 1 {
		t.Fatal("real usage lost", usage)
	}
	if _, err := app.StartCheck(ctx, ""); !errors.Is(err, storage.ErrBudgetExceeded) {
		t.Fatal("exhausted budget accepted", err)
	}
}

func TestProgressAndProviderTimeout(t *testing.T) {
	app := testApplication(t)
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	app.cfg.Providers = []config.ProviderConfig{{ID: "p", BaseURL: server.URL, Models: []string{"a"}, Enabled: true, ProbeEnabled: true, Probe: config.ProbeOptions{TimeoutSeconds: 0.3}}}
	task, err := app.StartCheck(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	<-started
	state := app.RunningState()
	if state.Progress.Phase != "probing" || state.Progress.Total != 1 || len(state.Progress.Active) != 1 || state.Progress.Active[0].Model != "a" {
		t.Fatal(state)
	}
	if done := awaitTask(t, app, task.ID); done.ErrorCount != 1 || done.ElapsedMS > 4000 {
		t.Fatal("provider timeout ignored", done)
	}
}

func TestProviderBatchAtomicAndMetadata(t *testing.T) {
	app := testApplication(t)
	ctx := context.Background()
	app.cfg.Providers = []config.ProviderConfig{{ID: "a", Enabled: true, ProbeEnabled: true}, {ID: "b", Enabled: true, ProbeEnabled: true}}
	if _, err := app.BatchProviders(ctx, config.ProviderBatch{IDs: []string{"a", "missing"}, Action: "pause"}); err == nil {
		t.Fatal("missing provider accepted")
	}
	if !app.currentConfig().Providers[0].ProbeEnabled {
		t.Fatal("partial batch committed")
	}
	if _, err := app.BatchProviders(ctx, config.ProviderBatch{IDs: []string{"a", "b"}, Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.BatchProviders(ctx, config.ProviderBatch{IDs: []string{"a"}, Action: "group", Group: "production"}); err != nil {
		t.Fatal(err)
	}
	cfg := app.currentConfig()
	if cfg.Providers[0].ProbeEnabled || cfg.Providers[1].ProbeEnabled || cfg.Providers[0].Group != "production" {
		t.Fatal(cfg.Providers)
	}
	persisted, _, err := app.store.LoadRuntimeConfig(ctx)
	if err != nil || persisted.Providers[0].Group != "production" {
		t.Fatal("metadata not persisted")
	}
}

func TestSelectedCheckUsesProjectedModelScope(t *testing.T) {
	for _, scope := range []struct {
		name string
		skip []string
		max  int
	}{
		{name: "skip", skip: []string{"m2"}},
		{name: "provider skip", skip: []string{"p/m2"}},
		{name: "provider name skip", skip: []string{"Provider::m2"}},
		{name: "limit", max: 1},
	} {
		t.Run(scope.name, func(t *testing.T) {
			app := testApplication(t)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var input struct{ Model string }
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Model != "m1" {
					t.Error("unexpected request", input.Model, err)
				}
				fmt.Fprint(w, `{"choices":[{"message":{"content":"pang"}}]}`)
			}))
			defer server.Close()
			app.cfg.Providers = []config.ProviderConfig{{
				ID: "p", Name: "Provider", BaseURL: server.URL, Models: []string{"m1", "m2"},
				Enabled: true, ProbeEnabled: true,
			}}
			app.cfg.SkipModels, app.cfg.MaxModelsPerProvider = scope.skip, scope.max
			current := report.WithConfig(report.Report{}, config.AdminConfigFromConfig(app.cfg))
			models := current.Providers[0].Results
			if len(models) != 1 || models[0].Model != "m1" || models[0].Status != "unknown" {
				t.Fatal("first-check report lost the configured selection", models)
			}
			ctx := context.Background()
			_, err := app.StartSelectedCheck(ctx, config.CheckSelection{Targets: []config.ModelTarget{{ProviderID: "p", Model: "m2"}}})
			if !errors.Is(err, web.ErrInvalidSelection) || calls.Load() != 0 {
				t.Fatal("excluded model accepted", err)
			}
			task, err := app.StartSelectedCheck(ctx, config.CheckSelection{Targets: []config.ModelTarget{{ProviderID: "p", Model: models[0].Model}}})
			if err != nil {
				t.Fatal(err)
			}
			if result := awaitTask(t, app, task.ID); result.Status != "success" || result.Total != 1 || calls.Load() != 1 {
				t.Fatal("projected model selection failed", result)
			}
		})
	}
}
