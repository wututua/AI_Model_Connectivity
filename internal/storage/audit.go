package storage

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
)

const auditRetentionDays = 90
const auditMaxRecords = 10000

var auditActions = []string{
	"auth.login", "auth.logout", "auth.password",
	"users.create", "users.update", "users.delete",
	"settings.update", "config.import", "config.export",
	"providers.create", "providers.update", "providers.delete", "providers.batch", "providers.discover",
	"detection.start", "detection.selected", "detection.stop", "detection.provider",
	"monitoring.settings", "monitoring.backup", "monitoring.verify", "monitoring.approve", "monitoring.ack", "monitoring.test-rule",
	"notifications.test", "notifications.retry",
	"metrics.create", "metrics.rotate", "metrics.revoke",
	"updates.check", "updates.start", "updates.resolve", "data.export",
}

type AuditEvent struct {
	ID         int64  `json:"id"`
	ActorID    int64  `json:"actor_id"`
	Actor      string `json:"actor"`
	Role       string `json:"role"`
	Action     string `json:"action"`
	Result     string `json:"result"`
	HTTPStatus int    `json:"http_status"`
	CreatedAt  string `json:"created_at"`
}

type AuditQuery struct {
	Actor  string
	Action string
	Result string
	Start  time.Time
	End    time.Time
	Before int64
	Limit  int
}

func AuditActions() []string { return slices.Clone(auditActions) }

func (q AuditQuery) Validate() error {
	if len(q.Actor) > 128 || q.Limit < 0 || q.Limit > 100 || q.Before < 0 ||
		q.Action != "" && !slices.Contains(auditActions, q.Action) ||
		!slices.Contains([]string{"", "success", "accepted", "denied", "error"}, q.Result) {
		return errors.New("invalid audit filter")
	}
	if !q.Start.IsZero() && !q.End.IsZero() && !q.Start.Before(q.End) {
		return errors.New("end must be after start")
	}
	return nil
}

func auditActor(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	if len([]rune(value)) > 128 {
		value = string([]rune(value)[:128])
	}
	return value
}

func (s *SQLiteStore) RecordAudit(ctx context.Context, event AuditEvent) error {
	if !slices.Contains(auditActions, event.Action) || event.HTTPStatus < 200 || event.HTTPStatus > 599 || event.ActorID < 0 {
		return errors.New("invalid audit event")
	}
	if event.Role != "" && event.Role != "admin" && event.Role != "user" {
		return errors.New("invalid audit actor role")
	}
	event.Actor = auditActor(event.Actor)
	if event.ActorID == 0 {
		event.Actor, event.Role = "", ""
	}
	switch {
	case event.HTTPStatus == 202:
		event.Result = "accepted"
	case event.HTTPStatus < 400:
		event.Result = "success"
	case event.HTTPStatus == 401 || event.HTTPStatus == 403 || event.HTTPStatus == 429:
		event.Result = "denied"
	default:
		event.Result = "error"
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO admin_audit(actor_id,actor,role,action,result,http_status,created_at) VALUES (?,?,?,?,?,?,?)`,
		event.ActorID, event.Actor, event.Role, event.Action, event.Result, event.HTTPStatus, now.Format(time.RFC3339)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_audit WHERE created_at<? OR id NOT IN (SELECT id FROM admin_audit ORDER BY id DESC LIMIT ?)`,
		now.AddDate(0, 0, -auditRetentionDays).Format(time.RFC3339), auditMaxRecords); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) QueryAudit(ctx context.Context, q AuditQuery) (HistoryPage[AuditEvent], error) {
	if err := q.Validate(); err != nil {
		return HistoryPage[AuditEvent]{}, err
	}
	if q.Limit == 0 {
		q.Limit = 50
	}
	clauses := []string{"created_at>=?"}
	args := []any{time.Now().UTC().AddDate(0, 0, -auditRetentionDays).Format(time.RFC3339)}
	add := func(clause string, value any) { clauses = append(clauses, clause); args = append(args, value) }
	if q.Actor != "" {
		add("actor=?", q.Actor)
	}
	if q.Action != "" {
		add("action=?", q.Action)
	}
	if q.Result != "" {
		add("result=?", q.Result)
	}
	if q.Before > 0 {
		add("id<?", q.Before)
	}
	if !q.Start.IsZero() {
		add("julianday(created_at)>=julianday(?)", q.Start.UTC().Format(time.RFC3339Nano))
	}
	if !q.End.IsZero() {
		add("julianday(created_at)<julianday(?)", q.End.UTC().Format(time.RFC3339Nano))
	}
	args = append(args, q.Limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT id,actor_id,actor,role,action,result,http_status,created_at FROM admin_audit WHERE `+strings.Join(clauses, " AND ")+` ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		return HistoryPage[AuditEvent]{}, err
	}
	defer rows.Close()
	items := []AuditEvent{}
	for rows.Next() {
		var item AuditEvent
		if err := rows.Scan(&item.ID, &item.ActorID, &item.Actor, &item.Role, &item.Action, &item.Result, &item.HTTPStatus, &item.CreatedAt); err != nil {
			return HistoryPage[AuditEvent]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return HistoryPage[AuditEvent]{}, err
	}
	return historyPage(items, q.Limit, func(v AuditEvent) int64 { return v.ID }), nil
}
