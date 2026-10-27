package mailer

import (
	"context"
	"fmt"

	"github.com/opsexpert/platform/internal/pkg/logger"
)

// Mock 把邮件打到日志里，不真发。
//
// 开发期没有 SMTP 账号也能把注册链路跑通——验证码会出现在
// 应用日志和（expose_code 打开时的）接口响应里。
type Mock struct{}

// NewMock 创建 mock 通道。
func NewMock() *Mock { return &Mock{} }

// Name 实现 Provider。
func (m *Mock) Name() string { return "mock" }

// Send 实现 Provider。
func (m *Mock) Send(_ context.Context, msg Message) error {
	logger.Info("邮件未真发（mock 通道）",
		"to", msg.To, "subject", msg.Subject, "text", msg.Text)
	return nil
}

// String 便于测试断言。
func (m *Mock) String() string { return fmt.Sprint("mailer.Mock") }
