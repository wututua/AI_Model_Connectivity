package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
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
	AdminConfig(context.Context) (config.AdminConfig, error)
	UpdateSettings(context.Context, config.RuntimeSettings) (config.AdminConfig, error)
	UpsertProvider(context.Context, string, config.ProviderUpdate) (config.SafeProviderConfig, error)
	DeleteProvider(context.Context, string) error
	ExportConfig(context.Context) (config.ConfigExport, error)
	ImportConfig(context.Context, config.ConfigImport) (config.AdminConfig, error)
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
	mux.HandleFunc("/api/check", s.checkNow)
	mux.HandleFunc("/api/events", s.events)
	mux.HandleFunc("/api/admin/detection", s.adminDetection)
	mux.HandleFunc("/api/admin/detection/start", s.adminDetectionStart)
	mux.HandleFunc("/api/admin/detection/stop", s.adminDetectionStop)
	mux.HandleFunc("/api/admin/config", s.adminConfig)
	mux.HandleFunc("/api/admin/config/export", s.adminConfigExport)
	mux.HandleFunc("/api/admin/config/import", s.adminConfigImport)
	mux.HandleFunc("/api/admin/settings", s.adminSettings)
	mux.HandleFunc("/api/admin/providers", s.adminProviders)
	mux.HandleFunc("/api/admin/providers/", s.adminProviderItem)
	mux.HandleFunc("/api/admin/tasks", s.adminTasks)
	mux.HandleFunc("/api/admin/tasks/", s.adminTaskItem)
	mux.Handle("/", http.FileServer(http.Dir(s.cfg.WebDir)))
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

	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case value := <-ch:
			writeSSE(w, flusher, value)
		case <-keepAlive.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
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
	if strings.HasSuffix(path, "/rerun") {
		id := strings.TrimSuffix(path, "/rerun")
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
	if s.cfg.AdminToken != "" && r.Header.Get("Authorization") != "Bearer "+s.cfg.AdminToken {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return false
	}
	if s.cfg.AdminToken == "" && isPublicBindHost(s.cfg.AppHost) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "ADMIN_TOKEN is required for admin APIs when APP_HOST is public"})
		return false
	}
	return true
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
	log.Printf("serving %s at http://%s/", filepath.Clean(s.cfg.WebDir), addr)
	return &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
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
