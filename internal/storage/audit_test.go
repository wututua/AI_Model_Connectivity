package storage

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAuditIdentityOutcomesAndPagination(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	for _, status := range []int{200, 202, 400, 401, 403, 429, 500} {
		if err := s.RecordAudit(ctx, AuditEvent{ActorID: 7, Actor: "ad\r\nmin", Role: "admin", Action: "providers.update", HTTPStatus: status}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.QueryAudit(ctx, AuditQuery{Actor: "admin", Action: "providers.update", Result: "denied", Limit: 2})
	if err != nil || len(page.Items) != 2 || !page.HasMore || page.NextBefore != page.Items[1].ID {
		t.Fatal(page, err)
	}
	for _, event := range page.Items {
		if event.ActorID != 7 || event.Actor != "admin" || event.Role != "admin" || event.Result != "denied" {
			t.Fatal(event)
		}
	}
	next, err := s.QueryAudit(ctx, AuditQuery{Result: "denied", Before: page.NextBefore})
	if err != nil || next.HasMore || len(next.Items) != 1 {
		t.Fatal(next, err)
	}
	if err := s.RecordAudit(ctx, AuditEvent{Actor: "unverified-user", Role: "admin", Action: "auth.login", HTTPStatus: 401}); err != nil {
		t.Fatal(err)
	}
	anonymous, _ := s.QueryAudit(ctx, AuditQuery{Action: "auth.login"})
	if len(anonymous.Items) != 1 || anonymous.Items[0].Actor != "" || anonymous.Items[0].Role != "" {
		t.Fatal("unverified identity was persisted", anonymous)
	}
	accepted, _ := s.QueryAudit(ctx, AuditQuery{Result: "accepted"})
	if len(accepted.Items) != 1 || accepted.Items[0].HTTPStatus != 202 {
		t.Fatal("async acceptance marked completed", accepted)
	}
	for _, q := range []AuditQuery{{Actor: "admin' OR 1=1 --"}, {Start: time.Now().Add(time.Hour)}, {End: time.Now().Add(-time.Hour)}} {
		page, err := s.QueryAudit(ctx, q)
		if err != nil || page.Items == nil || len(page.Items) != 0 {
			t.Fatal(q, page, err)
		}
	}
	actions := AuditActions()
	actions[0] = "invalid"
	if AuditActions()[0] == "invalid" {
		t.Fatal("caller mutated audit allowlist")
	}
}

func TestAuditRetentionAndValidation(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<10005)
		INSERT INTO admin_audit(actor_id,actor,role,action,result,http_status,created_at)
		SELECT 1,'admin','admin','settings.update','success',200,? FROM n`, now)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -91).Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO admin_audit(actor_id,actor,role,action,result,http_status,created_at) VALUES (1,'old','admin','settings.update','success',200,?)`, old); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAudit(ctx, AuditEvent{Action: "auth.login", HTTPStatus: 401}); err != nil {
		t.Fatal(err)
	}
	var count, oldCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),SUM(actor='old') FROM admin_audit`).Scan(&count, &oldCount); err != nil || count > auditMaxRecords || oldCount != 0 {
		t.Fatal(count, oldCount, err)
	}
	for _, q := range []AuditQuery{{Limit: -1}, {Limit: 101}, {Before: -1}, {Actor: strings.Repeat("x", 129)}, {Action: "raw.secret"}, {Result: "unknown"}} {
		if _, err := s.QueryAudit(ctx, q); err == nil {
			t.Fatal("invalid query accepted", q)
		}
	}
	if err := s.RecordAudit(ctx, AuditEvent{Action: "secret-value", HTTPStatus: 200}); err == nil {
		t.Fatal("unknown action persisted")
	}
	if err := s.RecordAudit(ctx, AuditEvent{Action: "auth.login", HTTPStatus: 0}); err == nil {
		t.Fatal("invalid status persisted")
	}
}
