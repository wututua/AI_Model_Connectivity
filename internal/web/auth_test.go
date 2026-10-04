package web

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cg/internal/config"
	"cg/internal/metrics"
	"cg/internal/storage"
)

func perform(s *Server, method, path, body string, role string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		authenticateTestRequest(req, role == "user")
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestLoginCookieAndNoPasswordLeaks(t *testing.T) {
	s, _ := newTestServer(t)
	rec := perform(s, "POST", "/api/auth/login", `{"username":"ADMIN","password":"TestPass1"}`, "")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/" || len(cookies[0].Value) != 43 {
		t.Fatal("unsafe session cookie")
	}
	if strings.Contains(rec.Body.String(), "TestPass1") || strings.Contains(rec.Body.String(), "pbkdf2") || strings.Contains(rec.Body.String(), cookies[0].Value) {
		t.Fatal("credential leaked")
	}
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	req.AddCookie(cookies[0])
	out := httptest.NewRecorder()
	s.Handler().ServeHTTP(out, req)
	if out.Code != 200 || strings.Contains(out.Body.String(), "password_hash") {
		t.Fatal("unsafe user listing")
	}
	s.cfg.SecureCookies = true
	rec = perform(s, "POST", "/api/auth/login", `{"username":"admin","password":"TestPass1"}`, "")
	if !rec.Result().Cookies()[0].Secure {
		t.Fatal("HTTPS deployment cookie missing Secure")
	}
}

func TestCSRFAndLegacyCredentialsRejected(t *testing.T) {
	s, _ := newTestServer(t)
	for _, origin := range []string{"https://attacker.test", "null"} {
		req := httptest.NewRequest("POST", "/api/admin/detection/stop", nil)
		authenticateTestRequest(req, false)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != 403 {
			t.Fatal("cross-origin mutation accepted")
		}
	}
	req := httptest.NewRequest("POST", "/api/admin/detection/stop", nil)
	authenticateTestRequest(req, false)
	req.Header.Del("X-CSRF-Token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	req = httptest.NewRequest("GET", "/api/admin/config", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatal("legacy bearer token accepted")
	}
	for _, path := range []string{"/api/admin/token", "/api/admin/view-token"} {
		if perform(s, "POST", path, `{}`, "admin").Code != 404 {
			t.Fatal("legacy token route remains")
		}
	}
}

func TestUserManagementAndSessionRevocation(t *testing.T) {
	s, store := newTestServer(t)
	rec := perform(s, "POST", "/api/admin/users", `{"username":"reader","password":"Reader123","role":"user","enabled":true}`, "admin")
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var user storage.User
	json.Unmarshal(rec.Body.Bytes(), &user)
	if !user.MustChangePassword {
		t.Fatal("initial password not marked for replacement")
	}
	if perform(s, "POST", "/api/admin/users", `{"username":"READER","password":"Reader123","role":"user","enabled":true}`, "admin").Code != 400 {
		t.Fatal("duplicate username accepted")
	}
	if perform(s, "DELETE", "/api/admin/users/1", "", "admin").Code != 400 {
		t.Fatal("self deletion accepted")
	}
	if perform(s, "PUT", "/api/admin/users/2", `{"username":"user","password":"","role":"user","enabled":false}`, "user").Code != 403 {
		t.Fatal("user can modify users")
	}
	if perform(s, "PUT", "/api/admin/users/2", `{"username":"user","password":"","role":"user","enabled":false}`, "admin").Code != 200 {
		t.Fatal("disable failed")
	}
	if perform(s, "GET", "/api/admin/detection", "", "user").Code != 401 {
		t.Fatal("disabled session remains valid")
	}
	if perform(s, "POST", "/api/auth/login", `{"username":"user","password":"TestPass1"}`, "").Code != 401 {
		t.Fatal("disabled user can login")
	}
	users, _ := store.ListUsers(context.Background())
	if len(users) != 3 {
		t.Fatal("unexpected user count")
	}
}

func TestInitialAndChangedPasswords(t *testing.T) {
	s, store := newTestServer(t)
	user, _ := store.FindUser(context.Background(), "user")
	user.MustChangePassword = true
	user, err := store.UpdateUser(context.Background(), user.ID, user, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSession(context.Background(), user, testSessionToken(true), "test-csrf", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	rec := perform(s, "GET", "/api/admin/detection", "", "user")
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "password_change_required") {
		t.Fatal("initial password bypass")
	}
	if perform(s, "POST", "/api/auth/password", `{"current_password":"wrong","password":"Newpass1"}`, "user").Code != 400 {
		t.Fatal("wrong current password accepted")
	}
	rec = perform(s, "POST", "/api/auth/password", `{"current_password":"TestPass1","password":"Newpass1"}`, "user")
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if perform(s, "GET", "/api/admin/detection", "", "user").Code != 401 {
		t.Fatal("old session not revoked")
	}
	if perform(s, "POST", "/api/auth/login", `{"username":"user","password":"TestPass1"}`, "").Code != 401 {
		t.Fatal("old password remains valid")
	}
	if perform(s, "POST", "/api/auth/login", `{"username":"user","password":"Newpass1"}`, "").Code != 200 {
		t.Fatal("new password rejected")
	}
}

type privateAdmin struct {
	stubAdmin
	private bool
}

func (a privateAdmin) AdminConfig(context.Context) (config.AdminConfig, error) {
	return config.AdminConfig{Settings: config.RuntimeSettings{StatusLoginRequired: a.private}}, nil
}

func TestStatusPrivacyProtectsRESTAndSSE(t *testing.T) {
	s, store := newTestServer(t)
	store.SaveLatestReport(context.Background(), sensitiveReport())
	s.admin = privateAdmin{private: true}
	for _, path := range []string{"/api/status", "/api/events"} {
		rec := perform(s, "GET", path, "", "")
		if rec.Code != 401 || strings.Contains(rec.Body.String(), "private-") {
			t.Fatal("private data leaked")
		}
	}
	for _, role := range []string{"admin", "user"} {
		if perform(s, "GET", "/api/status", "", role).Code != 200 {
			t.Fatal("account cannot read private status")
		}
	}
	s.admin = privateAdmin{private: false}
	if perform(s, "GET", "/api/status", "", "").Code != 200 {
		t.Fatal("public status rejected")
	}
}

func TestSSERechecksRevokedSessionsBeforeSending(t *testing.T) {
	s, store := newTestServer(t)
	s.admin = privateAdmin{private: true}
	store.SaveLatestReport(context.Background(), sensitiveReport())
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/events", nil)
	authenticateTestRequest(req, true)
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	if !scanner.Scan() || !strings.HasPrefix(scanner.Text(), "data:") {
		t.Fatal("missing initial event")
	}
	store.DeleteSession(context.Background(), testSessionToken(true))
	s.broker.Publish(sensitiveReport())
	found := false
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "event: auth-required") {
			found = true
		}
		if strings.HasPrefix(scanner.Text(), "data:") && scanner.Text() != "data: {}" {
			t.Fatal("revoked session got report")
		}
	}
	if !found {
		t.Fatal("missing revocation event")
	}
}

func TestLogoutExpiresServerSession(t *testing.T) {
	s, _ := newTestServer(t)
	rec := perform(s, "POST", "/api/auth/logout", "", "user")
	if rec.Code != 200 || rec.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("logout failed")
	}
	if perform(s, "GET", "/api/admin/detection", "", "user").Code != 401 {
		t.Fatal("logged-out session accepted")
	}
}

func TestRoleChangeRevokesSessionAndRechecksPermissions(t *testing.T) {
	s, store := newTestServer(t)
	ctx := context.Background()
	for _, role := range []string{"admin", "user"} {
		rec := perform(s, "PUT", "/api/admin/users/2", `{"username":"user","role":"`+role+`","enabled":true}`, "admin")
		if rec.Code != http.StatusOK {
			t.Fatal(rec.Body.String())
		}
		if perform(s, "GET", "/api/admin/users", "", "user").Code != http.StatusUnauthorized {
			t.Fatal("role change left an old session valid")
		}
		user, err := store.FindUser(ctx, "user")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.CreateSession(ctx, user, testSessionToken(true), "test-csrf", time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		want := http.StatusOK
		if role == "user" {
			want = http.StatusForbidden
		}
		if rec := perform(s, "GET", "/api/admin/users", "", "user"); rec.Code != want {
			t.Fatalf("role=%s: got %d, want %d", role, rec.Code, want)
		}
	}
}

func TestSameOriginAndLoginContentType(t *testing.T) {
	s, _ := newTestServer(t)
	for _, test := range []struct {
		origin string
		site   string
		want   int
	}{
		{"http://example.com:8081", "same-origin", 200},
		{"http://example.com", "same-origin", 403},
		{"https://example.com:8081", "same-origin", 403},
		{"http://example.com:8081", "cross-site", 403},
	} {
		req := httptest.NewRequest("POST", "http://example.com:8081/api/admin/detection/stop", nil)
		authenticateTestRequest(req, false)
		req.Header.Set("Origin", test.origin)
		req.Header.Set("Sec-Fetch-Site", test.site)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != test.want {
			t.Fatalf("%+v: got %d", test, rec.Code)
		}
	}
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"admin","password":"TestPass1"}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatal("cross-site compatible content type accepted for login")
	}
}

func TestMetricsRequireSessionAndForbidCaching(t *testing.T) {
	s, _ := newTestServer(t)
	s.SetMetrics(metrics.New())
	for _, role := range []string{"", "admin", "user"} {
		rec := perform(s, "GET", "/metrics", "", role)
		want := http.StatusOK
		if role == "" {
			want = http.StatusUnauthorized
		}
		if rec.Code != want || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("metrics role=%q status=%d cache=%q", role, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
}
