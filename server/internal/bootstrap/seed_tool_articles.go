package bootstrap

// 本文件把「工具教程集的站内正文」接进文章种子。
//
// 背景：教程集最初每一节都挂官网外链，读者点进去就离开本站。
// 现在改为每本教程集的正文都写在本站：每个工具三篇文章——
//
//	<tool>-install    多种部署方式（发行版仓库 / 二进制 / 容器 / 编排 / 离线）
//	<tool>-production 生产实战部署（参数、集群、验收）
//	<tool>-commands   常用命令速查（日常操作 + 排障）
//
// 正文是 Markdown，放在 data/articles/<slug>.md，go:embed 进二进制。
// 标题、摘要、分类、标签在这里的元数据表里维护——元数据走 Go 常量便于编译期检查，
// 正文走独立文件避免转义地狱（Markdown 里的代码块和反引号没法塞进字符串字面量）。
//
// 幂等与普通文章种子一致：按 slug 判存在，已存在就跳过。

import (
	"embed"
	"fmt"
)

//go:embed data/articles/*.md
var toolArticlesFS embed.FS

// toolArticleMeta 是一篇工具教程文章的元数据；正文按 File 从 embed 里读。
type toolArticleMeta struct {
	articleSeed
	File string
}

var toolArticleMetas = []toolArticleMeta{
	// ---------------- OpenResty / Nginx ----------------
	{articleSeed{Title: "OpenResty / Nginx 安装与多种部署方式", Slug: "openresty-install",
		CategorySlug: "middleware",
		Summary:      "发行版仓库、官方源、源码编译、容器化、离线安装五种方式的取舍与完整步骤。",
		Tags:         []string{"Nginx", "OpenResty", "部署"}}, "openresty-install.md"},
	{articleSeed{Title: "OpenResty / Nginx 生产环境实战部署", Slug: "openresty-production",
		CategorySlug: "middleware",
		Summary:      "worker 与连接数、反向代理与 upstream、限流、HTTPS 与高可用，一套能上生产的配置。",
		Tags:         []string{"Nginx", "OpenResty", "生产实践"}}, "openresty-production.md"},
	{articleSeed{Title: "OpenResty / Nginx 常用命令速查", Slug: "openresty-commands",
		CategorySlug: "middleware",
		Summary:      "启停重载、配置校验、日志切割与排障定位，附一张「改了配置没生效」的排查清单。",
		Tags:         []string{"Nginx", "命令行", "排障"}}, "openresty-commands.md"},

	// ---------------- Ansible ----------------
	{articleSeed{Title: "Ansible 安装与多种部署方式", Slug: "ansible-install",
		CategorySlug: "automation",
		Summary:      "pip、发行版包、容器执行镜像三种安装路径，以及控制节点与被管节点的关系。",
		Tags:         []string{"Ansible", "自动化", "部署"}}, "ansible-install.md"},
	{articleSeed{Title: "Ansible 生产环境实战部署", Slug: "ansible-production",
		CategorySlug: "automation",
		Summary:      "inventory 分层、变量覆盖顺序、ansible.cfg 调优与批量操作的安全阀，从能跑到敢跑。",
		Tags:         []string{"Ansible", "Playbook", "生产实践"}}, "ansible-production.md"},
	{articleSeed{Title: "Ansible 常用命令速查", Slug: "ansible-commands",
		CategorySlug: "automation",
		Summary:      "ad-hoc、playbook、playbook 检查与加密，附「改了 inventory 不生效」的排查清单。",
		Tags:         []string{"Ansible", "命令行"}}, "ansible-commands.md"},

	// ---------------- Argo CD ----------------
	{articleSeed{Title: "Argo CD 安装与多种部署方式", Slug: "argo-cd-install",
		CategorySlug: "cloud-native",
		Summary:      "非高可用 / 高可用两套 YAML、CLI 安装、Helm 与 Operator 的取舍。",
		Tags:         []string{"Argo CD", "GitOps", "部署"}}, "argo-cd-install.md"},
	{articleSeed{Title: "Argo CD 生产环境实战部署", Slug: "argo-cd-production",
		CategorySlug: "cloud-native",
		Summary:      "同步策略、RBAC 与 SSO、仓库凭证管理、应用集与灾备，一套可交付的 GitOps 形态。",
		Tags:         []string{"Argo CD", "GitOps", "生产实践"}}, "argo-cd-production.md"},
	{articleSeed{Title: "Argo CD 常用命令速查", Slug: "argo-cd-commands",
		CategorySlug: "cloud-native",
		Summary:      "登录、应用同步、回滚与 diff，附「同步卡在 Progressing」的排查顺序。",
		Tags:         []string{"Argo CD", "命令行"}}, "argo-cd-commands.md"},

	// ---------------- Prometheus ----------------
	{articleSeed{Title: "Prometheus 安装与多种部署方式", Slug: "prometheus-install",
		CategorySlug: "observability",
		Summary:      "二进制、Docker、Helm（kube-prometheus-stack）三种方式的步骤与适用边界。",
		Tags:         []string{"Prometheus", "监控", "部署"}}, "prometheus-install.md"},
	{articleSeed{Title: "Prometheus 生产环境实战部署", Slug: "prometheus-production",
		CategorySlug: "observability",
		Summary:      "保留期与内存估算、联邦与远端存储、告警规则分层与基数治理。",
		Tags:         []string{"Prometheus", "监控", "生产实践"}}, "prometheus-production.md"},
	{articleSeed{Title: "Prometheus 常用命令与 PromQL 速查", Slug: "prometheus-commands",
		CategorySlug: "observability",
		Summary:      "PromQL 常用查询、API 用法、TSDB 运维命令与「查不到数据」的排查清单。",
		Tags:         []string{"Prometheus", "PromQL", "命令行"}}, "prometheus-commands.md"},

	// ---------------- Grafana ----------------
	{articleSeed{Title: "Grafana 安装与多种部署方式", Slug: "grafana-install",
		CategorySlug: "observability",
		Summary:      "yum/apt 仓库、Docker、Helm 三种方式，以及配置文件与环境变量两种配置入口。",
		Tags:         []string{"Grafana", "可视化", "部署"}}, "grafana-install.md"},
	{articleSeed{Title: "Grafana 生产环境实战部署", Slug: "grafana-production",
		CategorySlug: "observability",
		Summary:      "数据库与高可用、数据源与大盘即代码、权限分层与匿名访问治理。",
		Tags:         []string{"Grafana", "可视化", "生产实践"}}, "grafana-production.md"},
	{articleSeed{Title: "Grafana 常用命令速查", Slug: "grafana-commands",
		CategorySlug: "observability",
		Summary:      "CLI 管理命令、配置校验、大盘导入导出与「打不开图」的排查清单。",
		Tags:         []string{"Grafana", "命令行"}}, "grafana-commands.md"},

	// ---------------- Docker ----------------
	{articleSeed{Title: "Docker 安装与多种部署方式", Slug: "docker-install",
		CategorySlug: "cloud-native",
		Summary:      "官方仓库、发行版包、二进制、离线 rpm/deb 四种安装路径，为什么不要用发行版里的 docker.io。",
		Tags:         []string{"Docker", "容器", "部署"}}, "docker-install.md"},
	{articleSeed{Title: "Docker 生产环境实战部署", Slug: "docker-production",
		CategorySlug: "cloud-native",
		Summary:      "daemon.json 全解、日志轮转、数据-root 迁移、镜像加速与安全基线。",
		Tags:         []string{"Docker", "容器", "生产实践"}}, "docker-production.md"},
	{articleSeed{Title: "Docker 常用命令速查", Slug: "docker-commands",
		CategorySlug: "cloud-native",
		Summary:      "镜像、容器、日志、清理四组命令，附「磁盘被 Docker 吃满」的回收顺序。",
		Tags:         []string{"Docker", "命令行"}}, "docker-commands.md"},

	// ---------------- Kubernetes ----------------
	{articleSeed{Title: "Kubernetes 集群安装与多种部署方式", Slug: "kubernetes-install",
		CategorySlug: "cloud-native",
		Summary:      "kubeadm、二进制、K3s、托管集群四种路径的取舍，以及控制面组件一览。",
		Tags:         []string{"Kubernetes", "集群", "部署"}}, "kubernetes-install.md"},
	{articleSeed{Title: "Kubernetes 生产环境实战部署", Slug: "kubernetes-production",
		CategorySlug: "cloud-native",
		Summary:      "高可用控制面、网络与 CNI 选型、etcd 运维、证书轮换与节点扩缩容。",
		Tags:         []string{"Kubernetes", "集群", "生产实践"}}, "kubernetes-production.md"},
	{articleSeed{Title: "Kubernetes 常用命令速查", Slug: "kubernetes-commands",
		CategorySlug: "cloud-native",
		Summary:      "资源查看、Pod 排障、节点与证书运维，附一套从告警到定位的组合拳。",
		Tags:         []string{"Kubernetes", "kubectl", "命令行"}}, "kubernetes-commands.md"},

	// ---------------- Redis ----------------
	{articleSeed{Title: "Redis 安装与多种部署方式", Slug: "redis-install",
		CategorySlug: "middleware",
		Summary:      "编译安装、发行版包、Docker、哨兵与 Cluster 五种形态的适用场景。",
		Tags:         []string{"Redis", "缓存", "部署"}}, "redis-install.md"},
	{articleSeed{Title: "Redis 生产环境实战部署", Slug: "redis-production",
		CategorySlug: "middleware",
		Summary:      "内存与淘汰策略、持久化取舍、主从与哨兵参数、慢查询与热 key 治理。",
		Tags:         []string{"Redis", "缓存", "生产实践"}}, "redis-production.md"},
	{articleSeed{Title: "Redis 常用命令速查", Slug: "redis-commands",
		CategorySlug: "middleware",
		Summary:      "连接、巡检、内存分析、主从与哨兵命令，附「缓存突然变慢」的排查顺序。",
		Tags:         []string{"Redis", "命令行", "排障"}}, "redis-commands.md"},

	// ---------------- Nginx ----------------
	{articleSeed{Title: "Nginx 安装与多种部署方式", Slug: "nginx-install",
		CategorySlug: "middleware",
		Summary:      "发行版 nginx 与官方源 nginx.org 的区别、编译安装、容器化与 OpenResty 的关系。",
		Tags:         []string{"Nginx", "部署"}}, "nginx-install.md"},
	{articleSeed{Title: "Nginx 生产环境实战部署", Slug: "nginx-production",
		CategorySlug: "middleware",
		Summary:      "反向代理标准结构、upstream 健康检查、限流限连、HTTPS 证书轮转。",
		Tags:         []string{"Nginx", "反向代理", "生产实践"}}, "nginx-production.md"},
	{articleSeed{Title: "Nginx 常用命令速查", Slug: "nginx-commands",
		CategorySlug: "middleware",
		Summary:      "启停重载与信号、日志分析、排障命令与「502/504 从哪查起」。",
		Tags:         []string{"Nginx", "命令行", "排障"}}, "nginx-commands.md"},

	// ---------------- Git ----------------
	{articleSeed{Title: "Git 安装与多种部署方式", Slug: "git-install",
		CategorySlug: "automation",
		Summary:      "发行版包、源码编译、自建 Git 服务（Gitea/GitLab）的路径与选择。",
		Tags:         []string{"Git", "版本控制", "部署"}}, "git-install.md"},
	{articleSeed{Title: "Git 生产环境实战：分支模型与大仓库治理", Slug: "git-production",
		CategorySlug: "automation",
		Summary:      "分支策略、钩子与 CI 门禁、大文件与历史瘦身、服务端备份。",
		Tags:         []string{"Git", "协作流程", "生产实践"}}, "git-production.md"},
	{articleSeed{Title: "Git 常用命令速查", Slug: "git-commands",
		CategorySlug: "automation",
		Summary:      "提交与回退、分支与变基、远端与暂存，附「改错了怎么救」对照表。",
		Tags:         []string{"Git", "命令行"}}, "git-commands.md"},

	// ---------------- Jenkins ----------------
	{articleSeed{Title: "Jenkins 安装与多种部署方式", Slug: "jenkins-install",
		CategorySlug: "automation",
		Summary:      "war 包、发行版包、Docker 与 K8s（JCasC）四种方式的取舍与步骤。",
		Tags:         []string{"Jenkins", "CI/CD", "部署"}}, "jenkins-install.md"},
	{articleSeed{Title: "Jenkins 生产环境实战部署", Slug: "jenkins-production",
		CategorySlug: "automation",
		Summary:      "agent 分布式构建、凭据与权限、备份迁移与流水线即代码。",
		Tags:         []string{"Jenkins", "CI/CD", "生产实践"}}, "jenkins-production.md"},
	{articleSeed{Title: "Jenkins 常用命令速查", Slug: "jenkins-commands",
		CategorySlug: "automation",
		Summary:      "CLI 命令、Groovy 控制台、日志定位与「构建队列堆积」的排查。",
		Tags:         []string{"Jenkins", "命令行"}}, "jenkins-commands.md"},

	// ---------------- Elasticsearch ----------------
	{articleSeed{Title: "Elasticsearch 安装与多种部署方式", Slug: "elasticsearch-install",
		CategorySlug: "middleware",
		Summary:      "压缩包、rpm/deb、Docker 三种方式，以及 JVM 与系统参数的前置检查。",
		Tags:         []string{"Elasticsearch", "搜索", "部署"}}, "elasticsearch-install.md"},
	{articleSeed{Title: "Elasticsearch 生产环境实战部署", Slug: "elasticsearch-production",
		CategorySlug: "middleware",
		Summary:      "角色分离与分片规划、索引生命周期、快照备份与安全配置。",
		Tags:         []string{"Elasticsearch", "搜索", "生产实践"}}, "elasticsearch-production.md"},
	{articleSeed{Title: "Elasticsearch 常用命令与 DSL 速查", Slug: "elasticsearch-commands",
		CategorySlug: "middleware",
		Summary:      "_cat 运维 API、集群健康诊断、常用查询 DSL 与「集群变黄变红」的处理。",
		Tags:         []string{"Elasticsearch", "命令行", "排障"}}, "elasticsearch-commands.md"},

	// ---------------- Terraform ----------------
	{articleSeed{Title: "Terraform 安装与多种部署方式", Slug: "terraform-install",
		CategorySlug: "automation",
		Summary:      "二进制与包管理器安装、版本管理器 tfenv、以及 Provider 与后端的初始化。",
		Tags:         []string{"Terraform", "IaC", "部署"}}, "terraform-install.md"},
	{articleSeed{Title: "Terraform 生产环境实战：状态与工作流", Slug: "terraform-production",
		CategorySlug: "automation",
		Summary:      "远端状态与锁、workspace 与目录结构、plan 审批流与漂移治理。",
		Tags:         []string{"Terraform", "IaC", "生产实践"}}, "terraform-production.md"},
	{articleSeed{Title: "Terraform 常用命令速查", Slug: "terraform-commands",
		CategorySlug: "automation",
		Summary:      "init/plan/apply 生命周期、state 操作、import 与「apply 失败后怎么办」。",
		Tags:         []string{"Terraform", "命令行"}}, "terraform-commands.md"},

	// ---------------- MySQL ----------------
	{articleSeed{Title: "MySQL 安装与多种部署方式", Slug: "mysql-install",
		CategorySlug: "database",
		Summary:      "官方 yum/apt 仓库、二进制、Docker 三种方式，与 8.0 安装后的必做初始化。",
		Tags:         []string{"MySQL", "数据库", "部署"}}, "mysql-install.md"},
	{articleSeed{Title: "MySQL 生产环境实战部署", Slug: "mysql-production",
		CategorySlug: "database",
		Summary:      "my.cnf 核心参数、主从复制搭建、备份策略与慢查询治理。",
		Tags:         []string{"MySQL", "数据库", "生产实践"}}, "mysql-production.md"},
	{articleSeed{Title: "MySQL 常用命令速查", Slug: "mysql-commands",
		CategorySlug: "database",
		Summary:      "账号权限、备份恢复、主从与巡检命令，附「CPU 打满先看什么」的排查路径。",
		Tags:         []string{"MySQL", "命令行", "排障"}}, "mysql-commands.md"},

	// ---------------- Loki ----------------
	{articleSeed{Title: "Loki 安装与多种部署方式", Slug: "loki-install",
		CategorySlug: "observability",
		Summary:      "二进制、Docker、Helm（含单体与微服务两种形态）的部署步骤与选择。",
		Tags:         []string{"Loki", "日志", "部署"}}, "loki-install.md"},
	{articleSeed{Title: "Loki 生产环境实战部署", Slug: "loki-production",
		CategorySlug: "observability",
		Summary:      "标签设计与 cardinality、保留与压缩、采集端（Promtail/Alloy）配置实战。",
		Tags:         []string{"Loki", "日志", "生产实践"}}, "loki-production.md"},
	{articleSeed{Title: "Loki 与 LogQL 常用命令速查", Slug: "loki-commands",
		CategorySlug: "observability",
		Summary:      "LogQL 查询语法、API 用法与「日志查不到」的排查清单。",
		Tags:         []string{"Loki", "LogQL", "命令行"}}, "loki-commands.md"},
}

// toolArticleSeeds 把元数据与 embed 的正文拼成普通文章种子。
// 正文文件缺失视为代码错误（编译期就该发现），直接 panic——
// 静默跳过会让教程集挂出一批只有标题的空节，比启动失败更难发现。
func toolArticleSeeds() []articleSeed {
	out := make([]articleSeed, 0, len(toolArticleMetas)+len(extraToolArticleMetas)+len(cmdbToolArticleMetas))
	for _, m := range append(append(append([]toolArticleMeta{}, toolArticleMetas...), extraToolArticleMetas...), cmdbToolArticleMetas...) {
		body, err := toolArticlesFS.ReadFile("data/articles/" + m.File)
		if err != nil {
			panic(fmt.Sprintf("工具教程文章正文缺失: %s: %v", m.File, err))
		}
		a := m.articleSeed
		a.Content = string(body)
		out = append(out, a)
	}
	return out
}
