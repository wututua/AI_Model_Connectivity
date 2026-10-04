package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"cg/internal/auth"
	"cg/internal/storage"
)

func (a *application) initializeUsers(ctx context.Context) error {
	users, err := a.store.ListUsers(ctx)
	if err != nil || len(users) > 0 {
		return err
	}
	username := auth.NormalizeUsername(a.baseCfg.AdminUsername)
	if username == "" {
		username = "admin"
	}
	if err := auth.ValidateUsername(username); err != nil {
		return err
	}
	password := a.baseCfg.AdminPassword
	source := "environment"
	if password == "" {
		password = a.baseCfg.AdminToken
		if password == "" {
			password, _, err = a.store.GetKV(ctx, "admin_stored_token")
			if err != nil {
				return err
			}
		}
		source = "legacy"
		if auth.ValidatePassword(password) != nil {
			generated, err := generateInitialPassword()
			if err != nil {
				return err
			}
			password, source = generated, "generated"
		}
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("initial administrator password: %w", err)
	}
	_, err = a.store.CreateUser(ctx, storage.User{Username: username, Role: "admin", Enabled: true, MustChangePassword: true, PasswordHash: hash}, true)
	if err != nil {
		return err
	}
	fmt.Printf("\nAdministrator account: %s\n", username)
	switch source {
	case "generated":
		fmt.Printf("Initial administrator password: %s\n", password)
	case "legacy":
		fmt.Println("Initial password migrated from the previous ADMIN_TOKEN.")
	default:
		fmt.Println("Initial password loaded from ADMIN_PASSWORD.")
	}
	fmt.Println("Change the initial password on first login. Legacy Bearer tokens are no longer accepted.")
	return nil
}

func generateInitialPassword() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate initial password: %w", err)
	}
	return "Aa1" + base64.RawURLEncoding.EncodeToString(buf), nil
}
