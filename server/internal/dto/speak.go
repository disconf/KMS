package dto

import "time"

// 发言申请的审核状态取值，与 model 里的常量一一对应。
// dto 不直接引 model，避免线上响应结构被模型字段改名带崩。

// SpeakApplyReq 提交实名发言申请。
//
// 只收姓名与联系方式，不收身份证号：目的只是"确认是个真人"，
// 而身份证属于强 PII，落库就要跟着做加密、脱敏与删除请求。
type SpeakApplyReq struct {
	RealName string `json:"realName" binding:"required,max=32" example:"张伟"`
	Contact  string `json:"contact" binding:"required,max=64" example:"13800000000 或 me@example.com"`
	Note     string `json:"note" binding:"max=255" example:"想参与运维问答，平时做 K8s 运维"`
}

// SpeakApplyState 当前账号的发言权限状态。
type SpeakApplyState struct {
	Status     string     `json:"status"` // none/pending/approved/rejected
	CanSpeak   bool       `json:"canSpeak"`
	AppliedAt  *time.Time `json:"appliedAt,omitempty"`
	ReviewedAt *time.Time `json:"reviewedAt,omitempty"`
	ReviewNote string     `json:"reviewNote,omitempty"`
}

// AdminSpeakApplyItem 后台申请列表的一行。
type AdminSpeakApplyItem struct {
	UserID    int64      `json:"userId"`
	Nickname  string     `json:"nickname"`
	Email     string     `json:"email"`
	Phone     string     `json:"phone"`
	Role      string     `json:"role"`
	RealName  string     `json:"realName"`
	Contact   string     `json:"contact"`
	Note      string     `json:"note"`
	Status    string     `json:"status"`
	AppliedAt *time.Time `json:"appliedAt"`
	CreatedAt time.Time  `json:"createdAt"`
}

// AdminSpeakReviewReq 审核发言申请。
type AdminSpeakReviewReq struct {
	Approve bool   `json:"approve"`
	Note    string `json:"note" binding:"max=255"`
}
