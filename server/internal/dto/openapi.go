package dto

import "time"

// ============================================================================
// 开放 API（密钥管理 + /open/v1 只读接口）。
// ============================================================================

// APIKeyItem 密钥列表项（永不含明文）。
type APIKeyItem struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Prefix       string     `json:"prefix"`
	Status       string     `json:"status"`
	StatusLabel  string     `json:"statusLabel"`
	LastUsedAt   *time.Time `json:"lastUsedAt"`
	RequestCount int64      `json:"requestCount"`
	CreatedAt    time.Time  `json:"createdAt"`
	RevokedAt    *time.Time `json:"revokedAt"`
}

// APIKeyCreated 新建密钥响应：明文只此一次。
type APIKeyCreated struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Prefix    string    `json:"prefix"`
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"createdAt"`
}

// APIKeyListResp 密钥列表。
type APIKeyListResp struct {
	Keys     []APIKeyItem `json:"keys"`
	MaxKeys  int          `json:"maxKeys"`
	Endpoint string       `json:"endpoint"`
}

// OpenArticleItem 开放接口的文章条目（比前台列表更瘦，脚本消费）。
type OpenArticleItem struct {
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	Summary     string    `json:"summary"`
	Channel     string    `json:"channel"`
	Tags        []string  `json:"tags"`
	Views       int64     `json:"views"`
	PublishedAt time.Time `json:"publishedAt"`
}

// OpenArticleDetail 文章详情（正文 Markdown 原文）。
type OpenArticleDetail struct {
	OpenArticleItem
	Content string `json:"content"`
}

// OpenQuizItem 开放接口的题目（判断题/选择题统一形状）。
type OpenQuizItem struct {
	ID        int64    `json:"id"`
	Direction string   `json:"direction"`
	Stem      string   `json:"stem"`
	Options   []string `json:"options,omitempty"`
	Answer    string   `json:"answer"`
	Analysis  string   `json:"analysis"`
}

// OpenPage 通用分页壳。
type OpenPage struct {
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}
