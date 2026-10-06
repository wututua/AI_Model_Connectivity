package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cg/internal/config"
)

func TestConfiguredProbeRequests(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			requests := make(chan map[string]any, 1)
			paths := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				requests <- body
				paths <- r.URL.Path
				fmt.Fprint(w, `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"pang"}]}],"choices":[{"message":{"content":"pang"},"finish_reason":"stop"}],"usage":{"input_tokens":2,"output_tokens":3,"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
			}))
			defer server.Close()
			client := NewOpenAICompatible(config.ProviderConfig{BaseURL: server.URL, Probe: config.ProbeOptions{
				Protocol: protocol, MaxTokens: 1024, TokenLimitField: "max_completion_tokens",
				OmitTemperature: true, Prompt: "custom", OmitSystemPrompt: true,
			}})
			defer client.CloseIdleConnections()
			text, usage, err := client.Chat(context.Background(), "fixture", "system", "original")
			if err != nil || text != "pang" || usage.TotalTokens != 5 {
				t.Fatalf("%q %+v %v", text, usage, err)
			}
			body := <-requests
			path := <-paths
			if _, ok := body["temperature"]; ok {
				t.Fatal("temperature was not omitted")
			}
			if _, ok := body["max_tokens"]; ok {
				t.Fatal("legacy token field present")
			}
			if protocol == "chat" {
				if path != "/chat/completions" || body["max_completion_tokens"] != float64(1024) {
					t.Fatal(body, path)
				}
				messages := body["messages"].([]any)
				if len(messages) != 1 || messages[0].(map[string]any)["content"] != "custom" {
					t.Fatal(messages)
				}
			} else {
				if path != "/responses" || body["store"] != false || body["max_output_tokens"] != float64(1024) || body["input"] != "custom" {
					t.Fatal(body, path)
				}
				if _, ok := body["instructions"]; ok {
					t.Fatal("omitted system prompt sent")
				}
			}
		})
	}
}

func TestStreamCompletionAndUsage(t *testing.T) {
	chatText := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"pang\"}}]}\n\n"
	chatEnd := "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"
	chatUsage := "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n"
	responseText := "event: response.output_text.delta\r\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"pang\"}\r\n\r\n"
	responseEnd := "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5}}}\n\n"
	for _, tc := range []struct {
		name, stream       string
		responses, success bool
		tokens             int
	}{
		{"chat", chatText + chatEnd + chatUsage + "data: [DONE]\n\n", false, true, 5},
		{"chat disconnect", chatText + chatEnd + chatUsage, false, false, 5},
		{"chat premature done", chatText + "data: [DONE]\n\n", false, false, 0},
		{"chat truncated", chatText + strings.ReplaceAll(chatEnd, "stop", "length") + chatUsage + "data: [DONE]\n\n", false, false, 5},
		{"chat reasoning only", strings.ReplaceAll(chatText, "pang", "<think>private</think>") + chatEnd + "data: [DONE]\n\n", false, false, 0},
		{"responses", responseText + responseEnd, true, true, 5},
		{"responses disconnect", responseText, true, false, 0},
		{"responses incomplete", responseText + strings.ReplaceAll(responseEnd, "completed", "incomplete"), true, false, 5},
		{"responses no text", responseEnd, true, false, 5},
		{"bad json", "data: nope\n\n", true, false, 0},
		{"error", `data: {"error":{"message":"secret-key failed"},"usage":{"prompt_tokens":2}}` + "\n\n", false, false, 2},
		{"multiline", "data: {\"choices\":\ndata: [{\"index\":0,\"delta\":{\"content\":\"pang\"}}]}\n\n" + chatEnd + "data: [DONE]\n\n", false, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewOpenAICompatible(config.ProviderConfig{APIKey: "secret-key"})
			text, usage, err := client.readStream(strings.NewReader(tc.stream), tc.responses, time.Now())
			if (err == nil) != tc.success || usage.TotalTokens != tc.tokens {
				t.Fatalf("%q %+v %v", text, usage, err)
			}
			if tc.success && (text != "pang" || usage.FirstTokenMS < 1) {
				t.Fatal("missing text/first output timing")
			}
			if err != nil && strings.Contains(err.Error(), "secret-key") {
				t.Fatal("secret exposed")
			}
		})
	}
}

func TestResponsesFailureRetainsUsage(t *testing.T) {
	for _, status := range []int{200, 400, 503} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			fmt.Fprint(w, `{"status":"failed","error":{"message":"secret failed"},"usage":{"input_tokens":2,"output_tokens":7}}`)
		}))
		client := NewOpenAICompatible(config.ProviderConfig{BaseURL: server.URL, APIKey: "secret", Probe: config.ProbeOptions{Protocol: "responses"}})
		_, usage, err := client.Chat(context.Background(), "m", "", "ping")
		client.CloseIdleConnections()
		server.Close()
		if err == nil || strings.Contains(err.Error(), "secret") || usage.TotalTokens != 9 {
			t.Fatalf("%+v %v", usage, err)
		}
	}
}

func TestStreamingHTTPPath(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["stream"] != true {
				t.Error("stream not enabled")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			if protocol == "chat" {
				if body["stream_options"] == nil {
					t.Error("usage not requested")
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"pang\"},\"finish_reason\":\"stop\"}]}\n\n")
				fmt.Fprint(w, "data: [DONE]\n\n")
			} else {
				fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"pang\"}\n\n")
				fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
			}
		}))
		client := NewOpenAICompatible(config.ProviderConfig{BaseURL: server.URL, Probe: config.ProbeOptions{Protocol: protocol, Stream: true}})
		text, _, err := client.Chat(context.Background(), "fixture", "", "ping")
		client.CloseIdleConnections()
		server.Close()
		if err != nil || text != "pang" {
			t.Fatal(text, err)
		}
	}
}

func TestJSONErrorInStreamingRequestKeepsUsage(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"error":{"message":"secret failure"},"usage":{"prompt_tokens":2,"completion_tokens":3,"input_tokens":2,"output_tokens":3}}`)
		}))
		client := NewOpenAICompatible(config.ProviderConfig{BaseURL: server.URL, APIKey: "secret", Probe: config.ProbeOptions{Protocol: protocol, Stream: true}})
		_, usage, err := client.Chat(context.Background(), "fixture", "", "ping")
		client.CloseIdleConnections()
		server.Close()
		if err == nil || strings.Contains(err.Error(), "secret") || usage.TotalTokens != 5 {
			t.Fatal(usage, err)
		}
	}
}

func TestSplitThinkingTagIsNotFirstEffectiveText(t *testing.T) {
	for _, text := range []string{"<", "<thi", "<think>", "<think>reason", "<think>reason</think>", " <think>reason</think> <thi"} {
		if firstVisibleText(text) != "" {
			t.Fatal("thinking fragment considered visible", text)
		}
	}
	if firstVisibleText("<think>reason</think>pang") != "pang" {
		t.Fatal("output text lost")
	}
}
