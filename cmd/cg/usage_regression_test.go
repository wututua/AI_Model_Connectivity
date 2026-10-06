package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"cg/internal/config"
)

func TestFailedCompletionUsageReachesBilling(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		for _, historyEnabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("http-%d/history-%t", status, historyEnabled), func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(status)
					w.Write([]byte(`{"error":{"message":"failed completion"},"usage":{"prompt_tokens":3,"completion_tokens":5}}`))
				}))
				defer upstream.Close()
				app := testApplication(t)
				app.cfg.EnableHistory = historyEnabled
				app.cfg.Providers = []config.ProviderConfig{{
					ID: "p1", BaseURL: upstream.URL, Models: []string{"m1"}, Enabled: true, ProbeEnabled: true,
				}}
				ctx := context.Background()
				value, err := app.check(ctx)
				if err != nil || value.ErrorCount != 1 || value.OKCount != 0 {
					t.Fatalf("failed completion not reported: %+v, %v", value, err)
				}
				latest, err := app.store.LatestReport(ctx)
				if err != nil || len(latest.Providers) != 1 || len(latest.Providers[0].Results) != 1 {
					t.Fatalf("missing failed completion snapshot: %+v, %v", latest, err)
				}
				result := latest.Providers[0].Results[0]
				if result.Status != "error" || result.PromptTokens != 3 || result.CompletionTokens != 5 || result.TotalTokens != 8 {
					t.Fatalf("snapshot discarded reported usage: %+v", result)
				}
				billing, err := app.store.LoadBillingSummary(ctx, 7)
				if err != nil || billing.TotalProbeCount != 1 || billing.TotalPromptTokens != 3 ||
					billing.TotalCompletionTokens != 5 || billing.TotalTokens != 8 {
					t.Fatalf("billing discarded failed completion usage: %+v, %v", billing, err)
				}
				history, err := app.store.LoadHistory(ctx, 100, 7)
				if err != nil || (len(history["p1::m1"]) == 1) != historyEnabled {
					t.Fatalf("history setting was not respected: %+v, %v", history, err)
				}
			})
		}
	}
}
