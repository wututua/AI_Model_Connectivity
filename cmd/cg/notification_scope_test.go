package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/notify"
)

func TestFullCheckScopeChangeDiscardsAlertEvidence(t *testing.T) {
	for _, change := range []string{"replace model", "add model", "expand skips", "expand limit", "endpoint ABA"} {
		for _, threshold := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/threshold%d", change, threshold), func(t *testing.T) {
				app := testApplication(t)
				ctx := context.Background()
				started, release := make(chan struct{}), make(chan struct{})
				var released sync.Once
				var probes, notifications atomic.Int32
				var fail atomic.Bool
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/notify" {
						notifications.Add(1)
						return
					}
					if probes.Add(1) == 1 {
						close(started)
						<-release
					}
					if fail.Load() {
						http.Error(w, `{"error":{"message":"probe failed"}}`, http.StatusServiceUnavailable)
						return
					}
					fmt.Fprint(w, `{"choices":[{"message":{"content":"pang"}}],"usage":{"total_tokens":3}}`)
				}))
				defer upstream.Close()
				defer released.Do(func() { close(release) })
				app.cfg.Providers = []config.ProviderConfig{{
					ID: "p", Name: "Provider", Type: "openai", BaseURL: upstream.URL,
					Models: []string{"old-model"}, Enabled: true, ProbeEnabled: true,
				}}
				if change == "expand skips" || change == "expand limit" {
					app.cfg.Providers[0].Models = []string{"old-model", "new-model"}
					if change == "expand skips" {
						app.cfg.SkipModels = []string{"new-model"}
					} else {
						app.cfg.MaxModelsPerProvider = 1
					}
				}
				app.cfg.NotifyPlatform, app.cfg.NotifyWebhookURL = "webhook", upstream.URL+"/notify"
				app.cfg.NotifyOnRecovery = true
				app.cfg.NotifyFailureThreshold = threshold
				if err := app.replaceRuntimeConfig(ctx, config.RuntimeConfigFromConfig(app.cfg)); err != nil {
					t.Fatal(err)
				}
				sentAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
				if err := app.store.WriteNotifyState(ctx, notify.State{
					Status: "ok", SentAt: sentAt, Candidate: "error", Consecutive: 1, CandidateScope: "old-scope",
				}); err != nil {
					t.Fatal(err)
				}
				task, err := app.StartCheck(ctx, "")
				if err != nil {
					t.Fatal(err)
				}
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					t.Fatal("probe did not start")
				}
				cfg := app.currentConfig()
				switch change {
				case "replace model", "add model":
					draft := providerDraft(cfg.Providers[0])
					draft.Models = []string{"new-model"}
					if change == "add model" {
						draft.Models = []string{"old-model", "new-model"}
					}
					_, err = app.UpsertProvider(ctx, "p", draft)
				case "expand skips", "expand limit":
					settings := config.SettingsFromConfig(cfg)
					settings.SkipModels, settings.MaxModelsPerProvider = nil, 0
					_, err = app.UpdateSettings(ctx, settings)
				case "endpoint ABA":
					draft := providerDraft(cfg.Providers[0])
					draft.BaseURL += "/changed"
					if _, err = app.UpsertProvider(ctx, "p", draft); err == nil {
						_, err = app.UpsertProvider(ctx, "p", providerDraft(cfg.Providers[0]))
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				released.Do(func() { close(release) })
				done := awaitTask(t, app, task.ID)
				if done.Status != "success" || done.OKCount != 1 || done.ErrorCount != 0 || probes.Load() != 1 {
					t.Fatal("old-scope check did not succeed", done)
				}
				latest, err := app.store.LatestReport(ctx)
				if err != nil || latest.UnknownCount != 1 {
					t.Fatal("current scope lost its unchecked model", latest, err)
				}
				state, err := app.store.ReadNotifyState(ctx)
				if err != nil || state != (notify.State{Status: "ok", SentAt: sentAt}) || notifications.Load() != 0 {
					t.Fatal("obsolete check sent an alert or retained candidate evidence", state, err)
				}
				billing, err := app.store.LoadBillingSummary(ctx, 1)
				if err != nil || billing.TotalProbeCount != 1 || billing.TotalTokens != 3 {
					t.Fatal("completed usage lost", billing, err)
				}
				history, err := app.store.LoadHistory(ctx, 100, 7)
				if err != nil || len(history["p::old-model"]) != 1 {
					t.Fatal("completed historical sample lost", history, err)
				}

				fail.Store(true)
				for observation := 1; observation <= threshold; observation++ {
					fresh, err := app.check(ctx)
					if err != nil || fresh.Total == 0 || fresh.ErrorCount != fresh.Total {
						t.Fatal("fresh check did not measure the current scope", fresh, err)
					}
					state, err = app.store.ReadNotifyState(ctx)
					if err != nil {
						t.Fatal(err)
					}
					if observation < threshold {
						if notifications.Load() != 0 || state.Consecutive != observation {
							t.Fatal("old check counted toward the new threshold", state)
						}
					} else if notifications.Load() != 1 || state.Status != "error" || state.Consecutive != 0 {
						t.Fatal("fresh observations did not trigger the alert", state)
					}
				}
			})
		}
	}
}
