package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiagnosticsAreMeasuredAndRedacted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "request-secret")
		w.Header().Set("Retry-After", "12")
		w.Header().Set("Authorization", "never expose")
		w.WriteHeader(429)
		w.Write([]byte("error"))
	}))
	defer server.Close()
	ctx, trace := TraceContext(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	client := New(0)
	defer client.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	value := trace.Snapshot("secret")
	if value.HTTPStatus != 429 || value.FirstByteMS == nil || value.ConnectMS == nil || value.TLSMS != nil || strings.Contains(value.RequestID, "secret") || value.RetryAfter != "12" {
		t.Fatalf("diagnostics=%+v", value)
	}
}
