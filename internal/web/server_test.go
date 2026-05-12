package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"cg/internal/config"
	"cg/internal/report"
	"cg/internal/storage"
)

// stubAdmin satisfies AdminController with no-op implementations.
type stubAdmin struct{ token string }

func (stubAdmin) CheckProvider(context.Context, string) (report.Report, error) {
	return report.Report{}, nil
}
func (stubAdmin) StopCheck() bool                                      { return false }
func (stubAdmin) RunningState() RunningState                           { return RunningState{} }
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
func (s stubAdmin) AdminToken() string                                  { return s.token }
func (stubAdmin) ChangeAdminToken(context.Context, string) error        { return nil }
func (stubAdmin) ActiveTheme() string                                   { return "default" }

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
