package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/logger"
)

// 本文件是「内容从哪来」和「内容怎么串起来」的种子：
// 资讯订阅源、工具的四类实战教程、资源的外链下载地址。
//
// 这些种子不是演示用的假数据，而是产品主张的具体化：
//   - 订阅源默认抓官方发布渠道，说明资讯是「可溯源的」；
//   - 工具教程优先指向官方文档，说明我们不做搬运，只做索引与串联；
//   - 资源外链给出国内镜像，说明这份清单是给国内运维用的。

// ============================ 工具的四类实战教程 ============================

type toolGuideSeed struct {
	ToolSlug    string
	Kind        string
	Title       string
	URL         string
	ArticleSlug string
	Sort        int
}

var toolGuideSeeds = []toolGuideSeed{
	// Prometheus 是收录最完整的示例：四类都齐，能直接看出「部署 → 接入 → 调优 → 集群」这条路径。
	{"prometheus", model.ToolGuideDeploy, "官方安装文档（二进制 / Docker / Helm）",
		"https://prometheus.io/docs/prometheus/latest/installation/", "", 10},
	{"prometheus", model.ToolGuideImplement, "数据模型与指标命名实践",
		"https://prometheus.io/docs/practices/naming/", "", 20},
	{"prometheus", model.ToolGuideOptimize, "高基数标签与采集目标治理",
		"https://prometheus.io/docs/practices/instrumentation/", "", 30},
	{"prometheus", model.ToolGuideCluster, "kube-prometheus：集群化部署与告警规则集",
		"https://github.com/prometheus-operator/kube-prometheus", "", 40},

	{"grafana", model.ToolGuideDeploy, "官方安装指引",
		"https://grafana.com/docs/grafana/latest/setup-grafana/installation/", "", 10},
	{"grafana", model.ToolGuideImplement, "数据源接入与大盘模板",
		"https://grafana.com/docs/grafana/latest/datasources/", "", 20},
	{"grafana", model.ToolGuideOptimize, "大盘性能优化：降采样与查询缓存",
		"https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/best-practices/", "", 30},

	// 站内实战教程的关联示例：实施阶段遇到发布后 Pod 起不来，这篇文章正好接上。
	{"argo-cd", model.ToolGuideDeploy, "Argo CD 官方入门",
		"https://argo-cd.readthedocs.io/en/stable/getting_started/", "", 10},
	{"argo-cd", model.ToolGuideImplement, "", "", "argocd-canary-release", 20},
	{"argo-cd", model.ToolGuideCluster, "高可用部署（HA）",
		"https://argo-cd.readthedocs.io/en/stable/operator-manual/high_availability/", "", 30},

	{"ansible", model.ToolGuideDeploy, "官方安装指南",
		"https://docs.ansible.com/ansible/latest/installation_guide/index.html", "", 10},
	{"ansible", model.ToolGuideImplement, "Playbook 编写指南",
		"https://docs.ansible.com/ansible/latest/playbook_guide/index.html", "", 20},
	{"ansible", model.ToolGuideOptimize, "执行性能调优：fork、pipelining 与 facts 缓存",
		"https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_strategies.html", "", 30},

	{"openresty", model.ToolGuideDeploy, "OpenResty 官方安装",
		"https://openresty.org/en/installation.html", "", 10},
	{"openresty", model.ToolGuideOptimize, "Nginx 官方调优与指令参考",
		"https://nginx.org/en/docs/http/ngx_http_core_module.html", "", 20},
}

// seedToolGuides 写入工具的教程关联，幂等。
//
// 幂等键是「该工具是否已有任何关联」：教程清单是整体维护的，
// 运营调过顺序之后，下次启动不该被种子覆盖回默认值。
//
// 注意判断必须按工具**分组后**做一次，不能放在逐条循环里：
// 逐条判断会写成「写完第一篇文章的教程后，第二篇就看到 count>0 直接跳过」，
// 结果每个工具只种进一条，且表面上看不出错。
func seedToolGuides(ctx context.Context, db *gorm.DB) error {
	order, grouped := groupGuidesByTool()

	for _, toolSlug := range order {
		var tool model.Tool
		if err := db.WithContext(ctx).Where("slug = ?", toolSlug).First(&tool).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return fmt.Errorf("查询工具 %s 失败: %w", toolSlug, err)
		}

		var count int64
		if err := db.WithContext(ctx).Model(&model.ToolGuide{}).
			Where("tool_id = ?", tool.ID).Count(&count).Error; err != nil {
			return fmt.Errorf("统计工具 %s 教程数失败: %w", toolSlug, err)
		}
		if count > 0 {
			continue
		}

		rows := make([]model.ToolGuide, 0, len(grouped[toolSlug]))
		for _, s := range grouped[toolSlug] {
			articleID := int64(0)
			if s.ArticleSlug != "" {
				var article model.Article
				if err := db.WithContext(ctx).Where("slug = ?", s.ArticleSlug).First(&article).Error; err != nil {
					// 文章种子没写成功时不要让整个种子流程失败，跳过这条关联即可。
					logger.Warn("教程关联的文章不存在，跳过", "slug", s.ArticleSlug)
					continue
				}
				articleID = article.ID
			}
			rows = append(rows, model.ToolGuide{
				ToolID:    tool.ID,
				Kind:      s.Kind,
				Title:     s.Title,
				URL:       s.URL,
				ArticleID: articleID,
				Sort:      s.Sort,
			})
		}
		if len(rows) == 0 {
			continue
		}
		if err := db.WithContext(ctx).Create(&rows).Error; err != nil {
			return fmt.Errorf("写入工具 %s 的教程失败: %w", toolSlug, err)
		}
	}
	return nil
}

// groupGuidesByTool 保持出现顺序地把种子按工具分组。
func groupGuidesByTool() ([]string, map[string][]toolGuideSeed) {
	// 自研 CMDB 平台的四类关联在 seed_tools_cmdb.go 的 cmdbToolGuideSeeds，
	// 这里合并后再分组，分组与幂等逻辑不变。
	seeds := make([]toolGuideSeed, 0, len(toolGuideSeeds)+len(cmdbToolGuideSeeds))
	seeds = append(seeds, toolGuideSeeds...)
	seeds = append(seeds, cmdbToolGuideSeeds...)

	order := make([]string, 0, 8)
	grouped := make(map[string][]toolGuideSeed, 8)
	for _, s := range seeds {
		if _, ok := grouped[s.ToolSlug]; !ok {
			order = append(order, s.ToolSlug)
		}
		grouped[s.ToolSlug] = append(grouped[s.ToolSlug], s)
	}
	return order, grouped
}

// ============================ 资源的外链下载 ============================

type resourceLinkSeed struct {
	ResourceSlug string
	Label        string
	URL          string
	Kind         string
	Note         string
	Sort         int
}

var resourceLinkSeeds = []resourceLinkSeed{
	{"nginx-production-baseline", "Nginx 官方文档", "https://nginx.org/en/docs/",
		model.ResourceLinkOfficial, "指令语义以官方为准，模板注释里标了常见的坑", 10},
	{"nginx-production-baseline", "Nginx 官方下载（含各发行版包）", "https://nginx.org/en/download.html",
		model.ResourceLinkOfficial, "编译参数与本模板的模块裁剪需要对齐", 20},
	{"mysql-replication-lag-diagnose", "MySQL 官方文档：复制延迟排查",
		"https://dev.mysql.com/doc/refman/8.0/en/replication-implementation-details.html",
		model.ResourceLinkOfficial, "脚本输出里 SHOW SLAVE STATUS 各字段的含义", 10},
	{"linux-health-check-scripts", "Linux 性能分析工具速查（Brendan Gregg）",
		"https://www.brendangregg.com/linuxperf.html", model.ResourceLinkOfficial,
		"脚本只覆盖常规项，深挖 IO 与调度时看这张图", 10},

	// 纯外链资源：不托管安装包，只给出下载入口。
	{"docker-engine-download", "Docker Engine 官方安装指引",
		"https://docs.docker.com/engine/install/", model.ResourceLinkOfficial,
		"按发行版给的仓库配置方式，比下静态二进制更好维护", 10},
	{"docker-engine-download", "阿里云 Docker 镜像加速说明",
		"https://help.aliyun.com/zh/acr/user-guide/accelerate-the-pulls-of-docker-official-images",
		model.ResourceLinkMirror, "拉官方镜像慢时先配这个", 20},
	{"docker-engine-download", "Docker 官方镜像仓库",
		"https://hub.docker.com/", model.ResourceLinkImage, "查 tag 与架构支持", 30},

	{"openjdk-21-download", "Eclipse Temurin（Adoptium）下载",
		"https://adoptium.net/temurin/releases/?version=21", model.ResourceLinkOfficial,
		"生产推荐 LTS，注意区分 JRE 与 JDK", 10},
	{"openjdk-21-download", "Temurin 清华镜像",
		"https://mirrors.tuna.tsinghua.edu.cn/Adoptium/", model.ResourceLinkMirror,
		"国内下载快很多，校验 sha256 再上线", 20},
	{"openjdk-21-download", "Amazon Corretto 21",
		"https://aws.amazon.com/corretto/", model.ResourceLinkOfficial,
		"长期免费提供安全更新，适合不愿跟 Oracle 版本节奏的团队", 30},
}

// seedResourceLinks 写入资源的外链，幂等（该资源已有链接就跳过）。
//
// 与工具教程同样的坑：判断必须按资源分组做一次，
// 否则第一个链接写进去之后，同一资源的后续链接会被判成「已有」全部跳过。
func seedResourceLinks(ctx context.Context, db *gorm.DB) error {
	order := make([]string, 0, 8)
	grouped := make(map[string][]resourceLinkSeed, 8)
	for _, s := range resourceLinkSeeds {
		if _, ok := grouped[s.ResourceSlug]; !ok {
			order = append(order, s.ResourceSlug)
		}
		grouped[s.ResourceSlug] = append(grouped[s.ResourceSlug], s)
	}

	for _, resSlug := range order {
		var res model.Resource
		if err := db.WithContext(ctx).Where("slug = ?", resSlug).First(&res).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return fmt.Errorf("查询资源 %s 失败: %w", resSlug, err)
		}

		var count int64
		if err := db.WithContext(ctx).Model(&model.ResourceLink{}).
			Where("resource_id = ?", res.ID).Count(&count).Error; err != nil {
			return fmt.Errorf("统计资源 %s 链接数失败: %w", resSlug, err)
		}
		if count > 0 {
			continue
		}

		rows := make([]model.ResourceLink, 0, len(grouped[resSlug]))
		for _, s := range grouped[resSlug] {
			rows = append(rows, model.ResourceLink{
				ResourceID: res.ID,
				Label:      s.Label,
				URL:        s.URL,
				Kind:       s.Kind,
				Note:       s.Note,
				Sort:       s.Sort,
			})
		}
		if err := db.WithContext(ctx).Create(&rows).Error; err != nil {
			return fmt.Errorf("写入资源 %s 的下载链接失败: %w", resSlug, err)
		}
	}
	return nil
}

// ============================ 运维资讯订阅源 ============================

type newsSourceSeed struct {
	Name         string
	URL          string
	Kind         string
	CategorySlug string
	Tags         []string
	AutoPublish  bool
	Sort         int
}

// 订阅源的取舍：只收「发布渠道」而不是聚合站。
//
// 聚合站（各类资讯门户）看起来内容丰富，但同一条消息会被多个站转载，
// 待审池里全是重复标题，人工反而要花更多时间挑。
// 官方 feed 与 Releases 天然一条消息只出现一次，且时间与版本号准确。
//
// autoPublish 只对「标题本身就是中文、且没有夸张空间」的源打开。
// Kubernetes Blog 是英文长文（标题多为问句或专题名），进待审池人工编译后再发；
// 版本发布类源由抓取侧统一生成中文标题与摘要，可以放心自动上线。
var newsSourceSeeds = []newsSourceSeed{
	{"Kubernetes Blog", "https://kubernetes.io/feed.xml", model.NewsSourceRSS,
		"cloud-native", []string{"Kubernetes", "云原生"}, false, 10},
	{"Kubernetes Releases", "kubernetes/kubernetes", model.NewsSourceGitHub,
		"cloud-native", []string{"Kubernetes", "版本发布"}, true, 20},
	{"Prometheus Releases", "prometheus/prometheus", model.NewsSourceGitHub,
		"observability", []string{"Prometheus", "监控"}, true, 30},
	{"Grafana Releases", "grafana/grafana", model.NewsSourceGitHub,
		"observability", []string{"Grafana", "监控"}, true, 40},
	{"OpenResty Releases", "openresty/openresty", model.NewsSourceGitHub,
		"middleware", []string{"OpenResty", "Nginx"}, true, 50},
	{"开源中国资讯", "https://www.oschina.net/news/rss", model.NewsSourceRSS,
		"open-source", []string{"开源软件"}, false, 90},
}

// seedNewsSources 写入订阅源，按名称幂等。
func seedNewsSources(ctx context.Context, db *gorm.DB) error {
	for _, s := range newsSourceSeeds {
		var existing model.NewsSource
		err := db.WithContext(ctx).Where("name = ?", s.Name).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询订阅源 %s 失败: %w", s.Name, err)
		}

		item := model.NewsSource{
			Name:        s.Name,
			URL:         s.URL,
			Kind:        s.Kind,
			CategoryID:  categoryIDBySlug(ctx, db, s.CategorySlug),
			Tags:        model.StringList(s.Tags),
			Enabled:     true,
			AutoPublish: s.AutoPublish,
			Sort:        s.Sort,
		}
		if err := db.WithContext(ctx).Create(&item).Error; err != nil {
			return fmt.Errorf("写入订阅源 %s 失败: %w", s.Name, err)
		}
	}
	return nil
}
