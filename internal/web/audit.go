package web

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"cg/internal/storage"
)

// Only named operations are audited. No paths, queries, bodies or headers persist.
func auditAction(r *http.Request) string {
	switch r.Method + " " + r.URL.Path {
	case "POST /api/auth/login":
		return "auth.login"
	case "POST /api/auth/logout":
		return "auth.logout"
	case "POST /api/auth/password":
		return "auth.password"
	case "POST /api/admin/users":
		return "users.create"
	case "PUT /api/admin/settings":
		return "settings.update"
	case "POST /api/admin/config/import":
		return "config.import"
	case "GET /api/admin/config/export":
		return "config.export"
	case "GET /api/admin/export":
		return "data.export"
	case "POST /api/admin/providers":
		return "providers.create"
	case "POST /api/admin/providers/batch":
		return "providers.batch"
	case "POST /api/admin/provider-models":
		return "providers.discover"
	case "POST /api/admin/check", "POST /api/admin/detection/start":
		return "detection.start"
	case "POST /api/admin/detection/selected":
		return "detection.selected"
	case "POST /api/admin/detection/stop":
		return "detection.stop"
	case "POST /api/admin/notifications/test":
		return "notifications.test"
	case "POST /api/admin/metrics-tokens":
		return "metrics.create"
	case "POST /api/admin/updates/check":
		return "updates.check"
	case "POST /api/admin/updates/start":
		return "updates.start"
	case "POST /api/admin/updates/resolve":
		return "updates.resolve"
	case "PUT /api/admin/monitoring/settings":
		return "monitoring.settings"
	case "POST /api/admin/monitoring/backup":
		return "monitoring.backup"
	case "POST /api/admin/monitoring/verify":
		return "monitoring.verify"
	case "POST /api/admin/monitoring/approve":
		return "monitoring.approve"
	case "POST /api/admin/monitoring/ack":
		return "monitoring.ack"
	case "POST /api/admin/monitoring/test-rule":
		return "monitoring.test-rule"
	}
	for prefix, actions := range map[string]map[string]string{
		"/api/admin/users/":          {http.MethodPut: "users.update", http.MethodDelete: "users.delete"},
		"/api/admin/providers/":      {http.MethodPut: "providers.update", http.MethodDelete: "providers.delete"},
		"/api/admin/metrics-tokens/": {http.MethodDelete: "metrics.revoke"},
	} {
		if strings.HasPrefix(r.URL.Path, prefix) {
			if action := actions[r.Method]; action != "" {
				return action
			}
		}
	}
	if r.Method == http.MethodPost {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/admin/providers/") && strings.HasSuffix(r.URL.Path, "/rerun"):
			return "detection.provider"
		case strings.HasPrefix(r.URL.Path, "/api/admin/notifications/") && strings.HasSuffix(r.URL.Path, "/retry"):
			return "notifications.retry"
		case strings.HasPrefix(r.URL.Path, "/api/admin/metrics-tokens/") && strings.HasSuffix(r.URL.Path, "/rotate"):
			return "metrics.rotate"
		}
	}
	return ""
}

type auditWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *auditWriter) WriteHeader(code int) {
	if code >= 200 && w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *auditWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (s *Server) serveAudited(next http.Handler, w http.ResponseWriter, r *http.Request) {
	action := auditAction(r)
	if action == "" || s.store == nil {
		next.ServeHTTP(w, r)
		return
	}
	recorder := &auditWriter{ResponseWriter: w}
	defer func() {
		status := recorder.status
		if status == 0 {
			status = http.StatusInternalServerError
		}
		actor := requestSession(r).User
		// Persist completed mutations even if the browser canceled the request.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err := s.store.RecordAudit(ctx, storage.AuditEvent{
			ActorID: actor.ID, Actor: actor.Username, Role: actor.Role, Action: action, HTTPStatus: status,
		})
		if err != nil {
			slog.Error("administrator audit record could not be persisted")
		}
	}()
	next.ServeHTTP(recorder, r)
}

func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	common, err := monitoringHistoryQuery(r, false)
	if err != nil {
		writeErrorText(w, 400, err.Error())
		return
	}
	values := r.URL.Query()
	q := storage.AuditQuery{Actor: values.Get("actor"), Action: values.Get("action"), Result: values.Get("result"),
		Before: common.Before, Limit: common.Limit, Start: common.Start, End: common.End}
	if err := q.Validate(); err != nil {
		writeErrorText(w, 400, err.Error())
		return
	}
	page, err := s.store.QueryAudit(r.Context(), q)
	if err != nil {
		writeErrorText(w, 500, "无法读取操作审计")
		return
	}
	writeJSON(w, 200, struct {
		storage.HistoryPage[storage.AuditEvent]
		Actions []string `json:"actions"`
	}{page, storage.AuditActions()})
}
