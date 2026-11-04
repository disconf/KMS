package model

import "time"

// ApiKey 开放 API 密钥。
//
// 让用户的脚本能调平台的只读接口（查文档、搜题库）。
// 安全模型与口令一致：库里只存哈希，明文只在创建那一刻展示一次；
// Prefix 明文存储用于列表识别与审计（泄露时可按前缀在日志里定位）。
type ApiKey struct {
	ID     int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID int64 `gorm:"index;not null;comment:属主" json:"userId"`

	Name string `gorm:"size:60;not null;comment:密钥名称（如：公司巡检脚本）" json:"name"`
	// Prefix 明文前缀，形如 opsk_a1b2c3d4，展示用，不能用于鉴权。
	Prefix string `gorm:"size:24;not null;index;comment:密钥前缀" json:"prefix"`
	// KeyHash sha256(明文密钥) 的 hex。鉴权时对请求里的明文重算哈希比对。
	KeyHash string `gorm:"size:64;not null;uniqueIndex;comment:密钥哈希" json:"-"`

	Status string `gorm:"size:16;not null;default:active;index;comment:active|revoked" json:"status"`

	// 使用统计：帮助用户判断哪些密钥还在被脚本用着，可以安心清理。
	LastUsedAt   *time.Time `gorm:"comment:最近调用时间" json:"lastUsedAt"`
	RequestCount int64      `gorm:"default:0;comment:累计调用次数" json:"requestCount"`

	CreatedAt time.Time  `gorm:"index" json:"createdAt"`
	RevokedAt *time.Time `gorm:"comment:吊销时间" json:"revokedAt"`
}

// TableName 固定表名。
func (ApiKey) TableName() string { return "api_keys" }

// 密钥状态。
const (
	ApiKeyStatusActive  = "active"
	ApiKeyStatusRevoked = "revoked"
)
