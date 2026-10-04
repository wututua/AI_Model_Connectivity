package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

var (
	ErrUserNotFound   = errors.New("用户不存在")
	ErrUsernameExists = errors.New("用户名已存在")
	ErrLastAdmin      = errors.New("必须保留至少一个启用的管理员")
	ErrUserChanged    = errors.New("账号已变更，请重新登录")
)

type User struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	Role               string `json:"role"`
	Enabled            bool   `json:"enabled"`
	MustChangePassword bool   `json:"must_change_password"`
	CreatedAt          string `json:"created_at"`
	PasswordHash       string `json:"-"`
}

type Session struct {
	User      User
	CSRFToken string
	ExpiresAt int64
}

const userColumns = `id, username, role, enabled, must_change_password, created_at, password_hash`

func (s *SQLiteStore) initUsers(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL COLLATE NOCASE UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL CHECK(role IN ('admin', 'user')),
			enabled INTEGER NOT NULL DEFAULT 1,
			must_change_password INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			csrf_token TEXT NOT NULL,
			expires_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions(expires_at)`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var user User
	err := row.Scan(&user.ID, &user.Username, &user.Role, &user.Enabled, &user.MustChangePassword, &user.CreatedAt, &user.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrUserNotFound
	}
	return user, err
}

func (s *SQLiteStore) FindUser(ctx context.Context, username string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE username = ?`, username))
}

func (s *SQLiteStore) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *SQLiteStore) CreateUser(ctx context.Context, user User, bootstrap bool) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	if bootstrap {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
			return User{}, err
		}
		if count != 0 {
			return User{}, ErrUserChanged
		}
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username = ?`, user.Username).Scan(&exists); err != nil {
		return User{}, err
	}
	if exists > 0 {
		return User{}, ErrUsernameExists
	}
	user.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(ctx, `INSERT INTO users(username,password_hash,role,enabled,must_change_password,created_at) VALUES(?,?,?,?,?,?)`,
		user.Username, user.PasswordHash, user.Role, user.Enabled, user.MustChangePassword, user.CreatedAt)
	if err != nil {
		return User{}, err
	}
	user.ID, err = result.LastInsertId()
	if err != nil {
		return User{}, err
	}
	if bootstrap {
		if _, err := tx.ExecContext(ctx, `DELETE FROM runtime_config WHERE key IN ('admin_stored_token','admin_token_first_use','admin_view_token')`); err != nil {
			return User{}, err
		}
	}
	return user, tx.Commit()
}

// The last-admin check and session revocation share the user mutation transaction.
func (s *SQLiteStore) UpdateUser(ctx context.Context, id int64, update User, remove bool) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	current, err := scanUser(tx.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
	if err != nil {
		return User{}, err
	}
	if current.Role == "admin" && current.Enabled && (remove || update.Role != "admin" || !update.Enabled) {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND enabled = 1 AND id != ?`, id).Scan(&count); err != nil {
			return User{}, err
		}
		if count == 0 {
			return User{}, ErrLastAdmin
		}
	}
	if remove {
		_, err = tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	} else {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username = ? AND id != ?`, update.Username, id).Scan(&exists); err != nil {
			return User{}, err
		}
		if exists > 0 {
			return User{}, ErrUsernameExists
		}
		if update.PasswordHash == "" {
			update.PasswordHash, update.MustChangePassword = current.PasswordHash, current.MustChangePassword
		}
		_, err = tx.ExecContext(ctx, `UPDATE users SET username=?,role=?,enabled=?,password_hash=?,must_change_password=? WHERE id=?`,
			update.Username, update.Role, update.Enabled, update.PasswordHash, update.MustChangePassword, id)
	}
	if err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		return User{}, err
	}
	update.ID, update.CreatedAt = id, current.CreatedAt
	return update, tx.Commit()
}

func (s *SQLiteStore) ChangePassword(ctx context.Context, user User, hash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=?,must_change_password=0 WHERE id=? AND password_hash=? AND enabled=1`, hash, user.ID, user.PasswordHash)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return ErrUserChanged
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, user.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func sessionHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *SQLiteStore) CreateSession(ctx context.Context, user User, token, csrf string, expiresAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix()); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at)
		SELECT ?,id,?,? FROM users WHERE id=? AND enabled=1 AND password_hash=?`,
		sessionHash(token), csrf, expiresAt.Unix(), user.ID, user.PasswordHash)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return ErrUserChanged
	}
	// Bound the number of persistent sessions per account.
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=? AND token_hash NOT IN
		(SELECT token_hash FROM sessions WHERE user_id=? ORDER BY expires_at DESC, rowid DESC LIMIT 10)`, user.ID, user.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) LoadSession(ctx context.Context, token string) (Session, error) {
	var session Session
	err := s.db.QueryRowContext(ctx, `SELECT u.id,u.username,u.role,u.enabled,u.must_change_password,u.created_at,u.password_hash,s.csrf_token,s.expires_at
		FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>? AND u.enabled=1`, sessionHash(token), time.Now().Unix()).
		Scan(&session.User.ID, &session.User.Username, &session.User.Role, &session.User.Enabled, &session.User.MustChangePassword, &session.User.CreatedAt, &session.User.PasswordHash, &session.CSRFToken, &session.ExpiresAt)
	return session, err
}

func (s *SQLiteStore) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, sessionHash(token))
	return err
}
