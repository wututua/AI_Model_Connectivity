package storage

import (
	"context"
	"errors"
	"strconv"
	"time"
)

var ErrExportTooLarge = errors.New("导出超过 10000 条，请缩小日期或模型范围")

type ExportQuery struct {
	Kind       string
	ProviderID string
	Model      string
	Start      time.Time
	End        time.Time
}

func (s *SQLiteStore) ExportRows(ctx context.Context, query ExportQuery) ([][]string, error) {
	if query.Kind == "usage" {
		return s.exportUsage(ctx, query)
	}
	if query.Kind != "history" {
		return nil, errors.New("unsupported export kind")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT provider, model, result, checked_at, latency_ms, first_token_ms,
		prompt_tokens, completion_tokens, total_tokens FROM probe_results
		WHERE julianday(checked_at) >= julianday(?) AND julianday(checked_at) < julianday(?)
		AND (? = '' OR provider = ?) AND (? = '' OR model = ?)
		ORDER BY julianday(checked_at), id LIMIT 10001`,
		query.Start.Format(time.RFC3339), query.End.Format(time.RFC3339),
		query.ProviderID, query.ProviderID, query.Model, query.Model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := [][]string{{"provider_id", "model", "status", "checked_at", "latency_ms", "first_token_ms", "prompt_tokens", "completion_tokens", "total_tokens"}}
	for rows.Next() {
		var provider, model, status, checked string
		var latency, first, input, output, total int64
		if err := rows.Scan(&provider, &model, &status, &checked, &latency, &first, &input, &output, &total); err != nil {
			return nil, err
		}
		result = append(result, []string{provider, model, status, checked, number(latency), number(first), number(input), number(output), number(total)})
		if len(result) > 10001 {
			return nil, ErrExportTooLarge
		}
	}
	return result, rows.Err()
}

func (s *SQLiteStore) exportUsage(ctx context.Context, query ExportQuery) ([][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT day, provider, model, prompt_tokens, completion_tokens, total_tokens, probe_count
		FROM usage_daily WHERE day >= ? AND day < ? AND (? = '' OR provider = ?) AND (? = '' OR model = ?)
		ORDER BY day, provider, model LIMIT 10001`,
		query.Start.Format(time.DateOnly), query.End.Format(time.DateOnly),
		query.ProviderID, query.ProviderID, query.Model, query.Model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := [][]string{{"day_utc", "provider_id", "model", "prompt_tokens", "completion_tokens", "total_tokens", "probe_count"}}
	for rows.Next() {
		var day, provider, model string
		var input, output, total, count int64
		if err := rows.Scan(&day, &provider, &model, &input, &output, &total, &count); err != nil {
			return nil, err
		}
		result = append(result, []string{day, provider, model, number(input), number(output), number(total), number(count)})
		if len(result) > 10001 {
			return nil, ErrExportTooLarge
		}
	}
	return result, rows.Err()
}

func number(value int64) string { return strconv.FormatInt(value, 10) }
