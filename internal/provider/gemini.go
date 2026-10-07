package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type geminiUsage struct {
	Input   *int `json:"promptTokenCount"`
	Output  *int `json:"candidatesTokenCount"`
	Thought *int `json:"thoughtsTokenCount"`
	Total   *int `json:"totalTokenCount"`
	Cached  *int `json:"cachedContentTokenCount"`
	Tool    *int `json:"toolUsePromptTokenCount"`
}

func (u geminiUsage) usage(first int) Usage {
	input, validInput := addTokens(tokenCount(u.Input), tokenCount(u.Tool))
	output, validOutput := addTokens(tokenCount(u.Output), tokenCount(u.Thought))
	total, validTotal := addTokens(input, output)
	known := validCount(u.Input) && validCount(u.Output) && validInput && validOutput && validTotal
	if validCount(u.Total) {
		if *u.Total != total {
			known = false
		}
		total = *u.Total
	} else if u.Total != nil {
		known = false
	}
	if u.Thought != nil && !validCount(u.Thought) || u.Tool != nil && *u.Tool != 0 || u.Cached != nil && *u.Cached != 0 {
		known = false
	}
	return Usage{Known: known, PromptTokens: input, CompletionTokens: output, TotalTokens: total, FirstTokenMS: first}
}

type geminiBody struct {
	Candidates []struct {
		Index   int `json:"index"`
		Content struct {
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Usage          *geminiUsage `json:"usageMetadata"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

func (p *Native) gemini(ctx context.Context, model, system, prompt string) (string, Usage, error) {
	model, err := geminiModel(model)
	if err != nil {
		return "", Usage{}, err
	}
	options := p.cfg.Probe
	tokens := options.MaxTokens
	if tokens == 0 {
		tokens = 16
	}
	generation := map[string]any{"maxOutputTokens": tokens, "candidateCount": 1}
	if !options.OmitTemperature {
		generation["temperature"] = options.Temperature
	}
	payload := map[string]any{
		"contents":         []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": prompt}}}},
		"generationConfig": generation,
	}
	if system != "" {
		payload["systemInstruction"] = map[string]any{"parts": []any{map[string]string{"text": system}}}
	}
	path := "/models/" + model + ":generateContent"
	if options.Stream {
		path = "/models/" + model + ":streamGenerateContent?alt=sse"
	}
	started := time.Now()
	resp, err := p.request(ctx, http.MethodPost, path, payload)
	if err != nil {
		return "", Usage{}, err
	}
	if options.Stream && resp.StatusCode >= 200 && resp.StatusCode < 300 &&
		strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		defer resp.Body.Close()
		return p.geminiStream(resp, started)
	}
	data, err := readNativeBody(resp)
	if err != nil {
		return "", Usage{}, p.redactError(err)
	}
	var body geminiBody
	parseErr := json.Unmarshal(data, &body)
	usage := Usage{}
	if body.Usage != nil {
		usage = body.Usage.usage(0)
	}
	if err := p.responseError(resp.StatusCode, data); err != nil {
		return "", usage, err
	}
	if parseErr != nil {
		return "", usage, errors.New("invalid Gemini response")
	}
	if options.Stream {
		return "", usage, errors.New("upstream returned JSON instead of a Gemini stream")
	}
	text, completed, err := body.text()
	if err != nil {
		return "", usage, err
	}
	output := stripThinkingTags(text)
	if !completed || output == "" {
		return "", usage, errors.New("Gemini response did not complete with text")
	}
	return output, usage, nil
}

func (b geminiBody) text() (string, bool, error) {
	if b.PromptFeedback.BlockReason != "" && b.PromptFeedback.BlockReason != "BLOCK_REASON_UNSPECIFIED" {
		return "", false, errors.New("Gemini prompt was blocked")
	}
	var text strings.Builder
	completed := false
	for _, candidate := range b.Candidates {
		if candidate.Index != 0 {
			continue
		}
		if candidate.FinishReason != "" {
			if candidate.FinishReason != "STOP" {
				return "", false, errors.New("Gemini response was blocked or truncated")
			}
			completed = true
		}
		for _, part := range candidate.Content.Parts {
			if !part.Thought {
				text.WriteString(part.Text)
			}
		}
	}
	return text.String(), completed, nil
}

func (p *Native) geminiStream(resp *http.Response, started time.Time) (string, Usage, error) {
	var text strings.Builder
	usage := Usage{}
	completed, finalUsage := false, false
	err := readNativeSSE(resp.Body, func(data []byte) (bool, error) {
		var body geminiBody
		if json.Unmarshal(data, &body) != nil {
			return false, errors.New("invalid Gemini stream event")
		}
		if body.Usage != nil {
			usage = body.Usage.usage(usage.FirstTokenMS)
		}
		if err := p.responseError(http.StatusOK, data); err != nil {
			return false, err
		}
		chunk, done, err := body.text()
		if err != nil {
			return false, err
		}
		if completed && (chunk != "" || done) {
			return false, errors.New("Gemini content after terminal response")
		}
		completed = completed || done
		if completed && body.Usage != nil {
			finalUsage = true
		}
		text.WriteString(chunk)
		if text.Len() > 1<<20 {
			return false, errors.New("stream output exceeds 1 MiB")
		}
		markFirstText(&usage, text.String(), started)
		// Usage-only events may follow the final candidate; consume the complete stream.
		return false, nil
	})
	if err != nil {
		return "", usage, p.redactError(err)
	}
	output := stripThinkingTags(text.String())
	if !completed || output == "" {
		return "", usage, errors.New("Gemini stream ended without a complete text response")
	}
	usage.Known = usage.Known && finalUsage
	return output, usage, nil
}
