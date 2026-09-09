package main

import (
	"context"
	"sync"
	"testing"
)

func TestTokenRotationPersistsAndSeparatesRoles(t *testing.T) {
	app := testApplication(t)
	ctx := context.Background()
	if err := app.initializeTokens(ctx); err != nil {
		t.Fatal(err)
	}
	if !app.adminFirstUse {
		t.Fatal("generated credential should require first-use rotation")
	}
	if err := app.ChangeViewToken(ctx, app.AdminToken()); err == nil {
		t.Fatal("view credential must not equal admin credential")
	}
	if err := app.ChangeViewToken(ctx, "read-only-test-token"); err != nil {
		t.Fatal(err)
	}
	if err := app.ChangeAdminToken(ctx, "read-only-test-token"); err == nil {
		t.Fatal("admin credential must not equal view credential")
	}
	if err := app.ChangeAdminToken(ctx, "replacement-admin-token"); err != nil {
		t.Fatal(err)
	}
	if err := app.initializeTokens(ctx); err != nil {
		t.Fatal(err)
	}
	if app.AdminToken() != "replacement-admin-token" || app.adminFirstUse || app.ViewToken() != "read-only-test-token" {
		t.Fatal("credentials did not survive reinitialization")
	}
}

func TestExternalAdminTokenCannotBeTemporarilyOverridden(t *testing.T) {
	app := testApplication(t)
	app.cfg.AdminToken = "environment-admin-token"
	app.baseCfg.AdminToken = app.cfg.AdminToken
	if err := app.initializeTokens(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := app.ChangeAdminToken(context.Background(), "replacement-admin-token"); err == nil {
		t.Fatal("external credential rotation must require an environment change")
	}
}

func TestConcurrentViewRotationKeepsMemoryAndStorageInSync(t *testing.T) {
	app := testApplication(t)
	var workers sync.WaitGroup
	for _, token := range []string{"first-view-token", "second-view-token", "third-view-token"} {
		workers.Add(1)
		go func(token string) {
			defer workers.Done()
			if err := app.ChangeViewToken(context.Background(), token); err != nil {
				t.Error(err)
			}
		}(token)
	}
	workers.Wait()
	stored, _, err := app.store.GetKV(context.Background(), "admin_view_token")
	if err != nil || stored != app.ViewToken() {
		t.Fatalf("credential persistence mismatch: %v", err)
	}
}
