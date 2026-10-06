package web

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"cg/internal/notify"
)

var ErrNotificationBusy = errors.New("已有手动通知正在发送，请稍后重试")

func (s *Server) adminNotifications(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	query := notify.DeliveryQuery{Limit: 20, Status: r.URL.Query().Get("status")}
	if text := r.URL.Query().Get("limit"); text != "" {
		value, err := strconv.Atoi(text)
		if err != nil || value < 1 || value > 200 {
			writeErrorText(w, http.StatusBadRequest, "limit 必须为 1 到 200")
			return
		}
		query.Limit = value
	}
	if text := r.URL.Query().Get("offset"); text != "" {
		value, err := strconv.Atoi(text)
		if err != nil || value < 0 {
			writeErrorText(w, http.StatusBadRequest, "offset 必须为非负整数")
			return
		}
		query.Offset = value
	}
	switch query.Status {
	case "", "sending", "success", "error", "unknown":
	default:
		writeErrorText(w, http.StatusBadRequest, "无效的通知状态")
		return
	}
	result, err := s.store.ListDeliveries(r.Context(), query)
	if err != nil {
		writeErrorText(w, http.StatusInternalServerError, "无法读取通知记录")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) adminNotificationTest(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	s.sendNotification(w, r, 0)
}

func (s *Server) adminNotificationRetry(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	idText, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/api/admin/notifications/"), "/retry")
	id, err := strconv.ParseInt(idText, 10, 64)
	if !ok || err != nil || id <= 0 {
		writeErrorText(w, http.StatusBadRequest, "无效的通知记录 ID")
		return
	}
	s.sendNotification(w, r, id)
}

func (s *Server) sendNotification(w http.ResponseWriter, r *http.Request, id int64) {
	result, err := s.admin.SendNotification(r.Context(), id)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, result)
	case errors.Is(err, sql.ErrNoRows):
		writeErrorText(w, http.StatusNotFound, "通知记录不存在或已过期")
	case errors.Is(err, notify.ErrNotConfigured):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, notify.ErrNotRetryable), errors.Is(err, ErrNotificationBusy):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, ErrShuttingDown):
		writeError(w, http.StatusServiceUnavailable, err)
	default:
		writeErrorText(w, http.StatusInternalServerError, "通知处理失败，请检查记录及接收端后再重试")
	}
}
