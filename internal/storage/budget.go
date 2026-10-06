package storage

import (
	"context"
	"errors"
	"time"
)

var ErrBudgetExceeded = errors.New("今日上游请求预算已用尽，UTC 次日重置")

type RequestBudget struct {
	Day       string `json:"day"`
	Used      int    `json:"used"`
	Limit     int    `json:"limit"`
	Remaining int    `json:"remaining"`
	Exhausted bool   `json:"exhausted"`
	ResetsAt  string `json:"resets_at"`
}

func (s *SQLiteStore) initBudget(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS request_budget (
		day TEXT PRIMARY KEY, used INTEGER NOT NULL DEFAULT 0
	)`)
	return err
}

func (s *SQLiteStore) RequestBudget(ctx context.Context, limit int) (RequestBudget, error) {
	now := time.Now().UTC()
	value := RequestBudget{Day: now.Format(time.DateOnly), Limit: limit,
		ResetsAt: now.Truncate(24 * time.Hour).Add(24 * time.Hour).Format(time.RFC3339)}
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT used FROM request_budget WHERE day = ?), 0)`, value.Day).Scan(&value.Used)
	value.Remaining = max(0, limit-value.Used)
	value.Exhausted = limit > 0 && value.Used >= limit
	return value, err
}

// ReserveRequest counts attempts before network I/O, even when the attempt fails.
func (s *SQLiteStore) ReserveRequest(ctx context.Context, limit int) error {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO request_budget(day, used) VALUES (?, 0)`, now.Format(time.DateOnly)); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE request_budget SET used = used + 1 WHERE day = ? AND (? <= 0 OR used < ?)`,
		now.Format(time.DateOnly), limit, limit)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrBudgetExceeded
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM request_budget WHERE day < ?`, now.AddDate(0, 0, -90).Format(time.DateOnly)); err != nil {
		return err
	}
	return tx.Commit()
}
