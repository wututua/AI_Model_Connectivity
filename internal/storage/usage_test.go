package storage

import (
	"context"
	"testing"
	"time"

	"cg/internal/probe"
	"cg/internal/report"
)

func TestBillingSurvivesHistoryPruningAndDisabledHistory(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	result := []probe.Result{{ProviderID: "p1", Model: "m1", HistoryKey: "p1::m1", Status: "ok", PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30}}
	for index := 0; index < 3; index++ {
		if err := store.RecordResults(ctx, result, time.Now(), 1, index < 2); err != nil {
			t.Fatal(err)
		}
	}
	history, err := store.LoadHistory(ctx, 10, 7)
	if err != nil || len(history["p1::m1"]) != 1 {
		t.Fatalf("history retention failed: %v", err)
	}
	summary, err := store.LoadBillingSummary(ctx, 30)
	if err != nil || summary.TotalTokens != 90 || summary.TotalProbeCount != 3 || len(summary.Daily) != 1 {
		t.Fatalf("billing lost history: %+v, %v", summary, err)
	}
	if err := store.initUsage(ctx); err != nil {
		t.Fatal(err)
	}
	summary, err = store.LoadBillingSummary(ctx, 30)
	if err != nil || summary.TotalTokens != 90 {
		t.Fatalf("reinitialization changed billing: %+v, %v", summary, err)
	}
}

func TestUsageMigratesLegacyHistoryOnlyOnce(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `DELETE FROM runtime_config WHERE key = 'usage_daily_migrated'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO probe_results(provider, model, result, checked_at, history_key, total_tokens) VALUES ('p1', 'm1', 'ok', ?, 'p1::m1', 42)`, time.Now().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := store.initUsage(ctx); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := store.LoadBillingSummary(ctx, 30)
	if err != nil || summary.TotalTokens != 42 || summary.TotalProbeCount != 1 {
		t.Fatalf("migration failed: %+v, %v", summary, err)
	}
}

func TestReportWriteFailureRollsBackHistoryAndUsage(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER fail_report BEFORE INSERT ON latest_report BEGIN SELECT RAISE(ABORT, 'test write failure'); END`); err != nil {
		t.Fatal(err)
	}
	results := []probe.Result{{ProviderID: "p1", Model: "m1", HistoryKey: "p1::m1", TotalTokens: 22}}
	err := store.RecordCheck(ctx, results, time.Now(), 100, true, &report.Report{GeneratedAt: "test"})
	if err == nil {
		t.Fatal("report failure was ignored")
	}
	history, err := store.LoadHistory(ctx, 100, 7)
	if err != nil || len(history) != 0 {
		t.Fatalf("partial history committed: %v", err)
	}
	summary, err := store.LoadBillingSummary(ctx, 30)
	if err != nil || summary.TotalTokens != 0 || summary.TotalProbeCount != 0 {
		t.Fatalf("partial usage committed: %+v, %v", summary, err)
	}
}
