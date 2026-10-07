package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cg/internal/config"
	"cg/internal/httpclient"
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
	ruleID     string
	cfg        config.Config
	stateStore StateStore
	httpClient *http.Client
	history    DeliveryStore
	httpStatus int
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
		httpClient: newHTTPClient(),
	}
}

func newHTTPClient() *http.Client {
	return httpclient.New(10 * time.Second)
}

// SendCheckIfNeeded rejects evidence from a check whose scope is no longer current.
func (c *Client) SendCheckIfNeeded(ctx context.Context, value report.Report, checkedConfig config.Config) error {
	if evidenceScope(checkedConfig) != evidenceScope(c.cfg) {
		// An empty observation resets candidates but preserves sent state and cooldown.
		value = report.Report{}
	}
	return c.SendIfNeeded(ctx, value)
}

func (c *Client) SendIfNeeded(ctx context.Context, value report.Report) error {
	defer c.httpClient.CloseIdleConnections()
	value = filterReport(value, c.cfg.NotifyProviders, c.cfg.NotifyModels)
	current := alertState(value)
	previous, err := c.stateStore.Read()
	if err != nil {
		return err
	}
	if !c.enabled() || current == "unknown" || current == previous.Status || c.cfg.InMaintenance(time.Now()) {
		if previous.Candidate != "" || previous.Consecutive != 0 || previous.CandidateScope != "" {
			previous.Candidate, previous.Consecutive, previous.CandidateScope = "", 0, ""
			return c.stateStore.Write(previous)
		}
		return nil
	}
	if current == "ok" && !c.cfg.NotifyOnRecovery {
		return c.stateStore.Write(State{Status: current, SentAt: previous.SentAt})
	}
	if current == "ok" && previous.Status == "" {
		return c.stateStore.Write(State{Status: current, SentAt: previous.SentAt})
	}
	threshold := max(1, c.cfg.NotifyFailureThreshold)
	if current == "ok" {
		threshold = max(1, c.cfg.NotifyRecoveryThreshold)
	}
	if threshold > 1 {
		scope := evidenceScope(c.cfg)
		if previous.Candidate != current || previous.CandidateScope != scope {
			previous.Candidate, previous.Consecutive = current, 0
		}
		previous.CandidateScope = scope
		previous.Consecutive++
		if previous.Consecutive < threshold {
			return c.stateStore.Write(previous)
		}
	}
	now := time.Now()
	if inCooldown(previous, now, c.cfg.NotifyCooldownMinutes) {
		return nil
	}
	body := buildPayload(value)
	summary := fmt.Sprintf("%s: ok=%d slow=%d error=%d total=%d provider_errors=%d", current, value.OKCount, value.SlowCount, value.ErrorCount, value.Total, len(value.ProviderErrors))
	if _, err := c.deliver(ctx, "alert", 0, summary, body); err != nil {
		return err
	}
	return c.stateStore.Write(State{Status: current, SentAt: now})
}

func (c *Client) enabled() bool {
	switch c.platform() {
	case "disabled":
		return false
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
	if value.ErrorCount > 0 || value.UnknownCount > 0 || len(value.ProviderErrors) > 0 {
		return "error"
	}
	unobserved := false
	for _, provider := range value.Providers {
		if provider.Status == "error" {
			return "error"
		}
		if provider.Status == "unknown" {
			unobserved = true
		}
	}
	// An empty/paused scope or an unchecked provider is not evidence of recovery.
	if unobserved || value.OKCount+value.SlowCount == 0 {
		return "unknown"
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
	if err := config.ValidateWebhookURL(c.cfg.NotifyWebhookURL); err != nil {
		return errors.New("通知地址未通过安全校验")
	}
	data, err := json.Marshal(c.webhookBody(body))
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.NotifyWebhookURL, bytes.NewReader(data))
	if err != nil {
		return errors.New("无法创建通知请求")
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
		return errors.New("无法创建通知请求")
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request)
}

func (c *Client) do(request *http.Request) error {
	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return errors.New("通知请求已取消，接收结果未确认")
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			return errors.New("通知请求超时，接收结果未确认")
		}
		return errors.New("通知连接失败，请检查网络和已保存的渠道配置")
	}
	defer response.Body.Close()
	c.httpStatus = response.StatusCode
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("通知平台返回 HTTP %d", response.StatusCode)
	}
	switch c.platform() {
	case "wecom", "wechat_work", "dingtalk", "telegram", "bark":
		var receipt struct {
			ErrCode *int  `json:"errcode"`
			OK      *bool `json:"ok"`
			Code    *int  `json:"code"`
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
		if err != nil || len(body) > 64<<10 || json.Unmarshal(body, &receipt) != nil {
			return errors.New("通知平台返回无效回执")
		}
		// Do not log the response body: a provider may echo credentials in it.
		switch c.platform() {
		case "telegram":
			if receipt.OK == nil || !*receipt.OK {
				return errors.New("Telegram 拒绝了通知")
			}
		case "bark":
			if receipt.Code == nil || *receipt.Code != 200 {
				return errors.New("Bark 拒绝了通知")
			}
		default:
			if receipt.ErrCode == nil || *receipt.ErrCode != 0 {
				return errors.New("通知平台拒绝了通知")
			}
		}
	}
	return nil
}
