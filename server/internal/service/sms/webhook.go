package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// Webhook 把验证码转发到自建短信网关。
//
// 请求约定（你的网关按此实现即可）：
//
//	POST <url>
//	Content-Type: application/json
//	Authorization: Bearer <token>        // token 为空时不带该头
//
//	{"phone":"13800000000","code":"123456","ttlSeconds":300,"scene":"login"}
//
// 网关返回 HTTP 2xx 视为成功，非 2xx 或不含 2xx 视为失败并记录响应体前 512 字节。
type Webhook struct {
	url     string
	token   string
	timeout time.Duration
	client  *http.Client
}

// NewWebhook 创建 Webhook 通道。
func NewWebhook(cfg config.SMSWebhookConfig) *Webhook {
	timeout := cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Webhook{
		url:     cfg.URL,
		token:   cfg.Token,
		timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

// Name 实现 Provider。
func (w *Webhook) Name() string { return "webhook" }

type webhookPayload struct {
	Phone      string `json:"phone"`
	Code       string `json:"code"`
	TTLSeconds int    `json:"ttlSeconds"`
	Scene      string `json:"scene,omitempty"`
}

// Send 实现 Provider。
func (w *Webhook) Send(ctx context.Context, phone, code string, ttl time.Duration) error {
	body, err := json.Marshal(webhookPayload{
		Phone:      phone,
		Code:       code,
		TTLSeconds: int(ttl.Seconds()),
		Scene:      SceneFromContext(ctx),
	})
	if err != nil {
		return errcode.ErrInternal.Wrap(fmt.Errorf("序列化短信请求失败: %w", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return errcode.ErrInternal.Wrap(fmt.Errorf("构造短信请求失败: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	if w.token != "" {
		req.Header.Set("Authorization", "Bearer "+w.token)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return errcode.ErrThirdParty.Wrap(fmt.Errorf("调用短信网关失败: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return errcode.ErrThirdParty.Wrap(fmt.Errorf("短信网关返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet))))
	}
	return nil
}

// sceneCtxKey 用 context 传递业务场景，避免为 Provider 接口增加每次都要忽略的参数。
type sceneCtxKey struct{}

// WithScene 在 context 中标记验证码用途。
func WithScene(ctx context.Context, scene string) context.Context {
	return context.WithValue(ctx, sceneCtxKey{}, scene)
}

// SceneFromContext 读取验证码用途，缺失时返回空串。
func SceneFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(sceneCtxKey{}).(string); ok {
		return v
	}
	return ""
}
