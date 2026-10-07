package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
)

type CostItem struct {
	ProviderID    string  `json:"provider_id"`
	Model         string  `json:"model"`
	EstimatedUSD  float64 `json:"estimated_usd"`
	PricedProbes  int64   `json:"priced_probes"`
	UnknownProbes int64   `json:"unknown_probes"`
}

type CostSummary struct {
	Month          string     `json:"month"`
	EstimatedUSD   float64    `json:"estimated_usd"`
	BudgetUSD      float64    `json:"budget_usd"`
	BudgetExceeded bool       `json:"budget_exceeded"`
	UnknownProbes  int64      `json:"unknown_probes"`
	Items          []CostItem `json:"items"`
}

func appendCosts(ctx context.Context, tx *sql.Tx, results []probe.Result, at time.Time) error {
	var encoded string
	settings := config.MonitoringSettings{}
	err := tx.QueryRowContext(ctx, `SELECT value_json FROM runtime_config WHERE key='monitoring'`).Scan(&encoded)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal([]byte(encoded), &settings); err != nil {
			return err
		}
	}
	prices := map[[2]string]config.ModelPrice{}
	for _, price := range settings.Prices {
		prices[[2]string{price.ProviderID, price.Model}] = price
	}
	for _, result := range results {
		if result.Status == "unknown" {
			continue
		}
		price, exists := prices[[2]string{result.ProviderID, result.Model}]
		cost := float64(0)
		priced, unknown := 0, 1
		if exists && result.UsageKnown {
			cost = (float64(max(0, result.PromptTokens))*price.InputPerMillion + float64(max(0, result.CompletionTokens))*price.OutputPerMillion) / 1e6
			priced, unknown = 1, 0
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO cost_daily(day,provider,model,usd,priced,unknown) VALUES (?,?,?,?,?,?) ON CONFLICT(day,provider,model) DO UPDATE SET usd=usd+excluded.usd,priced=priced+excluded.priced,unknown=unknown+excluded.unknown`, resultTime(result, at).Format(time.DateOnly), result.ProviderID, result.Model, cost, priced, unknown); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM cost_daily WHERE day<?`, at.UTC().AddDate(0, 0, -364).Format(time.DateOnly))
	return err
}

func (s *SQLiteStore) CostSummary(ctx context.Context, settings config.MonitoringSettings) (CostSummary, error) {
	value := CostSummary{Month: time.Now().UTC().Format("2006-01"), BudgetUSD: settings.MonthlyBudget, Items: []CostItem{}}
	start, _ := time.Parse("2006-01", value.Month)
	rows, err := s.db.QueryContext(ctx, `SELECT provider,model,SUM(usd),SUM(priced),SUM(unknown) FROM cost_daily WHERE day>=? AND day<? GROUP BY provider,model ORDER BY SUM(usd) DESC`, value.Month+"-01", start.AddDate(0, 1, 0).Format(time.DateOnly))
	if err != nil {
		return value, err
	}
	defer rows.Close()
	for rows.Next() {
		var v CostItem
		if err := rows.Scan(&v.ProviderID, &v.Model, &v.EstimatedUSD, &v.PricedProbes, &v.UnknownProbes); err != nil {
			return value, err
		}
		value.EstimatedUSD += v.EstimatedUSD
		value.UnknownProbes += v.UnknownProbes
		value.Items = append(value.Items, v)
	}
	value.BudgetExceeded = value.BudgetUSD > 0 && value.EstimatedUSD >= value.BudgetUSD
	return value, rows.Err()
}
