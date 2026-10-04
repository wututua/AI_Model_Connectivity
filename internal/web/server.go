package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"cg/internal/config"
	"cg/internal/metrics"
	"cg/internal/report"
	"cg/internal/storage"
)

type CheckFunc func(context.Context) (report.Report, error)

type AdminController interface {
	CheckProvider(context.Context, string) (report.Report, error)
	StopCheck() bool
	RunningState() RunningState
	AdminConfig(context.Context) (config.AdminConfig, error)
	UpdateSettings(context.Context, config.RuntimeSettings) (config.AdminConfig, error)
	UpsertProvider(context.Context, string, config.ProviderUpdate) (config.SafeProviderConfig, error)
	DiscoverModels(context.Context, config.ModelDiscoveryRequest) ([]string, error)
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
	ReadOnly                  bool    `json:"read_only"`
}

var ErrCheckAlreadyRunning = errors.New("check already running")
var ErrShuttingDown = errors.New("service is shutting down")

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
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- value:
			default:
			}
		}
	}
}

type Server struct {
	cfg           config.Config
	store         *storage.SQLiteStore
	check         CheckFunc
	broker        *Broker
	admin         AdminController
	metrics       *metrics.Metrics
	authFailures  authFailureLimiter
	passwordSlots chan struct{}
}

func NewServer(cfg config.Config, store *storage.SQLiteStore, check CheckFunc, broker *Broker, admin AdminController) *Server {
	if broker == nil {
		broker = NewBroker()
	}
	return &Server{cfg: cfg, store: store, check: check, broker: broker, admin: admin, passwordSlots: make(chan struct{}, 4)}
}

// SetMetrics wires a metrics collector and exposes /metrics.  Optional —
// callers that don't care about Prometheus simply skip this.
func (s *Server) SetMetrics(m *metrics.Metrics) {
	s.metrics = m
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
	mux.HandleFunc("/api/admin/provider-models", s.adminProviderModels)
	mux.HandleFunc("/api/admin/tasks", s.adminTasks)
	mux.HandleFunc("/api/admin/billing", s.adminBilling)
	mux.HandleFunc("/metrics", s.metricsHandler)
	mux.HandleFunc("/api/admin/tasks/", s.adminTaskItem)
	mux.HandleFunc("/api/auth/session", s.authSession)
	mux.HandleFunc("/api/auth/login", s.authLogin)
	mux.HandleFunc("/api/auth/logout", s.authLogout)
	mux.HandleFunc("/api/auth/password", s.authPassword)
	mux.HandleFunc("/api/admin/users", s.adminUsers)
	mux.HandleFunc("/api/admin/users/", s.adminUserItem)
	mux.Handle("/", spaHandler(s.cfg.WebDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/metrics" {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if !s.requireStatusAccess(w, r) {
		return
	}
	value, err := s.store.LatestReport(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if value.GeneratedAt == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "no report available"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.publicReport(r.Context(), value))
}

func (s *Server) publicReport(ctx context.Context, value report.Report) report.Report {
	show := s.cfg.ShowErrorDetail
	if s.admin != nil {
		cfg, err := s.admin.AdminConfig(ctx)
		show = err == nil && cfg.Settings.ShowErrorDetail
	}
	return report.WithErrorVisibility(value, show)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if !s.requireStatusAccess(w, r) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsubscribe := s.broker.Subscribe()
	defer unsubscribe()

	if value, err := s.store.LatestReport(r.Context()); err == nil && value.GeneratedAt != "" {
		if !s.streamAuthorized(r) {
			fmt.Fprint(w, "event: auth-required\ndata: {}\n\n")
			flusher.Flush()
			return
		}
		writeSSE(w, flusher, s.publicReport(r.Context(), value))
	}
	flusher.Flush()

	keepAlive := time.NewTimer(5 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case value, open := <-ch:
			if !open {
				return
			}
			if !s.streamAuthorized(r) {
				fmt.Fprint(w, "event: auth-required\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			writeSSE(w, flusher, s.publicReport(r.Context(), value))
			if !keepAlive.Stop() {
				select {
				case <-keepAlive.C:
				default:
				}
			}
			keepAlive.Reset(5 * time.Second)
		case <-keepAlive.C:
			if !s.streamAuthorized(r) {
				fmt.Fprint(w, "event: auth-required\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
			keepAlive.Reset(5 * time.Second)
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
	if !s.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	state := s.admin.RunningState()
	state.ReadOnly = requestSession(r).User.Role != "admin"
	writeJSON(w, http.StatusOK, state)
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
	switch r.Method {
	case http.MethodGet:
		if !s.requireAuth(w, r) {
			return
		}
		value, err := s.admin.AdminConfig(r.Context())
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, value.Providers)
	case http.MethodPost:
		if !s.requireAdmin(w, r) {
			return
		}
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

func (s *Server) adminProviderModels(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var query config.ModelDiscoveryRequest
	if !decodeJSON(w, r, &query) {
		return
	}
	models, err := s.admin.DiscoverModels(r.Context(), query)
	writeResult(w, models, err)
}

func (s *Server) adminProviderItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/providers/")
	id, rerun := strings.CutSuffix(path, "/rerun")
	if err := config.ValidateProviderID(id); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if rerun {
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
	if !s.requireAuth(w, r) {
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

// Metrics require an authenticated account, independent of status-page privacy.
func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if s.metrics == nil {
		http.NotFound(w, r)
		return
	}
	if !s.requireAuth(w, r) {
		return
	}
	s.metrics.Handler().ServeHTTP(w, r)
}

func (s *Server) adminBilling(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	summary, err := s.store.LoadBillingSummary(r.Context(), days)
	writeResult(w, summary, err)
}

func (s *Server) adminTaskItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
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

// randomToken returns a URL-safe base64 token with the given byte length
// of entropy.  Uses crypto/rand so tokens aren't guessable from system time.
func randomToken(bytes int) (string, error) {
	if bytes <= 0 {
		bytes = 16
	}
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *Server) writeCheckError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrShuttingDown) {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	if errors.Is(err, ErrCheckAlreadyRunning) {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
}

func (s *Server) HTTPServer() *http.Server {
	addr := net.JoinHostPort(strings.Trim(s.cfg.AppHost, "[]"), strconv.Itoa(s.cfg.AppPort))
	slog.Info("server started", "web_dir", filepath.Clean(s.cfg.WebDir), "addr", "http://"+addr+"/")
	return &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// spaHandler serves static files from webDir; falls back to index.html for
// any path that has no file extension and doesn't start with /api/, so that
// the React SPA handles client-side routing (e.g. /admin).
func spaHandler(webDir string) http.Handler {
	fs := http.FileServer(http.Dir(webDir))
	fonts := &fontVersionCache{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// If the file exists on disk, serve it directly (assets, index.html, etc.).
		fpath := filepath.Join(webDir, filepath.Clean(r.URL.Path))
		w.Header().Set("Cache-Control", "no-cache")
		if info, err := os.Stat(fpath); err == nil {
			if !info.IsDir() && strings.HasPrefix(r.URL.Path, "/fonts/") && strings.EqualFold(filepath.Ext(fpath), ".ttf") {
				if digest := fonts.digest(fpath, info); digest != "" {
					w.Header().Set("ETag", `"`+digest+`"`)
					if r.URL.Query().Get("v") == digest {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
				}
			}
			fs.ServeHTTP(w, r)
			return
		}
		// SPA fallback: let the React router handle the path.
		http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
	})
}

func IsPublicBindHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
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
	return decodeRequestJSON(w, r, value, false)
}

const maxRequestBody = 1 << 20

func decodeRequestJSON(w http.ResponseWriter, r *http.Request, value any, allowEmpty bool) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	var payload json.RawMessage
	err := decoder.Decode(&payload)
	if allowEmpty && errors.Is(err, io.EOF) {
		return true
	}
	if err == nil {
		if len(payload) == 0 || payload[0] != '{' {
			writeErrorText(w, http.StatusBadRequest, "request body must be a JSON object")
			return false
		}
		if err := json.Unmarshal(payload, value); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return false
		}
		var extra any
		err = decoder.Decode(&extra)
		if errors.Is(err, io.EOF) {
			return true
		}
		if err == nil {
			err = errors.New("request body must contain a single JSON value")
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeErrorText(w, http.StatusRequestEntityTooLarge, "request body exceeds 1 MiB")
	} else {
		writeError(w, http.StatusBadRequest, err)
	}
	return false
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
