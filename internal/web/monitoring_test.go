package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"cg/internal/config"
	"cg/internal/httpclient"
	"cg/internal/probe"
	"cg/internal/report"
)

func TestMonitoringPermissionAndCSRF(t *testing.T) {
	s, _ := newTestServer(t)
	s.admin = &featureAdmin{}
	for _, endpoint := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/admin/monitoring", "", 200},
		{"GET", "/api/admin/monitoring/diagnostics", "", 200},
		{"GET", "/api/admin/monitoring/incidents", "", 200},
		{"PUT", "/api/admin/monitoring/settings", `{"version":0,"backup_keep":3,"rules":[],"schedules":[],"prices":[]}`, 200},
		{"POST", "/api/admin/monitoring/backup", "", 201},
		{"POST", "/api/admin/monitoring/verify", `{"name":"../../cg.sqlite"}`, 400},
		{"POST", "/api/admin/monitoring/ack", `{"id":999,"note":"test"}`, 409},
		{"POST", "/api/admin/monitoring/test-rule", `{"id":"missing"}`, 404},
		{"POST", "/api/admin/monitoring/approve", `{"provider_id":"missing","revision":"old"}`, 409},
	} {
		for _, role := range []string{"", "user", "admin"} {
			expected := endpoint.status
			if role == "" {
				expected = 401
			} else if role == "user" {
				expected = 403
			}
			rec := perform(s, endpoint.method, endpoint.path, endpoint.body, role)
			if rec.Code != expected {
				t.Fatalf("%s %s: %d %s", role, endpoint.path, rec.Code, rec.Body)
			}
		}
		if endpoint.method != "GET" {
			req := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(endpoint.body))
			authenticateTestRequest(req, false)
			req.Header.Del("X-CSRF-Token")
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != 403 {
				t.Fatal("missing CSRF protection", endpoint.path)
			}
		}
	}
}

func TestMonitoringHistoryRejectsInvalidQueries(t *testing.T) {
	s, _ := newTestServer(t)
	for _, path := range []string{"/api/admin/monitoring/diagnostics", "/api/admin/monitoring/incidents"} {
		for _, query := range []string{"limit=0", "limit=101", "limit=oops", "before=-1", "before=9223372036854775808", "start=2026-10-01", "end=", "status=invalid", "start=2026-10-02T00:00:00Z&end=2026-10-01T00:00:00Z"} {
			if rec := perform(s, "GET", path+"?"+query, "", "admin"); rec.Code != 400 {
				t.Fatal(path, query, rec.Code, rec.Body)
			}
		}
		rec := perform(s, "GET", path+"?limit=25", "", "admin")
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"items":[]`) || !strings.Contains(rec.Body.String(), `"has_more":false`) {
			t.Fatal(rec.Code, rec.Body)
		}
		if rec := perform(s, "POST", path, "", "admin"); rec.Code != 405 {
			t.Fatal("method guard missing", rec.Code)
		}
	}
}

func TestMonitoringHidesSecretsAndPublicDiagnosticMetadata(t *testing.T) {
	s, _ := newTestServer(t)
	ctx := context.Background()
	settings, _ := s.store.MonitoringSettings(ctx)
	settings.Rules = []config.AlertRule{{ID: "r", Name: "rule", Platform: "webhook", URL: "https://example.invalid/private-credential", FailureThreshold: 1, RecoveryThreshold: 1}}
	if _, err := s.store.SaveMonitoringSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	rec := perform(s, "GET", "/api/admin/monitoring", "", "admin")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "private-credential") {
		t.Fatalf("secret leaked: %d", rec.Code)
	}
	result := probe.Result{ProviderID: "p", Model: "m", Status: "ok", Diagnostics: &httpclient.Diagnostics{RequestID: "private-request"}}
	cfg := config.Config{Providers: []config.ProviderConfig{{ID: "p", Enabled: true, ProbeEnabled: true, Models: []string{"m"}}}}
	s.cfg = cfg
	value := report.Report{Providers: []report.ProviderReport{{ProviderID: "p", Results: []report.ModelResult{{Result: result}}}}}
	encoded, _ := json.Marshal(s.publicReport(ctx, value))
	if strings.Contains(string(encoded), "private-request") {
		t.Fatal("public diagnostics exposed")
	}
}
