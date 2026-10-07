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
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Native shares transport, identity and redaction, but never OpenAI wire formats.
type Native struct{ *OpenAICompatible }

type discoveryPageGuardKey struct{}

// WithDiscoveryPageGuard budgets pages after the first, already reserved by callers.
func WithDiscoveryPageGuard(ctx context.Context, guard func(context.Context) error) context.Context {
	return context.WithValue(ctx, discoveryPageGuardKey{}, guard)
}

func (p *Native) request(ctx context.Context, method, path string, payload any) (*http.Response, error) {
	if p.cfg.BaseURL == "" {
		return nil, errors.New("base url is empty")
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, body)
	if err != nil {
		return nil, p.redactError(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.Probe.Protocol == "anthropic" {
		req.Header.Set("anthropic-version", "2023-06-01")
		if p.cfg.APIKey != "" {
			req.Header.Set("x-api-key", p.cfg.APIKey)
		}
	} else if p.cfg.APIKey != "" {
		req.Header.Set("x-goog-api-key", p.cfg.APIKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, p.redactError(err)
	}
	return resp, nil
}

func readNativeBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("native response exceeds 1 MiB")
	}
	return data, nil
}

func (p *Native) responseError(status int, data []byte) error {
	var body struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(data, &body)
	if body.Error != nil {
		return p.redactError(fmt.Errorf("http %d: %s", status, body.Error.Message))
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("native API returned HTTP %d", status)
	}
	return nil
}

func (p *Native) Models(ctx context.Context) ([]string, error) {
	if len(p.cfg.Models) > 0 {
		return slices.Clone(p.cfg.Models), nil
	}
	out := []string{}
	seen, cursors := map[string]bool{}, map[string]bool{}
	cursor, totalBytes := "", 0
	for page := 0; page < 100; page++ {
		if page > 0 {
			if guard, ok := ctx.Value(discoveryPageGuardKey{}).(func(context.Context) error); ok && guard != nil {
				if err := guard(ctx); err != nil {
					return nil, err
				}
			}
		}
		query := url.Values{}
		if p.cfg.Probe.Protocol == "anthropic" {
			query.Set("limit", "1000")
			if cursor != "" {
				query.Set("after_id", cursor)
			}
		} else {
			query.Set("pageSize", "1000")
			if cursor != "" {
				query.Set("pageToken", cursor)
			}
		}
		resp, err := p.request(ctx, http.MethodGet, "/models?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		data, err := readNativeBody(resp)
		if err != nil {
			return nil, p.redactError(err)
		}
		totalBytes += len(data)
		if totalBytes > 8<<20 {
			return nil, errors.New("model discovery exceeds 8 MiB")
		}
		if err := p.responseError(resp.StatusCode, data); err != nil {
			return nil, err
		}
		var body struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
			Models  []struct {
				Name    string   `json:"name"`
				Methods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
		}
		if json.Unmarshal(data, &body) != nil || strings.TrimSpace(string(data)) == "null" {
			return nil, errors.New("invalid native model list")
		}
		ids := []string{}
		next := ""
		if p.cfg.Probe.Protocol == "anthropic" {
			if body.Data == nil {
				return nil, errors.New("missing native model list")
			}
			for _, item := range body.Data {
				ids = append(ids, item.ID)
			}
			if body.HasMore {
				if body.LastID == "" {
					return nil, errors.New("model discovery cursor missing")
				}
				next = body.LastID
			}
		} else {
			// Google JSON responses may omit an empty repeated field.
			for _, item := range body.Models {
				if slices.Contains(item.Methods, "generateContent") {
					id, err := geminiModel(item.Name)
					if err != nil {
						return nil, err
					}
					ids = append(ids, id)
				}
			}
			next = body.NextPageToken
		}
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id != "" && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
		if len(out) > 10000 {
			return nil, errors.New("model discovery exceeds 10000 models")
		}
		if next == "" {
			return out, nil
		}
		if len(next) > 4096 || cursors[next] {
			return nil, errors.New("invalid or repeated model discovery cursor")
		}
		cursors[next], cursor = true, next
	}
	return nil, errors.New("model discovery exceeds 100 pages")
}

var geminiModelID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)

func geminiModel(model string) (string, error) {
	model = strings.TrimPrefix(model, "models/")
	if !geminiModelID.MatchString(model) {
		return "", errors.New("invalid Gemini model ID")
	}
	return model, nil
}

func (p *Native) Chat(ctx context.Context, model, system, prompt string) (string, Usage, error) {
	if err := p.cfg.Probe.Validate(); err != nil {
		return "", Usage{}, err
	}
	if p.cfg.Probe.Prompt != "" {
		prompt = p.cfg.Probe.Prompt
	}
	if p.cfg.Probe.SystemPrompt != "" {
		system = p.cfg.Probe.SystemPrompt
	}
	if p.cfg.Probe.OmitSystemPrompt {
		system = ""
	}
	if p.cfg.Probe.Protocol == "anthropic" {
		return p.anthropic(ctx, model, system, prompt)
	}
	return p.gemini(ctx, model, system, prompt)
}

// Native streams use bounded SSE framing; an EOF alone is never proof of success.
func readNativeSSE(reader io.Reader, process func([]byte) (bool, error)) error {
	limited := &io.LimitedReader{R: reader, N: (8 << 20) + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var event strings.Builder
	consumed := 0
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		consumed += len(line) + 1
		if consumed > 8<<20 {
			return errors.New("stream exceeds 8 MiB")
		}
		if line == "" {
			if event.Len() != 0 {
				done, err := process([]byte(strings.TrimSuffix(event.String(), "\n")))
				if err != nil || done {
					return err
				}
				event.Reset()
			}
		} else if strings.HasPrefix(line, "data:") {
			event.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			event.WriteByte('\n')
			if event.Len() > 1<<20 {
				return errors.New("stream event exceeds 1 MiB")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if limited.N == 0 {
		return errors.New("stream exceeds 8 MiB")
	}
	if event.Len() != 0 {
		return errors.New("incomplete stream event")
	}
	return nil
}

func markFirstText(usage *Usage, text string, started time.Time) {
	if usage.FirstTokenMS == 0 && firstVisibleText(text) != "" {
		usage.FirstTokenMS = max(1, int(time.Since(started).Milliseconds()))
	}
}
