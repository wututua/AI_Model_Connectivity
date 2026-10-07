package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type ScheduleStatus struct {
	Revision        string `json:"-"`
	ProviderID      string `json:"provider_id"`
	NextAt          string `json:"next_at"`
	IntervalMinutes int    `json:"interval_minutes"`
}

func (s *SQLiteStore) ScheduleDue(ctx context.Context, id, revision string, minutes int, now time.Time) (bool, error) {
	if minutes <= 0 {
		return false, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var oldRevision, next string
	var oldMinutes int
	err = tx.QueryRowContext(ctx, `SELECT revision,interval_minutes,next_at FROM provider_schedule WHERE provider=?`, id).Scan(&oldRevision, &oldMinutes, &next)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if errors.Is(err, sql.ErrNoRows) || oldRevision != revision || oldMinutes != minutes {
		_, err := tx.ExecContext(ctx, `INSERT INTO provider_schedule(provider,revision,interval_minutes,next_at) VALUES (?,?,?,?) ON CONFLICT(provider) DO UPDATE SET revision=excluded.revision,interval_minutes=excluded.interval_minutes,next_at=excluded.next_at`, id, revision, minutes, now.Add(time.Duration(minutes)*time.Minute).UTC().Format(time.RFC3339))
		if err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	due, err := time.Parse(time.RFC3339, next)
	if err != nil {
		return false, err
	}
	return !now.Before(due), tx.Commit()
}

func (s *SQLiteStore) CompleteSchedule(ctx context.Context, id, revision string, minutes int, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE provider_schedule SET next_at=? WHERE provider=? AND revision=? AND interval_minutes=?`, now.Add(time.Duration(minutes)*time.Minute).UTC().Format(time.RFC3339), id, revision, minutes)
	return err
}

func (s *SQLiteStore) ScheduleStatus(ctx context.Context) ([]ScheduleStatus, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT provider,next_at,interval_minutes,revision FROM provider_schedule ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScheduleStatus{}
	for rows.Next() {
		var v ScheduleStatus
		if err := rows.Scan(&v.ProviderID, &v.NextAt, &v.IntervalMinutes, &v.Revision); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
