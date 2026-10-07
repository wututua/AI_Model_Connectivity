package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"cg/internal/config"
	"cg/internal/notify"
	"cg/internal/probe"
)

type Incident struct {
	ID             int64  `json:"id"`
	ProviderID     string `json:"provider_id"`
	Model          string `json:"model"`
	Revision       string `json:"revision"`
	Status         string `json:"status"`
	OpenedAt       string `json:"opened_at"`
	LastSeenAt     string `json:"last_seen_at"`
	ResolvedAt     string `json:"resolved_at"`
	AcknowledgedAt string `json:"acknowledged_at"`
	Note           string `json:"note"`
}

func (s *SQLiteStore) ObserveIncidents(ctx context.Context, cfg config.Config, results []probe.Result) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	revisions := map[string]string{}
	for _, p := range cfg.Providers {
		revisions[p.ID] = p.ConnectionRevision
	}
	for _, result := range results {
		if !result.Completed || result.Status == "unknown" {
			continue
		}
		revision, exists := revisions[result.ProviderID]
		if !exists {
			continue
		}
		now := resultTime(result, time.Now()).UTC().Format(time.RFC3339)
		if _, err := tx.ExecContext(ctx, `UPDATE incidents SET status='superseded', resolved_at=? WHERE provider=? AND model=? AND revision<>? AND status='open'`, now, result.ProviderID, result.Model, revision); err != nil {
			return err
		}
		if result.Status == "ok" {
			if _, err := tx.ExecContext(ctx, `UPDATE incidents SET status='resolved', resolved_at=?, last_seen_at=? WHERE provider=? AND model=? AND revision=? AND status='open'`, now, now, result.ProviderID, result.Model, revision); err != nil {
				return err
			}
		} else if result.Status == "error" || result.Status == "slow" {
			var id int64
			err := tx.QueryRowContext(ctx, `SELECT id FROM incidents WHERE provider=? AND model=? AND revision=? AND status='open'`, result.ProviderID, result.Model, revision).Scan(&id)
			if errors.Is(err, sql.ErrNoRows) {
				_, err = tx.ExecContext(ctx, `INSERT INTO incidents(provider,model,revision,status,opened_at,last_seen_at) VALUES (?,?,?,'open',?,?)`, result.ProviderID, result.Model, revision, now, now)
			} else if err == nil {
				_, err = tx.ExecContext(ctx, `UPDATE incidents SET last_seen_at=? WHERE id=?`, now, id)
			}
			if err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM incidents WHERE status<>'open' AND id NOT IN (SELECT id FROM incidents ORDER BY id DESC LIMIT 2000)`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) Incidents(ctx context.Context) ([]Incident, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,provider,model,revision,status,opened_at,last_seen_at,resolved_at,acknowledged_at,note FROM incidents ORDER BY (status='open') DESC,id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return readIncidents(rows)
}

func readIncidents(rows *sql.Rows) ([]Incident, error) {
	out := []Incident{}
	for rows.Next() {
		var v Incident
		if err := rows.Scan(&v.ID, &v.ProviderID, &v.Model, &v.Revision, &v.Status, &v.OpenedAt, &v.LastSeenAt, &v.ResolvedAt, &v.AcknowledgedAt, &v.Note); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) AcknowledgeIncident(ctx context.Context, id int64, note string) error {
	if len([]rune(note)) > 2000 {
		return errors.New("note exceeds 2000 characters")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE incidents SET acknowledged_at=?,note=? WHERE id=? AND status='open'`, time.Now().UTC().Format(time.RFC3339), note, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return errors.New("incident is missing or no longer open")
	}
	return err
}

type RuleStateStore struct {
	Store *SQLiteStore
	Key   string
}

func (s RuleStateStore) Read() (notify.State, error) {
	var data string
	var value notify.State
	err := s.Store.db.QueryRow(`SELECT value_json FROM runtime_config WHERE key=?`, "rule-state:"+s.Key).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	err = json.Unmarshal([]byte(data), &value)
	return value, err
}

func (s RuleStateStore) Write(value notify.State) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.Store.db.Exec(`INSERT INTO runtime_config(key,value_json,updated_at) VALUES (?,?,?) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`, "rule-state:"+s.Key, string(data), time.Now().UTC().Format(time.RFC3339))
	return err
}

type MonitorEvent struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

func (s *SQLiteStore) MonitorEvents(ctx context.Context) ([]MonitorEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,detail,created_at FROM monitoring_events ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MonitorEvent{}
	for rows.Next() {
		var v MonitorEvent
		if err := rows.Scan(&v.ID, &v.Kind, &v.Detail, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) AddMonitorEvent(ctx context.Context, kind, detail string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO monitoring_events(kind,detail,created_at) VALUES (?,?,?)`, kind, detail, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM monitoring_events WHERE id NOT IN (SELECT id FROM monitoring_events ORDER BY id DESC LIMIT 1000)`)
	return err
}
