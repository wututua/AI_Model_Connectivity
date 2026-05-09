package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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
