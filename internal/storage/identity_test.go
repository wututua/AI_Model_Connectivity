package storage

import (
	"context"
	"net/url"
	"testing"
	"time"

	"cg/internal/probe"
)

func TestHistorySeparatesLegacyCollidingKeys(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	pairs := [][2]string{{"a", "b::c"}, {"a::b", "c"}, {"a:", "c"}, {"a", ":c"}, {"a%3A", "c"}}
	for i, pair := range pairs {
		for sample := 0; sample < 3; sample++ {
			_, err := store.db.ExecContext(ctx, `INSERT INTO probe_results
				(provider, model, result, latency_ms, checked_at, history_key)
				VALUES (?, ?, 'ok', ?, ?, ?)`, pair[0], pair[1], i*10+sample,
				now.Add(time.Duration(sample-3)*time.Minute).Format(time.RFC3339), pair[0]+"::"+pair[1])
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	history, err := store.LoadHistory(ctx, 2, 7)
	if err != nil || len(history) != len(pairs) {
		t.Fatalf("legacy rows mixed identities: %d groups, %v", len(history), err)
	}
	for i, pair := range pairs {
		records := history[url.QueryEscape(pair[0])+"::"+pair[1]]
		if len(records) != 2 || records[0].LatencyMS != i*10+1 || records[1].LatencyMS != i*10+2 {
			t.Fatalf("wrong independent history for %v: %+v", pair, records)
		}
	}
	first := pairs[0]
	if err := store.AppendResults(ctx, []probe.Result{{
		ProviderID: first[0], Model: first[1], Status: "error", HistoryKey: "untrusted-caller-key",
	}}, now, 1); err != nil {
		t.Fatal(err)
	}
	history, err = store.LoadHistory(ctx, 10, 7)
	if err != nil || len(history[first[0]+"::"+first[1]]) != 1 ||
		len(history[url.QueryEscape(pairs[1][0])+"::"+pairs[1][1]]) != 3 {
		t.Fatalf("per-model retention affected a different identity: %+v, %v", history, err)
	}
}
