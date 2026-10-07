package storage

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
)

type MonitoringHistoryQuery struct {
	ProviderID string
	Model      string
	Status     string
	Capability string
	ErrorType  string
	Scope      string
	Start      time.Time
	End        time.Time
	Before     int64
	Limit      int
}

type HistoryPage[T any] struct {
	Items      []T   `json:"items"`
	HasMore    bool  `json:"has_more"`
	NextBefore int64 `json:"next_before"`
}

func (q MonitoringHistoryQuery) Validate(incidents bool) error {
	if q.Before < 0 || q.Limit < 0 || q.Limit > 100 {
		return errors.New("limit must be between 1 and 100; before must be nonnegative")
	}
	if len(q.ProviderID) > 256 || len(q.Model) > 1024 || len(q.ErrorType) > 64 {
		return errors.New("history filter is too long")
	}
	if !q.Start.IsZero() && !q.End.IsZero() && !q.Start.Before(q.End) {
		return errors.New("end must be after start")
	}
	statuses := []string{"", "ok", "slow", "error", "unknown"}
	if incidents {
		statuses = []string{"", "open", "resolved", "superseded"}
		if q.Capability != "" || q.ErrorType != "" {
			return errors.New("capability and error_type apply only to diagnostics")
		}
	} else if q.Scope != "" {
		return errors.New("scope applies only to incidents")
	}
	if !slices.Contains(statuses, q.Status) || !slices.Contains([]string{"", "text", "tools", "embedding"}, q.Capability) ||
		!slices.Contains([]string{"", "models", "discovery"}, q.Scope) || q.Scope == "discovery" && q.Model != "" {
		return errors.New("invalid history filter")
	}
	return nil
}

// Column names come only from callers below; filter values always use parameters.
func historyWhere(q MonitoringHistoryQuery, statusColumn, timeColumn string) (string, []any) {
	clauses := []string{"1=1"}
	args := []any{}
	add := func(clause string, value any) { clauses = append(clauses, clause); args = append(args, value) }
	if q.ProviderID != "" {
		add("provider=?", q.ProviderID)
	}
	if q.Model != "" {
		add("model=?", q.Model)
	}
	if q.Status != "" {
		add(statusColumn+"=?", q.Status)
	}
	if q.Capability != "" {
		add("COALESCE(NULLIF(capability,''),'text')=?", q.Capability)
	}
	if q.ErrorType != "" {
		add("error_type=?", q.ErrorType)
	}
	if q.Scope == "discovery" {
		clauses = append(clauses, "model=''")
	} else if q.Scope == "models" {
		clauses = append(clauses, "model<>''")
	}
	if q.Before > 0 {
		add("id<?", q.Before)
	}
	if !q.Start.IsZero() {
		add("julianday("+timeColumn+")>=julianday(?)", q.Start.UTC().Format(time.RFC3339Nano))
	}
	if !q.End.IsZero() {
		add("julianday("+timeColumn+")<julianday(?)", q.End.UTC().Format(time.RFC3339Nano))
	}
	return strings.Join(clauses, " AND "), args
}

func historyPage[T any](items []T, limit int, id func(T) int64) HistoryPage[T] {
	page := HistoryPage[T]{Items: items, HasMore: len(items) > limit}
	if page.HasMore {
		page.Items = items[:limit]
		page.NextBefore = id(page.Items[len(page.Items)-1])
	}
	return page
}

func (s *SQLiteStore) QueryDiagnostics(ctx context.Context, q MonitoringHistoryQuery) (HistoryPage[DiagnosticRecord], error) {
	if err := q.Validate(false); err != nil {
		return HistoryPage[DiagnosticRecord]{}, err
	}
	if q.Limit == 0 {
		q.Limit = 50
	}
	where, args := historyWhere(q, "result", "checked_at")
	args = append(args, q.Limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT id,provider,model,result,error_type,checked_at,latency_ms,first_token_ms,diagnostics_json,capability,capability_status FROM probe_results WHERE `+where+` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		return HistoryPage[DiagnosticRecord]{}, err
	}
	defer rows.Close()
	items, err := readDiagnostics(rows)
	if err != nil {
		return HistoryPage[DiagnosticRecord]{}, err
	}
	return historyPage(items, q.Limit, func(v DiagnosticRecord) int64 { return v.ID }), nil
}

func (s *SQLiteStore) QueryIncidents(ctx context.Context, q MonitoringHistoryQuery) (HistoryPage[Incident], error) {
	if err := q.Validate(true); err != nil {
		return HistoryPage[Incident]{}, err
	}
	if q.Limit == 0 {
		q.Limit = 50
	}
	where, args := historyWhere(q, "status", "opened_at")
	args = append(args, q.Limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT id,provider,model,revision,status,opened_at,last_seen_at,resolved_at,acknowledged_at,note FROM incidents WHERE `+where+` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		return HistoryPage[Incident]{}, err
	}
	defer rows.Close()
	items, err := readIncidents(rows)
	if err != nil {
		return HistoryPage[Incident]{}, err
	}
	return historyPage(items, q.Limit, func(v Incident) int64 { return v.ID }), nil
}
