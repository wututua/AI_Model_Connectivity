package storage

import (
	"context"
	"database/sql"
	"time"

	"cg/internal/probe"
)

func (s *SQLiteStore) initUsage(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS usage_daily (
		day TEXT NOT NULL, provider TEXT NOT NULL, model TEXT NOT NULL,
		provider_name TEXT NOT NULL, provider_type TEXT NOT NULL,
		prompt_tokens INTEGER NOT NULL, completion_tokens INTEGER NOT NULL,
		total_tokens INTEGER NOT NULL, probe_count INTEGER NOT NULL,
		PRIMARY KEY(day, provider, model)
	)`); err != nil {
		return err
	}
	var migrated int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_config WHERE key = 'usage_daily_migrated'`).Scan(&migrated); err != nil {
		return err
	}
	if migrated == 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO usage_daily
			SELECT date(checked_at), provider, model, MAX(provider_name), MAX(provider_type),
			SUM(prompt_tokens), SUM(completion_tokens), SUM(total_tokens), COUNT(*)
			FROM probe_results WHERE date(checked_at) IS NOT NULL
			GROUP BY date(checked_at), provider, model`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_config(key, value_json, updated_at) VALUES ('usage_daily_migrated', 'true', ?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func appendUsage(ctx context.Context, tx *sql.Tx, results []probe.Result, checkedAt time.Time) error {
	statement, err := tx.PrepareContext(ctx, `INSERT INTO usage_daily
		(day, provider, model, provider_name, provider_type, prompt_tokens, completion_tokens, total_tokens, probe_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)
		ON CONFLICT(day, provider, model) DO UPDATE SET
		provider_name = excluded.provider_name, provider_type = excluded.provider_type,
		prompt_tokens = usage_daily.prompt_tokens + excluded.prompt_tokens,
		completion_tokens = usage_daily.completion_tokens + excluded.completion_tokens,
		total_tokens = usage_daily.total_tokens + excluded.total_tokens,
		probe_count = usage_daily.probe_count + 1`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, result := range results {
		if _, err := statement.ExecContext(ctx, checkedAt.UTC().Format(time.DateOnly), result.ProviderID, result.Model,
			result.ProviderName, result.ProviderType, result.PromptTokens, result.CompletionTokens, result.TotalTokens); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM usage_daily WHERE day < ?`, checkedAt.UTC().AddDate(0, 0, -364).Format(time.DateOnly))
	return err
}
