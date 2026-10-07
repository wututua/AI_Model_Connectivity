package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"cg/internal/config"
)

type Catalog struct {
	ProviderID string   `json:"provider_id"`
	Revision   string   `json:"revision"`
	Models     []string `json:"models"`
	Approved   []string `json:"approved"`
	Added      []string `json:"added"`
	Removed    []string `json:"removed"`
	UpdatedAt  string   `json:"updated_at"`
}

type CatalogEvent struct {
	Revision   string   `json:"revision"`
	ID         int64    `json:"id"`
	ProviderID string   `json:"provider_id"`
	Added      []string `json:"added"`
	Removed    []string `json:"removed"`
	CreatedAt  string   `json:"created_at"`
}

func (s *SQLiteStore) initMonitoring(ctx context.Context) error {
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS model_catalog (provider TEXT PRIMARY KEY, value_json TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS catalog_events (id INTEGER PRIMARY KEY, provider TEXT NOT NULL, value_json TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS monitoring_events (id INTEGER PRIMARY KEY, kind TEXT NOT NULL, detail TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS backups (name TEXT PRIMARY KEY, created_at TEXT NOT NULL, size INTEGER NOT NULL, sha256 TEXT NOT NULL, verified_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS incidents (id INTEGER PRIMARY KEY,provider TEXT NOT NULL,model TEXT NOT NULL,revision TEXT NOT NULL,status TEXT NOT NULL,opened_at TEXT NOT NULL,last_seen_at TEXT NOT NULL,resolved_at TEXT NOT NULL DEFAULT '',acknowledged_at TEXT NOT NULL DEFAULT '',note TEXT NOT NULL DEFAULT '')`,
		`CREATE UNIQUE INDEX IF NOT EXISTS incidents_open_identity ON incidents(provider,model,revision) WHERE status='open'`,
		`CREATE INDEX IF NOT EXISTS incidents_provider_id ON incidents(provider,model,id DESC)`,
		`CREATE INDEX IF NOT EXISTS incidents_status_id ON incidents(status,id DESC)`,
		`CREATE INDEX IF NOT EXISTS probe_results_provider_id ON probe_results(provider,model,id DESC)`,
		`CREATE INDEX IF NOT EXISTS probe_results_result_id ON probe_results(result,id DESC)`,
		`CREATE TABLE IF NOT EXISTS provider_schedule (provider TEXT PRIMARY KEY, revision TEXT NOT NULL, interval_minutes INTEGER NOT NULL, next_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS cost_daily (day TEXT NOT NULL,provider TEXT NOT NULL,model TEXT NOT NULL,usd REAL NOT NULL,priced INTEGER NOT NULL,unknown INTEGER NOT NULL,PRIMARY KEY(day,provider,model))`,
		`CREATE TABLE IF NOT EXISTS monitoring_notices (key TEXT PRIMARY KEY,status TEXT NOT NULL,attempts INTEGER NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS admin_audit (id INTEGER PRIMARY KEY AUTOINCREMENT,actor_id INTEGER NOT NULL,actor TEXT NOT NULL,role TEXT NOT NULL,action TEXT NOT NULL,result TEXT NOT NULL,http_status INTEGER NOT NULL,created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS admin_audit_actor_id ON admin_audit(actor,id DESC)`,
		`CREATE INDEX IF NOT EXISTS admin_audit_action_id ON admin_audit(action,id DESC)`,
		`CREATE INDEX IF NOT EXISTS admin_audit_created ON admin_audit(created_at)`,
	} {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

// Only successful, complete model discoveries reach this transaction.
func (s *SQLiteStore) ObserveCatalog(ctx context.Context, cfg config.ProviderConfig, models []string) ([]string, error) {
	discoveryOrder := slices.Clone(models)
	models = slices.Clone(models)
	slices.Sort(models)
	models = slices.Compact(models)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var encoded string
	value := Catalog{ProviderID: cfg.ID, Revision: cfg.ConnectionRevision, Models: models, Approved: models, Added: []string{}, Removed: []string{}}
	err = tx.QueryRowContext(ctx, `SELECT value_json FROM model_catalog WHERE provider = ?`, cfg.ID).Scan(&encoded)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		var old Catalog
		if err := json.Unmarshal([]byte(encoded), &old); err != nil {
			return nil, err
		}
		if old.Revision == cfg.ConnectionRevision {
			value.Approved = old.Approved
			value.Added, value.Removed = difference(models, old.Models), difference(old.Models, models)
			if len(value.Added)+len(value.Removed) > 0 {
				event, _ := json.Marshal(CatalogEvent{ProviderID: cfg.ID, Revision: cfg.ConnectionRevision, Added: value.Added, Removed: value.Removed})
				if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_events(provider, value_json, created_at) VALUES (?,?,?)`, cfg.ID, string(event), time.Now().UTC().Format(time.RFC3339)); err != nil {
					return nil, err
				}
			}
		}
	}
	value.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO model_catalog(provider, value_json) VALUES (?,?) ON CONFLICT(provider) DO UPDATE SET value_json=excluded.value_json`, cfg.ID, string(data)); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM catalog_events WHERE id NOT IN (SELECT id FROM catalog_events ORDER BY id DESC LIMIT 1000)`); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return intersection(discoveryOrder, value.Approved), nil
}

func difference(a, b []string) []string {
	known := make(map[string]bool, len(b))
	for _, value := range b {
		known[value] = true
	}
	out := []string{}
	for _, value := range a {
		if !known[value] {
			out = append(out, value)
		}
	}
	return out
}

func intersection(a, b []string) []string { return difference(a, difference(a, b)) }

func (s *SQLiteStore) Catalogs(ctx context.Context) ([]Catalog, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT value_json FROM model_catalog ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Catalog{}
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var value Catalog
		if err := json.Unmarshal([]byte(encoded), &value); err != nil {
			return nil, err
		}
		value.Added = difference(value.Models, value.Approved)
		out = append(out, value)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ApproveCatalog(ctx context.Context, id, revision, updated string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var encoded string
	if err := tx.QueryRowContext(ctx, `SELECT value_json FROM model_catalog WHERE provider=?`, id).Scan(&encoded); err != nil {
		return err
	}
	var value Catalog
	if err := json.Unmarshal([]byte(encoded), &value); err != nil {
		return err
	}
	if value.Revision != revision || value.UpdatedAt != updated {
		return errors.New("model catalog changed; refresh before approving")
	}
	value.Approved = append(slices.Clone(value.Approved), value.Models...)
	slices.Sort(value.Approved)
	value.Approved = slices.Compact(value.Approved)
	value.Added = []string{}
	data, _ := json.Marshal(value)
	if _, err := tx.ExecContext(ctx, `UPDATE model_catalog SET value_json=? WHERE provider=?`, string(data), id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) CatalogEvents(ctx context.Context) ([]CatalogEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, value_json, created_at FROM catalog_events ORDER BY id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CatalogEvent{}
	for rows.Next() {
		var v CatalogEvent
		var data string
		var id int64
		var created string
		if err := rows.Scan(&id, &data, &created); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			return nil, err
		}
		v.ID, v.CreatedAt = id, created
		out = append(out, v)
	}
	return out, rows.Err()
}
