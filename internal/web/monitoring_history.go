package web

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"cg/internal/storage"
)

func monitoringHistoryQuery(r *http.Request, incidents bool) (storage.MonitoringHistoryQuery, error) {
	values := r.URL.Query()
	q := storage.MonitoringHistoryQuery{
		ProviderID: values.Get("provider_id"), Model: values.Get("model"), Status: values.Get("status"),
		Capability: values.Get("capability"), ErrorType: values.Get("error_type"), Scope: values.Get("scope"), Limit: 50,
	}
	var err error
	if values.Has("limit") {
		q.Limit, err = strconv.Atoi(values.Get("limit"))
		if err != nil || q.Limit < 1 || q.Limit > 100 {
			return q, errors.New("limit must be between 1 and 100")
		}
	}
	if values.Has("before") {
		q.Before, err = strconv.ParseInt(values.Get("before"), 10, 64)
		if err != nil || q.Before < 0 {
			return q, errors.New("invalid before cursor")
		}
	}
	for name, target := range map[string]*time.Time{"start": &q.Start, "end": &q.End} {
		if !values.Has(name) {
			continue
		}
		*target, err = time.Parse(time.RFC3339Nano, values.Get(name))
		if err != nil {
			return q, errors.New("start and end must be RFC3339 timestamps with time zones")
		}
	}
	return q, q.Validate(incidents)
}

func (s *Server) monitoringHistory(w http.ResponseWriter, r *http.Request, incidents bool) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	q, err := monitoringHistoryQuery(r, incidents)
	if err != nil {
		writeErrorText(w, http.StatusBadRequest, err.Error())
		return
	}
	var result any
	if incidents {
		result, err = s.store.QueryIncidents(r.Context(), q)
	} else {
		result, err = s.store.QueryDiagnostics(r.Context(), q)
	}
	if err != nil {
		writeErrorText(w, http.StatusInternalServerError, "无法读取历史记录")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
