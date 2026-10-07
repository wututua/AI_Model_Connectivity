package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/auth"
	"cg/internal/config"
	"cg/internal/storage"
	"cg/internal/web"
)

func TestHTTPNativeProbeHistoryBillingAndAudit(t *testing.T) {
	for _, protocol := range []string{"anthropic", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			app := testApplication(t)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("X-Request-ID", "native-http-fixture")
				if protocol == "anthropic" {
					if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "fixture-key" {
						t.Error("incorrect native request")
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"type":"message","content":[{"type":"text","text":"pang"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`)
				} else {
					if r.URL.Path != "/v1/models/fixture:streamGenerateContent" || r.URL.Query().Get("alt") != "sse" || r.Header.Get("x-goog-api-key") != "fixture-key" {
						t.Error("incorrect native stream request")
					}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"pang\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":2,\"candidatesTokenCount\":3}}\n\n")
				}
			}))
			defer upstream.Close()
			hash, err := auth.HashPassword("Integration123")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := app.store.CreateUser(context.Background(), storage.User{Username: "admin", Role: "admin", Enabled: true, PasswordHash: hash}, false); err != nil {
				t.Fatal(err)
			}
			service := httptest.NewServer(web.NewServer(app.cfg, app.store, app.check, nil, app).Handler())
			defer service.Close()
			jar, err := cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Jar: jar, Timeout: 15 * time.Second}
			defer client.CloseIdleConnections()
			csrf := ""
			request := func(method, path string, input, output any, status int) {
				t.Helper()
				var body bytes.Buffer
				if input != nil {
					if err := json.NewEncoder(&body).Encode(input); err != nil {
						t.Fatal(err)
					}
				}
				req, err := http.NewRequest(method, service.URL+path, &body)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-CSRF-Token", csrf)
				res, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				if res.StatusCode != status {
					t.Fatalf("%s %s: HTTP %d, want %d", method, path, res.StatusCode, status)
				}
				if output != nil {
					if err := json.NewDecoder(res.Body).Decode(output); err != nil {
						t.Fatal(err)
					}
				}
			}
			var session struct {
				CSRF string `json:"csrf_token"`
			}
			request("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "Integration123"}, &session, 200)
			csrf = session.CSRF
			if csrf == "" {
				t.Fatal("login did not return CSRF")
			}
			var saved config.SafeProviderConfig
			request("POST", "/api/admin/providers", config.ProviderUpdate{
				ID: "native", Name: "Native fixture", Type: "custom", BaseURL: upstream.URL + "/v1", APIKey: "fixture-key",
				Models: []string{"fixture"}, Enabled: true, ProbeEnabled: true,
				Probe: config.ProbeOptions{Protocol: protocol, Stream: protocol == "gemini", MaxTokens: 32, OmitTemperature: true},
			}, &saved, 200)
			if saved.Probe.Protocol != protocol || !saved.APIKeySet {
				t.Fatal("protocol or credential setting lost")
			}
			var accepted struct {
				Task storage.CheckTask `json:"task"`
			}
			request("POST", "/api/admin/check", nil, &accepted, 202)
			deadline := time.Now().Add(10 * time.Second)
			for {
				var task storage.CheckTask
				request("GET", "/api/admin/tasks/"+strconv.FormatInt(accepted.Task.ID, 10), nil, &task, 200)
				if task.Status == "success" {
					if task.OKCount != 1 || task.Total != 1 {
						t.Fatal("incorrect completed task", task)
					}
					break
				}
				if task.Status != "running" || time.Now().After(deadline) {
					t.Fatal("native task did not complete", task.Status)
				}
				time.Sleep(10 * time.Millisecond)
			}
			var diagnostics storage.HistoryPage[storage.DiagnosticRecord]
			request("GET", "/api/admin/monitoring/diagnostics?provider_id=native&status=ok", nil, &diagnostics, 200)
			if len(diagnostics.Items) != 1 || diagnostics.Items[0].Diagnostics == nil || diagnostics.Items[0].Diagnostics.RequestID != "native-http-fixture" {
				t.Fatal("native diagnostics not persisted", diagnostics)
			}
			var billing storage.BillingSummary
			request("GET", "/api/admin/billing", nil, &billing, 200)
			if billing.TotalTokens != 5 || billing.TotalProbeCount != 1 || calls.Load() != 1 {
				t.Fatal("native accounting or request count mismatch", billing, calls.Load())
			}
			var audit storage.HistoryPage[storage.AuditEvent]
			request("GET", "/api/admin/audit", nil, &audit, 200)
			if len(audit.Items) != 3 || audit.Items[0].Action != "detection.start" || audit.Items[0].Result != "accepted" ||
				audit.Items[1].Action != "providers.create" || audit.Items[2].Action != "auth.login" {
				t.Fatal("HTTP operations not audited", audit)
			}
			for _, event := range audit.Items {
				if event.Actor != "admin" {
					t.Fatal("incorrect HTTP actor")
				}
			}
		})
	}
}
