package main

import (
	"context"
	"testing"

	"cg/internal/auth"
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
