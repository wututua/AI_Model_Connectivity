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
