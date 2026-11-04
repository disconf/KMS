package bootstrap

// 本文件给「事故复盘」栏目准备三篇示例正文。
//
// 复盘是平台上唯一一种"读别人的失败"的内容形态：教程教怎么做对，
// 复盘讲怎么错、错在哪、下次怎么不重样。它比教程更难写——要求作者
// 承认自己的误判，而互联网上愿意这么写的人不多，所以起步阶段必须
// 先把样子摆出来：读者看到"原来复盘该是这个结构"，才可能跟着写。
//
// 内容纪律与工具教程一致：只写方法与通用故障形态，不写任何真实客户、
// 域名、IP 与内部系统名；每篇结尾明确标注"按常见故障形态整理"，
// 不假装这些是我们平台上真实发生过的事故。
//
// 正文放在 data/articles/pm-*.md，与工具教程共用同一个 embed 文件系统
// （见 seed_tool_articles.go 的 //go:embed data/articles/*.md）。
// 幂等与普通文章一致：按 slug 判存在，已存在就跳过。

import (
	"fmt"

	"github.com/opsexpert/platform/internal/model"
)

// postmortemMeta 是一篇复盘文章的元数据；正文按 File 从 embed 里读。
//
// 单独定义而不是复用 toolArticleMeta：那个类型代表"工具教程集里的一节"，
// 名字对不上会在半年后误导读者——两个概念共用一个结构，早晚有人
// 往里加一个只对其中一方有意义的字段。
type postmortemMeta struct {
	articleSeed
	File string
}

var postmortemMetas = []postmortemMeta{
	{
		articleSeed: articleSeed{
			Title:        "40GB binlog 写满数据盘：一次主从延迟 50 分钟的复盘",
			Slug:         "postmortem-mysql-disk-full",
			CategorySlug: "database",
			Summary:      "从库磁盘被 binlog 与 relay log 写满，SQL 线程停止，业务静默读到 40 分钟前的旧数据。复盘重点不是「删日志」，而是「谁在写、为什么没人预警」。",
			Tags:         []string{"MySQL", "主从复制", "磁盘", "故障复盘"},
			ViewCount:    1280,
			LikeCount:    46,
		},
		File: "pm-mysql-disk-full.md",
	},
	{
		articleSeed: articleSeed{
			Title:        "证书没换进进程：一次 26 分钟全站 502 的复盘",
			Slug:         "postmortem-cert-not-reloaded",
			CategorySlug: "security",
			Summary:      "续签脚本把新证书拷到了位，却没人 reload nginx，进程仍持有过期证书。磁盘文件是新的、nginx -t 通过、端口能连——三个「看起来正常」骗过了所有人。",
			Tags:         []string{"HTTPS", "证书", "Nginx", "故障复盘"},
			ViewCount:    960,
			LikeCount:    38,
		},
		File: "pm-cert-expired.md",
	},
	{
		articleSeed: articleSeed{
			Title:        "conntrack 表满导致节点 NotReady：一次 K8s 节点失联的复盘",
			Slug:         "postmortem-conntrack-full",
			CategorySlug: "cloud-native",
			Summary:      "节点报 NotReady，但 Pod 还在跑、kubelet 也没挂——内核 conntrack 表被打满后开始丢包，kubelet 心跳超时。一条 dmesg 找到了方向。",
			Tags:         []string{"Kubernetes", "conntrack", "网络", "故障复盘"},
			ViewCount:    1120,
			LikeCount:    41,
		},
		File: "pm-conntrack-full.md",
	},
}

// postmortemArticleSeeds 把元数据与 embed 正文拼成文章种子。
//
// 正文缺失视为代码错误：编译期 embed 已经保证了文件存在，
// 运行期再读不到只能是路径写错。静默跳过会让栏目里出现空文章，
// 比启动失败更难发现。
func postmortemArticleSeeds() []articleSeed {
	out := make([]articleSeed, 0, len(postmortemMetas))
	for _, m := range postmortemMetas {
		data, err := toolArticlesFS.ReadFile("data/articles/" + m.File)
		if err != nil {
			panic(fmt.Sprintf("读取复盘正文 %s 失败: %v", m.File, err))
		}
		seed := m.articleSeed
		seed.Channel = model.ChannelOpsPostmortem
		seed.Content = string(data)
		out = append(out, seed)
	}
	return out
}
