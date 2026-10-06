package report

import (
	"reflect"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
	"cg/internal/provider"
)

func TestReportHistorySeparatesCollidingIdentities(t *testing.T) {
	now := time.Now().UTC()
	cfg := config.Config{HistorySize: 2, MaxHistoryRecords: 10, StatsWindowDays: 7, Providers: []config.ProviderConfig{
		{ID: "a", Models: []string{"b::c"}, Enabled: true, ProbeEnabled: true},
		{ID: "a::b", Models: []string{"c"}, Enabled: true, ProbeEnabled: true},
	}}
	history := map[string][]HistoryRecord{}
	var results []probe.Result
	statuses := []string{"error", "slow"}
	for i, p := range cfg.Providers {
		model := p.Models[0]
		history[provider.ModelKey(p.ID, model)] = []HistoryRecord{{Status: statuses[i], CheckedAt: now.Add(-time.Minute).Format(time.RFC3339)}}
		results = append(results, probe.Result{ProviderID: p.ID, Model: model, Status: "ok", HistoryKey: p.ID + "::" + model})
	}
	value, updated := Build(cfg, results, nil, history, now)
	if len(value.Providers) != 2 || len(updated) != 2 {
		t.Fatalf("report merged distinct identities: %+v", value)
	}
	for i, group := range value.Providers {
		if len(group.Results) != 1 || !reflect.DeepEqual(group.Results[0].History, []string{statuses[i], "ok"}) {
			t.Fatalf("wrong historical samples: %+v", group)
		}
		// Old persisted snapshots may still contain the ambiguous key.
		value.Providers[i].Results[0].HistoryKey = "a::b::c"
	}
	projected := WithConfig(value, config.AdminConfigFromConfig(cfg))
	for _, group := range projected.Providers {
		result := group.Results[0]
		if result.HistoryKey != provider.ModelKey(group.ProviderID, result.Model) {
			t.Fatalf("cached result kept its legacy key: %+v", result)
		}
	}
}
