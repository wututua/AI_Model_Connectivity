package web

import (
	"context"
	"fmt"
	"net/http"

	"cg/internal/config"
	"cg/internal/storage"
)

type selectionController interface {
	StartSelectedCheck(context.Context, config.CheckSelection) (storage.CheckTask, error)
	BatchProviders(context.Context, config.ProviderBatch) (config.AdminConfig, error)
}

func (s *Server) adminDetectionSelected(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input config.CheckSelection
	if !decodeJSON(w, r, &input) {
		return
	}
	controller, ok := s.admin.(selectionController)
	if !ok {
		writeErrorText(w, http.StatusServiceUnavailable, "检测服务不可用")
		return
	}
	task, err := controller.StartSelectedCheck(r.Context(), input)
	if err != nil {
		s.writeCheckError(w, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/api/admin/tasks/%d", task.ID))
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "task": task})
}

func (s *Server) adminProviderBatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input config.ProviderBatch
	if !decodeJSON(w, r, &input) {
		return
	}
	controller, ok := s.admin.(selectionController)
	if !ok {
		writeErrorText(w, http.StatusServiceUnavailable, "配置服务不可用")
		return
	}
	value, err := controller.BatchProviders(r.Context(), input)
	writeResult(w, value, err)
}

func (s *Server) adminBudget(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	cfg, err := s.admin.AdminConfig(r.Context())
	if err != nil {
		writeErrorText(w, 500, "无法读取配置")
		return
	}
	value, err := s.store.RequestBudget(r.Context(), cfg.Settings.DailyRequestLimit)
	writeResult(w, value, err)
}
