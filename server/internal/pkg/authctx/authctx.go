// Package authctx 负责认证信息在 gin.Context 中的读写，供中间件与 handler 共用。
package authctx

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/pkg/errcode"
)

const (
	keyUserID       = "auth.user_id"
	keyRole         = "auth.role"
	keyExpertStatus = "auth.expert_status"
	keySpeakStatus  = "auth.speak_status"
	keyJTI          = "auth.jti"
)

// Identity 是当前请求的登录身份。
type Identity struct {
	UserID       int64
	Role         string
	ExpertStatus string
	// SpeakStatus 发言权限状态（none/pending/approved/rejected）。
	// 与 Role 分开：角色决定"你是谁"，它决定"你能不能说话"。
	SpeakStatus string
	JTI         string
}

// Set 写入登录身份。
func Set(c *gin.Context, id Identity) {
	c.Set(keyUserID, id.UserID)
	c.Set(keyRole, id.Role)
	c.Set(keyExpertStatus, id.ExpertStatus)
	c.Set(keySpeakStatus, id.SpeakStatus)
	c.Set(keyJTI, id.JTI)
}

// Get 读取登录身份，未登录时 ok 为 false。
func Get(c *gin.Context) (Identity, bool) {
	uid := UserID(c)
	if uid <= 0 {
		return Identity{}, false
	}
	return Identity{
		UserID:       uid,
		Role:         Role(c),
		ExpertStatus: ExpertStatus(c),
		SpeakStatus:  SpeakStatus(c),
		JTI:          JTI(c),
	}, true
}

// UserID 返回当前用户 ID，未登录返回 0。
func UserID(c *gin.Context) int64 {
	if v, ok := c.Get(keyUserID); ok {
		if id, ok := v.(int64); ok {
			return id
		}
	}
	return 0
}

// Role 返回当前角色，未登录返回空串。
func Role(c *gin.Context) string { return getString(c, keyRole) }

// ExpertStatus 返回当前专家的审核状态，非专家返回空串。
func ExpertStatus(c *gin.Context) string { return getString(c, keyExpertStatus) }

// SpeakStatus 返回当前账号的发言权限状态，未登录返回空串。
func SpeakStatus(c *gin.Context) string { return getString(c, keySpeakStatus) }

// CanSpeak 判断当前请求是否有发布内容的资格。
//
// 管理员与专家直接放行（他们是审核链的一环），其余看 speak_status。
// 放在这里而不是各 handler 各写一遍：漏掉一个入口就等于开了一个后门。
func CanSpeak(c *gin.Context) bool {
	role := Role(c)
	if role == "admin" || role == "expert" {
		return true
	}
	return SpeakStatus(c) == "approved"
}

// JTI 返回当前访问令牌的唯一标识，用于登出时吊销。
func JTI(c *gin.Context) string { return getString(c, keyJTI) }

// RequireUserID 返回当前用户 ID，未登录时给出统一的业务错误。
func RequireUserID(c *gin.Context) (int64, error) {
	uid := UserID(c)
	if uid <= 0 {
		return 0, errcode.ErrUnauthorized
	}
	return uid, nil
}

func getString(c *gin.Context, key string) string {
	if v, ok := c.Get(key); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
