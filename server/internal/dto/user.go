package dto

import "time"

// UpdateProfileReq 更新个人资料。指针字段实现"未传即不改"的局部更新语义。
type UpdateProfileReq struct {
	Nickname *string `json:"nickname"`
	Avatar   *string `json:"avatar"`
	Email    *string `json:"email"`
}

const (
	nicknameMinLen = 2
	nicknameMaxLen = 20
	avatarMaxLen   = 512
	emailMaxLen    = 128
)

// NicknameLen 返回昵称长度约束，供 service 层校验复用。
func NicknameLen() (int, int) { return nicknameMinLen, nicknameMaxLen }

// AvatarMaxLen 返回头像地址长度上限。
func AvatarMaxLen() int { return avatarMaxLen }

// EmailMaxLen 返回邮箱长度上限。
func EmailMaxLen() int { return emailMaxLen }

// ============================ 后台用户治理 ============================

// AdminUserListQuery 后台用户列表查询。
type AdminUserListQuery struct {
	PageQuery

	Keyword string `form:"keyword"`
	Role    string `form:"role" binding:"omitempty,oneof=user expert admin"`
	Status  string `form:"status" binding:"omitempty,oneof=active disabled"`
}

// AdminUserItem 后台用户列表项。
//
// 手机号是脱敏的：后台列表页经常被投屏、被截图发群里，
// 一个能直接看到全部号码的列表就是一份通讯录泄露。
// 真要核对身份，走"按完整号码搜索"——那条路径会留下查询记录。
type AdminUserItem struct {
	ID       int64  `json:"id"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	// MaskedPhone 形如 138****8000；未绑定时为空串。
	MaskedPhone string `json:"maskedPhone"`
	Email       string `json:"email"`

	Role            string     `json:"role"`
	Status          string     `json:"status"`
	StatusReason    string     `json:"statusReason"`
	StatusChangedAt *time.Time `json:"statusChangedAt"`

	// ExpertID > 0 表示这个人有专家档案；ExpertStatus 为审核状态。
	ExpertID     int64  `json:"expertId"`
	ExpertStatus string `json:"expertStatus"`

	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
}

// UserOrderStats 用户的订单聚合，后台判断"封了他会不会卡住在途订单"用。
type UserOrderStats struct {
	AsUserTotal         int64 `json:"asUserTotal"`
	AsUserPaidCents     int64 `json:"asUserPaidCents"`
	AsExpertTotal       int64 `json:"asExpertTotal"`
	AsExpertIncomeCents int64 `json:"asExpertIncomeCents"`
	ActiveCount         int64 `json:"activeCount"`
}

// AdminUserExpert 专家档案摘要（仅当该用户是专家时返回）。
type AdminUserExpert struct {
	ID                int64      `json:"id"`
	RealName          string     `json:"realName"`
	Status            string     `json:"status"`
	Rating            float64    `json:"rating"`
	OrderCount        int64      `json:"orderCount"`
	BalanceCents      int64      `json:"balanceCents"`
	FrozenCents       int64      `json:"frozenCents"`
	TotalIncomeCents  int64      `json:"totalIncomeCents"`
	AcceptingOrder    bool       `json:"acceptingOrder"`
	PricePerHourCents int64      `json:"pricePerHourCents"`
	PricePerTaskCents int64      `json:"pricePerTaskCents"`
	AuditRemark       string     `json:"auditRemark"`
	AppliedAt         time.Time  `json:"appliedAt"`
	AuditedAt         *time.Time `json:"auditedAt"`
}

// AdminAuditItem 一条审计记录。
type AdminAuditItem struct {
	ID        int64          `json:"id"`
	Action    string         `json:"action"`
	UserRole  string         `json:"userRole"`
	Detail    map[string]any `json:"detail"`
	IP        string         `json:"ip"`
	CreatedAt time.Time      `json:"createdAt"`
}

// AdminUserDetail 后台用户详情。
type AdminUserDetail struct {
	AdminUserItem

	OrderStats UserOrderStats   `json:"orderStats"`
	Expert     *AdminUserExpert `json:"expert"`
	AuditLogs  []AdminAuditItem `json:"auditLogs"`
}

// AdminUpdateStatusReq 封禁 / 解封。
//
// 用显式的 status 而不是 disabled:true —— 布尔字段缺省即 false，
// 请求体少一个字段就会把「封禁」变成「解封」。在封号这种事上，
// 默认值必须是"什么都不做"，而不是"反向执行"。
type AdminUpdateStatusReq struct {
	Status string `json:"status" binding:"required,oneof=active disabled"`
	Reason string `json:"reason"`
}

// AdminUpdateRoleReq 修改角色。
type AdminUpdateRoleReq struct {
	Role string `json:"role" binding:"required,oneof=user expert admin"`
}
