package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDatabaseRestrictsWindowsACL(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "private.sqlite")
	store, err := NewSQLite(context.Background(), path, directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		descriptor, err := windows.GetNamedSecurityInfo(path+suffix, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		actual := descriptor.String()
		if !strings.Contains(actual, "D:P") || !strings.Contains(actual, user.User.Sid.String()) || strings.Count(actual, "(A;") != 2 {
			t.Errorf("%s has an unexpected ACL: %s", suffix, actual)
		}
	}
}
