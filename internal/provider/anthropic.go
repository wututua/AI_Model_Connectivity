package provider

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"
)

type anthropicUsage struct {
	Input         *int `json:"input_tokens"`
	Output        *int `json:"output_tokens"`
	CacheCreation *int `json:"cache_creation_input_tokens"`
	CacheRead     *int `json:"cache_read_input_tokens"`
}

func validCount(value *int) bool { return value != nil && *value >= 0 }
func tokenCount(value *int) int {
	if validCount(value) {
		return *value
	}
	return 0
}

func addTokens(values ...int) (int, bool) {
	total := 0
	for _, value := range values {
		if value < 0 || value > math.MaxInt-total {
			return 0, false
		}
		total += value
	}
	return total, true
}

func (u anthropicUsage) usage(first int) Usage {
	input, validInput := addTokens(tokenCount(u.Input), tokenCount(u.CacheCreation), tokenCount(u.CacheRead))
	output := tokenCount(u.Output)
	total, validTotal := addTokens(input, output)
	known := validCount(u.Input) && validCount(u.Output) && validInput && validTotal
	// Cache-specific pricing is not representable by the two configured token rates.
	for _, cache := range []*int{u.CacheCreation, u.CacheRead} {
		if cache != nil && *cache != 0 {
			known = false
		}
	}
	return Usage{Known: known, PromptTokens: input, CompletionTokens: output, TotalTokens: total, FirstTokenMS: first}
}

func (u *anthropicUsage) merge(next anthropicUsage) {
	for _, pair := range []struct{ to, from **int }{
		{&u.Input, &next.Input}, {&u.Output, &next.Output},
		{&u.CacheCreation, &next.CacheCreation}, {&u.CacheRead, &next.CacheRead},
	} {
		if *pair.from != nil {
			*pair.to = *pair.from
		}
	}
}

type anthropicBody struct {
	Type    string `json:"type"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      anthropicUsage `json:"usage"`
}

func (p *Native) anthropic(ctx context.Context, model, system, prompt string) (string, Usage, error) {
	options := p.cfg.Probe
	tokens := options.MaxTokens
	if tokens == 0 {
		tokens = 16
	}
	payload := map[string]any{
		"model": model, "max_tokens": tokens, "stream": options.Stream,
		"messages": []chatMessage{{Role: "user", Content: prompt}},
	}
	if system != "" {
		payload["system"] = system
	}
	if !options.OmitTemperature {
		payload["temperature"] = options.Temperature
	}
	started := time.Now()
	resp, err := p.request(ctx, http.MethodPost, "/messages", payload)
	if err != nil {
		return "", Usage{}, err
	}
	if options.Stream && resp.StatusCode >= 200 && resp.StatusCode < 300 &&
		strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		defer resp.Body.Close()
		return p.anthropicStream(resp, started)
	}
	data, err := readNativeBody(resp)
	if err != nil {
		return "", Usage{}, p.redactError(err)
	}
	var body anthropicBody
	parseErr := json.Unmarshal(data, &body)
	usage := body.Usage.usage(0)
	if err := p.responseError(resp.StatusCode, data); err != nil {
		return "", usage, err
	}
	if parseErr != nil {
		return "", usage, errors.New("invalid Anthropic response")
	}
	if options.Stream {
		return "", usage, errors.New("upstream returned JSON instead of an Anthropic stream")
	}
	var text strings.Builder
	for _, content := range body.Content {
		if content.Type == "text" {
			text.WriteString(content.Text)
		}
	}
	output := stripThinkingTags(text.String())
	if body.Type != "message" || !anthropicStopped(body.StopReason) || output == "" {
		return "", usage, errors.New("Anthropic response did not complete with text")
	}
	return output, usage, nil
}

func anthropicStopped(reason string) bool { return reason == "end_turn" || reason == "stop_sequence" }

func (p *Native) anthropicStream(resp *http.Response, started time.Time) (string, Usage, error) {
	var text strings.Builder
	var counts anthropicUsage
	usage := Usage{}
	start, ended, stopped, finalUsage := false, false, false, false
	err := readNativeSSE(resp.Body, func(data []byte) (bool, error) {
		var event struct {
			Type    string         `json:"type"`
			Message *anthropicBody `json:"message"`
			Usage   anthropicUsage `json:"usage"`
			Delta   struct {
				Type       string `json:"type"`
				Text       string `json:"text"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			ContentBlock struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content_block"`
		}
		if json.Unmarshal(data, &event) != nil {
			return false, errors.New("invalid Anthropic stream event")
		}
		if event.Message != nil {
			counts.merge(event.Message.Usage)
		}
		counts.merge(event.Usage)
		usage = counts.usage(usage.FirstTokenMS)
		if err := p.responseError(http.StatusOK, data); err != nil {
			return false, err
		}
		if !start && event.Type != "message_start" && event.Type != "ping" {
			return false, errors.New("Anthropic event before message start")
		}
		if stopped && strings.HasPrefix(event.Type, "content_block_") {
			return false, errors.New("Anthropic content after terminal response")
		}
		switch event.Type {
		case "message_start":
			if start || event.Message == nil || event.Message.Type != "message" {
				return false, errors.New("invalid Anthropic message start")
			}
			start = true
		case "content_block_start":
			if event.ContentBlock.Type == "text" {
				text.WriteString(event.ContentBlock.Text)
			}
		case "content_block_delta":
			if event.Delta.Type == "text_delta" {
				text.WriteString(event.Delta.Text)
			}
		case "message_delta":
			finalUsage = finalUsage || validCount(event.Usage.Output)
			if event.Delta.StopReason != "" {
				if !anthropicStopped(event.Delta.StopReason) {
					return false, errors.New("Anthropic stream was truncated or did not produce text")
				}
				stopped = true
			}
		case "message_stop":
			ended = true
		case "error":
			return false, errors.New("Anthropic stream failed")
		}
		if text.Len() > 1<<20 {
			return false, errors.New("stream output exceeds 1 MiB")
		}
		markFirstText(&usage, text.String(), started)
		return ended, nil
	})
	if err != nil {
		return "", usage, p.redactError(err)
	}
	output := stripThinkingTags(text.String())
	if !start || !ended || !stopped || output == "" {
		return "", usage, errors.New("Anthropic stream ended without a complete text response")
	}
	usage.Known = usage.Known && finalUsage
	return output, usage, nil
}
