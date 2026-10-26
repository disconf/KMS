package dto

import "time"

// ============================ 查询入参 ============================

// ArticleListQuery 前台文章列表查询条件。
type ArticleListQuery struct {
	PageQuery

	// Channel 限定栏目（linux-tutorial / ops-news）。栏目页必须带上，
	// 否则教程页会把资讯也一起列出来。
	Channel string `form:"channel"`
	// CategoryID 与 CategorySlug 二选一，同时传以 CategoryID 为准。
	CategoryID   int64  `form:"categoryId"`
	CategorySlug string `form:"categorySlug"`
	// Keyword 对标题 / 摘要 / 正文 / 标签模糊匹配。
	Keyword string `form:"keyword"`
	// Tag 精确匹配单个标签。
	Tag string `form:"tag"`
	// Sort 取值 latest（默认）/ hot（按阅读数）/ like（按点赞数）。
	Sort string `form:"sort" binding:"omitempty,oneof=latest hot like"`
}

// AdminArticleListQuery 后台文章列表查询条件，可查全部状态。
type AdminArticleListQuery struct {
	PageQuery

	// 列表要能按 pending / rejected 过滤：审核队列就是靠这个把待审内容捞出来。
	// 写入路径（Create/Update）仍然只允许 draft / published，两者刻意不同。
	Status     string `form:"status" binding:"omitempty,oneof=draft pending published rejected offline"`
	Channel    string `form:"channel"`
	CategoryID int64  `form:"categoryId"`
	Keyword    string `form:"keyword"`
}

// ============================ 出参 ============================

// CategoryItem 知识库分类。前台只返回启用中的分类，后台返回全部。
type CategoryItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Icon string `json:"icon"`
	Sort int    `json:"sort"`
	// ArticleCount 文章数：前台口径只统计已发布，后台口径统计全部。
	ArticleCount int64 `json:"articleCount"`
	Enabled      bool  `json:"enabled"`
}

// TagItem 标签及其文章数。
type TagItem struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// ArticleListItem 列表卡片字段。
// 刻意不包含 Content：列表接口带上正文会让响应体膨胀几十倍，且正文前台用不上。
type ArticleListItem struct {
	ID           int64      `json:"id"`
	Title        string     `json:"title"`
	Slug         string     `json:"slug"`
	Summary      string     `json:"summary"`
	Cover        string     `json:"cover"`
	Channel      string     `json:"channel"`
	CategoryID   int64      `json:"categoryId"`
	CategoryName string     `json:"categoryName"`
	CategorySlug string     `json:"categorySlug"`
	Tags         []string   `json:"tags"`
	AuthorName   string     `json:"authorName"`
	ViewCount    int64      `json:"viewCount"`
	LikeCount    int64      `json:"likeCount"`
	PublishedAt  *time.Time `json:"publishedAt"`
	// SourceName / SourceURL 只对订阅源抓来的资讯有意义：
	// 列表要标明来源，详情要能点回原文。人工撰写的文章两者为空。
	SourceName string `json:"sourceName,omitempty"`
	SourceURL  string `json:"sourceUrl,omitempty"`
}

// SEOInfo 详情页 SEO 结构化字段，前端据此输出 TechArticle 结构化数据。
type SEOInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Keywords    string `json:"keywords"`
}

// ArticleDetail 文章详情。
type ArticleDetail struct {
	ArticleListItem

	Content string  `json:"content"`
	SEO     SEOInfo `json:"seo"`
	// Source 来源：manual 人工撰写 / order 订单案例脱敏沉淀。
	Source string `json:"source"`
	// Liked 当前登录用户是否已点赞；未登录恒为 false。
	Liked   bool              `json:"liked"`
	Related []ArticleListItem `json:"related"`
}

// LikeResult 点赞切换结果。
type LikeResult struct {
	Liked     bool  `json:"liked"`
	LikeCount int64 `json:"likeCount"`
}

// AdminArticleItem 后台列表项，比前台多出状态与作者。
type AdminArticleItem struct {
	ArticleListItem

	Status   string `json:"status"`
	AuthorID int64  `json:"authorId"`
	// 审核痕迹。待审队列里能看到上一位审核人留下的意见，
	// 避免两个管理员对同一篇内容做出相反的判断。
	ReviewNote string     `json:"reviewNote,omitempty"`
	ReviewerID int64      `json:"reviewerId,omitempty"`
	ReviewedAt *time.Time `json:"reviewedAt,omitempty"`
}

// AdminArticleDetail 后台编辑态详情，含正文与全部 SEO 字段。
type AdminArticleDetail struct {
	AdminArticleItem

	Content string  `json:"content"`
	SEO     SEOInfo `json:"seo"`
	Source  string  `json:"source"`
}

// ============================ 后台入参 ============================

// ArticleCreateReq 新建文章。status 省略时为草稿，避免误发。
type ArticleCreateReq struct {
	Title      string   `json:"title" binding:"required"`
	Slug       string   `json:"slug"`
	Summary    string   `json:"summary"`
	Cover      string   `json:"cover"`
	Content    string   `json:"content"`
	CategoryID int64    `json:"categoryId"`
	Tags       []string `json:"tags"`
	// Channel 省略时归入 Linux 实战教程：绝大多数文章都是教程类，
	// 强制前端每次都传一个默认值只会让调用方更容易漏。
	Channel string `json:"channel"`

	SEOTitle       string `json:"seoTitle"`
	SEODescription string `json:"seoDescription"`
	SEOKeywords    string `json:"seoKeywords"`

	Status string `json:"status" binding:"omitempty,oneof=draft published"`
}

// ArticleUpdateReq 更新文章，指针字段实现"未传即不改"。
type ArticleUpdateReq struct {
	Title      *string   `json:"title"`
	Slug       *string   `json:"slug"`
	Summary    *string   `json:"summary"`
	Cover      *string   `json:"cover"`
	Content    *string   `json:"content"`
	CategoryID *int64    `json:"categoryId"`
	Tags       *[]string `json:"tags"`
	Channel    *string   `json:"channel"`

	SEOTitle       *string `json:"seoTitle"`
	SEODescription *string `json:"seoDescription"`
	SEOKeywords    *string `json:"seoKeywords"`
}

// CategoryCreateReq 新建分类。
type CategoryCreateReq struct {
	Name string `json:"name" binding:"required"`
	Slug string `json:"slug"`
	Icon string `json:"icon"`
	Sort int    `json:"sort"`
	// Enabled 省略时默认启用。
	Enabled *bool `json:"enabled"`
}

// CategoryUpdateReq 更新分类，指针字段实现"未传即不改"。
type CategoryUpdateReq struct {
	Name    *string `json:"name"`
	Slug    *string `json:"slug"`
	Icon    *string `json:"icon"`
	Sort    *int    `json:"sort"`
	Enabled *bool   `json:"enabled"`
}

// ============================ 长度约束 ============================
//
// 集中在此处，service 层校验与 OpenAPI 文档共用一套数字，避免两边写岔。

const (
	articleTitleMinLen   = 2
	articleTitleMaxLen   = 200
	articleSlugMaxLen    = 200
	articleSummaryMaxLen = 500
	articleCoverMaxLen   = 512
	// articleContentMaxLen 正文长度上限。
	// longtext 本身能存 4GB，但单请求带上百兆正文会直接打爆内存与网关，
	// 因此这里给一个远小于数据库上限、又足够写长文的业务约束。
	articleContentMaxLen     = 100000
	articleSEOTitleMaxLen    = 200
	articleSEODescMaxLen     = 500
	articleSEOKeywordsMaxLen = 500

	maxTagsPerArticle = 10
	tagMaxLen         = 32

	categoryNameMaxLen = 64
	categorySlugMaxLen = 64
	categoryIconMaxLen = 255
)

// ArticleTitleLen 返回标题长度约束（按字符数计）。
func ArticleTitleLen() (int, int) { return articleTitleMinLen, articleTitleMaxLen }

// ArticleLimits 返回文章各字段长度上限。
func ArticleLimits() (summary, cover, content, seoTitle, seoDesc, seoKeywords int) {
	return articleSummaryMaxLen, articleCoverMaxLen, articleContentMaxLen,
		articleSEOTitleMaxLen, articleSEODescMaxLen, articleSEOKeywordsMaxLen
}

// TagLimits 返回标签个数与单个标签长度上限。
func TagLimits() (count, length int) { return maxTagsPerArticle, tagMaxLen }

// CategoryLimits 返回分类字段长度上限。
func CategoryLimits() (name, slug, icon int) {
	return categoryNameMaxLen, categorySlugMaxLen, categoryIconMaxLen
}

// CategorySlugMaxLen 返回分类 slug 长度上限。
func CategorySlugMaxLen() int { return categorySlugMaxLen }

// ArticleSlugMaxLen 返回文章 slug 长度上限。
func ArticleSlugMaxLen() int { return articleSlugMaxLen }

// ============================ 投稿与审核 ============================

// ArticleContributeReq 用户投稿文章。
//
// 与后台创建的字段差别是刻意的：不给 status、source、authorId。
// 状态一律 pending（否则用户能自己发布），作者一律取登录态（否则能冒名），
// 来源一律 manual（否则可以伪造成从订阅源抓来的官方内容）。
type ArticleContributeReq struct {
	Title      string   `json:"title" binding:"required"`
	Summary    string   `json:"summary"`
	Content    string   `json:"content" binding:"required"`
	Channel    string   `json:"channel"`
	CategoryID int64    `json:"categoryId"`
	Tags       []string `json:"tags"`
	Cover      string   `json:"cover"`
}

// ArticleReviewReq 审核动作。驳回必须给原因。
type ArticleReviewReq struct {
	Action string `json:"action" binding:"required,oneof=approve reject"`
	Note   string `json:"note"`
}

// MyContributionItem 我的投稿。
//
// 必须带 status 与 reviewNote：被驳回之后作者要知道改什么，
// 只显示「已驳回」等于让人对着一个不存在的错误反复试。
//
// 文章与题目共用这个结构（Kind 区分）：投稿人眼里这两件事都是「我交上去的内容」，
// 只是题目没有 slug、没有阅读数。分成两个结构会让前端「我的投稿」写两套卡片模板，
// 而字段差异只有三个空值。
type MyContributionItem struct {
	// Kind 取值 article / quiz。旧客户端不认这个字段时按 article 处理即可。
	Kind        string     `json:"kind"`
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Slug        string     `json:"slug"`
	Channel     string     `json:"channel"`
	Status      string     `json:"status"`
	ReviewNote  string     `json:"reviewNote,omitempty"`
	ReviewedAt  *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
	ViewCount   int64      `json:"viewCount"`
}
