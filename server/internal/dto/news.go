package dto

import "time"

// ============================ 运维资讯订阅源 ============================

// NewsSourceItem 后台订阅源列表项。
//
// 把「最近抓取结果」一起返回，是因为后台真正要回答的问题是
// 「哪个源坏了」，而不是「我配了哪些源」。
type NewsSourceItem struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	Kind          string     `json:"kind"`
	Enabled       bool       `json:"enabled"`
	AutoPublish   bool       `json:"autoPublish"`
	LastFetchedAt *time.Time `json:"lastFetchedAt"`
	LastStatus    string     `json:"lastStatus"`
	LastError     string     `json:"lastError"`
	FetchedCount  int64      `json:"fetchedCount"`
	// PendingTotal 全部源累计的待采用条数（不是本源的），供页面顶部显示。
	PendingTotal int64 `json:"pendingTotal"`
}

// NewsSourceReq 新建订阅源。enabled 默认为 true，
// 因此用指针接收，让「没传」与「显式传 false」区分开。
type NewsSourceReq struct {
	Name        string   `json:"name" binding:"required"`
	URL         string   `json:"url" binding:"required"`
	Kind        string   `json:"kind" binding:"omitempty,oneof=rss github"`
	CategoryID  int64    `json:"categoryId"`
	Tags        []string `json:"tags"`
	Enabled     *bool    `json:"enabled"`
	AutoPublish bool     `json:"autoPublish"`
	Sort        int      `json:"sort"`
}

// NewsSourceUpdateReq 更新订阅源，指针字段实现「未传即不改」。
type NewsSourceUpdateReq struct {
	Name        *string   `json:"name"`
	URL         *string   `json:"url"`
	Kind        *string   `json:"kind" binding:"omitempty,oneof=rss github"`
	CategoryID  *int64    `json:"categoryId"`
	Tags        *[]string `json:"tags"`
	Enabled     *bool     `json:"enabled"`
	AutoPublish *bool     `json:"autoPublish"`
	Sort        *int      `json:"sort"`
}

// FetchResultItem 手动触发抓取的结果。
type FetchResultItem struct {
	SourceID int64  `json:"sourceId"`
	Name     string `json:"name"`
	Written  int    `json:"written"`
	Error    string `json:"error,omitempty"`
}
