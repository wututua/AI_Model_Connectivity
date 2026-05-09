package provider

import (
	"context"
	"strings"

	"cg/internal/config"
)

type Provider interface {
	ID() string
	Name() string
	Type() string
	BaseURL() string
	Models(ctx context.Context) ([]string, error)
	Chat(ctx context.Context, model, systemPrompt, prompt string) (string, error)
}

func New(cfg config.ProviderConfig) Provider {
	switch strings.ToLower(cfg.Type) {
	case "", "openai", "openai-compatible", "ollama", "openrouter", "siliconflow", "deepseek", "dashscope":
		return NewOpenAICompatible(cfg)
	default:
		return NewOpenAICompatible(cfg)
	}
}
