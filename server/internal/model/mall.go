package model

import "time"

// ============================================================================
// 积分商城。
//
// 积分在引入商城之前是「只进不出」的展示数据（见 user.go 的注释），
// 商城是它第一个消费出口。因此这里的规则比看上去严格：
//
//  1. 扣分与加分一样必须落流水（point_logs，负数分值），
//     users.contribution_points 只是物化结果——对账时流水求和必须能还原余额。
//  2. 扣分用 CAS（WHERE contribution_points >= ?）防并发扣成负数，
//     库存扣减同理（WHERE stock > 0）。
//  3. 兑换单快照积分价格：商品改价不影响已下的单，退款按快照退。
// ============================================================================

// 商城商品类型。
const (
	MallKindDownload   = "download" // 下载解锁（资源文件、安装包）
	MallKindTemplate   = "template" // 模板文档（周报模板、简历模板、复盘模板）
	MallKindRedeemCode = "code"     // 兑换码（合作方权益码，发放时填入）
	MallKindBenefit    = "benefit"  // 站内权益（置顶展示、徽章皮肤等）
)

// MallItem 商城商品。
//
// 商品是运营配置的数据，走软删（Base）：下架改 Active 即可，
// 真删会让历史兑换单的 item 关联悬空。
type MallItem struct {
	Base
	Title       string `gorm:"size:80;not null;comment:商品名" json:"title"`
	Description string `gorm:"size:500;comment:兑换说明" json:"description"`
	Kind        string `gorm:"size:16;not null;default:download;comment:类型 download|template|code|benefit" json:"kind"`
	PointsCost  int64  `gorm:"not null;comment:所需积分" json:"pointsCost"`
	// Stock 剩余库存；Unlimited 为 true 时忽略此字段。
	Stock int `gorm:"default:0;comment:剩余库存" json:"stock"`
	// Unlimited 是否不限量。虚拟权益大多不限量，别让运营手填一个大数。
	Unlimited bool `gorm:"default:false;comment:是否不限量" json:"unlimited"`
	Active    bool `gorm:"default:true;comment:是否上架" json:"active"`
	Sort      int  `gorm:"default:0;comment:排序（小的在前）" json:"sort"`
	// RedeemNote 兑换后如何发放的说明，兑换成功后原样展示给用户。
	RedeemNote string `gorm:"size:300;comment:发放说明" json:"redeemNote"`
}

// TableName 固定表名。
func (MallItem) TableName() string { return "mall_items" }

// 兑换单状态。
const (
	MallRedemptionPending   = "pending"   // 待发放
	MallRedemptionFulfilled = "fulfilled" // 已发放
	MallRedemptionCancelled = "cancelled" // 已取消（积分已退回）
)

// MallRedemption 兑换单。
//
// 状态机：pending → fulfilled（发放）；pending → cancelled（退分）。
// fulfilled 与 cancelled 都是终态，互相不可达——发放后要收回走线下，
// 不能从这里退分，否则积分可以刷。
type MallRedemption struct {
	Base
	UserID int64 `gorm:"index;not null;comment:兑换人" json:"userId"`
	ItemID int64 `gorm:"index;not null;comment:商品 ID" json:"itemId"`

	// 快照：商品可能改名/改价/下架，兑换单只认下单那一刻。
	ItemTitle  string `gorm:"size:80;not null;comment:商品名快照" json:"itemTitle"`
	PointsCost int64  `gorm:"not null;comment:积分快照" json:"pointsCost"`

	Status string `gorm:"size:16;not null;default:pending;index;comment:pending|fulfilled|cancelled" json:"status"`
	// Remark 发放凭证（兑换码文本 / 文件链接 / 说明），管理员发放时填写。
	Remark string `gorm:"size:500;comment:发放凭证" json:"remark"`

	FulfilledAt *time.Time `gorm:"comment:发放时间" json:"fulfilledAt"`
	CancelledAt *time.Time `gorm:"comment:取消时间" json:"cancelledAt"`
}

// TableName 固定表名。
func (MallRedemption) TableName() string { return "mall_redemptions" }
