package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"cg/internal/config"
	"cg/internal/provider"
)

func TestIsSkipped(t *testing.T) {
	skip := skipSet([]string{
		"gpt-4",
		"openai/gpt-3.5-turbo",
		"anthropic::claude-3-opus",
	})

	cases := []struct {
		providerID   string
		providerName string
		model        string
		want         bool
	}{
		// bare model name match
		{"openai", "OpenAI", "gpt-4", true},
		// provider/model format
		{"openai", "OpenAI", "gpt-3.5-turbo", true},
		// provider::model format
		{"anthropic", "Anthropic", "claude-3-opus", true},
		// providerName/model fallback
		{"p1", "openai", "gpt-3.5-turbo", true},
		// no match
		{"openai", "OpenAI", "gpt-4o", false},
		{"openai", "OpenAI", "gpt-4o-mini", false},
	}

	for _, c := range cases {
		got := isSkipped(skip, c.providerID, c.providerName, c.model)
		if got != c.want {
			t.Errorf("isSkipped(%q, %q, %q) = %v, want %v", c.providerID, c.providerName, c.model, got, c.want)
		}
	}
}

func TestDedupe(t *testing.T) {
	input := []string{"a", "b", "a", " b ", "c", ""}
	got := dedupe(input)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("dedupe(%v) = %v, want %v", input, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dedupe index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		input string
		limit int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		// Unicode: each rune should count, not bytes
		{"你好世界测试", 4, "你好世界..."},
		{"", 5, ""},
	}
	for _, c := range cases {
		got := truncate(c.input, c.limit)
		if got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.input, c.limit, got, c.want)
		}
	}
}

func TestSanitizeErrorText(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// URL脱敏：移除 /v1/ 之前的域名，保留路径及后续错误信息
		{
			`Post "https://api.openai.com/v1/chat/completions": dial tcp: lookup failed`,
			`Post "/v1/chat/completions": dial tcp: lookup failed`,
		},
		// DNS错误：移除主机名，只保留 "lookup : no such host"
		{
			`lookup api.openai.com: no such host`,
			`lookup : no such host`,
		},
		// 无匹配，原文返回
		{
			`connection refused`,
			`connection refused`,
		},
	}
	for _, c := range cases {
		got := sanitizeErrorText(c.input)
		if got != c.want {
			t.Errorf("sanitizeErrorText(%q)\n  got  %q\n  want %q", c.input, got, c.want)
		}
	}
}

func TestSkipSet(t *testing.T) {
	items := []string{"A", "b", " C ", "b"} // 重复 + 大小写 + 空格
	set := skipSet(items)
	if !set["a"] {
		t.Error("expected 'a' in skip set (case-insensitive)")
	}
	if !set["b"] {
		t.Error("expected 'b' in skip set")
	}
	if !set["c"] {
		t.Error("expected 'c' in skip set after trim")
	}
	if len(set) != 3 {
		t.Errorf("expected 3 entries, got %d", len(set))
	}
}

func TestModelLimitAfterExclusionAndCurrentModel(t *testing.T) {
	runner := NewRunner(config.Config{
		MaxModelsPerProvider: 1,
		SkipModels:           []string{"Display/skip"},
		Providers: []config.ProviderConfig{{
			ID: "p1", Name: "Display", Enabled: true, ProbeEnabled: true,
			Models: []string{"skip", " skip ", "chosen", "other"},
		}},
	})
	targets, failures := runner.collectTargets(context.Background())
	if len(failures) != 0 || len(targets) != 1 || targets[0].Model != "chosen" || targets[0].CurrentModel != "chosen" {
		t.Fatalf("wrong selected targets: %+v, %+v", targets, failures)
	}
}

func TestFailedCompletionPreservesUsageInResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":""}}],"usage":{"total_tokens":8}}`))
	}))
	defer server.Close()
	runner := NewRunner(config.Config{
		Providers: []config.ProviderConfig{{ID: "p1", Enabled: true, ProbeEnabled: true, BaseURL: server.URL, Models: []string{"m1"}}},
	})
	results, _, err := runner.Run(context.Background())
	if err != nil || len(results) != 1 || results[0].Status != "error" || results[0].TotalTokens != 8 || results[0].CheckedAt == "" {
		t.Fatalf("wrong failed probe result: %+v, %v", results, err)
	}
}

func TestProviderModelIdentityDoesNotCollide(t *testing.T) {
	pairs := [][2]string{
		{"a", "b::c"}, {"a::b", "c"},
		{"a:", "c"}, {"a", ":c"},
		{"a%3A", "c"}, {"a%253A", "c"},
	}
	cfg := config.Config{}
	for _, pair := range pairs {
		cfg.Providers = append(cfg.Providers, config.ProviderConfig{
			ID: pair[0], Models: []string{pair[1]}, Enabled: true, ProbeEnabled: true,
		})
	}
	targets, failures := NewRunner(cfg).collectTargets(context.Background())
	if len(failures) != 0 || len(targets) != len(pairs) {
		t.Fatalf("identity collision skipped probes: got %d targets, want %d", len(targets), len(pairs))
	}
	keys := map[string]bool{}
	for _, target := range targets {
		key := resultPayload(target, "ok", 1, "", "", provider.Usage{}).HistoryKey
		if keys[key] {
			t.Errorf("history identity collision: %q", key)
		}
		keys[key] = true
	}
}
