package web

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) adminMetricsTokens(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		value, err := s.store.ListMetricsTokens(r.Context())
		if err != nil {
			writeErrorText(w, 500, "无法读取指标凭据")
			return
		}
		writeJSON(w, 200, value)
	case http.MethodPost:
		var input struct {
			Name string `json:"name"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}
		value, err := s.store.IssueMetricsToken(r.Context(), input.Name, 0)
		if err != nil {
			writeErrorText(w, 400, "创建失败，请检查名称和数量（最多 20 个）")
			return
		}
		writeJSON(w, 201, value)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) adminMetricsTokenItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/metrics-tokens/")
	idText, rotate := strings.CutSuffix(path, "/rotate")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		writeErrorText(w, 400, "无效的凭据 ID")
		return
	}
	var value any = map[string]bool{"ok": true}
	if rotate && r.Method == http.MethodPost {
		value, err = s.store.IssueMetricsToken(r.Context(), "", id)
	} else if !rotate && r.Method == http.MethodDelete {
		err = s.store.RevokeMetricsToken(r.Context(), id)
	} else {
		methodNotAllowed(w)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeErrorText(w, 404, "指标凭据不存在")
		return
	}
	if err != nil {
		writeErrorText(w, 500, "指标凭据操作失败")
		return
	}
	writeJSON(w, 200, value)
}
