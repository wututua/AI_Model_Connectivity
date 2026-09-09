package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/report"
	"cg/internal/storage"
)

// stubAdmin satisfies AdminController with no-op implementations.
type stubAdmin struct {
	token     string
	viewToken string
}

func TestPublicBindDetection(t *testing.T) {
	for _, host := range []string{"", "0.0.0.0", "::", "[::]", "::0", "192.168.1.10", "example.test"} {
		if !IsPublicBindHost(host) {
			t.Errorf("%q considered private", host)
		}
	}
	for _, host := range []string{"localhost", "127.0.0.1", "127.0.0.2", "::1", "[::1]"} {
		if IsPublicBindHost(host) {
			t.Errorf("%q considered public", host)
		}
	}
}

func TestAuthenticationFailureRateLimit(t *testing.T) {
	srv, _ := newTestServer(t)
	handler := srv.Handler()
	for attempt := 0; attempt <= authFailureLimit; attempt++ {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
		req.Header.Set("Authorization", "Bearer wrong")
		req.Header.Set("X-Forwarded-For", "203.0.113.99")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		want := http.StatusUnauthorized
		if attempt == authFailureLimit {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("attempt %d: got %d, want %d", attempt, rec.Code, want)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.RemoteAddr = "203.0.113.1:4321"
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatal("unrelated client is blocked")
	}
}

func TestAuthenticationLimitExpires(t *testing.T) {
	var limiter authFailureLimiter
	now := time.Now()
	for attempt := 0; attempt < authFailureLimit; attempt++ {
		limiter.fail("client", now)
	}
	if limiter.retryAfter("client", now) <= 0 {
		t.Fatal("limit not applied")
	}
	if limiter.retryAfter("client", now.Add(authFailureWindow)) != 0 {
		t.Fatal("limit did not expire")
	}
}

func TestAdminJSONBoundary(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, path := range []string{"/api/admin/token", "/api/admin/view-token", "/api/admin/config/import", "/api/admin/providers"} {
		for _, test := range []struct {
			body   string
			status int
		}{
			{`{"token":"` + strings.Repeat("a", maxRequestBody) + `"}`, http.StatusRequestEntityTooLarge},
			{`{} {}`, http.StatusBadRequest},
			{`null`, http.StatusBadRequest},
			{`[]`, http.StatusBadRequest},
			{`{"token":`, http.StatusBadRequest},
		} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(test.body))
			req.Header.Set("Authorization", "Bearer secret")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != test.status {
				t.Errorf("%s got %d, want %d", path, rec.Code, test.status)
			}
		}
	}
}

func (stubAdmin) CheckProvider(context.Context, string) (report.Report, error) {
	return report.Report{}, nil
}
func (stubAdmin) StopCheck() bool            { return false }
func (stubAdmin) RunningState() RunningState { return RunningState{} }
func (stubAdmin) AdminConfig(context.Context) (config.AdminConfig, error) {
	return config.AdminConfig{}, nil
}
func (stubAdmin) UpdateSettings(context.Context, config.RuntimeSettings) (config.AdminConfig, error) {
	return config.AdminConfig{}, nil
}
func (stubAdmin) UpsertProvider(context.Context, string, config.ProviderUpdate) (config.SafeProviderConfig, error) {
	return config.SafeProviderConfig{}, nil
}
func (stubAdmin) DeleteProvider(context.Context, string) error { return nil }
func (stubAdmin) ExportConfig(context.Context) (config.ConfigExport, error) {
	return config.ConfigExport{}, nil
}
func (stubAdmin) ImportConfig(context.Context, config.ConfigImport) (config.AdminConfig, error) {
	return config.AdminConfig{}, nil
}
func (stubAdmin) ReloadConfig(context.Context) (config.AdminConfig, error) {
	return config.AdminConfig{}, nil
}
func (stubAdmin) ListTasks(context.Context, storage.TaskQuery) ([]storage.CheckTask, error) {
	return nil, nil
}
func (stubAdmin) GetTask(context.Context, int64) (storage.CheckTask, error) {
	return storage.CheckTask{}, nil
}
func (s stubAdmin) AdminToken() string                           { return s.token }
func (stubAdmin) ChangeAdminToken(context.Context, string) error { return nil }
func (s stubAdmin) ViewToken() string                            { return s.viewToken }
func (stubAdmin) ChangeViewToken(context.Context, string) error  { return nil }

func newTestServer(t *testing.T) (*Server, *storage.SQLiteStore) {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.NewSQLite(context.Background(), filepath.Join(dir, "test.sqlite"), dir)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := config.Config{AdminToken: "secret", AppHost: "127.0.0.1", AppPort: 8080}
	srv := NewServer(cfg, store, func(context.Context) (report.Report, error) {
		return report.Report{}, nil
	}, nil, stubAdmin{token: "secret"})
	return srv, store
}

func TestHealthEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("expected ok=true, got %v", body)
	}
}

func TestStatusEndpointNoReport(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when no report, got %d", rec.Code)
	}
}

func TestStatusEndpointWithReport(t *testing.T) {
	srv, store := newTestServer(t)
	r := report.Report{Title: "integration-test", GeneratedAt: "2026-05-11 12:00:00", Total: 5, OKCount: 5}
	if err := store.SaveLatestReport(context.Background(), r); err != nil {
		t.Fatalf("save report: %v", err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["title"] != "integration-test" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestAdminEndpointRequiresToken(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/config", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rec.Code)
	}
}

func TestAdminEndpointAcceptsToken(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid token, got %d", rec.Code)
	}
}

func TestViewTokenCannotReadSensitiveConfig(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.admin = stubAdmin{token: "secret", viewToken: "view-only"}
	for _, path := range []string{"/api/admin/config", "/api/admin/config/export"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer view-only")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected view token to be rejected for %s, got %d", path, rec.Code)
		}
	}
}

func TestViewTokenRejectsShortToken(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/view-token", strings.NewReader(`{"token":"short"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected short view token to be rejected, got %d", rec.Code)
	}
}

func TestViewTokenRoleAndMutationBoundary(t *testing.T) {
	server, _ := newTestServer(t)
	server.admin = stubAdmin{token: "secret", viewToken: "view-only"}
	handler := server.Handler()
	for _, path := range []string{"/api/admin/check", "/api/admin/detection/start", "/api/admin/detection/stop", "/api/admin/providers/p1/rerun", "/api/admin/token", "/api/admin/view-token", "/api/admin/config/reload", "/api/admin/config/import"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer view-only")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("view token mutation %s: %d", path, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/detection", nil)
	request.Header.Set("Authorization", "Bearer view-only")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var state RunningState
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil || !state.ReadOnly || state.FirstUse {
		t.Fatalf("wrong read-only session: %+v, %v", state, err)
	}
}
