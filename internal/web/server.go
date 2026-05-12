package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"cg/internal/config"
	"cg/internal/report"
	"cg/internal/storage"
)

type CheckFunc func(context.Context) (report.Report, error)

type AdminController interface {
	CheckProvider(context.Context, string) (report.Report, error)
	StopCheck() bool
	RunningState() RunningState
	AdminToken() string
	ChangeAdminToken(context.Context, string) error
	DashboardTheme() string
	AdminTheme() string
	AdminConfig(context.Context) (config.AdminConfig, error)
	UpdateSettings(context.Context, config.RuntimeSettings) (config.AdminConfig, error)
	UpsertProvider(context.Context, string, config.ProviderUpdate) (config.SafeProviderConfig, error)
	DeleteProvider(context.Context, string) error
	ExportConfig(context.Context) (config.ConfigExport, error)
	ImportConfig(context.Context, config.ConfigImport) (config.AdminConfig, error)
	ReloadConfig(context.Context) (config.AdminConfig, error)
	ListTasks(context.Context, storage.TaskQuery) ([]storage.CheckTask, error)
	GetTask(context.Context, int64) (storage.CheckTask, error)
}

type RunningState struct {
	Running                   bool    `json:"running"`
	TaskID                    int64   `json:"task_id"`
	Kind                      string  `json:"kind"`
	ProviderID                string  `json:"provider_id"`
	AutoCheckIntervalMinHours float64 `json:"auto_check_interval_min_hours"`
	AutoCheckIntervalMaxHours float64 `json:"auto_check_interval_max_hours"`
	FirstUse                  bool    `json:"first_use"`
}

var ErrCheckAlreadyRunning = errors.New("check already running")

type Broker struct {
	mu      sync.Mutex
	clients map[chan report.Report]struct{}
}

func NewBroker() *Broker {
	return &Broker{clients: map[chan report.Report]struct{}{}}
}

func (b *Broker) Subscribe() (chan report.Report, func()) {
	ch := make(chan report.Report, 1)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.clients, ch)
		close(ch)
		b.mu.Unlock()
	}
}

func (b *Broker) Publish(value report.Report) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- value:
		default:
		}
	}
}

type Server struct {
	cfg    config.Config
	store  *storage.SQLiteStore
	check  CheckFunc
	broker *Broker
	admin  AdminController
}

func NewServer(cfg config.Config, store *storage.SQLiteStore, check CheckFunc, broker *Broker, admin AdminController) *Server {
	if broker == nil {
		broker = NewBroker()
	}
	return &Server{cfg: cfg, store: store, check: check, broker: broker, admin: admin}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/api/status", s.status)
	mux.HandleFunc("/api/admin/check", s.checkNow)
	mux.HandleFunc("/api/events", s.events)
	mux.HandleFunc("/api/admin/detection", s.adminDetection)
	mux.HandleFunc("/api/admin/detection/start", s.adminDetectionStart)
	mux.HandleFunc("/api/admin/detection/stop", s.adminDetectionStop)
	mux.HandleFunc("/api/admin/config", s.adminConfig)
	mux.HandleFunc("/api/admin/config/export", s.adminConfigExport)
	mux.HandleFunc("/api/admin/config/import", s.adminConfigImport)
	mux.HandleFunc("/api/admin/config/reload", s.adminConfigReload)
	mux.HandleFunc("/api/admin/settings", s.adminSettings)
	mux.HandleFunc("/api/admin/providers", s.adminProviders)
	mux.HandleFunc("/api/admin/providers/", s.adminProviderItem)
	mux.HandleFunc("/api/admin/tasks", s.adminTasks)
	mux.HandleFunc("/api/admin/tasks/", s.adminTaskItem)
	mux.HandleFunc("/api/admin/token", s.adminChangeToken)
	mux.HandleFunc("/api/admin/themes", s.adminThemes)
	mux.Handle("/", spaHandler(s.cfg.WebDir, s.admin.DashboardTheme, s.admin.AdminTheme))
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	value, err := s.store.LatestReport(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if value.GeneratedAt == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "no report available"})
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsubscribe := s.broker.Subscribe()
	defer unsubscribe()

	if value, err := s.store.LatestReport(r.Context()); err == nil && value.GeneratedAt != "" {
		writeSSE(w, flusher, value)
	}

	keepAlive := time.NewTimer(25 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case value := <-ch:
			writeSSE(w, flusher, value)
			if !keepAlive.Stop() {
				select {
				case <-keepAlive.C:
				default:
				}
			}
			keepAlive.Reset(25 * time.Second)
		case <-keepAlive.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
			keepAlive.Reset(25 * time.Second)
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) checkNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	value, err := s.check(ctx)
	if err != nil {
		s.writeCheckError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "report": value})
}

func (s *Server) adminDetection(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, s.admin.RunningState())
}

func (s *Server) adminDetectionStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	value, err := s.check(ctx)
	if err != nil {
		s.writeCheckError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "report": value})
}

func (s *Server) adminDetectionStop(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stopped": s.admin.StopCheck()})
}

func (s *Server) adminConfig(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	value, err := s.admin.AdminConfig(r.Context())
	writeResult(w, value, err)
}

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}
	var value config.RuntimeSettings
	if !decodeJSON(w, r, &value) {
		return
	}
	result, err := s.admin.UpdateSettings(r.Context(), value)
	writeResult(w, result, err)
}

func (s *Server) adminProviders(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.admin.AdminConfig(r.Context())
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, value.Providers)
	case http.MethodPost:
		var value config.ProviderUpdate
		if !decodeJSON(w, r, &value) {
			return
		}
		result, err := s.admin.UpsertProvider(r.Context(), "", value)
		writeResult(w, result, err)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) adminProviderItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/providers/")
	if id, ok := strings.CutSuffix(path, "/rerun"); ok {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()
		value, err := s.admin.CheckProvider(ctx, id)
		if err != nil {
			s.writeCheckError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "report": value})
		return
	}
	id := strings.Trim(path, "/")
	if id == "" {
		writeErrorText(w, http.StatusBadRequest, "provider id is required")
		return
	}
	switch r.Method {
	case http.MethodPut:
		var value config.ProviderUpdate
		if !decodeJSON(w, r, &value) {
			return
		}
		result, err := s.admin.UpsertProvider(r.Context(), id, value)
		writeResult(w, result, err)
	case http.MethodDelete:
		writeResult(w, map[string]any{"ok": true}, s.admin.DeleteProvider(r.Context(), id))
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) adminConfigExport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	value, err := s.admin.ExportConfig(r.Context())
	writeResult(w, value, err)
}

func (s *Server) adminConfigImport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var value config.ConfigImport
	if !decodeJSON(w, r, &value) {
		return
	}
	result, err := s.admin.ImportConfig(r.Context(), value)
	writeResult(w, result, err)
}

func (s *Server) adminConfigReload(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	result, err := s.admin.ReloadConfig(r.Context())
	if err == nil {
		go s.checkAfterReload()
	}
	writeResult(w, result, err)
}

func (s *Server) checkAfterReload() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if _, err := s.check(ctx); err != nil {
		slog.Warn("reload check failed", "err", err)
	}
}

func (s *Server) adminTasks(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	tasks, err := s.admin.ListTasks(r.Context(), storage.TaskQuery{Limit: limit, Offset: offset, Status: r.URL.Query().Get("status"), ProviderID: r.URL.Query().Get("provider_id")})
	writeResult(w, tasks, err)
}

func (s *Server) adminTaskItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/admin/tasks/"), 10, 64)
	if err != nil {
		writeErrorText(w, http.StatusBadRequest, "invalid task id")
		return
	}
	value, err := s.admin.GetTask(r.Context(), id)
	writeResult(w, value, err)
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	token := s.admin.AdminToken()
	if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return false
	}
	if token == "" && isPublicBindHost(s.cfg.AppHost) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "ADMIN_TOKEN is required for admin APIs when APP_HOST is public"})
		return false
	}
	return true
}

func (s *Server) adminChangeToken(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "token cannot be empty"})
		return
	}
	if err := s.admin.ChangeAdminToken(r.Context(), body.Token); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) writeCheckError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrCheckAlreadyRunning) {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
}

func (s *Server) HTTPServer() *http.Server {
	addr := fmt.Sprintf("%s:%d", s.cfg.AppHost, s.cfg.AppPort)
	slog.Info("server started", "web_dir", filepath.Clean(s.cfg.WebDir), "addr", "http://"+addr+"/")
	return &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// spaHandler serves static files from the appropriate theme directory.
// /admin* paths use the admin theme; everything else uses the dashboard
// theme.  If the requested theme directory doesn't exist on disk, falls
// back to webDir/themes/default/, then to webDir itself (legacy single-
// theme layouts).
//
// For any path that has no file extension and doesn't start with /api/,
// the React SPA's index.html is served so client-side routing works.
func spaHandler(webDir string, dashboardTheme, adminTheme func() string) http.Handler {
	resolveDir := func(theme string) string {
		theme = strings.TrimSpace(theme)
		if theme == "" {
			theme = "default"
		}
		candidates := []string{
			filepath.Join(webDir, "themes", theme),
			filepath.Join(webDir, "themes", "default"),
			webDir,
		}
		for _, dir := range candidates {
			if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
				return dir
			}
		}
		return webDir
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// /admin and /admin/* render the admin theme; everything else uses dashboard.
		theme := dashboardTheme()
		if r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, "/admin/") {
			theme = adminTheme()
		}
		dir := resolveDir(theme)
		fpath := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if _, err := os.Stat(fpath); err == nil && !strings.HasSuffix(fpath, string(os.PathSeparator)) {
			http.FileServer(http.Dir(dir)).ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

// adminThemes lists themes available under webDir/themes/ (each subdirectory
// containing an index.html is considered a built theme).
func (s *Server) adminThemes(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	themesDir := filepath.Join(s.cfg.WebDir, "themes")
	entries, err := os.ReadDir(themesDir)
	if err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	type themeInfo struct {
		ID    string `json:"id"`
		Built bool   `json:"built"`
	}
	themes := []themeInfo{}
	seen := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		_, err := os.Stat(filepath.Join(themesDir, name, "index.html"))
		themes = append(themes, themeInfo{ID: name, Built: err == nil})
		seen[name] = true
	}
	// Ensure built-in theme IDs always show up, even if not yet built.
	for _, id := range []string{"default", "argon"} {
		if !seen[id] {
			themes = append(themes, themeInfo{ID: id, Built: false})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dashboard_theme": s.admin.DashboardTheme(),
		"admin_theme":     s.admin.AdminTheme(),
		"themes":          themes,
	})
}

func isPublicBindHost(host string) bool {
	switch strings.TrimSpace(strings.ToLower(host)) {
	case "", "0.0.0.0", "::", "[::]":
		return true
	default:
		return false
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, value report.Report) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	if err := json.NewDecoder(r.Body).Decode(value); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func writeResult(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeErrorText(w, status, err.Error())
}

func writeErrorText(w http.ResponseWriter, status int, text string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": text})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
