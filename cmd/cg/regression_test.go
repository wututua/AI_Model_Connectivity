package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
	"cg/internal/report"
	"cg/internal/web"
)

func TestCompletedUsageSurvivesCancellation(t *testing.T) {
	app := testApplication(t)
	var calls atomic.Int32
	waiting, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if calls.Add(1) == 1 {
			w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`))
			return
		}
		close(waiting)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	app.cfg.Providers = []config.ProviderConfig{{ID: "p1", BaseURL: server.URL, Models: []string{"one", "two"}, Enabled: true, ProbeEnabled: true}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := app.check(ctx); done <- err }()
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("second probe did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("check did not finish")
	}
	billing, err := app.store.LoadBillingSummary(context.Background(), 7)
	if err != nil || billing.TotalTokens != 10 || billing.TotalProbeCount != 1 {
		t.Fatalf("completed usage lost or canceled probe charged: %+v, %v", billing, err)
	}
	history, err := app.store.LoadHistory(context.Background(), 100, 7)
	if err != nil || len(history) != 0 {
		t.Fatalf("interrupted batch replaced history: %+v, %v", history, err)
	}
}

func TestDeletedProviderDisappearsFromStatus(t *testing.T) {
	app := testApplication(t)
	app.cfg.Providers = []config.ProviderConfig{{ID: "p1", Name: "Provider", Type: "openai", BaseURL: "https://example.invalid/v1", Models: []string{"one"}, Enabled: true, ProbeEnabled: true}}
	app.broker = web.NewBroker()
	events, unsubscribe := app.broker.Subscribe()
	defer unsubscribe()
	value, _ := report.Build(app.cfg, []probe.Result{{ProviderID: "p1", Model: "one", Status: "ok"}}, nil, nil, time.Now())
	if err := app.store.SaveLatestReport(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteProvider(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	latest, err := app.store.LatestReport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case pushed := <-events:
		if len(pushed.Providers) != 0 || len(latest.Providers) != 0 || latest.State != "unconfigured" || latest.GeneratedAt != value.GeneratedAt {
			t.Fatalf("deleted provider or false timestamp: pushed=%+v stored=%+v", pushed, latest)
		}
	default:
		t.Fatal("no status update published")
	}
}

func TestAcceptedTaskSurvivesDisconnectAndRejectsConcurrentRuns(t *testing.T) {
	app := testApplication(t)
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}],"usage":{"total_tokens":10}}`))
	}))
	defer server.Close()
	// Always release the upstream if an assertion fails.
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	app.cfg.Providers = []config.ProviderConfig{{ID: "p1", BaseURL: server.URL, Models: []string{"one"}, Enabled: true, ProbeEnabled: true}}
	ctx, cancel := context.WithCancel(context.Background())
	task, err := app.StartCheck(ctx, "")
	cancel()
	if err != nil || task.ID == 0 || task.Status != "running" {
		t.Fatalf("acceptance failed: %+v, %v", task, err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("disconnected request canceled background task")
	}
	if _, err := app.StartCheck(context.Background(), ""); !errors.Is(err, web.ErrCheckAlreadyRunning) {
		t.Fatalf("concurrent run allowed: %v", err)
	}
	if err := app.DeleteProvider(context.Background(), "p1"); err != nil {
		t.Fatal(err)
	}
	close(release)
	deadline := time.After(5 * time.Second)
	for app.RunningState().Running {
		select {
		case <-deadline:
			t.Fatal("task did not finish")
		case <-time.After(10 * time.Millisecond):
		}
	}
	finished, err := app.GetTask(context.Background(), task.ID)
	if err != nil || finished.Status != "success" || finished.Total != 1 {
		t.Fatalf("task not finalized: %+v, %v", finished, err)
	}
	billing, err := app.store.LoadBillingSummary(context.Background(), 7)
	if err != nil || billing.TotalTokens != 10 || billing.TotalProbeCount != 1 {
		t.Fatalf("usage missing or double counted: %+v, %v", billing, err)
	}
	latest, err := app.store.LatestReport(context.Background())
	if err != nil || len(latest.Providers) != 0 {
		t.Fatalf("in-flight check resurrected deleted provider: %+v, %v", latest, err)
	}
	if _, err := app.StartCheck(context.Background(), "p1"); !errors.Is(err, web.ErrProviderUnavailable) {
		t.Fatalf("missing provider accepted: %v", err)
	}
}
