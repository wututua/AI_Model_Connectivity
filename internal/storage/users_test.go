package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLastAdministratorGuardIsAtomic(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	var users []User
	for _, name := range []string{"admin1", "admin2"} {
		u, err := s.CreateUser(ctx, User{Username: name, Role: "admin", Enabled: true, PasswordHash: "hash"}, false)
		if err != nil {
			t.Fatal(err)
		}
		users = append(users, u)
	}
	var wg sync.WaitGroup
	var rejected atomic.Int32
	for _, user := range users {
		wg.Add(1)
		go func(user User) {
			defer wg.Done()
			_, err := s.UpdateUser(ctx, user.ID, User{}, true)
			if errors.Is(err, ErrLastAdmin) {
				rejected.Add(1)
			} else if err != nil {
				t.Error(err)
			}
		}(user)
	}
	wg.Wait()
	if rejected.Load() != 1 {
		t.Fatal("last administrator deleted")
	}
}

func TestSessionExpiryRevocationAndPasswordRace(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	user, _ := s.CreateUser(ctx, User{Username: "admin", Role: "admin", Enabled: true, PasswordHash: "oldhash"}, false)
	if err := s.CreateSession(ctx, user, "token", "csrf", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadSession(ctx, "token"); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangePassword(ctx, user, "newhash"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadSession(ctx, "token"); err == nil {
		t.Fatal("password change left old session")
	}
	if err := s.CreateSession(ctx, user, "racing-login", "csrf", time.Now().Add(time.Hour)); !errors.Is(err, ErrUserChanged) {
		t.Fatal("old password login raced reset")
	}
	if err := s.ChangePassword(ctx, user, "stale-update"); !errors.Is(err, ErrUserChanged) {
		t.Fatal("stale password update accepted")
	}
	user, _ = s.FindUser(ctx, "admin")
	if err := s.CreateSession(ctx, user, "expired", "csrf", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadSession(ctx, "expired"); err == nil {
		t.Fatal("expired session accepted")
	}
	var stored string
	if err := s.db.QueryRow(`SELECT token_hash FROM sessions LIMIT 1`).Scan(&stored); err != nil || stored == "expired" {
		t.Fatal("session secret stored in plaintext")
	}
}

func TestLastAdminCannotBeDisabledOrDemoted(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	user, err := s.CreateUser(ctx, User{Username: "admin", Role: "admin", Enabled: true, PasswordHash: "hash"}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, update := range []User{
		{Username: "admin", Role: "admin", Enabled: false},
		{Username: "admin", Role: "user", Enabled: true},
	} {
		if _, err := s.UpdateUser(ctx, user.ID, update, false); !errors.Is(err, ErrLastAdmin) {
			t.Fatalf("last administrator guard failed: %v", err)
		}
	}
}

func TestSessionLimitKeepsNewestAndUserDeletionCascades(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()
	user, err := s.CreateUser(ctx, User{Username: "viewer", Role: "user", Enabled: true, PasswordHash: "hash"}, false)
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour)
	for i := 0; i < 12; i++ {
		if err := s.CreateSession(ctx, user, fmt.Sprintf("token-%d", i), "csrf", expires); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 12; i++ {
		_, err := s.LoadSession(ctx, fmt.Sprintf("token-%d", i))
		if (err == nil) != (i >= 2) {
			t.Fatalf("session %d: %v", i, err)
		}
	}
	if _, err := s.UpdateUser(ctx, user.ID, User{}, true); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&count); err != nil || count != 0 {
		t.Fatal("deleted user's sessions remain")
	}
}
