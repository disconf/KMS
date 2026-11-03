package bootstrap

import "github.com/opsexpert/platform/internal/model"

// 本文件是「CMDB 授权签发中心」的种子：CMDB 智能运维平台的配套授权管理站。
//
// **2026-11-03 起对外下线**（该工具不对外公开）：已从 seed_kb / seed_tool_articles /
// seed_toolbook / seed_news 四处合并点摘除，本文件仅保留内容备查；
// 重新上线时把 licToolSeeds / licToolArticleMetas / licToolBookSeeds /
// licToolGuideSeeds 在那四处接回即可，六篇正文仍在 data/articles/ 下。
// 注意：摘除种子不会清理线上存量数据，重新上线前需先清理或确认幂等跳过。
//
// 内容来源是项目仓库的 README（部署、配置、使用、安全约束章节都在其中），
// 不是泛泛的百科式介绍。注意两条内容纪律：
//   - 线上环境的账号密码一律不写进任何种子内容，只保留地址与角色说明；
//   - 公钥指纹、SHA256 等细节不进种子——它们属于仓库 README 的职责范围。
//
// 该项目当前为 GitHub 私有仓库，RepoURL 仍按真实地址收录；
// 授权中心是单副本单文件存储，没有集群形态，故只配 deploy/implement/optimize 三类指南。

// ---------------- 工具条目 ----------------

var licToolSeeds = []toolSeed{
	{
		Name:    "CMDB 授权签发中心",
		Slug:    "cmdb-license-center",
		Summary: "为 CMDB 智能运维平台离线签发 Ed25519 商业许可证：绑定 Deployment ID、控制版本/资产额度/功能包/有效期，私钥与平台物理分离。",
		Content: "## 适用场景\n\nCMDB 智能运维平台的商业交付与续费管理：客户买了多少资产额度、开了哪些功能包、" +
			"授权到哪天，都由这里签出的许可证说了算。签发全程离线，不要求客户平台联网。\n\n" +
			"## 能力一览\n\n" +
			"- **离线签发**：Ed25519 私钥签名，许可证是 `CMDB-LIC-v1.<payload>.<signature>` 一段文本，复制或下载 `.license` 文件交付。\n" +
			"- **实例绑定**：许可证绑定客户平台的 Deployment ID，只能在对应实例激活，复制到别处无效。\n" +
			"- **额度与功能包**：商业版本（Enterprise/Professional）、最大资产额度（>免费版 200）、授权天数（1~3650 或 0 永久）、逗号分隔功能包。\n" +
			"- **签发记录**：单 JSON 文件持久化在命名卷，容器重启/升级不丢，续期覆盖旧证后记录保留供审计。\n" +
			"- **开箱加固**：只读根文件系统、cap_drop ALL、非 root、私钥只读挂载不进镜像，单机 Compose 一键部署。\n\n" +
			"## 落地时最容易踩的坑\n\n" +
			"- **严禁运行 `license-keygen`**：它只用于全新产品线，在现有产品上跑一遍，所有已部署平台全部验签失败。\n" +
			"- **严禁 `docker compose down -v`**：命名卷里是签发记录账本，`-v` 一删全没。\n" +
			"- **`BIND_ADDRESS` 生产必须 `127.0.0.1`**：签发中心等于印钞机，裸暴露 8090 等于把私钥使用权交出去，前面必须有 Nginx HTTPS + IP 白名单。\n" +
			"- **仓库必须保持 Private**：可见性一变，立即轮换密钥并重新签发所有许可证。",
		RepoURL:      "https://github.com/disconf/CMDB-",
		HomepageURL:  "",
		License:      "未声明（仓库暂无 LICENSE 文件）",
		Language:     "Go",
		Stars:        0,
		ForkCount:    0,
		DeployModes:  []string{"docker"},
		Tags:         []string{"CMDB", "许可证", "商业化"},
		CategorySlug: "automation",
		Featured:     false,
		Sort:         310,
		ViewCount:    320,
	},
}

// ---------------- 教程集站内正文元数据 ----------------

var licToolArticleMetas = []toolArticleMeta{
	{articleSeed{Title: "CMDB 授权签发中心 软件介绍与核心概念", Slug: "cmdb-license-center-intro",
		CategorySlug: "automation",
		Summary:      "私钥与平台物理分离的商业授权站：许可证格式、Deployment ID 绑定、离线签发与覆盖式续期的完整闭环。",
		Tags:         []string{"CMDB", "许可证", "架构"}}, "cmdb-license-center-intro.md"},
	{articleSeed{Title: "CMDB 授权签发中心 安装与多种部署方式", Slug: "cmdb-license-center-install",
		CategorySlug: "automation",
		Summary:      "Docker Compose 从 clone 到 healthz 的六步部署，密钥权限收紧、key-check 验证与容器安全加固清单。",
		Tags:         []string{"CMDB", "部署", "Docker Compose"}}, "cmdb-license-center-install.md"},
	{articleSeed{Title: "CMDB 授权签发中心 配置参数详解", Slug: "cmdb-license-center-config",
		CategorySlug: "automation",
		Summary:      ".env 六个参数、容器内注入变量、私钥八条纪律与六条典型误配清单。",
		Tags:         []string{"CMDB", "配置", "密钥"}}, "cmdb-license-center-config.md"},
	{articleSeed{Title: "CMDB 授权签发中心 生产环境实战部署", Slug: "cmdb-license-center-production",
		CategorySlug: "automation",
		Summary:      "Nginx HTTPS + IP 白名单前置、资源规划、命名卷备份恢复、升级回滚与上线验收清单。",
		Tags:         []string{"CMDB", "生产实践", "HTTPS"}}, "cmdb-license-center-production.md"},
	{articleSeed{Title: "CMDB 授权签发中心 常用命令速查", Slug: "cmdb-license-center-commands",
		CategorySlug: "automation",
		Summary:      "Compose 日常命令、密钥校验、签发一张许可证的完整字段说明与一张七行排障速查表。",
		Tags:         []string{"CMDB", "命令行", "排障"}}, "cmdb-license-center-commands.md"},
	{articleSeed{Title: "CMDB 授权签发中心 官方文档导读", Slug: "cmdb-license-center-docs",
		CategorySlug: "automation",
		Summary:      "仓库地图、必读顺序、1 天/1 周学习路径、API 一览与安全约束速记。",
		Tags:         []string{"CMDB", "文档"}}, "cmdb-license-center-docs.md"},
}

// ---------------- 教程集 ----------------

var licToolBookSeeds = []toolBookSeed{
	{
		ToolSlug: "cmdb-license-center",
		Title:    "CMDB 授权签发中心从部署到实战",
		Summary:  "认识授权闭环 → Compose 部署 → 吃透配置与密钥纪律 → 上生产加固 → 日常命令 → 读文档，六章签出第一张商业许可证。",
		Level:    model.ToolBookLevelBeginner,
		Sort:     170,
		Chapters: toolBookChapters("cmdb-license-center", "CMDB 授权签发中心"),
	},
}

// ---------------- 三类实战教程关联 ----------------

var licToolGuideSeeds = []toolGuideSeed{
	{"cmdb-license-center", model.ToolGuideDeploy, "CMDB 授权签发中心 安装与多种部署方式（站内教程）",
		"", "cmdb-license-center-install", 10},
	{"cmdb-license-center", model.ToolGuideImplement, "CMDB 授权签发中心 配置参数详解（站内教程）",
		"", "cmdb-license-center-config", 20},
	{"cmdb-license-center", model.ToolGuideOptimize, "CMDB 授权签发中心 生产环境实战部署（站内教程）",
		"", "cmdb-license-center-production", 30},
}
