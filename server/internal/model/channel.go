package model

// ContentKind 栏目承载的内容模型。
//
// 五个栏目看着并列，底层却落在四种不同的数据结构上：
// 教程与资讯共用文章表，工具、题库、资源各有一张专属表。
// 前端只认 Kind —— 它决定用哪种卡片渲染、点进去跳到哪个详情页，
// 所以栏目清单里必须带上，不能让前端靠 slug 硬编码判断。
const (
	ContentKindArticle  = "article"  // 图文长文（教程 / 资讯）
	ContentKindTool     = "tool"     // 开源工具卡片
	ContentKindQuiz     = "quiz"     // 题库题目
	ContentKindResource = "resource" // 可下载资源
)

// 栏目 slug。同时充当 articles.channel 的取值，两处共用一个常量避免写岔。
//
// 注意 slug 与展示名不是一回事：`linux-tutorial` 这个 slug 是栏目刚上线时定的，
// 后来内容扩到「不只是 Linux」，展示名改成了「运维实战教程」。
// **slug 不改**——它已经进了外链、收藏与前端路由，改名会让这些全部 404，
// 而用户看到的只有展示名。常量名同样保留，避免一次无意义的全局重命名。
const (
	ChannelLinuxTutorial = "linux-tutorial"
	ChannelOpsNews       = "ops-news"
	ChannelOpenSource    = "open-source-tools"
	ChannelOpsQuiz       = "ops-quiz"
	ChannelResources     = "resources"
)

// ChannelMeta 描述一个栏目。
//
// 刻意做成代码内的注册表而不是数据库表：这五个栏目是产品结构的一部分，
// 增减栏目本就该改代码、走发布，放进数据库只会多一份需要同步的真相。
// 分类、标签这些运营随时要调的维度才是表。
type ChannelMeta struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	Icon string `json:"icon"`
	Desc string `json:"desc"`
	Kind string `json:"kind"`
	Sort int    `json:"sort"`
	// Path 是前端路由前缀，由服务端给出，避免三端各写一份拼错。
	Path string `json:"path"`
}

// channels 是有序的栏目注册表，Sort 即导航栏顺序。
var channels = []ChannelMeta{
	{
		Slug: ChannelLinuxTutorial,
		Name: "运维实战教程",
		Icon: "terminal",
		Desc: "从排查思路到落地命令的完整实战记录：Linux、网络、中间件、云原生",
		Kind: ContentKindArticle,
		Sort: 10,
		Path: "/kb/linux-tutorial",
	},
	{
		Slug: ChannelOpsQuiz,
		Name: "运维题库",
		Icon: "quiz",
		Desc: "在线答题、即时判分，附详细解析与错题本",
		Kind: ContentKindQuiz,
		Sort: 20,
		Path: "/kb/quiz",
	},
	{
		Slug: ChannelOpsNews,
		Name: "运维资讯",
		Icon: "news",
		Desc: "版本发布、安全公告与行业动态",
		Kind: ContentKindArticle,
		Sort: 30,
		Path: "/kb/news",
	},
	{
		Slug: ChannelOpenSource,
		Name: "开源工具推荐",
		Icon: "tools",
		Desc: "经过实战验证的运维工具，含部署方式与适用场景",
		Kind: ContentKindTool,
		Sort: 40,
		Path: "/kb/tools",
	},
	{
		Slug: ChannelResources,
		Name: "资源下载",
		Icon: "download",
		Desc: "脚本、配置模板与排查手册，下载即用",
		Kind: ContentKindResource,
		Sort: 50,
		Path: "/kb/resources",
	},
}

// AllChannels 返回全部栏目（按导航顺序）。
func AllChannels() []ChannelMeta {
	out := make([]ChannelMeta, len(channels))
	copy(out, channels)
	return out
}

// LookupChannel 按 slug 查栏目，不存在时 ok 为 false。
func LookupChannel(slug string) (ChannelMeta, bool) {
	for _, c := range channels {
		if c.Slug == slug {
			return c, true
		}
	}
	return ChannelMeta{}, false
}

// ArticleChannels 返回所有以文章表承载的栏目 slug。
// 用于校验 articles.channel 的取值合法性。
func ArticleChannels() []string {
	out := make([]string, 0, len(channels))
	for _, c := range channels {
		if c.Kind == ContentKindArticle {
			out = append(out, c.Slug)
		}
	}
	return out
}

// IsArticleChannel 判断给定 slug 是否为合法的文章栏目。
func IsArticleChannel(slug string) bool {
	for _, c := range channels {
		if c.Slug == slug && c.Kind == ContentKindArticle {
			return true
		}
	}
	return false
}
