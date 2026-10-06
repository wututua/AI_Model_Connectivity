package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
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

func TestCompressedFontNegotiation(t *testing.T) {
	dir := t.TempDir()
	fontDir := filepath.Join(dir, "fonts")
	if err := os.MkdirAll(fontDir, 0755); err != nil {
		t.Fatal(err)
	}
	original := bytes.Repeat([]byte("original font bytes"), 100)
	filename := filepath.Join(fontDir, "test.ttf")
	digest := fmt.Sprintf("%x", sha256.Sum256(original))
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	writer.Write(original)
	writer.Close()
	if err := os.WriteFile(filename, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename+"."+digest+".gz", compressed.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	handler := spaHandler(dir)
	for _, tc := range []struct {
		accept, rangeHeader, match string
		wantGzip                   bool
		status                     int
	}{
		{"gzip, br", "", "", true, 200},
		{"gzip;q=0.5", "", "", true, 200},
		{"*;q=1, gzip;q=0", "", "", false, 200},
		{"gzip;q=invalid", "", "", false, 200},
		{"br", "", "", false, 200},
		{"gzip", "bytes=0-3", "", false, 206},
		{"gzip", "", `"` + digest + `"`, true, 200},
		{"gzip", "", `"` + digest + `-gzip"`, true, 304},
	} {
		req := httptest.NewRequest(http.MethodGet, "/fonts/test.ttf?v="+digest, nil)
		req.Header.Set("Accept-Encoding", tc.accept)
		req.Header.Set("Range", tc.rangeHeader)
		req.Header.Set("If-None-Match", tc.match)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.status || (res.Code != 304 && (res.Header().Get("Content-Encoding") == "gzip") != tc.wantGzip) || res.Header().Get("Vary") != "Accept-Encoding" {
			t.Fatalf("%+v: status=%d headers=%v", tc, res.Code, res.Header())
		}
		if tc.wantGzip && res.Code == 200 {
			reader, err := gzip.NewReader(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := io.ReadAll(reader)
			reader.Close()
			if err != nil || !bytes.Equal(decoded, original) {
				t.Fatal("compression altered font bytes")
			}
		}
	}
}
