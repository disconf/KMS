package model

import "time"

// 结算状态。
const (
	SettlementStatusPending = "pending" // 待结算
	SettlementStatusSettled = "settled" // 已结算
	SettlementStatusFailed  = "failed"  // 结算失败
)

// Settlement 平台对单笔订单给专家的分账记录。
type Settlement struct {
	Base

	SettlementNo string `gorm:"size:32;uniqueIndex;comment:结算单号" json:"settlementNo"`
	// OrderID 用唯一索引而不是普通索引：一单只能有一条分账记录。
	// 这是"不会重复给专家打钱"的最后一道防线——定时任务重跑、
	// 两个人同时点结算，都由数据库来裁决，不靠应用层先查一遍。
	OrderID  int64  `gorm:"uniqueIndex;comment:订单 ID（一单一条分账记录）" json:"orderId"`
	OrderNo  string `gorm:"size:32;index;comment:订单号快照" json:"orderNo"`
	ExpertID int64  `gorm:"index;comment:专家用户 ID" json:"expertId"`

	AmountCents       int64 `gorm:"comment:订单总额（分）" json:"amountCents"`
	CommissionCents   int64 `gorm:"comment:平台佣金（分）" json:"commissionCents"`
	ExpertAmountCents int64 `gorm:"comment:专家实收（分）" json:"expertAmountCents"`
	CommissionRateBP  int   `gorm:"comment:抽佣基点快照" json:"commissionRateBp"`

	Status    string     `gorm:"size:16;index;default:pending;comment:状态" json:"status"`
	SettledAt *time.Time `gorm:"comment:结算时间" json:"settledAt"`
	Remark    string     `gorm:"size:255;comment:备注" json:"remark"`
}

// TableName 固定表名。
func (Settlement) TableName() string { return "settlements" }

// 提现状态。
const (
	WithdrawalStatusPending  = "pending"  // 待审核
	WithdrawalStatusApproved = "approved" // 审核通过，待打款
	WithdrawalStatusPaid     = "paid"     // 已打款
	WithdrawalStatusRejected = "rejected" // 已驳回
	WithdrawalStatusFailed   = "failed"   // 打款失败
)

// 收款账户类型。
const (
	AccountTypeBank   = "bank"   // 银行卡
	AccountTypeAlipay = "alipay" // 支付宝
	AccountTypeWechat = "wechat" // 微信零钱
)

// withdrawalTransitions 提现状态机。
//
// 与订单一样，流转规则只有这一份：service 与 handler 都必须走 CanTransitionTo，
// 不许在别处再写一遍 if status == pending。
//
//	pending ──通过──→ approved ──打款完成──→ paid
//	   │                  └────打款失败──→ failed
//	   └──驳回──→ rejected
//
// rejected / failed 都是「把钱退回可提现余额」；paid 是「冻结金额被扣掉，钱已离开平台」。
// 三条终态都不允许再改，否则同一笔钱会被退两次。
var withdrawalTransitions = map[string][]string{
	WithdrawalStatusPending:  {WithdrawalStatusApproved, WithdrawalStatusRejected},
	WithdrawalStatusApproved: {WithdrawalStatusPaid, WithdrawalStatusFailed},
	WithdrawalStatusRejected: nil,
	WithdrawalStatusPaid:     nil,
	WithdrawalStatusFailed:   nil,
}

// CanTransitionTo 判断提现能否从当前状态流转到目标状态。
func (w *Withdrawal) CanTransitionTo(target string) bool {
	for _, next := range withdrawalTransitions[w.Status] {
		if next == target {
			return true
		}
	}
	return false
}

// ReleasesFrozen 判断流转到目标状态时是否应当把冻结金额退回可提现余额。
//
// 驳回与打款失败都要退；通过（钱仍在冻结里）与已打款（冻结被扣掉）都不退。
func (w *Withdrawal) ReleasesFrozen(target string) bool {
	return target == WithdrawalStatusRejected || target == WithdrawalStatusFailed
}

// Withdrawal 专家提现申请。
type Withdrawal struct {
	Base

	WithdrawalNo string `gorm:"size:32;uniqueIndex;comment:提现单号" json:"withdrawalNo"`
	ExpertID     int64  `gorm:"index;comment:专家用户 ID" json:"expertId"`

	AmountCents int64 `gorm:"comment:申请金额（分）" json:"amountCents"`
	FeeCents    int64 `gorm:"default:0;comment:手续费（分）" json:"feeCents"`
	ActualCents int64 `gorm:"comment:实际到账（分）" json:"actualCents"`

	AccountType     string `gorm:"size:16;comment:账户类型" json:"accountType"`
	AccountName     string `gorm:"size:64;comment:收款人姓名" json:"accountName"`
	AccountNoMasked string `gorm:"size:64;comment:收款账号（脱敏存储）" json:"accountNoMasked"`

	Status     string     `gorm:"size:16;index;default:pending;comment:状态" json:"status"`
	Remark     string     `gorm:"size:255;comment:备注/驳回原因" json:"remark"`
	ReviewedAt *time.Time `gorm:"comment:审核时间" json:"reviewedAt"`
	ReviewedBy int64      `gorm:"comment:审核人用户 ID" json:"-"`
	PaidAt     *time.Time `gorm:"comment:打款时间" json:"paidAt"`
}

// TableName 固定表名。
func (Withdrawal) TableName() string { return "withdrawals" }

// 争议状态。
const (
	DisputeStatusOpen       = "open"       // 待受理
	DisputeStatusProcessing = "processing" // 处理中
	DisputeStatusResolved   = "resolved"   // 已解决
	DisputeStatusRejected   = "rejected"   // 已驳回
)

// Dispute 订单争议，方案约定 3 个工作日处理。
type Dispute struct {
	Base

	OrderID  int64 `gorm:"index;comment:订单 ID" json:"orderId"`
	UserID   int64 `gorm:"index;comment:发起人" json:"userId"`
	ExpertID int64 `gorm:"index;comment:被投诉专家" json:"expertId"`

	Reason   string     `gorm:"size:500;comment:争议原因" json:"reason"`
	Evidence StringList `gorm:"comment:证据材料" json:"evidence"`

	Status    string     `gorm:"size:16;index;default:open;comment:状态" json:"status"`
	Result    string     `gorm:"size:1000;comment:处理结果" json:"result"`
	HandlerID int64      `gorm:"comment:处理人用户 ID" json:"handlerId"`
	HandledAt *time.Time `gorm:"comment:处理完成时间" json:"handledAt"`
	Deadline  *time.Time `gorm:"comment:承诺处理截止时间" json:"deadline"`
}

// TableName 固定表名。
func (Dispute) TableName() string { return "disputes" }
