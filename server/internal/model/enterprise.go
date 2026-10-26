package model

import "time"

// 企业状态。
const (
	EnterpriseStatusPending  = "pending"  // 待认证
	EnterpriseStatusApproved = "approved" // 已认证
	EnterpriseStatusDisabled = "disabled" // 已停用
)

// Enterprise 企业客户。MVP 阶段仅建表，V1.5 启用完整能力。
type Enterprise struct {
	Base

	Name         string `gorm:"size:128;index;comment:企业名称" json:"name"`
	CreditCode   string `gorm:"size:64;uniqueIndex;comment:统一社会信用代码" json:"creditCode"`
	ContactName  string `gorm:"size:64;comment:联系人" json:"contactName"`
	ContactPhone string `gorm:"size:20;comment:联系电话" json:"contactPhone"`
	Status       string `gorm:"size:16;index;default:pending;comment:状态" json:"status"`
	Remark       string `gorm:"size:500;comment:备注" json:"remark"`
}

// TableName 固定表名。
func (Enterprise) TableName() string { return "enterprises" }

// 企业工单状态。
const (
	TicketStatusOpen       = "open"       // 待分配
	TicketStatusAssigned   = "assigned"   // 已分配
	TicketStatusProcessing = "processing" // 处理中
	TicketStatusResolved   = "resolved"   // 已解决
	TicketStatusClosed     = "closed"     // 已关闭
)

// 工单优先级。
const (
	TicketPriorityLow    = "low"
	TicketPriorityNormal = "normal"
	TicketPriorityHigh   = "high"
	TicketPriorityUrgent = "urgent"
)

// Ticket 企业工单。
type Ticket struct {
	Base

	EnterpriseID int64 `gorm:"index;comment:企业 ID" json:"enterpriseId"`
	UserID       int64 `gorm:"index;comment:提单人" json:"userId"`
	OrderID      int64 `gorm:"comment:关联订单 ID" json:"orderId"`

	Title       string `gorm:"size:200;comment:标题" json:"title"`
	Description string `gorm:"type:text;comment:描述" json:"description"`
	Priority    string `gorm:"size:16;index;default:normal;comment:优先级" json:"priority"`
	Status      string `gorm:"size:16;index;default:open;comment:状态" json:"status"`

	AssigneeID int64      `gorm:"index;comment:处理人" json:"assigneeId"`
	ResolvedAt *time.Time `gorm:"comment:解决时间" json:"resolvedAt"`
	ClosedAt   *time.Time `gorm:"comment:关闭时间" json:"closedAt"`
}

// TableName 固定表名。
func (Ticket) TableName() string { return "tickets" }

// 订阅状态。
const (
	SubscriptionStatusActive    = "active"
	SubscriptionStatusExpired   = "expired"
	SubscriptionStatusCancelled = "cancelled"
)

// Subscription 企业订阅，按月/年购买专家支持时长。
type Subscription struct {
	Base

	EnterpriseID int64  `gorm:"index;comment:企业 ID" json:"enterpriseId"`
	Plan         string `gorm:"size:64;comment:套餐标识" json:"plan"`

	StartAt time.Time `gorm:"comment:生效时间" json:"startAt"`
	EndAt   time.Time `gorm:"index;comment:到期时间" json:"endAt"`

	QuotaMin int `gorm:"comment:包含支持时长（分钟）" json:"quotaMin"`
	UsedMin  int `gorm:"default:0;comment:已使用时长（分钟）" json:"usedMin"`

	AmountCents int64  `gorm:"comment:订阅金额（分）" json:"amountCents"`
	Status      string `gorm:"size:16;index;default:active;comment:状态" json:"status"`
}

// TableName 固定表名。
func (Subscription) TableName() string { return "subscriptions" }

// RemainingMin 返回剩余可用时长。
func (s *Subscription) RemainingMin() int {
	if s.QuotaMin <= s.UsedMin {
		return 0
	}
	return s.QuotaMin - s.UsedMin
}
