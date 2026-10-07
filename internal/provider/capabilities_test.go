package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"cg/internal/config"
)

func TestUsageCompletenessDoesNotInventZeroCost(t *testing.T) {
	for _, test := range []struct {
		raw             string
		chat, responses bool
	}{
		{`{}`, false, false},
		{`{"total_tokens":5}`, false, false},
		{`{"prompt_tokens":0,"completion_tokens":0}`, true, false},
		{`{"input_tokens":0,"output_tokens":0}`, false, true},
		{`{"prompt_tokens":1,"completion_tokens":null}`, false, false},
		{`{"prompt_tokens":-1,"completion_tokens":2}`, false, false},
	} {
		var usage responseUsage
		if err := json.Unmarshal([]byte(test.raw), &usage); err != nil {
			t.Fatal(err)
		}
		if usage.usage(false, 0).Known != test.chat || usage.usage(true, 0).Known != test.responses {
			t.Fatalf("incorrect completeness: %s", test.raw)
		}
	}
}

func TestCapabilityAssertions(t *testing.T) {
	for _, test := range []struct {
		text    string
		options config.ProbeOptions
		failed  bool
	}{
		{`{"status":"ok"}`, config.ProbeOptions{AssertJSON: true, AssertJSONKeys: "status", AssertContains: "ok"}, false},
		{`null`, config.ProbeOptions{AssertJSON: true}, true},
		{`[]`, config.ProbeOptions{AssertJSON: true}, true},
		{`{"status":"ok"}`, config.ProbeOptions{AssertJSONKeys: "missing"}, true},
		{`pong`, config.ProbeOptions{AssertContains: "pang"}, true},
	} {
		err := AssertText(test.text, test.options)
		if errors.Is(err, ErrAssertion) != test.failed {
			t.Fatalf("%s: %v", test.text, err)
		}
	}
}

func TestEmbeddingAndToolProbes(t *testing.T) {
	for _, test := range []struct {
		capability, path, body string
		failed                 bool
	}{
		{"embedding", "/embeddings", `{"data":[{"index":0,"embedding":[0.1,-0.2]}],"usage":{"prompt_tokens":2,"total_tokens":2}}`, false},
		{"embedding", "/embeddings", `{"data":[]}`, true},
		{"tools", "/chat/completions", `{"choices":[{"finish_reason":"tool_calls","message":{"tool_calls":[{"type":"function","function":{"name":"connectivity_check","arguments":"{\"ok\":true}"}}]}}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`, false},
		{"tools", "/chat/completions", `{"choices":[{"finish_reason":"stop","message":{"content":"OK"}}]}`, true},
	} {
		t.Run(test.capability+test.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != test.path {
					t.Errorf("path=%s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(test.body))
			}))
			defer server.Close()
			p := NewOpenAICompatible(config.ProviderConfig{BaseURL: server.URL, Probe: config.ProbeOptions{Capability: test.capability}})
			defer p.CloseIdleConnections()
			_, usage, err := p.Chat(context.Background(), "fixture", "", "ping")
			if (err != nil) != test.failed {
				t.Fatalf("err=%v", err)
			}
			if !test.failed && !usage.Known {
				t.Fatal("usage missing")
			}
		})
	}
}
