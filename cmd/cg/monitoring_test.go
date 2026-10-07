package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/notify"
	"cg/internal/probe"
	"cg/internal/storage"
)

func TestRuleNotificationRetryPreservesRoute(t *testing.T) {
	app := testApplication(t)
	ctx := context.Background()
	var wrong, correct atomic.Int32
	global := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { wrong.Add(1) }))
	defer global.Close()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { correct.Add(1) }))
	defer target.Close()
	app.cfg.NotifyWebhookURL = global.URL
	settings, _ := app.store.MonitoringSettings(ctx)
	rule := config.AlertRule{ID: "r", Name: "rule", Enabled: true, Platform: "webhook", URL: target.URL, FailureThreshold: 1, RecoveryThreshold: 1}
	settings.Rules = []config.AlertRule{rule}
	if _, err := app.store.SaveMonitoringSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	record, err := app.store.CreateDelivery(ctx, notify.Delivery{RuleID: "r", Kind: "alert", Platform: "webhook", Summary: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	record.Status = "error"
	if err := app.store.FinishDelivery(ctx, record); err != nil {
		t.Fatal(err)
	}
	retry, err := app.SendNotification(ctx, record.ID)
	if err != nil || retry.RuleID != "r" || correct.Load() != 1 || wrong.Load() != 0 {
		t.Fatalf("wrong route: %+v %v correct=%d wrong=%d", retry, err, correct.Load(), wrong.Load())
	}
}

func TestSkippedProvidersCannotResolveDiscoveryIncidents(t *testing.T) {
	app := testApplication(t)
	ctx := context.Background()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{"data":[{"id":"m"}],"choices":[{"message":{"content":"pang"}}]}`))
	}))
	defer server.Close()
	app.cfg.Providers = []config.ProviderConfig{
		{ID: "disabled", BaseURL: server.URL, ProbeEnabled: true, ConnectionRevision: "one"},
		{ID: "paused", BaseURL: server.URL, Enabled: true, ConnectionRevision: "two"},
		{ID: "active", BaseURL: server.URL, Enabled: true, ProbeEnabled: true, ConnectionRevision: "three"},
	}
	results := []probe.Result{}
	for _, p := range app.cfg.Providers {
		results = append(results, probe.Result{ProviderID: p.ID, Status: "error", Completed: true})
	}
	if err := app.store.ObserveIncidents(ctx, app.cfg, results); err != nil {
		t.Fatal(err)
	}
	if _, err := app.runCheck(ctx, checkOptions{SaveLatest: true}, &storage.CheckTaskUpdate{}); err != nil {
		t.Fatal(err)
	}
	incidents, err := app.store.Incidents(ctx)
	if err != nil || len(incidents) != 3 || calls.Load() != 2 {
		t.Fatal(incidents, err, calls.Load())
	}
	for _, incident := range incidents {
		want := "open"
		if incident.ProviderID == "active" {
			want = "resolved"
		}
		if incident.Status != want {
			t.Fatalf("unobserved incident changed: %+v", incident)
		}
	}
}

func TestIndependentScheduleOnlyProbesDueProvider(t *testing.T) {
	app := testApplication(t)
	ctx := context.Background()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"pong"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	app.cfg.Providers = []config.ProviderConfig{
		{ID: "fast", BaseURL: server.URL, Models: []string{"m"}, Enabled: true, ProbeEnabled: true, ConnectionRevision: "fast-v1"},
		{ID: "slow", BaseURL: server.URL, Models: []string{"m"}, Enabled: true, ProbeEnabled: true, ConnectionRevision: "slow-v1"},
	}
	settings, _ := app.store.MonitoringSettings(ctx)
	settings.Schedules = []config.ProviderSchedule{{ProviderID: "fast", IntervalMinutes: 1}, {ProviderID: "slow", IntervalMinutes: 60}}
	settings, err := app.store.SaveMonitoringSettings(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.ScheduleDue(ctx, "fast", "fast-v1", 1, time.Now().Add(-2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	app.runProviderSchedules(ctx, app.cfg, settings)
	if calls.Load() != 1 {
		t.Fatalf("unexpected requests: %d", calls.Load())
	}
	tasks, err := app.store.ListCheckTasks(ctx, storage.TaskQuery{})
	if err != nil || len(tasks) != 1 || tasks[0].ProviderID != "fast" || tasks[0].Kind != "scheduled-provider" {
		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}
	due, err := app.store.ScheduleDue(ctx, "fast", "fast-v1", 1, time.Now())
	if err != nil || due {
		t.Fatalf("schedule not advanced: %v %v", due, err)
	}
}

func TestOperationalNotificationsAreDeduplicated(t *testing.T) {
	app := testApplication(t)
	ctx := context.Background()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	settings, _ := app.store.MonitoringSettings(ctx)
	settings.Rules = []config.AlertRule{{ID: "ops", Name: "ops", Enabled: true, Platform: "webhook", URL: server.URL, FailureThreshold: 1, RecoveryThreshold: 1, BackupFailures: true}}
	if _, err := app.store.SaveMonitoringSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if err := app.store.AddMonitorEvent(ctx, "backup_error", "fixture"); err != nil {
		t.Fatal(err)
	}
	app.sendOperationalNotices(ctx)
	app.sendOperationalNotices(ctx)
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
	records, err := app.store.ListDeliveries(ctx, notify.DeliveryQuery{})
	if err != nil || len(records) != 1 || records[0].RuleID != "ops" {
		t.Fatalf("records=%+v err=%v", records, err)
	}
}
