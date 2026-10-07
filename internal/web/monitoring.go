package web

import (
	"errors"
	"net/http"
	"strings"

	"cg/internal/config"
	"cg/internal/notify"
	"cg/internal/storage"
)

func (s *Server) adminMonitoring(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/api/admin/monitoring")
	if action == "/diagnostics" || action == "/incidents" {
		s.monitoringHistory(w, r, action == "/incidents")
		return
	}
	if action == "" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		settings, err := s.store.MonitoringSettings(r.Context())
		if err != nil {
			writeErrorText(w, 500, "无法读取监控配置")
			return
		}
		catalogs, err := s.store.Catalogs(r.Context())
		if err != nil {
			writeErrorText(w, 500, "无法读取模型清单")
			return
		}
		events, err := s.store.CatalogEvents(r.Context())
		if err != nil {
			writeErrorText(w, 500, "无法读取模型变更")
			return
		}
		backups, err := s.store.Backups(r.Context())
		if err != nil {
			writeErrorText(w, 500, "无法读取备份")
			return
		}
		diagnostics, err := s.store.DiagnosticRecords(r.Context())
		if err != nil {
			writeErrorText(w, 500, "无法读取诊断")
			return
		}
		incidents, err := s.store.Incidents(r.Context())
		if err != nil {
			writeErrorText(w, 500, "无法读取事件")
			return
		}
		audit, err := s.store.MonitorEvents(r.Context())
		if err != nil {
			writeErrorText(w, 500, "无法读取运行记录")
			return
		}
		cost, err := s.store.CostSummary(r.Context(), settings)
		if err != nil {
			writeErrorText(w, 500, "无法读取费用")
			return
		}
		schedules, err := s.store.ScheduleStatus(r.Context())
		if err != nil {
			writeErrorText(w, 500, "无法读取调度")
			return
		}
		visibleSchedules := []storage.ScheduleStatus{}
		if s.admin != nil && settings.IndependentSchedules() {
			cfg, configErr := s.admin.AdminConfig(r.Context())
			if configErr != nil {
				writeErrorText(w, 503, "无法读取当前调度配置")
				return
			}
			for _, item := range schedules {
				for _, p := range cfg.Providers {
					if item.ProviderID == p.ID && item.Revision == p.ConnectionRevision && p.Enabled && p.ProbeEnabled {
						visibleSchedules = append(visibleSchedules, item)
					}
				}
			}
		}
		writeJSON(w, 200, map[string]any{"settings": settings.Safe(), "catalogs": catalogs, "catalog_events": events, "backups": backups, "diagnostics": diagnostics, "incidents": incidents, "events": audit, "cost": cost, "schedules": visibleSchedules})
		return
	}
	if action == "/settings" {
		if r.Method != http.MethodPut {
			methodNotAllowed(w)
			return
		}
		var input config.MonitoringSettings
		if !decodeJSON(w, r, &input) {
			return
		}
		value, err := s.store.SaveMonitoringSettings(r.Context(), input)
		if err != nil {
			status := 400
			if errors.Is(err, storage.ErrMonitoringConflict) {
				status = 409
			}
			writeErrorText(w, status, err.Error())
			return
		}
		writeJSON(w, 200, value)
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	switch action {
	case "/test-rule":
		var input struct {
			ID string `json:"id"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		settings, err := s.store.MonitoringSettings(r.Context())
		if err != nil {
			writeErrorText(w, 500, "无法读取告警规则")
			return
		}
		for _, rule := range settings.Rules {
			if rule.ID != input.ID {
				continue
			}
			client := notify.New(rule.Apply(s.cfg), storage.RuleStateStore{Store: s.store, Key: rule.ID})
			client.SetHistory(s.store)
			client.SetRuleID(rule.ID)
			value, err := client.SendTest(r.Context())
			if err != nil {
				writeErrorText(w, 502, "规则通知测试失败，请检查通知记录")
				return
			}
			writeJSON(w, 200, value)
			return
		}
		writeErrorText(w, 404, "告警规则不存在")
	case "/backup":
		settings, err := s.store.MonitoringSettings(r.Context())
		if err != nil {
			writeErrorText(w, 500, "无法读取备份配置")
			return
		}
		value, err := s.store.CreateBackup(r.Context(), settings.BackupKeep)
		if err != nil {
			_ = s.store.AddMonitorEvent(r.Context(), "backup_error", "manual backup failed")
			writeErrorText(w, 500, "备份失败，请检查磁盘空间及数据库权限")
			return
		}
		_ = s.store.AddMonitorEvent(r.Context(), "backup", "manual backup completed")
		writeJSON(w, 201, value)
	case "/verify":
		var input struct {
			Name string `json:"name"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if err := s.store.VerifyBackup(r.Context(), input.Name); err != nil {
			writeErrorText(w, 400, "备份不存在或完整性校验失败")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "/approve":
		var input struct {
			ProviderID string `json:"provider_id"`
			Revision   string `json:"revision"`
			UpdatedAt  string `json:"updated_at"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if s.admin == nil {
			writeErrorText(w, 503, "配置服务不可用")
			return
		}
		cfg, err := s.admin.AdminConfig(r.Context())
		valid := false
		for _, p := range cfg.Providers {
			if p.ID == input.ProviderID && p.ConnectionRevision == input.Revision && len(p.Models) == 0 {
				valid = true
			}
		}
		if err != nil || !valid {
			writeErrorText(w, 409, "Provider 已变更，请刷新")
			return
		}
		if err := s.store.ApproveCatalog(r.Context(), input.ProviderID, input.Revision, input.UpdatedAt); err != nil {
			writeErrorText(w, 409, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	case "/ack":
		var input struct {
			ID   int64  `json:"id"`
			Note string `json:"note"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		if err := s.store.AcknowledgeIncident(r.Context(), input.ID, input.Note); err != nil {
			writeErrorText(w, 409, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	default:
		writeErrorText(w, 404, "not found")
	}
}
