package storage

import (
	"context"
	"time"
)

// Reserve before sending: interrupted deliveries require manual review, not replay.
func (s *SQLiteStore) ReserveNotice(ctx context.Context, key string, now time.Time) (bool, error) {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM monitoring_notices WHERE updated_at<?`, now.AddDate(0, 0, -90).UTC().Format(time.RFC3339)); err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO monitoring_notices(key,status,attempts,updated_at) VALUES (?,'sending',1,?)
		ON CONFLICT(key) DO UPDATE SET status='sending',attempts=attempts+1,updated_at=excluded.updated_at
		WHERE status='error' AND attempts<3 AND updated_at<?`, key, now.UTC().Format(time.RFC3339), now.Add(-15*time.Minute).UTC().Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (s *SQLiteStore) FinishNotice(ctx context.Context, key string, failed bool) error {
	status := "success"
	if failed {
		status = "error"
	}
	_, err := s.db.ExecContext(ctx, `UPDATE monitoring_notices SET status=?,updated_at=? WHERE key=? AND status='sending'`, status, time.Now().UTC().Format(time.RFC3339), key)
	return err
}
