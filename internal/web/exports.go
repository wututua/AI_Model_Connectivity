package web

import (
	"encoding/csv"
	"errors"
	"net/http"
	"runtime"
	"strings"
	"time"
	"unicode"

	"cg/internal/storage"
)

func (s *Server) adminExportData(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	params := r.URL.Query()
	start, startErr := time.Parse(time.DateOnly, params.Get("start"))
	end, endErr := time.Parse(time.DateOnly, params.Get("end"))
	kind := params.Get("kind")
	if startErr != nil || endErr != nil || end.Before(start) || end.Sub(start) > 365*24*time.Hour ||
		(kind != "history" && kind != "usage") {
		writeErrorText(w, 400, "请指定 history 或 usage，以及不超过 366 天的 UTC 日期范围")
		return
	}
	rows, err := s.store.ExportRows(r.Context(), storage.ExportQuery{Kind: kind, Start: start, End: end.AddDate(0, 0, 1),
		ProviderID: params.Get("provider_id"), Model: params.Get("model")})
	if errors.Is(err, storage.ErrExportTooLarge) {
		writeError(w, 413, err)
		return
	}
	if err != nil {
		writeErrorText(w, 500, "导出失败")
		return
	}
	for _, row := range rows {
		for i := range row {
			row[i] = csvSafe(row[i])
		}
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+kind+`.csv"`)
	_, _ = w.Write([]byte{0xef, 0xbb, 0xbf})
	writer := csv.NewWriter(w)
	writer.UseCRLF = true
	writer.WriteAll(rows)
}

func csvSafe(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if strings.ContainsAny(value, "\t\r\n") || strings.HasPrefix(trimmed, "=") ||
		strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "@") {
		return "'" + value
	}
	return value
}

func (s *Server) adminDiagnostics(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	cfg, err := s.admin.AdminConfig(r.Context())
	if err != nil {
		writeErrorText(w, 500, "无法读取诊断信息")
		return
	}
	budget, err := s.store.RequestBudget(r.Context(), cfg.Settings.DailyRequestLimit)
	if err != nil {
		writeErrorText(w, 500, "无法读取请求预算")
		return
	}
	enabled, probing, streaming, responses := 0, 0, 0, 0
	for _, item := range cfg.Providers {
		if item.Enabled {
			enabled++
		}
		if item.Enabled && item.ProbeEnabled {
			probing++
		}
		if item.Probe.Stream {
			streaming++
		}
		if item.Probe.Protocol == "responses" {
			responses++
		}
	}
	// Explicit allowlist: never export the effective config or raw task errors.
	value := map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339), "go_version": runtime.Version(),
		"os": runtime.GOOS, "arch": runtime.GOARCH,
		"provider_count": len(cfg.Providers), "enabled_count": enabled, "probe_count": probing,
		"streaming_count": streaming, "responses_count": responses,
		"status_login_required": cfg.Settings.StatusLoginRequired,
		"history_enabled":       cfg.Settings.EnableHistory, "concurrency": cfg.Settings.Concurrency,
		"request_budget": budget, "running": s.admin.RunningState().Running,
	}
	w.Header().Set("Content-Disposition", `attachment; filename="diagnostics.json"`)
	writeJSON(w, 200, value)
}
