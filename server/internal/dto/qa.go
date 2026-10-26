package dto

import "time"

// ============================ 问答广场（公开） ============================

// QaAskReq 发布问题。
//
// 没有 status、没有 userId：与投稿同理，让客户端传这两个值等于把
// 「冒充别人提问」「自己发布违规内容」两个后门一起打开。
type QaAskReq struct {
	Title       string   `json:"title" binding:"required"`
	Description string   `json:"description" binding:"required"`
	Attachments []string `json:"attachments"`
	SkillIDs    []int64  `json:"skillIds"`
}

// QaAnswerReq 发布 / 编辑回答。
type QaAnswerReq struct {
	Content string `json:"content" binding:"required"`
}

// QaQuestionItem 列表卡片。
type QaQuestionItem struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	// Excerpt 是正文截断，列表里不返回全文——一页 20 条正文全带会让响应翻几倍。
	Excerpt    string `json:"excerpt"`
	UserID     int64  `json:"userId"`
	Nickname   string `json:"nickname"`
	Avatar     string `json:"avatar"`
	AuthorRole string `json:"authorRole"`

	AnswerCount int64  `json:"answerCount"`
	ViewCount   int64  `json:"viewCount"`
	Status      string `json:"status"`

	SkillIDs []int64 `json:"skillIds"`

	CreatedAt    time.Time  `json:"createdAt"`
	LastAnswerAt *time.Time `json:"lastAnswerAt"`
	AcceptedAt   *time.Time `json:"acceptedAt"`
	// HasAccepted 是给列表画「已解决」徽标的显式布尔，
	// 前端不必自己拿 status 去猜（关帖也是非 open，但未解决）。
	HasAccepted bool `json:"hasAccepted"`
}

// QaAnswerItem 回答。
type QaAnswerItem struct {
	ID         int64  `json:"id"`
	QuestionID int64  `json:"questionId"`
	UserID     int64  `json:"userId"`
	Nickname   string `json:"nickname"`
	Avatar     string `json:"avatar"`
	// ContributionPoints / Badge 是「贡献等级」展示，纯荣誉不涉钱。
	ContributionPoints int64  `json:"contributionPoints"`
	Badge              string `json:"badge"`
	AuthorRole         string `json:"authorRole"`
	// IsExpert 是审核通过专家的显式标记：回答列表里专家的答案可信度更高，
	// 值得一个角标让人一眼看到（不是权限，只是展示）。
	IsExpert bool `json:"isExpert"`

	Content    string `json:"content"`
	Score      int64  `json:"score"`
	IsAccepted bool   `json:"isAccepted"`
	// Liked 是「当前访问者是否点过赞」，未登录恒为 false。
	Liked bool `json:"liked"`
	// Status 非 published 时只有作者与管理员看得到（前端据此显示「审核中」）。
	Status string `json:"status"`
	// ReviewNote 只在被驳回且查看者是作者时下发。
	ReviewNote string `json:"reviewNote,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// QaQuestionDetail 详情：问题 + 回答 + 当前访问者的可操作项。
type QaQuestionDetail struct {
	QaQuestionItem
	Description string   `json:"description"`
	Attachments []string `json:"attachments"`

	Answers    []QaAnswerItem `json:"answers"`
	AcceptedID int64          `json:"acceptedAnswerId"`
	// CanAccept / CanClose 由服务端判定，前端只用来决定按钮显隐。
	// 权限判断必须落服务端：前端算出来的只能是"看起来能不能点"。
	CanAccept bool `json:"canAccept"`
	CanClose  bool `json:"canClose"`
	// MyAnswerID 非 0 表示我已经答过，页面把输入框换成「编辑我的回答」。
	MyAnswerID int64 `json:"myAnswerId"`
}

// QaListPage 列表响应。
type QaListPage struct {
	List     []QaQuestionItem `json:"list"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Stats    *QaStatsDTO      `json:"stats,omitempty"`
}

// QaStatsDTO 顶部三档计数（待解答 / 已解决 / 已发布回答数）。
//
// 三个数都是**全局**统计，与登录无关；只在列表第一页下发，
// 翻页请求不再计算——前端本来就只需要在列表顶部显示一次。
type QaStatsDTO struct {
	Open     int64 `json:"open"`
	Resolved int64 `json:"resolved"`
	Answers  int64 `json:"answers"`
}

// ============================ 问答广场（后台） ============================

// QaReviewReq 审核回答。action 只接受 approve / reject。
type QaReviewReq struct {
	Action string `json:"action" binding:"required"`
	Note   string `json:"note"`
}

// QaAdminAnswerItem 后台待审项：比对问题时需要标题，单列出来。
type QaAdminAnswerItem struct {
	QaAnswerItem
	QuestionTitle string `json:"questionTitle"`
}

// QaAdminList 后台列表。
type QaAdminList struct {
	List     []QaAdminAnswerItem `json:"list"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
}

// QaToArticleReq 把「问题 + 采纳回答」转成教程草稿。
//
// 只生成 draft，不给「直接发布」：问答转教程必须人工重新组织语言，
// 直接把问答原文发出去，读者看到的是一问一答的碎片而不是一篇教程。
type QaToArticleReq struct {
	Channel    string `json:"channel"`
	CategoryID int64  `json:"categoryId"`
	Title      string `json:"title"`
}

// QaToArticleResp 转换结果。
type QaToArticleResp struct {
	ArticleID int64  `json:"articleId"`
	Slug      string `json:"slug"`
	// EditPath 直接给前端跳后台编辑器的路，省一次拼接。
	EditPath string `json:"editPath"`
}

// ============================ 积分 ============================

// PointLogItem 积分流水项。
type PointLogItem struct {
	Action     string    `json:"action"`
	ActionText string    `json:"actionText"`
	Points     int64     `json:"points"`
	TargetType string    `json:"targetType"`
	TargetID   int64     `json:"targetId"`
	CreatedAt  time.Time `json:"createdAt"`
}

// MyPointsResp 我的贡献。
type MyPointsResp struct {
	Points int64  `json:"points"`
	Badge  string `json:"badge"`
	// NextBadge / NextAt 给出「离下一级还差多少」，让积分有目标感；
	// 已经是最高级时两者为空。
	NextBadge string         `json:"nextBadge,omitempty"`
	NextAt    int64          `json:"nextAt,omitempty"`
	Recent    []PointLogItem `json:"recent"`
}
