package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/storage"
)

func TestDiscoverModelsUsesDraftAndNeverSavesOrProbes(t *testing.T) {
	app := testApplication(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer stored-secret" {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"data":[{"id":" model-a "},{"id":"model-b"},{"id":"model-a"},{"id":""}]}`))
	}))
	defer upstream.Close()
	existing := config.ProviderConfig{ID: "saved", BaseURL: upstream.URL + "/v1", APIKey: "stored-secret", Models: []string{"manual"}}
	app.cfg.Providers = []config.ProviderConfig{existing}
	if err := app.store.SaveRuntimeConfig(context.Background(), config.RuntimeConfigFromConfig(app.cfg)); err != nil {
		t.Fatal(err)
	}
	models, err := app.DiscoverModels(context.Background(), config.ModelDiscoveryRequest{ProviderID: "saved", BaseURL: upstream.URL + "/v1/"})
	if err != nil || !reflect.DeepEqual(models, []string{"model-a", "model-b"}) || calls.Load() != 1 {
		t.Fatalf("discovery failed: %v, %v, calls=%d", models, err, calls.Load())
	}
	runtime, _, err := app.store.LoadRuntimeConfig(context.Background())
	if err != nil || !reflect.DeepEqual(runtime.Providers, []config.ProviderConfig{existing}) || !reflect.DeepEqual(app.currentConfig().Providers, runtime.Providers) {
		t.Fatal("discovery changed provider configuration")
	}
	tasks, err := app.ListTasks(context.Background(), storage.TaskQuery{})
	if err != nil || len(tasks) != 0 {
		t.Fatal("discovery created a detection task")
	}
}

func TestDiscoverModelsDraftKeyClearAndURLChange(t *testing.T) {
	app := testApplication(t)
	headers := make(chan string, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Get("Authorization")
		w.Write([]byte(`{"data":[]}`))
	}))
	defer upstream.Close()
	app.cfg.Providers = []config.ProviderConfig{{ID: "saved", BaseURL: "https://original.example/v1", APIKey: "old-secret"}}
	query := config.ModelDiscoveryRequest{ProviderID: "saved", BaseURL: upstream.URL}
	if _, err := app.DiscoverModels(context.Background(), query); err == nil {
		t.Fatal("stored key silently sent to a changed URL")
	}
	for _, test := range []struct {
		key   string
		clear bool
		want  string
	}{{"new-secret", false, "Bearer new-secret"}, {"", true, ""}, {"ignored", true, ""}} {
		query.APIKey, query.ClearAPIKey = test.key, test.clear
		models, err := app.DiscoverModels(context.Background(), query)
		if err != nil || models == nil || len(models) != 0 {
			t.Fatalf("empty discovery: %v %v", models, err)
		}
		if got := <-headers; got != test.want {
			t.Fatalf("authorization=%q, want %q", got, test.want)
		}
	}
	query.ProviderID, query.APIKey, query.ClearAPIKey = "", "draft-secret", false
	if _, err := app.DiscoverModels(context.Background(), query); err != nil || <-headers != "Bearer draft-secret" {
		t.Fatal("unsaved provider cannot discover models")
	}
}

func TestDiscoverModelsValidationRedactionAndCancellation(t *testing.T) {
	app := testApplication(t)
	for _, query := range []config.ModelDiscoveryRequest{
		{BaseURL: ""},
		{BaseURL: "file:///etc/passwd"},
		{BaseURL: "http://169.254.169.254/latest"},
		{BaseURL: "http://user:password@localhost/v1"},
		{BaseURL: "http://localhost/v1?key=secret"},
		{ProviderID: "missing", BaseURL: "http://localhost/v1"},
	} {
		if _, err := app.DiscoverModels(context.Background(), query); err == nil {
			t.Fatalf("invalid discovery accepted: %+v", query)
		}
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow/models" {
			<-r.Context().Done()
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"rejected private-api-key"}}`))
	}))
	defer upstream.Close()
	_, err := app.DiscoverModels(context.Background(), config.ModelDiscoveryRequest{BaseURL: upstream.URL, APIKey: "private-api-key"})
	if err == nil || strings.Contains(err.Error(), "private-api-key") || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("unsafe discovery error: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := app.DiscoverModels(ctx, config.ModelDiscoveryRequest{BaseURL: upstream.URL + "/slow"}); err == nil || time.Since(start) > 3*time.Second {
		t.Fatal("discovery did not respect cancellation")
	}
}
