package notify

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrNotConfigured = errors.New("通知未启用或已保存的渠道配置不完整")
var ErrNotRetryable = errors.New("只能重试失败或结果未知的通知")
var ErrResultNotSaved = errors.New("通知已尝试发送，但结果无法落库；请核对接收端后再重试")

type Delivery struct {
	ID           int64  `json:"id"`
	Kind         string `json:"kind"`
	RetryOf      int64  `json:"retry_of"`
	Platform     string `json:"platform"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	FinishedAt   string `json:"finished_at"`
	ElapsedMS    int64  `json:"elapsed_ms"`
	HTTPStatus   int    `json:"http_status"`
	Summary      string `json:"summary"`
	ErrorMessage string `json:"error_message"`
}

type DeliveryQuery struct {
	Limit  int
	Offset int
	Status string
}

type DeliveryStore interface {
	CreateDelivery(context.Context, Delivery) (Delivery, error)
	FinishDelivery(context.Context, Delivery) error
}

func (c *Client) SetHistory(store DeliveryStore) {
	c.history = store
}

func (c *Client) SendTest(ctx context.Context) (Delivery, error) {
	if !c.enabled() {
		return Delivery{}, ErrNotConfigured
	}
	body := payload{
		Status: "TEST", Title: "通知测试",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Summary:     "模型连通性测试通知", Text: "模型连通性测试通知（不代表模型检测结果）",
		ProviderText: []string{},
	}
	return c.deliver(ctx, "test", 0, body.Summary, body)
}

func (c *Client) Retry(ctx context.Context, previous Delivery) (Delivery, error) {
	if previous.ID <= 0 || (previous.Status != "error" && previous.Status != "unknown") {
		return Delivery{}, ErrNotRetryable
	}
	if !c.enabled() {
		return Delivery{}, ErrNotConfigured
	}
	// Replay only the stored aggregate, never old credentials or upstream error text.
	body := payload{
		Status: "RETRY", Title: "历史通知重试",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Summary:     previous.Summary, ProviderText: []string{},
		Text: fmt.Sprintf("历史通知 #%d（%s）的摘要重发，不代表当前模型状态：\n%s", previous.ID, previous.CreatedAt, previous.Summary),
	}
	return c.deliver(ctx, "retry", previous.ID, previous.Summary, body)
}

func (c *Client) deliver(ctx context.Context, kind string, retryOf int64, summary string, body payload) (Delivery, error) {
	defer c.httpClient.CloseIdleConnections()
	started := time.Now()
	record := Delivery{
		Kind: kind, RetryOf: retryOf, Platform: c.platform(), Status: "sending",
		CreatedAt: started.UTC().Format(time.RFC3339Nano), Summary: summary,
	}
	if c.history != nil {
		var err error
		record, err = c.history.CreateDelivery(ctx, record)
		if err != nil {
			return Delivery{}, errors.New("无法保存通知记录，本次未发送")
		}
	}
	c.httpStatus = 0
	sendErr := c.send(ctx, body)
	record.Status = "success"
	if sendErr != nil {
		record.Status = "error"
		record.ErrorMessage = sendErr.Error()
	}
	record.HTTPStatus = c.httpStatus
	record.ElapsedMS = time.Since(started).Milliseconds()
	record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if c.history != nil {
		// Preserve the result even when the initiating browser disconnects.
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := c.history.FinishDelivery(finishCtx, record); err != nil {
			return record, ErrResultNotSaved
		}
	}
	return record, sendErr
}
