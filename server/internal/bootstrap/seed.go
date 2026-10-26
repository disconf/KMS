package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/hash"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/pkg/storage"
)

// DevAdminPassword 是开发环境种子管理员的初始口令，生产环境不会创建该账号。
const DevAdminPassword = "Admin@12345"

// SeedAdminPasswordEnv 允许在初始化阶段指定管理员口令。
//
// 生产库的内容初始化要借 test 环境跑一遍种子（prod 会跳过全部种子），
// 此时若沿用人尽皆知的开发口令，等于把后台大门敞开在公网上。
// 部署脚本生成随机口令并通过这个变量传入。
const SeedAdminPasswordEnv = "SEED_ADMIN_PASSWORD"

// Seed 写入开发/测试环境的基础字典数据，幂等。
// 生产环境调用它会直接返回，不写入任何数据。
//
// store 用于把示例资源文件真正写进存储：资源下载栏目如果只有元数据没有文件，
// 点下载必然 404，前端联调时会被误判成接口坏了。
func Seed(ctx context.Context, db *gorm.DB, cfg *config.Config, store storage.Storage) error {
	if cfg.IsProd() {
		logger.Info("生产环境跳过种子数据写入")
		return nil
	}

	if err := seedSkills(ctx, db); err != nil {
		return err
	}
	if err := seedArticleCategories(ctx, db); err != nil {
		return err
	}
	// 先建管理员再建文章：种子文章要挂到管理员账号上，否则列表里作者名是空的。
	if err := seedAdmin(ctx, db, cfg); err != nil {
		return err
	}
	if err := seedArticles(ctx, db); err != nil {
		return err
	}
	// 专家必须排在技能之后：ExpertSkill 关联的是技能字典的主键。
	if err := seedExperts(ctx, db); err != nil {
		return err
	}
	// 示例问答要引用提问人与专家账号，排在专家之后。
	if err := seedQa(ctx, db); err != nil {
		return err
	}
	// 知识库其余栏目。分类表已就绪，这几张都引用它的主键。
	if err := seedTools(ctx, db); err != nil {
		return err
	}
	if err := seedQuizQuestions(ctx, db); err != nil {
		return err
	}
	// 系统题库（scripts/quizgen 生成，按方向各一千道以上）排在分类字典之后：
	// 题目要挂 category_id，分类还没建好就只能整批跳过。
	if err := seedQuizBank(ctx, db); err != nil {
		return err
	}
	// 资讯订阅源与工具教程都引用分类与文章，必须排在它们之后。
	if err := seedNewsSources(ctx, db); err != nil {
		return err
	}
	if err := seedToolGuides(ctx, db); err != nil {
		return err
	}
	// 教程集引用工具与文章，排在两者之后。
	if err := seedToolBooks(ctx, db); err != nil {
		return err
	}
	if err := seedResources(ctx, db, store); err != nil {
		return err
	}
	// 外链挂在资源上，排在资源之后。
	return seedResourceLinks(ctx, db)
}

type skillSeed struct {
	Name     string
	Category string
	Sort     int
}

// 技能字典按方案第十一节"先窄后宽"原则，先只开 K8s / 数据库 / Linux 网络三个方向。
var skillSeeds = []skillSeed{
	{"Kubernetes", "云原生", 10},
	{"Docker", "云原生", 20},
	{"容器网络", "云原生", 30},
	{"MySQL", "数据库", 40},
	{"PostgreSQL", "数据库", 50},
	{"Redis", "数据库", 60},
	{"主从复制", "数据库", 70},
	{"Linux 系统", "操作系统", 80},
	{"Nginx", "中间件", 90},
	{"网络排查", "网络", 100},
	{"DNS", "网络", 110},
	{"负载均衡", "网络", 120},
	{"Shell 脚本", "自动化", 130},
	{"监控告警", "可观测性", 140},
	{"日志分析", "可观测性", 150},
}

func seedSkills(ctx context.Context, db *gorm.DB) error {
	for _, s := range skillSeeds {
		var existing model.Skill
		err := db.WithContext(ctx).Where("name = ?", s.Name).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询技能 %s 失败: %w", s.Name, err)
		}
		item := model.Skill{Name: s.Name, Category: s.Category, Sort: s.Sort, Enabled: true}
		if err := db.WithContext(ctx).Create(&item).Error; err != nil {
			return fmt.Errorf("写入技能 %s 失败: %w", s.Name, err)
		}
	}
	return nil
}

// 分类是全站内容共用的维度：文章、题库、工具、资源都挂同一张分类表。
//
// 为什么共用而不是每类内容各建一套：用户脑子里的分类只有一个 ——
// 「云原生」既是教程的分类，也是题目的分类、工具的分类。拆成三套之后，
// 同一个词要维护三遍，运营改一处另外两处就悄悄不一致了。
//
// 字段说明见 LegacySlug：栏目刚上线时分类叫 k8s / linux-network，
// 后来按运维领域重新划分，用旧 slug 做原地改名，避免关联断裂。
var categorySeeds = []struct {
	Name       string
	Slug       string
	LegacySlug string
	Sort       int
}{
	{"网络", "network", "linux-network", 10},
	{"Linux 系统", "linux", "", 20},
	{"云原生", "cloud-native", "k8s", 30},
	{"Windows Server", "windows-server", "", 40},
	{"中间件", "middleware", "", 50},
	{"开源软件", "open-source", "", 60},
	{"数据库", "database", "", 70},
	{"可观测性", "observability", "", 80},
	{"自动化与脚本", "automation", "", 90},
	{"安全与加固", "security", "", 100},
}

// seedArticleCategories 写入/校正分类字典，幂等。
//
// 这里不是简单的「没有就插」：分类改过名，必须按 LegacySlug 找到旧记录原地更新。
// 如果只按新 slug 判断存在性，旧记录会留下来变成重复分类，
// 而挂在它上面的文章、题目、工具会继续指向一个没人维护的孤儿分类。
func seedArticleCategories(ctx context.Context, db *gorm.DB) error {
	for _, c := range categorySeeds {
		var current model.ArticleCategory
		err := db.WithContext(ctx).Where("slug = ?", c.Slug).First(&current).Error
		if err == nil {
			// 新 slug 已经在库里，只校正展示名与排序，不动 slug。
			if current.Name != c.Name || current.Sort != c.Sort {
				updates := map[string]any{"name": c.Name, "sort": c.Sort}
				if err := db.WithContext(ctx).Model(&model.ArticleCategory{}).
					Where("id = ?", current.ID).Updates(updates).Error; err != nil {
					return fmt.Errorf("校正分类 %s 失败: %w", c.Slug, err)
				}
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询分类 %s 失败: %w", c.Slug, err)
		}

		// 再试旧 slug：命中就原地改名，category_id 不变，关联自动跟着走。
		if c.LegacySlug != "" {
			var legacy model.ArticleCategory
			err := db.WithContext(ctx).Where("slug = ?", c.LegacySlug).First(&legacy).Error
			if err == nil {
				updates := map[string]any{"slug": c.Slug, "name": c.Name, "sort": c.Sort}
				if err := db.WithContext(ctx).Model(&model.ArticleCategory{}).
					Where("id = ?", legacy.ID).Updates(updates).Error; err != nil {
					return fmt.Errorf("迁移分类 %s -> %s 失败: %w", c.LegacySlug, c.Slug, err)
				}
				logger.Info("分类已原地改名", "from", c.LegacySlug, "to", c.Slug)
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("查询旧分类 %s 失败: %w", c.LegacySlug, err)
			}
		}

		item := model.ArticleCategory{Name: c.Name, Slug: c.Slug, Sort: c.Sort, Enabled: true}
		if err := db.WithContext(ctx).Create(&item).Error; err != nil {
			return fmt.Errorf("写入分类 %s 失败: %w", c.Slug, err)
		}
	}
	return nil
}

// articleSeed 是开发环境示例文章，用于让前端第一次联调就有真实数据可渲染。
type articleSeed struct {
	Title        string
	Slug         string
	CategorySlug string
	Summary      string
	Content      string
	Tags         []string
	ViewCount    int64
	LikeCount    int64
}

var articleSeeds = []articleSeed{
	{
		Title:        "K8s Pod 频繁重启排查手册",
		Slug:         "k8s-pod-restart-troubleshooting",
		CategorySlug: "cloud-native",
		Summary:      "从 CrashLoopBackOff 到 OOMKilled，按现象反查原因的标准流程。",
		Content: "## 先看状态\n\n`kubectl get pod -o wide` 拿到重启次数与节点，`kubectl describe pod` 看 Last State。\n\n" +
			"## 常见原因\n\n1. **CrashLoopBackOff**：进程启动即退出。先看 `kubectl logs --previous`。\n" +
			"2. **OOMKilled**：内存超 limits。检查 `resources.limits.memory` 与运行时堆上限是否一致。\n" +
			"3. **探针失败**：liveness 超时过短，业务启动慢会被反复杀掉。\n\n" +
			"## 定位顺序\n\n先区分是「起不来」还是「跑一会儿死」，这两类的原因完全不同。",
		Tags:      []string{"Kubernetes", "容器", "故障排查"},
		ViewCount: 1280,
		LikeCount: 32,
	},
	{
		Title:        "MySQL 主从延迟的五个真实原因",
		Slug:         "mysql-replication-lag-causes",
		CategorySlug: "database",
		Summary:      "Seconds_Behind_Master 飙升时，按顺序排查这五处。",
		Content: "## 一、大事务\n\n单个事务写入几十万行，从库只能串行回放。拆批是唯一解。\n\n" +
			"## 二、从库单线程回放\n\nMySQL 5.7 起可开 `slave_parallel_workers`，但要注意同库并发仍受限。\n\n" +
			"## 三、从库上有大查询\n\n长查询持有 MDL，阻塞回放线程。\n\n" +
			"## 四、网络带宽打满\n\n`SHOW SLAVE STATUS` 里的 `Master_Log_File` 追不上就是这一条。\n\n" +
			"## 五、磁盘 IO 打满\n\n回放本身是写密集，从库磁盘规格低于主库就会持续落后。",
		Tags:      []string{"MySQL", "主从复制", "性能"},
		ViewCount: 964,
		LikeCount: 21,
	},
	{
		Title:        "一次 DNS 解析超时导致的线上抖动",
		Slug:         "dns-timeout-incident-review",
		CategorySlug: "network",
		Summary:      "应用偶发超时，最终定位到 resolv.conf 的 timeout 与 attempts 配置。",
		Content: "## 现象\n\n每分钟有个位数的 502，重启应用后短暂恢复。\n\n" +
			"## 排查\n\n`dig` 与 `getent hosts` 对比，发现走 glibc 解析时偶发 5 秒延迟。\n\n" +
			"## 根因\n\n`/etc/resolv.conf` 中两个 nameserver 只有一个可用，另一个静默丢包，" +
			"glibc 默认 `timeout:5 attempts:2`，最坏情况单次解析要等 10 秒。\n\n" +
			"## 处置\n\n清理不可用的 nameserver，并启用本地缓存如 `nscd` 或 `systemd-resolved`。",
		Tags:      []string{"DNS", "网络排查", "故障复盘"},
		ViewCount: 731,
		LikeCount: 18,
	},
	{
		Title:        "Argo CD 灰度发布落地：同步策略、探针与回滚",
		Slug:         "argocd-canary-release",
		CategorySlug: "cloud-native",
		Summary:      "把 Argo CD 从「自动同步」调成能安全上线的形态，重点在三个开关与回滚路径。",
		Content: "## 三个开关先定下来\n\n" +
			"`syncPolicy.automated` 打开之后，Git 一改集群就跟着变。这时候必须同时决定两件事：\n\n" +
			"- `prune: true`：删除 Git 里的资源时集群也跟着删。不开会留下大量孤儿资源，" +
			"但开了就要小心「误删一个 manifest 等于删线上服务」。\n" +
			"- `selfHeal: true`：有人手工 `kubectl edit` 之后会被拉回 Git 的状态。" +
			"这是 GitOps 的前提，但排障时临时改参数会被立刻覆盖，需要提前和团队说清楚。\n\n" +
			"## 灰度不要交给 Argo CD\n\n" +
			"Argo CD 负责「集群状态等于 Git 状态」，它不理解流量比例。" +
			"灰度要交给 Rollout（Argo Rollouts）或 Service Mesh：\n\n" +
			"```yaml\n" +
			"strategy:\n" +
			"  canary:\n" +
			"    steps:\n" +
			"      - setWeight: 10\n" +
			"      - pause: { duration: 5m }\n" +
			"      - setWeight: 50\n" +
			"      - pause: {}\n" +
			"```\n\n" +
			"`pause: {}` 是无限期暂停，等人工确认。这一步不能省——" +
			"自动推进到底的灰度只是「慢一点的滚动更新」，没有观察窗口就失去意义。\n\n" +
			"## 回滚路径要提前验证\n\n" +
			"`argocd app rollback` 回的是 Git 历史版本，前提是历史 commit 对应的 manifest 还能跑。" +
			"如果镜像 tag 用了 `latest`，回滚会拉到一个不知道是什么的版本。\n" +
			"**镜像必须用不可变 tag（commit sha 或语义化版本）**，这是灰度可回滚的前提。\n\n" +
			"## 上线后 Pod 起不来的排查顺序\n\n" +
			"1. `kubectl describe pod` 看事件，先区分是调度失败还是启动失败；\n" +
			"2. 启动失败看 `kubectl logs --previous`，容器反复重启时当前日志是空的；\n" +
			"3. 探针失败先怀疑 `initialDelaySeconds` 太短，而不是先怀疑应用坏了。",
		Tags:      []string{"Argo CD", "GitOps", "灰度发布"},
		ViewCount: 486,
		LikeCount: 12,
	},
}

func seedArticles(ctx context.Context, db *gorm.DB) error {
	var admin model.User
	adminID := int64(0)
	err := db.WithContext(ctx).Where("role = ?", model.RoleAdmin).Order("id ASC").First(&admin).Error
	switch {
	case err == nil:
		adminID = admin.ID
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 管理员账号缺失（如生产环境不播种）时文章先挂空作者，不影响展示。
	default:
		return fmt.Errorf("查询种子文章作者失败: %w", err)
	}

	// 工具教程集的站内正文（seed_tool_articles.go）与示例文章走同一条写入路径：
	// 同样按 slug 幂等、同样发布态、同样挂管理员名下。
	toolArticles := toolArticleSeeds()
	all := make([]articleSeed, 0, len(articleSeeds)+len(toolArticles))
	all = append(all, articleSeeds...)
	all = append(all, toolArticles...)

	for _, a := range all {
		var existing model.Article
		err := db.WithContext(ctx).Where("slug = ?", a.Slug).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询种子文章 %s 失败: %w", a.Slug, err)
		}

		categoryID := int64(0)
		var cat model.ArticleCategory
		if err := db.WithContext(ctx).Where("slug = ?", a.CategorySlug).First(&cat).Error; err == nil {
			categoryID = cat.ID
		}

		now := time.Now()
		item := model.Article{
			Title:          a.Title,
			Slug:           a.Slug,
			Summary:        a.Summary,
			Content:        a.Content,
			CategoryID:     categoryID,
			Tags:           model.StringList(a.Tags),
			SEOTitle:       a.Title,
			SEODescription: a.Summary,
			SEOKeywords:    strings.Join(a.Tags, ","),
			Source:         model.ArticleSourceManual,
			Status:         model.ArticleStatusPublished,
			AuthorID:       adminID,
			PublishedAt:    &now,
			ViewCount:      a.ViewCount,
			LikeCount:      a.LikeCount,
		}
		if err := db.WithContext(ctx).Create(&item).Error; err != nil {
			return fmt.Errorf("写入种子文章 %s 失败: %w", a.Slug, err)
		}
	}
	return nil
}

// seedAdmin 创建开发用管理员账号。
// 初始口令不落日志，见 README「开发环境账号」一节。
func seedAdmin(ctx context.Context, db *gorm.DB, cfg *config.Config) error {
	const adminPhone = "13800000000"

	password := strings.TrimSpace(os.Getenv(SeedAdminPasswordEnv))
	if password == "" {
		password = DevAdminPassword
	}
	encoded, err := hash.Hash(password)
	if err != nil {
		return err
	}

	var existing model.User
	err = db.WithContext(ctx).Where("phone = ?", adminPhone).First(&existing).Error
	if err == nil {
		// 已存在的账号只在没有口令时补一个。已经有口令说明有人改过，
		// 每次启动都覆盖会让改密这件事永远不生效。
		if existing.PasswordHash == "" {
			if err := db.WithContext(ctx).Model(&existing).
				Update("password_hash", encoded).Error; err != nil {
				return fmt.Errorf("补写管理员口令失败: %w", err)
			}
			logger.Info("已为既有管理员补写登录口令", "phone", adminPhone)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("查询管理员账号失败: %w", err)
	}

	admin := model.User{
		Phone:        model.StrPtr(adminPhone),
		Nickname:     "平台管理员",
		Role:         model.RoleAdmin,
		Status:       model.UserStatusActive,
		PasswordHash: encoded,
	}
	if err := db.WithContext(ctx).Create(&admin).Error; err != nil {
		return fmt.Errorf("创建管理员账号失败: %w", err)
	}

	logger.Info("已创建管理员账号", "phone", adminPhone, "env", cfg.App.Env)
	return nil
}

// expertSeed 是开发环境的示例专家。
//
// 存在的意义是让前端第一次联调专家列表 / 详情 / 工作台时就有真实数据，
// 不必先手工走一遍"注册 → 上传资质 → 申请 → 后台审核"。
type expertSeed struct {
	Phone    string
	Nickname string
	RealName string
	// IDCardNo 用真实校验位算过，方便直接拿去跑申请接口的参数校验。
	IDCardNo string

	Skills       []string
	ServiceModes []string

	Intro      string
	Experience string

	PricePerHourCents int64
	PricePerTaskCents int64

	Rating         float64
	OrderCount     int64
	GoodRate       float64
	ResolveRate    float64
	AvgResponseSec int64

	// 这两个数必须相等。种子里不造提现流水，一旦写成「累计 4680 元、余额 1286 元」，
	// 就等于声称这个人提过 3394 元却没有任何提现记录——后台对账的
	// 专家台账检查（累计收入 = 余额 + 冻结 + 已打款）会对每一个新库直接报红，
	// 而红得毫无意义，真出问题时反而没人看。演示数据也要自洽。
	BalanceCents     int64
	TotalIncomeCents int64

	AcceptingOrder bool
}

var expertSeeds = []expertSeed{
	{
		Phone:    "13900000001",
		Nickname: "陈工",
		RealName: "陈志远",
		IDCardNo: "110101199001011239",
		Skills:   []string{"Kubernetes", "Docker", "容器网络"},
		ServiceModes: []string{
			model.ServiceModeIM, model.ServiceModeVoice, model.ServiceModeScreen,
		},
		Intro: "十年云原生一线经验，主导过三次从虚拟机到 K8s 的整体迁移，" +
			"擅长处理集群升级、网络插件异常、调度失衡这类「能上生产」的问题。",
		Experience: "2015-2019 某电商平台 基础架构工程师，负责容器平台建设；\n" +
			"2019-2024 某云厂商 容器服务技术专家，支撑 2000+ 节点集群。",
		PricePerHourCents: 40000,
		PricePerTaskCents: 19900,
		Rating:            4.92,
		OrderCount:        186,
		GoodRate:          98.4,
		ResolveRate:       95.2,
		AvgResponseSec:    95,
		BalanceCents:      4680000,
		TotalIncomeCents:  4680000,
		AcceptingOrder:    true,
	},
	{
		Phone:    "13900000002",
		Nickname: "老周说库",
		RealName: "周明辉",
		IDCardNo: "110101198805061235",
		Skills:   []string{"MySQL", "主从复制", "PostgreSQL", "Redis"},
		ServiceModes: []string{
			model.ServiceModeIM, model.ServiceModeVoice,
		},
		Intro: "专注 MySQL 性能与高可用十二年，处理过上千次慢查询与主从延迟工单。" +
			"不做「重启试试」这类建议，只给能落地的执行方案。",
		Experience: "某银行核心系统 DBA 八年，负责两地三中心容灾；\n" +
			"后转做数据库咨询，服务过物流、零售多个行业的头部客户。",
		PricePerHourCents: 50000,
		PricePerTaskCents: 25900,
		Rating:            4.98,
		OrderCount:        241,
		GoodRate:          99.2,
		ResolveRate:       97.5,
		AvgResponseSec:    72,
		BalanceCents:      7320000,
		TotalIncomeCents:  7320000,
		AcceptingOrder:    true,
	},
	{
		Phone:    "13900000003",
		Nickname: "Lin 的网络笔记",
		RealName: "林薇",
		IDCardNo: "110101199507129996",
		Skills:   []string{"网络排查", "DNS", "Nginx", "负载均衡"},
		ServiceModes: []string{
			model.ServiceModeIM, model.ServiceModeScreen,
		},
		Intro: "网络方向出身，擅长从抓包结果反推问题链路。" +
			"处理过 DNS 递归超时、四层转发丢包、TLS 握手失败等疑难问题。",
		Experience: "运营商网络运维三年，互联网公司 SRE 五年；\n" +
			"负责过 CDN 回源链路优化与自建 DNS 集群改造。",
		PricePerHourCents: 32000,
		PricePerTaskCents: 15900,
		Rating:            4.85,
		OrderCount:        97,
		GoodRate:          96.9,
		ResolveRate:       93.8,
		AvgResponseSec:    140,
		BalanceCents:      1860000,
		TotalIncomeCents:  1860000,
		AcceptingOrder:    false,
	},
}

func seedExperts(ctx context.Context, db *gorm.DB) error {
	for _, s := range expertSeeds {
		expertID, err := ensureSeedExpert(ctx, db, s)
		if err != nil {
			return err
		}
		if expertID == 0 {
			continue // 已存在，跳过技能写入
		}
		if err := seedExpertSkills(ctx, db, expertID, s.Skills); err != nil {
			return err
		}
	}
	return nil
}

// ensureSeedExpert 保证示例专家账号与其档案存在，返回新建档案的 ID；
// 若档案已存在则返回 0 表示无需后续处理。
func ensureSeedExpert(ctx context.Context, db *gorm.DB, s expertSeed) (int64, error) {
	var user model.User
	err := db.WithContext(ctx).Where("phone = ?", s.Phone).First(&user).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, fmt.Errorf("查询示例专家账号 %s 失败: %w", s.Phone, err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user = model.User{
			Phone:    model.StrPtr(s.Phone),
			Nickname: s.Nickname,
			Role:     model.RoleExpert,
			Status:   model.UserStatusActive,
		}
		if err := db.WithContext(ctx).Create(&user).Error; err != nil {
			return 0, fmt.Errorf("创建示例专家账号 %s 失败: %w", s.Phone, err)
		}
	}

	var existing model.Expert
	err = db.WithContext(ctx).Where("user_id = ?", user.ID).First(&existing).Error
	if err == nil {
		return 0, nil // 档案已存在，说明之前播过种
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, fmt.Errorf("查询示例专家档案 %s 失败: %w", s.Phone, err)
	}

	now := time.Now()
	expert := model.Expert{
		UserID:            user.ID,
		RealName:          s.RealName,
		IDCardNo:          s.IDCardNo,
		CertFiles:         model.StringList{},
		Intro:             s.Intro,
		Experience:        s.Experience,
		PricePerHourCents: s.PricePerHourCents,
		PricePerTaskCents: s.PricePerTaskCents,
		ServiceModes:      model.StringList(s.ServiceModes),
		AcceptingOrder:    s.AcceptingOrder,
		Rating:            s.Rating,
		OrderCount:        s.OrderCount,
		GoodRate:          s.GoodRate,
		ResolveRate:       s.ResolveRate,
		AvgResponseSec:    s.AvgResponseSec,
		BalanceCents:      s.BalanceCents,
		TotalIncomeCents:  s.TotalIncomeCents,
		// 种子专家直接置为已通过，省掉手工审核这一步。
		Status:    model.ExpertStatusApproved,
		AppliedAt: now,
		AuditedAt: &now,
	}
	if err := db.WithContext(ctx).Create(&expert).Error; err != nil {
		return 0, fmt.Errorf("创建示例专家档案 %s 失败: %w", s.Phone, err)
	}
	return expert.ID, nil
}

func seedExpertSkills(ctx context.Context, db *gorm.DB, expertID int64, names []string) error {
	for i, name := range names {
		var skill model.Skill
		err := db.WithContext(ctx).Where("name = ?", name).First(&skill).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 技能字典里没有这个名字：跳过而不是报错，
			// 否则调整技能字典会让整个启动流程挂掉。
			logger.Warn("示例专家引用了不存在的技能，已跳过", "skill", name, "expert_id", expertID)
			continue
		}
		if err != nil {
			return fmt.Errorf("查询技能 %s 失败: %w", name, err)
		}

		link := model.ExpertSkill{
			ExpertID: expertID,
			SkillID:  skill.ID,
			Level:    model.SkillLevelSenior,
			// 首位技能给 expert 等级，让列表排序里能看出主次。
			Years: 6 + i*2,
		}
		if i == 0 {
			link.Level = model.SkillLevelExpert
		}
		if err := db.WithContext(ctx).Create(&link).Error; err != nil {
			return fmt.Errorf("创建专家技能关联失败 expert=%d skill=%s: %w", expertID, name, err)
		}
	}
	return nil
}
