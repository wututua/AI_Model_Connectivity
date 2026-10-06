package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
	"cg/internal/report"
	"cg/internal/web"
)

func providerDraft(p config.ProviderConfig) config.ProviderUpdate {
	return config.ProviderUpdate{ID: p.ID, Name: p.Name, Type: p.Type, BaseURL: p.BaseURL,
		Models: append([]string(nil), p.Models...), Enabled: p.Enabled, ProbeEnabled: p.ProbeEnabled}
}

func seedProviderStatus(t *testing.T, app *application) report.Report {
	t.Helper()
	ctx := context.Background()
	if err := app.replaceRuntimeConfig(ctx, config.RuntimeConfigFromConfig(app.cfg)); err != nil {
		t.Fatal(err)
	}
	results := []probe.Result{}
	for _, p := range app.cfg.Providers {
		results = append(results, probe.Result{ProviderID: p.ID, Model: "fixture", Status: "ok",
			HistoryKey: p.ID + "::fixture", LatencyMS: 50, ResponsePreview: "old response", Completed: true,
			CheckedAt: time.Now().UTC().Format(time.RFC3339), TotalTokens: 10})
	}
	value, _ := report.Build(app.cfg, results, nil, nil, time.Now())
	value = report.WithConfig(value, config.AdminConfigFromConfig(app.cfg))
	if err := app.store.RecordCheck(ctx, results, time.Now(), 100, true, &value); err != nil {
		t.Fatal(err)
	}
	stored, err := app.store.LatestReport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

func fixtureProvider() config.ProviderConfig {
	return config.ProviderConfig{ID: "production", Name: "Production", Type: "openai",
		BaseURL: "https://old.invalid/v1", APIKey: "fixture-secret", Models: []string{"fixture"},
		Enabled: true, ProbeEnabled: true}
}

func TestProviderCreateAndUpdateIdentity(t *testing.T) {
	app := testApplication(t)
	ctx := context.Background()
	app.cfg.Providers = []config.ProviderConfig{fixtureProvider()}
	seedProviderStatus(t, app)
	before := app.currentConfig()
	persisted, _, _ := app.store.LoadRuntimeConfig(ctx)
	for _, id := range []string{"production", "PRODUCTION", " production "} {
		draft := providerDraft(before.Providers[0])
		draft.ID, draft.BaseURL = id, "https://untrusted.invalid/v1"
		if _, err := app.UpsertProvider(ctx, "", draft); !errors.Is(err, config.ErrProviderExists) {
			t.Fatalf("duplicate create %q: %v", id, err)
		}
	}
	if _, err := app.UpsertProvider(ctx, "deleted", providerDraft(before.Providers[0])); !errors.Is(err, config.ErrProviderNotFound) {
		t.Fatalf("missing PUT recreated provider: %v", err)
	}
	after, _, err := app.store.LoadRuntimeConfig(ctx)
	if err != nil || !reflect.DeepEqual(before, app.currentConfig()) || !reflect.DeepEqual(persisted, after) {
		t.Fatal("rejected request mutated memory or persisted config")
	}
}

func TestImportAndUpdateCannotReuseKeyAtNewEndpoint(t *testing.T) {
	for _, operation := range []string{"update", "import", "import-case"} {
		t.Run(operation, func(t *testing.T) {
			app := testApplication(t)
			ctx := context.Background()
			app.cfg.Providers = []config.ProviderConfig{fixtureProvider()}
			before := seedProviderStatus(t, app)
			cfg := app.currentConfig()
			draft := providerDraft(cfg.Providers[0])
			draft.BaseURL = "https://untrusted.invalid/v1"
			if operation == "import-case" {
				draft.ID = strings.ToUpper(draft.ID)
			}
			var err error
			if operation == "update" {
				_, err = app.UpsertProvider(ctx, "production", draft)
			} else {
				_, err = app.ImportConfig(ctx, config.ConfigImport{Settings: config.SettingsFromConfig(cfg), Providers: []config.ProviderUpdate{draft}})
			}
			if err == nil || !reflect.DeepEqual(cfg, app.currentConfig()) {
				t.Fatal("endpoint change implicitly reused the old credential")
			}
			latest, err := app.store.LatestReport(ctx)
			if err != nil || latest.OKCount != before.OKCount || latest.GeneratedAt != before.GeneratedAt {
				t.Fatal("rejected update mutated status")
			}
		})
	}
}

func TestConnectionChangesInvalidateCurrentStatusOnly(t *testing.T) {
	for _, change := range []string{"url", "key", "clear-key", "type", "import", "rename"} {
		t.Run(change, func(t *testing.T) {
			app := testApplication(t)
			ctx := context.Background()
			unaffected := fixtureProvider()
			unaffected.ID = "other"
			app.cfg.Providers = []config.ProviderConfig{fixtureProvider(), unaffected}
			original := seedProviderStatus(t, app)
			historyBefore, _ := app.store.LoadHistory(ctx, 100, 7)
			billingBefore, _ := app.store.LoadBillingSummary(ctx, 7)
			app.broker = web.NewBroker()
			events, unsubscribe := app.broker.Subscribe()
			defer unsubscribe()
			draft := providerDraft(app.cfg.Providers[0])
			switch change {
			case "url", "import":
				draft.BaseURL, draft.APIKey = "https://new.invalid/v1", "explicit-key"
			case "key":
				draft.APIKey = "new-key"
			case "clear-key":
				draft.ClearAPIKey = true
			case "type":
				draft.Type = "anthropic"
			case "rename":
				draft.Name = "Cosmetic rename"
			}
			var err error
			if change == "import" {
				_, err = app.ImportConfig(ctx, config.ConfigImport{Settings: config.SettingsFromConfig(app.cfg),
					Providers: []config.ProviderUpdate{draft, providerDraft(app.cfg.Providers[1])}})
			} else {
				_, err = app.UpsertProvider(ctx, "production", draft)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := app.store.LatestReport(ctx)
			if err != nil {
				t.Fatal(err)
			}
			group := got.Providers[0]
			model := group.Results[0]
			if change == "rename" {
				if got.OKCount != 2 || model.CheckedAt == "" || group.ProviderName != draft.Name {
					t.Fatal("rename invalidated a healthy connection")
				}
			} else if got.OKCount != 1 || got.UnknownCount != 1 || group.Status != "unknown" ||
				group.CheckedAt != "" || model.CheckedAt != "" || model.LatencyMS != 0 ||
				model.ResponsePreview != "" || model.Error != "" || model.TotalTokens != 0 {
				t.Fatalf("changed connection retains current measurements: %+v", got)
			}
			if got.GeneratedAt != original.GeneratedAt || !reflect.DeepEqual(model.History, original.Providers[0].Results[0].History) ||
				!reflect.DeepEqual(got.Providers[1], original.Providers[1]) {
				t.Fatal("unaffected provider, history or check timestamp changed")
			}
			historyAfter, _ := app.store.LoadHistory(ctx, 100, 7)
			billingAfter, _ := app.store.LoadBillingSummary(ctx, 7)
			if !reflect.DeepEqual(historyBefore, historyAfter) || !reflect.DeepEqual(billingBefore, billingAfter) {
				t.Fatal("config update changed history or billing")
			}
			select {
			case pushed := <-events:
				if pushed.OKCount != got.OKCount || pushed.UnknownCount != got.UnknownCount ||
					pushed.Providers[0].ConnectionRevision != got.Providers[0].ConnectionRevision {
					t.Fatal("SSE differs from saved snapshot")
				}
			default:
				t.Fatal("updated status not published")
			}
			stored, ok, err := app.store.LoadRuntimeConfig(ctx)
			if err != nil || !ok {
				t.Fatal("runtime config not saved")
			}
			restarted := config.ApplyRuntimeConfig(app.baseCfg, stored)
			projected := report.WithConfig(original, config.AdminConfigFromConfig(restarted))
			if projected.OKCount != got.OKCount || projected.UnknownCount != got.UnknownCount {
				t.Fatal("restart resurrected stale snapshot")
			}
		})
	}
}

func TestInFlightOldGenerationCannotResurrectStatus(t *testing.T) {
	for _, change := range []string{"endpoint-ABA", "key-ABA", "delete-recreate"} {
		t.Run(change, func(t *testing.T) {
			app := testApplication(t)
			ctx := context.Background()
			started, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					close(started)
					<-release
				}
				w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}],"usage":{"total_tokens":10}}`))
			}))
			defer upstream.Close()
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			p := fixtureProvider()
			p.BaseURL = upstream.URL
			app.cfg.Providers = []config.ProviderConfig{p}
			if err := app.replaceRuntimeConfig(ctx, config.RuntimeConfigFromConfig(app.cfg)); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, err := app.check(ctx); done <- err }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("probe did not start")
			}
			draft := providerDraft(p)
			draft.APIKey = p.APIKey
			if change == "delete-recreate" {
				if err := app.DeleteProvider(ctx, p.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := app.UpsertProvider(ctx, "", draft); err != nil {
					t.Fatal(err)
				}
			} else {
				modified := draft
				if change == "endpoint-ABA" {
					modified.BaseURL += "/changed"
				} else {
					modified.APIKey = "changed"
				}
				for _, item := range []config.ProviderUpdate{modified, draft} {
					if _, err := app.UpsertProvider(ctx, p.ID, item); err != nil {
						t.Fatal(err)
					}
				}
			}
			close(release)
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("check did not finish")
			}
			latest, err := app.store.LatestReport(ctx)
			if err != nil || latest.OKCount != 0 || latest.UnknownCount != 1 || latest.Providers[0].CheckedAt != "" {
				t.Fatalf("old generation resurrected status: %+v, %v", latest, err)
			}
			billing, err := app.store.LoadBillingSummary(ctx, 7)
			if err != nil || billing.TotalTokens != 10 || billing.TotalProbeCount != 1 {
				t.Fatalf("completed usage lost: %+v, %v", billing, err)
			}
			history, err := app.store.LoadHistory(ctx, 100, 7)
			if err != nil || len(history[p.ID+"::fixture"]) != 1 {
				t.Fatalf("completed historical sample lost: %+v, %v", history, err)
			}
			fresh, err := app.check(ctx)
			if err != nil || fresh.OKCount != 1 || fresh.UnknownCount != 0 {
				t.Fatalf("new generation cannot become healthy: %+v, %v", fresh, err)
			}
		})
	}
}

func TestReloadReconcilesProviderGeneration(t *testing.T) {
	app := testApplication(t)
	t.Chdir(t.TempDir())
	t.Setenv("PROVIDER_1_ID", "production")
	t.Setenv("PROVIDER_1_BASE_URL", "https://old.invalid/v1")
	t.Setenv("PROVIDER_1_API_KEY", "fixture-secret")
	t.Setenv("PROVIDER_1_MODELS", "fixture")
	loaded, err := config.Load(".env")
	if err != nil {
		t.Fatal(err)
	}
	app.baseCfg, app.cfg = loaded, loaded
	seedProviderStatus(t, app)
	before := app.cfg.Providers[0].ConnectionRevision
	if _, err := app.ReloadConfig(context.Background()); err != nil {
		t.Fatal(err)
	}
	if app.cfg.Providers[0].ConnectionRevision != before {
		t.Fatal("unchanged reload rotated the generation")
	}
	t.Setenv("PROVIDER_1_BASE_URL", "https://changed.invalid/v1")
	if _, err := app.ReloadConfig(context.Background()); err != nil {
		t.Fatal(err)
	}
	latest, err := app.store.LatestReport(context.Background())
	if err != nil || latest.OKCount != 0 || latest.UnknownCount != 1 {
		t.Fatalf("reload kept old status: %+v, %v", latest, err)
	}
}
