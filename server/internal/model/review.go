package model

import "time"

// 内容审核状态。文章、工具、资源、题目共用这一套取值。
//
// 为什么统一：后台的「待审内容」是一条流水线，如果四类内容各用一套状态名，
// 列表查询、计数角标、批量通过都要写四套分支，最后必然有一处漏改。
//
// 各模型原有的 XXXStatusDraft / Published / Offline 常量继续保留（取值与这里一致），
// 这里补的是投稿流程新增的两个状态：等待审核、审核驳回。
const (
	ContentStatusDraft     = "draft"
	ContentStatusPending   = "pending"
	ContentStatusPublished = "published"
	ContentStatusRejected  = "rejected"
	ContentStatusOffline   = "offline"
)

// ReviewInfo 是内容上的审核痕迹，匿名嵌入到各内容模型，展开为三个列。
//
// ReviewNote 记「为什么驳回」，ReviewerID 记「谁点的通过」。
// 没有这两项，投稿人收到驳回只能反复试错，运营也无从追溯是谁放行的。
type ReviewInfo struct {
	ReviewNote string     `gorm:"size:500;comment:审核意见" json:"reviewNote,omitempty"`
	ReviewerID int64      `gorm:"comment:审核人用户 ID" json:"reviewerId,omitempty"`
	ReviewedAt *time.Time `gorm:"comment:审核时间" json:"reviewedAt,omitempty"`
}
