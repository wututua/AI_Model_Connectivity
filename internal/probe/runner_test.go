package probe

import (
	"testing"
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
