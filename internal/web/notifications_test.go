package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"cg/internal/notify"
)

type notificationAdmin struct {
	stubAdmin
	err   error
	calls int
	id    int64
}

func (a *notificationAdmin) SendNotification(_ context.Context, id int64) (notify.Delivery, error) {
	a.calls++
	a.id = id
	return notify.Delivery{ID: 9, Status: "error", HTTPStatus: 503, ErrorMessage: "platform unavailable"}, a.err
}

func TestNotificationAccessAndCSRF(t *testing.T) {
	s, _ := newTestServer(t)
	admin := &notificationAdmin{}
	s.admin = admin
	for _, endpoint := range []struct{ method, path string }{
		{"GET", "/api/admin/notifications"},
		{"POST", "/api/admin/notifications/test"},
		{"POST", "/api/admin/notifications/1/retry"},
	} {
		for role, status := range map[string]int{"": 401, "user": 403, "admin": 200} {
			if rec := perform(s, endpoint.method, endpoint.path, "", role); rec.Code != status {
				t.Fatalf("%s %s %s: %d %s", endpoint.method, endpoint.path, role, rec.Code, rec.Body)
			}
		}
		if endpoint.method == "POST" {
			req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			authenticateTestRequest(req, false)
			req.Header.Del("X-CSRF-Token")
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != 403 {
				t.Fatal("missing CSRF allowed")
			}
			req = httptest.NewRequest(endpoint.method, endpoint.path, nil)
			authenticateTestRequest(req, false)
			req.Header.Set("Origin", "https://attacker.invalid")
			rec = httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != 403 {
				t.Fatal("cross-site request allowed")
			}
		}
	}
	if admin.calls != 2 {
		t.Fatalf("unauthorized request reached sender: %d", admin.calls)
	}
}

func TestNotificationHTTPValidation(t *testing.T) {
	s, _ := newTestServer(t)
	admin := &notificationAdmin{}
	s.admin = admin
	for _, query := range []string{"limit=0", "limit=201", "limit=abc", "offset=-1", "offset=x", "status=failed"} {
		if rec := perform(s, "GET", "/api/admin/notifications?"+query, "", "admin"); rec.Code != 400 {
			t.Fatalf("invalid query accepted: %s", query)
		}
	}
	for _, path := range []string{"x/retry", "0/retry", "-1/retry", "1/other", "1/retry/extra"} {
		if rec := perform(s, "POST", "/api/admin/notifications/"+path, "", "admin"); rec.Code != 400 {
			t.Fatalf("invalid path accepted: %s", path)
		}
	}
	for _, endpoint := range []struct{ method, path string }{
		{"POST", "/api/admin/notifications"},
		{"GET", "/api/admin/notifications/test"},
		{"DELETE", "/api/admin/notifications/1/retry"},
	} {
		if rec := perform(s, endpoint.method, endpoint.path, "", "admin"); rec.Code != 405 {
			t.Fatal("unsupported method accepted")
		}
	}
	for _, tc := range []struct {
		err    error
		status int
	}{
		{nil, 200}, {sql.ErrNoRows, 404}, {notify.ErrNotConfigured, 400},
		{notify.ErrNotRetryable, 409}, {ErrNotificationBusy, 409},
		{ErrShuttingDown, 503}, {errors.New("secret-storage-details"), 500},
	} {
		admin.err = tc.err
		rec := perform(s, "POST", "/api/admin/notifications/42/retry", "", "admin")
		if rec.Code != tc.status || admin.id != 42 || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body)
		}
		if tc.status == 500 && strings.Contains(rec.Body.String(), "secret-storage-details") {
			t.Fatal("internal error leaked")
		}
	}
}
