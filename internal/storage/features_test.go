package storage

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/notify"
	"cg/internal/probe"
)

func TestRequestBudgetAtomic(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	var accepted atomic.Int32
	var group sync.WaitGroup
	for range 30 {
		group.Go(func() {
			err := s.ReserveRequest(ctx, 7)
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrBudgetExceeded) {
				t.Error(err)
			}
		})
	}
	group.Wait()
	budget, err := s.RequestBudget(ctx, 7)
	if err != nil || accepted.Load() != 7 || budget.Used != 7 || !budget.Exhausted {
		t.Fatal(budget, err, accepted.Load())
	}
	if err := s.initBudget(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ReserveRequest(ctx, 0); err != nil {
		t.Fatal(err)
	}
	budget, _ = s.RequestBudget(ctx, 0)
	if budget.Used != 8 || budget.Exhausted {
		t.Fatal("unlimited or persistence failed", budget)
	}
}

func TestMetricsCredentialLifecycle(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	issued, err := s.IssueMetricsToken(ctx, "prometheus", 0)
	if err != nil {
		t.Fatal(err)
	}
	if valid, err := s.ValidMetricsToken(ctx, issued.Token); !valid || err != nil {
		t.Fatal(valid, err)
	}
	var hash []byte
	if err := s.db.QueryRow(`SELECT token_hash FROM metrics_tokens WHERE id = ?`, issued.ID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if len(hash) != 32 || strings.Contains(string(hash), issued.Token) {
		t.Fatal("plaintext token persisted")
	}
	rotated, err := s.IssueMetricsToken(ctx, "", issued.ID)
	if err != nil || rotated.Token == issued.Token || rotated.Name != issued.Name {
		t.Fatal(rotated, err)
	}
	if valid, _ := s.ValidMetricsToken(ctx, issued.Token); valid {
		t.Fatal("rotated token still accepted")
	}
	if valid, _ := s.ValidMetricsToken(ctx, rotated.Token); !valid {
		t.Fatal("new token rejected")
	}
	if err := s.RevokeMetricsToken(ctx, issued.ID); err != nil {
		t.Fatal(err)
	}
	if valid, _ := s.ValidMetricsToken(ctx, rotated.Token); valid {
		t.Fatal("revoked token still accepted")
	}
	if items, err := s.ListMetricsTokens(ctx); err != nil || len(items) != 0 {
		t.Fatal(items, err)
	}
}

func TestExportsFilterAndRetainFirstTextTiming(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, provider := range []string{"p1", "p2"} {
		if err := s.RecordResults(ctx, []probe.Result{{ProviderID: provider, Model: "m", Status: "ok", FirstTokenMS: 12, LatencyMS: 45, TotalTokens: 8}}, now, 100, true); err != nil {
			t.Fatal(err)
		}
	}
	start := now.Truncate(24 * time.Hour)
	for _, kind := range []string{"history", "usage"} {
		rows, err := s.ExportRows(ctx, ExportQuery{Kind: kind, ProviderID: "p1", Model: "m", Start: start, End: start.Add(24 * time.Hour)})
		if err != nil || len(rows) != 2 {
			t.Fatal(rows, err)
		}
		if kind == "history" && rows[1][5] != "12" {
			t.Fatal("first text timing lost", rows)
		}
		empty, err := s.ExportRows(ctx, ExportQuery{Kind: kind, ProviderID: "missing", Start: start, End: start.Add(24 * time.Hour)})
		if err != nil || len(empty) != 1 {
			t.Fatal(empty, err)
		}
	}
}

func TestDebounceStateMigration(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	if _, err := s.db.Exec(`ALTER TABLE notify_state DROP COLUMN candidate`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`ALTER TABLE notify_state DROP COLUMN consecutive`); err != nil {
		t.Fatal(err)
	}
	if err := s.init(ctx); err != nil {
		t.Fatal(err)
	}
	value := notify.State{Status: "ok", Candidate: "error", Consecutive: 2}
	if err := s.WriteNotifyState(ctx, value); err != nil {
		t.Fatal(err)
	}
	if err := s.init(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadNotifyState(ctx)
	if err != nil || got != value {
		t.Fatal(got, err)
	}
}
