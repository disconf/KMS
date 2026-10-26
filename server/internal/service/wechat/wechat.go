// Package wechat 封装微信开放平台授权登录。
//
// 支持两种模式：
//   - mock：code 形如 "mock:<openid>"，直接映射为用户，用于前后端联调（生产被配置校验禁止）
//   - real：走微信开放平台网站应用/公众号网页授权，code 换 access_token 再取用户信息
package wechat

import (
	"context"
	"fmt"

	"github.com/opsexpert/platform/internal/config"
)

// UserInfo 是微信返回的用户标识与资料。
type UserInfo struct {
	OpenID   string
	UnionID  string
	Nickname string
	Avatar   string
}

// Provider 是微信授权能力抽象。
type Provider interface {
	// Name 返回模式名称。
	Name() string
	// AuthorizeURL 生成前端跳转的授权地址；state 用于防 CSRF。
	AuthorizeURL(state string) string
	// Exchange 用授权 code 换取用户信息。
	Exchange(ctx context.Context, code string) (*UserInfo, error)
}

// New 按配置装配 Provider。
func New(cfg config.WechatConfig) Provider {
	if cfg.Mock {
		return NewMock()
	}
	return NewOfficial(cfg)
}

// checkCredentials 校验真实模式所需的凭据是否齐全。
func checkCredentials(cfg config.WechatConfig) error {
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return fmt.Errorf("微信 AppID / AppSecret 未配置")
	}
	return nil
}
