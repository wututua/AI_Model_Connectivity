package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/metrics"
	"cg/internal/probe"
	"cg/internal/storage"
)

type featureAdmin struct {
	stubAdmin
	calls int
}

func (a *featureAdmin) StartSelectedCheck(context.Context, config.CheckSelection) (storage.CheckTask, error) {
	a.calls++
	return storage.CheckTask{ID: 3}, nil
}
func (a *featureAdmin) BatchProviders(context.Context, config.ProviderBatch) (config.AdminConfig, error) {
	a.calls++
	return config.AdminConfig{}, nil
}

func TestFeatureEndpointPermissions(t *testing.T) {
	s, _ := newTestServer(t)
	s.admin = &featureAdmin{}
	for _, endpoint := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/api/admin/detection/selected", `{"failed_only":true}`, 202},
		{"POST", "/api/admin/providers/batch", `{"ids":["p"],"action":"pause"}`, 200},
		{"PUT", "/api/admin/providers/batch", `{"id":"batch","name":"Batch provider"}`, 200},
		{"DELETE", "/api/admin/providers/batch", "", 200},
		{"GET", "/api/admin/budget", "", 200},
		{"GET", "/api/admin/metrics-tokens", "", 200},
		{"POST", "/api/admin/metrics-tokens", `{"name":"metrics"}`, 201},
		{"GET", "/api/admin/diagnostics", "", 200},
		{"GET", "/api/admin/export?kind=usage&start=2026-10-01&end=2026-10-06", "", 200},
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
				t.Fatalf("%s %s %s: %d %s", role, endpoint.method, endpoint.path, rec.Code, rec.Body)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("sensitive endpoint cacheable")
			}
		}
		if endpoint.method != "GET" {
			req := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(endpoint.body))
			authenticateTestRequest(req, false)
			req.Header.Del("X-CSRF-Token")
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != 403 {
				t.Fatal("CSRF not enforced", endpoint.path)
			}
		}
	}
}

type providerRouteAdmin struct {
	featureAdmin
	updatedID string
	deletedID string
}

func (a *providerRouteAdmin) UpsertProvider(_ context.Context, id string, _ config.ProviderUpdate) (config.SafeProviderConfig, error) {
	a.updatedID = id
	return config.SafeProviderConfig{ID: id}, nil
}

func (a *providerRouteAdmin) DeleteProvider(_ context.Context, id string) error {
	a.deletedID = id
	return nil
}

func TestBatchRoutePreservesProviderID(t *testing.T) {
	s, _ := newTestServer(t)
	admin := &providerRouteAdmin{}
	s.admin = admin
	if err := config.ValidateProviderID("batch"); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []struct{ method, path, body string }{
		{"PUT", "/api/admin/providers/batch", `{"id":"batch","name":"Batch provider"}`},
		{"DELETE", "/api/admin/providers/batch", ""},
		{"POST", "/api/admin/providers/batch", `{"ids":["batch"],"action":"pause"}`},
	} {
		rec := perform(s, endpoint.method, endpoint.path, endpoint.body, "admin")
		if rec.Code != 200 {
			t.Fatalf("%s %s: %d %s", endpoint.method, endpoint.path, rec.Code, rec.Body)
		}
	}
	if admin.updatedID != "batch" || admin.deletedID != "batch" || admin.calls != 1 {
		t.Fatalf("wrong handler: %+v", admin)
	}
	if rec := perform(s, "POST", "/api/admin/providers/batch/rerun", "", "admin"); rec.Code != 202 {
		t.Fatal("provider rerun route changed", rec.Code, rec.Body)
	}
}

func TestMetricsTokenHasOnlyReadMetricsScope(t *testing.T) {
	s, _ := newTestServer(t)
	s.SetMetrics(metrics.New())
	issued, err := s.store.IssueMetricsToken(context.Background(), "monitor", 0)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path, token string) int {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if call("GET", "/metrics", issued.Token) != 200 {
		t.Fatal("metrics credential rejected")
	}
	if call("POST", "/metrics", issued.Token) != 405 {
		t.Fatal("credential allowed write method")
	}
	for _, path := range []string{"/api/admin/config", "/api/admin/metrics-tokens", "/api/admin/diagnostics", "/api/admin/notifications", "/api/admin/detection"} {
		if call("GET", path, issued.Token) != 401 {
			t.Fatal("credential scope escaped", path)
		}
	}
	rec := perform(s, "POST", "/api/admin/metrics-tokens/"+strconv.FormatInt(issued.ID, 10)+"/rotate", "", "admin")
	if rec.Code != 200 {
		t.Fatal(rec.Body)
	}
	var replacement storage.IssuedMetricsToken
	json.Unmarshal(rec.Body.Bytes(), &replacement)
	if call("GET", "/metrics", issued.Token) != 401 || call("GET", "/metrics", replacement.Token) != 200 {
		t.Fatal("rotation not effective")
	}
	rec = perform(s, "DELETE", "/api/admin/metrics-tokens/"+strconv.FormatInt(issued.ID, 10), "", "admin")
	if rec.Code != 200 || call("GET", "/metrics", replacement.Token) != 401 {
		t.Fatal("revocation not effective")
	}
}

func TestExportSafetyAndValidation(t *testing.T) {
	s, _ := newTestServer(t)
	now := time.Now().UTC()
	if err := s.store.RecordResults(context.Background(), []probe.Result{{ProviderID: "=secret-formula", Model: "@danger", Status: "error", Error: "private-response", ResponsePreview: "private-preview"}}, now, 100, true); err != nil {
		t.Fatal(err)
	}
	day := now.Format(time.DateOnly)
	rec := perform(s, "GET", "/api/admin/export?kind=history&start="+day+"&end="+day, "", "admin")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "'=secret-formula") || !strings.Contains(rec.Body.String(), "'@danger") || strings.Contains(rec.Body.String(), "private-") {
		t.Fatal("unsafe CSV", rec.Code, rec.Body)
	}
	for _, query := range []string{"kind=bad", "kind=history&start=2026-10-07&end=2026-10-01", "kind=history&start=2020-01-01&end=2026-10-06"} {
		if rec := perform(s, "GET", "/api/admin/export?"+query, "", "admin"); rec.Code != 400 {
			t.Fatal("invalid date accepted")
		}
	}
	for _, value := range []string{"=x", "+x", "-x", "@x", " \t=x", "hello\n=1"} {
		if !strings.HasPrefix(csvSafe(value), "'") {
			t.Fatal("formula injection", value)
		}
	}
	rec = perform(s, "GET", "/api/admin/diagnostics", "", "admin")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "private-") || strings.Contains(rec.Body.String(), "api_key") || strings.Contains(rec.Body.String(), "base_url") {
		t.Fatal("unsafe diagnostics", rec.Body)
	}
}
