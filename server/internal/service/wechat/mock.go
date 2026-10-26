package wechat

import (
	"context"
	"fmt"
	"strings"

	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// mockPrefix 是 mock code 的前缀。
const mockPrefix = "mock:"

// Mock 是联调用的微信通道。
type Mock struct{}

// NewMock 创建 mock 通道。
func NewMock() *Mock { return &Mock{} }

// Name 实现 Provider。
func (m *Mock) Name() string { return "mock" }

// AuthorizeURL 实现 Provider，直接返回前端回跳地址并带上 mock code。
func (m *Mock) AuthorizeURL(state string) string {
	openID := "mock_openid_" + strings.TrimSpace(state)
	if state == "" {
		openID = "mock_openid_default"
	}
	return "/auth/wechat/callback?code=" + mockPrefix + openID + "&state=" + state
}

// Exchange 实现 Provider，解析 "mock:<openid>"。
func (m *Mock) Exchange(_ context.Context, code string) (*UserInfo, error) {
	if !strings.HasPrefix(code, mockPrefix) {
		return nil, errcode.ErrWechatAuthFailed.WithMsg(
			fmt.Sprintf("mock 模式要求 code 形如 %q，当前收到 %q", mockPrefix+"<openid>", code))
	}
	openID := strings.TrimPrefix(code, mockPrefix)
	if openID == "" {
		return nil, errcode.ErrWechatAuthFailed.WithMsg("mock code 缺少 openid")
	}
	return &UserInfo{
		OpenID:   openID,
		UnionID:  "mock_union_" + openID,
		Nickname: "微信用户",
		Avatar:   "",
	}, nil
}
