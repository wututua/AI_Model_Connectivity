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

func TestImportReconcilesProviderGeneration(t *testing.T) {
	app := testApplication(t)
	app.cfg.Providers = []config.ProviderConfig{fixtureProvider()}
	seedProviderStatus(t, app)
	before := app.cfg.Providers[0].ConnectionRevision
	value := config.ConfigImport{
		Settings:  config.SettingsFromConfig(app.cfg),
		Providers: []config.ProviderUpdate{providerDraft(app.cfg.Providers[0])},
	}
	if _, err := app.ImportConfig(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if app.cfg.Providers[0].ConnectionRevision != before {
		t.Fatal("unchanged import rotated the generation")
	}
	value.Providers[0].BaseURL = "https://changed.invalid/v1"
	value.Providers[0].APIKey = "explicit-replacement"
	if _, err := app.ImportConfig(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	latest, err := app.store.LatestReport(context.Background())
	if err != nil || latest.OKCount != 0 || latest.UnknownCount != 1 {
		t.Fatalf("import kept old status: %+v, %v", latest, err)
	}
}

func TestStartupEnvironmentPreservesSavedConfiguration(t *testing.T) {
	app := testApplication(t)
	ctx := context.Background()
	app.cfg.Providers = []config.ProviderConfig{fixtureProvider()}
	app.cfg.DashboardTitle = "Saved title"
	app.cfg.StatusLoginRequired = true
	seedProviderStatus(t, app)
	stored, ok, err := app.store.LoadRuntimeConfig(ctx)
	if err != nil || !ok {
		t.Fatal("saved configuration is missing")
	}
	t.Setenv("APP_PORT", "9091")
	t.Setenv("SECURE_COOKIES", "true")
	t.Setenv("DASHBOARD_TITLE", "New environment title")
	t.Setenv("STATUS_LOGIN_REQUIRED", "false")
	t.Setenv("PROVIDER_1_ID", "replacement")
	t.Setenv("PROVIDER_1_BASE_URL", "https://replacement.invalid/v1")
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	restarted := config.ApplyRuntimeConfig(loaded, stored)
	if !reflect.DeepEqual(restarted.Providers, app.cfg.Providers) ||
		!reflect.DeepEqual(config.SettingsFromConfig(restarted), config.SettingsFromConfig(app.cfg)) {
		t.Fatal("startup environment replaced saved providers or runtime settings")
	}
	if restarted.AppPort != 9091 || !restarted.SecureCookies {
		t.Fatal("saved runtime configuration replaced startup-only settings")
	}
	app.cfg, app.baseCfg = restarted, loaded
	handler := web.NewServer(restarted, app.store, app.check, nil, app).Handler()
	for _, path := range []string{"/api/status", "/api/events"} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
		cancel()
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("saved login requirement did not protect %s: %d", path, recorder.Code)
		}
	}
	for _, required := range []bool{false, true} {
		settings := config.SettingsFromConfig(app.currentConfig())
		settings.StatusLoginRequired = required
		if _, err := app.UpdateSettings(ctx, settings); err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/status", nil))
		want := http.StatusOK
		if required {
			want = http.StatusUnauthorized
		}
		if recorder.Code != want {
			t.Fatalf("runtime login update did not take effect: got %d, want %d", recorder.Code, want)
		}
	}
}
