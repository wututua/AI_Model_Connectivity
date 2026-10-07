package httpclient

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Diagnostics struct {
	DNSMS       *int64 `json:"dns_ms,omitempty"`
	ConnectMS   *int64 `json:"connect_ms,omitempty"`
	TLSMS       *int64 `json:"tls_ms,omitempty"`
	FirstByteMS *int64 `json:"first_byte_ms,omitempty"`
	Reused      bool   `json:"connection_reused"`
	HTTPStatus  int    `json:"http_status,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
	RetryAfter  string `json:"retry_after,omitempty"`
}

type diagnosticKey struct{}

type Trace struct {
	mu                         sync.Mutex
	value                      Diagnostics
	started, dns, connect, tls time.Time
}

func TraceContext(ctx context.Context) (context.Context, *Trace) {
	t := &Trace{started: time.Now()}
	trace := &httptrace.ClientTrace{
		DNSStart:             func(httptrace.DNSStartInfo) { t.mu.Lock(); defer t.mu.Unlock(); t.dns = time.Now() },
		DNSDone:              func(httptrace.DNSDoneInfo) { t.mu.Lock(); defer t.mu.Unlock(); elapsed(&t.value.DNSMS, t.dns) },
		ConnectStart:         func(string, string) { t.mu.Lock(); defer t.mu.Unlock(); t.connect = time.Now() },
		ConnectDone:          func(string, string, error) { t.mu.Lock(); defer t.mu.Unlock(); elapsed(&t.value.ConnectMS, t.connect) },
		TLSHandshakeStart:    func() { t.mu.Lock(); defer t.mu.Unlock(); t.tls = time.Now() },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { t.mu.Lock(); defer t.mu.Unlock(); elapsed(&t.value.TLSMS, t.tls) },
		GotConn:              func(info httptrace.GotConnInfo) { t.mu.Lock(); defer t.mu.Unlock(); t.value.Reused = info.Reused },
		GotFirstResponseByte: func() { t.mu.Lock(); defer t.mu.Unlock(); elapsed(&t.value.FirstByteMS, t.started) },
	}
	return context.WithValue(httptrace.WithClientTrace(ctx, trace), diagnosticKey{}, t), t
}

func elapsed(target **int64, start time.Time) {
	if start.IsZero() {
		return
	}
	ms := time.Since(start).Milliseconds()
	if *target != nil {
		ms += **target
	}
	*target = &ms
}

func (t *Trace) Snapshot(secret string) *Diagnostics {
	t.mu.Lock()
	defer t.mu.Unlock()
	value := t.value
	if secret != "" {
		value.RequestID = strings.ReplaceAll(value.RequestID, secret, "[redacted]")
		value.RetryAfter = strings.ReplaceAll(value.RetryAfter, secret, "[redacted]")
	}
	return &value
}

func captureResponse(ctx context.Context, response *http.Response) {
	t, _ := ctx.Value(diagnosticKey{}).(*Trace)
	if t == nil || response == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.value.HTTPStatus = response.StatusCode
	t.value.RequestID = safeHeader(response.Header.Get("X-Request-ID"))
	if t.value.RequestID == "" {
		t.value.RequestID = safeHeader(response.Header.Get("Request-ID"))
	}
	t.value.RetryAfter = safeHeader(response.Header.Get("Retry-After"))
}

func safeHeader(value string) string {
	if len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return value
}
