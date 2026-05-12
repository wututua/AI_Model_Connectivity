package provider

import (
	"context"
	"strings"

	"cg/internal/config"
)

// Usage captures token consumption reported by the upstream completion.
// Zero values mean the provider didn't return usage data (e.g. some local
// runtimes or proxies omit the field) — callers should not treat 0 as
// "no tokens charged" for billing purposes.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Provider interface {
	ID() string
	Name() string
	Type() string
	BaseURL() string
	Models(ctx context.Context) ([]string, error)
	Chat(ctx context.Context, model, systemPrompt, prompt string) (string, Usage, error)
}

func New(cfg config.ProviderConfig) Provider {
	switch strings.ToLower(cfg.Type) {
	case "", "openai", "openai-compatible", "ollama", "openrouter", "siliconflow", "deepseek", "dashscope":
		return NewOpenAICompatible(cfg)
	default:
		return NewOpenAICompatible(cfg)
	}
}
