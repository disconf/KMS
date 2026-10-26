package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/logger"
)

// 微信开放平台端点。
//
// 说明：以下地址取自微信开放平台公开的 OAuth2 网页授权流程。
// 上线前请对照当前版本的官方文档复核一次（尤其是 scope 取值：
// 网站应用扫码登录用 snsapi_login，公众号内网页授权用 snsapi_userinfo），
// 因为 scope 与可用参数会随应用类型不同而变化。
const (
	endpointQRConnect = "https://open.weixin.qq.com/connect/qrconnect"
	endpointOAuth2    = "https://open.weixin.qq.com/connect/oauth2/authorize"
	endpointToken     = "https://api.weixin.qq.com/sns/oauth2/access_token"
	endpointUserInfo  = "https://api.weixin.qq.com/sns/userinfo"

	// scopeQRLogin 网站应用（扫码登录）
	scopeQRLogin = "snsapi_login"
)

// Official 是真实微信通道。
type Official struct {
	cfg        config.WechatConfig
	httpClient *http.Client
}

// NewOfficial 创建真实微信通道。
func NewOfficial(cfg config.WechatConfig) *Official {
	return &Official{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
}

// Name 实现 Provider。
func (o *Official) Name() string { return "official" }

// AuthorizeURL 实现 Provider，生成扫码登录地址。
func (o *Official) AuthorizeURL(state string) string {
	q := url.Values{}
	q.Set("appid", o.cfg.AppID)
	q.Set("redirect_uri", o.cfg.RedirectURI)
	q.Set("response_type", "code")
	q.Set("scope", scopeQRLogin)
	q.Set("state", state)
	return endpointQRConnect + "?" + q.Encode() + "#wechat_redirect"
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	OpenID       string `json:"openid"`
	Scope        string `json:"scope"`
	UnionID      string `json:"unionid"`
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
}

type userInfoResponse struct {
	OpenID     string `json:"openid"`
	Nickname   string `json:"nickname"`
	Sex        int    `json:"sex"`
	Province   string `json:"province"`
	City       string `json:"city"`
	Country    string `json:"country"`
	HeadImgURL string `json:"headimgurl"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

// Exchange 实现 Provider：code → access_token → 用户信息。
func (o *Official) Exchange(ctx context.Context, code string) (*UserInfo, error) {
	if err := checkCredentials(o.cfg); err != nil {
		return nil, errcode.ErrWechatAuthFailed.WithMsg(err.Error())
	}

	tok, err := o.exchangeToken(ctx, code)
	if err != nil {
		return nil, err
	}

	info := &UserInfo{
		OpenID:   tok.OpenID,
		UnionID:  tok.UnionID,
		Nickname: "微信用户",
	}

	// 拉取昵称/头像失败不应阻断登录，openid 已足以完成账号识别。
	profile, err := o.fetchUserInfo(ctx, tok.AccessToken, tok.OpenID)
	if err != nil {
		logger.Warn("获取微信用户信息失败，仅使用 openid 登录",
			"err", err.Error(), "openid", tok.OpenID)
		return info, nil
	}
	if profile.Nickname != "" {
		info.Nickname = profile.Nickname
	}
	info.Avatar = profile.HeadImgURL
	if profile.UnionID != "" {
		info.UnionID = profile.UnionID
	}
	return info, nil
}

func (o *Official) exchangeToken(ctx context.Context, code string) (*tokenResponse, error) {
	q := url.Values{}
	q.Set("appid", o.cfg.AppID)
	q.Set("secret", o.cfg.AppSecret)
	q.Set("code", code)
	q.Set("grant_type", "authorization_code")

	var out tokenResponse
	if err := o.getJSON(ctx, endpointToken+"?"+q.Encode(), &out); err != nil {
		return nil, err
	}
	if out.ErrCode != 0 {
		return nil, errcode.ErrWechatAuthFailed.WithMsg(
			fmt.Sprintf("微信授权失败(%d): %s", out.ErrCode, out.ErrMsg))
	}
	if out.OpenID == "" {
		return nil, errcode.ErrWechatAuthFailed.WithMsg("微信未返回 openid")
	}
	return &out, nil
}

func (o *Official) fetchUserInfo(ctx context.Context, accessToken, openID string) (*userInfoResponse, error) {
	q := url.Values{}
	q.Set("access_token", accessToken)
	q.Set("openid", openID)
	q.Set("lang", "zh_CN")

	var out userInfoResponse
	if err := o.getJSON(ctx, endpointUserInfo+"?"+q.Encode(), &out); err != nil {
		return nil, err
	}
	if out.ErrCode != 0 {
		return nil, fmt.Errorf("微信返回错误(%d): %s", out.ErrCode, out.ErrMsg)
	}
	return &out, nil
}

func (o *Official) getJSON(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return errcode.ErrInternal.Wrap(fmt.Errorf("构造微信请求失败: %w", err))
	}
	resp, err := o.httpClient.Do(req)
	if err != nil {
		return errcode.ErrThirdParty.Wrap(fmt.Errorf("调用微信接口失败: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()

	// 微信接口在 HTTP 200 下也会返回 errcode，因此这里不按状态码判成功。
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return errcode.ErrThirdParty.Wrap(fmt.Errorf("读取微信响应失败: %w", err))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errcode.ErrThirdParty.Wrap(fmt.Errorf("微信接口返回 HTTP %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body))))
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return errcode.ErrThirdParty.Wrap(fmt.Errorf("解析微信响应失败: %w", err))
	}
	return nil
}
