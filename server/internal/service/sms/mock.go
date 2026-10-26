package sms

import (
	"context"
	"log/slog"
	"time"

	"github.com/opsexpert/platform/internal/pkg/logger"
)

// Mock 是开发环境通道：不发送真实短信，只把验证码写进日志。
//
// 安全边界：mock 通道在生产环境会被 config.Validate 拒绝启动，
// 因此这里的明文日志不会泄露到线上环境。
type Mock struct{}

// NewMock 创建 mock 通道。
func NewMock() *Mock { return &Mock{} }

// Name 实现 Provider。
func (m *Mock) Name() string { return "mock" }

// Send 实现 Provider。
func (m *Mock) Send(_ context.Context, phone, code string, ttl time.Duration) error {
	logger.L().Warn("【开发环境】短信验证码未真实发送",
		slog.String("phone", phone),
		slog.String("code", code),
		slog.Duration("ttl", ttl),
		slog.String("hint", "生产环境请切换 sms.provider 为 webhook/aliyun/tencent"),
	)
	return nil
}
