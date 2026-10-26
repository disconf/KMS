package dto

import "time"

// ============================ 栏目导航 ============================

// ChannelItem 栏目导航项。
//
// Count 是该栏目"已发布"内容的条数，供导航栏显示角标。
// 各栏目内容落在不同表里，所以这是分别统计后拼出来的，不是单表 count。
type ChannelItem struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Desc  string `json:"desc"`
	Kind  string `json:"kind"`
	Sort  int    `json:"sort"`
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

// ============================ 开源工具推荐 ============================

// ToolListQuery 工具列表查询条件。
type ToolListQuery struct {
	PageQuery

	CategoryID   int64  `form:"categoryId"`
	CategorySlug string `form:"categorySlug"`
	Keyword      string `form:"keyword"`
	// Language / License 精确匹配，用于"只看 Go 写的、Apache 协议的"这类筛选。
	Language string `form:"language"`
	License  string `form:"license"`
	Tag      string `form:"tag"`
	// FeaturedOnly 只看精选。
	FeaturedOnly bool  `form:"featuredOnly"`
	MinStars     int64 `form:"minStars"`
	// Sort 取值 latest（默认）/ hot / stars / sort。
	Sort string `form:"sort" binding:"omitempty,oneof=latest hot stars sort"`
}

// AdminToolListQuery 后台工具列表查询条件，可查全部状态。
type AdminToolListQuery struct {
	PageQuery

	Status     string `form:"status" binding:"omitempty,oneof=draft pending published rejected offline"`
	CategoryID int64  `form:"categoryId"`
	Keyword    string `form:"keyword"`
}

// ToolItem 工具卡片。
type ToolItem struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Slug         string     `json:"slug"`
	Summary      string     `json:"summary"`
	Logo         string     `json:"logo"`
	RepoURL      string     `json:"repoUrl"`
	HomepageURL  string     `json:"homepageUrl"`
	License      string     `json:"license"`
	Language     string     `json:"language"`
	Stars        int64      `json:"stars"`
	ForkCount    int64      `json:"forkCount"`
	DeployModes  []string   `json:"deployModes"`
	Tags         []string   `json:"tags"`
	CategoryID   int64      `json:"categoryId"`
	CategoryName string     `json:"categoryName"`
	CategorySlug string     `json:"categorySlug"`
	Featured     bool       `json:"featured"`
	ViewCount    int64      `json:"viewCount"`
	PublishedAt  *time.Time `json:"publishedAt"`
}

// ToolDetail 工具详情，补上正文与 SEO。
type ToolDetail struct {
	ToolItem

	Content      string     `json:"content"`
	SEO          SEOInfo    `json:"seo"`
	StarsUpdated *time.Time `json:"starsUpdated"`
	Related      []ToolItem `json:"related"`
	// Guides 四类实战教程（部署 / 实施 / 优化 / 集群），前端按 kind 分节展示。
	Guides []ToolGuideItem `json:"guides"`
	// Books 系统教程集（从零基础到深入的章节化路径）。
	//
	// 与 Guides 并存而不是取代：Guides 是「散装好文推荐」，回答"还有哪些值得看"；
	// Books 是「有顺序的学习路径」，回答"我该怎么系统学会它"。
	// 把两者合并会同时丢掉两个语义——推荐位被目录挤掉，目录又被塞进无关文章。
	Books []ToolBookItem `json:"books"`
}

// ToolGuideItem 工具详情页的一条实战教程。
//
// ArticleID 与 URL 二选一：站内文章带上 slug，前端跳 #/article/<slug>；
// 站外文档（官方文档、别人的博客）直接新窗口打开。
// 两种都保留，是因为「部署教程」往往官方文档写得最好，没必要为了闭环抄一遍。
type ToolGuideItem struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	ArticleID   int64  `json:"articleId"`
	ArticleSlug string `json:"articleSlug"`
	Summary     string `json:"summary"`
	Sort        int    `json:"sort"`
}

// ToolGuideReq 后台提交的一条教程关联。
// binding 里校验 kind 枚举，避免前端拼错一个字母就写进一条永远不会被展示的记录。
type ToolGuideReq struct {
	Kind      string `json:"kind" binding:"required,oneof=deploy implement optimize cluster"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	ArticleID int64  `json:"articleId"`
	Sort      int    `json:"sort"`
}

// ToolGuidesReq 整体替换某工具的教程清单。
type ToolGuidesReq struct {
	Guides []ToolGuideReq `json:"guides"`
}

// AdminToolItem 后台列表项，多出状态与排序权重。
type AdminToolItem struct {
	ToolItem

	Status    string    `json:"status"`
	Sort      int       `json:"sort"`
	AuthorID  int64     `json:"authorId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AdminToolDetail 后台编辑态详情。
type AdminToolDetail struct {
	AdminToolItem

	Content string  `json:"content"`
	SEO     SEOInfo `json:"seo"`
}

// ToolCreateReq 新建工具。
type ToolCreateReq struct {
	Name        string `json:"name" binding:"required"`
	Slug        string `json:"slug"`
	Summary     string `json:"summary"`
	Content     string `json:"content"`
	Logo        string `json:"logo"`
	RepoURL     string `json:"repoUrl"`
	HomepageURL string `json:"homepageUrl"`
	License     string `json:"license"`
	Language    string `json:"language"`

	Stars     int64 `json:"stars" binding:"gte=0"`
	ForkCount int64 `json:"forkCount" binding:"gte=0"`

	DeployModes []string `json:"deployModes"`
	Tags        []string `json:"tags"`
	CategoryID  int64    `json:"categoryId"`

	SEOTitle       string `json:"seoTitle"`
	SEODescription string `json:"seoDescription"`
	SEOKeywords    string `json:"seoKeywords"`

	Featured bool   `json:"featured"`
	Sort     int    `json:"sort"`
	Status   string `json:"status" binding:"omitempty,oneof=draft published"`
}

// ToolUpdateReq 更新工具，指针字段实现"未传即不改"。
type ToolUpdateReq struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Summary     *string `json:"summary"`
	Content     *string `json:"content"`
	Logo        *string `json:"logo"`
	RepoURL     *string `json:"repoUrl"`
	HomepageURL *string `json:"homepageUrl"`
	License     *string `json:"license"`
	Language    *string `json:"language"`

	Stars     *int64 `json:"stars"`
	ForkCount *int64 `json:"forkCount"`

	DeployModes *[]string `json:"deployModes"`
	Tags        *[]string `json:"tags"`
	CategoryID  *int64    `json:"categoryId"`

	SEOTitle       *string `json:"seoTitle"`
	SEODescription *string `json:"seoDescription"`
	SEOKeywords    *string `json:"seoKeywords"`

	Featured *bool `json:"featured"`
	Sort     *int  `json:"sort"`
}

// ============================ 资源下载 ============================

// ResourceListQuery 资源列表查询条件。
type ResourceListQuery struct {
	PageQuery

	CategoryID   int64  `form:"categoryId"`
	CategorySlug string `form:"categorySlug"`
	Keyword      string `form:"keyword"`
	Tag          string `form:"tag"`
	FileExt      string `form:"fileExt"`
	// Sort 取值 latest（默认）/ hot / download。
	Sort string `form:"sort" binding:"omitempty,oneof=latest hot download"`
}

// AdminResourceListQuery 后台资源列表查询条件。
type AdminResourceListQuery struct {
	PageQuery

	Status     string `form:"status" binding:"omitempty,oneof=draft pending published rejected offline"`
	CategoryID int64  `form:"categoryId"`
	Keyword    string `form:"keyword"`
}

// ResourceItem 资源卡片。
//
// 刻意不含 FileKey / 下载直链：列表里给出可下载地址等于把签名暴露在
// 一个会被大量缓存与转发的响应里，下载必须走单条详情接口现场签发。
type ResourceItem struct {
	ID             int64    `json:"id"`
	Title          string   `json:"title"`
	Slug           string   `json:"slug"`
	Summary        string   `json:"summary"`
	Cover          string   `json:"cover"`
	FileName       string   `json:"fileName"`
	FileSize       int64    `json:"fileSize"`
	FileExt        string   `json:"fileExt"`
	Checksum       string   `json:"checksum"`
	Version        string   `json:"version"`
	CompatibleWith string   `json:"compatibleWith"`
	CategoryID     int64    `json:"categoryId"`
	CategoryName   string   `json:"categoryName"`
	CategorySlug   string   `json:"categorySlug"`
	Tags           []string `json:"tags"`
	NeedLogin      bool     `json:"needLogin"`
	// HasFile 表示我们托管了文件、可以直接下载。
	// FileKey 本身不下发（直链会失效、也可能被盗链），前端只能通过这个布尔值判断按钮形态。
	HasFile       bool       `json:"hasFile"`
	DownloadCount int64      `json:"downloadCount"`
	ViewCount     int64      `json:"viewCount"`
	PublishedAt   *time.Time `json:"publishedAt"`
}

// ResourceDetail 资源详情，含使用说明与外部下载链接。
type ResourceDetail struct {
	ResourceItem

	Content string `json:"content"`
	// Links 指向官方站 / 镜像 / 包管理器等处的外部地址。
	// 与托管文件并存：一个组件既可能我们自己传了一份，也可能只给官网地址。
	Links   []ResourceLinkItem `json:"links"`
	Related []ResourceItem     `json:"related"`
}

// ResourceLinkItem 一条外部下载链接。
type ResourceLinkItem struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
	URL   string `json:"url"`
	Kind  string `json:"kind"`
	Note  string `json:"note"`
	Sort  int    `json:"sort"`
}

// ResourceLinkReq 后台提交的一条下载链接。
type ResourceLinkReq struct {
	Label string `json:"label"`
	URL   string `json:"url" binding:"required"`
	Kind  string `json:"kind"`
	Note  string `json:"note"`
	Sort  int    `json:"sort"`
}

// ResourceLinksReq 整体替换某资源的链接清单。
type ResourceLinksReq struct {
	Links []ResourceLinkReq `json:"links"`
}

// AdminResourceItem 后台列表项。
type AdminResourceItem struct {
	ResourceItem

	Status    string    `json:"status"`
	AuthorID  int64     `json:"authorId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AdminResourceDetail 后台编辑态详情，回显 FileKey 以便替换文件。
type AdminResourceDetail struct {
	AdminResourceItem

	Content string `json:"content"`
	FileKey string `json:"fileKey"`
}

// ResourceCreateReq 新建资源。
//
// FileKey 必须是上传接口返回的 key，不接受外部 URL：
// 让运营直接填外链等于把平台的下载入口变成别人站点的镜像，
// 链接一失效整个资源页就成了死链，也无法统计真实下载量。
type ResourceCreateReq struct {
	Title   string `json:"title" binding:"required"`
	Slug    string `json:"slug"`
	Summary string `json:"summary"`
	Content string `json:"content"`
	Cover   string `json:"cover"`

	FileKey string `json:"fileKey" binding:"required"`
	// FileName 只用于展示与推导扩展名，属装饰性字段；
	// 大小与校验和一律从落盘内容实测，不采信这里传的值。
	FileName string `json:"fileName"`

	Version        string `json:"version"`
	CompatibleWith string `json:"compatibleWith"`

	CategoryID int64    `json:"categoryId"`
	Tags       []string `json:"tags"`

	NeedLogin bool   `json:"needLogin"`
	Status    string `json:"status" binding:"omitempty,oneof=draft published"`
}

// ResourceUpdateReq 更新资源，指针字段实现"未传即不改"。
type ResourceUpdateReq struct {
	Title   *string `json:"title"`
	Slug    *string `json:"slug"`
	Summary *string `json:"summary"`
	Content *string `json:"content"`
	Cover   *string `json:"cover"`

	FileKey  *string `json:"fileKey"`
	FileName *string `json:"fileName"`

	Version        *string `json:"version"`
	CompatibleWith *string `json:"compatibleWith"`

	CategoryID *int64    `json:"categoryId"`
	Tags       *[]string `json:"tags"`

	NeedLogin *bool `json:"needLogin"`
}

// ResourceDownload 下载凭证。
type ResourceDownload struct {
	URL      string `json:"url"`
	ExpireAt int64  `json:"expireAt,omitempty"`
	FileName string `json:"fileName"`
	FileSize int64  `json:"fileSize"`
	Checksum string `json:"checksum"`
	Version  string `json:"version"`
}

// ============================ 长度与取值约束 ============================

const (
	toolNameMaxLen     = 128
	toolSlugMaxLen     = 128
	toolSummaryMaxLen  = 500
	toolContentMaxLen  = 100000
	toolURLMaxLen      = 512
	toolLicenseMaxLen  = 64
	toolLanguageMaxLen = 64
	maxTagsPerContent  = 10
	contentTagMaxLen   = 32
	// maxDeployModes 限制部署方式的个数，防止有人把标签塞进这个字段。
	maxDeployModes = 8

	resourceTitleMaxLen   = 200
	resourceSlugMaxLen    = 200
	resourceSummaryMaxLen = 500
	resourceContentMaxLen = 2000
	resourceVersionMaxLen = 64
	resourceCompatMaxLen  = 255
)

// ToolLimits 返回工具各字段长度上限。
func ToolLimits() (name, slug, summary, content, url, license, language int) {
	return toolNameMaxLen, toolSlugMaxLen, toolSummaryMaxLen, toolContentMaxLen,
		toolURLMaxLen, toolLicenseMaxLen, toolLanguageMaxLen
}

// ToolDeployModes 返回合法的部署方式取值。
func ToolDeployModes() []string {
	return []string{"binary", "docker", "helm", "k8s-operator", "source", "cloud"}
}

// ContentTagLimits 返回标签个数与单个标签长度上限。
func ContentTagLimits() (count, length int) { return maxTagsPerContent, contentTagMaxLen }

// DeployModeLimit 返回部署方式个数上限。
func DeployModeLimit() int { return maxDeployModes }

// ResourceLimits 返回资源各字段长度上限。
func ResourceLimits() (title, slug, summary, content, version, compat int) {
	return resourceTitleMaxLen, resourceSlugMaxLen, resourceSummaryMaxLen,
		resourceContentMaxLen, resourceVersionMaxLen, resourceCompatMaxLen
}
