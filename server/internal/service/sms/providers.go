package sms

import (
	"context"
	"fmt"
	"time"

	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// 关于 aliyun / tencent 两个通道的说明
// ---------------------------------------------------------------------------
// 这里刻意只保留骨架，不实现发送逻辑，原因：
//   两家厂商的短信 API 都依赖"签名算法 + 模板参数"这类细节，
//   签名算法与参数名会随 SDK/接口版本变化，凭记忆手写极易出现
//   线上才暴露的签名错误，且无法在本地验证。
//
// 落地时二选一，改动只在这个文件里：
//  1) 引入官方 SDK（推荐，签名由 SDK 负责）
//     - 阿里云：github.com/alibabacloud-go/dysmsapi-20170525/v3
//     - 腾讯云：github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111
//     然后在 Send 内构造请求并调用。config.SMSAliyunConfig / SMSTencentConfig
//     里已经预留了 AccessKey、SignName、TemplateCode 等字段。
//  2) 不引入 SDK：让内部网关统一对接厂商，
//     把 sms.provider 设为 webhook，指向网关即可（Webhook 通道已实现）。
//
// 两条路径都不用改业务代码，业务只依赖 Provider 接口。
// ---------------------------------------------------------------------------

// Aliyun 阿里云短信通道占位实现。
type Aliyun struct {
	cfg config.SMSAliyunConfig
}

// NewAliyun 创建阿里云通道。
func NewAliyun(cfg config.SMSAliyunConfig) *Aliyun { return &Aliyun{cfg: cfg} }

// Name 实现 Provider。
func (a *Aliyun) Name() string { return "aliyun" }

// Send 实现 Provider，当前返回未实现错误并给出明确指引。
func (a *Aliyun) Send(_ context.Context, _, _ string, _ time.Duration) error {
	if a.cfg.AccessKeyID == "" || a.cfg.AccessKeySecret == "" {
		return errcode.ErrNotImplemented.WithMsg("阿里云短信通道未配置 AccessKey，无法发送")
	}
	return errcode.ErrNotImplemented.WithMsg(
		"阿里云短信通道尚未接入：请引入 dysmsapi SDK 后实现 sms.Aliyun.Send，或改用 sms.provider=webhook")
}

// Tencent 腾讯云短信通道占位实现。
type Tencent struct {
	cfg config.SMSTencentConfig
}

// NewTencent 创建腾讯云通道。
func NewTencent(cfg config.SMSTencentConfig) *Tencent { return &Tencent{cfg: cfg} }

// Name 实现 Provider。
func (t *Tencent) Name() string { return "tencent" }

// Send 实现 Provider，当前返回未实现错误并给出明确指引。
func (t *Tencent) Send(_ context.Context, _, _ string, _ time.Duration) error {
	if t.cfg.SecretID == "" || t.cfg.SecretKey == "" {
		return errcode.ErrNotImplemented.WithMsg("腾讯云短信通道未配置 SecretId，无法发送")
	}
	return errcode.ErrNotImplemented.WithMsg(
		"腾讯云短信通道尚未接入：请引入 sms SDK 后实现 sms.Tencent.Send，或改用 sms.provider=webhook")
}

// Describe 返回通道当前是否可直接投递，供启动日志使用。
func Describe(p Provider) string {
	switch v := p.(type) {
	case *Mock:
		return "mock（开发环境，不真实发送）"
	case *Webhook:
		_ = v
		return "webhook（已配置网关）"
	case *Aliyun:
		if v.cfg.AccessKeyID == "" {
			return "aliyun（未配置，不可用）"
		}
		return "aliyun（已配置凭据，但发送逻辑待接入）"
	case *Tencent:
		if v.cfg.SecretID == "" {
			return "tencent（未配置，不可用）"
		}
		return "tencent（已配置凭据，但发送逻辑待接入）"
	default:
		return fmt.Sprintf("未知通道 %T", p)
	}
}
