package dto

import "time"

// ============================================================================
// 积分商城。
// ============================================================================

// MallItemItem 商品卡片。
type MallItemItem struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	KindLabel   string `json:"kindLabel"`
	PointsCost  int64  `json:"pointsCost"`
	Stock       int    `json:"stock"`
	Unlimited   bool   `json:"unlimited"`
	Active      bool   `json:"active"`
	SoldOut     bool   `json:"soldOut"`
	RedeemNote  string `json:"redeemNote"`
}

// MallListResp 商城页数据。
type MallListResp struct {
	// MyPoints 当前登录人的积分余额；未登录为 -1，前端据此显示登录引导。
	MyPoints int64          `json:"myPoints"`
	Items    []MallItemItem `json:"items"`
}

// MallRedemptionItem 兑换记录。
type MallRedemptionItem struct {
	ID          int64      `json:"id"`
	ItemTitle   string     `json:"itemTitle"`
	PointsCost  int64      `json:"pointsCost"`
	Status      string     `json:"status"`
	StatusLabel string     `json:"statusLabel"`
	Remark      string     `json:"remark"`
	CreatedAt   time.Time  `json:"createdAt"`
	FulfilledAt *time.Time `json:"fulfilledAt"`
}

// MallRedeemResp 兑换成功返回。
type MallRedeemResp struct {
	Redemption MallRedemptionItem `json:"redemption"`
	// PointsAfter 兑换后的最新余额，前端就地更新徽章，不用再拉一次 /me。
	PointsAfter int64 `json:"pointsAfter"`
}

// MallAdminItemUpsert 后台商品新建/编辑入参。
type MallAdminItemUpsert struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	PointsCost  int64  `json:"pointsCost"`
	Stock       int    `json:"stock"`
	Unlimited   bool   `json:"unlimited"`
	Active      bool   `json:"active"`
	Sort        int    `json:"sort"`
	RedeemNote  string `json:"redeemNote"`
}

// MallAdminFulfill 后台发放入参。
type MallAdminFulfill struct {
	Remark string `json:"remark"`
}
