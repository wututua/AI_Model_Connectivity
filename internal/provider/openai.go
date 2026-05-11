package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"cg/internal/config"
)

var (
	thinkTagCompleteRE = regexp.MustCompile(`(?is)<think(?:ing)?>\s*.*?\s*</think(?:ing)?>`)
	thinkTagOpenRE     = regexp.MustCompile(`(?is)<think(?:ing)?>.*`)
)

func stripThinkingTags(s string) string {
	s = thinkTagCompleteRE.ReplaceAllString(s, "")
	// Handle truncated responses where </think> was cut off by MaxTokens
	s = thinkTagOpenRE.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

type OpenAICompatible struct {
	cfg    config.ProviderConfig
	client *http.Client
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func NewOpenAICompatible(cfg config.ProviderConfig) *OpenAICompatible {
	return &OpenAICompatible{cfg: cfg, client: http.DefaultClient}
}

func (p *OpenAICompatible) ID() string      { return p.cfg.ID }
func (p *OpenAICompatible) Name() string    { return p.cfg.Name }
func (p *OpenAICompatible) Type() string    { return p.cfg.Type }
func (p *OpenAICompatible) BaseURL() string { return p.cfg.BaseURL }

func (p *OpenAICompatible) Models(ctx context.Context) ([]string, error) {
	if len(p.cfg.Models) > 0 {
		return append([]string(nil), p.cfg.Models...), nil
	}
	if p.cfg.BaseURL == "" {
		return nil, fmt.Errorf("base url is empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	p.authorize(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var parsed modelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse models response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, responseError(resp.StatusCode, parsed.Error, string(body))
	}
	models := []string{}
	seen := map[string]bool{}
	for _, item := range parsed.Data {
		model := strings.TrimSpace(item.ID)
		if model != "" && !seen[model] {
			seen[model] = true
			models = append(models, model)
		}
	}
	return models, nil
}

func (p *OpenAICompatible) Chat(ctx context.Context, model, systemPrompt, prompt string) (string, error) {
	if p.cfg.BaseURL == "" {
		return "", fmt.Errorf("base url is empty")
	}
	payload := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: 0,
		MaxTokens:   16,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	p.authorize(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse chat response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", responseError(resp.StatusCode, parsed.Error, string(respBody))
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("empty choices")
	}
	return stripThinkingTags(parsed.Choices[0].Message.Content), nil
}

func (p *OpenAICompatible) authorize(req *http.Request) {
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}
}

func responseError(status int, apiErr interface{}, body string) error {
	message := ""
	switch value := apiErr.(type) {
	case *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	}:
		if value != nil {
			message = value.Message
		}
	}
	if message == "" {
		message = strings.TrimSpace(body)
	}
	if len(message) > 300 {
		message = message[:300] + "..."
	}
	return fmt.Errorf("http %d: %s", status, message)
}
