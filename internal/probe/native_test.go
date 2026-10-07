package probe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"cg/internal/config"
)

func TestNativeRunnerUsesPagedBudgetAndDiagnostics(t *testing.T) {
	for _, protocol := range []string{"anthropic", "gemini"} {
		for _, limit := range []int32{1, 3} {
			t.Run(fmt.Sprintf("%s/budget=%d", protocol, limit), func(t *testing.T) {
				var calls, reservations atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Request-ID", "native-fixture")
					if r.Method == "GET" {
						if protocol == "anthropic" {
							if r.URL.Query().Get("after_id") == "" {
								fmt.Fprint(w, `{"data":[],"has_more":true,"last_id":"next"}`)
							} else {
								fmt.Fprint(w, `{"data":[{"id":"a"}]}`)
							}
						} else if r.URL.Query().Get("pageToken") == "" {
							fmt.Fprint(w, `{"nextPageToken":"next"}`)
						} else {
							fmt.Fprint(w, `{"models":[{"name":"models/a","supportedGenerationMethods":["generateContent"]}]}`)
						}
						return
					}
					if protocol == "anthropic" {
						fmt.Fprint(w, `{"type":"message","content":[{"type":"text","text":"pang"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`)
					} else {
						fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"pang"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}}`)
					}
				}))
				defer server.Close()
				cfg := config.Config{TimeoutSeconds: 2, ModelListTimeoutSeconds: 2, SlowThresholdMS: 2000,
					Providers: []config.ProviderConfig{{ID: "native", BaseURL: server.URL, Enabled: true, ProbeEnabled: true, Probe: config.ProbeOptions{Protocol: protocol}}}}
				runner := NewRunner(cfg)
				budgetErr := errors.New("budget exhausted")
				runner.SetRequestGuard(func(context.Context) error {
					if reservations.Add(1) > limit {
						return budgetErr
					}
					return nil
				})
				results, failures, err := runner.Run(context.Background())
				if calls.Load() != limit {
					t.Fatal("budget bypassed", calls.Load(), limit)
				}
				if limit == 1 {
					if !errors.Is(err, budgetErr) || len(results) != 0 || len(failures) != 1 {
						t.Fatal(results, failures, err)
					}
				} else if err != nil || len(failures) != 0 || len(results) != 1 {
					t.Fatal(results, failures, err)
				} else {
					result := results[0]
					if result.Status != "ok" || !result.Completed || !result.UsageKnown || result.TotalTokens != 5 ||
						result.Diagnostics == nil || result.Diagnostics.RequestID != "native-fixture" {
						t.Fatal(result)
					}
				}
			})
		}
	}
}
