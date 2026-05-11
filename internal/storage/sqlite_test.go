package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/notify"
	"cg/internal/probe"
	"cg/internal/report"
)

func TestSQLiteStoreLatestReport(t *testing.T) {
	store := newTestSQLiteStore(t)
	value := report.Report{Title: "test", GeneratedAt: "2026-05-09 12:00:00", Total: 2, OKCount: 1, ErrorCount: 1}
	if err := store.SaveLatestReport(context.Background(), value); err != nil {
		t.Fatalf("save latest report: %v", err)
	}
	loaded, err := store.LatestReport(context.Background())
	if err != nil {
		t.Fatalf("load latest report: %v", err)
	}
	if loaded.Title != value.Title || loaded.GeneratedAt != value.GeneratedAt || loaded.Total != value.Total {
		t.Fatalf("loaded report mismatch: %#v", loaded)
	}
}

func TestSQLiteStoreAppendAndLoadHistory(t *testing.T) {
	store := newTestSQLiteStore(t)
	checkedAt := time.Now().UTC()
	results := []probe.Result{
		{ProviderID: "openai-main", ProviderType: "openai", ProviderName: "OpenAI", Model: "gpt-4o-mini", Status: "ok", LatencyMS: 120, HistoryKey: "openai-main::gpt-4o-mini"},
		{ProviderID: "openai-main", ProviderType: "openai", ProviderName: "OpenAI", Model: "gpt-4.1-mini", Status: "error", LatencyMS: 300, Error: "timeout after 30s", HistoryKey: "openai-main::gpt-4.1-mini"},
	}
	if err := store.AppendResults(context.Background(), results, checkedAt); err != nil {
		t.Fatalf("append results: %v", err)
	}
	history, err := store.LoadHistory(context.Background(), 10, 7)
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	if len(history["openai-main::gpt-4o-mini"]) != 1 {
		t.Fatalf("expected one ok history record, got %#v", history)
	}
	if history["openai-main::gpt-4.1-mini"][0].Status != "error" {
		t.Fatalf("expected error history record, got %#v", history)
	}
}

func TestSQLiteNotifyStateStore(t *testing.T) {
	store := newTestSQLiteStore(t)
	stateStore := SQLiteNotifyStateStore{Store: store}
	state := notify.State{Status: "error", SentAt: time.Now().UTC().Truncate(time.Second)}
	if err := stateStore.Write(state); err != nil {
		t.Fatalf("write notify state: %v", err)
	}
	loaded, err := stateStore.Read()
	if err != nil {
		t.Fatalf("read notify state: %v", err)
	}
	if loaded.Status != state.Status || !loaded.SentAt.Equal(state.SentAt) {
		t.Fatalf("notify state mismatch: %#v", loaded)
	}
}

func TestSQLiteRuntimeConfig(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	// first load returns not-found
	_, ok, err := store.LoadRuntimeConfig(ctx)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}
	if ok {
		t.Fatal("expected no runtime config initially")
	}

	// save then reload
	cfg := config.RuntimeConfig{Settings: config.RuntimeSettings{DashboardTitle: "test-title", TimeoutSeconds: 42}}
	if err := store.SaveRuntimeConfig(ctx, cfg); err != nil {
		t.Fatalf("save runtime config: %v", err)
	}
	loaded, ok, err := store.LoadRuntimeConfig(ctx)
	if err != nil {
		t.Fatalf("load runtime config after save: %v", err)
	}
	if !ok {
		t.Fatal("expected runtime config to exist after save")
	}
	if loaded.Settings.DashboardTitle != cfg.Settings.DashboardTitle || loaded.Settings.TimeoutSeconds != cfg.Settings.TimeoutSeconds {
		t.Fatalf("runtime config mismatch: got %+v", loaded)
	}
}

func TestSQLiteTaskLifecycle(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	id, err := store.CreateCheckTask(ctx, CheckTask{Kind: "manual", Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive task id, got %d", id)
	}

	update := CheckTaskUpdate{
		Status:     "ok",
		FinishedAt: time.Now().UTC(),
		ElapsedMS:  250,
		OKCount:    3,
		Total:      3,
	}
	if err := store.FinishCheckTask(ctx, id, update); err != nil {
		t.Fatalf("finish task: %v", err)
	}

	task, err := store.GetCheckTask(ctx, id)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.Status != "ok" || task.OKCount != 3 || task.ElapsedMS != 250 {
		t.Fatalf("task mismatch: %+v", task)
	}

	tasks, err := store.ListCheckTasks(ctx, TaskQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != id {
		t.Fatalf("list tasks returned unexpected results: %+v", tasks)
	}
}

func TestSQLiteHistoryLimitPerKey(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	for i := range 5 {
		results := []probe.Result{{
			ProviderID:   "p1",
			ProviderName: "P1",
			Model:        "m1",
			Status:       "ok",
			LatencyMS:    100 + i,
			HistoryKey:   "p1::m1",
		}}
		if err := store.AppendResults(ctx, results, time.Now().UTC()); err != nil {
			t.Fatalf("append results iter %d: %v", i, err)
		}
	}

	history, err := store.LoadHistory(ctx, 3, 7)
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	if len(history["p1::m1"]) != 3 {
		t.Fatalf("expected 3 records (limit), got %d", len(history["p1::m1"]))
	}
}

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	store, err := NewSQLite(context.Background(), filepath.Join(dir, "test.sqlite"), dir)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
