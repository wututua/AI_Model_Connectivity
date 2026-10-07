package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cg/internal/storage"
)

func TestAuditAttributionPermissionsAndSecretExclusion(t *testing.T) {
	s, store := newTestServer(t)
	s.admin = stubAdmin{}
	for _, role := range []string{"", "user", "admin"} {
		rec := perform(s, "POST", "/api/admin/metrics-tokens", `{"name":"secret-marker-not-for-audit"}`, role)
		want := map[string]int{"": 401, "user": 403, "admin": 201}[role]
		if rec.Code != want {
			t.Fatal(rec.Code, rec.Body)
		}
	}
	req := httptest.NewRequest("POST", "/api/admin/metrics-tokens", strings.NewReader(`{"name":"csrf"}`))
	authenticateTestRequest(req, false)
	req.Header.Del("X-CSRF-Token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatal(rec.Code)
	}
	login := perform(s, "POST", "/api/auth/login", `{"username":"admin","password":"TestPass1"}`, "")
	if login.Code != 200 {
		t.Fatal(login.Code)
	}
	failed := perform(s, "POST", "/api/auth/login", `{"username":"secret-marker-not-for-audit","password":"password-marker-not-for-audit"}`, "")
	if failed.Code != 401 {
		t.Fatal(failed.Code)
	}
	if logout := perform(s, "POST", "/api/auth/logout", "", "user"); logout.Code != 200 {
		t.Fatal(logout.Code)
	}
	page, err := store.QueryAudit(context.Background(), storage.AuditQuery{})
	if err != nil || len(page.Items) != 7 {
		t.Fatal(page, err)
	}
	wantActors := []string{"user", "", "admin", "admin", "admin", "user", ""}
	for i, event := range page.Items {
		if event.Actor != wantActors[i] || (event.ActorID == 0) != (event.Actor == "") {
			t.Fatal("incorrect actor attribution", i, event)
		}
	}
	encoded, _ := json.Marshal(page)
	for _, secret := range []string{"secret-marker", "password-marker", "TestPass1", "test-csrf", "cgm_", login.Result().Cookies()[0].Value} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("audit contains credential or request data")
		}
	}
	for _, role := range []string{"", "user", "admin"} {
		rec := perform(s, "GET", "/api/admin/audit?result=denied&limit=2", "", role)
		// Logout revoked the user's session.
		want := map[string]int{"": 401, "user": 401, "admin": 200}[role]
		if rec.Code != want {
			t.Fatal(role, rec.Code, rec.Body)
		}
		if role == "admin" && (!strings.Contains(rec.Body.String(), `"has_more":true`) || !strings.Contains(rec.Body.String(), `"actions":`)) {
			t.Fatal(rec.Body)
		}
	}
	for _, query := range []string{"limit=0", "before=-2", "action=raw", "result=unknown", "start=invalid"} {
		if rec := perform(s, "GET", "/api/admin/audit?"+query, "", "admin"); rec.Code != 400 {
			t.Fatal(query, rec.Code)
		}
	}
	after, _ := store.QueryAudit(context.Background(), storage.AuditQuery{})
	if len(after.Items) != len(page.Items) {
		t.Fatal("reads generated audit noise")
	}
}

func TestAuditCanceledRequestsAndOrdinaryUserRead(t *testing.T) {
	s, store := newTestServer(t)
	if rec := perform(s, "GET", "/api/admin/audit", "", "user"); rec.Code != 403 {
		t.Fatal("ordinary user read audit", rec.Code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("POST", "/api/admin/detection/start", nil).WithContext(ctx)
	authenticateTestRequest(req, false)
	rec := httptest.NewRecorder()
	s.serveAudited(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.requireAdmin(w, r) {
			t.Fatal("fixture authentication failed")
		}
		cancel()
		w.WriteHeader(202)
	}), rec, req)
	page, err := store.QueryAudit(context.Background(), storage.AuditQuery{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Result != "accepted" || page.Items[0].Actor != "admin" {
		t.Fatal("cancellation lost completed request audit", page, err)
	}
}

func TestAuditAllowlist(t *testing.T) {
	for _, path := range []string{"/health", "/api/status", "/api/events", "/api/admin/updates", "/api/admin/audit", "/api/admin/providers"} {
		if auditAction(httptest.NewRequest("GET", path, nil)) != "" {
			t.Fatal("ordinary read audited", path)
		}
	}
	for _, tc := range []struct{ method, path, action string }{
		{"PUT", "/api/admin/providers/p", "providers.update"},
		{"DELETE", "/api/admin/users/2", "users.delete"},
		{"POST", "/api/admin/metrics-tokens/1/rotate", "metrics.rotate"},
		{"POST", "/api/admin/notifications/1/retry", "notifications.retry"},
		{"POST", "/api/admin/providers/p/rerun", "detection.provider"},
		{"GET", "/api/admin/config/export", "config.export"},
		{"POST", "/api/admin/updates/start", "updates.start"},
	} {
		if got := auditAction(httptest.NewRequest(tc.method, tc.path, nil)); got != tc.action {
			t.Fatal(tc, got)
		}
	}
}
