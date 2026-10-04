package web

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/auth"
	"cg/internal/config"
	"cg/internal/probe"
	"cg/internal/report"
	"cg/internal/storage"
)

// stubAdmin satisfies AdminController with no-op implementations.
type stubAdmin struct{}

type visibilityAdmin struct {
	stubAdmin
	show atomic.Bool
	fail atomic.Bool
}

func (a *visibilityAdmin) AdminConfig(context.Context) (config.AdminConfig, error) {
	if a.fail.Load() {
		return config.AdminConfig{}, errors.New("config unavailable")
	}
	return config.AdminConfig{Settings: config.RuntimeSettings{ShowErrorDetail: a.show.Load()}}, nil
}

func sensitiveReport() report.Report {
	return report.Report{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		Providers:      []report.ProviderReport{{ProviderID: "p1", Results: []report.ModelResult{{Result: probe.Result{Error: "private-model"}}}}},
		ProviderErrors: []probe.ProviderError{{ProviderID: "p1", Error: "private-provider"}},
	}
}

func TestStatusAppliesCurrentVisibilityToOldReport(t *testing.T) {
	srv, store := newTestServer(t)
	admin := &visibilityAdmin{}
	srv.admin = admin
	if err := store.SaveLatestReport(context.Background(), sensitiveReport()); err != nil {
		t.Fatal(err)
	}
	for _, show := range []bool{true, false, true} {
		admin.show.Store(show)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
		if strings.Contains(rec.Body.String(), "private-") != show || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("visibility=%v, body=%s", show, rec.Body)
		}
	}
	admin.fail.Store(true)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if strings.Contains(rec.Body.String(), "private-") {
		t.Fatal("failed config lookup exposed details")
	}
}

func TestSSEAppliesCurrentVisibilityToInitialAndPublishedReports(t *testing.T) {
	srv, store := newTestServer(t)
	admin := &visibilityAdmin{}
	srv.admin = admin
	value := sensitiveReport()
	if err := store.SaveLatestReport(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(srv.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/events", nil)
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	readEvent := func(show bool) {
		t.Helper()
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "data:") {
				if strings.Contains(scanner.Text(), "private-") != show {
					t.Fatalf("visibility=%v, event=%s", show, scanner.Text())
				}
				return
			}
		}
		t.Fatalf("missing SSE event: %v", scanner.Err())
	}
	readEvent(false)
	admin.show.Store(true)
	srv.broker.Publish(value)
	readEvent(true)
	admin.show.Store(false)
	srv.broker.Publish(value)
	readEvent(false)
	if value.ProviderErrors[0].Error == "" || value.Providers[0].Results[0].Error == "" {
		t.Fatal("SSE modified shared report")
	}
}

func TestBrokerReplacesPendingReportWithNewest(t *testing.T) {
	broker := NewBroker()
	updates, unsubscribe := broker.Subscribe()
	defer unsubscribe()
	broker.Publish(report.Report{Title: "old"})
	broker.Publish(report.Report{Title: "new"})
	if value := <-updates; value.Title != "new" {
		t.Fatal("slow subscriber received stale report")
	}
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
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"Wrong123"}`))
		req.Header.Set("Content-Type", "application/json")
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
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"TestPass1"}`))
	req.RemoteAddr = "203.0.113.1:4321"
	req.Header.Set("Content-Type", "application/json")
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
	for _, path := range []string{"/api/auth/password", "/api/admin/users", "/api/admin/config/import", "/api/admin/providers"} {
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
			authenticateTestRequest(req, false)
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
func (stubAdmin) DiscoverModels(context.Context, config.ModelDiscoveryRequest) ([]string, error) {
	return []string{"test-model"}, nil
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

func newTestServer(t *testing.T) (*Server, *storage.SQLiteStore) {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.NewSQLite(context.Background(), filepath.Join(dir, "test.sqlite"), dir)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	hashOnce.Do(func() { testPasswordHash, _ = auth.HashPassword("TestPass1") })
	for _, role := range []string{"admin", "user"} {
		user, err := store.CreateUser(context.Background(), storage.User{Username: role, Role: role, Enabled: true, PasswordHash: testPasswordHash}, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.CreateSession(context.Background(), user, testSessionToken(role == "user"), "test-csrf", time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{AppHost: "127.0.0.1", AppPort: 8080}
	srv := NewServer(cfg, store, func(context.Context) (report.Report, error) {
		return report.Report{}, nil
	}, nil, stubAdmin{})
	return srv, store
}

var hashOnce sync.Once
var testPasswordHash string

func testSessionToken(readOnly bool) string {
	if readOnly {
		return strings.Repeat("u", 43)
	}
	return strings.Repeat("a", 43)
}

func authenticateTestRequest(r *http.Request, readOnly bool) {
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: testSessionToken(readOnly)})
	r.Header.Set("X-CSRF-Token", "test-csrf")
	r.Header.Set("Content-Type", "application/json")
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

func TestAdminEndpointRequiresSession(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/config", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", rec.Code)
	}
}

func TestAdminEndpointAcceptsSession(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	authenticateTestRequest(req, false)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid session, got %d", rec.Code)
	}
}

func TestOrdinaryUserCannotReadSensitiveConfig(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, path := range []string{"/api/admin/config", "/api/admin/config/export"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		authenticateTestRequest(req, true)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected ordinary user to be rejected for %s, got %d", path, rec.Code)
		}
	}
}

func TestNewUserRejectsWeakPassword(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", strings.NewReader(`{"username":"newuser","password":"short","role":"user","enabled":true}`))
	authenticateTestRequest(req, false)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected weak password to be rejected, got %d", rec.Code)
	}
}

func TestOrdinaryUserRoleAndMutationBoundary(t *testing.T) {
	server, _ := newTestServer(t)
	handler := server.Handler()
	for _, path := range []string{"/api/admin/check", "/api/admin/detection/start", "/api/admin/detection/stop", "/api/admin/providers/p1/rerun", "/api/admin/users", "/api/admin/config/reload", "/api/admin/config/import"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		authenticateTestRequest(request, true)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("ordinary user mutation %s: %d", path, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/detection", nil)
	authenticateTestRequest(request, true)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var state RunningState
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil || !state.ReadOnly {
		t.Fatalf("wrong read-only session: %+v, %v", state, err)
	}
}
