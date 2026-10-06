package notify

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/report"
)

func TestDebounceRecoveryAndMaintenance(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	cfg := config.Config{NotifyWebhookURL: server.URL, NotifyOnRecovery: true,
		OperationsSettings: config.OperationsSettings{NotifyFailureThreshold: 2, NotifyRecoveryThreshold: 2}}
	state := &memoryState{state: State{Status: "ok"}}
	send := func(value report.Report) {
		t.Helper()
		if err := New(cfg, state).SendIfNeeded(context.Background(), value); err != nil {
			t.Fatal(err)
		}
	}
	good, bad := report.Report{OKCount: 1}, report.Report{ErrorCount: 1}
	send(bad)
	if requests.Load() != 0 || state.state.Consecutive != 1 {
		t.Fatal("early failure alert")
	}
	send(good)
	send(bad)
	if requests.Load() != 0 {
		t.Fatal("nonconsecutive failures counted")
	}
	send(bad)
	if requests.Load() != 1 || state.state.Status != "error" {
		t.Fatal("failure threshold did not send")
	}
	send(good)
	send(report.Report{})
	send(good)
	if requests.Load() != 1 {
		t.Fatal("unknown report counted toward recovery")
	}
	send(good)
	if requests.Load() != 2 || state.state.Status != "ok" {
		t.Fatal("recovery threshold did not send")
	}
	cfg.MaintenanceStart = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	cfg.MaintenanceEnd = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	send(bad)
	send(bad)
	if requests.Load() != 2 || state.state.Status != "ok" {
		t.Fatal("maintenance did not silence")
	}
	client := New(cfg, state)
	if _, err := client.SendTest(context.Background()); err != nil || requests.Load() != 3 {
		t.Fatal("maintenance blocked manual validation", err)
	}
	cfg.MaintenanceStart, cfg.MaintenanceEnd = "", ""
	send(bad)
	send(bad)
	if requests.Load() != 4 {
		t.Fatal("alert not sent after maintenance")
	}
}

func TestDebounceResetsAfterDetectionScopeChange(t *testing.T) {
	for _, change := range []struct {
		name string
		edit func(*config.Config)
	}{
		{"models", func(c *config.Config) { c.Providers[0].Models = []string{"new-model"} }},
		{"model order", func(c *config.Config) { c.Providers[0].Models = []string{"b", "a"} }},
		{"automatic discovery", func(c *config.Config) { c.Providers[0].Models = nil }},
		{"skip models", func(c *config.Config) { c.SkipModels = []string{"b"} }},
		{"model limit", func(c *config.Config) { c.MaxModelsPerProvider = 1 }},
		{"discovery limit", func(c *config.Config) { c.DiscoveryModelLimit = 5 }},
		{"provider name", func(c *config.Config) { c.Providers[0].Name = "renamed" }},
	} {
		for _, recovery := range []bool{false, true} {
			t.Run(change.name+map[bool]string{true: "/recovery", false: "/failure"}[recovery], func(t *testing.T) {
				var sent atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { sent.Add(1) }))
				defer server.Close()
				cfg := config.Config{
					NotifyWebhookURL: server.URL, NotifyOnRecovery: true,
					OperationsSettings: config.OperationsSettings{NotifyFailureThreshold: 2, NotifyRecoveryThreshold: 2},
					Providers: []config.ProviderConfig{{
						ID: "p", Name: "provider", ConnectionRevision: "unchanged",
						Models: []string{"a", "b"}, Enabled: true, ProbeEnabled: true,
					}},
				}
				state := &memoryState{state: State{Status: "ok"}}
				value := report.Report{ErrorCount: 1}
				if recovery {
					state.state.Status, value = "error", report.Report{OKCount: 1}
				}
				send := func() {
					t.Helper()
					if err := New(cfg, state).SendIfNeeded(context.Background(), value); err != nil {
						t.Fatal(err)
					}
				}
				send()
				if state.state.Consecutive != 1 || sent.Load() != 0 {
					t.Fatal("incorrect first observation", state.state)
				}
				oldScope := state.state.CandidateScope
				change.edit(&cfg)
				send()
				if sent.Load() != 0 || state.state.Consecutive != 1 || state.state.CandidateScope == oldScope {
					t.Fatal("new scope reused old evidence", state.state)
				}
				send()
				if sent.Load() != 1 || state.state.Candidate != "" || state.state.Consecutive != 0 {
					t.Fatal("new scope did not reach threshold", state.state)
				}
			})
		}
	}
}

func TestEvidenceScopeIgnoresUnrelatedMetadataAndSecrets(t *testing.T) {
	cfg := config.Config{Providers: []config.ProviderConfig{{
		ID: "p", Name: "provider", ConnectionRevision: "same", Models: []string{"a"},
		Enabled: true, ProbeEnabled: true,
	}}}
	scope := evidenceScope(cfg)
	cfg.DashboardTitle = "new title"
	cfg.Providers[0].Group = "production"
	cfg.Providers[0].Tags = []string{"primary"}
	cfg.Providers[0].APIKey = "not part of the scope hash"
	cfg.NotifyWebhookURL = "https://example.invalid/secret"
	if evidenceScope(cfg) != scope {
		t.Fatal("display metadata or raw secrets changed the scope")
	}
}

func TestDisabledNotificationsClearDebounceWithoutAdvancingState(t *testing.T) {
	for _, disabled := range []struct {
		name string
		edit func(*config.Config)
	}{
		{"disabled platform", func(c *config.Config) { c.NotifyPlatform = "disabled" }},
		{"missing webhook", func(c *config.Config) { c.NotifyWebhookURL = "" }},
		{"incomplete telegram", func(c *config.Config) { c.NotifyPlatform = "telegram"; c.NotifyTelegramBotToken = "fixture" }},
	} {
		for _, recovery := range []bool{false, true} {
			t.Run(disabled.name+map[bool]string{true: "/recovery", false: "/failure"}[recovery], func(t *testing.T) {
				var sent atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { sent.Add(1) }))
				defer server.Close()
				cfg := config.Config{
					NotifyPlatform: "webhook", NotifyWebhookURL: server.URL, NotifyOnRecovery: true,
					OperationsSettings: config.OperationsSettings{NotifyFailureThreshold: 2, NotifyRecoveryThreshold: 2},
				}
				before := State{Status: "ok", SentAt: time.Now().Add(-time.Hour)}
				value, interruption := report.Report{ErrorCount: 1}, report.Report{OKCount: 1}
				if recovery {
					before.Status, value, interruption = "error", interruption, value
				}
				state := &memoryState{state: before}
				send := func(settings config.Config, observation report.Report) {
					t.Helper()
					if err := New(settings, state).SendIfNeeded(context.Background(), observation); err != nil {
						t.Fatal(err)
					}
				}
				send(cfg, value)
				if state.state.Consecutive != 1 {
					t.Fatal("missing candidate")
				}
				muted := cfg
				disabled.edit(&muted)
				send(muted, interruption)
				if state.state != before || sent.Load() != 0 {
					t.Fatal("disabled observation did not only clear candidate", state.state)
				}
				send(cfg, value)
				if state.state.Consecutive != 1 || sent.Load() != 0 {
					t.Fatal("old observations counted after re-enabling", state.state)
				}
				send(cfg, value)
				if sent.Load() != 1 {
					t.Fatal("notification did not resume at threshold")
				}
			})
		}
	}
}

func TestObsoleteCheckResetsFailureAndRecoveryEvidence(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		for _, threshold := range []int{1, 2} {
			name := "failure"
			if recovery {
				name = "recovery"
			}
			t.Run(fmt.Sprintf("%s/threshold%d", name, threshold), func(t *testing.T) {
				var sent atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { sent.Add(1) }))
				defer server.Close()
				cfg := config.Config{
					NotifyWebhookURL: server.URL, NotifyOnRecovery: true,
					OperationsSettings: config.OperationsSettings{NotifyFailureThreshold: threshold, NotifyRecoveryThreshold: threshold},
					Providers:          []config.ProviderConfig{{ID: "p", Models: []string{"new"}, Enabled: true, ProbeEnabled: true}},
				}
				checked := cfg
				checked.Providers = []config.ProviderConfig{{ID: "p", Models: []string{"old"}, Enabled: true, ProbeEnabled: true}}
				before := State{Status: "ok", SentAt: time.Now().Add(-time.Hour)}
				value := report.Report{ErrorCount: 1}
				candidate := "error"
				if recovery {
					before.Status, candidate, value = "error", "ok", report.Report{OKCount: 1}
				}
				state := &memoryState{state: before}
				state.state.Candidate, state.state.Consecutive = candidate, 1
				state.state.CandidateScope = evidenceScope(checked)
				if err := New(cfg, state).SendCheckIfNeeded(context.Background(), value, checked); err != nil {
					t.Fatal(err)
				}
				if sent.Load() != 0 || state.state != before {
					t.Fatal("obsolete check changed sent state or retained candidates", state.state)
				}
				for i := 1; i <= threshold; i++ {
					if err := New(cfg, state).SendCheckIfNeeded(context.Background(), value, cfg); err != nil {
						t.Fatal(err)
					}
					if i < threshold && (sent.Load() != 0 || state.state.Consecutive != i) {
						t.Fatal("obsolete observation counted toward the threshold", state.state)
					}
				}
				if sent.Load() != 1 || state.state.Status != candidate {
					t.Fatal("fresh checks did not notify", state.state)
				}
			})
		}
	}
}

func TestCheckMetadataChangeStillNotifies(t *testing.T) {
	var sent atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { sent.Add(1) }))
	defer server.Close()
	checked := config.Config{NotifyWebhookURL: server.URL, Providers: []config.ProviderConfig{{
		ID: "p", Models: []string{"m"}, Enabled: true, ProbeEnabled: true,
	}}}
	current := checked
	current.DashboardTitle = "Updated title"
	current.Providers = append([]config.ProviderConfig(nil), checked.Providers...)
	current.Providers[0].Group, current.Providers[0].Tags = "production", []string{"primary"}
	state := &memoryState{state: State{Status: "ok"}}
	if err := New(current, state).SendCheckIfNeeded(context.Background(), report.Report{ErrorCount: 1}, checked); err != nil {
		t.Fatal(err)
	}
	if sent.Load() != 1 || state.state.Status != "error" {
		t.Fatal("cosmetic edit discarded valid observations", state.state)
	}
}
