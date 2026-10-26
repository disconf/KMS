package model

import "time"

// AuditLog 操作审计日志。远程协助、订单强制流转、后台审核等敏感操作必须落此表。
type AuditLog struct {
	ID int64 `gorm:"primaryKey;autoIncrement" json:"id"`

	UserID     int64  `gorm:"index;comment:操作人用户 ID" json:"userId"`
	UserRole   string `gorm:"size:16;comment:操作人角色快照" json:"userRole"`
	Action     string `gorm:"size:64;index;comment:动作标识，如 expert.audit" json:"action"`
	TargetType string `gorm:"size:32;index;comment:对象类型" json:"targetType"`
	TargetID   int64  `gorm:"index;comment:对象 ID" json:"targetId"`

	Detail    JSONMap `gorm:"comment:变更明细" json:"detail"`
	IP        string  `gorm:"size:64;comment:来源 IP" json:"ip"`
	UserAgent string  `gorm:"size:255;comment:User-Agent" json:"userAgent"`
	RequestID string  `gorm:"size:64;comment:请求链路 ID" json:"requestId"`

	CreatedAt time.Time `gorm:"index" json:"createdAt"`
}

// TableName 固定表名。
func (AuditLog) TableName() string { return "audit_logs" }
