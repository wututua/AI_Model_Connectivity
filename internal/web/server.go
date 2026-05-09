package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cg/internal/config"
	"cg/internal/report"
	"cg/internal/storage"
)

type CheckFunc func(context.Context) (report.Report, error)

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
	store  storage.JSONStore
	check  CheckFunc
	broker *Broker
}

func NewServer(cfg config.Config, store storage.JSONStore, check CheckFunc, broker *Broker) *Server {
	if broker == nil {
		broker = NewBroker()
	}
	return &Server{cfg: cfg, store: store, check: check, broker: broker}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/api/status", s.status)
	mux.HandleFunc("/api/check", s.checkNow)
	mux.HandleFunc("/api/events", s.events)
	mux.Handle("/", http.FileServer(http.Dir(s.cfg.WebDir)))
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	value, err := storage.ReadJSON[report.Report](s.store.LatestReportPath(), report.Report{})
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

	if value, err := storage.ReadJSON[report.Report](s.store.LatestReportPath(), report.Report{}); err == nil && value.GeneratedAt != "" {
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
	if s.cfg.AdminToken != "" && r.Header.Get("Authorization") != "Bearer "+s.cfg.AdminToken {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	if s.cfg.AdminToken == "" && isPublicBindHost(s.cfg.AppHost) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "ADMIN_TOKEN is required when APP_HOST exposes /api/check publicly"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	value, err := s.check(ctx)
	if err != nil {
		if errors.Is(err, ErrCheckAlreadyRunning) {
			writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "report": value})
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
