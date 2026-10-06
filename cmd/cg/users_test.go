package main

import (
	"context"
	"testing"
	"time"

	"cg/internal/auth"
	"cg/internal/storage"
)

func TestAccountBootstrapMigratesLegacyAndDoesNotResetOnRestart(t *testing.T) {
	app := testApplication(t)
	ctx := context.Background()
	if err := app.store.SetKVs(ctx, map[string]string{"admin_stored_token": "LegacyAdmin123", "admin_view_token": "old-view-token"}); err != nil {
		t.Fatal(err)
	}
	if err := app.initializeUsers(ctx); err != nil {
		t.Fatal(err)
	}
	user, err := app.store.FindUser(ctx, "admin")
	if err != nil || !user.Enabled || user.Role != "admin" || !user.MustChangePassword || !auth.VerifyPassword(user.PasswordHash, "LegacyAdmin123") {
		t.Fatalf("migration failed: %+v %v", user, err)
	}
	for _, key := range []string{"admin_stored_token", "admin_view_token", "admin_token_first_use"} {
		if _, exists, err := app.store.GetKV(ctx, key); err != nil || exists {
			t.Fatalf("legacy secret retained: %s %v", key, err)
		}
	}
	app.baseCfg.AdminPassword = "Different123"
	if err := app.initializeUsers(ctx); err != nil {
		t.Fatal(err)
	}
	same, _ := app.store.FindUser(ctx, "admin")
	if same.PasswordHash != user.PasswordHash {
		t.Fatal("restart reset password from environment")
	}
}

func TestOfflineAdminRecoveryPreservesDataAndRevokesSessions(t *testing.T) {
	app := testApplication(t)
	ctx := context.Background()
	app.baseCfg.AdminPassword = "Initial123"
	if err := app.initializeUsers(ctx); err != nil {
		t.Fatal(err)
	}
	before, _ := app.store.FindUser(ctx, "admin")
	if err := app.store.CreateSession(ctx, before, "session", "csrf", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := app.store.SetKVs(ctx, map[string]string{"recovery-test": "preserved"}); err != nil {
		t.Fatal(err)
	}
	password, err := recoverAdmin(ctx, app.store, "ADMIN")
	if err != nil || auth.ValidatePassword(password) != nil {
		t.Fatalf("recovery failed: %v", err)
	}
	after, _ := app.store.FindUser(ctx, "admin")
	if after.ID != before.ID || !after.MustChangePassword || !auth.VerifyPassword(after.PasswordHash, password) || auth.VerifyPassword(after.PasswordHash, "Initial123") {
		t.Fatal("wrong recovered account")
	}
	if _, err := app.store.LoadSession(ctx, "session"); err == nil {
		t.Fatal("old session retained")
	}
	if value, exists, err := app.store.GetKV(ctx, "recovery-test"); err != nil || !exists || value != "preserved" {
		t.Fatal("recovery removed data")
	}
	user, err := app.store.CreateUser(ctx, storage.User{Username: "viewer", Role: "user", Enabled: true, PasswordHash: before.PasswordHash}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recoverAdmin(ctx, app.store, user.Username); err == nil {
		t.Fatal("recovery elevated an ordinary user")
	}
	if _, err := recoverAdmin(ctx, app.store, "missing"); err == nil {
		t.Fatal("recovery created a new user")
	}
}

func TestAccountBootstrapRejectsWeakConfiguredPassword(t *testing.T) {
	app := testApplication(t)
	app.baseCfg.AdminPassword = "weak"
	if err := app.initializeUsers(context.Background()); err == nil {
		t.Fatal("weak initial password accepted")
	}
}

func TestAccountBootstrapGeneratesWhenLegacyPasswordIsWeak(t *testing.T) {
	app := testApplication(t)
	app.baseCfg.AdminToken = "legacy-token-without-digits"
	if err := app.initializeUsers(context.Background()); err != nil {
		t.Fatal(err)
	}
	user, err := app.store.FindUser(context.Background(), "admin")
	if err != nil || auth.VerifyPassword(user.PasswordHash, app.baseCfg.AdminToken) {
		t.Fatal("weak legacy token reused")
	}
}
