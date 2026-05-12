package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cg/internal/config"
	"cg/internal/notify"
	"cg/internal/probe"
	"cg/internal/report"

	_ "modernc.org/sqlite"
)

const sqliteRetentionDays = 90

type SQLiteStore struct {
	db      *sql.DB
	dataDir string
}

type SQLiteNotifyStateStore struct {
	Store *SQLiteStore
}

type CheckTask struct {
	ID                int64  `json:"id"`
	Kind              string `json:"kind"`
	Status            string `json:"status"`
	ProviderID        string `json:"provider_id"`
	StartedAt         string `json:"started_at"`
	FinishedAt        string `json:"finished_at"`
	ElapsedMS         int    `json:"elapsed_ms"`
	OKCount           int    `json:"ok_count"`
	SlowCount         int    `json:"slow_count"`
	ErrorCount        int    `json:"error_count"`
	Total             int    `json:"total"`
	ErrorMessage      string `json:"error_message"`
	ReportGeneratedAt string `json:"report_generated_at"`
}

type CheckTaskUpdate struct {
	Status            string
	FinishedAt        time.Time
	ElapsedMS         int
	OKCount           int
	SlowCount         int
	ErrorCount        int
	Total             int
	ErrorMessage      string
	ReportGeneratedAt string
}

type TaskQuery struct {
	Limit      int
	Offset     int
	Status     string
	ProviderID string
}

func NewSQLite(ctx context.Context, databasePath, dataDir string) (*SQLiteStore, error) {
	if databasePath == "" {
		databasePath = filepath.Join(dataDir, "cg.sqlite")
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, err
	}
	// SQLite is single-writer; cap connections to 1 to avoid "database is locked".
	// WAL mode still allows concurrent readers within the same connection pool.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	store := &SQLiteStore{db: db, dataDir: dataDir}
	if err := store.init(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.importLegacy(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) init(ctx context.Context) error {
	statements := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA synchronous = NORMAL`,
		`PRAGMA cache_size = -64000`,
		`PRAGMA temp_store = MEMORY`,
		`PRAGMA mmap_size = 30000000`,
		`CREATE TABLE IF NOT EXISTS probe_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider TEXT NOT NULL,
			provider_type TEXT NOT NULL DEFAULT '',
			provider_name TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL,
			result TEXT NOT NULL,
			latency_ms INTEGER NOT NULL DEFAULT 0,
			checked_at TEXT NOT NULL,
			error_type TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			response_preview TEXT NOT NULL DEFAULT '',
			history_key TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_probe_results_history_checked ON probe_results(history_key, checked_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_probe_results_checked ON probe_results(checked_at DESC)`,
		`CREATE TABLE IF NOT EXISTS latest_report (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			generated_at TEXT NOT NULL,
			report_json TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS notify_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			status TEXT NOT NULL DEFAULT '',
			sent_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS runtime_config (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS check_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			provider_id TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			finished_at TEXT,
			elapsed_ms INTEGER NOT NULL DEFAULT 0,
			ok_count INTEGER NOT NULL DEFAULT 0,
			slow_count INTEGER NOT NULL DEFAULT 0,
			error_count INTEGER NOT NULL DEFAULT 0,
			total INTEGER NOT NULL DEFAULT 0,
			error_message TEXT NOT NULL DEFAULT '',
			report_generated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_check_tasks_started ON check_tasks(started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_check_tasks_status ON check_tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_check_tasks_status_provider ON check_tasks(status, provider_id)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) LoadHistory(ctx context.Context, limitPerKey int, statsWindowDays int) (map[string][]report.HistoryRecord, error) {
	if limitPerKey <= 0 {
		limitPerKey = 1
	}
	if statsWindowDays <= 0 {
		statsWindowDays = 1
	}
	cutoff := time.Now().Add(-time.Duration(statsWindowDays) * 24 * time.Hour).Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `SELECT history_key, result, latency_ms, checked_at FROM probe_results WHERE checked_at >= ? ORDER BY history_key, checked_at ASC`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := map[string][]report.HistoryRecord{}
	for rows.Next() {
		var key, status, checkedAt string
		var latency int
		if err := rows.Scan(&key, &status, &latency, &checkedAt); err != nil {
			return nil, err
		}
		history[key] = append(history[key], report.HistoryRecord{Status: status, LatencyMS: latency, CheckedAt: checkedAt})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for key, records := range history {
		if len(records) > limitPerKey {
			history[key] = records[len(records)-limitPerKey:]
		}
	}
	return history, nil
}

func (s *SQLiteStore) AppendResults(ctx context.Context, results []probe.Result, checkedAt time.Time) error {
	if len(results) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO probe_results (provider, provider_type, provider_name, model, result, latency_ms, checked_at, error_type, error_message, response_preview, history_key) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`) //nolint:lll
	if err != nil {
		return err
	}
	defer stmt.Close()

	checked := checkedAt.Format(time.RFC3339)
	for _, result := range results {
		if _, err := stmt.ExecContext(ctx, result.ProviderID, result.ProviderType, result.ProviderName, result.Model, result.Status, result.LatencyMS, checked, errorType(result), result.Error, result.ResponsePreview, result.HistoryKey); err != nil {
			return err
		}
	}
	cutoff := checkedAt.Add(-sqliteRetentionDays * 24 * time.Hour).Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `DELETE FROM probe_results WHERE checked_at < ?`, cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) LatestReport(ctx context.Context) (report.Report, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT report_json FROM latest_report WHERE id = 1`).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return report.Report{}, nil
	}
	if err != nil {
		return report.Report{}, err
	}
	var value report.Report
	if err := json.Unmarshal([]byte(data), &value); err != nil {
		return report.Report{}, err
	}
	return value, nil
}

func (s *SQLiteStore) SaveLatestReport(ctx context.Context, value report.Report) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO latest_report (id, generated_at, report_json) VALUES (1, ?, ?) ON CONFLICT(id) DO UPDATE SET generated_at = excluded.generated_at, report_json = excluded.report_json`, value.GeneratedAt, string(data))
	return err
}

func (s *SQLiteStore) ReadNotifyState(ctx context.Context) (notify.State, error) {
	var status string
	var sentAt sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT status, sent_at FROM notify_state WHERE id = 1`).Scan(&status, &sentAt)
	if errors.Is(err, sql.ErrNoRows) {
		return notify.State{}, nil
	}
	if err != nil {
		return notify.State{}, err
	}
	state := notify.State{Status: status}
	if sentAt.Valid && sentAt.String != "" {
		parsed, err := time.Parse(time.RFC3339, sentAt.String)
		if err != nil {
			return notify.State{}, err
		}
		state.SentAt = parsed
	}
	return state, nil
}

func (s *SQLiteStore) WriteNotifyState(ctx context.Context, value notify.State) error {
	var sentAt any
	if !value.SentAt.IsZero() {
		sentAt = value.SentAt.Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO notify_state (id, status, sent_at) VALUES (1, ?, ?) ON CONFLICT(id) DO UPDATE SET status = excluded.status, sent_at = excluded.sent_at`, value.Status, sentAt)
	return err
}

func (s SQLiteNotifyStateStore) Read() (notify.State, error) {
	return s.Store.ReadNotifyState(context.Background())
}

func (s SQLiteNotifyStateStore) Write(value notify.State) error {
	return s.Store.WriteNotifyState(context.Background(), value)
}

func (s *SQLiteStore) LoadRuntimeConfig(ctx context.Context) (config.RuntimeConfig, bool, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM runtime_config WHERE key = 'runtime'`).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return config.RuntimeConfig{}, false, nil
	}
	if err != nil {
		return config.RuntimeConfig{}, false, err
	}
	var value config.RuntimeConfig
	if err := json.Unmarshal([]byte(data), &value); err != nil {
		return config.RuntimeConfig{}, false, err
	}
	return value, true, nil
}

func (s *SQLiteStore) SaveRuntimeConfig(ctx context.Context, value config.RuntimeConfig) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO runtime_config (key, value_json, updated_at) VALUES ('runtime', ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, string(data), time.Now().Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) GetKV(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM runtime_config WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return value, err == nil, err
}

func (s *SQLiteStore) SetKV(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO runtime_config (key, value_json, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`,
		key, value, time.Now().Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) CreateCheckTask(ctx context.Context, task CheckTask) (int64, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO check_tasks (kind, status, provider_id, started_at) VALUES (?, ?, ?, ?)`, task.Kind, task.Status, task.ProviderID, task.StartedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) FinishCheckTask(ctx context.Context, id int64, update CheckTaskUpdate) error {
	finishedAt := ""
	if !update.FinishedAt.IsZero() {
		finishedAt = update.FinishedAt.Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE check_tasks SET status = ?, finished_at = ?, elapsed_ms = ?, ok_count = ?, slow_count = ?, error_count = ?, total = ?, error_message = ?, report_generated_at = ? WHERE id = ?`, update.Status, finishedAt, update.ElapsedMS, update.OKCount, update.SlowCount, update.ErrorCount, update.Total, update.ErrorMessage, update.ReportGeneratedAt, id)
	return err
}

func (s *SQLiteStore) ListCheckTasks(ctx context.Context, query TaskQuery) ([]CheckTask, error) {
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	where := []string{}
	args := []any{}
	if query.Status != "" {
		where = append(where, "status = ?")
		args = append(args, query.Status)
	}
	if query.ProviderID != "" {
		where = append(where, "provider_id = ?")
		args = append(args, query.ProviderID)
	}
	statement := `SELECT id, kind, status, provider_id, started_at, COALESCE(finished_at, ''), elapsed_ms, ok_count, slow_count, error_count, total, error_message, report_generated_at FROM check_tasks`
	if len(where) > 0 {
		statement += " WHERE " + strings.Join(where, " AND ")
	}
	statement += " ORDER BY started_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []CheckTask{}
	for rows.Next() {
		task, err := scanCheckTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *SQLiteStore) GetCheckTask(ctx context.Context, id int64) (CheckTask, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, kind, status, provider_id, started_at, COALESCE(finished_at, ''), elapsed_ms, ok_count, slow_count, error_count, total, error_message, report_generated_at FROM check_tasks WHERE id = ?`, id)
	return scanCheckTask(row)
}

func scanCheckTask(scanner interface{ Scan(...any) error }) (CheckTask, error) {
	var task CheckTask
	err := scanner.Scan(&task.ID, &task.Kind, &task.Status, &task.ProviderID, &task.StartedAt, &task.FinishedAt, &task.ElapsedMS, &task.OKCount, &task.SlowCount, &task.ErrorCount, &task.Total, &task.ErrorMessage, &task.ReportGeneratedAt)
	return task, err
}

func (s *SQLiteStore) importLegacy(ctx context.Context) error {
	if err := s.importLegacyHistory(ctx); err != nil {
		return err
	}
	if err := s.importLegacyLatestReport(ctx); err != nil {
		return err
	}
	return s.importLegacyNotifyState(ctx)
}

func (s *SQLiteStore) importLegacyHistory(ctx context.Context) error {
	if !s.tableEmpty(ctx, "probe_results") {
		return nil
	}
	path := filepath.Join(s.dataDir, "probe_history.json")
	history, err := ReadJSON[map[string][]report.HistoryRecord](path, map[string][]report.HistoryRecord{})
	if err != nil {
		return err
	}
	if len(history) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO probe_results (provider, model, result, latency_ms, checked_at, error_type, history_key) VALUES (?, ?, ?, ?, ?, ?, ?)`) //nolint:lll
	if err != nil {
		return err
	}
	defer stmt.Close()
	keys := make([]string, 0, len(history))
	for key := range history {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		providerID, model := splitHistoryKey(key)
		for _, record := range history[key] {
			if record.CheckedAt == "" {
				continue
			}
			errorType := ""
			if record.Status == "error" {
				errorType = "unknown"
			}
			if _, err := stmt.ExecContext(ctx, providerID, model, record.Status, record.LatencyMS, record.CheckedAt, errorType, key); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) importLegacyLatestReport(ctx context.Context) error {
	if !s.tableEmpty(ctx, "latest_report") {
		return nil
	}
	path := filepath.Join(s.dataDir, "latest_report.json")
	value, err := ReadJSON[report.Report](path, report.Report{})
	if err != nil {
		return err
	}
	if value.GeneratedAt == "" {
		return nil
	}
	return s.SaveLatestReport(ctx, value)
}

func (s *SQLiteStore) importLegacyNotifyState(ctx context.Context) error {
	if !s.tableEmpty(ctx, "notify_state") {
		return nil
	}
	path := filepath.Join(s.dataDir, "notify_state.txt")
	state, err := notify.FileStateStore{Path: path}.Read()
	if err != nil {
		return err
	}
	if state.Status == "" {
		return nil
	}
	return s.WriteNotifyState(ctx, state)
}

func (s *SQLiteStore) tableEmpty(ctx context.Context, table string) bool {
	var query string
	switch table {
	case "probe_results":
		query = "SELECT COUNT(*) FROM probe_results"
	case "latest_report":
		query = "SELECT COUNT(*) FROM latest_report"
	case "notify_state":
		query = "SELECT COUNT(*) FROM notify_state"
	default:
		return false
	}
	var count int
	if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return false
	}
	return count == 0
}

func splitHistoryKey(key string) (string, string) {
	parts := strings.SplitN(key, "::", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", key
}

func errorType(result probe.Result) string {
	if result.Status != "error" {
		return ""
	}
	text := strings.ToLower(result.Error)
	switch {
	case strings.Contains(text, "timeout"):
		return "timeout"
	case strings.Contains(text, "no such host"), strings.Contains(text, "lookup"):
		return "dns"
	case strings.Contains(text, "401"), strings.Contains(text, "unauthorized"):
		return "auth"
	case strings.Contains(text, "429"), strings.Contains(text, "rate"):
		return "rate_limit"
	case strings.Contains(text, " 5"), strings.Contains(text, "status 5"):
		return "server"
	default:
		return "unknown"
	}
}
