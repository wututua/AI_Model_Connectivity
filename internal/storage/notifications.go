package storage

import (
	"context"
	"database/sql"
	"time"

	"cg/internal/notify"
)

func (s *SQLiteStore) initNotifications(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS notification_deliveries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		kind TEXT NOT NULL,
		retry_of INTEGER NOT NULL DEFAULT 0,
		platform TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL,
		finished_at TEXT NOT NULL DEFAULT '',
		elapsed_ms INTEGER NOT NULL DEFAULT 0,
		http_status INTEGER NOT NULL DEFAULT 0,
		summary TEXT NOT NULL,
		error_message TEXT NOT NULL DEFAULT ''
	)`)
	return err
}

func (s *SQLiteStore) RecoverInterruptedNotifications(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notification_deliveries
		SET status = 'unknown', finished_at = ?, error_message = '进程中断，接收结果未确认'
		WHERE status = 'sending'`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLiteStore) CreateDelivery(ctx context.Context, value notify.Delivery) (notify.Delivery, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return notify.Delivery{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO notification_deliveries
		(kind, retry_of, platform, status, created_at, summary) VALUES (?, ?, ?, 'sending', ?, ?)`,
		value.Kind, value.RetryOf, value.Platform, value.CreatedAt, value.Summary)
	if err != nil {
		return notify.Delivery{}, err
	}
	value.ID, err = result.LastInsertId()
	if err != nil {
		return notify.Delivery{}, err
	}
	// Retain at most 1,000 recent records plus any requests still in flight.
	_, err = tx.ExecContext(ctx, `DELETE FROM notification_deliveries WHERE status != 'sending' AND
		(created_at < ? OR id NOT IN (SELECT id FROM notification_deliveries ORDER BY id DESC LIMIT 1000))`,
		time.Now().UTC().AddDate(0, 0, -90).Format(time.RFC3339Nano))
	if err != nil {
		return notify.Delivery{}, err
	}
	if err := tx.Commit(); err != nil {
		return notify.Delivery{}, err
	}
	value.Status = "sending"
	return value, nil
}

func (s *SQLiteStore) FinishDelivery(ctx context.Context, value notify.Delivery) error {
	result, err := s.db.ExecContext(ctx, `UPDATE notification_deliveries SET
		status = ?, finished_at = ?, elapsed_ms = ?, http_status = ?, error_message = ?
		WHERE id = ? AND status = 'sending'`,
		value.Status, value.FinishedAt, value.ElapsedMS, value.HTTPStatus, value.ErrorMessage, value.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return err
}

const deliveryColumns = `id, kind, retry_of, platform, status, created_at, finished_at, elapsed_ms, http_status, summary, error_message`

func (s *SQLiteStore) ListDeliveries(ctx context.Context, query notify.DeliveryQuery) ([]notify.Delivery, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	query.Limit = min(query.Limit, 200)
	query.Offset = max(query.Offset, 0)
	rows, err := s.db.QueryContext(ctx, `SELECT `+deliveryColumns+` FROM notification_deliveries
		WHERE (? = '' OR status = ?) ORDER BY id DESC LIMIT ? OFFSET ?`,
		query.Status, query.Status, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []notify.Delivery{}
	for rows.Next() {
		value, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) GetDelivery(ctx context.Context, id int64) (notify.Delivery, error) {
	return scanDelivery(s.db.QueryRowContext(ctx, `SELECT `+deliveryColumns+` FROM notification_deliveries WHERE id = ?`, id))
}

func scanDelivery(row interface{ Scan(...any) error }) (notify.Delivery, error) {
	var value notify.Delivery
	err := row.Scan(&value.ID, &value.Kind, &value.RetryOf, &value.Platform, &value.Status,
		&value.CreatedAt, &value.FinishedAt, &value.ElapsedMS, &value.HTTPStatus, &value.Summary, &value.ErrorMessage)
	return value, err
}
