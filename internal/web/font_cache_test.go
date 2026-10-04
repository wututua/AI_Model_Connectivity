package web

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFontVersionedCache(t *testing.T) {
	dir := t.TempDir()
	fontDir := filepath.Join(dir, "fonts", "harmonyos-sans")
	if err := os.MkdirAll(fontDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := []byte("font fixture")
	fontPath := filepath.Join(fontDir, "Regular.ttf")
	if err := os.WriteFile(fontPath, content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0644); err != nil {
		t.Fatal(err)
	}
	handler := spaHandler(dir)
	version := fmt.Sprintf("%x", sha256.Sum256(content))
	url := "/fonts/harmonyos-sans/Regular.ttf"
	for _, tc := range []struct{ url, cache string }{
		{url + "?v=" + version, "public, max-age=31536000, immutable"},
		{url, "no-cache"},
		{url + "?v=outdated", "no-cache"},
		{"/admin/settings", "no-cache"},
		{"/", "no-cache"},
	} {
		t.Run(tc.url, func(t *testing.T) {
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if res.Code != http.StatusOK || res.Header().Get("Cache-Control") != tc.cache {
				t.Fatalf("status=%d cache=%q", res.Code, res.Header().Get("Cache-Control"))
			}
		})
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("If-None-Match", `"`+version+`"`)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNotModified || res.Body.Len() != 0 {
		t.Fatalf("expected empty 304, got %d", res.Code)
	}
	if err := os.WriteFile(fontPath, []byte("updated font fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, url+"?v="+version, nil))
	if res.Header().Get("Cache-Control") != "no-cache" || res.Header().Get("ETag") == `"`+version+`"` {
		t.Fatal("changed font must not reuse the previous immutable version")
	}
}
