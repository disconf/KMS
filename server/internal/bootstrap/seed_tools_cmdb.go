package bootstrap

import "github.com/opsexpert/platform/internal/model"

// 本文件是「CMDB 智能运维平台」的种子：平台自研的智能运维中心，以第 16 个工具
// 收录进开源工具推荐，并配齐六篇站内正文 + 教程集 + 四类实战教程关联。
//
// 内容来源是项目仓库的 README、项目简介与部署手册
// （docs/deployment/平台部署手册-单机与Kubernetes.md 等），不是泛泛的百科式介绍。
// 注意：线上环境的账号密码一律不写进任何种子内容，只保留地址与角色说明。

// ---------------- 工具条目 ----------------

var cmdbToolSeeds = []toolSeed{
	{
		Name:    "CMDB 智能运维平台",
		Slug:    "cmdb",
		Summary: "以 CMDB 为唯一资产数据源的智能运维平台：资产、发现、监控、任务、工单、CI/CD、远程运维与 AIOps 一体。",
		Content: "## 适用场景\n\n千台级服务器、多集群、多云环境的资产治理与运维自动化：" +
			"监控目标、任务执行、发布记录都挂在同一份资产数据上，用 Kafka 事件总线联动。\n\n" +
			"## 能力一览\n\n" +
			"- **CMDB**：CI 模型、资产台账、机房机柜 U 位、关系拓扑、CSV 导入、责任治理。\n" +
			"- **Agent 与发现**：Linux/Windows Agent（mTLS）、SSH/SNMP/Redfish、VMware、云平台、K8s 采集。\n" +
			"- **监控**：Prometheus + Alertmanager + Thanos + Loki + Tempo + Grafana，资产变更自动生成监控目标。\n" +
			"- **任务与远程运维**：Ansible Runner 真实执行、WebSSH、录像回放、高危命令审批。\n" +
			"- **AIOps**：RAG 知识问答、LLM 网关；AI 只调白名单工具且全量审计，高风险操作必须人批。\n\n" +
			"## 落地时最容易踩的坑\n\n" +
			"- **资产模型先设计再上量**：「项目—应用—服务—实例—基础设施」的标识体系没定好，" +
			"后面的监控、任务、AI 都接不住，返工成本极高。\n" +
			"- **生产忘了 `CMDB_ENABLE_DEMO_DATA=false`**：演示资产混进真库很难清。\n" +
			"- **Keycloak 地址逐字符对齐**：`KEYCLOAK_ISSUER_URL` 与浏览器登录地址差一个端口就是全站 401。\n" +
			"- **单机重启先解封 Vault**：文件存储默认封存，`unseal-vault.sh` 是重启后的第一件事。",
		RepoURL:      "https://github.com/disconf/CMDB",
		HomepageURL:  "https://github.com/disconf/CMDB",
		License:      "未声明（仓库暂无 LICENSE 文件）",
		Language:     "Go",
		Stars:        1,
		ForkCount:    1,
		DeployModes:  []string{"binary", "docker", "helm"},
		Tags:         []string{"CMDB", "智能运维", "自动化平台"},
		CategorySlug: "automation",
		Featured:     false,
		Sort:         300,
		ViewCount:    860,
	},
}

// ---------------- 教程集站内正文元数据 ----------------

var cmdbToolArticleMetas = []toolArticleMeta{
	{articleSeed{Title: "CMDB 智能运维平台 软件介绍与核心概念", Slug: "cmdb-intro",
		CategorySlug: "automation",
		Summary:      "以 CMDB 为唯一资产数据源的运维自动化平台：架构、九大能力、统一资源模型与事件总线协作。",
		Tags:         []string{"CMDB", "智能运维", "架构"}}, "cmdb-intro.md"},
	{articleSeed{Title: "CMDB 智能运维平台 安装与多种部署方式", Slug: "cmdb-install",
		CategorySlug: "automation",
		Summary:      "本地开发、单机 Docker Compose 一键部署、Kubernetes 生产部署三条路径，含资源结论与部署顺序。",
		Tags:         []string{"CMDB", "部署", "Docker Compose"}}, "cmdb-install.md"},
	{articleSeed{Title: "CMDB 智能运维平台 配置参数详解", Slug: "cmdb-config",
		CategorySlug: "automation",
		Summary:      ".env 与 K8s Secret 两层配置、Gateway 密钥清单、Keycloak Realm/角色、端口暴露与五条误配。",
		Tags:         []string{"CMDB", "配置", "Keycloak"}}, "cmdb-config.md"},
	{articleSeed{Title: "CMDB 智能运维平台 生产环境实战部署", Slug: "cmdb-production",
		CategorySlug: "automation",
		Summary:      "K8s 生产形态、中间件高可用清单、升级回滚纪律、RPO/RTO 备份目标与上线验收清单。",
		Tags:         []string{"CMDB", "生产实践", "Kubernetes"}}, "cmdb-production.md"},
	{articleSeed{Title: "CMDB 智能运维平台 常用命令速查", Slug: "cmdb-commands",
		CategorySlug: "automation",
		Summary:      "单机脚本五件套、kubectl 升级回滚、Vault 解封与一张十行排障速查表。",
		Tags:         []string{"CMDB", "命令行", "排障"}}, "cmdb-commands.md"},
	{articleSeed{Title: "CMDB 智能运维平台 官方文档导读", Slug: "cmdb-docs",
		CategorySlug: "automation",
		Summary:      "仓库地图、必读文档清单、1 天/1 周/1 个月学习路径与线上环境入口说明。",
		Tags:         []string{"CMDB", "文档"}}, "cmdb-docs.md"},
}

// ---------------- 教程集 ----------------

var cmdbToolBookSeeds = []toolBookSeed{
	{
		ToolSlug: "cmdb",
		Title:    "CMDB 智能运维平台从部署到实战",
		Summary:  "认识平台 → 三种方式装起来 → 吃透配置 → 上生产 → 日常命令 → 读官方文档，六章走完一套可交付的部署。",
		Level:    model.ToolBookLevelBeginner,
		Sort:     160,
		Chapters: toolBookChapters("cmdb", "CMDB 智能运维平台"),
	},
}

// ---------------- 四类实战教程关联 ----------------

var cmdbToolGuideSeeds = []toolGuideSeed{
	{"cmdb", model.ToolGuideDeploy, "平台部署手册（单机与 Kubernetes）",
		"https://github.com/disconf/CMDB/blob/main/docs/deployment/平台部署手册-单机与Kubernetes.md", "", 10},
	{"cmdb", model.ToolGuideImplement, "CMDB 安装与多种部署方式（站内教程）",
		"", "cmdb-install", 20},
	{"cmdb", model.ToolGuideOptimize, "生产就绪与灾备演练",
		"https://github.com/disconf/CMDB/blob/main/docs/operations/生产就绪与灾备演练.md", "", 30},
	{"cmdb", model.ToolGuideCluster, "Kubernetes 部署清单（deploy/k8s）",
		"https://github.com/disconf/CMDB/tree/main/deploy/k8s", "", 40},
}
