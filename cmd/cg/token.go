package main

import (
	"context"
	"errors"
	"fmt"

	"cg/internal/config"
)

func (a *application) initializeTokens(ctx context.Context) error {
	adminToken := a.baseCfg.AdminToken
	firstUse := false
	generated := false
	if adminToken == "" {
		stored, _, err := a.store.GetKV(ctx, "admin_stored_token")
		if err != nil {
			return err
		}
		adminToken = stored
		firstUseValue, _, err := a.store.GetKV(ctx, "admin_token_first_use")
		if err != nil {
			return err
		}
		firstUse = firstUseValue == "true"
		if adminToken == "" {
			adminToken, err = generateAdminToken()
			if err != nil {
				return err
			}
			firstUse, generated = true, true
			if err := a.store.SetKVs(ctx, map[string]string{"admin_stored_token": adminToken, "admin_token_first_use": "true"}); err != nil {
				return err
			}
		}
	}
	if err := config.ValidateToken(adminToken); err != nil {
		return fmt.Errorf("stored admin token: %w", err)
	}
	viewToken, _, err := a.store.GetKV(ctx, "admin_view_token")
	if err != nil {
		return err
	}
	if viewToken != "" {
		if err := config.ValidateToken(viewToken); err != nil {
			return fmt.Errorf("stored view token: %w", err)
		}
		if viewToken == adminToken {
			return errors.New("admin token and view token must differ")
		}
	}
	a.adminToken, a.adminFirstUse, a.viewToken = adminToken, firstUse, viewToken
	if generated {
		fmt.Printf("\nAuto-generated ADMIN_TOKEN: %s\nPlease change it on first login.\n\n", adminToken)
	}
	return nil
}
