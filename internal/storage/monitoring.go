package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"cg/internal/config"
	"cg/internal/httpclient"
)

var ErrMonitoringConflict = errors.New("monitoring settings changed; refresh and try again")

func (s *SQLiteStore) MonitoringSettings(ctx context.Context) (config.MonitoringSettings, error) {
	value := config.MonitoringSettings{BackupKeep: 7, Rules: []config.AlertRule{}, Schedules: []config.ProviderSchedule{}, Prices: []config.ModelPrice{}}
	var encoded string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM runtime_config WHERE key='monitoring'`).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	err = json.Unmarshal([]byte(encoded), &value)
	normalizeMonitoring(&value)
	return value, err
}

func normalizeMonitoring(value *config.MonitoringSettings) {
	if value.Rules == nil {
		value.Rules = []config.AlertRule{}
	}
	if value.Schedules == nil {
		value.Schedules = []config.ProviderSchedule{}
	}
	if value.Prices == nil {
		value.Prices = []config.ModelPrice{}
	}
}

func (s *SQLiteStore) SaveMonitoringSettings(ctx context.Context, value config.MonitoringSettings) (config.MonitoringSettings, error) {
	s.monitorMu.Lock()
	defer s.monitorMu.Unlock()
	old, err := s.MonitoringSettings(ctx)
	if err != nil {
		return value, err
	}
	if old.Version != value.Version {
		return value, ErrMonitoringConflict
	}
	normalizeMonitoring(&value)
	for i := range value.Rules {
		generation := ""
		for _, previous := range old.Rules {
			r := &value.Rules[i]
			if r.ID != previous.ID || r.Platform != previous.Platform {
				continue
			}
			if r.URL == "" {
				r.URL = previous.URL
			}
			if r.Token == "" {
				r.Token = previous.Token
			}
			if r.ChatID == "" {
				r.ChatID = previous.ChatID
			}
			oldGeneration := previous.Generation
			candidate := *r
			candidate.Generation, previous.Generation = "", ""
			candidate.CredentialsSet, previous.CredentialsSet = false, false
			if candidate == previous {
				generation = oldGeneration
			}
		}
		if generation == "" {
			generation = rand.Text()
		}
		value.Rules[i].Generation = generation
		value.Rules[i].CredentialsSet = false
	}
	if err := value.Validate(); err != nil {
		return value, err
	}
	value.Version++
	encoded, err := json.Marshal(value)
	if err != nil {
		return value, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return value, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_config(key,value_json,updated_at) VALUES ('monitoring',?,?) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json, updated_at=excluded.updated_at`, string(encoded), time.Now().UTC().Format(time.RFC3339)); err != nil {
		return value, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO monitoring_events(kind,detail,created_at) VALUES ('settings','monitoring settings updated',?)`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return value, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM monitoring_events WHERE id NOT IN (SELECT id FROM monitoring_events ORDER BY id DESC LIMIT 1000)`); err != nil {
		return value, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM runtime_config WHERE key LIKE 'rule-state:%' AND updated_at<?`, time.Now().UTC().AddDate(0, 0, -90).Format(time.RFC3339)); err != nil {
		return value, err
	}
	if err := tx.Commit(); err != nil {
		return value, err
	}
	return value.Safe(), nil
}

type DiagnosticRecord struct {
	Capability       string                  `json:"capability"`
	CapabilityStatus string                  `json:"capability_status"`
	ID               int64                   `json:"id"`
	ProviderID       string                  `json:"provider_id"`
	Model            string                  `json:"model"`
	Status           string                  `json:"status"`
	ErrorType        string                  `json:"error_type"`
	CheckedAt        string                  `json:"checked_at"`
	LatencyMS        int                     `json:"latency_ms"`
	FirstTokenMS     int                     `json:"first_token_ms"`
	Diagnostics      *httpclient.Diagnostics `json:"diagnostics,omitempty"`
}

func (s *SQLiteStore) DiagnosticRecords(ctx context.Context) ([]DiagnosticRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,provider,model,result,error_type,checked_at,latency_ms,first_token_ms,diagnostics_json,capability,capability_status FROM probe_results ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return readDiagnostics(rows)
}

func readDiagnostics(rows *sql.Rows) ([]DiagnosticRecord, error) {
	out := []DiagnosticRecord{}
	for rows.Next() {
		var v DiagnosticRecord
		var encoded string
		if err := rows.Scan(&v.ID, &v.ProviderID, &v.Model, &v.Status, &v.ErrorType, &v.CheckedAt, &v.LatencyMS, &v.FirstTokenMS, &encoded, &v.Capability, &v.CapabilityStatus); err != nil {
			return nil, err
		}
		if encoded != "{}" {
			if err := json.Unmarshal([]byte(encoded), &v.Diagnostics); err != nil {
				return nil, err
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
