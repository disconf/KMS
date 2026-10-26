package model

import "time"

// 订阅源类型。
const (
	// NewsSourceRSS 标准 RSS 2.0 / Atom 订阅，覆盖绝大多数技术博客与官方公告。
	NewsSourceRSS = "rss"
	// NewsSourceGitHub 仓库 Releases，用来回答「这个组件发新版本了吗」。
	NewsSourceGitHub = "github"
)

// NewsSource 是运维资讯的订阅源。
//
// 为什么要落库而不是写死在代码里：源会失效、会换域名、会新增。
// 运营发现某个组件发了新的官方博客，应当能立刻加进去，
// 而不是等下一次发版——资讯的价值就在时效。
//
// 不存任何访问令牌：GitHub 匿名调用每小时 60 次，按 30 分钟一次、
// 十来个源算完全够用。一旦引入 token 字段，就得解决加密存储与轮换，
// 而这层复杂度当前换不来实际收益。
type NewsSource struct {
	Base

	Name string `gorm:"size:120;uniqueIndex;comment:来源名称" json:"name"`
	// URL 是 feed 地址（kind=rss）或仓库标识（kind=github，形如 owner/repo）。
	URL  string `gorm:"size:512;comment:订阅地址或仓库" json:"url"`
	Kind string `gorm:"size:16;default:rss;comment:类型 rss|github" json:"kind"`

	// CategoryID / Tags 会带到抓取出来的文章上，省得每条再手工分类。
	CategoryID int64      `gorm:"index;comment:默认分类 ID" json:"categoryId"`
	Tags       StringList `gorm:"comment:默认标签" json:"tags"`

	Enabled bool `gorm:"default:true;index;comment:是否启用" json:"enabled"`
	// AutoPublish 为 false（默认）时抓到的东西进待审池，人工过一遍再上线。
	//
	// 默认不自动发布是刻意的：自动抓取解决的是「不用人找」，
	// 不是「不用人看」。直接把第三方标题推上首页，一次翻车就要靠删稿挽回。
	// 对特别信任的官方源可以单独打开。
	AutoPublish bool `gorm:"default:false;comment:抓取后直接发布" json:"autoPublish"`

	// 抓取状态，用于后台一眼看出哪个源坏了。
	LastFetchedAt *time.Time `gorm:"comment:最近抓取时间" json:"lastFetchedAt"`
	LastStatus    string     `gorm:"size:16;comment:最近结果 ok|error" json:"lastStatus"`
	LastError     string     `gorm:"size:500;comment:最近错误" json:"lastError"`
	// FetchedCount 累计写入条数（去重后），用于判断源是否还有效。
	FetchedCount int64 `gorm:"default:0;comment:累计抓取条数" json:"fetchedCount"`

	Sort int `gorm:"default:0;comment:排序" json:"sort"`
}

// TableName 固定表名。
func (NewsSource) TableName() string { return "news_sources" }

// IsEnabled 判断订阅源是否启用。
func (s *NewsSource) IsEnabled() bool { return s.Enabled }
