package dto

import "time"

// 验证码使用场景。
const (
	SceneLogin    = "login"    // 登录
	SceneBind     = "bind"     // 绑定手机号
	SceneRegister = "register" // 邮箱注册
)

// ValidMailScene 判断邮件验证码场景是否合法。
func ValidMailScene(scene string) bool {
	return scene == SceneLogin || scene == SceneRegister || scene == SceneBind
}

// SendEmailCodeReq 发送邮箱验证码。
type SendEmailCodeReq struct {
	Email string `json:"email" binding:"required" example:"me@example.com"`
	Scene string `json:"scene" binding:"omitempty,oneof=login bind register" example:"register"`
}

// RegisterEmailReq 邮箱注册：邮箱 + 验证码 + 自设密码。
//
// 不设"确认密码"字段：那是给手滑留的保险，前端做一次比对即可，
// 后端再要一遍只是多一个能填错的字段。
type RegisterEmailReq struct {
	Email    string `json:"email" binding:"required" example:"me@example.com"`
	Code     string `json:"code" binding:"required" example:"123456"`
	Password string `json:"password" binding:"required,min=8,max=64" example:"MyPass1234"`
	// Nickname 可选，留空取邮箱前缀
	Nickname string `json:"nickname" example:"老王"`
}

// LoginEmailReq 邮箱 + 密码登录。
type LoginEmailReq struct {
	Email    string `json:"email" binding:"required" example:"me@example.com"`
	Password string `json:"password" binding:"required,max=64" example:"MyPass1234"`
}

// LoginEmailCodeReq 邮箱 + 验证码登录。
type LoginEmailCodeReq struct {
	Email string `json:"email" binding:"required" example:"me@example.com"`
	Code  string `json:"code" binding:"required" example:"123456"`
}

// SendSMSCodeReq 发送短信验证码。
type SendSMSCodeReq struct {
	Phone string `json:"phone" binding:"required" example:"13800000000"`
	Scene string `json:"scene" binding:"omitempty,oneof=login bind" example:"login"`
}

// SendSMSCodeResp 发送结果。
type SendSMSCodeResp struct {
	// ExpiresIn 验证码有效期（秒）
	ExpiresIn int `json:"expiresIn" example:"300"`
	// Interval 下次可发送的间隔（秒）
	Interval int `json:"interval" example:"60"`
	// DevCode 仅 dev/test 且 sms.expose_code=true 时返回，生产环境永远为空
	DevCode string `json:"devCode,omitempty" example:"123456"`
}

// LoginPasswordReq 手机号 + 口令登录。
//
// 不自动注册：没有口令的账号一律走验证码。这里的定位是"已有后台账号的
// 人员登录"，不是新用户注册通道。
type LoginPasswordReq struct {
	Phone    string `json:"phone" binding:"required" example:"13800000000"`
	Password string `json:"password" binding:"required,min=6,max=64" example:"Admin@12345"`
}

// ChangePasswordReq 修改自己的登录口令。
type ChangePasswordReq struct {
	OldPassword string `json:"oldPassword" binding:"required,max=64" example:"Admin@12345"`
	NewPassword string `json:"newPassword" binding:"required,min=8,max=64" example:"N3w-Strong-Pass"`
}

// LoginSMSReq 手机号 + 验证码登录，账号不存在时自动注册。
type LoginSMSReq struct {
	Phone string `json:"phone" binding:"required" example:"13800000000"`
	Code  string `json:"code" binding:"required" example:"123456"`
	// Nickname 可选，仅首次注册时使用
	Nickname string `json:"nickname" example:"老王"`
}

// WechatAuthURLResp 微信授权地址。
type WechatAuthURLResp struct {
	URL   string `json:"url"`
	State string `json:"state"`
}

// LoginWechatReq 微信登录。mock 模式下 code 可传 "mock:<openid>"。
type LoginWechatReq struct {
	Code  string `json:"code" binding:"required"`
	State string `json:"state"`
	// Nickname / Avatar 来自微信用户信息授权，可选
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Phone    string `json:"phone"`
	SmsCode  string `json:"smsCode"`
}

// RefreshReq 刷新访问令牌。
type RefreshReq struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

// LogoutReq 登出。refreshToken 可选，传入则同时使刷新令牌失效。
type LogoutReq struct {
	RefreshToken string `json:"refreshToken"`
}

// TokenPair 令牌组。
type TokenPair struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType" example:"Bearer"`
	// ExpiresIn 访问令牌有效期（秒）
	ExpiresIn    int    `json:"expiresIn" example:"7200"`
	RefreshToken string `json:"refreshToken"`
	// RefreshExpiresIn 刷新令牌有效期（秒）
	RefreshExpiresIn int `json:"refreshExpiresIn" example:"604800"`
}

// AuthResp 登录 / 刷新返回。
type AuthResp struct {
	Token     TokenPair   `json:"token"`
	User      UserProfile `json:"user"`
	IsNewUser bool        `json:"isNewUser"`
}

// UserProfile 对外用户信息。手机号已脱敏，openid / 身份证号等不输出。
type UserProfile struct {
	ID           int64     `json:"id"`
	Phone        string    `json:"phone"`
	Nickname     string    `json:"nickname"`
	Avatar       string    `json:"avatar"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	ExpertStatus string    `json:"expertStatus,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`

	// SpeakStatus 决定前端要不要把"发布"按钮变成"申请发言"。
	// 放在 profile 里而不是单独接口：每个页面都要据此渲染，
	// 多一次请求换不来什么，反而多一个可能失败的依赖。
	SpeakStatus     string     `json:"speakStatus"`
	SpeakReviewNote string     `json:"speakReviewNote,omitempty"`
	SpeakAppliedAt  *time.Time `json:"speakAppliedAt,omitempty"`
	// CanSpeak 是服务端算好的最终结论，前端不用自己拼角色与状态。
	CanSpeak bool `json:"canSpeak"`
}
