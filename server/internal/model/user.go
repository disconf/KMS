package model

import "time"

// 用户角色。
const (
	RoleUser   = "user"   // 普通求助用户
	RoleExpert = "expert" // 专家（同时具备用户能力）
	RoleAdmin  = "admin"  // 平台运营
)

// 用户状态。
const (
	UserStatusActive   = "active"   // 正常
	UserStatusDisabled = "disabled" // 已封禁
)

// User 平台账号。
//
// Phone / WxOpenID 均为可空：微信登录可以先只带 openid 建号，之后再绑手机号；
// 手机号注册的用户也可以始终不绑微信。用指针而非空串是必须的——
// 唯一索引下 MySQL 与 SQLite 都允许多个 NULL，却不允许多个空串。
type User struct {
	Base

	Phone     *string `gorm:"size:20;uniqueIndex;comment:手机号" json:"-"`
	WxOpenID  string  `gorm:"size:64;index;comment:微信 openid" json:"-"`
	WxUnionID string  `gorm:"size:64;index;comment:微信 unionid" json:"-"`

	Nickname string `gorm:"size:64;comment:昵称" json:"nickname"`
	Avatar   string `gorm:"size:512;comment:头像地址" json:"avatar"`
	Email    string `gorm:"size:128;comment:邮箱" json:"email"`

	Role   string `gorm:"size:16;index;default:user;comment:角色 user/expert/admin" json:"role"`
	Status string `gorm:"size:16;index;default:active;comment:状态 active/disabled" json:"status"`

	// 封禁留痕。原因必须落库：半年后有人来申诉，只看到 status=disabled
	// 却没人说得清当初为什么封，结果往往是先解封再说——等于封禁白做。
	// 两个字段只在后台 DTO 中出现，不下发到前台。
	StatusReason    string     `gorm:"size:255;comment:最近一次状态变更原因" json:"-"`
	StatusChangedAt *time.Time `gorm:"comment:最近一次状态变更时间" json:"-"`

	// PasswordHash 后台口令的 bcrypt 哈希，空串表示该账号没有口令。
	//
	// 普通用户仍然是短信验证码登录，不设口令；口令登录是给管理员与运营用的
	// 应急/首选入口——线上短信通道尚未接通时必须有一条能进后台的路，
	// 否则内容一旦发布就再也改不了。空串账号即使拿着正确口令也登不进去，
	// 这是"默认关闭"的开关，不需要额外的布尔字段。
	PasswordHash string `gorm:"size:100;comment:登录口令 bcrypt 哈希，空=未设置" json:"-"`

	LastLoginAt *time.Time `gorm:"comment:最近登录时间" json:"lastLoginAt"`
	LastLoginIP string     `gorm:"size:64;comment:最近登录 IP" json:"-"`

	// ContributionPoints 社区贡献积分，只做展示（徽章/等级），不兑换任何权益。
	//
	// 冗余在用户表是为了列表与回答卡片能直接带出等级：一页 20 条回答
	// 逐条去 point_logs 求和，为的只是画一个小徽章，不值。
	// 流水（point_logs）才是真相，这里是它的物化结果。
	ContributionPoints int64 `gorm:"default:0;comment:贡献积分（展示用）" json:"contributionPoints"`
}

// TableName 固定表名，避免 GORM 复数化规则变动影响线上表。
func (User) TableName() string { return "users" }

// PhoneValue 返回手机号明文，未绑定时返回空串。
func (u *User) PhoneValue() string {
	if u.Phone == nil {
		return ""
	}
	return *u.Phone
}

// HasPhone 判断是否已绑定手机号。
func (u *User) HasPhone() bool { return u.PhoneValue() != "" }

// IsActive 判断账号是否可用。
func (u *User) IsActive() bool { return u.Status == UserStatusActive }

// IsAdmin 判断是否平台管理员。
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// CanLogin 判断账号是否允许换取令牌。
func (u *User) CanLogin() bool { return u.IsActive() }

// ValidUserRole 判断角色取值是否在白名单内。
//
// 不做法定的「角色机」：user/expert/admin 是并列角色而非流转关系，
// 但后台改角色必须挡住拼错的字符串，否则会写进一个既不认管理员也不认专家的角色，
// 用户看起来正常、实际上所有受保护接口都拒绝他。
func ValidUserRole(role string) bool {
	switch role {
	case RoleUser, RoleExpert, RoleAdmin:
		return true
	}
	return false
}

// ValidUserStatus 判断状态取值是否在白名单内。
func ValidUserStatus(status string) bool {
	return status == UserStatusActive || status == UserStatusDisabled
}

// MaskedPhone 返回脱敏手机号，用于日志与前台展示。
func (u *User) MaskedPhone() string { return maskPhone(u.PhoneValue()) }

func maskPhone(phone string) string {
	if len(phone) < 7 {
		return phone
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}

// MaskPhone 供其他包复用（如后台列表页）。
func MaskPhone(phone string) string { return maskPhone(phone) }

// StrPtr 返回字符串指针，便于在构造模型时传值。
func StrPtr(s string) *string { return &s }
