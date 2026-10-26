// Package sms 抽象短信发送能力。
//
// 为什么做成 Provider 接口：本地开发不应该被真实短信通道卡住，
// 生产又必须能对接厂商。业务层只依赖 Provider，通道切换只改配置。
//
// 当前已实现：
//   - mock    ：不真发，把验证码打到日志里，配合 sms.expose_code 可直接联调
//   - webhook ：把验证码 POST 到你自己的短信网关（可用于桥接任意厂商）
//
// 未实现（保留骨架，避免编造未经验证的签名算法）：
//   - aliyun / tencent：需要引入官方 SDK 或按厂商文档实现签名，见 providers.go 内的说明
package sms

import (
	"context"
	"fmt"
	"time"

	"github.com/opsexpert/platform/internal/config"
)

// Provider 是短信通道抽象。
type Provider interface {
	// Name 返回通道名称，用于启动日志与排障。
	Name() string
	// Send 发送验证码短信。ttl 用于向通道声明验证码有效期（部分通道模板需要）。
	Send(ctx context.Context, phone, code string, ttl time.Duration) error
}

// New 按配置装配 Provider。
func New(cfg config.SMSConfig) (Provider, error) {
	switch cfg.Provider {
	case "mock":
		return NewMock(), nil
	case "webhook":
		return NewWebhook(cfg.Webhook), nil
	case "aliyun":
		return NewAliyun(cfg.Aliyun), nil
	case "tencent":
		return NewTencent(cfg.Tencent), nil
	default:
		return nil, fmt.Errorf("未知的短信通道: %s", cfg.Provider)
	}
}
