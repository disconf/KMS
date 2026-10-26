package bootstrap

// 本文件补齐「常用开源工具」的推荐条目。
//
// 与 seed_kb.go 里的 toolSeeds 是同一张表的两批数据，分开写只是为了不让
// 那个文件无限变长：先有的 5 个是内容最重的（prometheus/grafana/argocd/
// ansible/openresty），这里补的是运维日常一定会碰到的 10 个。
//
// 每个工具都在 seed_toolbook_more.go 里配一本按章节组织的教程集，
// 章节挂官方文档外链——工具推荐负责「这个工具是什么」，
// 教程集负责「按什么顺序学」，两者不是重复内容。

var extraToolSeeds = []toolSeed{
	{
		Name:    "Docker",
		Slug:    "docker",
		Summary: "容器运行时与镜像工具链的事实标准，运维容器化的起点。",
		Content: "## 适用场景\n\n单机容器编排、镜像构建与分发、CI 内的构建环境隔离。" +
			"K8s 之上它不再是运行时主角（那边是 containerd/CRI-O），但构建与本地调试仍然离不开它。\n\n" +
			"## 最容易踩的坑\n\n" +
			"- **latest 标签**：`docker pull` 拿到的内容会变，生产必须用 digest 或不可变 tag。\n" +
			"- **层叠删除不省空间**：一个 RUN 里装了又删，前面那层照样占体积；安装与清理要在同一条 RUN。\n" +
			"- **默认 bridge 网络没有 DNS**：容器间互访要么用自定义网络，要么用服务名发现。\n" +
			"- **日志驱动默认 json-file 无上限**：不配 `max-size` 会把磁盘写满，这是最常见的宿主机事故。",
		RepoURL:      "https://github.com/moby/moby",
		HomepageURL:  "https://docs.docker.com",
		License:      "Apache-2.0",
		Language:     "Go",
		Stars:        69500,
		ForkCount:    18600,
		DeployModes:  []string{"binary", "docker", "package"},
		Tags:         []string{"容器", "镜像", "CI"},
		CategorySlug: "cloud-native",
		Featured:     true,
		Sort:         200,
		ViewCount:    5320,
	},
	{
		Name:    "Kubernetes",
		Slug:    "kubernetes",
		Summary: "容器编排的事实标准，声明式 API + 控制器循环是它的一切设计出发点。",
		Content: "## 适用场景\n\n多节点容器调度、服务发现、滚动发布与自愈。" +
			"它的价值不在「能跑容器」，而在于把「期望状态」变成可审计、可回滚的声明。\n\n" +
			"## 运维要知道的三件事\n\n" +
			"- **绝大多数「K8s 问题」是应用配置问题**：探针、resources、securityContext 写错，" +
			"表现却是 Pod 起不来。先看 `kubectl describe` 的 Events。\n" +
			"- **不要用 `kubectl edit` 改生产**：改动会被下一次同步覆盖，且没有记录。一切走 Git。\n" +
			"- **升级是集群级变更**：kubelet、控制面、API 版本弃用要逐个小版本推进，跳版本升级必翻车。",
		RepoURL:      "https://github.com/kubernetes/kubernetes",
		HomepageURL:  "https://kubernetes.io/docs/home/",
		License:      "Apache-2.0",
		Language:     "Go",
		Stars:        111000,
		ForkCount:    39600,
		DeployModes:  []string{"binary", "k8s-operator", "package"},
		Tags:         []string{"容器编排", "云原生", "声明式"},
		CategorySlug: "cloud-native",
		Featured:     true,
		Sort:         210,
		ViewCount:    6180,
	},
	{
		Name:    "Redis",
		Slug:    "redis",
		Summary: "内存数据结构服务，缓存、排行榜、分布式锁、限流都能用它，但每种的正确用法不同。",
		Content: "## 适用场景\n\n缓存与热点数据、计数与排行榜（zset）、简单的分布式锁（SET NX PX）、" +
			"限流（令牌桶用 Lua 保证原子）。\n\n" +
			"## 落地要点\n\n" +
			"- **内存是有上限的**：必须配 `maxmemory` 与 `maxmemory-policy`，" +
			"不配的话 OOM 时整个实例被内核杀掉，而不是淘汰 key。\n" +
			"- **缓存穿透/雪崩**：空结果要缓存、过期时间要加随机抖动，别让一批 key 同时失效。\n" +
			"- **持久化不是备份**：RDB/AOF 只解决重启恢复，误删与逻辑损坏仍然要靠备份。\n" +
			"- **禁止 KEYS 与 FLUSHALL 上生产**：前者会阻塞主线程，后者没有回收站。" +
			"扫键用 SCAN，删除用 UNLINK。",
		RepoURL:      "https://github.com/redis/redis",
		HomepageURL:  "https://redis.io/docs/latest/",
		License:      "BSD-3-Clause（7.4 起另有 RSALv2/SSPLv1 双许可）",
		Language:     "C",
		Stars:        68000,
		ForkCount:    23900,
		DeployModes:  []string{"binary", "docker", "helm", "package"},
		Tags:         []string{"缓存", "KV", "分布式锁"},
		CategorySlug: "database",
		Featured:     true,
		Sort:         220,
		ViewCount:    4760,
	},
	{
		Name:    "Nginx",
		Slug:    "nginx",
		Summary: "反向代理与静态资源服务的默认选择，配置简单、生态成熟、可观测手段齐全。",
		Content: "## 与 OpenResty 的关系\n\n同源：OpenResty = Nginx + LuaJIT + 一堆模块。" +
			"纯转发、TLS 终止、静态文件用官方 Nginx 就够；需要在网关层写逻辑再上 OpenResty。\n\n" +
			"## 常见误区\n\n" +
			"- **`worker_connections` 不是并发上限**：真实并发还受 `worker_processes`、" +
			"upstream keepalive 与后端连接数限制。\n" +
			"- **改完配置不 `nginx -t`**：语法错误会导致 reload 失败但旧进程还在，" +
			"表现为「改了没生效」，很难第一时间想到。\n" +
			"- **默认 `proxy_buffering on`**：对流式响应（SSE、大文件下载）要关掉，否则会攒包。",
		RepoURL:      "https://github.com/nginx/nginx",
		HomepageURL:  "https://nginx.org/en/docs/",
		License:      "BSD-2-Clause",
		Language:     "C",
		Stars:        26000,
		ForkCount:    6900,
		DeployModes:  []string{"binary", "docker", "source", "package"},
		Tags:         []string{"反向代理", "负载均衡", "静态资源"},
		CategorySlug: "network",
		Featured:     false,
		Sort:         230,
		ViewCount:    3980,
	},
	{
		Name:    "Git",
		Slug:    "git",
		Summary: "分布式版本控制，运维用它管配置与脚本，是「可回滚」最便宜的实现方式。",
		Content: "## 为什么运维也要用好 Git\n\n配置与脚本进仓库之后，变更才有 diff、有作者、有回滚点。" +
			"「谁在什么时候改了哪一行」这个问题，只有版本控制能回答。\n\n" +
			"## 常用但不常被讲清的几件事\n\n" +
			"- **`reset` 与 `revert`**：前者重写历史（只用于本地未推送），后者追加反向提交（公共分支唯一正确做法）。\n" +
			"- **`rebase` 的价值是保持线性**：提交历史能直接读出变更顺序，排障时不用在合并网里绕。\n" +
			"- **`reflog` 是后悔药**：误 reset 之后先看它，大多数「丢了的提交」都能找回来。\n" +
			"- **大文件不要进 Git**：一旦进了历史，删提交也删不掉，仓库会永久变慢。",
		RepoURL:      "https://github.com/git/git",
		HomepageURL:  "https://git-scm.com/doc",
		License:      "GPL-2.0",
		Language:     "C",
		Stars:        53000,
		ForkCount:    25800,
		DeployModes:  []string{"binary", "package", "source"},
		Tags:         []string{"版本控制", "协作", "配置管理"},
		CategorySlug: "automation",
		Featured:     false,
		Sort:         240,
		ViewCount:    3120,
	},
	{
		Name:    "Jenkins",
		Slug:    "jenkins",
		Summary: "老牌持续集成服务器，插件生态最全，适合异构环境与存量流水线改造。",
		Content: "## 适用场景\n\n需要在同一套流水线里跑多种系统（Windows 构建、老版本 JDK）、" +
			"或者已有大量存量 Job 需要渐进式改造。新项目若全在 K8s 上，GitLab CI / GitHub Actions 通常更省心。\n\n" +
			"## 运维要点\n\n" +
			"- **Jenkinsfile 一定要进仓库**：在 Web 界面里写脚本等于把流水线变成不可审计的黑盒。\n" +
			"- **凭据只走 credentials 绑定**：任何 `echo $PASSWORD` 都会永久留在构建日志里。\n" +
			"- **磁盘会被构建产物吃掉**：配置丢弃旧构建策略，产物转制品库。\n" +
			"- **主从架构是必须的**：让构建跑在 agent 上，controller 只负责调度，否则一次重构建就能拖垮它。",
		RepoURL:      "https://github.com/jenkinsci/jenkins",
		HomepageURL:  "https://www.jenkins.io/doc/",
		License:      "MIT",
		Language:     "Java",
		Stars:        23500,
		ForkCount:    8900,
		DeployModes:  []string{"docker", "helm", "package", "binary"},
		Tags:         []string{"CI/CD", "流水线", "自动化"},
		CategorySlug: "automation",
		Featured:     false,
		Sort:         250,
		ViewCount:    2740,
	},
	{
		Name:    "Elasticsearch",
		Slug:    "elasticsearch",
		Summary: "分布式搜索与分析引擎，日志检索、全文搜索、聚合分析的常用底座。",
		Content: "## 适用场景\n\n日志检索（配 Filebeat/Logstash）、站内全文搜索、指标聚合分析。" +
			"注意它不擅长「事务型查询」与「频繁更新」——那是数据库的活。\n\n" +
			"## 运维要点\n\n" +
			"- **默认配置不能直接上生产**：单节点、无认证、堆内存未设上限，这三件事凑一起就是事故。\n" +
			"- **堆内存不超过物理内存一半、且不超过 32G**：超过 32G 会失去指针压缩，性能反而下降。\n" +
			"- **分片不是越多越好**：单分片建议 10–50GB，分片过多会让集群状态维护成本暴涨。\n" +
			"- **许可证要看清**：7.11 起部分功能走 Elastic License / SSPL，商用前必须过一遍法务。",
		RepoURL:      "https://github.com/elastic/elasticsearch",
		HomepageURL:  "https://www.elastic.co/guide/index.html",
		License:      "Elastic License 2.0 / SSPL / AGPL（版本不同）",
		Language:     "Java",
		Stars:        71000,
		ForkCount:    25100,
		DeployModes:  []string{"docker", "helm", "binary", "k8s-operator"},
		Tags:         []string{"搜索", "日志", "ELK"},
		CategorySlug: "observability",
		Featured:     false,
		Sort:         260,
		ViewCount:    3560,
	},
	{
		Name:    "Terraform",
		Slug:    "terraform",
		Summary: "基础设施即代码的事实标准，用声明式 HCL 管理云资源与网络。",
		Content: "## 适用场景\n\n跨云/跨厂商的资源编排、可复现的环境搭建、需要审批与审计的基础设施变更。\n\n" +
			"## 三条硬规矩\n\n" +
			"- **状态文件必须远端存储并加锁**：本地 state 多人用必然互相覆盖，等于基础设施账本被涂抹。\n" +
			"- **apply 前必须人工读 plan**：`forces replacement` 那一行往往意味着重建数据库。\n" +
			"- **禁止在控制台直接改资源**：漂移一旦产生，下一次 plan 就是一堆看不懂的 diff。\n\n" +
			"## 许可证提醒\n\n1.6 起改为 BSL 1.1（非 OSI 开源），配套工具链可考虑 OpenTofu（MPL-2.0 分叉）。",
		RepoURL:      "https://github.com/hashicorp/terraform",
		HomepageURL:  "https://developer.hashicorp.com/terraform/docs",
		License:      "BSL 1.1（1.6 起）",
		Language:     "Go",
		Stars:        43000,
		ForkCount:    9600,
		DeployModes:  []string{"binary", "docker", "ci-plugin"},
		Tags:         []string{"IaC", "云资源", "声明式"},
		CategorySlug: "automation",
		Featured:     false,
		Sort:         270,
		ViewCount:    2890,
	},
	{
		Name:    "MySQL",
		Slug:    "mysql",
		Summary: "最广泛使用的关系型数据库，主从复制与 InnoDB 调优是运维的基本功。",
		Content: "## 适用场景\n\n业务主库、需要强事务与成熟生态的关系型存储。" +
			"分库分表之前先想清楚：多数业务的问题不是数据量大，而是索引与 SQL 写错了。\n\n" +
			"## 运维要点\n\n" +
			"- **慢查询日志默认是关的**：`slow_query_log` + `long_query_time` 是性能治理的起点。\n" +
			"- **主从延迟的常见原因**：大事务、从库单线程回放、从库上有长查询、磁盘 IO 打满。\n" +
			"- **备份要能恢复才算备份**：逻辑备份（mysqldump）与物理备份（XtraBackup）各有场景，" +
			"恢复演练必须做。\n" +
			"- **改表用在线工具**：大表直接 ALTER 会锁表，用 gh-ost / pt-online-schema-change。",
		RepoURL:      "https://github.com/mysql/mysql-server",
		HomepageURL:  "https://dev.mysql.com/doc/",
		License:      "GPL-2.0",
		Language:     "C++",
		Stars:        11200,
		ForkCount:    4300,
		DeployModes:  []string{"binary", "docker", "helm", "package"},
		Tags:         []string{"关系型数据库", "主从复制", "事务"},
		CategorySlug: "database",
		Featured:     false,
		Sort:         280,
		ViewCount:    4210,
	},
	{
		Name:    "Loki",
		Slug:    "loki",
		Summary: "只索引标签、不索引正文的日志系统，成本比全文索引方案低一个量级。",
		Content: "## 适用场景\n\nK8s 环境日志聚合，尤其是「量很大、查得不频繁」的场景。" +
			"它刻意不做全文索引：查询靠标签筛选 + 正则过滤，因此存储成本远低于 ES。\n\n" +
			"## 落地要点\n\n" +
			"- **标签基数决定成败**：把 `request_id`、`user_id` 当标签会让索引爆炸。" +
			"标签只放「维度」（服务名、环境、命名空间），其余进日志正文。\n" +
			"- **查询范围要收敛**：不限时间范围的查询会扫大量 chunk，慢到超时。\n" +
			"- **采集侧限流**：promtail/agent 要配限速，否则日志风暴会先把 Loki 打挂。",
		RepoURL:      "https://github.com/grafana/loki",
		HomepageURL:  "https://grafana.com/docs/loki/latest/",
		License:      "AGPL-3.0",
		Language:     "Go",
		Stars:        23500,
		ForkCount:    3400,
		DeployModes:  []string{"binary", "docker", "helm", "k8s-operator"},
		Tags:         []string{"日志", "可观测性", "轻量"},
		CategorySlug: "observability",
		Featured:     false,
		Sort:         290,
		ViewCount:    2180,
	},
}
