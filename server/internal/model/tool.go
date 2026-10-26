package model

import "time"

// Tool 状态。与文章一致，草稿不对外可见。
const (
	ToolStatusDraft     = "draft"
	ToolStatusPending   = "pending"
	ToolStatusPublished = "published"
	ToolStatusRejected  = "rejected"
	ToolStatusOffline   = "offline"
)

// Tool 开源工具推荐条目。
//
// 为什么不复用 Article：工具的核心信息是结构化的——语言、License、Star 数、
// 部署方式。这些字段要参与筛选（"只看 Go 写的、Apache 协议的"）和排序（按 Star）。
// 塞进文章表只能靠 tags 硬编码，一旦要做筛选就必须全表 LIKE，等于没有索引。
type Tool struct {
	Base

	Name    string `gorm:"size:128;index;comment:工具名称" json:"name"`
	Slug    string `gorm:"size:128;uniqueIndex;comment:URL 标识" json:"slug"`
	Summary string `gorm:"size:500;comment:一句话简介" json:"summary"`
	// Content 是详细介绍，Markdown。
	Content string `gorm:"type:longtext;comment:详细介绍" json:"content"`

	Logo        string `gorm:"size:512;comment:Logo 地址" json:"logo"`
	RepoURL     string `gorm:"size:512;comment:代码仓库地址" json:"repoUrl"`
	HomepageURL string `gorm:"size:512;comment:官网地址" json:"homepageUrl"`

	// License 用 SPDX 标识（Apache-2.0 / MIT / GPL-3.0），便于统一筛选与合规检查。
	License  string `gorm:"size:64;index;comment:开源协议" json:"license"`
	Language string `gorm:"size:64;index;comment:主要实现语言" json:"language"`

	// Stars / ForkCount 是采集时的快照，不是实时值——外部仓库的 star 数
	// 每分钟都在变，实时拉取既慢又受限于第三方限流，按快照展示并标注时间即可。
	Stars        int64      `gorm:"default:0;comment:Star 数快照" json:"stars"`
	ForkCount    int64      `gorm:"default:0;comment:Fork 数快照" json:"forkCount"`
	StarsUpdated *time.Time `gorm:"comment:Star 数采集时间" json:"starsUpdated"`

	// DeployModes 部署方式：binary / docker / helm / k8s-operator / source。
	DeployModes StringList `gorm:"comment:支持的部署方式" json:"deployModes"`
	Tags        StringList `gorm:"comment:标签" json:"tags"`

	CategoryID int64 `gorm:"index;comment:所属分类 ID" json:"categoryId"`

	SEOTitle       string `gorm:"size:200;comment:SEO 标题" json:"seoTitle"`
	SEODescription string `gorm:"size:500;comment:SEO 描述" json:"seoDescription"`
	SEOKeywords    string `gorm:"size:500;comment:SEO 关键词" json:"seoKeywords"`

	// 审核痕迹：用户投稿的工具先进入待审，通过后才对外可见。
	ReviewInfo

	Status string `gorm:"size:16;index;default:draft;comment:状态" json:"status"`
	// Featured 置顶推荐，用于栏目首页的头条位。
	Featured bool `gorm:"default:false;comment:是否精选" json:"featured"`
	Sort     int  `gorm:"default:0;comment:排序权重，越大越靠前" json:"sort"`

	AuthorID    int64      `gorm:"index;comment:录入人用户 ID" json:"authorId"`
	PublishedAt *time.Time `gorm:"comment:发布时间" json:"publishedAt"`

	ViewCount int64 `gorm:"default:0;comment:浏览数" json:"viewCount"`
}

// TableName 固定表名。
func (Tool) TableName() string { return "tools" }

// IsPublished 判断工具是否已发布。
func (t *Tool) IsPublished() bool { return t.Status == ToolStatusPublished }

// 教程类型。这四类是运维落地一个开源组件的完整路径：
// 先装起来（deploy）→ 接进业务（implement）→ 调好（optimize）→ 上规模（cluster）。
// 顺序不是随便定的，前端也按这个顺序展示。
const (
	ToolGuideDeploy    = "deploy"
	ToolGuideImplement = "implement"
	ToolGuideOptimize  = "optimize"
	ToolGuideCluster   = "cluster"
)

// ToolGuideKinds 返回四类教程的稳定顺序。
func ToolGuideKinds() []string {
	return []string{ToolGuideDeploy, ToolGuideImplement, ToolGuideOptimize, ToolGuideCluster}
}

// IsValidToolGuideKind 判断类型是否合法。
func IsValidToolGuideKind(kind string) bool {
	for _, k := range ToolGuideKinds() {
		if k == kind {
			return true
		}
	}
	return false
}

// ToolGuide 是工具与实战教程的关联条目。
//
// 为什么不直接在 tools 表上开四个长文本字段：
// 一个工具在每一类下往往不止一篇（部署分单机与 K8s 两种，优化分参数与容量两篇），
// 而且这些内容本身就是 articles 里的文章 —— 用关联表才能「工具页点进去看教程、
// 教程底部又能回到工具」，形成互链；塞成四个字段则只能各写一段孤立的文字。
//
// 刻意不嵌入 Base（不要软删除）：这是关系表，维护时整体替换，
// 软删除会让被替换掉的条目一直占着行，查询还得处处带 deleted_at 条件。
type ToolGuide struct {
	ID     int64 `gorm:"primaryKey;autoIncrement" json:"id"`
	ToolID int64 `gorm:"index;comment:工具 ID" json:"toolId"`
	// Kind 取值见 ToolGuideDeploy 等常量。
	Kind string `gorm:"size:16;index;comment:教程类型 deploy|implement|optimize|cluster" json:"kind"`
	// ArticleID 非 0 表示指向站内实战教程（articles.id）。
	ArticleID int64 `gorm:"index;comment:关联站内文章 ID" json:"articleId"`
	// Title 在指向外部链接时必填；指向站内文章时留空则取文章标题。
	Title string `gorm:"size:200;comment:标题" json:"title"`
	// URL 指向站外文档（官方文档、博客）。与 ArticleID 二选一。
	URL  string `gorm:"size:512;comment:外部链接" json:"url"`
	Sort int    `gorm:"default:0;comment:同类型内排序" json:"sort"`
}

// TableName 固定表名。
func (ToolGuide) TableName() string { return "tool_guides" }
