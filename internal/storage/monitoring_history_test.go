package storage

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestMonitoringHistoryFiltersAndDateBoundaries(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i, row := range []struct{ provider, model, status, capability, errorType string }{
		{"p", "m", "ok", "", ""},
		{"p", "m", "error", "text", "rate_limit"},
		{"p", "embedding", "ok", "embedding", ""},
		{"q", "m", "error", "tools", "auth"},
	} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO probe_results(provider,model,result,checked_at,history_key,capability,error_type) VALUES (?,?,?,?,?,?,?)`,
			row.provider, row.model, row.status, base.Add(time.Duration(i)*time.Hour).Format(time.RFC3339), "fixture", row.capability, row.errorType); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query MonitoringHistoryQuery
		ids   []int64
	}{
		{MonitoringHistoryQuery{ProviderID: "p", Model: "m", Status: "error", Capability: "text", ErrorType: "rate_limit"}, []int64{2}},
		{MonitoringHistoryQuery{Capability: "text"}, []int64{2, 1}},
		{MonitoringHistoryQuery{Start: base.Add(time.Hour).In(time.FixedZone("local", 8*3600)), End: base.Add(2 * time.Hour)}, []int64{2}},
		{MonitoringHistoryQuery{ProviderID: "p' OR 1=1 --"}, []int64{}},
		{MonitoringHistoryQuery{Model: "%"}, []int64{}},
		{MonitoringHistoryQuery{Before: 3}, []int64{2, 1}},
	} {
		page, err := s.QueryDiagnostics(ctx, tc.query)
		if err != nil || page.Items == nil || page.HasMore || len(page.Items) != len(tc.ids) {
			t.Fatal(tc.query, page, err)
		}
		for i, id := range tc.ids {
			if page.Items[i].ID != id {
				t.Fatal(tc.query, page.Items)
			}
		}
	}
	for i, row := range []struct{ provider, model, status string }{
		{"p", "", "open"}, {"p", "m", "resolved"}, {"q", "m", "superseded"},
	} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO incidents(provider,model,revision,status,opened_at,last_seen_at) VALUES (?,?,'one',?,?,?)`,
			row.provider, row.model, row.status, base.Add(time.Duration(i)*time.Hour).Format(time.RFC3339), base.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query MonitoringHistoryQuery
		ids   []int64
	}{
		{MonitoringHistoryQuery{ProviderID: "p", Status: "open", Scope: "discovery"}, []int64{1}},
		{MonitoringHistoryQuery{Scope: "models"}, []int64{3, 2}},
		{MonitoringHistoryQuery{Model: "m", Start: base.Add(time.Hour), End: base.Add(2 * time.Hour)}, []int64{2}},
		{MonitoringHistoryQuery{ProviderID: "p' OR 1=1 --"}, []int64{}},
	} {
		page, err := s.QueryIncidents(ctx, tc.query)
		if err != nil || page.Items == nil || len(page.Items) != len(tc.ids) {
			t.Fatal(tc.query, page, err)
		}
		for i, id := range tc.ids {
			if page.Items[i].ID != id {
				t.Fatal(tc.query, page.Items)
			}
		}
	}
}

func TestMonitoringHistoryCursorSurvivesNewRecords(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	for i := 0; i < 225; i++ {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO probe_results(provider,model,result,checked_at,history_key) VALUES ('p','m','ok','2026-10-01T00:00:00Z','fixture')`); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO incidents(provider,model,revision,status,opened_at,last_seen_at) VALUES ('p',?,'one','open','2026-10-01T00:00:00Z','2026-10-01T00:00:00Z')`, fmt.Sprintf("m%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for _, incidents := range []bool{false, true} {
		q := MonitoringHistoryQuery{Limit: 50}
		seen := map[int64]bool{}
		for {
			ids := []int64{}
			next, more := int64(0), false
			if incidents {
				page, err := s.QueryIncidents(ctx, q)
				if err != nil {
					t.Fatal(err)
				}
				for _, item := range page.Items {
					ids = append(ids, item.ID)
				}
				next, more = page.NextBefore, page.HasMore
			} else {
				page, err := s.QueryDiagnostics(ctx, q)
				if err != nil {
					t.Fatal(err)
				}
				for _, item := range page.Items {
					ids = append(ids, item.ID)
				}
				next, more = page.NextBefore, page.HasMore
			}
			for _, id := range ids {
				if seen[id] || id > 225 {
					t.Fatal("duplicate or new record entered subsequent page", id)
				}
				seen[id] = true
			}
			if !more {
				if next != 0 || len(seen) != 225 {
					t.Fatal("truncated history", len(seen), next)
				}
				break
			}
			if len(ids) != 50 || next != ids[len(ids)-1] {
				t.Fatal("invalid cursor", ids, next)
			}
			if q.Before == 0 {
				if incidents {
					_, err := s.db.ExecContext(ctx, `INSERT INTO incidents(provider,model,revision,status,opened_at,last_seen_at) VALUES ('p','new','one','open','','')`)
					if err != nil {
						t.Fatal(err)
					}
				} else {
					_, err := s.db.ExecContext(ctx, `INSERT INTO probe_results(provider,model,result,checked_at,history_key) VALUES ('p','new','ok','','fixture')`)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			q.Before = next
		}
	}
}

func TestMonitoringHistoryValidationAndCancellation(t *testing.T) {
	s := newTestSQLiteStore(t)
	for _, q := range []MonitoringHistoryQuery{{Before: -1}, {Limit: 101}, {Limit: -1}, {Status: "bad"}, {Scope: "discovery", Model: "m"}, {Capability: "bad"}} {
		if _, err := s.QueryDiagnostics(context.Background(), q); err == nil {
			t.Fatal("invalid query accepted", q)
		}
		if _, err := s.QueryIncidents(context.Background(), q); err == nil {
			t.Fatal("invalid query accepted", q)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.QueryDiagnostics(ctx, MonitoringHistoryQuery{}); err == nil {
		t.Fatal("canceled query succeeded")
	}
}
