package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Backup struct {
	Name       string `json:"name"`
	CreatedAt  string `json:"created_at"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	VerifiedAt string `json:"verified_at"`
}

var backupName = regexp.MustCompile(`^cg-[0-9]{8}T[0-9]{6}-[A-Za-z0-9]+\.sqlite$`)

func (s *SQLiteStore) backupPath(name string) (string, error) {
	if !backupName.MatchString(name) {
		return "", errors.New("invalid backup name")
	}
	dir := filepath.Join(s.dataDir, "backups")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("backup directory must not be a link")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(filepath.Join(dir, name))
	return absolute, err
}

func (s *SQLiteStore) CreateBackup(ctx context.Context, keep int) (value Backup, err error) {
	if !s.backupMu.TryLock() {
		return value, errors.New("backup operation already running")
	}
	defer s.backupMu.Unlock()
	if keep < 1 || keep > 100 {
		return value, errors.New("invalid retention")
	}
	value.Name = "cg-" + time.Now().UTC().Format("20060102T150405") + "-" + rand.Text() + ".sqlite"
	path, err := s.backupPath(value.Name)
	if err != nil {
		return value, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return value, err
	}
	file.Close()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(path)
		}
	}()
	if err = restrictFileAccess(path); err != nil {
		return value, err
	}
	if _, err = s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return value, errors.New("database snapshot failed")
	}
	snapshot, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return value, err
	}
	err = snapshot.Sync()
	closeErr := snapshot.Close()
	if err != nil || closeErr != nil {
		return value, errors.New("backup flush failed")
	}
	value.Size, value.SHA256, err = inspectBackup(ctx, path)
	if err != nil {
		return value, err
	}
	value.CreatedAt, value.VerifiedAt = time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)
	if _, err = s.db.ExecContext(ctx, `INSERT INTO backups(name,created_at,size,sha256,verified_at) VALUES (?,?,?,?,?)`, value.Name, value.CreatedAt, value.Size, value.SHA256, value.VerifiedAt); err != nil {
		return value, err
	}
	success = true
	backups, err := s.Backups(ctx)
	if err != nil {
		return value, err
	}
	for i := keep; i < len(backups); i++ {
		if err := s.deleteBackup(ctx, backups[i].Name); err != nil {
			return value, err
		}
	}
	return value, nil
}

func inspectBackup(ctx context.Context, path string) (int64, string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0, "", errors.New("backup file unavailable")
	}
	uri := filepath.ToSlash(path)
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: uri, RawQuery: "mode=ro&immutable=1"}).String())
	if err != nil {
		return 0, "", err
	}
	defer db.Close()
	var check string
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&check); err != nil || check != "ok" {
		return 0, "", errors.New("backup integrity check failed")
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return 0, "", err
	}
	return info.Size(), hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *SQLiteStore) Backups(ctx context.Context) ([]Backup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name,created_at,size,sha256,verified_at FROM backups ORDER BY created_at DESC, rowid DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Backup{}
	for rows.Next() {
		var v Backup
		if err := rows.Scan(&v.Name, &v.CreatedAt, &v.Size, &v.SHA256, &v.VerifiedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) VerifyBackup(ctx context.Context, name string) error {
	s.backupMu.Lock()
	defer s.backupMu.Unlock()
	var expected string
	if err := s.db.QueryRowContext(ctx, `SELECT sha256 FROM backups WHERE name=?`, name).Scan(&expected); err != nil {
		return err
	}
	path, err := s.backupPath(name)
	if err != nil {
		return err
	}
	_, hash, err := inspectBackup(ctx, path)
	if err != nil {
		return err
	}
	if hash != expected {
		return errors.New("backup checksum mismatch")
	}
	_, err = s.db.ExecContext(ctx, `UPDATE backups SET verified_at=? WHERE name=?`, time.Now().UTC().Format(time.RFC3339), name)
	return err
}

func (s *SQLiteStore) deleteBackup(ctx context.Context, name string) error {
	path, err := s.backupPath(name)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular backup: %s", name)
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM backups WHERE name=?`, name)
	return err
}
