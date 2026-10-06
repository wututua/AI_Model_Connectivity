package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"cg/internal/update"
)

type UpdateService interface {
	Status() (update.Status, error)
	Resolve(string) (update.Status, error)
	Check(context.Context, string) (update.Check, error)
	Start(context.Context, string, string, string) (update.Job, error)
}

func (s *Server) SetUpdater(updater UpdateService) { s.updater = updater }

func (s *Server) updateAccess(w http.ResponseWriter, r *http.Request, method string) bool {
	if !s.requireAdmin(w, r) {
		return false
	}
	if r.Method != method {
		methodNotAllowed(w)
		return false
	}
	if s.updater == nil {
		writeErrorText(w, 503, "更新服务不可用")
		return false
	}
	return true
}

func (s *Server) adminUpdates(w http.ResponseWriter, r *http.Request) {
	if !s.updateAccess(w, r, http.MethodGet) {
		return
	}
	result, err := s.updater.Status()
	s.writeUpdateResult(w, 200, result, err)
}

func (s *Server) adminUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if !s.updateAccess(w, r, http.MethodPost) {
		return
	}
	var input struct {
		Channel string `json:"channel"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := s.updater.Check(r.Context(), input.Channel)
	s.writeUpdateResult(w, 200, result, err)
}

func (s *Server) adminUpdateResolve(w http.ResponseWriter, r *http.Request) {
	if !s.updateAccess(w, r, http.MethodPost) {
		return
	}
	var input struct {
		RequestID string `json:"request_id"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.RequestID == "" {
		writeErrorText(w, 400, "缺少更新提交凭据")
		return
	}
	result, err := s.updater.Resolve(input.RequestID)
	s.writeUpdateResult(w, 200, result, err)
}

func (s *Server) adminUpdateStart(w http.ResponseWriter, r *http.Request) {
	if !s.updateAccess(w, r, http.MethodPost) {
		return
	}
	var input struct {
		Channel   string `json:"channel"`
		Version   string `json:"version"`
		Confirm   bool   `json:"confirm"`
		RequestID string `json:"request_id"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if !input.Confirm {
		writeErrorText(w, 400, "请确认更新期间的停服与备份操作")
		return
	}
	if input.RequestID == "" {
		writeErrorText(w, 400, "缺少更新提交凭据，请刷新状态")
		return
	}
	if s.admin != nil && s.admin.RunningState().Running {
		writeErrorText(w, 409, "检测正在运行，请等待完成或停止检测后更新")
		return
	}
	result, err := s.updater.Start(r.Context(), input.Channel, input.Version, input.RequestID)
	s.writeUpdateResult(w, 202, result, err)
}

func (s *Server) writeUpdateResult(w http.ResponseWriter, status int, result any, err error) {
	if err == nil {
		writeJSON(w, status, result)
		return
	}
	switch {
	case errors.Is(err, update.ErrChannel), errors.Is(err, update.ErrVersion):
		writeErrorText(w, 400, err.Error())
	case errors.Is(err, update.ErrBusy), errors.Is(err, update.ErrUnsupported), errors.Is(err, update.ErrRequest):
		writeErrorText(w, 409, err.Error())
	default:
		slog.Warn("system update request failed", "err", err)
		writeErrorText(w, 503, "暂时无法检查或提交更新，请检查网络、更新服务及服务器日志")
	}
}
