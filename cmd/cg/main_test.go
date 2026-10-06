package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
	"cg/internal/report"
	"cg/internal/storage"
	"cg/internal/web"
)

func testApplication(t *testing.T) *application {
	t.Helper()
	directory := t.TempDir()
	store, err := storage.NewSQLite(context.Background(), filepath.Join(directory, "test.sqlite"), directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	cfg := config.Config{TimeoutSeconds: 5, ModelListTimeoutSeconds: 5, SlowThresholdMS: 10000, Concurrency: 1, ProviderConcurrency: 1, EnableHistory: true, StatsWindowDays: 7, HistorySize: 10, MaxHistoryRecords: 100}
	return &application{cfg: cfg, baseCfg: cfg, store: store, schedulerWake: make(chan struct{}, 1)}
}

func TestCanceledCheckDoesNotReplaceReportOrNotify(t *testing.T) {
	app := testApplication(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var notifications atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/notify" {
			notifications.Add(1)
			return
		}
		io.Copy(io.Discard, request.Body)
		close(started)
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	app.cfg.Providers = []config.ProviderConfig{{ID: "p1", BaseURL: server.URL, Models: []string{"m1"}, Enabled: true, ProbeEnabled: true}}
	app.cfg.NotifyWebhookURL = server.URL + "/notify"
	previous := report.Report{GeneratedAt: "before"}
	if err := app.store.SaveLatestReport(context.Background(), previous); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { _, err := app.check(context.Background()); finished <- err }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("probe did not start")
	}
	if !app.StopCheck() {
		t.Fatal("stop did not cancel the task")
	}
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want cancellation", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("probe did not stop")
	}
	latest, err := app.store.LatestReport(context.Background())
	if err != nil || latest.GeneratedAt != previous.GeneratedAt {
		t.Fatalf("latest report replaced: %+v, %v", latest, err)
	}
	history, err := app.store.LoadHistory(context.Background(), 100, 7)
	if err != nil || len(history) != 0 {
		t.Fatalf("partial history saved: %+v, %v", history, err)
	}
	tasks, err := app.ListTasks(context.Background(), storage.TaskQuery{})
	if err != nil || len(tasks) != 1 || tasks[0].Status != "canceled" {
		t.Fatalf("wrong task status: %+v, %v", tasks, err)
	}
	if notifications.Load() != 0 {
		t.Fatal("cancellation triggered a notification")
	}
}

func TestFailedProviderRerunSavesFailureAndPreservesOthers(t *testing.T) {
	app := testApplication(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
		writer.Write([]byte(`{"error":{"message":"unavailable"}}`))
	}))
	defer server.Close()
	app.cfg.Providers = []config.ProviderConfig{
		{ID: "p1", BaseURL: server.URL, Enabled: true, ProbeEnabled: true},
		{ID: "p2", Enabled: true, ProbeEnabled: true},
	}
	previous, _ := report.Build(app.cfg, []probe.Result{{ProviderID: "p1", Model: "m1", Status: "ok"}, {ProviderID: "p2", Model: "m2", Status: "ok"}}, nil, nil, time.Now())
	if err := app.store.SaveLatestReport(context.Background(), previous); err != nil {
		t.Fatal(err)
	}
	value, err := app.CheckProvider(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Providers) != 2 || value.Providers[0].Status != "error" || value.Providers[1].OKCount != 1 || len(value.ProviderErrors) != 1 {
		t.Fatalf("incorrect rerun report: %+v", value)
	}
	latest, err := app.store.LatestReport(context.Background())
	if err != nil || latest.OverallStatus != "DEGRADED" {
		t.Fatalf("failure was not saved: %+v, %v", latest, err)
	}
}

func TestEmptySecretUpdatePreservesAndExplicitClearRemoves(t *testing.T) {
	app := testApplication(t)
	app.cfg.NotifyWebhookURL = "https://example.test/notify?token=secret"
	settings := config.SettingsFromConfig(app.cfg)
	settings.NotifyWebhookURL = ""
	settings.NotifyWebhookURLSet = false
	updated, err := app.UpdateSettings(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Settings.NotifyWebhookURLSet || updated.Settings.NotifyWebhookURL != "" {
		t.Fatal("secret was removed or exposed")
	}
	settings.ClearNotifyWebhookURL = true
	if _, err := app.UpdateSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	stored, _, err := app.store.LoadRuntimeConfig(context.Background())
	if err != nil || stored.Settings.NotifyWebhookURL != "" {
		t.Fatalf("secret not cleared: %v", err)
	}
}

func TestProviderTaskCountsOnlyRerunProvider(t *testing.T) {
	app := testApplication(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"pang"}}]}`))
	}))
	defer server.Close()
	app.cfg.Providers = []config.ProviderConfig{
		{ID: "p1", Enabled: true, ProbeEnabled: true, BaseURL: server.URL, Models: []string{"m1"}},
		{ID: "p2", Enabled: true, ProbeEnabled: true, BaseURL: server.URL, Models: []string{"m2"}},
	}
	if _, err := app.check(context.Background()); err != nil {
		t.Fatal(err)
	}
	value, err := app.CheckProvider(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := app.ListTasks(context.Background(), storage.TaskQuery{Limit: 1})
	if err != nil || len(tasks) != 1 || tasks[0].Total != 1 || tasks[0].OKCount != 1 || value.Total != 2 {
		t.Fatalf("rerun counters: tasks=%+v report=%+v err=%v", tasks, value, err)
	}
}

func TestHidingErrorsPublishesOldReportAndSanitizesMerge(t *testing.T) {
	app := testApplication(t)
	app.broker = web.NewBroker()
	updates, unsubscribe := app.broker.Subscribe()
	defer unsubscribe()
	app.cfg.ShowErrorDetail = true
	app.cfg.Providers = []config.ProviderConfig{
		{ID: "p1", Enabled: true, ProbeEnabled: false, BaseURL: "https://example.test"},
		{ID: "p2", Enabled: true, ProbeEnabled: true, BaseURL: "https://example.test"},
	}
	app.cfg.Providers = config.ReconcileProviderRevisions(nil, app.cfg.Providers)
	base, _ := report.Build(app.cfg, []probe.Result{
		{ProviderID: "p2", Model: "m2", Status: "error", Error: "sensitive-detail", HistoryKey: "p2::m2"},
	}, []probe.ProviderError{{ProviderID: "p2", Error: "sensitive-discovery"}}, nil, time.Now())
	if err := app.store.SaveLatestReport(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	settings := config.SettingsFromConfig(app.cfg)
	settings.ShowErrorDetail = false
	if _, err := app.UpdateSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-updates:
		if value.ProviderErrors[0].Error != "" || value.Providers[0].Results[0].Error != "" {
			t.Fatal("settings update published sensitive details")
		}
	default:
		t.Fatal("settings update did not refresh subscribers")
	}
	if _, err := app.CheckProvider(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	latest, err := app.store.LatestReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range latest.Providers {
		for _, item := range group.Results {
			if item.Error != "" {
				t.Fatalf("merged report leaks %q", item.Error)
			}
		}
	}
	if latest.ProviderErrors[0].Error != "" {
		t.Fatal("merged discovery error was not hidden")
	}
}

func TestDiscoveryOutageRecordsUnknownWithoutCharging(t *testing.T) {
	app := testApplication(t)
	var outage atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			if outage.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				w.Write([]byte(`{"data":[{"id":"m1"}]}`))
			}
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"pang"}}],"usage":{"total_tokens":10}}`))
	}))
	defer server.Close()
	app.cfg.Providers = []config.ProviderConfig{{ID: "p1", Enabled: true, ProbeEnabled: true, BaseURL: server.URL}}
	for _, failed := range []bool{false, true, false} {
		outage.Store(failed)
		value, err := app.check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if failed && (value.UnknownCount != 1 || value.Providers[0].Results[0].Status != "unknown") {
			t.Fatalf("missing unknown sample: %+v", value)
		}
	}
	history, err := app.store.LoadHistory(context.Background(), 100, 7)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := app.store.LatestReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(history["p1::m1"]) != 3 || history["p1::m1"][1].Status != "unknown" || latest.Providers[0].Results[0].Availability != "66.7%" {
		t.Fatalf("wrong outage history: %+v, %+v", history, latest)
	}
	billing, err := app.store.LoadBillingSummary(context.Background(), 7)
	if err != nil || billing.TotalProbeCount != 2 || billing.TotalTokens != 20 {
		t.Fatalf("unknown sample charged: %+v, %v", billing, err)
	}
}

func TestShutdownCancelsAndWaitsForTask(t *testing.T) {
	app := testApplication(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	app.cfg.Providers = []config.ProviderConfig{{ID: "p1", Enabled: true, ProbeEnabled: true, BaseURL: server.URL, Models: []string{"m1"}}}
	finished := make(chan error, 1)
	go func() { _, err := app.check(context.Background()); finished <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if app.RunningState().Running {
		t.Fatal("shutdown returned before task finished")
	}
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
	tasks, err := app.ListTasks(context.Background(), storage.TaskQuery{})
	if err != nil || len(tasks) != 1 || tasks[0].Status != "canceled" {
		t.Fatalf("task not finalized: %+v, %v", tasks, err)
	}
	if _, err := app.check(context.Background()); !errors.Is(err, web.ErrShuttingDown) {
		t.Fatalf("shutdown allowed new check: %v", err)
	}
}
