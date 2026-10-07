package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cg/internal/config"
)

const anthropicOK = `{"type":"message","content":[{"type":"thinking","thinking":"private"},{"type":"text","text":"pang"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`
const geminiOK = `{"candidates":[{"index":0,"content":{"parts":[{"text":"private","thought":true},{"text":"pang"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"thoughtsTokenCount":4,"totalTokenCount":9}}`

func nativeForTest(t *testing.T, protocol, response string, status int, stream bool) Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if stream && strings.HasPrefix(response, "data:") {
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		fmt.Fprint(w, response)
	}))
	t.Cleanup(server.Close)
	client := New(config.ProviderConfig{BaseURL: server.URL, APIKey: "private-key", Probe: config.ProbeOptions{Protocol: protocol, Stream: stream}})
	t.Cleanup(client.(interface{ CloseIdleConnections() }).CloseIdleConnections)
	return client
}

func TestNativeFactoryIsExplicitAndLegacyTypesAreUnchanged(t *testing.T) {
	for _, branding := range []string{"anthropic", "gemini", "custom"} {
		if _, ok := New(config.ProviderConfig{Type: branding}).(*OpenAICompatible); !ok {
			t.Fatalf("legacy branding changed wire protocol: %s", branding)
		}
	}
	for _, protocol := range []string{"anthropic", "gemini"} {
		if _, ok := New(config.ProviderConfig{Probe: config.ProbeOptions{Protocol: protocol}}).(*Native); !ok {
			t.Fatal("native selection ignored")
		}
	}
}

func TestNativeWireRequests(t *testing.T) {
	for _, protocol := range []string{"anthropic", "gemini"} {
		for _, omit := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/omit=%t", protocol, omit), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != "POST" || r.Header.Get("Authorization") != "" || r.URL.RawQuery != "" {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					}
					var body map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if protocol == "anthropic" {
						if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "private-key" || r.Header.Get("anthropic-version") != "2023-06-01" ||
							string(body["max_tokens"]) != "128" || string(body["model"]) != `"fixture"` {
							t.Error("Anthropic path, auth or token limit")
						}
						if (body["system"] == nil) != omit || (body["temperature"] == nil) != omit {
							t.Error("Anthropic omit settings")
						}
						fmt.Fprint(w, anthropicOK)
					} else {
						if r.URL.Path != "/v1/models/fixture:generateContent" || r.Header.Get("x-goog-api-key") != "private-key" || body["messages"] != nil {
							t.Error("Gemini wire format")
						}
						var generation map[string]any
						json.Unmarshal(body["generationConfig"], &generation)
						if generation["maxOutputTokens"] != float64(128) || generation["candidateCount"] != float64(1) ||
							(body["systemInstruction"] == nil) != omit || (generation["temperature"] == nil) != omit {
							t.Error("Gemini generation settings")
						}
						fmt.Fprint(w, geminiOK)
					}
					if !strings.Contains(string(body["messages"])+string(body["contents"]), "custom-prompt") {
						t.Error("custom prompt missing")
					}
				}))
				defer server.Close()
				client := New(config.ProviderConfig{BaseURL: server.URL + "/v1", APIKey: "private-key", Probe: config.ProbeOptions{
					Protocol: protocol, MaxTokens: 128, Prompt: "custom-prompt", SystemPrompt: "custom-system",
					OmitSystemPrompt: omit, OmitTemperature: omit,
				}})
				defer client.(interface{ CloseIdleConnections() }).CloseIdleConnections()
				text, usage, err := client.Chat(context.Background(), "fixture", "default-system", "default-prompt")
				if err != nil || text != "pang" || !usage.Known || usage.PromptTokens != 2 {
					t.Fatalf("%q %+v %v", text, usage, err)
				}
				if protocol == "gemini" && (usage.TotalTokens != 9 || usage.CompletionTokens != 7) {
					t.Fatal("thought tokens omitted", usage)
				}
			})
		}
	}
}

func TestNativeResponseFailuresPreserveUsage(t *testing.T) {
	for _, protocol := range []string{"anthropic", "gemini"} {
		for _, tc := range []struct {
			name, body string
			status     int
		}{
			{"error", `{"error":{"message":"private-key rejected"},"usage":{"input_tokens":2,"output_tokens":3},"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}}`, 429},
			{"in-band-error", `{"error":{"message":"private-key rejected"},"usage":{"input_tokens":2,"output_tokens":3},"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}}`, 200},
			{"truncated", `{"type":"message","content":[{"type":"text","text":"partial"}],"stop_reason":"max_tokens","usage":{"input_tokens":2,"output_tokens":3},"candidates":[{"content":{"parts":[{"text":"partial"}]},"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}}`, 200},
			{"blocked", `{"type":"message","stop_reason":"refusal","usage":{"input_tokens":2,"output_tokens":3},"promptFeedback":{"blockReason":"SAFETY"},"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}}`, 200},
		} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				client := nativeForTest(t, protocol, tc.body, tc.status, false)
				_, usage, err := client.Chat(context.Background(), "fixture", "", "ping")
				if err == nil || strings.Contains(err.Error(), "private-key") || usage.TotalTokens != 5 {
					t.Fatalf("%+v %v", usage, err)
				}
			})
		}
		for _, body := range []string{"{}", "null", "{", `{"candidates":[]}`, `{"content":[]}`} {
			client := nativeForTest(t, protocol, body, 200, false)
			if _, _, err := client.Chat(context.Background(), "fixture", "", "ping"); err == nil {
				t.Fatalf("invalid %s response accepted: %s", protocol, body)
			}
		}
	}
}

func TestNativeUsageCompletenessAndSpecialPricing(t *testing.T) {
	for _, tc := range []struct {
		body  string
		known bool
		total int
	}{
		{`{"input_tokens":0,"output_tokens":0}`, true, 0},
		{`{"input_tokens":2}`, false, 2},
		{`{"input_tokens":null,"output_tokens":3}`, false, 3},
		{`{"input_tokens":-1,"output_tokens":3}`, false, 3},
		{`{"input_tokens":2,"output_tokens":3,"cache_read_input_tokens":4,"cache_creation_input_tokens":5}`, false, 14},
		{fmt.Sprintf(`{"input_tokens":%d,"output_tokens":1}`, math.MaxInt), false, 0},
	} {
		var u anthropicUsage
		if err := json.Unmarshal([]byte(tc.body), &u); err != nil {
			t.Fatal(err)
		}
		got := u.usage(2)
		if got.Known != tc.known || got.TotalTokens != tc.total || got.FirstTokenMS != 2 {
			t.Fatal(tc.body, got)
		}
	}
	for _, tc := range []struct {
		body  string
		known bool
		total int
	}{
		{`{"promptTokenCount":0,"candidatesTokenCount":0}`, true, 0},
		{`{"promptTokenCount":2}`, false, 2},
		{`{"promptTokenCount":2,"candidatesTokenCount":null}`, false, 2},
		{`{"promptTokenCount":2,"candidatesTokenCount":3,"thoughtsTokenCount":4,"totalTokenCount":9}`, true, 9},
		{`{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":99}`, false, 99},
		{`{"promptTokenCount":2,"candidatesTokenCount":3,"cachedContentTokenCount":1}`, false, 5},
		{`{"promptTokenCount":2,"candidatesTokenCount":-3}`, false, 2},
	} {
		var u geminiUsage
		if err := json.Unmarshal([]byte(tc.body), &u); err != nil {
			t.Fatal(err)
		}
		got := u.usage(1)
		if got.Known != tc.known || got.TotalTokens != tc.total {
			t.Fatal(tc.body, got)
		}
	}
}

func TestNativePaginatedModels(t *testing.T) {
	for _, protocol := range []string{"anthropic", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			var calls, budget atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if r.URL.Path != "/models" || r.Method != "GET" || r.URL.Query().Get("key") != "" {
					t.Error("unsafe model request")
				}
				if protocol == "anthropic" {
					if n == 1 {
						fmt.Fprint(w, `{"data":[{"id":"z"},{"id":"a"}],"has_more":true,"last_id":"a"}`)
					} else {
						if r.URL.Query().Get("after_id") != "a" {
							t.Error("missing Anthropic cursor")
						}
						fmt.Fprint(w, `{"data":[{"id":"a"},{"id":"b"}],"has_more":false}`)
					}
				} else if n == 1 {
					fmt.Fprint(w, `{"models":[{"name":"models/z","supportedGenerationMethods":["generateContent"]},{"name":"models/embedding","supportedGenerationMethods":["embedContent"]},{"name":"models/a","supportedGenerationMethods":["generateContent"]}],"nextPageToken":"opaque +/="}`)
				} else {
					if r.URL.Query().Get("pageToken") != "opaque +/=" {
						t.Error("Gemini cursor not encoded")
					}
					fmt.Fprint(w, `{"models":[{"name":"models/a","supportedGenerationMethods":["generateContent"]},{"name":"models/b","supportedGenerationMethods":["generateContent"]}]}`)
				}
			}))
			defer server.Close()
			client := New(config.ProviderConfig{BaseURL: server.URL, Probe: config.ProbeOptions{Protocol: protocol}})
			defer client.(interface{ CloseIdleConnections() }).CloseIdleConnections()
			ctx := WithDiscoveryPageGuard(context.Background(), func(context.Context) error { budget.Add(1); return nil })
			models, err := client.Models(ctx)
			if err != nil || !reflect.DeepEqual(models, []string{"z", "a", "b"}) || calls.Load() != 2 || budget.Load() != 1 {
				t.Fatal(models, err, calls.Load(), budget.Load())
			}
		})
	}
}

func TestNativeDiscoveryNeverReturnsPartialCatalog(t *testing.T) {
	for _, mode := range []string{"budget", "repeated", "missing", "failed", "invalid", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				n := calls.Add(1)
				switch {
				case mode == "failed" && n == 2:
					w.WriteHeader(500)
					fmt.Fprint(w, `{"error":{"message":"private-key failed"}}`)
				case mode == "invalid":
					fmt.Fprint(w, `{}`)
				case mode == "missing":
					fmt.Fprint(w, `{"data":[{"id":"z"}],"has_more":true}`)
				case mode == "oversize":
					fmt.Fprint(w, strings.Repeat(" ", (1<<20)+1))
				default:
					fmt.Fprint(w, `{"data":[{"id":"z"}],"has_more":true,"last_id":"same"}`)
				}
			}))
			defer server.Close()
			client := New(config.ProviderConfig{BaseURL: server.URL, APIKey: "private-key", Probe: config.ProbeOptions{Protocol: "anthropic"}})
			defer client.(interface{ CloseIdleConnections() }).CloseIdleConnections()
			budgetErr := errors.New("budget depleted")
			ctx := WithDiscoveryPageGuard(context.Background(), func(context.Context) error {
				if mode == "budget" {
					return budgetErr
				}
				return nil
			})
			models, err := client.Models(ctx)
			if err == nil || models != nil || strings.Contains(err.Error(), "private-key") {
				t.Fatal(models, err)
			}
			if mode == "budget" && (!errors.Is(err, budgetErr) || calls.Load() != 1) {
				t.Fatal("budget not respected", calls.Load(), err)
			}
		})
	}
}

func TestNativeCancellationRedirectAndGeminiPathSafety(t *testing.T) {
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { leaked.Add(1) }))
	defer target.Close()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/slow/") {
			io.Copy(io.Discard, r.Body)
			select {
			case <-r.Context().Done():
			case <-release:
			}
		} else {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		}
	}))
	defer server.Close()
	defer close(release)
	for _, protocol := range []string{"anthropic", "gemini"} {
		for _, suffix := range []string{"", "/slow"} {
			client := New(config.ProviderConfig{BaseURL: server.URL + suffix, APIKey: "private-key", Probe: config.ProbeOptions{Protocol: protocol}})
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			_, _, err := client.Chat(ctx, "fixture", "", "ping")
			cancel()
			client.(interface{ CloseIdleConnections() }).CloseIdleConnections()
			if err == nil || leaked.Load() != 0 {
				t.Fatal("redirect or cancellation ignored")
			}
		}
	}
	for _, id := range []string{"../secret", ".", "models/", "models/a/b", "a?key=x", "a#x", "a%2fb", "a:method"} {
		if _, err := geminiModel(id); err == nil {
			t.Fatal("unsafe model ID", id)
		}
	}
	if id, err := geminiModel("models/gemini-fixture"); err != nil || id != "gemini-fixture" {
		t.Fatal(id, err)
	}
}

func TestNativeSSEFramingAndLimits(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n"} {
		got := ""
		err := readNativeSSE(strings.NewReader(strings.Join([]string{": keepalive", "data: one", "data:two", "", ""}, ending)), func(data []byte) (bool, error) {
			got = string(data)
			return false, nil
		})
		if err != nil || got != "one\ntwo" {
			t.Fatal(got, err)
		}
		oversize := strings.Repeat(": keepalive"+ending, (8<<20)/10)
		if err := readNativeSSE(strings.NewReader(oversize), func([]byte) (bool, error) { return false, nil }); err == nil {
			t.Fatal("oversized stream accepted")
		}
	}
	if err := readNativeSSE(strings.NewReader("data: "+strings.Repeat("x", 1<<20)+"\n\n"), func([]byte) (bool, error) { return false, nil }); err == nil {
		t.Fatal("oversized event accepted")
	}
}

func TestGeminiEmptyDiscoveryAndMalformedEnvelope(t *testing.T) {
	for _, body := range []string{`{}`, `{"models":[]}`, `null`, `[]`, `{"models":null,"error":{"message":"bad"}}`} {
		client := nativeForTest(t, "gemini", body, 200, false)
		models, err := client.Models(WithDiscoveryPageGuard(context.Background(), nil))
		valid := body == "{}" || body == `{"models":[]}`
		if (err == nil) != valid || valid && (models == nil || len(models) != 0) {
			t.Fatal(body, models, err)
		}
	}
}

func TestNativeStreamsRequireFinalUsageForPricing(t *testing.T) {
	for _, tc := range []struct{ protocol, body string }{
		{"anthropic", "data: {\"type\":\"message_start\",\"message\":{\"type\":\"message\",\"usage\":{\"input_tokens\":2,\"output_tokens\":0}}}\n\n" +
			"data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"pang\"}}\n\n" +
			"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"},
		{"gemini", "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"pang\"}]}}],\"usageMetadata\":{\"promptTokenCount\":2,\"candidatesTokenCount\":1}}\n\n" +
			"data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n"},
	} {
		client := nativeForTest(t, tc.protocol, tc.body, 200, true)
		text, usage, err := client.Chat(context.Background(), "fixture", "", "ping")
		if err != nil || text != "pang" || usage.Known || usage.TotalTokens == 0 {
			t.Fatal(tc.protocol, text, usage, err)
		}
	}
}

func TestNativeStreamingCompletion(t *testing.T) {
	aStart := "data: {\"type\":\"message_start\",\"message\":{\"type\":\"message\",\"usage\":{\"input_tokens\":2,\"output_tokens\":0}}}\n\n"
	aText := "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"pang\"}}\n\n"
	aDelta := "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":3}}\n\n"
	aEnd := "data: {\"type\":\"message_stop\"}\n\n"
	gText := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"pang\"}]}}]}\n\n"
	gEnd := "data: {\"candidates\":[{\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":2,\"candidatesTokenCount\":3}}\n\n"
	for _, tc := range []struct {
		name, protocol, stream string
		success                bool
		total                  int
	}{
		{"anthropic", "anthropic", aStart + aText + aDelta + aEnd, true, 5},
		{"anthropic disconnect", "anthropic", aStart + aText + aDelta, false, 5},
		{"anthropic no reason", "anthropic", aStart + aText + aEnd, false, 2},
		{"anthropic no start", "anthropic", aText + aDelta + aEnd, false, 0},
		{"anthropic length", "anthropic", aStart + aText + strings.ReplaceAll(aDelta, "end_turn", "max_tokens") + aEnd, false, 5},
		{"anthropic error", "anthropic", aStart + "data: {\"type\":\"error\",\"error\":{\"message\":\"private-key failed\"}}\n\n", false, 2},
		{"anthropic thinking", "anthropic", aStart + strings.ReplaceAll(aText, "text_delta", "thinking_delta") + aDelta + aEnd, false, 5},
		{"gemini", "gemini", gText + gEnd, true, 5},
		{"gemini disconnect", "gemini", gText, false, 0},
		{"gemini truncation", "gemini", gText + strings.ReplaceAll(gEnd, "STOP", "MAX_TOKENS"), false, 5},
		{"gemini after end", "gemini", gText + gEnd + gText, false, 5},
		{"gemini missing delimiter", "gemini", gText + strings.TrimRight(gEnd, "\n"), false, 0},
		{"gemini json error", "gemini", gText + "data: {\"error\":{\"message\":\"private-key failed\"},\"usageMetadata\":{\"promptTokenCount\":2,\"candidatesTokenCount\":3}}\n\n", false, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := nativeForTest(t, tc.protocol, tc.stream, 200, true)
			text, usage, err := client.Chat(context.Background(), "fixture", "", "ping")
			if (err == nil) != tc.success || usage.TotalTokens != tc.total {
				t.Fatalf("%q %+v %v", text, usage, err)
			}
			if tc.success && (text != "pang" || usage.FirstTokenMS < 1 || !usage.Known) {
				t.Fatal("text, usage or timing missing", text, usage)
			}
			if err != nil && strings.Contains(err.Error(), "private-key") {
				t.Fatal("stream leaked key")
			}
		})
	}
}
