package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type responseUsage struct {
	inputReported      bool
	outputReported     bool
	promptReported     bool
	completionReported bool
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	PromptTokens       int `json:"prompt_tokens"`
	CompletionTokens   int `json:"completion_tokens"`
	TotalTokens        int `json:"total_tokens"`
}

func (u *responseUsage) UnmarshalJSON(data []byte) error {
	type plain responseUsage
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	reported := func(key string) bool {
		value, ok := fields[key]
		return ok && string(value) != "null"
	}
	*u = responseUsage(value)
	u.inputReported = reported("input_tokens") && u.InputTokens >= 0
	u.outputReported = reported("output_tokens") && u.OutputTokens >= 0
	u.promptReported = reported("prompt_tokens") && u.PromptTokens >= 0
	u.completionReported = reported("completion_tokens") && u.CompletionTokens >= 0
	return nil
}

func (u responseUsage) usage(responses bool, first int) Usage {
	input, output := u.PromptTokens, u.CompletionTokens
	known := u.promptReported && u.completionReported
	if responses {
		input, output = u.InputTokens, u.OutputTokens
		known = u.inputReported && u.outputReported
	}
	total := u.TotalTokens
	sum, validTotal := addTokens(max(0, input), max(0, output))
	known = known && validTotal && total >= 0 && (total == 0 || total == sum)
	if total == 0 {
		total = sum
	}
	return Usage{Known: known, PromptTokens: max(0, input), CompletionTokens: max(0, output), TotalTokens: max(0, total), FirstTokenMS: first}
}

type responsesBody struct {
	Status string `json:"status"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage *responseUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (b responsesBody) text() string {
	var out strings.Builder
	for _, item := range b.Output {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" {
				out.WriteString(content.Text)
			}
		}
	}
	return stripThinkingTags(out.String())
}

func (p *OpenAICompatible) responses(ctx context.Context, model, system, prompt string) (string, Usage, error) {
	options := p.cfg.Probe
	tokens := options.MaxTokens
	if tokens == 0 {
		tokens = 16
	}
	payload := map[string]any{"model": model, "input": prompt, "max_output_tokens": tokens, "store": false, "stream": options.Stream}
	if system != "" {
		payload["instructions"] = system
	}
	if !options.OmitTemperature {
		payload["temperature"] = options.Temperature
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", Usage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, p.redactError(err)
	}
	req.Header.Set("Content-Type", "application/json")
	p.authorize(req)
	started := time.Now()
	resp, err := p.client.Do(req)
	if err != nil {
		return "", Usage{}, p.redactError(err)
	}
	defer resp.Body.Close()
	if options.Stream && resp.StatusCode >= 200 && resp.StatusCode < 300 && !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		return p.readStream(resp.Body, true, started)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return "", Usage{}, p.redactError(err)
	}
	if len(data) > 1<<20 {
		return "", Usage{}, errors.New("response exceeds 1 MiB")
	}
	var parsed responsesBody
	parseErr := json.Unmarshal(data, &parsed)
	usage := Usage{}
	if parsed.Usage != nil {
		usage = parsed.Usage.usage(true, 0)
	}
	if parsed.Error != nil {
		return "", usage, p.redactError(errors.New(parsed.Error.Message))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", usage, fmt.Errorf("responses returned HTTP %d", resp.StatusCode)
	}
	if parseErr != nil {
		return "", usage, errors.New("invalid responses body")
	}
	if options.Stream {
		return "", usage, errors.New("upstream returned JSON instead of a stream")
	}
	if parsed.Status != "completed" {
		return "", usage, errors.New("response did not complete (failed or token limit)")
	}
	text := parsed.text()
	if text == "" {
		return "", usage, errors.New("empty response content")
	}
	return text, usage, nil
}

// SSE events may span multiple data lines; only a terminal event confirms success.
func (p *OpenAICompatible) readStream(reader io.Reader, responses bool, started time.Time) (string, Usage, error) {
	scanner := bufio.NewScanner(io.LimitReader(reader, (8<<20)+1))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var text, event strings.Builder
	usage := Usage{}
	completed, ended, truncated := false, false, false
	consumed := 0
	process := func(data string) error {
		if data == "" {
			return nil
		}
		if strings.TrimSpace(data) == "[DONE]" && !responses {
			ended = true
			return nil
		}
		var chunk struct {
			Type     string         `json:"type"`
			Delta    string         `json:"delta"`
			Response *responsesBody `json:"response"`
			Usage    *responseUsage `json:"usage"`
			Error    *struct {
				Message string `json:"message"`
			} `json:"error"`
			Choices []struct {
				Index int `json:"index"`
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return errors.New("invalid stream event")
		}
		if chunk.Usage != nil {
			usage = chunk.Usage.usage(responses, usage.FirstTokenMS)
		}
		if chunk.Response != nil && chunk.Response.Usage != nil {
			usage = chunk.Response.Usage.usage(true, usage.FirstTokenMS)
		}
		if chunk.Error != nil {
			return p.redactError(errors.New(chunk.Error.Message))
		}
		if responses {
			switch chunk.Type {
			case "response.output_text.delta":
				text.WriteString(chunk.Delta)
			case "response.completed":
				if chunk.Response == nil || chunk.Response.Status != "completed" {
					return errors.New("invalid terminal response")
				}
				completed, ended = true, true
			case "response.failed", "response.incomplete", "error":
				return errors.New("stream response failed or was incomplete")
			}
		} else {
			for _, choice := range chunk.Choices {
				if choice.Index != 0 {
					continue
				}
				text.WriteString(choice.Delta.Content)
				if choice.FinishReason != "" {
					completed = choice.FinishReason == "stop"
					truncated = choice.FinishReason != "stop"
				}
			}
		}
		if text.Len() > 1<<20 {
			return errors.New("stream output exceeds 1 MiB")
		}
		if usage.FirstTokenMS == 0 && firstVisibleText(text.String()) != "" {
			usage.FirstTokenMS = max(1, int(time.Since(started).Milliseconds()))
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		consumed += len(line) + 1
		if consumed > 8<<20 {
			return "", usage, errors.New("stream exceeds 8 MiB")
		}
		if line == "" {
			if err := process(strings.TrimSuffix(event.String(), "\n")); err != nil {
				return "", usage, err
			}
			event.Reset()
			if ended {
				break
			}
		} else if strings.HasPrefix(line, "data:") {
			event.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			event.WriteByte('\n')
			if event.Len() > 1<<20 {
				return "", usage, errors.New("stream event exceeds 1 MiB")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", usage, p.redactError(err)
	}
	output := stripThinkingTags(text.String())
	if !ended || !completed || truncated || output == "" {
		return "", usage, errors.New("stream ended without a complete text response")
	}
	return output, usage, nil
}

func firstVisibleText(text string) string {
	visible := stripThinkingTags(text)
	lower := strings.ToLower(visible)
	if strings.HasPrefix("<think>", lower) || strings.HasPrefix("<thinking>", lower) {
		return ""
	}
	return visible
}
