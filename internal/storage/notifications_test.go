package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"cg/internal/notify"
)

func TestDeliveryPersistenceAndRecovery(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	for _, status := range []string{"success", "error", "sending"} {
		value, err := store.CreateDelivery(ctx, notify.Delivery{Kind: "test", Platform: "webhook", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Summary: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if status != "sending" {
			value.Status, value.FinishedAt, value.HTTPStatus = status, "finished", 200
			if err := store.FinishDelivery(ctx, value); err != nil {
				t.Fatal(err)
			}
			if err := store.FinishDelivery(ctx, value); !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("terminal record was modified")
			}
		}
	}
	if err := store.RecoverInterruptedNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	values, err := store.ListDeliveries(ctx, notify.DeliveryQuery{Limit: 2})
	if err != nil || len(values) != 2 || values[0].Status != "unknown" || values[1].Status != "error" {
		t.Fatalf("invalid history: %+v, %v", values, err)
	}
	if values[0].FinishedAt == "" || values[0].ErrorMessage == "" {
		t.Fatal("interruption not explained")
	}
	values, err = store.ListDeliveries(ctx, notify.DeliveryQuery{Offset: 2})
	if err != nil || len(values) != 1 || values[0].Status != "success" || values[0].FinishedAt != "finished" {
		t.Fatal("recovery changed a completed record")
	}
	values, err = store.ListDeliveries(ctx, notify.DeliveryQuery{Status: "unknown"})
	if err != nil || len(values) != 1 {
		t.Fatal("status filter failed")
	}
	if _, err := store.GetDelivery(ctx, 999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("missing record not reported")
	}
	if err := store.initNotifications(ctx); err != nil {
		t.Fatal("migration is not idempotent", err)
	}
}

func TestDeliveryRetention(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := 0; i < 1005; i++ {
		status, created := "success", now
		if i == 0 {
			status = "sending"
		}
		if i == 1004 {
			created = time.Now().UTC().AddDate(0, 0, -91).Format(time.RFC3339Nano)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO notification_deliveries (kind, platform, status, created_at, summary) VALUES ('test', 'webhook', ?, ?, ?)`, status, created, fmt.Sprint(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateDelivery(ctx, notify.Delivery{Kind: "test", Platform: "webhook", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notification_deliveries`).Scan(&count); err != nil || count != 1000 {
		t.Fatalf("unexpected retention: %d %v", count, err)
	}
	if _, err := store.GetDelivery(ctx, 1); err != nil {
		t.Fatal("in-flight record was pruned")
	}
	for _, id := range []int64{2, 1005} {
		if _, err := store.GetDelivery(ctx, id); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("expired record %d retained", id)
		}
	}
}
