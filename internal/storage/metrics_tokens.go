package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

type MetricsToken struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	RotatedAt string `json:"rotated_at"`
}

type IssuedMetricsToken struct {
	MetricsToken
	Token string `json:"token"`
}

func (s *SQLiteStore) initMetricsTokens(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS metrics_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL,
		token_hash BLOB NOT NULL UNIQUE, created_at TEXT NOT NULL, rotated_at TEXT NOT NULL
	)`)
	return err
}

func (s *SQLiteStore) ListMetricsTokens(ctx context.Context) ([]MetricsToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, created_at, rotated_at FROM metrics_tokens ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []MetricsToken{}
	for rows.Next() {
		var item MetricsToken
		if err := rows.Scan(&item.ID, &item.Name, &item.CreatedAt, &item.RotatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLiteStore) IssueMetricsToken(ctx context.Context, name string, rotateID int64) (IssuedMetricsToken, error) {
	name = strings.TrimSpace(name)
	if rotateID == 0 && (name == "" || len([]rune(name)) > 64) {
		return IssuedMetricsToken{}, errors.New("凭据名称须为 1 到 64 个字符")
	}
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return IssuedMetricsToken{}, err
	}
	token := "cgm_" + base64.RawURLEncoding.EncodeToString(entropy)
	hash := sha256.Sum256([]byte(token))
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IssuedMetricsToken{}, err
	}
	defer tx.Rollback()
	value := IssuedMetricsToken{MetricsToken: MetricsToken{ID: rotateID, Name: name, CreatedAt: now, RotatedAt: now}, Token: token}
	if rotateID == 0 {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM metrics_tokens`).Scan(&count); err != nil {
			return IssuedMetricsToken{}, err
		}
		if count >= 20 {
			return IssuedMetricsToken{}, errors.New("指标凭据最多 20 个，请先撤销不再使用的凭据")
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO metrics_tokens(name, token_hash, created_at, rotated_at) VALUES (?, ?, ?, ?)`, name, hash[:], now, now)
		if err != nil {
			return IssuedMetricsToken{}, err
		}
		value.ID, err = result.LastInsertId()
		if err != nil {
			return IssuedMetricsToken{}, err
		}
	} else {
		if err := tx.QueryRowContext(ctx, `SELECT name, created_at FROM metrics_tokens WHERE id = ?`, rotateID).Scan(&value.Name, &value.CreatedAt); err != nil {
			return IssuedMetricsToken{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE metrics_tokens SET token_hash = ?, rotated_at = ? WHERE id = ?`, hash[:], now, rotateID); err != nil {
			return IssuedMetricsToken{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return IssuedMetricsToken{}, err
	}
	return value, nil
}

func (s *SQLiteStore) RevokeMetricsToken(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM metrics_tokens WHERE id = ?`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return err
}

func (s *SQLiteStore) ValidMetricsToken(ctx context.Context, token string) (bool, error) {
	if len(token) != 47 || !strings.HasPrefix(token, "cgm_") {
		return false, nil
	}
	hash := sha256.Sum256([]byte(token))
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM metrics_tokens WHERE token_hash = ?`, hash[:]).Scan(&count)
	return count == 1, err
}
