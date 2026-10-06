package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"cg/internal/auth"
	"cg/internal/config"
	"cg/internal/storage"
	"cg/internal/web"
)

func TestHTTPAcceptedTaskIntegration(t *testing.T) {
	app := testApplication(t)
	started, release := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
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
	app.cfg.Providers = []config.ProviderConfig{{ID: "p1", Name: "Local fixture", Enabled: true, ProbeEnabled: true, BaseURL: upstream.URL, Models: []string{"fixture"}}}
	hash, err := auth.HashPassword("Integration123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.store.CreateUser(context.Background(), storage.User{Username: "admin", Role: "admin", Enabled: true, PasswordHash: hash}, false); err != nil {
		t.Fatal(err)
	}
	service := httptest.NewServer(web.NewServer(app.cfg, app.store, app.check, nil, app).Handler())
	defer service.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 2 * time.Second}
	login, err := client.Post(service.URL+"/api/auth/login", "application/json", strings.NewReader(`{"username":"admin","password":"Integration123"}`))
	if err != nil {
		t.Fatal(err)
	}
	var session struct {
		CSRF string `json:"csrf_token"`
	}
	err = json.NewDecoder(login.Body).Decode(&session)
	login.Body.Close()
	if err != nil || login.StatusCode != 200 || session.CSRF == "" {
		t.Fatalf("login failed: %d, %v", login.StatusCode, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, service.URL+"/api/admin/check", nil)
	req.Header.Set("X-CSRF-Token", session.CSRF)
	response, err := client.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	var accepted struct {
		Task storage.CheckTask `json:"task"`
	}
	err = json.NewDecoder(response.Body).Decode(&accepted)
	response.Body.Close()
	cancel()
	client.CloseIdleConnections()
	if err != nil || response.StatusCode != 202 || accepted.Task.ID == 0 {
		t.Fatalf("not accepted: %d, %+v, %v", response.StatusCode, accepted, err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("accepted task did not start")
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		res, err := client.Get(service.URL + "/api/admin/tasks/" + strconv.FormatInt(accepted.Task.ID, 10))
		if err != nil {
			t.Fatal(err)
		}
		var task storage.CheckTask
		err = json.NewDecoder(res.Body).Decode(&task)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("task lookup failed: %d, %v", res.StatusCode, err)
		}
		if task.Status == "success" {
			if task.Total != 1 {
				t.Fatal("incorrect task total")
			}
			break
		}
		if task.Status != "running" || time.Now().After(deadline) {
			t.Fatalf("background task failed after HTTP disconnect: %+v", task)
		}
		time.Sleep(10 * time.Millisecond)
	}
	res, err := client.Get(service.URL + "/api/admin/tasks/999999")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("missing task returned %d", res.StatusCode)
	}
}
