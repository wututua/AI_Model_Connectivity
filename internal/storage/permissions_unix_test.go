//go:build !windows

package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseRestrictsExistingFilePermissions(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "private.sqlite")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	store, err := NewSQLite(context.Background(), path, directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		info, err := os.Stat(path + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("%s permissions: %v", suffix, info.Mode().Perm())
		}
	}
}
