// Package mailer 抽象"发一封邮件"这件事。
//
// 和 sms 包同样的取舍：业务层只依赖 Provider，
// 开发期用 mock（验证码写日志），生产换成 smtp，业务代码一行不改。
//
// 只实现 smtp 一种真实通道，不接各厂商 HTTP API：
// 邮箱注册只需要发验证码，SMTP 是唯一通用协议，
// QQ 邮箱、163、腾讯企业邮、阿里云邮件推送都支持，接一家等于接了全部。
package mailer

import (
	"context"
	"fmt"
	"time"

	"github.com/opsexpert/platform/internal/config"
)

// Message 一封待发送的邮件。
//
// Text 与 HTML 都给：只发 HTML 会被部分老客户端判为垃圾，
// 只发纯文本又做不出像样的验证码卡片，所以两者都提供，
// 由客户端自己挑（multipart/alternative）。
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Provider 是邮件通道抽象。
type Provider interface {
	// Name 返回通道名称，用于启动日志与排障。
	Name() string
	// Send 发送一封邮件。
	Send(ctx context.Context, msg Message) error
}

// New 按配置装配 Provider。
func New(cfg config.MailConfig) (Provider, error) {
	switch cfg.Provider {
	case "mock":
		return NewMock(), nil
	case "smtp":
		return NewSMTP(cfg.SMTP), nil
	default:
		return nil, fmt.Errorf("未知的邮件通道: %s", cfg.Provider)
	}
}

// VerifyCodeMessage 生成验证码邮件的内容。
//
// 放在 mailer 而不是业务层：模板改了不影响业务逻辑，
// 而且以后加"登录提醒""密码变更通知"时，模板集中在一处。
func VerifyCodeMessage(to, code, scene string, ttl time.Duration) Message {
	purpose := "登录"
	switch scene {
	case "register":
		purpose = "注册账号"
	case "bind":
		purpose = "绑定邮箱"
	}
	minutes := int(ttl.Minutes())
	if minutes <= 0 {
		minutes = 10
	}

	text := fmt.Sprintf(
		"你的验证码是 %s\n\n用于%s，%d 分钟内有效。\n不是本人操作请忽略本邮件。",
		code, purpose, minutes)

	html := fmt.Sprintf(`<!DOCTYPE html><html><body style="margin:0;padding:24px;background:#f5f6f8;font-family:-apple-system,'PingFang SC','Microsoft YaHei',sans-serif">
<div style="max-width:480px;margin:0 auto;background:#fff;border-radius:12px;padding:32px">
  <p style="margin:0 0 8px;font-size:14px;color:#606266">个人工作经验分享</p>
  <h2 style="margin:0 0 24px;font-size:20px;color:#1f2329">%s验证码</h2>
  <div style="font-size:32px;letter-spacing:8px;font-weight:700;color:#1a6ff5;padding:16px 0">%s</div>
  <p style="margin:16px 0 0;font-size:14px;color:#606266">%d 分钟内有效。不是本人操作请忽略本邮件。</p>
</div></body></html>`, purpose, code, minutes)

	return Message{
		To:      to,
		Subject: fmt.Sprintf("【个人工作经验分享】%s验证码 %s", purpose, code),
		Text:    text,
		HTML:    html,
	}
}
