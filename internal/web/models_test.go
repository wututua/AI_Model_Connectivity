package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cg/internal/config"
)

type discoveryAdmin struct {
	stubAdmin
	query config.ModelDiscoveryRequest
	calls int
}

func (a *discoveryAdmin) DiscoverModels(_ context.Context, query config.ModelDiscoveryRequest) ([]string, error) {
	a.query, a.calls = query, a.calls+1
	return []string{"model-one", "model-two"}, nil
}

func TestModelDiscoveryAuthenticationAndRequest(t *testing.T) {
	s, _ := newTestServer(t)
	admin := &discoveryAdmin{}
	s.admin = admin
	path := "/api/admin/provider-models"
	body := `{"provider_id":"saved","type":"openai","base_url":"https://example.test/v1","api_key":"private-key","clear_api_key":false}`
	if perform(s, "POST", path, body, "").Code != http.StatusUnauthorized {
		t.Fatal("anonymous discovery accepted")
	}
	if perform(s, "POST", path, body, "user").Code != http.StatusForbidden {
		t.Fatal("ordinary user can use discovery")
	}
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	authenticateTestRequest(req, false)
	req.Header.Del("X-CSRF-Token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || admin.calls != 0 {
		t.Fatal("discovery bypassed CSRF")
	}
	rec = perform(s, "POST", path, body, "admin")
	var models []string
	if err := json.Unmarshal(rec.Body.Bytes(), &models); err != nil || len(models) != 2 || rec.Code != 200 {
		t.Fatal("invalid model response")
	}
	if admin.calls != 1 || admin.query.ProviderID != "saved" || admin.query.APIKey != "private-key" || admin.query.BaseURL != "https://example.test/v1" {
		t.Fatal("draft not forwarded")
	}
	if strings.Contains(rec.Body.String(), "private-key") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("credentials leaked or response can be cached")
	}
	if perform(s, "GET", path, "", "admin").Code != http.StatusMethodNotAllowed {
		t.Fatal("unexpected method accepted")
	}
	for _, body := range []string{"null", "[]", "{} {}", `{"api_key":"` + strings.Repeat("a", maxRequestBody) + `"}`} {
		rec := perform(s, "POST", path, body, "admin")
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatal("invalid body accepted")
		}
	}
}
