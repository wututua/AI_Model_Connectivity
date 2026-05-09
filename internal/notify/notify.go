package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cg/internal/config"
	"cg/internal/report"
)

type StateStore interface {
	Read() (State, error)
	Write(State) error
}

type FileStateStore struct {
	Path string
}

func (s FileStateStore) Read() (State, error) {
	return readState(s.Path)
}

func (s FileStateStore) Write(value State) error {
	return writeState(s.Path, value)
}

type Client struct {
	cfg        config.Config
	stateStore StateStore
	httpClient *http.Client
}

type payload struct {
	Status       string   `json:"status"`
	Title        string   `json:"title"`
	GeneratedAt  string   `json:"generated_at"`
	Summary      string   `json:"summary"`
	ErrorCount   int      `json:"error_count"`
	SlowCount    int      `json:"slow_count"`
	OKCount      int      `json:"ok_count"`
	Total        int      `json:"total"`
	ProviderText []string `json:"provider_text"`
	Text         string   `json:"text"`
}

func New(cfg config.Config, stateStore StateStore) *Client {
	return &Client{
		cfg:        cfg,
		stateStore: stateStore,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) SendIfNeeded(ctx context.Context, value report.Report) error {
	if !c.enabled() {
		return nil
	}
	value = filterReport(value, c.cfg.NotifyProviders, c.cfg.NotifyModels)
	current := alertState(value)
	previous, err := c.stateStore.Read()
	if err != nil {
		return err
	}
	if current == previous.Status {
		return nil
	}
	if current == "ok" && !c.cfg.NotifyOnRecovery {
		return c.stateStore.Write(State{Status: current, SentAt: previous.SentAt})
	}
	if current == "ok" && previous.Status == "" {
		return c.stateStore.Write(State{Status: current, SentAt: previous.SentAt})
	}
	now := time.Now()
	if inCooldown(previous, now, c.cfg.NotifyCooldownMinutes) {
		return c.stateStore.Write(State{Status: current, SentAt: previous.SentAt})
	}
	if err := c.send(ctx, buildPayload(value)); err != nil {
		return err
	}
	return c.stateStore.Write(State{Status: current, SentAt: now})
}

func (c *Client) enabled() bool {
	switch c.platform() {
	case "telegram":
		return c.cfg.NotifyTelegramBotToken != "" && c.cfg.NotifyTelegramChatID != ""
	default:
		return strings.TrimSpace(c.cfg.NotifyWebhookURL) != ""
	}
}

func (c *Client) platform() string {
	platform := strings.ToLower(strings.TrimSpace(c.cfg.NotifyPlatform))
	if platform == "" {
		return "webhook"
	}
	return platform
}

func inCooldown(previous State, now time.Time, minutes int) bool {
	if minutes <= 0 || previous.SentAt.IsZero() {
		return false
	}
	return now.Sub(previous.SentAt) < time.Duration(minutes)*time.Minute
}

func alertState(value report.Report) string {
	if value.ErrorCount > 0 || len(value.ProviderErrors) > 0 {
		return "error"
	}
	if value.SlowCount > 0 {
		return "slow"
	}
	return "ok"
}

func buildPayload(value report.Report) payload {
	providers := providerLines(value)
	summary := fmt.Sprintf("%s：正常 %d / 较慢 %d / 异常 %d / 总计 %d", value.Title, value.OKCount, value.SlowCount, value.ErrorCount, value.Total)
	if len(value.ProviderErrors) > 0 {
		summary = fmt.Sprintf("%s / Provider 错误 %d", summary, len(value.ProviderErrors))
	}
	text := summary
	if len(providers) > 0 {
		text += "\n" + strings.Join(providers, "\n")
	}
	return payload{
		Status:       value.OverallStatus,
		Title:        value.Title,
		GeneratedAt:  value.GeneratedAt,
		Summary:      summary,
		ErrorCount:   value.ErrorCount,
		SlowCount:    value.SlowCount,
		OKCount:      value.OKCount,
		Total:        value.Total,
		ProviderText: providers,
		Text:         text,
	}
}

func providerLines(value report.Report) []string {
	lines := []string{}
	for _, provider := range value.Providers {
		if provider.ErrorCount == 0 && provider.SlowCount == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s：正常 %d，较慢 %d，异常 %d", provider.ProviderName, provider.OKCount, provider.SlowCount, provider.ErrorCount))
	}
	for _, item := range value.ProviderErrors {
		lines = append(lines, fmt.Sprintf("- %s：%s", item.ProviderID, item.Error))
	}
	if len(lines) > 10 {
		return append(lines[:10], fmt.Sprintf("- 其余 %d 项已省略", len(lines)-10))
	}
	return lines
}

func (c *Client) send(ctx context.Context, body payload) error {
	switch c.platform() {
	case "telegram":
		return c.sendTelegram(ctx, body)
	case "discord", "bark", "wecom", "wechat_work", "dingtalk", "webhook":
		return c.sendWebhook(ctx, body)
	default:
		return c.sendWebhook(ctx, body)
	}
}

func (c *Client) sendWebhook(ctx context.Context, body payload) error {
	data, err := json.Marshal(c.webhookBody(body))
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.NotifyWebhookURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request)
}

func (c *Client) webhookBody(body payload) any {
	switch c.platform() {
	case "discord":
		return map[string]any{"content": body.Text}
	case "bark":
		return map[string]any{"title": body.Title, "body": body.Text}
	case "wecom", "wechat_work":
		return map[string]any{"msgtype": "text", "text": map[string]string{"content": body.Text}}
	case "dingtalk":
		return map[string]any{"msgtype": "text", "text": map[string]string{"content": body.Text}}
	default:
		return body
	}
}

func (c *Client) sendTelegram(ctx context.Context, body payload) error {
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", url.PathEscape(c.cfg.NotifyTelegramBotToken))
	data, err := json.Marshal(map[string]string{"chat_id": c.cfg.NotifyTelegramChatID, "text": body.Text})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request)
}

func (c *Client) do(request *http.Request) error {
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("notify webhook returned %s", response.Status)
	}
	return nil
}
