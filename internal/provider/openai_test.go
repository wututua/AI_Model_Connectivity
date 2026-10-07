package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cg/internal/config"
)

func TestErrorResponsesAndCredentialRedaction(t *testing.T) {
	for _, status := range []int{200, 302, 401, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(status)
			writer.Write([]byte(`{"error":{"message":"invalid key secret-api-key"},"choices":[{"message":{"content":"ignored"}}]}`))
		}))
		provider := NewOpenAICompatible(config.ProviderConfig{BaseURL: server.URL, APIKey: "secret-api-key"})
		_, err := provider.Models(context.Background())
		if err == nil || strings.Contains(err.Error(), "secret-api-key") {
			t.Errorf("unsafe models error: %v", err)
		}
		_, _, err = provider.Chat(context.Background(), "test", "", "test")
		if err == nil || strings.Contains(err.Error(), "secret-api-key") {
			t.Errorf("unsafe chat error: %v", err)
		}
		provider.CloseIdleConnections()
		server.Close()
	}
}

func TestChatValidatesCompletionAndPreservesUsage(t *testing.T) {
	for _, tc := range []struct {
		name, choices, want string
	}{
		{"no choices", `[]`, ""},
		{"missing message", `[{}]`, ""},
		{"null message", `[{"message":null}]`, ""},
		{"empty content", `[{"message":{"content":"  "}}]`, ""},
		{"thinking only", `[{"message":{"content":"<think>reason</think>"}}]`, ""},
		{"unfinished thinking", `[{"message":{"content":"<thinking>reason"}}]`, ""},
		{"truncated", `[{"message":{"content":"partial"},"finish_reason":"length"}]`, ""},
		{"valid", `[{"message":{"content":"pang"},"finish_reason":"stop"}]`, "pang"},
		{"compatible", `[{"message":{"content":"<think>reason</think> pang"}}]`, "pang"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(`{"choices":` + tc.choices + `,"usage":{"prompt_tokens":3,"completion_tokens":5}}`))
			}))
			defer server.Close()
			client := NewOpenAICompatible(config.ProviderConfig{BaseURL: server.URL})
			defer client.CloseIdleConnections()
			text, usage, err := client.Chat(context.Background(), "m1", "", "ping")
			if (err == nil) != (tc.want != "") || text != tc.want {
				t.Fatalf("text=%q err=%v", text, err)
			}
			if usage.PromptTokens != 3 || usage.CompletionTokens != 5 || usage.TotalTokens != 8 {
				t.Fatalf("lost usage: %+v", usage)
			}
		})
	}
}

func TestErrorEnvelopePreservesReportedUsage(t *testing.T) {
	for _, status := range []int{200, 400, 429, 503} {
		for _, usageJSON := range []string{
			`{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}`,
			`{"prompt_tokens":3,"completion_tokens":5}`,
		} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte(`{"error":{"message":"failed secret-key"},"usage":` + usageJSON + `}`))
			}))
			client := NewOpenAICompatible(config.ProviderConfig{BaseURL: server.URL, APIKey: "secret-key"})
			text, usage, err := client.Chat(context.Background(), "fixture", "", "ping")
			client.CloseIdleConnections()
			server.Close()
			if err == nil || text != "" || strings.Contains(err.Error(), "secret-key") {
				t.Fatalf("HTTP %d: unsafe or missing error: %v", status, err)
			}
			if usage != (Usage{Known: true, PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8}) {
				t.Errorf("HTTP %d discarded reported usage: %+v", status, usage)
			}
		}
	}
}
