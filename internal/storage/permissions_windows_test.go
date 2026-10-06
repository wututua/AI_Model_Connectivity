package storage

import (
	"context"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestDatabaseRestrictsWindowsACL(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "private.sqlite")
	store, err := NewSQLite(context.Background(), path, directory)
	if err != nil {
		t.Fatal(err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
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
		control, _, err := descriptor.Control()
		if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
			t.Fatalf("%s DACL is not protected: %v", suffix, err)
		}
		acl, _, err := descriptor.DACL()
		if err != nil || acl == nil || acl.AceCount != 2 {
			t.Fatalf("%s has an unexpected DACL: %v", suffix, err)
		}
		seen := map[string]bool{}
		for i := uint32(0); i < uint32(acl.AceCount); i++ {
			var ace *windows.ACCESS_ALLOWED_ACE
			if err := windows.GetAce(acl, i, &ace); err != nil {
				t.Fatal(err)
			}
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE ||
				ace.Header.AceFlags&windows.INHERITED_ACE != 0 ||
				ace.Mask != windows.STANDARD_RIGHTS_REQUIRED|windows.SYNCHRONIZE|0x1ff ||
				(!sid.Equals(user.User.Sid) && !sid.Equals(system)) {
				t.Fatalf("%s has an unexpected ACE: %s", suffix, descriptor.String())
			}
			seen[sid.String()] = true
		}
		if !seen[user.User.Sid.String()] || !seen[system.String()] {
			t.Errorf("%s is missing an expected principal", suffix)
		}
	}
}
