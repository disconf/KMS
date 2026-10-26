package model

import "time"

// 资源状态。
const (
	ResourceStatusDraft     = "draft"
	ResourceStatusPending   = "pending"
	ResourceStatusPublished = "published"
	ResourceStatusRejected  = "rejected"
	ResourceStatusOffline   = "offline"
)

// Resource 可下载资源：脚本、配置模板、排查手册等。
//
// 与文章最大的差别是「主体是文件而不是正文」：用户来这里的目的是拿走文件，
// 所以文件名、大小、版本、校验和才是核心字段，正文只是补充说明。
// 另一个差别是下载必须计数——下载量是这类栏目唯一有意义的排序依据。
type Resource struct {
	Base

	Title   string `gorm:"size:200;index;comment:资源标题" json:"title"`
	Slug    string `gorm:"size:200;uniqueIndex;comment:URL 标识" json:"slug"`
	Summary string `gorm:"size:500;comment:简介" json:"summary"`
	// Content 是使用说明，Markdown。
	Content string `gorm:"size:2000;comment:使用说明" json:"content"`
	Cover   string `gorm:"size:512;comment:封面图地址" json:"cover"`

	// FileKey 是上传服务返回的文件标识，而不是可直接访问的 URL。
	//
	// 只存 key：直链会随节点迁移、签名过期而失效，也存在被外部站点盗链的风险。
	// 真正的下载地址在请求时按 key 现场签发。
	FileKey string `gorm:"size:512;comment:存储文件标识" json:"-"`
	// 以下三项在上传时从文件本身读出并落库，避免每次下载都去读一次文件头。
	FileName string `gorm:"size:255;comment:原始文件名" json:"fileName"`
	FileSize int64  `gorm:"default:0;comment:文件大小（字节）" json:"fileSize"`
	FileExt  string `gorm:"size:32;comment:文件扩展名" json:"fileExt"`
	// Checksum 是文件内容的 SHA256，用于完整性校验——运维脚本被篡改的后果
	// 远比其他类型的内容严重，给出校验值是这类资源的最低要求。
	Checksum string `gorm:"size:64;index;comment:SHA256 校验和" json:"checksum"`

	Version string `gorm:"size:64;comment:版本号" json:"version"`
	// CompatibleWith 声明适用环境，如 "CentOS 7+/Ubuntu 20.04"。
	CompatibleWith string `gorm:"size:255;comment:适用环境" json:"compatibleWith"`

	CategoryID int64      `gorm:"index;comment:所属分类 ID" json:"categoryId"`
	Tags       StringList `gorm:"comment:标签" json:"tags"`

	// NeedLogin 标记是否需要登录才能下载。
	// 默认 false：资源下载是最强的获客钩子，先让人拿到东西再谈注册。
	NeedLogin bool `gorm:"default:false;comment:是否需登录下载" json:"needLogin"`

	// 审核痕迹：用户投稿先进入待审。
	ReviewInfo

	Status      string     `gorm:"size:16;index;default:draft;comment:状态" json:"status"`
	AuthorID    int64      `gorm:"index;comment:录入人用户 ID" json:"authorId"`
	PublishedAt *time.Time `gorm:"comment:发布时间" json:"publishedAt"`

	DownloadCount int64 `gorm:"default:0;comment:下载次数" json:"downloadCount"`
	ViewCount     int64 `gorm:"default:0;comment:浏览数" json:"viewCount"`
}

// TableName 固定表名。
func (r *Resource) TableName() string { return "resources" }

// IsPublished 判断资源是否已发布。
func (r *Resource) IsPublished() bool { return r.Status == ResourceStatusPublished }

// ReadableName 返回适合出现在下载框里的文件名。
// 入库时没有原始文件名就退回标题，避免前端拿到空字符串。
func (r *Resource) ReadableName() string {
	if r.FileName != "" {
		return r.FileName
	}
	return r.Title
}

// 下载链接类型。用于给链接分组排序与显示图标，
// 不需要穷举所有来源，认不出的落到 other 也照常展示。
const (
	ResourceLinkOfficial = "official" // 官网下载页
	ResourceLinkMirror   = "mirror"   // 国内镜像
	ResourceLinkPackage  = "package"  // 包管理器（apt/yum/brew/npm/pip）
	ResourceLinkRepo     = "repo"     // 代码仓库（GitHub/Gitee）
	ResourceLinkImage    = "image"    // 容器镜像（Docker Hub/镜像站）
	ResourceLinkOther    = "other"
)

// ResourceLink 是资源的一条外部下载链接。
//
// 为什么要有它：资源栏目的价值是「让人拿到东西」，而很多东西不适合由我们托管 ——
// 开源组件的安装包几十上百 MB、版本更新频繁、镜像站还分地域。
// 这类资源正确的做法是给出官方地址与国内镜像，而不是把 200MB 的 tar 包塞进自己的存储。
//
// 一个资源可以有多条链接（官网 + 镜像 + 包管理器），所以是独立的表而不是一个字段。
// 与 ToolGuide 同理不嵌 Base：纯子表，维护时整体替换，物理删除。
type ResourceLink struct {
	ID         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	ResourceID int64  `gorm:"index;comment:资源 ID" json:"resourceId"`
	Label      string `gorm:"size:120;comment:链接文字" json:"label"`
	URL        string `gorm:"size:512;comment:下载地址" json:"url"`
	// Kind 见 ResourceLinkOfficial 等常量，只影响图标与排序。
	Kind string `gorm:"size:16;comment:链接类型" json:"kind"`
	Note string `gorm:"size:255;comment:补充说明，如适用版本" json:"note"`
	Sort int    `gorm:"default:0;comment:排序" json:"sort"`
}

// TableName 固定表名。
func (ResourceLink) TableName() string { return "resource_links" }

// HasFile 判断这份资源是否有我们托管的文件（可直接下载）。
// 只有链接的资源是合法的：用户点链接跳转到官方或镜像站。
func (r *Resource) HasFile() bool { return r.FileKey != "" }
