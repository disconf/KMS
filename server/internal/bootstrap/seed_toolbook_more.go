package bootstrap

import "github.com/opsexpert/platform/internal/model"

// 本文件补齐其余工具的教程集（seed_toolbook.go 里只有 openresty 一本）。
//
// 组织原则与那本一致：六章固定结构——软件介绍 / 多种部署方式 / 配置参数详解 /
// 生产实战部署 / 常用命令速查 / 官方文档导读，每章一节，正文就是 <tool> 的
// 六篇站内文章（见 seed_tool_articles.go 与 seed_tool_articles_extra.go），
// 节导语自动取文章摘要，不再挂官网外链。
//
// 书名保持与外链版一致：幂等键是「工具 + 书名」，改名会让老库长出第二本。

var extraToolBookSeeds = []toolBookSeed{
	{
		ToolSlug: "prometheus",
		Title:    "Prometheus 从采集到告警",
		Summary:  "先把单机跑通，再谈规则与长期存储：这条路上的坑基本都在「指标怎么命名」和「告警怎么写」两处。",
		Level:    model.ToolBookLevelBeginner,
		Sort:     20,
		Chapters: toolBookChapters("prometheus", "Prometheus"),
	},
	{
		ToolSlug: "grafana",
		Title:    "Grafana 看板与告警落地",
		Summary:  "从接数据源到做出「出事时第一个该看的看板」，重点在变量、阈值与告警路由。",
		Level:    model.ToolBookLevelBeginner,
		Sort:     30,
		Chapters: toolBookChapters("grafana", "Grafana"),
	},
	{
		ToolSlug: "argo-cd",
		Title:    "Argo CD 与 GitOps 上线流程",
		Summary:  "把「集群状态等于 Git 状态」这件事落地，含同步策略、健康检查与回滚路径。",
		Level:    model.ToolBookLevelAdvanced,
		Sort:     40,
		Chapters: toolBookChapters("argo-cd", "Argo CD"),
	},
	{
		ToolSlug: "ansible",
		Title:    "Ansible 从单条命令到可复用 role",
		Summary:  "按「先跑通 → 再抽象成 role → 最后管住密钥」的顺序学，能少走很多重复路。",
		Level:    model.ToolBookLevelBeginner,
		Sort:     50,
		Chapters: toolBookChapters("ansible", "Ansible"),
	},
	{
		ToolSlug: "docker",
		Title:    "Docker 从装到生产可用",
		Summary:  "安装、镜像构建、日志与网络、安全加固四段，都是宿主机上真会踩的点。",
		Level:    model.ToolBookLevelBeginner,
		Sort:     60,
		Chapters: toolBookChapters("docker", "Docker"),
	},
	{
		ToolSlug: "kubernetes",
		Title:    "Kubernetes 运维主线",
		Summary:  "从 kubeadm 建集群到工作负载、探针、升级与安全基线：一条能直接照着做的主线。",
		Level:    model.ToolBookLevelAdvanced,
		Sort:     70,
		Chapters: toolBookChapters("kubernetes", "Kubernetes"),
	},
	{
		ToolSlug: "redis",
		Title:    "Redis 运维与调优",
		Summary:  "配置、持久化、主从与内存治理四块，决定了 Redis 是缓存还是事故源。",
		Level:    model.ToolBookLevelBeginner,
		Sort:     80,
		Chapters: toolBookChapters("redis", "Redis"),
	},
	{
		ToolSlug: "nginx",
		Title:    "Nginx 配置与调优",
		Summary:  "从初次配置到 upstream、TLS 与平滑重载：入口层该掌握的都在这。",
		Level:    model.ToolBookLevelBeginner,
		Sort:     90,
		Chapters: toolBookChapters("nginx", "Nginx"),
	},
	{
		ToolSlug: "git",
		Title:    "Git 运维实用手册",
		Summary:  "不追求全量掌握，只把「回滚、追责、协作」三类高频操作练熟。",
		Level:    model.ToolBookLevelBeginner,
		Sort:     100,
		Chapters: toolBookChapters("git", "Git"),
	},
	{
		ToolSlug: "jenkins",
		Title:    "Jenkins 流水线工程化",
		Summary:  "以 Jenkinsfile 为中心：安装、语法、凭据、主从与维护。",
		Level:    model.ToolBookLevelAdvanced,
		Sort:     110,
		Chapters: toolBookChapters("jenkins", "Jenkins"),
	},
	{
		ToolSlug: "elasticsearch",
		Title:    "Elasticsearch 集群运维",
		Summary:  "安装、关键参数、分片规划与快照恢复：四个决定集群能不能长期活着的点。",
		Level:    model.ToolBookLevelAdvanced,
		Sort:     120,
		Chapters: toolBookChapters("elasticsearch", "Elasticsearch"),
	},
	{
		ToolSlug: "terraform",
		Title:    "Terraform 基础设施即代码",
		Summary:  "语言、状态、模块、流程四步走：把云资源变成可评审、可回滚的代码。",
		Level:    model.ToolBookLevelAdvanced,
		Sort:     130,
		Chapters: toolBookChapters("terraform", "Terraform"),
	},
	{
		ToolSlug: "mysql",
		Title:    "MySQL 运维与性能",
		Summary:  "从安装、配置、慢查询到复制与备份：DBA 日常的四条主线。",
		Level:    model.ToolBookLevelAdvanced,
		Sort:     140,
		Chapters: toolBookChapters("mysql", "MySQL"),
	},
	{
		ToolSlug: "loki",
		Title:    "Loki 日志系统落地",
		Summary:  "标签设计、采集、查询与存储：Loki 的成本优势全建立在「标签要克制」上。",
		Level:    model.ToolBookLevelAdvanced,
		Sort:     150,
		Chapters: toolBookChapters("loki", "Loki"),
	},
}
