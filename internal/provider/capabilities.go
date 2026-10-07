package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"cg/internal/config"
)

var ErrAssertion = errors.New("capability assertion failed")

func AssertText(text string, options config.ProbeOptions) error {
	if options.AssertContains != "" && !strings.Contains(text, options.AssertContains) {
		return fmt.Errorf("%w: required text missing", ErrAssertion)
	}
	if options.AssertJSON || strings.TrimSpace(options.AssertJSONKeys) != "" {
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(text), &object); err != nil || object == nil {
			return fmt.Errorf("%w: expected JSON object", ErrAssertion)
		}
		for _, key := range strings.Split(options.AssertJSONKeys, ",") {
			if key = strings.TrimSpace(key); key != "" {
				if _, exists := object[key]; !exists {
					return fmt.Errorf("%w: required JSON field missing", ErrAssertion)
				}
			}
		}
	}
	return nil
}

func (p *OpenAICompatible) capabilityProbe(ctx context.Context, model, prompt string) (string, Usage, error) {
	path := "/embeddings"
	payload := map[string]any{"model": model, "input": prompt, "encoding_format": "float"}
	if p.cfg.Probe.Capability == "tools" {
		path = "/chat/completions"
		tokens := p.cfg.Probe.MaxTokens
		if tokens == 0 {
			tokens = 64
		}
		field := p.cfg.Probe.TokenLimitField
		if field == "" {
			field = "max_tokens"
		}
		payload = map[string]any{
			"model": model, field: tokens,
			"messages": []chatMessage{{Role: "user", Content: "Call connectivity_check with ok set to true."}},
			"tools": []any{map[string]any{"type": "function", "function": map[string]any{
				"name": "connectivity_check", "description": "Connectivity probe; no external action.",
				"parameters": map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]string{"type": "boolean"}}, "required": []string{"ok"}, "additionalProperties": false},
			}}},
			"tool_choice": map[string]any{"type": "function", "function": map[string]string{"name": "connectivity_check"}},
		}
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	p.authorize(req)
	res, err := p.client.Do(req)
	if err != nil {
		return "", Usage{}, p.redactError(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil {
		return "", Usage{}, p.redactError(err)
	}
	if len(data) > 1<<20 {
		return "", Usage{}, errors.New("capability response exceeds 1 MiB")
	}
	var parsed struct {
		Usage *responseUsage `json:"usage"`
		Data  []struct {
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				ToolCalls []struct {
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Error json.RawMessage `json:"error"`
	}
	parseErr := json.Unmarshal(data, &parsed)
	usage := Usage{}
	if parsed.Usage != nil {
		usage = parsed.Usage.usage(false, 0)
		if p.cfg.Probe.Capability == "embedding" {
			usage.Known = parsed.Usage.promptReported
		}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", usage, fmt.Errorf("capability probe returned HTTP %d", res.StatusCode)
	}
	if parseErr != nil || len(parsed.Error) > 0 && string(parsed.Error) != "null" {
		return "", usage, errors.New("invalid capability response")
	}
	if p.cfg.Probe.Capability == "embedding" {
		if len(parsed.Data) != 1 || parsed.Data[0].Index != 0 || len(parsed.Data[0].Embedding) == 0 {
			return "", usage, fmt.Errorf("%w: missing embedding", ErrAssertion)
		}
		for _, n := range parsed.Data[0].Embedding {
			if math.IsNaN(n) || math.IsInf(n, 0) {
				return "", usage, fmt.Errorf("%w: invalid vector", ErrAssertion)
			}
		}
		return fmt.Sprintf("embedding: %d dimensions", len(parsed.Data[0].Embedding)), usage, nil
	}
	if len(parsed.Choices) != 1 || parsed.Choices[0].FinishReason != "tool_calls" || len(parsed.Choices[0].Message.ToolCalls) != 1 {
		return "", usage, fmt.Errorf("%w: tool call missing or incomplete", ErrAssertion)
	}
	call := parsed.Choices[0].Message.ToolCalls[0]
	var arguments map[string]any
	if call.Type != "function" || call.Function.Name != "connectivity_check" || json.Unmarshal([]byte(call.Function.Arguments), &arguments) != nil || arguments["ok"] != true || len(arguments) != 1 {
		return "", usage, fmt.Errorf("%w: invalid tool arguments", ErrAssertion)
	}
	return "tool call validated", usage, nil
}
