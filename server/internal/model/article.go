package model

import "time"

// 文章状态。
const (
	ArticleStatusDraft     = "draft"     // 草稿
	ArticleStatusPending   = "pending"   // 待审核（用户投稿 / 抓取待采用）
	ArticleStatusPublished = "published" // 已发布
	ArticleStatusRejected  = "rejected"  // 审核驳回
	ArticleStatusOffline   = "offline"   // 已下线
)

// 文章来源。
const (
	ArticleSourceManual = "manual" // 人工撰写
	ArticleSourceOrder  = "order"  // 订单案例脱敏沉淀
	ArticleSourceFeed   = "feed"   // 订阅源自动抓取
)

// ArticleCategory 知识库分类。MVP 只开 1~2 个方向（K8s / 数据库 / 网络）。
type ArticleCategory struct {
	Base

	Name         string `gorm:"size:64;uniqueIndex;comment:分类名" json:"name"`
	Slug         string `gorm:"size:64;uniqueIndex;comment:URL 标识" json:"slug"`
	Icon         string `gorm:"size:255;comment:分类图标" json:"icon"`
	Sort         int    `gorm:"default:0;comment:排序权重" json:"sort"`
	Enabled      bool   `gorm:"default:true;comment:是否启用" json:"enabled"`
	ArticleCount int64  `gorm:"-" json:"articleCount"` // 运行时统计，不落库
}

// TableName 固定表名。
func (ArticleCategory) TableName() string { return "article_categories" }

// Article 知识库文章。SSR + SEO 是平台最低成本的获客入口。
type Article struct {
	Base

	Title   string `gorm:"size:200;index;comment:标题" json:"title"`
	Slug    string `gorm:"size:200;uniqueIndex;comment:SEO 友好 URL 标识" json:"slug"`
	Summary string `gorm:"size:500;comment:摘要" json:"summary"`
	Cover   string `gorm:"size:512;comment:封面图地址" json:"cover"`
	Content string `gorm:"type:longtext;comment:Markdown 正文" json:"content"`

	CategoryID int64      `gorm:"index;comment:分类 ID" json:"categoryId"`
	Tags       StringList `gorm:"comment:标签：技术栈/错误码/故障现象" json:"tags"`

	// Channel 标记这篇文章归属哪个栏目（linux-tutorial / ops-news）。
	//
	// 教程与资讯都是图文长文，差别只在时效性与列表密度，共用一张表即可；
	// 但两者必须能分别查询，否则前端一个栏目页要把另一栏的内容也捞出来。
	// 取值由 model.ArticleChannels() 约束，不引入外键——栏目是代码资产。
	Channel string `gorm:"size:64;index;default:linux-tutorial;comment:所属栏目" json:"channel"`

	// SEO 结构化字段，前端输出 TechArticle / FAQPage
	SEOTitle       string `gorm:"size:200;comment:SEO 标题" json:"seoTitle"`
	SEODescription string `gorm:"size:500;comment:SEO 描述" json:"seoDescription"`
	SEOKeywords    string `gorm:"size:500;comment:SEO 关键词" json:"seoKeywords"`

	// 来源：manual 人工撰写 / order 订单案例脱敏沉淀 / feed 订阅源抓取
	Source  string `gorm:"size:16;default:manual;comment:来源" json:"source"`
	OrderID int64  `gorm:"comment:沉淀来源订单 ID" json:"orderId"`

	// 以下三项服务于「运维资讯自动抓取」。
	//
	// ExternalID 是订阅源内的唯一标识（RSS 的 guid、GitHub release 的 id），
	// 加唯一索引做去重：抓取任务每 30 分钟跑一次，同一篇公告会被反复看到，
	// 幂等只能靠数据库约束，应用层先查一次在并发下会漏。
	//
	// 用 *string 而不是 string：唯一索引下 MySQL/SQLite 都允许多个 NULL，
	// 但不允许多个空串。人工撰写的文章没有 ExternalID，必须存 NULL。
	ExternalID *string `gorm:"size:255;uniqueIndex;comment:订阅源内唯一标识" json:"externalId,omitempty"`
	SourceName string  `gorm:"size:120;comment:来源名称，如 Kubernetes Blog" json:"sourceName,omitempty"`
	SourceURL  string  `gorm:"size:512;comment:原文链接" json:"sourceUrl,omitempty"`

	// 审核痕迹：投稿与抓取都要经过人工过一遍才上线。
	ReviewInfo

	Status      string     `gorm:"size:16;index;default:draft;comment:状态" json:"status"`
	AuthorID    int64      `gorm:"index;comment:作者用户 ID" json:"authorId"`
	PublishedAt *time.Time `gorm:"comment:发布时间" json:"publishedAt"`

	ViewCount int64 `gorm:"default:0;comment:阅读数" json:"viewCount"`
	LikeCount int64 `gorm:"default:0;comment:点赞数" json:"likeCount"`
}

// TableName 固定表名。
func (Article) TableName() string { return "articles" }

// IsPublished 判断文章是否已发布。
func (a *Article) IsPublished() bool { return a.Status == ArticleStatusPublished }

// articleStatusLabels 是状态到中文的映射。
//
// 单独一张表而不是在 API 层拼字符串：驳回给用户看的话术里要带当前状态，
// 若那里写成 `"当前状态：" + a.Status`，用户会看到 "pending" 这种内部枚举。
var articleStatusLabels = map[string]string{
	ArticleStatusDraft:     "草稿",
	ArticleStatusPending:   "待审核",
	ArticleStatusPublished: "已发布",
	ArticleStatusRejected:  "已驳回",
	ArticleStatusOffline:   "已下线",
}

// StatusLabel 返回状态的中文名。未知状态原样返回，便于发现数据异常。
func (a *Article) StatusLabel() string {
	if label, ok := articleStatusLabels[a.Status]; ok {
		return label
	}
	return a.Status
}

// ArticleLike 记录"哪个用户点赞了哪篇文章"。
//
// 之所以要独立成表而不是只在 Article 上累加计数：计数无法回答
// "这个人是否已点过赞"，会导致刷新页面就能重复点赞刷数据。
// (user_id, article_id) 唯一索引让重复点赞在数据库层直接失败，无需应用层加锁。
//
// 刻意不嵌入 Base：点赞是一张纯关系表，取消点赞必须是**物理删除**。
// 若带上 DeletedAt 软删除，被删的行仍占着唯一索引，再次点赞时
// `ON CONFLICT DO NOTHING` 会静默失败——接口返回"已点赞"、库里却没有记录。
type ArticleLike struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`

	UserID    int64 `gorm:"uniqueIndex:uk_article_like;index;comment:点赞用户 ID" json:"userId"`
	ArticleID int64 `gorm:"uniqueIndex:uk_article_like;index;comment:被点赞文章 ID" json:"articleId"`
}

// TableName 固定表名。
func (ArticleLike) TableName() string { return "article_likes" }
