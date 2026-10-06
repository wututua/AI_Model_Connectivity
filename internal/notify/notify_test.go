package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
	"cg/internal/report"
)

func TestWebhookDoesNotFollowRedirects(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalls.Add(1)
	}))
	defer target.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()

	client := New(config.Config{NotifyWebhookURL: server.URL}, nil)
	err := client.sendWebhook(context.Background(), payload{Text: "test"})
	if err == nil {
		t.Fatal("expected redirect response to be reported as an error")
	}
	if targetCalls.Load() != 0 {
		t.Fatal("webhook client followed redirect")
	}
}

func TestWebhookRejectsLinkLocalURL(t *testing.T) {
	client := New(config.Config{NotifyWebhookURL: "http://169.254.169.254/metadata"}, nil)
	if err := client.sendWebhook(context.Background(), payload{Text: "test"}); err == nil {
		t.Fatal("expected link-local webhook URL to be rejected")
	}
}

type memoryState struct{ state State }

func (store *memoryState) Read() (State, error)    { return store.state, nil }
func (store *memoryState) Write(state State) error { store.state = state; return nil }

func TestCooldownDoesNotDiscardPendingAlert(t *testing.T) {
	var sent atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { sent.Add(1) }))
	defer server.Close()
	store := &memoryState{state: State{Status: "ok", SentAt: time.Now()}}
	client := New(config.Config{NotifyWebhookURL: server.URL, NotifyCooldownMinutes: 10}, store)
	value := report.Report{ErrorCount: 1}
	if err := client.SendIfNeeded(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if store.state.Status != "ok" || sent.Load() != 0 {
		t.Fatal("cooldown discarded pending state")
	}
	store.state.SentAt = time.Now().Add(-11 * time.Minute)
	if err := client.SendIfNeeded(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if sent.Load() != 1 || store.state.Status != "error" {
		t.Fatal("pending alert not sent after cooldown")
	}
}

func TestDisabledNotificationRetainsCredentialsWithoutSending(t *testing.T) {
	var sent atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { sent.Add(1) }))
	defer server.Close()
	store := &memoryState{state: State{Status: "ok"}}
	client := New(config.Config{NotifyPlatform: "disabled", NotifyWebhookURL: server.URL}, store)
	if err := client.SendIfNeeded(context.Background(), report.Report{ErrorCount: 1}); err != nil {
		t.Fatal(err)
	}
	if sent.Load() != 0 || store.state.Status != "ok" {
		t.Fatal("disabled notification sent or modified state")
	}
	if !New(config.Config{NotifyWebhookURL: server.URL}, store).enabled() {
		t.Fatal("legacy empty platform must still use webhook")
	}
}

func TestProviderNameFilterPreservesDiscoveryFailure(t *testing.T) {
	value := report.Report{
		Providers:      []report.ProviderReport{{ProviderID: "p1", ProviderName: "Display Name", Status: "error"}},
		ProviderErrors: []probe.ProviderError{{ProviderID: "p1", Error: "unavailable"}},
	}
	filtered := filterReport(value, []string{"display name"}, nil)
	if len(filtered.ProviderErrors) != 1 || alertState(filtered) != "error" {
		t.Fatalf("filter lost failure: %+v", filtered)
	}
	var sent atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { sent.Add(1) }))
	defer server.Close()
	store := &memoryState{state: State{Status: "error"}}
	client := New(config.Config{NotifyWebhookURL: server.URL, NotifyProviders: []string{"Display Name"}, NotifyOnRecovery: true}, store)
	if err := client.SendIfNeeded(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if sent.Load() != 0 || store.state.Status != "error" {
		t.Fatal("discovery failure produced a false recovery")
	}
}

func TestPlatformReceipts(t *testing.T) {
	for _, test := range []struct {
		platform string
		body     string
		status   int
		ok       bool
	}{
		{"wecom", `{"errcode":0}`, 200, true},
		{"wechat_work", `{"errcode":93000}`, 200, false},
		{"dingtalk", `{"errcode":310000,"errmsg":"secret"}`, 200, false},
		{"dingtalk", `{"errcode":0}`, 200, true},
		{"telegram", `{"ok":true}`, 200, true},
		{"telegram", `{"ok":false}`, 200, false},
		{"bark", `{"code":200}`, 200, true},
		{"bark", `{"code":400}`, 200, false},
		{"wecom", `{}`, 200, false},
		{"wecom", `not-json`, 200, false},
		{"wecom", strings.Repeat(" ", 64<<10) + `{}`, 200, false},
		{"webhook", "", 204, true},
		{"discord", "", 204, true},
		{"webhook", "", 500, false},
	} {
		t.Run(test.platform+"/"+test.body[:min(len(test.body), 40)], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				w.Write([]byte(test.body))
			}))
			defer server.Close()
			client := New(config.Config{NotifyPlatform: test.platform}, nil)
			defer client.httpClient.CloseIdleConnections()
			request, _ := http.NewRequest(http.MethodPost, server.URL, nil)
			err := client.do(request)
			if (err == nil) != test.ok || err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatalf("unexpected receipt result: %v", err)
			}
		})
	}
}

func TestBusinessFailureRemainsPendingForRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Write([]byte(`{"errcode":93000,"errmsg":"invalid key"}`))
		} else {
			w.Write([]byte(`{"errcode":0}`))
		}
	}))
	defer server.Close()
	store := &memoryState{state: State{Status: "ok"}}
	client := New(config.Config{NotifyPlatform: "wecom", NotifyWebhookURL: server.URL}, store)
	value := report.Report{ErrorCount: 1}
	if err := client.SendIfNeeded(context.Background(), value); err == nil || store.state.Status != "ok" {
		t.Fatal("failed notification was marked as delivered")
	}
	if err := client.SendIfNeeded(context.Background(), value); err != nil || store.state.Status != "error" || calls.Load() != 2 {
		t.Fatalf("failed notification was not retried: %v", err)
	}
}
