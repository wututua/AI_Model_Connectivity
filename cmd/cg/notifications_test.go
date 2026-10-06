package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/notify"
	"cg/internal/report"
	"cg/internal/storage"
	"cg/internal/web"
)

func TestNotificationLifecycle(t *testing.T) {
	app := testApplication(t)
	ctx := context.Background()
	var calls atomic.Int32
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(502)
		w.Write([]byte("secret-receipt"))
	}))
	defer old.Close()
	app.cfg.NotifyWebhookURL = old.URL + "?secret=key"
	state := notify.State{Status: "error", SentAt: time.Now().UTC().Truncate(time.Second)}
	if err := app.store.WriteNotifyState(ctx, state); err != nil {
		t.Fatal(err)
	}
	value, err := app.SendNotification(ctx, 0)
	if err != nil || value.Status != "error" || value.Kind != "test" || value.HTTPStatus != 502 {
		t.Fatalf("failed attempt not returned: %+v %v", value, err)
	}
	retriedBody := make(chan string, 1)
	next := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		retriedBody <- body.Text
	}))
	defer next.Close()
	app.cfg.NotifyWebhookURL = next.URL
	retried, err := app.SendNotification(ctx, value.ID)
	if err != nil || retried.Status != "success" || retried.RetryOf != value.ID || calls.Load() != 1 {
		t.Fatalf("retry did not use current saved target: %+v %v", retried, err)
	}
	retriedText := <-retriedBody
	if !strings.Contains(retriedText, "历史通知") || !strings.Contains(retriedText, "不代表当前模型状态") {
		t.Fatal("historical replay is not labeled")
	}
	current, err := app.store.ReadNotifyState(ctx)
	if err != nil || current != state {
		t.Fatalf("manual notification changed alert state: %+v %v", current, err)
	}
	original, err := app.store.GetDelivery(ctx, value.ID)
	if err != nil || original.Status != "error" {
		t.Fatal("retry rewrote original history")
	}
	records, err := app.store.ListDeliveries(ctx, notify.DeliveryQuery{})
	if err != nil || len(records) != 2 {
		t.Fatal("attempt history incomplete")
	}
	data, _ := json.Marshal(records)
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), old.URL) || strings.Contains(string(data), next.URL) {
		t.Fatal("sensitive data in history")
	}
	tasks, _ := app.store.ListCheckTasks(ctx, storage.TaskQuery{})
	if len(tasks) != 0 {
		t.Fatal("notification test started a model check")
	}
	if _, err := app.SendNotification(ctx, retried.ID); !errors.Is(err, notify.ErrNotRetryable) {
		t.Fatal("successful notification retried")
	}
}

func TestManualNotificationConcurrencyAndShutdown(t *testing.T) {
	app := testApplication(t)
	app.notificationMu.Lock()
	_, err := app.SendNotification(context.Background(), 0)
	app.notificationMu.Unlock()
	if !errors.Is(err, web.ErrNotificationBusy) {
		t.Fatal("concurrent manual send was allowed")
	}
	app.shuttingDown = true
	if _, err := app.SendNotification(context.Background(), 0); !errors.Is(err, web.ErrShuttingDown) {
		t.Fatal("send accepted during shutdown")
	}
}

func TestAutomaticNotificationHistory(t *testing.T) {
	app := testApplication(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	app.cfg.NotifyWebhookURL = server.URL
	if err := app.notificationClient(app.cfg).SendIfNeeded(context.Background(), report.Report{ErrorCount: 1}); err != nil {
		t.Fatal(err)
	}
	values, err := app.store.ListDeliveries(context.Background(), notify.DeliveryQuery{})
	if err != nil || len(values) != 1 || values[0].Kind != "alert" || values[0].Status != "success" {
		t.Fatalf("automatic alert not recorded: %+v %v", values, err)
	}
}
