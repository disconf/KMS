package bootstrap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/idgen"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/pkg/storage"
)

// 本文件是知识库其余三个栏目（开源工具推荐 / 运维题库 / 资源下载）的开发种子。
//
// 与文章种子同样的定位：让前端第一次联调就有真实数据可渲染，
// 不必先手工录入几十条内容才能看到列表长什么样。

// ============================ 开源工具推荐 ============================

type toolSeed struct {
	Name         string
	Slug         string
	Summary      string
	Content      string
	RepoURL      string
	HomepageURL  string
	License      string
	Language     string
	Stars        int64
	ForkCount    int64
	DeployModes  []string
	Tags         []string
	CategorySlug string
	Featured     bool
	Sort         int
	ViewCount    int64
}

var toolSeeds = []toolSeed{
	{
		Name:    "Prometheus",
		Slug:    "prometheus",
		Summary: "云原生监控的事实标准，拉模式采集 + PromQL 查询，配合 Alertmanager 做告警。",
		Content: "## 适用场景\n\n容器与微服务环境的指标监控。拉模式（pull）天然适配动态实例，" +
			"配合服务发现可以免去维护静态目标清单。\n\n" +
			"## 部署方式选择\n\n- **单机 binary**：快速验证，数据落本地 TSDB，不适合生产。\n" +
			"- **Helm Chart**：K8s 上的常规选择，`kube-prometheus-stack` 一并带上 Grafana 与告警规则。\n" +
			"- **Operator**：需要按 CRD 管理采集目标时使用。\n\n" +
			"## 落地时最容易踩的坑\n\n1. **基数爆炸**：把 `user_id`、`request_id` 之类的标签打进指标，" +
			"内存会在几小时内被撑爆。指标标签必须是有界的。\n" +
			"2. **默认只留 15 天**：长期看趋势需要远端存储或降采样，别等数据被删了才发现。\n" +
			"3. **告警规则没写 for**：瞬时抖动会刷出一堆无效告警，`for: 5m` 是基本礼节。",
		RepoURL:      "https://github.com/prometheus/prometheus",
		HomepageURL:  "https://prometheus.io",
		License:      "Apache-2.0",
		Language:     "Go",
		Stars:        56200,
		ForkCount:    9100,
		DeployModes:  []string{"binary", "docker", "helm", "k8s-operator"},
		Tags:         []string{"监控", "指标", "可观测性", "PromQL"},
		CategorySlug: "observability",
		Featured:     true,
		Sort:         100,
		ViewCount:    4820,
	},
	{
		Name:    "Grafana",
		Slug:    "grafana",
		Summary: "可视化与告警面板，支持 Prometheus / Loki / MySQL 等几十种数据源。",
		Content: "## 适用场景\n\n把散落在多个数据源的指标、日志、链路汇到一张面板上。" +
			"值班时最需要的不是更多数据，而是一屏能看懂的东西。\n\n" +
			"## 落地建议\n\n- **面板即代码**：用 JSON 或 Terraform 管理 Dashboard，" +
			"手点出来的面板无法评审、无法回滚、换个人就没人会改。\n" +
			"- **别把它当告警主入口**：Grafana 告警适合作为补充，" +
			"核心告警仍建议放在 Alertmanager 里统一收敛，避免两处告警口径不一致。\n" +
			"- **控制变量模板的基数**：`instance` 变量在一个大集群里能拉出上千个选项，面板会直接卡死。",
		RepoURL:      "https://github.com/grafana/grafana",
		HomepageURL:  "https://grafana.com",
		License:      "AGPL-3.0",
		Language:     "TypeScript",
		Stars:        64800,
		ForkCount:    12100,
		DeployModes:  []string{"binary", "docker", "helm"},
		Tags:         []string{"可视化", "Dashboard", "可观测性"},
		CategorySlug: "observability",
		Featured:     true,
		Sort:         95,
		ViewCount:    3960,
	},
	{
		Name:    "Argo CD",
		Slug:    "argo-cd",
		Summary: "K8s 声明式 GitOps 持续交付，以 Git 仓库为唯一事实来源。",
		Content: "## 适用场景\n\n需要「集群当前状态 = Git 仓库期望状态」这种可审计的发布方式时。" +
			"每一次变更都有 commit 记录，回滚就是 revert。\n\n" +
			"## 与 CI 的分工\n\nCI 负责构建镜像并更新 Git 中的镜像 tag；" +
			"Argo CD 负责把 Git 中的期望状态同步到集群。**不要让 CI 直接 kubectl apply**，" +
			"否则发布路径出现两条，出事时没人说得清线上是哪个版本。\n\n" +
			"## 注意点\n\n- **仓库凭证与权限**：Argo CD 需要有仓库读权限，" +
			"建议用只读 deploy key，不要用个人 token。\n" +
			"- **同步波次**：有依赖关系的资源用 sync wave 排序，否则会因为顺序问题反复自愈失败。",
		RepoURL:      "https://github.com/argoproj/argo-cd",
		HomepageURL:  "https://argo-cd.readthedocs.io",
		License:      "Apache-2.0",
		Language:     "Go",
		Stars:        18600,
		ForkCount:    5600,
		DeployModes:  []string{"helm", "k8s-operator", "binary"},
		Tags:         []string{"GitOps", "持续交付", "Kubernetes"},
		CategorySlug: "cloud-native",
		Featured:     false,
		Sort:         90,
		ViewCount:    2140,
	},
	{
		Name:    "Ansible",
		Slug:    "ansible",
		Summary: "无 Agent 的配置管理与批量运维工具，用 YAML 描述目标状态。",
		Content: "## 适用场景\n\n存量物理机与虚拟机的批量配置、发布与巡检。" +
			"被管节点不需要装 Agent，只要能 SSH 就能纳管，这是它在传统机房不可替代的原因。\n\n" +
			"## 使用要点\n\n- **幂等**：Playbook 要能反复执行而不产生副作用，" +
			"用 `template`、`copy` 这类模块而不是 `shell` 拼命令。\n" +
			"- **`shell` 是最后的选项**：它总是返回 changed，会让幂等性统计彻底失真。\n" +
			"- **控制并发**：`forks` 默认只有 5，几千台机器要调大，" +
			"但同时要注意目标端的连接数上限。\n" +
			"- **敏感变量进 Vault**：不要把密码写在 inventory 里提交到仓库。",
		RepoURL:      "https://github.com/ansible/ansible",
		HomepageURL:  "https://www.ansible.com",
		License:      "GPL-3.0",
		Language:     "Python",
		Stars:        63500,
		ForkCount:    23900,
		DeployModes:  []string{"binary", "docker", "source"},
		Tags:         []string{"配置管理", "批量运维", "自动化"},
		CategorySlug: "automation",
		Featured:     false,
		Sort:         85,
		ViewCount:    2870,
	},
	{
		Name:    "OpenResty",
		Slug:    "openresty",
		Summary: "基于 Nginx 与 LuaJIT 的 Web 平台，适合做网关与动态分流。",
		Content: "## 适用场景\n\n需要在 Nginx 层做动态逻辑时——灰度分流、鉴权、" +
			"响应改写。相比写 Nginx 模块，Lua 的迭代成本低一个数量级。\n\n" +
			"## 注意点\n\n- **Lua 代码里的阻塞调用是致命的**：Nginx 是事件驱动，" +
			"一个 `os.execute` 就会阻塞整个 worker。必须用 OpenResty 提供的非阻塞 API。\n" +
			"- **共享内存字典有上限**：`lua_shared_dict` 的大小要估算清楚，" +
			"写满后行为是静默丢数据，不会报错。\n" +
			"- **别在里面做复杂业务**：它擅长的是轻量流量处理，不是业务编排。",
		RepoURL:      "https://github.com/openresty/openresty",
		HomepageURL:  "https://openresty.org",
		License:      "BSD-2-Clause",
		Language:     "C",
		Stars:        12700,
		ForkCount:    1560,
		DeployModes:  []string{"binary", "docker", "source"},
		Tags:         []string{"网关", "Nginx", "Lua"},
		CategorySlug: "network",
		Featured:     false,
		Sort:         80,
		ViewCount:    1640,
	},
}

func seedTools(ctx context.Context, db *gorm.DB) error {
	adminID := findAdminID(ctx, db)

	// 三批种子合并处理：内容重的 5 个写在 toolSeeds，常用工具在
	// seed_tools_more.go 的 extraToolSeeds，自研的 CMDB 平台在
	// seed_tools_cmdb.go 的 cmdbToolSeeds，幂等逻辑完全相同。
	seeds := make([]toolSeed, 0, len(toolSeeds)+len(extraToolSeeds)+len(cmdbToolSeeds))
	seeds = append(seeds, toolSeeds...)
	seeds = append(seeds, extraToolSeeds...)
	seeds = append(seeds, cmdbToolSeeds...)

	for _, s := range seeds {
		var existing model.Tool
		err := db.WithContext(ctx).Where("slug = ?", s.Slug).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询种子工具 %s 失败: %w", s.Slug, err)
		}

		now := time.Now()
		item := model.Tool{
			Name:           s.Name,
			Slug:           s.Slug,
			Summary:        s.Summary,
			Content:        s.Content,
			RepoURL:        s.RepoURL,
			HomepageURL:    s.HomepageURL,
			License:        s.License,
			Language:       s.Language,
			Stars:          s.Stars,
			ForkCount:      s.ForkCount,
			StarsUpdated:   &now,
			DeployModes:    model.StringList(s.DeployModes),
			Tags:           model.StringList(s.Tags),
			CategoryID:     categoryIDBySlug(ctx, db, s.CategorySlug),
			SEOTitle:       s.Name + " 使用指南与落地建议",
			SEODescription: s.Summary,
			SEOKeywords:    strings.Join(s.Tags, ","),
			Status:         model.ToolStatusPublished,
			Featured:       s.Featured,
			Sort:           s.Sort,
			AuthorID:       adminID,
			PublishedAt:    &now,
			ViewCount:      s.ViewCount,
		}
		if err := db.WithContext(ctx).Create(&item).Error; err != nil {
			return fmt.Errorf("写入种子工具 %s 失败: %w", s.Slug, err)
		}
	}
	return nil
}

// ============================ 运维题库 ============================

type quizSeed struct {
	Stem         string
	Type         string
	Options      []model.QuizOption
	Correct      []string
	Analysis     string
	Difficulty   string
	Tags         []string
	CategorySlug string
	AnswerCount  int64
	CorrectCount int64
}

var quizSeeds = []quizSeed{
	{
		Stem: "Pod 处于 CrashLoopBackOff，下列哪项排查动作应当最先执行？",
		Type: model.QuizTypeSingle,
		Options: []model.QuizOption{
			{Key: "A", Text: "扩容 Deployment 副本数"},
			{Key: "B", Text: "kubectl logs --previous 查看上一次容器退出前的日志"},
			{Key: "C", Text: "重启所在节点"},
			{Key: "D", Text: "删除并重建 Pod"},
		},
		Correct: []string{"B"},
		Analysis: "CrashLoopBackOff 的含义是「进程反复启动后退出」，" +
			"所以要回答的是「它为什么退出」。当前容器的日志通常只有启动失败的那几行，" +
			"`--previous` 才能拿到上一次运行的完整输出。扩容、重启节点、删 Pod 都只是" +
			"掩盖现象，容器重新拉起后依旧会退出。",
		Difficulty:   model.QuizDifficultyEasy,
		Tags:         []string{"Kubernetes", "故障排查", "Pod"},
		CategorySlug: "cloud-native",
		AnswerCount:  1240,
		CorrectCount: 1093,
	},
	{
		Stem: "关于容器内存 OOMKilled，下列说法正确的是（多选）？",
		Type: model.QuizTypeMultiple,
		Options: []model.QuizOption{
			{Key: "A", Text: "进程超过 cgroup 内存上限时被内核杀掉，退出码通常为 137"},
			{Key: "B", Text: "只要调大 limits.memory 就一定能解决问题"},
			{Key: "C", Text: "Java 应用的堆上限若高于容器 limit，容器会比 JVM 更早被 OOM"},
			{Key: "D", Text: "requests 未设置时，调度可能把多个大内存 Pod 放到同一节点，加剧 OOM"},
		},
		Correct: []string{"A", "C", "D"},
		Analysis: "B 是典型的错误认知：盲目调大 limit 只是把问题推迟，" +
			"根因往往是内存泄漏或堆上限配置不当。C 是最常见的组合陷阱——" +
			"JVM 默认按宿主机内存计算堆大小，在容器里必须显式设置 `-XX:MaxRAMPercentage`。" +
			"D 说明 requests 不只是调度参数，它还决定了资源竞争时的相对权重。",
		Difficulty:   model.QuizDifficultyMedium,
		Tags:         []string{"Kubernetes", "内存", "OOMKilled"},
		CategorySlug: "cloud-native",
		AnswerCount:  986,
		CorrectCount: 512,
	},
	{
		Stem: "MySQL 主从延迟持续升高，SHOW SLAVE STATUS 中 Seconds_Behind_Master 很大，" +
			"且从库 IO 线程状态为 Waiting for master to send event。最可能的原因是？",
		Type: model.QuizTypeSingle,
		Options: []model.QuizOption{
			{Key: "A", Text: "从库 SQL 线程回放速度跟不上主库写入"},
			{Key: "B", Text: "从库无法连接到主库"},
			{Key: "C", Text: "主库 binlog 被误删"},
			{Key: "D", Text: "从库磁盘空间不足"},
		},
		Correct: []string{"A"},
		Analysis: "IO 线程状态为 `Waiting for master to send event` 说明它与主库的连接是正常的，" +
			"已经在等主库发送新事件——也就是说瓶颈不在网络传输，而在从库自身的回放。" +
			"若连接异常，状态会是 `Connecting to master` 或 `Reconnecting`。" +
			"定位方向应转向大事务、从库长查询阻塞回放、以及回放线程并发度。",
		Difficulty:   model.QuizDifficultyMedium,
		Tags:         []string{"MySQL", "主从复制", "性能"},
		CategorySlug: "database",
		AnswerCount:  842,
		CorrectCount: 473,
	},
	{
		Stem: "Redis 出现内存告警，排查时发现大量 key 没有设置过期时间。下列做法正确的是（多选）？",
		Type: model.QuizTypeMultiple,
		Options: []model.QuizOption{
			{Key: "A", Text: "直接执行 FLUSHALL 释放内存"},
			{Key: "B", Text: "用 SCAN 分批排查并清理无主 key，避免使用 KEYS"},
			{Key: "C", Text: "配置合理的内存淘汰策略，如 allkeys-lru"},
			{Key: "D", Text: "对业务写入的缓存 key 统一补充 TTL"},
		},
		Correct: []string{"B", "C", "D"},
		Analysis: "A 会清空全部数据，在缓存场景下等于把全部流量瞬时打到数据库，" +
			"大概率引发雪崩。B 强调不要用 `KEYS`——它是 O(N) 且阻塞单线程，" +
			"在线上执行会直接导致服务不可用。C 是兜底，D 才是根因治理：" +
			"缓存 key 不带 TTL，淘汰策略再好也只是延迟了问题爆发。",
		Difficulty:   model.QuizDifficultyMedium,
		Tags:         []string{"Redis", "内存", "缓存"},
		CategorySlug: "database",
		AnswerCount:  731,
		CorrectCount: 498,
	},
	{
		Stem: "应用偶发超时，排查发现 /etc/resolv.conf 配置了两个 nameserver，" +
			"其中第二个不可达。下列说法正确的是（多选）？",
		Type: model.QuizTypeMultiple,
		Options: []model.QuizOption{
			{Key: "A", Text: "glibc 默认 timeout:5 attempts:2，最坏情况下单次解析可能等待十几秒"},
			{Key: "B", Text: "nameserver 不可达时会立刻返回失败，不会产生延迟"},
			{Key: "C", Text: "在容器环境里应清理无效的 nameserver，或部署本地 DNS 缓存"},
			{Key: "D", Text: "这类问题通常表现为低频、无明显规律的偶发超时，容易被误判为应用问题"},
		},
		Correct: []string{"A", "C", "D"},
		Analysis: "B 说反了：nameserver 静默丢包（比如安全组未放通）不会立刻报错，" +
			"而是等到超时才重试下一个，这正是延迟的来源。" +
			"D 是这类问题最耗时的部分——它不像持续故障那样有明显的错误率抬升，" +
			"排查时很容易被归因为「应用偶发卡顿」而查错方向。",
		Difficulty:   model.QuizDifficultyHard,
		Tags:         []string{"DNS", "网络排查", "超时"},
		CategorySlug: "network",
		AnswerCount:  612,
		CorrectCount: 241,
	},
	{
		Stem: "判断：Nginx 的 worker_connections 设置为 1024，则单机最大并发连接数就是 1024。",
		Type: model.QuizTypeJudge,
		Options: []model.QuizOption{
			{Key: "A", Text: "正确"},
			{Key: "B", Text: "错误"},
		},
		Correct: []string{"B"},
		Analysis: "最大连接数 = worker_processes × worker_connections。" +
			"而且 Nginx 作为反向代理时，每个客户端连接会同时占用一个后端连接，" +
			"两者都要计入 worker_connections，所以实际可承载的客户端连接数还要再打对折。" +
			"这也是为什么调优时 `worker_connections` 与 `worker_processes` 必须一起看。",
		Difficulty:   model.QuizDifficultyHard,
		Tags:         []string{"Nginx", "性能", "并发"},
		CategorySlug: "middleware",
		AnswerCount:  548,
		CorrectCount: 219,
	},

	// ---------------- 以下按运维方向补齐，保证每个分类都有题可刷 ----------------
	{
		Stem: "df -h 显示根分区还有 20% 可用，但写入文件时报 No space left on device。最应先查什么？",
		Type: model.QuizTypeSingle,
		Options: []model.QuizOption{
			{Key: "A", Text: "扩容根分区"},
			{Key: "B", Text: "df -i 查看 inode 使用率"},
			{Key: "C", Text: "重启服务器释放句柄"},
			{Key: "D", Text: "清理 /tmp 目录"},
		},
		Correct: []string{"B"},
		Analysis: "磁盘空间与 inode 是两套独立的配额，空间够但 inode 耗尽时写入同样失败，" +
			"报错文案还完全一样。典型诱因是海量小文件：session 文件、缓存碎片、" +
			"没做滚动的容器日志。`df -i` 一眼就能区分是「没空间」还是「没 inode」。" +
			"扩容和重启都不解决问题——inode 数量在格式化时就定了。",
		Difficulty:   model.QuizDifficultyMedium,
		Tags:         []string{"Linux", "磁盘", "inode"},
		CategorySlug: "linux",
		AnswerCount:  1860,
		CorrectCount: 1204,
	},
	{
		Stem: "关于 Linux swap 的使用，下列说法正确的是（多选）？",
		Type: model.QuizTypeMultiple,
		Options: []model.QuizOption{
			{Key: "A", Text: "vm.swappiness=0 表示完全不使用 swap"},
			{Key: "B", Text: "频繁换入换出会抬高 iowait，表现为尾延迟明显变差"},
			{Key: "C", Text: "数据库类服务通常建议关闭 swap 或把 swappiness 调到很低"},
			{Key: "D", Text: "只要 swap 空间够大，内存不足带来的性能问题就解决了"},
		},
		Correct: []string{"B", "C"},
		Analysis: "A 是常见误解：`vm.swappiness=0` 只是**尽量**不用 swap，" +
			"在内存压力下内核仍会换出匿名页以避免 OOM。D 把 swap 当成了内存扩容——" +
			"swap 落在磁盘上，命中它意味着一次磁盘往返，数据库这类对延迟敏感的服务" +
			"宁可被 OOM 杀掉重启，也不愿整段时间都跑在 swap 上。" +
			"B 才是 swap 过量的真实体感：不是「慢一点」，而是长尾抖动。",
		Difficulty:   model.QuizDifficultyMedium,
		Tags:         []string{"Linux", "内存", "swap"},
		CategorySlug: "linux",
		AnswerCount:  1420,
		CorrectCount: 682,
	},
	{
		Stem: "Windows Server 上 IIS 应用池按默认策略定期回收后，用户登录态大面积丢失。" +
			"最合适的处理方式是？",
		Type: model.QuizTypeSingle,
		Options: []model.QuizOption{
			{Key: "A", Text: "关闭应用池回收，改为永不回收"},
			{Key: "B", Text: "把会话状态移出进程（State Server 或 Redis），保留回收策略"},
			{Key: "C", Text: "把回收间隔调得更短，让影响更平滑"},
			{Key: "D", Text: "增加应用池的最大工作进程数"},
		},
		Correct: []string{"B"},
		Analysis: "回收会结束工作进程，进程内 Session 自然一起消失——这是设计使然，" +
			"不是缺陷。A 把回收关掉等于放弃了「定期释放内存碎片与泄漏」这层保护，" +
			"长期运行后 w3wp.exe 会越吃越多。C 会让登录态丢得更频繁。" +
			"正解是把会话外置：State Server 适合单机，Redis 适合多节点共享。" +
			"D 增加工作进程（Web Garden）反而会让进程内会话在多个进程间不一致。",
		Difficulty:   model.QuizDifficultyMedium,
		Tags:         []string{"Windows Server", "IIS", "会话"},
		CategorySlug: "windows-server",
		AnswerCount:  386,
		CorrectCount: 214,
	},
	{
		Stem: "Kafka 消费者组频繁触发 rebalance，下列哪些是常见原因（多选）？",
		Type: model.QuizTypeMultiple,
		Options: []model.QuizOption{
			{Key: "A", Text: "session.timeout.ms 小于消费者实际发生 GC 停顿的时长"},
			{Key: "B", Text: "max.poll.interval.ms 小于两轮 poll 之间的业务处理耗时"},
			{Key: "C", Text: "消费者实例数超过分区数"},
			{Key: "D", Text: "消费线程被慢查询或同步调用阻塞，导致心跳发送不及时"},
		},
		Correct: []string{"A", "B", "D"},
		Analysis: "rebalance 的本质是「协调者认为有成员失联」。A、D 都作用于心跳：" +
			"心跳线程被阻塞或超时窗口太短，成员就会被踢出组。B 作用于 poll 循环——" +
			"处理太慢导致长时间不 poll，同样会被判定失联。" +
			"C 不会触发 rebalance：多出来的消费者只是分不到分区、空转着，" +
			"这是资源浪费而不是抖动来源。区分这两类现象能省下大量排查时间。",
		Difficulty:   model.QuizDifficultyHard,
		Tags:         []string{"Kafka", "中间件", "消费者组"},
		CategorySlug: "middleware",
		AnswerCount:  742,
		CorrectCount: 268,
	},
	{
		Stem: "判断：Nginx 配置 upstream keepalive 后，到上游的长连接就会生效。",
		Type: model.QuizTypeJudge,
		Options: []model.QuizOption{
			{Key: "A", Text: "正确"},
			{Key: "B", Text: "错误"},
		},
		Correct: []string{"B"},
		Analysis: "只写 `keepalive 32` 不够。Nginx 默认对上游使用 HTTP/1.0，" +
			"而 1.0 默认 `Connection: close`，连接用完即断。还必须同时配 " +
			"`proxy_http_version 1.1;` 和 `proxy_set_header Connection \"\";`（清空而不是 close）。" +
			"三条齐全才真正复用。漏了后两条时现象很有迷惑性：配置看起来生效了，" +
			"但 `ss` 里上游连接仍在快速 TIME_WAIT 堆积。",
		Difficulty:   model.QuizDifficultyHard,
		Tags:         []string{"Nginx", "中间件", "长连接"},
		CategorySlug: "middleware",
		AnswerCount:  964,
		CorrectCount: 331,
	},
	{
		Stem: "Elasticsearch 集群健康状态为 yellow，说明什么？",
		Type: model.QuizTypeSingle,
		Options: []model.QuizOption{
			{Key: "A", Text: "至少一个主分片未分配，部分数据不可查"},
			{Key: "B", Text: "主分片全部分配，但存在副本分片未分配"},
			{Key: "C", Text: "有节点掉线，集群正在选主"},
			{Key: "D", Text: "磁盘使用率超过水位线，索引被置为只读"},
		},
		Correct: []string{"B"},
		Analysis: "三色语义要记牢：green 全部主副分片就绪；yellow 主分片齐全、副本缺失——" +
			"数据可读可写但没有冗余；red 才是主分片缺失、部分数据不可用。" +
			"单节点集群几乎必然是 yellow，因为副本不能与主分片落在同一节点，" +
			"这不是故障。D 的典型表现是写入报 `blocked by: [FORBIDDEN/12/index read-only`，" +
			"健康状态可能仍是 green。",
		Difficulty:   model.QuizDifficultyMedium,
		Tags:         []string{"Elasticsearch", "开源软件", "集群"},
		CategorySlug: "open-source",
		AnswerCount:  812,
		CorrectCount: 546,
	},
	{
		Stem: "团队基于某个 GPLv3 组件做了二次开发，只以 SaaS 形式对外提供服务、" +
			"不向客户交付二进制。下列判断正确的是？",
		Type: model.QuizTypeSingle,
		Options: []model.QuizOption{
			{Key: "A", Text: "SaaS 通常不构成分发，一般不触发 GPL 的源码开放义务，但需重点评估 AGPL"},
			{Key: "B", Text: "只要不收费就不受协议约束"},
			{Key: "C", Text: "GPL 允许闭源分发，注明出处即可"},
			{Key: "D", Text: "必须把公司全部自有代码一并开源"},
		},
		Correct: []string{"A"},
		Analysis: "GPL 的义务绑定在「分发」这个动作上：用户拿不到二进制，通常就不触发。" +
			"但这正是 AGPL 存在的理由——它把「通过网络提供服务」也视为分发，" +
			"所以选型时必须先看协议版本，AGPL 组件做 SaaS 是要开源的。" +
			"C 混淆了 GPL 与 MIT/ Apache：GPL 的传染性要求衍生作品同样以 GPL 发布，" +
			"「注明出处」远远不够。D 过度恐慌——义务只覆盖衍生作品本身。",
		Difficulty:   model.QuizDifficultyHard,
		Tags:         []string{"开源协议", "合规", "GPL"},
		CategorySlug: "open-source",
		AnswerCount:  524,
		CorrectCount: 287,
	},
	{
		Stem: "Prometheus 内存持续上涨直至 OOM，排查发现指标标签里带上了 URL 路径参数。" +
			"正确的治理动作是（多选）？",
		Type: model.QuizTypeMultiple,
		Options: []model.QuizOption{
			{Key: "A", Text: "把 user_id、trace_id 也加进标签，便于精确定位"},
			{Key: "B", Text: "对路径做模板化归一（/user/:id），不做逐值打点"},
			{Key: "C", Text: "清理无业务价值的高基数标签与已废弃指标"},
			{Key: "D", Text: "直接给 Prometheus 加内存，容量跟上即可"},
		},
		Correct: []string{"B", "C"},
		Analysis: "时间序列数量 = 指标名 × 标签组合数，高基数标签会让它指数级膨胀。" +
			"单个请求 ID 作为标签几乎是一对一，等于给每个请求建一条时序——" +
			"A 是加速爆炸，不是解法。D 只能推迟故障：基数没有上界，内存就没有上界，" +
			"而且更大的内存意味着更长的 GC 停顿与更慢的查询。" +
			"B、C 才是把基数**变成有界**的唯一办法。",
		Difficulty:   model.QuizDifficultyMedium,
		Tags:         []string{"Prometheus", "可观测性", "基数"},
		CategorySlug: "observability",
		AnswerCount:  1032,
		CorrectCount: 619,
	},
	{
		Stem: "Linux 服务器 SSH 加固时，哪一项最容易被漏掉、但实际风险最高？",
		Type: model.QuizTypeSingle,
		Options: []model.QuizOption{
			{Key: "A", Text: "把默认端口 22 改掉"},
			{Key: "B", Text: "禁止 root 直接登录"},
			{Key: "C", Text: "只允许密钥登录，关闭密码认证"},
			{Key: "D", Text: "安装 fail2ban 封禁爆破 IP"},
		},
		Correct: []string{"C"},
		Analysis: "A 只是降低了被扫到的概率，对定向攻击无效；B 挡住了 root，" +
			"但普通账号的弱口令照样能被爆破——拿到普通账号再提权，路径并不长。" +
			"D 是事后补救，且分布式低频爆破能绕开阈值。" +
			"只有关闭密码认证，才把「猜口令」这条路彻底封死，这是从「提高成本」" +
			"变成「不可行」的差别。加固顺序建议：先确认密钥可登录，再关密码认证，" +
			"否则会把自己锁在门外。",
		Difficulty:   model.QuizDifficultyEasy,
		Tags:         []string{"安全加固", "SSH", "运维"},
		CategorySlug: "security",
		AnswerCount:  1584,
		CorrectCount: 1103,
	},
	{
		Stem: "判断：Ansible 的 shell 模块和 command 模块与 copy、service 一样，都是幂等的。",
		Type: model.QuizTypeJudge,
		Options: []model.QuizOption{
			{Key: "A", Text: "正确"},
			{Key: "B", Text: "错误"},
		},
		Correct: []string{"B"},
		Analysis: "command / shell 只是「执行一条命令」，Ansible 无法判断它该不该执行，" +
			"所以每次都会跑、每次都可能报 changed。要幂等得靠 `creates` / `removes` " +
			"这类条件参数，或用 `changed_when` / `failed_when` 自己声明判定口径。" +
			"copy、template、service 这些专用模块内部先比对状态再决定是否动手，" +
			"天生幂等。这也是写 playbook 的第一原则：能用专用模块就不要用 shell。",
		Difficulty:   model.QuizDifficultyEasy,
		Tags:         []string{"Ansible", "自动化", "幂等"},
		CategorySlug: "automation",
		AnswerCount:  1186,
		CorrectCount: 743,
	},
}

func seedQuizQuestions(ctx context.Context, db *gorm.DB) error {
	adminID := findAdminID(ctx, db)

	for i, s := range quizSeeds {
		// 题干没有唯一索引，用题干做幂等键：种子题目之间不会重复，
		// 且运营手动录入的题目几乎不可能与种子题干完全一致。
		var count int64
		if err := db.WithContext(ctx).Model(&model.QuizQuestion{}).
			Where("stem = ?", s.Stem).Count(&count).Error; err != nil {
			return fmt.Errorf("查询种子题目失败: %w", err)
		}
		if count > 0 {
			continue
		}

		now := time.Now()
		item := model.QuizQuestion{
			Stem:         s.Stem,
			Type:         s.Type,
			Options:      model.QuizOptionList(s.Options),
			Correct:      model.StringList(s.Correct),
			Analysis:     s.Analysis,
			Difficulty:   s.Difficulty,
			CategoryID:   categoryIDBySlug(ctx, db, s.CategorySlug),
			Tags:         model.StringList(s.Tags),
			Status:       model.QuizStatusPublished,
			AuthorID:     adminID,
			PublishedAt:  &now,
			AnswerCount:  s.AnswerCount,
			CorrectCount: s.CorrectCount,
		}
		if err := db.WithContext(ctx).Create(&item).Error; err != nil {
			return fmt.Errorf("写入种子题目 #%d 失败: %w", i+1, err)
		}
	}
	return nil
}

// ============================ 资源下载 ============================

type resourceSeed struct {
	Title          string
	Slug           string
	Summary        string
	Content        string
	FileName       string
	FileContent    string
	Version        string
	CompatibleWith string
	Tags           []string
	CategorySlug   string
	NeedLogin      bool
	DownloadCount  int64
	ViewCount      int64
}

var resourceSeeds = []resourceSeed{
	{
		Title:   "MySQL 主从延迟一键诊断脚本",
		Slug:    "mysql-replication-lag-diagnose",
		Summary: "一条命令输出 Seconds_Behind_Master、回放线程状态、大事务与长查询四类关键信息。",
		Content: "## 用法\n\n```\n./mysql-lag-diagnose.sh -h 127.0.0.1 -u root -p******\n```\n\n" +
			"## 输出解读\n\n- `Seconds_Behind_Master` 持续增大且 IO 线程正常 → 回放瓶颈，看下方的大事务与长查询。\n" +
			"- `Slave_SQL_Running` 为 No → 回放线程已中断，先看 `Last_SQL_Error`。\n" +
			"- `Long Running Queries` 非空 → 从库上的长查询持有 MDL，正在阻塞回放。\n\n" +
			"> 脚本只读取状态，不会修改任何数据，可放心在生产从库执行。",
		FileName: "mysql-lag-diagnose.sh",
		FileContent: `#!/usr/bin/env bash
#
# MySQL 主从延迟一键诊断
# 只读操作，不修改任何数据。
#
set -euo pipefail

HOST="127.0.0.1"
PORT="3306"
USER="root"

while getopts "h:P:u:p:" opt; do
  case "$opt" in
    h) HOST="$OPTARG" ;;
    P) PORT="$OPTARG" ;;
    u) USER="$OPTARG" ;;
    p) export MYSQL_PWD="$OPTARG" ;;
    *) echo "用法: $0 -h <host> -P <port> -u <user> -p <password>" >&2; exit 2 ;;
  esac
done

mysql_cli() { mysql -h "$HOST" -P "$PORT" -u "$USER" --batch --raw "$@"; }

echo "===== 1. 复制状态 ====="
mysql_cli -e "SHOW SLAVE STATUS\G" | grep -E \
  "Master_Host|Slave_IO_Running|Slave_SQL_Running|Seconds_Behind_Master|Last_SQL_Error|Retrieved_Gtid_Set|Executed_Gtid_Set" || true

echo
echo "===== 2. 回放线程并发度 ====="
mysql_cli -e "SHOW VARIABLES LIKE 'slave_parallel_workers';"

echo
echo "===== 3. 未提交的大事务 ====="
mysql_cli -e "
SELECT trx_id, trx_state, trx_started, trx_rows_modified>
  FROM information_schema.innodb_trx
 WHERE trx_rows_modified > 10000
 ORDER BY trx_rows_modified DESC
 LIMIT 10;"

echo
echo "===== 4. 运行超过 5 秒的查询 ====="
mysql_cli -e "
SELECT id, user, time, state, LEFT(info, 80) AS query
  FROM information_schema.processlist
 WHERE command <> 'Sleep' AND time > 5
 ORDER BY time DESC
 LIMIT 10;"

echo
echo "诊断完成。判读方式见资源页的使用说明。"
`,
		Version:        "1.2.0",
		CompatibleWith: "MySQL 5.7 / 8.0，Linux",
		Tags:           []string{"MySQL", "主从复制", "诊断脚本"},
		CategorySlug:   "database",
		DownloadCount:  1840,
		ViewCount:      5210,
	},
	{
		Title:   "Nginx 生产级基础配置模板",
		Slug:    "nginx-production-baseline",
		Summary: "包含 worker 调优、日志切割、gzip、限流与安全响应头的可上线基线配置。",
		Content: "## 这份模板解决什么\n\n从零写 nginx.conf 最容易漏掉的是「不会立刻出问题、" +
			"但早晚会出问题」的那几项：日志不切割、缺少安全响应头、限流缺失。\n\n" +
			"## 上线前需要改的地方\n\n- `worker_processes`：与 CPU 核数一致即可，不要盲目调大。\n" +
			"- `client_max_body_size`：按业务最大上传体积设置，默认 1m 会挡住大部分文件上传。\n" +
			"- `limit_req_zone` 的 `rate`：需要结合实际 QPS 调整，留出 2 倍余量。\n\n" +
			"## 为什么默认关闭 server_tokens\n\n版本号暴露会让人可以针对特定版本的已知漏洞直接尝试利用，" +
			"没有任何业务价值。",
		FileName: "nginx-production.conf",
		FileContent: `# Nginx 生产级基础配置
# 上线前请按文件顶部注释逐项确认。

worker_processes auto;
worker_rlimit_nofile 65535;

events {
    worker_connections 10240;
    multi_accept on;
    use epoll;
}

http {
    # ---- 基础 ----
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    types_hash_max_size 2048;

    # 不暴露版本号，避免被针对特定版本的漏洞利用
    server_tokens off;

    keepalive_timeout 65;
    client_header_timeout 15;
    client_body_timeout 15;
    send_timeout 15;

    # 按业务最大上传体积调整，默认 1m 会挡住大部分文件上传
    client_max_body_size 20m;

    # ---- 日志 ----
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" rt=$request_time ut=$upstream_response_time';

    access_log /var/log/nginx/access.log main buffer=32k flush=5s;
    error_log  /var/log/nginx/error.log warn;

    # ---- 压缩 ----
    gzip on;
    gzip_vary on;
    gzip_min_length 1024;
    gzip_comp_level 5;
    gzip_types text/plain text/css application/json application/javascript
               application/xml text/xml image/svg+xml;

    # ---- 限流 ----
    # 按客户端 IP 限流；rate 需结合实际 QPS 调整，建议留 2 倍余量
    limit_req_zone $binary_remote_addr zone=perip:10m rate=20r/s;
    limit_conn_zone $binary_remote_addr zone=conn_perip:10m;

    # ---- 安全响应头 ----
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    include /etc/nginx/conf.d/*.conf;
}
`,
		Version:        "2.0.1",
		CompatibleWith: "Nginx 1.20+，CentOS 7+/Ubuntu 20.04+",
		Tags:           []string{"Nginx", "配置模板", "安全加固"},
		CategorySlug:   "linux-network",
		NeedLogin:      true,
		DownloadCount:  2630,
		ViewCount:      7410,
	},
	{
		Title:   "Linux 系统巡检 Shell 脚本集",
		Slug:    "linux-health-check-scripts",
		Summary: "CPU / 内存 / 磁盘 / 网络 / 关键进程五项巡检，输出可直接用于日报。",
		Content: "## 设计原则\n\n巡检脚本最容易犯的错是「输出一大堆原始命令结果」，" +
			"值班的人还得自己判断哪项异常。这套脚本对每项检查都给出明确的阈值判定与结论。\n\n" +
			"## 用法\n\n```\nbash linux-health-check.sh          # 全量巡检\nbash linux-health-check.sh --brief  # 只输出异常项，适合日报\n```\n\n" +
			"## 阈值说明\n\n磁盘 85%、内存 90%、load 高于核数 2 倍会被判为异常。" +
			"这些阈值应结合业务实际调整——比如日志盘的告警线通常应该更低。",
		FileName: "linux-health-check.sh",
		FileContent: `#!/usr/bin/env bash
#
# Linux 系统巡检
#   bash linux-health-check.sh          全量巡检
#   bash linux-health-check.sh --brief  只输出异常项
#
set -uo pipefail

BRIEF=0
[[ "${1:-}" == "--brief" ]] && BRIEF=1

DISK_WARN=85
MEM_WARN=90
ABNORMAL=0

ok()   { [[ $BRIEF -eq 1 ]] || echo "  [OK]   $*"; }
warn() { ABNORMAL=$((ABNORMAL+1)); echo "  [WARN] $*"; }

echo "========== 系统巡检 $(date '+%F %T') =========="

echo "-- CPU 负载 --"
CORES=$(nproc)
LOAD1=$(awk '{print $1}' /proc/loadavg)
LIMIT=$(awk -v c="$CORES" 'BEGIN{printf "%.2f", c*2}')
awk -v l="$LOAD1" -v t="$LIMIT" 'BEGIN{exit !(l>t)}' \
  && warn "1 分钟负载 $LOAD1 超过核数 2 倍（$CORES 核，阈值 $LIMIT）" \
  || ok "1 分钟负载 $LOAD1（$CORES 核）"

echo "-- 内存 --"
MEM_USED_PCT=$(free | awk '/^Mem:/ {printf "%.0f", $3/$2*100}')
[[ "$MEM_USED_PCT" -ge "$MEM_WARN" ]] \
  && warn "内存使用率 ${MEM_USED_PCT}%（阈值 ${MEM_WARN}%）" \
  || ok "内存使用率 ${MEM_USED_PCT}%"

echo "-- 磁盘 --"
while read -r fs size used avail pct mnt; do
  p="${pct%\%}"
  case "$fs" in
    tmpfs|devtmpfs|overlay) continue ;;
  esac
  [[ "$p" -ge "$DISK_WARN" ]] \
    && warn "挂载点 $mnt 使用率 ${p}%（可用 ${avail}）" \
    || ok "挂载点 $mnt 使用率 ${p}%"
done < <(df -hP | tail -n +2)

echo "-- 网络连接 --"
ESTAB=$(ss -s 2>/dev/null | awk '/^TCP:/ {print $4}' | tr -d ',')
TIME_WAIT=$(ss -tan state time-wait 2>/dev/null | wc -l)
ok "ESTAB=${ESTAB:-0}  TIME_WAIT=${TIME_WAIT}"

echo "-- 关键进程 --"
for proc in sshd crond; do
  pgrep -x "$proc" >/dev/null && ok "$proc 运行中" || warn "$proc 未运行"
done

echo "=============================================="
if [[ $ABNORMAL -gt 0 ]]; then
  echo "巡检结论：发现 $ABNORMAL 项异常，请及时处理。"
  exit 1
fi
echo "巡检结论：全部正常。"
`,
		Version:        "1.4.0",
		CompatibleWith: "CentOS 7+ / Ubuntu 18.04+ / Kylin",
		Tags:           []string{"Shell", "系统巡检", "运维脚本"},
		CategorySlug:   "automation",
		DownloadCount:  3120,
		ViewCount:      8960,
	},
	{
		// 只给链接的资源：不托管安装包，只把「去哪下、下哪个」讲清楚。
		// 这类内容的价值在入口的准确性（官方 + 国内镜像 + 校验方式），
		// 而不在于把 200MB 的二进制塞进自己的存储。
		Title:   "Docker Engine 安装包与国内镜像入口",
		Slug:    "docker-engine-download",
		Summary: "官方安装指引、镜像加速与镜像仓库三个入口，附各发行版的选型建议。",
		Content: "## 该用哪种安装方式\n\n" +
			"- **生产**：走发行版仓库（官方 script 或 yum/apt 源）。升级与安全补丁能跟着包管理器走。\n" +
			"- **离线环境**：下静态二进制，注意同时准备 `containerd.io` 与 `runc`。\n" +
			"- **不要**在生产用 `curl | sh` 一把梭：它装的是最新版，且不受包管理器管理。\n\n" +
			"## 拉不动镜像怎么办\n\n" +
			"先配镜像加速（见下方链接），再考虑改 `daemon.json` 的 `registry-mirrors`。" +
			"注意加速器只代理 Docker Hub，自建仓库不生效。\n\n" +
			"## 上线前检查\n\n" +
			"`docker info` 确认 `Storage Driver` 与 `Cgroup Driver`；" +
			"K8s 节点上 cgroup driver 必须与 kubelet 一致，否则会出现节点 NotReady。",
		Version:        "24.0+",
		CompatibleWith: "CentOS 7+ / Ubuntu 20.04+",
		Tags:           []string{"Docker", "容器", "下载入口"},
		CategorySlug:   "cloud-native",
		ViewCount:      6420,
	},
	{
		Title:   "OpenJDK 21 各发行版下载汇总",
		Slug:    "openjdk-21-download",
		Summary: "Temurin、Corretto 与国内镜像的下载入口，附生产选型与校验建议。",
		Content: "## 选哪个发行版\n\n" +
			"三家的字节码行为一致，差别在**更新节奏与支持周期**：\n\n" +
			"- **Temurin（Adoptium）**：社区主流，更新最及时，适合大多数场景。\n" +
			"- **Corretto**：AWS 长期免费提供安全更新，版本节奏稳。\n" +
			"- **Oracle JDK**：生产使用需注意许可条款，不是「免费下载」就能免费商用。\n\n" +
			"## 下载后必须做的一步\n\n" +
			"核对 SHA256 再解压上线。内网环境尤其重要——中间环节多一次转发就多一次被替换的机会。\n\n" +
			"## 别忘了 JVM 参数\n\n" +
			"容器里跑要加 `-XX:MaxRAMPercentage` 而不是固定 `-Xmx`，" +
			"否则容器内存限制调整后堆还是老值，OOMKilled 会反复出现。",
		Version:        "21 LTS",
		CompatibleWith: "Linux x86_64 / aarch64",
		Tags:           []string{"JDK", "Java", "下载入口"},
		CategorySlug:   "middleware",
		ViewCount:      4180,
	},
}

func seedResources(ctx context.Context, db *gorm.DB, store storage.Storage) error {
	adminID := findAdminID(ctx, db)

	policy, ok := storage.LookupPolicy(storage.SceneResourceFile)
	if !ok {
		return errors.New("资源文件上传场景未注册，无法写入示例资源")
	}

	for _, s := range resourceSeeds {
		var existing model.Resource
		err := db.WithContext(ctx).Where("slug = ?", s.Slug).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("查询种子资源 %s 失败: %w", s.Slug, err)
		}

		now := time.Now()
		item := model.Resource{
			Title:          s.Title,
			Slug:           s.Slug,
			Summary:        s.Summary,
			Content:        s.Content,
			Version:        s.Version,
			CompatibleWith: s.CompatibleWith,
			CategoryID:     categoryIDBySlug(ctx, db, s.CategorySlug),
			Tags:           model.StringList(s.Tags),
			NeedLogin:      s.NeedLogin,
			Status:         model.ResourceStatusPublished,
			AuthorID:       adminID,
			PublishedAt:    &now,
			DownloadCount:  s.DownloadCount,
			ViewCount:      s.ViewCount,
		}

		// FileContent 为空的条目是「只提供外链」的资源：不再写文件，
		// 用户点的是官网页或镜像地址。安装包动辄上百 MB 且版本更新频繁，
		// 这类东西自己托管不划算，给对入口才是正确的做法。
		if s.FileContent != "" {
			content := []byte(s.FileContent)
			// 脚本与配置都是纯文本，按 text/plain 走统一场景校验。
			key := policy.BuildKey("text/plain", idgen.NumericCode(16))
			if err := store.Save(ctx, key, bytes.NewReader(content), int64(len(content))); err != nil {
				// 存储不可用时跳过示例资源而不是让整个服务起不来：
				// 生产环境用的是对象存储，本地种子文件写不进去很正常。
				logger.Warn("写入示例资源文件失败，跳过该条", "slug", s.Slug, "err", err.Error())
				continue
			}
			sum := sha256.Sum256(content)
			item.FileKey = key
			item.FileName = s.FileName
			item.FileSize = int64(len(content))
			item.FileExt = strings.ToLower(strings.TrimPrefix(fileExt(s.FileName), "."))
			item.Checksum = hex.EncodeToString(sum[:])
		}

		if err := db.WithContext(ctx).Create(&item).Error; err != nil {
			return fmt.Errorf("写入种子资源 %s 失败: %w", s.Slug, err)
		}
	}
	return nil
}

// fileExt 取文件名扩展名（含点）。
func fileExt(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i:]
	}
	return ""
}

// findAdminID 取一个管理员账号 ID 作为种子内容的作者，取不到时返回 0。
func findAdminID(ctx context.Context, db *gorm.DB) int64 {
	var id int64
	err := db.WithContext(ctx).Model(&model.User{}).
		Where("role = ?", model.RoleAdmin).Order("id ASC").
		Limit(1).Pluck("id", &id).Error
	if err != nil {
		return 0
	}
	return id
}

// categoryIDBySlug 按 slug 查分类 ID，查不到返回 0（不指定分类）。
func categoryIDBySlug(ctx context.Context, db *gorm.DB, slug string) int64 {
	if strings.TrimSpace(slug) == "" {
		return 0
	}
	var id int64
	err := db.WithContext(ctx).Model(&model.ArticleCategory{}).
		Where("slug = ?", slug).Limit(1).Pluck("id", &id).Error
	if err != nil {
		return 0
	}
	return id
}
