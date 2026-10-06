package notify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/report"
)

type memoryDeliveries struct {
	created   []Delivery
	finished  []Delivery
	createErr error
	finishErr error
	started   chan Delivery
}

func (m *memoryDeliveries) CreateDelivery(_ context.Context, value Delivery) (Delivery, error) {
	if m.createErr != nil {
		return Delivery{}, m.createErr
	}
	value.ID = int64(len(m.created) + 1)
	m.created = append(m.created, value)
	if m.started != nil {
		m.started <- value
	}
	return value, nil
}

func (m *memoryDeliveries) FinishDelivery(ctx context.Context, value Delivery) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.finished = append(m.finished, value)
	return m.finishErr
}

func TestManualNotificationsPreserveAlertState(t *testing.T) {
	for _, status := range []int{200, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			history := &memoryDeliveries{started: make(chan Delivery, 1)}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if (<-history.started).ID == 0 {
					t.Error("outbound request was not recorded first")
				}
				w.WriteHeader(status)
				w.Write([]byte("secret-platform-receipt"))
			}))
			defer server.Close()
			state := State{Status: "error", SentAt: time.Now()}
			store := &memoryState{state: state}
			client := New(config.Config{NotifyWebhookURL: server.URL + "?key=secret-key", NotifyCooldownMinutes: 60}, store)
			client.SetHistory(history)
			value, err := client.SendTest(context.Background())
			if (err == nil) != (status == 200) || value.HTTPStatus != status || value.ID == 0 {
				t.Fatalf("unexpected result: %+v, %v", value, err)
			}
			if store.state != state || len(history.finished) != 1 || value.FinishedAt == "" {
				t.Fatal("manual test changed state or did not finish")
			}
			for _, entry := range history.finished {
				if strings.Contains(entry.Summary+entry.ErrorMessage, "secret") {
					t.Fatal("credentials or receipt leaked into history")
				}
			}
			value.Status = "error"
			retry, _ := client.Retry(context.Background(), value)
			if retry.RetryOf != value.ID || retry.Kind != "retry" || retry.ID == value.ID || store.state != state {
				t.Fatalf("retry mutated original state: %+v", retry)
			}
		})
	}
}

func TestNotificationHistoryRecordsOnlyAttemptedSends(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	history := &memoryDeliveries{}
	store := &memoryState{}
	client := New(config.Config{NotifyWebhookURL: server.URL}, store)
	client.SetHistory(history)
	for _, value := range []report.Report{{}, {OKCount: 1}, {OKCount: 1}, {ErrorCount: 1}, {ErrorCount: 1}} {
		if err := client.SendIfNeeded(context.Background(), value); err != nil {
			t.Fatal(err)
		}
	}
	if len(history.created) != 1 || history.finished[0].Kind != "alert" || store.state.Status != "error" {
		t.Fatalf("skipped alerts entered history: %+v", history)
	}
}

func TestNotificationHistoryFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	store := &memoryState{state: State{Status: "ok"}}
	client := New(config.Config{NotifyWebhookURL: server.URL}, store)
	history := &memoryDeliveries{createErr: errors.New("storage failure with secret")}
	client.SetHistory(history)
	if _, err := client.SendTest(context.Background()); err == nil || requests.Load() != 0 || strings.Contains(err.Error(), "secret") {
		t.Fatal("sent without an audit record")
	}
	history.createErr, history.finishErr = nil, errors.New("storage failure with secret")
	err := client.SendIfNeeded(context.Background(), report.Report{ErrorCount: 1})
	if !errors.Is(err, ErrResultNotSaved) || requests.Load() != 1 || store.state.Status != "ok" {
		t.Fatalf("incomplete persistence advanced alert state: %v", err)
	}
}

func TestNotificationCanceledRequestStillFinishesHistory(t *testing.T) {
	history := &memoryDeliveries{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cancel() }))
	defer server.Close()
	client := New(config.Config{NotifyWebhookURL: server.URL}, nil)
	client.SetHistory(history)
	value, _ := client.SendTest(ctx)
	if value.ID == 0 || len(history.finished) != 1 || history.finished[0].FinishedAt == "" {
		t.Fatalf("cancellation lost history: %+v", history)
	}
}

func TestNotificationPreconditions(t *testing.T) {
	history := &memoryDeliveries{}
	for _, cfg := range []config.Config{{}, {NotifyPlatform: "disabled", NotifyWebhookURL: "https://example.invalid"}, {NotifyPlatform: "telegram", NotifyTelegramBotToken: "secret"}} {
		client := New(cfg, nil)
		client.SetHistory(history)
		if _, err := client.SendTest(context.Background()); !errors.Is(err, ErrNotConfigured) {
			t.Fatalf("incomplete configuration accepted: %v", err)
		}
	}
	client := New(config.Config{NotifyWebhookURL: "https://example.invalid"}, nil)
	client.SetHistory(history)
	for _, status := range []string{"success", "sending", ""} {
		if _, err := client.Retry(context.Background(), Delivery{ID: 1, Status: status}); !errors.Is(err, ErrNotRetryable) {
			t.Fatalf("unexpected retry acceptance: %s", status)
		}
	}
	if len(history.created) != 0 {
		t.Fatal("precondition failures created delivery attempts")
	}
}

func TestNotificationReceiptHistoryIsSanitized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode":93000,"errmsg":"secret-token-and-private-details"}`))
	}))
	defer server.Close()
	client := New(config.Config{NotifyPlatform: "wecom", NotifyWebhookURL: server.URL}, nil)
	history := &memoryDeliveries{}
	client.SetHistory(history)
	value, err := client.SendTest(context.Background())
	if err == nil || value.Status != "error" || value.HTTPStatus != 200 || strings.Contains(value.ErrorMessage, "secret") {
		t.Fatalf("platform receipt not safely recorded: %+v %v", value, err)
	}
}
