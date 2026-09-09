package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
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
