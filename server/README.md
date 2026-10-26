# 运维专家服务平台 · 后端（Go）

按 `../前端功能设计-MVP.md` 与 `../运维专家服务平台方案.md` 实现的后端服务。
知识库五个栏目（教程 / 题库 / 资讯 / 工具 / 资源）的接口与页面都在这个仓库里，
`go run ./cmd/api` 之后打开 `http://localhost:8080` 就是一个能用的知识库站点。

| 阶段 | 内容 | 状态 |
|---|---|---|
| 一 | 工程骨架 + 认证闭环（短信 / 微信登录、双令牌、个人资料） | ✅ 完成 |
| 二 | 知识库（分类 / 文章 / 搜索 / 标签 / 点赞 / 后台发布下线） | ✅ 完成 |
| 三 | 知识库多栏目扩展（栏目导航、开源工具、运维题库、资源下载） | ✅ 完成 |
| 四 | 知识库前端页面（零构建静态站点、由本服务托管、二进制内嵌） | ✅ 完成 |
| 五 | 文件上传（本地上传 + 私有文件签名下载，OSS/COS 待接） | ✅ 完成 |
| 六 | 专家模块（入驻 / 审核 / 工作台 / 服务管理 / 钱包提现） | ✅ 完成 |
| 七 | 订单模块（悬赏 / 预约、状态流转、会话、评价、争议、结算分账）+ 支付通道（mock / offline） | ✅ 完成 |
| 八 | 定时任务（支付超时关单、48 小时自动确认、结算兜底扫描） | ✅ 完成 |
| 九 | 订单前端页面（发布悬赏、订单中心、接单大厅、支付 / 会话 / 评价 / 争议） | ✅ 完成 |
| 十 | 内容运营（栏目更名、题库十个方向、工具四类教程、资源外链、资讯订阅源） | ✅ 完成 |
| 十一 | 运营后台（内容审核队列、用户与专家治理、系统配置） | ✅ 完成 |
| 十二 | 专家端页面（工作台、服务管理、钱包提现） | ✅ 完成 |
| 十三 | 提现审批（两段式 pending→approved→paid，打款必填流水号） | ✅ 完成 |
| 十四 | 用户治理（封禁只改状态不动业务数据、专家审核队列） | ✅ 完成 |
| 十五 | 财务对账（四时间口径分开算、佣金+专家=总额恒等式） | ✅ 完成 |
| 十六 | 系统配置（抽佣比例进 system_settings、技能字典、分类字典） | ✅ 完成 |
| 十七 | 专家主页与评价（汇总、分布、匿名脱敏、专家回复） | ✅ 完成 |
| 十八 | 问答广场（提问 / 回答 / 采纳 / 积分） | ✅ 完成 |
| 十九 | 题库批量导入与题目投稿 | ✅ 完成 |
| 二十 | 教程集 ToolBook（目录树只做目录、章节挂官方文档外链） | ✅ 完成 |
| 二十一 | 题库与工具内容填充（10 个方向 10557 题、15 个工具各配一本教程集） | ✅ 完成 |

---

## 一、快速开始

### 方式 A：零依赖启动（推荐先跑这个）

不需要 MySQL，不需要 Redis，不需要 Docker。默认用 SQLite + 进程内缓存。

```bash
cd server
cp configs/config.example.yaml configs/config.yaml   # 可选，不拷也能起（走内置默认值）
go run ./cmd/api
```

Windows 下等价命令：

```powershell
cd server
Copy-Item configs\config.example.yaml configs\config.yaml
go run .\cmd\api
```

启动后：

| 地址 | 说明 |
|---|---|
| http://localhost:8080/ | **知识库前端站点**（首页、五个栏目、搜索、登录） |
| http://localhost:8080/docs | **Swagger UI，可直接调接口** |
| http://localhost:8080/openapi.yaml | OpenAPI 规范（可导入 Postman / Apifox） |
| http://localhost:8080/healthz | 存活探针 |
| http://localhost:8080/readyz | 就绪探针（含数据库检查） |

前端是**零构建**的原生 ESM 站点，源码在 `web/static/`，用 `go:embed all:static` 打进二进制——
一份 exe 就是完整的站点，不需要 Node、不需要 `npm install`、不需要单独部署静态资源。

改前端时用 `WEB_DIR` 指到磁盘目录，刷新浏览器即生效，不用重新编译：

```bash
WEB_DIR=./web/static go run ./cmd/api     # 留空则用二进制里的内嵌副本（生产自包含）
```

`web.Register` 只接管 `/` 与 `/assets/*`，**刻意不挂通配兜底**——否则会把 `/healthz`、`/docs`、
`/api/v1/**` 一起吞掉。同理前端路由用的是 hash（`#/kb/articles`）而不是 History API：
后者在纯静态托管下需要通配回落到 `index.html`，正好是上面那个会冲突的做法。

前端外壳是**单行顶栏 + 居中内容**（品牌 · 栏目导航 · 搜索 · 账号），没有左侧栏：
纵向菜单把画面重心压在左上，宽屏下正文被挤得偏右，整体像博客模板而不是平台。
导航是横向贴底的，活动项用下划线指示；窄屏下导航换到第二行横向滚动（活动项自动滚到中间），
搜索折叠成图标点开整行。服务入口（悬赏 / 订单 / 大厅）由 `app.js` 顶部的
`FEATURES.service` 开关控制——路由和页面代码都在，改成 `true` 即恢复露出，不要去动路由表。

### 方式 B：MySQL + Redis

```bash
cd server
docker compose -f deploy/docker-compose.yml up -d     # 起 MySQL 8 + Redis 7

# 切到 MySQL
export DB_DRIVER=mysql
export DB_DSN='root:root@tcp(127.0.0.1:3306)/ops_expert?charset=utf8mb4&parseTime=True&loc=Local'
export REDIS_ADDR=127.0.0.1:6379
go run ./cmd/api
```

Windows PowerShell 用 `$env:DB_DRIVER="mysql"` 这种写法，或直接改 `configs/config.yaml`。
配置项持久化推荐复制 `.env.example` 为 `.env`——启动时会自动加载。

---

## 二、验证认证闭环（30 秒）

开发环境短信通道是 `mock`，验证码不真发，直接从接口返回的 `devCode` 字段拿。

```bash
# 1) 发验证码
curl -s -X POST http://localhost:8080/api/v1/auth/sms/send \
  -H 'Content-Type: application/json' \
  -d '{"phone":"13800000001","scene":"login"}'
# => {"code":0,"message":"ok","data":{"expiresIn":300,"interval":60,"devCode":"123456"},...}

# 2) 用 devCode 登录（首次会自动注册）
curl -s -X POST http://localhost:8080/api/v1/auth/login/sms \
  -H 'Content-Type: application/json' \
  -d '{"phone":"13800000001","code":"<上一步的 devCode>","nickname":"老王"}'

# 3) 带上 accessToken 查资料
curl -s http://localhost:8080/api/v1/me -H 'Authorization: Bearer <accessToken>'
```

微信登录在开发环境用 `mock` 通道，`code` 传 `mock:<任意openid>` 即可跑通：

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/login/wechat \
  -H 'Content-Type: application/json' \
  -d '{"code":"mock:wx_openid_demo"}'
```

### 开发环境管理员账号

种子数据会在 dev/test 环境创建：

- 手机号 `13800000000`，角色 `admin`
- 初始口令 `Admin@12345`（登录后台用，口令不入日志）
- 三个知识库分类（`k8s` / `database` / `linux-network`）与三篇示例文章

生产环境会**跳过全部种子数据**。

---

## 二·补充、验证知识库（游客可读，无需登录）

知识库有 5 个栏目，各自承载不同的内容模型。栏目由 `internal/model/channel.go` 的注册表定义：

| 栏目 | slug | 内容模型 | 已验证的种子数量 |
|---|---|---|---|
| Linux 实战教程 | `linux-tutorial` | article | 3 篇 |
| 运维题库 | `ops-quiz` | quiz | 6 题 |
| 运维资讯 | `ops-news` | article | 0 条（本期仅人工录入，未做自动聚合） |
| 开源工具推荐 | `open-source-tools` | tool | 5 个 |
| 资源下载 | `resources` | resource | 3 个 |

```bash
# 栏目导航（含各栏目已发布内容数，用于渲染角标）
curl -s http://localhost:8080/api/v1/channels

# 教程 / 资讯共用文章接口，用 channel 区分
curl -s 'http://localhost:8080/api/v1/kb/articles?channel=linux-tutorial&sort=hot'
curl -s 'http://localhost:8080/api/v1/kb/articles?keyword=重启'
curl -s http://localhost:8080/api/v1/kb/articles/k8s-pod-restart-troubleshooting
curl -s http://localhost:8080/api/v1/kb/categories
curl -s http://localhost:8080/api/v1/kb/tags
```

### 检索：可以用一句话提问

`keyword` 不必是关键词，整句问句也能搜到东西：

```bash
# 直接问「磁盘满了怎么办」——会被解析成「磁盘 / 硬盘 / 磁盘空间 / disk / df」再去匹配
curl -s 'http://localhost:8080/api/v1/kb/articles?keyword=磁盘满了怎么办'
curl -s 'http://localhost:8080/api/v1/tools?keyword=k8s集群怎么部署'

# 只是想看看某个输入会被解析成什么，以及有哪些热门词
curl -s 'http://localhost:8080/api/v1/kb/search/suggest?q=磁盘满了怎么办'
# → {"terms":["磁盘"],"expanded":["磁盘","硬盘","磁盘空间","空间不足","盘满","disk","df"],"hot":[...]}
```

解析规则写在 `internal/service/search_query.go`：归一化（全角转半角、标点转空格）→
去掉疑问词与语气词 → 按汉字/非汉字切段 → 用领域词典对连续汉字做最长匹配切分 → 同义词展开。
词典是一张 `map[string][]string`，**加词只需加一行**，检索与切分同时生效。

中英混排是常态，`cpu占用高怎么办` 会先切成 `cpu` 与 `占用高` 再各按各的规则处理；
`占用高` 不是词，词典里收了 `占用` 就能切出来（不收就只能整串保留，而整串 `LIKE` 一条也命中不了）。
切完之后 `cpu` 与 `占用` 落在同一组，组间去重又会把两个词并成一个条件。

解析不出任何词时**回落到整串匹配，而不是「不过滤」**。这个区别很关键：
后者的表现是「搜什么都返回全站内容」——比搜不到更难被用户理解成搜索坏了。

### 开源工具推荐

```bash
# 列表：按语言精确筛选 + 按 Star 降序
curl -s 'http://localhost:8080/api/v1/tools?language=Go&sort=stars'

# 筛选项：languages / licenses 从真实数据聚合，deployModes 是后端白名单
curl -s http://localhost:8080/api/v1/tools/filters

# 详情：key 可传 slug 或数字 ID，浏览数自增
curl -s http://localhost:8080/api/v1/tools/prometheus
```

`/tools/filters` 是静态段，与 `/tools/{key}` 同层共存不会冲突——gin 的静态节点优先匹配。

### 资源下载

```bash
# 列表与文件类型筛选项
curl -s 'http://localhost:8080/api/v1/resources?sort=download'
curl -s http://localhost:8080/api/v1/resources/file-exts

# 详情
curl -s http://localhost:8080/api/v1/resources/nginx-ops-scripts

# 取下载地址（现场签发签名 URL，并把下载数加一）
curl -s http://localhost:8080/api/v1/resources/nginx-ops-scripts/download
```

标了 `needLogin` 的资源，未登录取下载地址会被拒（业务码 **40031**）；其余资源游客可直接下载。
签名 URL 篡改后访问返回 **70005**。

### 运维题库（答题需登录）

```bash
# 题目列表（出参不含答案与解析）
curl -s 'http://localhost:8080/api/v1/quiz/questions?difficulty=medium'

# 随机抽题（在 SQL 层完成，不是取回全表再内存洗牌）
curl -s 'http://localhost:8080/api/v1/quiz/practice?count=5&difficulty=medium'

TOKEN='<登录后的 accessToken>'

# 单题作答并即时判分，无论对错都返回正确答案与解析
curl -s -X POST http://localhost:8080/api/v1/quiz/questions/1/answer \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"chosen":["A"]}'

# 整卷提交，返回成绩单与逐题解析
curl -s -X POST http://localhost:8080/api/v1/quiz/attempts \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"items":[{"questionId":1,"chosen":["A"]},{"questionId":2,"chosen":["B","C"]}],"durationSec":120}'

# 我的战绩 / 历史 / 错题本
curl -s http://localhost:8080/api/v1/quiz/stats      -H "Authorization: Bearer $TOKEN"
curl -s http://localhost:8080/api/v1/quiz/attempts   -H "Authorization: Bearer $TOKEN"
curl -s http://localhost:8080/api/v1/quiz/wrong-book -H "Authorization: Bearer $TOKEN"
```

判分口径见下文「设计决定」第 11–13 条。

### 按方向刷题

```bash
# 10 个方向，各自题量 + 当前用户进度（未登录也能调，进度为 0）
curl -s http://localhost:8080/api/v1/quiz/categories

# 只在这个方向里抽题
curl -s 'http://localhost:8080/api/v1/quiz/practice?count=5&categoryId=3' \
  -H "Authorization: Bearer $TOKEN"
```

`/quiz/categories` 与 `/kb/categories` 是两回事：后者的 count 是**文章数**，与题目数无关。
题量为 0 的方向也会返回，前端标「筹备中」并置灰——让一个方向凭空消失会让运营以为分类没建。

### 工具的四类实战教程

```bash
# 详情里带 guides：deploy / implement / optimize / cluster
curl -s http://localhost:8080/api/v1/tools/prometheus
```

每条教程要么关联一篇**已发布**的站内文章（`articleSlug` 非空，前端跳 `#/article/<slug>`），
要么给一个外链（`url`，必须 http/https），两者互斥。前台会跳过关联了已下线文章的条目，
后台不过滤——运营得看到「这条挂的文章打不开」。

```bash
# 后台整体替换（不是逐条增删），每工具最多 24 条
curl -s -X PUT http://localhost:8080/api/v1/admin/kb/tools/1/guides \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"guides":[{"kind":"deploy","articleId":3},{"kind":"optimize","url":"https://example.com/tune"}]}'
```

### 系统教程集（ToolBook 两级章节）

教程集与四类实战教程**并存而不是互相替代**：

- `ToolGuide`（四类教程）回答「这个工具有哪些值得看的文章」——条目散装、无序；
- `ToolBook`（教程集）回答「我该怎么系统地学会它」——章 / 节两级、有严格顺序。

教程集**只做目录组织，不存正文**：每一节指向一篇站内文章或一个外链，
编辑器、图片上传、审核流、SEO 全部复用文章那一套。

```bash
# 工具详情里带 books（含每本书的完整目录树）
curl -s http://localhost:8080/api/v1/tools/openresty
# 独立打开一本教程集（收藏夹 / 分享链接直达）
curl -s http://localhost:8080/api/v1/tool-books/1

# 后台：建书（固定草稿）→ 大纲整体替换 → 发布
curl -s -X POST http://localhost:8080/api/v1/admin/kb/tools/5/books \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"title":"Nginx 从装到集群","level":"beginner"}'
curl -s -X PUT http://localhost:8080/api/v1/admin/kb/tool-books/1/outline \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"outline":"# 第一章 装起来\n## 1.1 官方安装 | https://openresty.org/en/installation.html | 两种方式的取舍\n## 1.2 一次 DNS 超时复盘 | dns-timeout-incident-review"}'
curl -s -X POST http://localhost:8080/api/v1/admin/kb/tool-books/1/publish -H "Authorization: Bearer $ADMIN"
```

大纲格式：`# 章` 与 `## 节标题 | 文章slug或外链 | 导语`（竖线全角也接受）。
保存是**整体替换**，解析不出正文的行保留标题去掉链接并记入 `warnings`，不阻断保存。
目录为空的书不允许发布；失效节点（文章被删/下线）前台直接跳过、后台标红。

### 系统题库：事实文件 + 生成引擎

十个方向各一千道以上的题不是手写的，而是**事实文件 + 模板生成**：

```
scripts/quizgen/
├── generate.py          # 模板引擎（cmd/flag/symptom/config/concept 五类事实展开成题）
├── bank/linux.py        # 事实矩阵，一个方向一个文件
├── bank/linux_extra.py  # 同 CAT 的补充文件会和主文件合并，用于把题量堆到 1000+
└── ...
internal/bootstrap/data/quiz_bank.json   # 生成产物，go:embed 进二进制
```

```bash
cd scripts/quizgen
python generate.py --min 1000     # 重新生成；任一方向不足 1000 直接非零退出
```

改内容只需动 `bank/*.py` 里的事实，不要手改 JSON——JSON 每次运行整体重写。
`SEED=20261023` 是固定的，同一份事实两次生成结果逐字相同，diff 才有意义。

几条约束（都是踩过的）：

1. **干扰项只从同方向同 kind 的事实池抽**，所以错的选项也是真命令，不会出现「以上都不是」凑数。
2. **judge 的否定式必须由模板自己给**。早先是把肯定句扔给「是→不是」的字符串替换翻，
   遇到「…里全是 Sending data 的典型原因是 Y」，第一个「是」在症状里，翻出来就成了
   「里全不是 Sending data…」——症状被改坏，题就读不懂了。
3. **概念事实的 `wrong` 字段本身就是错话**，答案固定「错误」；再翻一次等于把错话标成「正确」，
   是答案键错误，不只是文案问题。
4. 同一方向内题干去重后才落盘：库里挂两条一模一样的题干，刷题的人会以为系统出错了。

### 种子与幂等（升级重启不会翻倍）

首次启动会灌一批演示数据（分类、文章、专家、订单、题库、工具、教程集）。
判重不看 id，看**业务键**——所以升级重启不会翻倍：

| 种子 | 幂等键 |
|---|---|
| 系统题库 | `category_id + stem`，且 `tags LIKE '%系统题库%'` |
| 工具 / 资源 / 资讯 | `slug` / `external_id` |
| 教程集 | `tool_id + title` |

题目按批写入（SQLite 100 条/批、MySQL 200 条/批），一万多道题冷启动约几秒。
只对**新题干**入库，所以改了事实文件后旧题不会自动消失：

```bash
# 要让线上按新 JSON 重建（会连带清掉这些题的作答记录）
sqlite3 data/ops_expert.db "delete from quiz_records where question_id in
  (select id from quiz_questions where tags like '%系统题库%');"
sqlite3 data/ops_expert.db "delete from quiz_questions where tags like '%系统题库%';"
systemctl restart ops-expert
```

### 工具教程集的站内正文（15 工具 × 6 篇）

教程集最初每一节都挂官网外链，读者点进去就离开本站。现已改为**正文写在本站**：
每个工具六篇文章，教程集六章各挂一节，全部站内 hash 路由（可点赞、有相关推荐）：

| 章 | 正文 | 内容 |
|---|---|---|
| 第一章 软件介绍 | `<tool>-intro` | 定位、解决什么问题、架构与核心概念、生态、适用边界 |
| 第二章 多种部署方式 | `<tool>-install` | 发行版仓库 / 二进制 / 容器 / 编排 / 离线安装的取舍与步骤 |
| 第三章 配置参数详解 | `<tool>-config` | 配置文件位置、核心参数分组讲解、常见误配清单 |
| 第四章 生产实战部署 | `<tool>-production` | 核心参数、集群形态、备份与验收 |
| 第五章 常用命令速查 | `<tool>-commands` | 日常操作 + 排障清单 |
| 第六章 官方文档导读 | `<tool>-docs` | 必读页面清单、1天/1周/1月学习路径、版本升级、求助渠道 |

- 正文是 Markdown，放在 `internal/bootstrap/data/articles/<slug>.md`（90 篇），
  `go:embed` 进二进制；标题 / 摘要 / 分类 / 标签在 `seed_tool_articles.go`
  与 `seed_tool_articles_extra.go` 维护；节导语自动取文章摘要，单一事实来源。
- 文章幂等与普通文章种子一致：按 slug 判存在，已存在就跳过——**改了 md 不会自动更新**，
  线上要改就进后台编辑该文章。
- 目录结构升级迁移：启动时发现书里缺种子期望的节标题（旧结构三章、或还有外链节），
  就把该书目录物理删除重建（日志「教程集目录重建为最新结构」）。
  运营手工建的目录若含全部期望节标题则不会被动；只增不减的新章节不会误伤手工目录
  之外的书。

### 资源：托管文件 与 只给外链


安装包动辄上百 MB、版本更新频繁，自己托管既不划算也容易过期。所以资源有两种形态：

| 形态 | 判据 | 前台表现 |
|---|---|---|
| 平台托管文件 | `hasFile=true` | 显示下载按钮，走签名 URL |
| 只给外链 | `hasFile=false` | 只列官网 / 镜像 / 包管理器 / 代码仓库 / 容器镜像 |

```bash
curl -s http://localhost:8080/api/v1/resources/docker-engine-download   # hasFile=false + links
curl -s http://localhost:8080/api/v1/resources/docker-engine-download/download  # 400，提示去点链接
```

后者请求 `/download` 会被拒（**40030**），提示语是「请在资源页点击对应链接获取」而不是
「请重新上传」——用户手里没有文件，那是运营的动作。

```bash
# 后台整体替换外链，每资源最多 8 条；kind 不认识会归一成 other 而不报错
curl -s -X PUT http://localhost:8080/api/v1/admin/kb/resources/2/links \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"links":[{"label":"官方安装指引","url":"https://docs.docker.com/engine/install/","kind":"official"}]}'
```

### 运维资讯：订阅源自动抓取

```bash
# 订阅源列表（含上次抓取时间、状态、错误原因、全局待采用条数）
curl -s http://localhost:8080/api/v1/admin/news/sources -H "Authorization: Bearer $ADMIN"

# 新增：kind=rss 填 feed 地址；kind=github 填 owner/repo，走 Releases API
curl -s -X POST http://localhost:8080/api/v1/admin/news/sources \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"name":"Cilium Releases","url":"cilium/cilium","kind":"github","autoPublish":true}'

# 立即抓取（绕过节流，运营加完源要马上验证）
curl -s -X POST http://localhost:8080/api/v1/admin/news/sources/1/fetch \
  -H "Authorization: Bearer $ADMIN"
```

几个刻意的取舍：

- `autoPublish` **默认 false**，抓回来的内容进 `pending` 待采用池，由运营挑选发布。
- 调度器分钟级唤醒，但 `NewsService` 内部有 **20 分钟节流**——配了 1m 也不会每分钟去打别人的站。
- 去重靠 `articles.external_id` 唯一索引；重复抓到的返回 `written=0` 而不是报错。
- 单源失败只记 warn，不让整轮任务报错：抓取不碰钱，失败也不影响交易。
- 列表里每条会带 `sourceName` / `sourceUrl`，前端显示来源徽标与「查看原文」。

### 内容投稿与审核

发布权限不是「有账号就能发」：任何登录用户都能投稿，但**落库固定为 `pending`**，
审核通过才上线。

```bash
# 投稿（没有 status 字段——让客户端传状态等于把发布权限交给浏览器）
curl -s -X POST http://localhost:8080/api/v1/kb/articles/contribute \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Nginx 502 排查：keepalive 超时导致连接被复用","content":"## 现象\n…","channel":"linux-tutorial","tags":["nginx","502"]}'

# 我的投稿（含状态与驳回原因）
curl -s http://localhost:8080/api/v1/me/contributions -H "Authorization: Bearer $TOKEN"

# 审核：approve 通过并发布；reject 必须带原因
curl -s -X POST http://localhost:8080/api/v1/admin/kb/articles/12/review \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"action":"reject","note":"缺少验证步骤，命令没有版本号"}'
```

- 运维资讯**不接受投稿**：开放等于让任何人往「行业动态」里塞软文，且无法与抓取内容区分。
- 驳回必须填原因，否则投稿人只能反复试错。
- 审核用条件更新（`WHERE status='pending'`）：两个管理员同时点「通过」，第二次会报
  「这篇内容已被其他人处理」，而不是把发布时间再刷一遍。

题目投稿走同一套语义（入口统一在 `#/contribute`，tab 切换表单）：

```bash
# 投稿一道题：解析必填，请求里没有 status 字段，落库固定 pending
curl -s -X POST http://localhost:8080/api/v1/quiz/contribute \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"stem":"容器内 df 显示已满但 du 求和远小于容量，最可能的原因是什么？","type":"single","options":[{"key":"A","text":"有进程持有已被删除的文件"},{"key":"B","text":"磁盘没有真正挂载"}],"correct":["A"],"analysis":"被删除但仍被进程打开的文件会继续占用空间。"}'

# 我投的题（与文章投稿共用同一份卡片结构，kind 字段区分）
curl -s "http://localhost:8080/api/v1/quiz/my/contributions" -H "Authorization: Bearer $TOKEN"

# 审核：通过则发布并给投稿人计 5 贡献分（只记一次）；驳回必填原因
curl -s -X POST http://localhost:8080/api/v1/admin/kb/quiz/questions/9/review \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"action":"reject","note":"正确答案标错了，A 才对"}'
```

「我的投稿」页面在 `#/me/contributions`，两个 tab 分别列教程与题目；
题目没有单题前台页，卡片不给链接，状态与驳回原因直接摆在行内。

### 问答广场（读游客可读，写需登录）

和「悬赏提问」是**两套东西**：悬赏有金额、有订单、有结算；问答广场是免费社区帖，
没有金额字段、不进订单链路。发一条免费问题不需要付钱，也就不需要为它设计退款与纠纷。

```bash
# 列表（游客可读；第一页带 stats：待解答 / 已解决 / 已发布回答数，翻页不带）
curl -s 'http://localhost:8080/api/v1/qa/questions?status=open&unanswered=true&pageSize=10'

# 一句话检索同样走查询理解层：「磁盘满了怎么办」能捞到「容器磁盘空间告警排查」
curl -s 'http://localhost:8080/api/v1/qa/questions?keyword=磁盘满了怎么办'

# 提问（同一账号 10 分钟一条）
curl -s -X POST http://localhost:8080/api/v1/qa/questions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"K8s Pod 一直 CrashLoopBackOff 该从哪查？","description":"describe 里 reason=Error，日志只有一行 Started…","skillIds":[1]}'

# 回答（一人一答；普通账号前 3 条进 pending，专家与管理员直发）
curl -s -X POST http://localhost:8080/api/v1/qa/questions/7/answers \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"content":"先看 lastState.terminated.exitCode，137 与 1 的处理方向完全不同。"}'

# 采纳（只有提问人能做，采纳给回答作者 +10 分）
curl -s -X POST http://localhost:8080/api/v1/qa/questions/7/accept/12 \
  -H "Authorization: Bearer $TOKEN"

# 点赞（幂等；分记在回答作者头上，自赞不记分）
curl -s -X POST http://localhost:8080/api/v1/qa/answers/12/like -H "Authorization: Bearer $TOKEN"

# 我的贡献（积分只做展示，不兑换权益）
curl -s http://localhost:8080/api/v1/qa/my/points -H "Authorization: Bearer $TOKEN"
```

问答广场几个刻意的取舍：

- **一人一答**（`(question_id, user_id)` 联合唯一索引）：想补充就编辑原回答。
  否则一个帖子会被同一个人用「补充一下 / 再补充」刷成几十层。
- **审核分层**：新账号前 3 条回答进 `pending`，管理员与**已过审专家**免审直发。
  一律先审会把「有人愿意答」这个最稀缺的动作拖成隔天见，问答区当场就死了。
  专家的判定要求 `role=expert` **且** `expertStatus=approved`——只设角色不给档案不算。
- **采纳是 CAS**：`UPDATE ... WHERE id=? AND user_id=? AND status='open'`，
  一条语句同时完成身份校验、状态校验与写入。拆成「先查再改」会让两个人各拿一次积分。
- **积分只做展示**（采纳 +10、获赞 +1、投稿过审 +20、题目过审 +5，徽章阈值 0/10/50/200），
  不兑换任何权益。一旦能换钱，就得防刷、要对账、要能追回，等于又开一条准资金链路。
- **后台把「问题 + 采纳回答」转成教程草稿**（`POST /admin/qa/questions/:id/to-article`），
  只生成 `draft`、不给直接发布：问答原文是一问一答的碎片，直接发出去读者看到的不是教程。

### 后台录入（需 admin 令牌）

四类内容的后台接口统一挂在 `/api/v1/admin/kb` 下，导航上「知识库」一级菜单下直接就是各类内容的子菜单。

```bash
TOKEN='<管理员 accessToken>'

# —— 文章 ——
curl -s -X POST http://localhost:8080/api/v1/admin/kb/articles \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Redis 大 key 排查","content":"## 现象\n\n延迟毛刺。","channel":"linux-tutorial","categoryId":2,"tags":["Redis"]}'
curl -s -X POST http://localhost:8080/api/v1/admin/kb/articles/1/publish -H "Authorization: Bearer $TOKEN"
curl -s -X POST http://localhost:8080/api/v1/admin/kb/articles/1/offline -H "Authorization: Bearer $TOKEN"

# —— 工具 ——
curl -s -X POST http://localhost:8080/api/v1/admin/kb/tools \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Terraform","summary":"基础设施即代码","repoUrl":"https://github.com/hashicorp/terraform","license":"BUSL-1.1","language":"Go","deployModes":["binary","docker"],"tags":["IaC"],"status":"published"}'

# —— 资源（先上传拿 fileKey，再建条目）——
curl -s -X POST http://localhost:8080/api/v1/upload \
  -H "Authorization: Bearer $TOKEN" -F 'scene=resource_file' -F 'file=@./scripts/smoke.sh'
curl -s -X POST http://localhost:8080/api/v1/admin/kb/resources \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Nginx 巡检脚本","fileKey":"resource/2025/10/xxxx.sh","fileName":"nginx-check.sh","version":"v1.0.0","tags":["nginx"],"status":"published"}'

# —— 题目 ——
curl -s -X POST http://localhost:8080/api/v1/admin/kb/quiz/questions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"stem":"Pod 处于 Pending 的常见原因有哪些？","type":"multiple","options":[{"key":"A","text":"节点资源不足"},{"key":"B","text":"未绑定 PVC"},{"key":"C","text":"镜像拉取失败"}],"correct":["A","B"],"analysis":"镜像拉取失败时 Pod 是 ImagePullBackOff。","difficulty":"medium","status":"published"}'

# —— 题目批量导入（Excel 复制出的 TSV / CSV / JSON 数组都行，前端先解析成同一种结构）——
curl -s -X POST http://localhost:8080/api/v1/admin/kb/quiz/questions/import \
  -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
  -d '{"items":[{"stem":"df 与 du 结果不一致最可能的原因？","type":"single","options":[{"key":"A","text":"进程持有已删除的文件"}],"correct":["A"],"analysis":"lsof +L1 可查"}]}'
```

批量导入的规则（`scripts/render_check.mjs` 里有真接口断言）：

- **全量校验后一次落库**：任一行有问题则一条都不写（`imported=0`），错误带行号一次给全——
  半批入库是最难收拾的状态，用户不知道哪几条进去了，再传怕重复；
- **导入一律落草稿**：批量灌进来的题没人逐条看过，直接 `published` 等于把未检查内容推给刷题用户；
- 单次上限 200 条——上限不是为性能，是让「整体失败」仍然可用；
- 校验规则与单题录入完全一致，不会因为走导入就少校验几项。

录入会被拦下的几类错误（已实测）：

| 提交内容 | 业务码 | 提示 |
|---|---|---|
| `repoUrl` 用 `javascript:` 协议 | 40019 | 仓库地址必须以 http:// 或 https:// 开头 |
| `deployModes` 传 `k8s` | 40018 | 不支持的部署方式：k8s，可选：binary / docker / helm / k8s-operator / source / cloud |
| 多选题 `correct` 覆盖全部选项 | 40023 | 多选题的正确答案不能覆盖全部选项 |
| 资源的 `fileKey` 不是 `resource_file` 场景 | 40030 | 文件不是通过资源上传场景获得的，请使用 scene=resource_file 重新上传 |

### 运营后台（浏览器里维护内容）

`http://localhost:8080/#/admin`。上表的接口原先只能 curl，运营改一篇文章、加一个订阅源都得找人，
所以补了一层页面：**概览 → 待审队列 → 资讯源 → 文章 → 工具 / 四类教程 → 资源 / 外链 → 题库 → 专家审核 → 提现审批 → 对账 → 订单 → 用户**。
入口只出现在账号区，且仅 `admin` 可见。

几条刻意的做法：

- **路由不藏**。`/admin/*` 对所有人可访问，页面自己判断：未登录给登录引导、非管理员给「没有后台权限」。
  把路由藏起来只会让普通用户看到「页面不存在」，排查时更费劲；权限本来就在服务端（`RequireRole(admin)`），
  前端这层只决定「入口摆不摆、表单渲不渲」。
- **后台列表的 `status` 过滤比写入宽**。审核队列要靠 `?status=pending` 捞内容，而写入路径仍只允许
  `draft / published`——两者刻意不同，不要"顺手对齐"。
- **教程与外链整组保存**。页面上改完一次性提交，与 `ReplaceGuides` / `SetResourceLinks` 的事务语义对齐；
  空行会被忽略（多加一行没填不该变成保存失败）。
- **抓取失败的源要显示原因**。列表里 `lastStatus` / `lastError` 直接铺在「上次抓取」列，运营不用去翻日志。
- **提现审批是两步，不是一步**。「通过」只表示同意提，钱还冻在专家账户里；线下转账完成后单独点
  「已打款」才扣冻结。做成「通过即打款」等于把核对收款人这一步省掉了。驳回与打款失败都会把冻结
  退回可提现余额，页面上每次都写明这一步的金额去向。
- **用户列表的手机号是脱敏的，且只能按完整号码查人**。列表页会被投屏、被截图发到群里，
  一个能看见全部号码的表格就是一份通讯录。要核对身份就走"输入 11 位完整号码"的全等查询——
  片段模糊匹配等于把"这个号段里有哪些人"开放出去。
- **对账表不给"当日利润"**。收款、退款、结算、打款发生在四个时点，合成一个净额必然有人算错：
  收款里含着代管中的专家款，结算只是把钱从代管挪进专家余额（现金没动），打款才是钱离开平台。
  页面只给一个定义写在列头里的 `净现金 = 收款 − 退款 − 打款`，其余各列各算各的。
- **导出的 CSV 里不写校验结论**。这张表是给 Excel 求和的，末尾多一段自由文本会让行数不确定、
  求和范围跟着错位；不平的结论留在页面上说。
- **封禁只改账号状态，不动任何业务数据**。专家档案、订单、余额、提现记录一概保留，解封后原样恢复；
  详情弹窗会在封禁前提示"这个人还有几笔在途订单"——那些单子不会因为封禁自动取消，得去订单页逐笔处理。
- 后台是密集界面，行高压得比前台紧、字号小一档：这里「一屏能看几行」比「读起来舒服」更重要。

改完后台页面，除了 `render_check.mjs`，建议在真实浏览器里再走一遍：后台的排版问题（栅格多一列、
表格撑破容器）字符串断言看不出来。量化的做法是拿 CDP 打开页面直接读盒模型——
`documentElement.scrollWidth - clientWidth`（横向溢出）、`scrollHeight > clientHeight` 且
`overflow:hidden`（纵向被切）、`.admin-guide-row` 子元素的 `getBoundingClientRect().width`（栅格列宽），
1440 与 820 两个宽度各跑一遍即可，比截图对比稳定。

### 专家端：广场、入驻、工作台

`#/experts`（广场）→ `#/experts/:id`（主页）→ `#/expert/apply`（入驻）→ `#/expert`（工作台）。
这一层补的是整条链路上最后一段：接口从第一个版本就在，但**专家本人没有自助入口**——
申请要靠 curl，看收入也要靠 curl。

- **广场的空态就是招募位**。首批专家还在认证时，这页显示的是「申请成为专家」而不是一片空白；
  顶栏入口因此常驻，不必等"有专家了再放出来"。
- **入驻是一页四段，不是四步向导**。被驳回的人回来通常只改一个字段，向导会逼他把前面几步再点一遍。
  分段保留"走到哪了"的视觉节奏，但不用维护步骤状态机，也不用处理刷新后回到第几步。
- **身份证号在重新提交时可以留空**。接口从不回显完整证件号（只有 `maskedIdCard`），
  强制必填等于让同一个人把 18 位号码重抄一遍，抄错的概率比"少一次校验"更大。
  留空时服务端沿用库里那份；首次申请没有底可沿，仍会拒。
- **提交成功先换令牌再跳工作台**。`role` 与 `expertStatus` 是签在 JWT 里的（`middleware.Auth` 直接读 claims），
  服务端改库不会影响已签发的那一枚。不调 `/auth/refresh` 就进工作台，看到的是「请先提交入驻申请」。
- **工作台的技能与报价由 dashboard 一并下发**。公开详情按专家 ID 取，而本人手里只有 `userID`；
  让前端先去列表里搜自己再取详情，会被分页与筛选坑掉。空表单一旦被保存，等于一次点击清空已有资料。
- **评价区在 `#/experts/:id` 页**（`GET /experts/:id/reviews`，游客可读）。评价与详情是两个接口：
  评价要分页、要「加载更多」，塞进详情会让首屏返回体随评价数增长。汇总（总数/均分/好评率/
  星级分布）跟着第一页一起下发，前端不必再请求一次。
- **公开评价在服务端就完成脱敏**：匿名项的昵称头像换成「匿名用户」，
  `orderId` / `userId` 一概不下发——订单号能反查下单人，用户 ID 能点进个人主页。
  前端渲染再兜一次底：拿到匿名项也照抹（展示层不信任上游）。
- **未过审的专家连评价接口都是 404**，且与「不存在」同一个错误码：
  不然评价接口就成了绕过「审核中不可见」的后门。
- **好评率字段两处都是 0-100 的百分比**，前端不得再乘 100。此前专家主页把 `goodRate`/`resolveRate`
  乘了 100，99.2% 显示成 9920%——字段口径只在服务端定一次，展示端不做二次加工。
- **审核中/被驳回的人能进工作台，但编辑区整块换成说明**。接口本来就要求 `RequireApprovedExpert`，
  少画几个按钮不解决问题——这里如实说明"审核通过后才能改"，而不是给一张必然报错的表单。
- 资质证书存 `key`（私有场景，走签名 URL），头像存 `url`（公开场景）。**两者的引用方式不可互换**：
  证书的 URL 有有效期，存进库隔天就失效；头像的 key 不是可访问地址，`<img src>` 直接裂图。
- 上传走单独一条 `fetch` 通道（`api.uploadFile`）。`FormData` 的 `Content-Type` 必须由浏览器自己带
  `boundary`，套用 JSON 请求头会把请求写坏。

---

## 二·再补充、订单与支付接口一览

订单是两条下单路径（悬赏 / 预约）共用一套状态机，接口按「谁在调」分三层。
想看可执行的版本——每一步的请求体、期望状态码与断言——直接读 `scripts/smoke_order.py`，
它把这条链路完整跑了一遍并逐条说明了在守哪条规则。

| 层 | 接口 | 说明 |
|---|---|---|
| 需求方（登录即可） | `POST /questions` | 发布悬赏，**同一事务里**建出对应订单 |
| | `GET /questions` · `GET /questions/:id` | 我的悬赏列表 / 详情（只有发布者与管理员能看） |
| | `POST /questions/:id/cancel` | 取消悬赏——问题与订单两个状态机一起收口 |
| | `POST /orders` | 预约专家，金额按时薪由**服务端**算 |
| | `GET /orders` · `GET /orders/counts` | 我的订单 / 各状态计数（首页角标用） |
| | `GET /orders/:orderNo` | 订单详情（时间轴 + 会话 + 评价 + 争议 + 分账） |
| | `GET /orders/pay-channels` | 可选支付通道及其 `available` |
| | `POST /orders/:orderNo/pay` | 发起支付 |
| | `POST /orders/:orderNo/cancel` | 取消未支付订单 |
| | `POST /orders/:orderNo/confirm` | 确认完成 → 自动结算 |
| | `POST /orders/:orderNo/review` | 评价（一单一评，唯一索引兜底） |
| | `POST /orders/:orderNo/dispute` | 发起争议 |
| 服务专家（审核通过） | `GET /orders/hall` | 接单大厅（只列未指派且未过期的悬赏） |
| | `POST /orders/:orderNo/grab` | 抢单（CAS，并发只有一个能赢） |
| | `POST /orders/:orderNo/sessions` | 开始服务会话（重复进入复用同一会话） |
| | `POST /orders/:orderNo/sessions/end` | 结束会话 |
| | `POST /orders/:orderNo/finish` | 标记完成，进入待确认 |
| | `POST /orders/:orderNo/review/reply` | 回复评价 |
| | `POST /orders/:orderNo/review/appeal` | 对评价申诉 |
| 后台（admin） | `GET /admin/orders` · `GET /admin/orders/:orderNo` | 订单列表 / 详情 |
| | `POST /admin/orders/:orderNo/mark-paid` | 线下转账人工核账 |
| | `POST /admin/orders/:orderNo/transition` | 强制流转（**仍受状态机约束**） |
| | `POST /admin/orders/:orderNo/settle` | 手动结算 |
| | `POST /admin/disputes/:id/resolve` | 处理争议（判退 / 判完成） |
| | `GET /admin/withdrawals` | 提现列表（按状态 / 单号 / 专家姓名筛） |
| | `POST /admin/withdrawals/:id/review` | 审核（`approve` 通过 / `reject` 驳回并退余额） |
| | `POST /admin/withdrawals/:id/mark-paid` | 标记已打款（扣冻结，**必须填流水号**） |
| | `POST /admin/withdrawals/:id/fail` | 打款失败（退余额，必须写原因） |
| | `GET /admin/users` | 用户列表（手机号**全等**搜索 / 昵称与邮箱模糊 / 角色 / 状态） |
| | `GET /admin/users/:id` | 详情：账号 + 专家档案 + 订单聚合 + 操作留痕 |
| | `POST /admin/users/:id/status` | 封禁 / 解封（`status` 显式传值，封禁必须写原因） |
| | `POST /admin/users/:id/role` | 改角色（升专家要求已通过入驻审核） |
| | `GET /admin/finance/reconciliation` | 对账报表（日表 + 区间合计 + 平台欠付 + 账面校验），默认近 30 天、上限 92 天 |
| | `GET /admin/finance/reconciliation/day/:date` | 单日明细：当天收款 / 结算 / 打款三组 |
| | `GET /admin/finance/reconciliation/export` | 导出 CSV（与页面同一份查询，带 BOM、金额以元） |
| | `GET /admin/settings` | 系统配置总览（当前抽佣比例 + 是否被后台覆盖过） |
| | `PUT /admin/settings/commission` | 改抽佣比例（基点 0-5000；同值幂等；真实变更落审计，带改前/改后值） |
| | `GET /admin/skills` | 技能字典全量（含停用），带引用数 |
| | `POST /admin/skills` · `PUT /admin/skills/:id` · `DELETE /admin/skills/:id` | 新增 / 编辑 / 删除技能（被专家引用时删除返回 409，提示改用停用） |

路径参数统一是 `:orderNo`，**不是数据库主键**（gin 在同一路径位置混用不同名的参数会直接 panic）。
订单号形如 `OE20261015215555756339`，形状不合法在参数层就被拦成 400，不会拿它去查库。
争议单独走 `/admin/disputes/:id/...` 而不是挂在订单下：后台是从「待处理争议」列表点进来的，
手里拿的是争议 ID。

页面侧的四个路由（hash 路由，刷新不依赖服务端回落）：

| 路由 | 页面 | 说明 |
|---|---|---|
| `#/ask` | 发布悬赏 | 标题 / 描述 / 技术方向 / 预算 / 有效期，发布后直接落到订单详情 |
| `#/orders` | 订单中心 | 「我下的单 / 我接的单」两侧切换，按状态分页筛选，Tab 上带角标 |
| `#/orders/:orderNo` | 订单详情 | 状态横幅 + 履约时间轴 + 会话 + 评价 + 争议 + 分账，右侧是当前可做的动作 |
| `#/hall` | 接单大厅 | 已审核专家可见，按技术方向筛选、抢单（并发由服务端 CAS 判定） |

订单状态的中文文案、金额格式、倒计时、以及「这个状态下该显示哪些按钮」都集中在
`web/static/assets/js/orders.js`——它同时是 `internal/model/order.go` 状态机的镜像
（`ORDER_TRANSITIONS`）。服务端只返回英文枚举，**文案只有前端这一份**；
反过来，按钮能不能点最终仍由服务端判定，前端抄一份流转表只是为了把按钮显示对。

---

## 三、目录结构

```
server/
├── cmd/api/main.go              # 入口：加载配置 → 初始化依赖 → 启动 → 优雅退出
├── configs/config.example.yaml  # 配置模板（全部字段都有注释）
├── deploy/
│   ├── docker-compose.yml       # MySQL 8 + Redis 7
│   └── Dockerfile               # 多阶段构建，非 root 运行
├── scripts/
│   ├── smoke.sh                 # 一趟端到端冒烟：知识库 + 订单 + 定时任务，每套件各一个全新库与进程
│   ├── smoke_kb.py              # 知识库五栏目端到端冒烟（178 项断言）
│   ├── smoke_order.py           # 订单全链路端到端冒烟（104 项断言）
│   ├── smoke_sched.py           # 定时任务验证（37 项断言，需短周期启动，只验"时间过去后系统自己做了什么"）
│   ├── smoke_twice.sh           # 连跑两趟，逐套件比对断言总数是否稳定
│   ├── render_check.mjs         # 前端渲染自检（真实接口数据驱动，271 项断言）
│   └── depcheck.js              # ESM 导入符号核对（挡白屏类错误）
├── web/
│   ├── embed.go                 # go:embed 内嵌 + 静态托管（只接管 / 与 /assets/*）
│   └── static/                  # 前端源码：零构建原生 ESM
│       ├── index.html
│       └── assets/
│           ├── app.css          # 单份 CSS，设计 token 集中在 :root
│           └── js/
│               ├── app.js       # 启动、hash 路由
│               ├── flags.js     # 功能开关（入口显示与否）+ 徽章阈值
│               ├── api.js       # 接口封装 + 会话持久化 + 401 静默续期
│               ├── util.js      # 转义、图标、Markdown 渲染、格式化
│               ├── ui.js        # toast / 模态 / 分页 / 空态 / 骨架屏
│               ├── cards.js     # 各栏目卡片
│               ├── orders.js    # 订单域公共知识：状态文案、金额、倒计时、可见动作
│               ├── pages-orders.js   # 订单中心：列表与详情（支付/会话/评价/争议）
│               ├── pages-question.js # 发布悬赏表单
│               ├── pages-hall.js     # 接单大厅（专家抢单）
│               ├── searchbox.js # 顶栏搜索建议（防抖 + 竞态丢弃）
│               ├── pages-admin.js # 运营后台：概览/审核/资讯源/文章/工具/教程/教程集/资源/外链/题库(导入·审核)/专家/提现/订单
│               ├── pages-experts.js # 专家：广场/详情/入驻申请/工作台（服务设置·主页·钱包）
│               ├── pages-contribute.js # 内容投稿：教程 / 题目两个表单 + 我的投稿（两个 tab）
│               ├── pages-qa.js  # 问答广场：列表 / 提问 / 详情（采纳·点赞·关闭）
│               └── pages-*.js   # 首页与各栏目页面（纯函数式渲染）
├── internal/
│   ├── config/                  # 配置加载与校验（YAML + 环境变量双通道）
│   ├── bootstrap/               # 依赖装配：数据库、缓存、种子数据
│   ├── model/                   # 全部 GORM 模型 + 自动迁移
│   ├── repository/              # 数据访问，隔离 GORM 细节（tool_book_repo.go 含目录树的整体替换）
│   ├── service/                 # 业务逻辑
│   │   ├── auth_service.go      # 注册登录、令牌、个人资料
│   │   ├── article_service.go   # 知识库：文章、分类、标签、点赞
│   │   ├── channel_service.go   # 栏目注册表 + 各栏目内容数聚合
│   │   ├── tool_service.go      # 开源工具：录入校验、筛选、详情
│   │   ├── quiz_service.go      # 运维题库：录入校验、抽题、判分、错题本、批量导入、投稿与审核
│   │   ├── tool_book_service.go # 系统教程集：大纲解析、目录树、失效节点过滤
│   │   ├── resource_service.go  # 资源下载：文件实测、签名下发、引用检查
│   │   ├── upload_service.go    # 文件上传与私有文件签名
│   │   ├── expert_service.go    # 专家：申请、审核、工作台、钱包
│   │   ├── order_service.go     # 订单：悬赏/预约、状态流转、结算分账、评价、争议
│   │   ├── qa_service.go        # 问答广场：提问/回答/采纳/点赞、审核分层、贡献积分
│   │   ├── audit.go             # 共享审计写入（内容审核、问答转教程、封禁、抽佣变更）
│   │   ├── content_validate.go  # 内容类通用校验（URL 协议、标签、部署方式白名单）
│   │   ├── payment/             # 支付通道（mock / offline 已通；wechat / alipay 留桩）
│   │   ├── sms/                 # 短信通道（mock / webhook / aliyun / tencent）
│   │   └── wechat/              # 微信授权（mock / official）
│   ├── handler/                 # HTTP 接入层，只做绑定与响应
│   ├── middleware/              # requestid / 访问日志 / panic 恢复 / CORS / 鉴权
│   ├── router/                  # 路由注册（+ 路由树自检，见 router_test.go）
│   ├── scheduler/               # 进程内周期任务（panic 隔离、单任务超时、优雅停机）
│   ├── dto/                     # 出入参结构，敏感字段在此脱敏
│   ├── docs/                    # 内嵌 OpenAPI + Swagger UI
│   └── pkg/                     # 通用能力
│       ├── errcode/             # 统一错误码
│       ├── response/            # 统一响应体
│       ├── jwtx/                # JWT 签发与校验
│       ├── authctx/             # 登录身份在 context 中的读写
│       ├── tokenstore/          # 刷新令牌白名单 + 访问令牌吊销名单
│       ├── cache/               # 缓存抽象（Redis / 内存双实现）
│       ├── validate/            # 手机号、脱敏等基础校验
│       ├── idgen/               # 业务单号与验证码生成
│       ├── hash/                # bcrypt
│       └── logger/              # 基于 log/slog 的结构化日志
└── internal/**/*_test.go        # 单元测试与集成测试（认证、知识库、订单均走真实 SQLite）
```

---

## 四、几个刻意的设计决定

这些决定会影响你后续开发，先说清楚，避免走回头路。

### 1. 金额一律用「分」存整数

所有金额字段是 `int64`，字段名带 `Cents` 后缀。**不要改成浮点数**——佣金按 15% 拆分时浮点会累加出分位误差，最后变成对不上账。

对外接口同样传分，由前端格式化。若某天确实要传元，在 DTO 层转换，不要动模型。

### 2. 订单状态机只认一处

`internal/model/order.go` 的 `orderTransitions` 是唯一权威。任何状态流转必须走 `Order.CanTransitionTo()`，不允许在 handler 或 service 里写 if-else 判断状态。前端 `orderStatusMap` 要与它逐字对齐。（落库时的并发语义见第 16 条。）

### 3. 缓存是可降级的

`redis.addr` 留空会降级为进程内缓存。**这是为本地开发准备的，多实例部署必须配 Redis**——否则登出后令牌吊销、验证码限流都会在实例间失效。启动日志里有明确告警。

### 4. 令牌分两层，刷新即轮换

- 访问令牌 2 小时，无状态校验
- 刷新令牌 7 天，服务端有白名单，**用过一次立即失效**（防重放）

前端并发请求时要注意：刷新接口要做单飞（single-flight），否则两个并发刷新会有一个失败。

### 5. 手机号字段是 `*string` 而不是 `string`

因为唯一索引下 MySQL 和 SQLite 都允许多个 `NULL`，却不允许多个空串。微信可以不带手机号建号，所以必须可空。取明文统一用 `user.PhoneValue()`。

### 6. 微信 / 短信通道都是可插拔的

短信：`mock`（开发）/ `webhook`（转发到你自己的网关）/ `aliyun` / `tencent`。
- `webhook` 用 HTTP 转发，请求体格式见 `internal/service/sms/webhook.go` 顶部注释——**如果你已有内部短信网关，这是最省事的接法**。
- `aliyun` / `tencent` **目前是骨架，未实现发送逻辑**。原因写在 `internal/service/sms/providers.go` 里：厂商签名算法凭记忆手写极易在线上才暴露错误，建议引入官方 SDK 后再实现。生产环境配置校验会拒绝 `mock`。

微信：`mock` / `official`。`official` 走开放平台扫码登录，端点地址与 scope 已按公开流程实现，**上线前请对照当前版本官方文档复核一次**。

### 7. 知识库的公开读与后台写是两条独立路径

前台 `/api/v1/kb/*` 用 `OptionalAuth`：游客能读是 SEO 获客的前提，但带令牌时要能识别"这篇我点过赞"，所以是**有条件地**解析身份，而不是简单放行或强制登录。

后台 `/api/v1/admin/kb/*` 走 `Auth + RequireRole(admin)`。两条路径共用同一个 `ArticleService`，区别只在调用哪些方法——**不要**为了让前台能读而在 service 里塞 `if role == admin` 的分支，那会让越权判定散布到业务逻辑里。

### 8. 点赞用关联表 + 唯一索引，且**不做软删除**

`article_likes` 上有 `(user_id, article_id)` 唯一索引，写入用 `ON CONFLICT DO NOTHING`。重复点赞在**数据库层**就失败，应用层不需要加锁；只有真的插入了新行才给 `like_count` 加一，从根上堵死"刷新页面刷点赞数"。

**这张表刻意不嵌入 `model.Base`**，因此没有 `deleted_at`。取消点赞是物理删除——如果带上软删除，被删的行仍占着唯一索引，用户第二次点赞时 `ON CONFLICT DO NOTHING` 会静默失败，于是出现"接口返回已点赞、详情页却是未点赞"的状态不一致。这个问题只有在"点赞 → 取消 → 再点赞"三步连做时才暴露，单测和冒烟都必须覆盖到第三步。

### 9. 文章 slug 自动生成并避让

未指定 slug 时由标题推导，规则是「字母数字与汉字保留，其余折叠为连字符，大写转小写」。汉字保留是刻意的——现代浏览器会做百分号编码，可读性远好于 pinyin 转换或随机串。撞名时依次追加 `-2`、`-3`，超过 20 次挂随机后缀，保证写入不会因唯一索引失败。

### 10. 浏览数与点赞数走 SQL 表达式自增

`view_count = view_count + 1` 而不是「读出来 +1 再写回」。后者在并发下必然丢计数。点赞数自减带 `CASE WHEN ... < 0 THEN 0` 钳制，防止并发取消时出现负数。

### 11. 栏目是代码内的注册表，不是数据库表

`internal/model/channel.go` 里的 `AllChannels()` 是栏目清单的唯一来源。栏目承载的是**产品结构**——增减栏目意味着前端路由、内容模型、后台菜单都要跟着动，这是一次代码发布，不该让后台运营随手改。反过来，分类、标签这类日常运营频繁调整的维度才落库。

写路径（`resolveArticleChannel`）空值兜底为 `linux-tutorial`，读路径（`resolveArticleChannelFilter`）空值表示**不限栏目**。同一个字段在两个方向上语义不同是刻意的：前者决定内容归属，后者决定查询范围。

### 12. 题库判分：多选是集合相等，不给部分分

漏选意味着操作规程不完整——运维现场少执行一步就是事故，所以不给"答对一半"的分。

`AnswerCount` / `CorrectCount` 只在**首次作答**时累加（`Fresh` 标记），重复刷同一道题不会把正确率刷上去。同理 `/quiz/stats` 的 `totalAnswered` 是去重后的题数，不是作答次数。

判断题的选项固定是「正确 / 错误」，**由服务端填充，调用方可以整个省略 `options`**。让运营自己填会让同一个意思出现"对/错""是/否""True/False"三种写法，统计与复习都会乱。

> 这里踩过一次坑：DTO 上原来写的是 `binding:"required,min=2"`，空切片会在校验层被提前拒掉，
> 于是 service 里"判断题允许 0 个选项"那条分支永远走不到，判断题非得手动传两个选项过去。
> 现在改成 `omitempty,min=2,max=8`，两条路径才对得上。

### 13. 错题本的口径是「最近一次作答仍然答错」

实现上先按题取 `MAX(id)` 拿到最近一条流水，再筛其中 `correct = false` 的。如果按「曾经答错过」来算，用户改对之后错题本里还会一直挂着，复习清单就不再可信。

### 14. 资源文件是私有存储，大小与校验和一律实测

资源文件走 `SceneResourceFile`（私有场景），下载地址由服务端现场签发 HMAC 签名，不是公开直链。

`fileSize` 与 `checksum` **不采信客户端传值**，一律从落盘内容流式计算 SHA256（`Inspect`，不把文件读进内存，上限 50MB）。上传时还强制 `HasPrefix(key, scene)`，防止用资质证书之类的私有 key 冒充公开资源。

删除资源时先查该文件是否被其它资源引用（`ExtExists`），确认无引用才删物理文件——否则多个资源指向同一份文件时，删一个会把其余的一起弄成死链。

### 15. 检索要迁就用户的说法，不是迁就库里的写法

用户搜的是「磁盘满了怎么办」，库里存的是「容器磁盘空间告警排查」。拿整句去 `LIKE`，结果必然是空页——
**这不是用户搜错了，是检索没做适配**。所以关键词进来先过一层查询理解（`search_query.go`），
再用展开后的词去匹配。

落库时有三个刻意的选择：

- **组内 OR、组间先 AND 后 OR。** 同一概念的不同说法取 OR；不同概念先用 AND 试一次，
  命中说明用户每个词都出现了，那基本就是他要找的；一个都没有才退化为 OR。
  只走 AND 会让大白话永远空页，只走 OR 会把只沾一个词的无关内容全捞进来。
- **排序按命中位置加权。** 标题命中算 3 分、摘要 2 分、正文与标签 1 分，命中越核心排越前。
  没有这层的话，正文里顺带提一句「重启」的文章会和标题就是《Pod 频繁重启排查手册》的并列。
  之所以不用分数列而用 `ORDER BY (CASE ...)` 表达式：前者会让 `Count` 与 `Find` 的列对不齐，
  后者在 MySQL 与 SQLite 上语义一致，不需要按方言分叉。
- **同义词词典就是分词词表。** 一张 `map[string][]string` 同时承担展开与汉字切分，
  所以补词时顺手把常见别名写进去，两者一起变准。全角转半角也是必需的——
  中文输入法下打出的「Ｋ８Ｓ」不归一化，和库里的 `k8s` 永远匹配不上。
- **中英混排先按文字系统切段。** `cpu占用高` 整串当汉字查词典时英文前缀会被静默跳过——
  `cpu` 明明是个有效概念却提不出来，用户搜它只能拿到零结果；整串按英文处理又会把中文全丢掉。
- **解析不出词时回落整串，而不是不过滤。** 一句全是语气词、或超过长度上限的输入会让检索条件为空，
  于是静默返回全站内容。回落成整串匹配之后得到的是零结果，语义上才是对的。

已知边界：这是基于 `LIKE` 的词面匹配，**不做语义相似**。用户问「怎么防止数据丢失」，
而文章标题是「备份策略」，两者词面不重叠，仍然搜不到。真要解决得上向量检索，
不是往这张词典里继续加词能办到的。

### 16. 每次状态流转都是 CAS，后台也不例外

第 2 条说的是"订单状态机只认一处"，那只解决了**能不能转**，没解决**谁转成了**。
落库不是"读出来判断再写回"，而是

```sql
UPDATE orders SET status = ? WHERE id = ? AND status = ?   -- 末尾这个是流转前的状态
```

靠 `RowsAffected` 判断谁赢了。两个人同时抢同一单、两个并发的支付请求，都只有一个能成功，
另一个拿到 `ErrOrderTaken` / `ErrOrderStateInvalid`，而不是把 `accepted_at`、`paid_at` 覆盖一遍。

**后台的「强制流转」也走同一条路。** 管理员需要的是"处理完争议后把订单推进到下一状态"，
而不是"随便改成已完成"——放开状态机等于把订单数据变成可以手改的文本，之后所有基于状态的统计
都不再可信。

状态只以英文枚举返回（`pending_accept` / `in_service` / …），中文标签由前端 `orderStatusMap` 映射。
服务端一旦也开始返回中文，两边文案就会各改各的。

### 17. 抽佣比例存订单快照，结算不看当前配置

`orders.commission_rate_bp` 是**下单那一刻**的快照，单位是基点（`1500` = 15%）。
把配置改成 50% 也不会动到已成交订单的分账——否则改一次配置就等于把历史账目重算一遍。

`commission_cents + expert_amount_cents == amount_cents` 是硬恒等式，不平就拒绝结算。
测试里专门有一条把配置改成 50% 再走结算，确认它取的是快照而不是当前值。

### 18. 支付通道：不认识是 400，认识但没接通是 501，**没有静默降级**

- `IsKnownChannel` 为假 → 400：前端传错了，属于参数问题
- 认识但 `IsAvailableChannel` 为假 → 501 `ErrPayChannelUnavailable`：是"这个能力还没上"，不是"你传错了"
- 可用 → 真正切到这个 provider 再发起支付

> 这里踩过一个真 bug：第一版写成"认识的通道才切 provider"，于是选了未接通的微信支付会
> **静默落到默认的 mock 通道**上——用户以为在用微信，订单却被 mock 立刻置成了已支付。
> 现在 `channel != 默认通道` 时**无条件** `payment.New`，失败即报错。冒烟里两条断言
> （未知通道 400 / 未接通通道 501）钉的就是这个行为。

`GET /orders/pay-channels` 返回每个通道的 `available`，前端只渲染真正能用的那几个。
线下通道（`offline`）是 `Manual` 的：只记下用户选了它，订单仍留在待支付等后台核账——
**绝不能顺手置成已支付**，钱还没到。

### 19. 唯一索引当规则防线，而不是靠应用层查一次

`settlements.order_id` 与 `reviews.order_id` 都是唯一索引。应用层先查一次是为了给出友好提示，
**真正兜底的是索引**：重复结算、重复评价在数据库层就失败，`isDuplicateKeyError` 认出双方言
（SQLite 的 `UNIQUE constraint failed` / MySQL 的 1062）后翻译成业务码。

`settlements` 同时也是"钱动过"的唯一凭证——用户确认、后台强制流转、定时兜底扫描三条路
都只有它能打钱，重复走任何一条都不会重复入账。

### 20. 订单号形状只有一个来源

`idgen.ValidOrderNo` 定义形状：`OE` + 14 位时间 + 6 位随机。路由里的路径参数统一叫 `:orderNo`
（gin 混用不同名的路径参数会直接 panic），进 handler 先过形状校验——`GET /orders/123`
在参数层就被拦成 400，不会拿一个明显不是订单号的字符串去查库。

对齐它的还有两件事：`OE` 开头让日志里一眼能认出订单号；时间前缀让同一秒内的订单按号排序
基本等于按创建时间排序，排查问题时不用再关联时间字段。

### 21. 定时任务：宁可启动失败，也不要跑一个节奏不对的任务

`internal/scheduler` 里只有三个任务，但每一个都关乎「钱」或「接单名额」：

| 任务 | 不做会怎样 |
|---|---|
| `cancel-expired-payments` | 待支付订单一直挂着，专家的接单名额被占住，这条悬赏再没人看得见 |
| `auto-confirm-expired` | 用户忘了点确认，专家干了活永远拿不到钱 |
| `settle-completed` | 确认成功但结算那步失败的单子永远停在已完成，钱卡在平台账上 |

几条刻意的选择：

**不引第三方 cron 库。** 只有三个固定任务，不需要表达式解析；而多数 cron 实现自带
「停机期间错过的补跑」语义，对这里的任务反而是负担——补跑十次和跑一次的结果一样，
因为数据已经被最新一轮处理过了。

**每个任务一个 goroutine。** 共享一个循环的话，任何一次慢查询都会让另外两个跟着延后。
分开之后，关单被大批积压拖慢时，自动确认与结算兜底照常按自己的节奏跑。

**panic 就地 recover 并打完整堆栈。** 后台 goroutine 里逃出去的 panic 会直接终结进程，
而它发生时没有请求上下文、没有人在看屏幕——一次数据异常就能把线上服务打挂。

**不给积压的 tick 排队。** `time.Ticker` 在消费者跟不上时只保留最近一个 tick，其余丢弃，
这正是需要的语义：一轮花了 5 分钟，排队补跑 10 次没有意义，跑最新状态就行。真要排队，
进程只会越跑越滞后。

**配置超范围直接启动失败**，不做「超范围就退回默认值」。`interval` 写成 `30h`（本意 30s）
时静默改回 1m 是解决了问题；但写成 `30m` 不违反任何下限，超时单就会在库里多躺半小时，
而这种偏差从外部完全看不出来。

**多副本同时跑是安全的，但这不是调度器的功劳。** 调度器不做分布式锁、不选主——
三个任务在订单侧都幂等：状态流转是 CAS（`UPDATE ... WHERE status = from`），
结算与评价分别有 `settlements.order_id` / `reviews.order_id` 唯一索引兜底。
两个副本同时扫到同一笔超时单，只有一个能关掉。**将来加不幂等的任务，必须先补分布式锁
再注册进来**，不能靠「应该只有一个副本」来保证。

配套的验证方式是 `scripts/smoke_sched.py`：它的断言全是「时间过去之后系统自己做了什么」，
所以必须以 `SCHEDULER_INTERVAL=1s` / `PAYMENT_PAY_TIMEOUT=4s` / `PAYMENT_AUTO_CONFIRM_AFTER=6s`
启动服务，`smoke.sh` 里已经固定了这组变量。里面最要紧的一条断言是
「自动确认后专家钱包余额恰好增加 `expertAmountCents`」——这是整条链路唯一不可出错的地方。

---

### 22. 投稿落库固定 pending，请求体里没有 status

「有账号就能发」和「谁能发布」是两件事。任何登录用户都可以投，但状态由服务端定死为 `pending`，
审核通过才变 `published`。请求体里只要出现 status，客户端就掌握了发布权限——那不需要破解任何东西，
改一行请求体就能把软文推上首页。

同一套状态也用在抓取来的资讯上（`draft/pending/published/rejected/offline` 五态共用），
这样后台的「待审内容」是一条流水线，而不是四类内容各写一套状态名。

### 23. 栏目 slug 是外部契约，展示名不是

`linux-tutorial` 这个 slug 在栏目内容扩到「不只是 Linux」之后仍然保留，只把 `Name` 改成
「运维实战教程」。原因很实际：slug 已经进了外链、收藏、前端路由与别人的文档，改一次全部 404，
而用户看到的只有展示名。同理分类改名走 `LegacySlug` 原地改（`k8s` → `cloud-native`），
保住 `category_id` 上的关联，而不是删旧建新。

### 24. 关联表用「整体替换」，不提供逐条增删

工具的四类教程、资源的外链都是「某个工具/资源的配置」，不是独立的业务实体。前端一次性提交
完整清单，服务端在同一个事务里先删后插：不会有「删了一半、另一半还挂着」的中间态，
也不用为「第 3 条改成第 2 条」设计移动接口。上限（教程 24 条、外链 8 条）本身就是设计的一部分
——一个工具挂 200 条教程时，页面已经不是「推荐」而是垃圾场了。

### 25. `[hidden]` 必须在样式表里有兜底规则

浏览器默认的 `[hidden] { display: none }` 来自 UA 样式表，**任何一条类选择器的 `display` 都能压过它**。
后台教程行用 `hidden` 切换「站内文章 / 外部链接」，而行内每个字段都是 `.field { display: flex }`——
于是两侧同时显示，五列栅格多出一列、整行被挤到第二行。字符串断言全绿，因为 HTML 里 `hidden` 属性
确实在，问题只在"它没生效"。

样式表顶部因此有一条 `[hidden] { display: none !important; }`。同理，凡是用属性控制显隐的地方，
不要再依赖 UA 默认值；要按样式显隐就用类，要按属性显隐就保证这条兜底规则在。

### 26. 身份以库为准，令牌只负责证明「你是谁」

`role` 与 `expertStatus` 都还在 JWT 的 claims 里，但 `middleware.Auth` **不再信它们**：
它按 `uid` 查一次库（`UserRepo.FindAccount`，一次 LEFT JOIN 拿到账号与专家审核状态），
用库里的值写上下文。

早期版本是直接读 claims 的，理由是"省一次查询"。代价有两个，而且都会真的发生：

- **封禁无效**。把 `users.status` 改成 `disabled` 之后，对方手里那枚令牌还能用到自然过期
  （访问令牌 2 小时）。被封的人继续下单、继续提现，等于封了个寂寞。
- **改角色要催用户重登**。审核刚通过的专家刷新页面还是进不了工作台，得先调 `/auth/refresh`。
  运营每天都要解释一遍"您重新登录一下"。

所以现在的口径是：**令牌只证明"你是谁"，权限一律现查**。多出来的这一次主键查询，
对这套系统的量级（低 QPS、SQLite 本地库）可以忽略，换来的是"改完立刻生效"。

由此带来两个变化，写代码时要记住：

- 不需要再为了"让新角色生效"去调 `/auth/refresh`；提交入驻申请后直接刷新页面就行。
- 反过来，**任何绕过接口直接改库的提权都会立刻生效**。开发时改库提权很方便，
  生产上这意味着数据库权限本身就是最高权限，别再额外给。

`OptionalAuth` 仍然不查库（游客页面只需认一下"我点过赞没有"），所有写接口都在 `Auth` 后面。

### 27. 重新申请可以沿用它已经提交过的证件号

入驻接口的出参里只有 `maskedIdCard`（前 6 + 后 4），完整号码永不下发。于是"被驳回后重新提交"
就出现一个别扭的点：`idCardNo` 在请求体里是必填，而用户手上已经看不到上次填的号码。

让同一个人的 18 位号码重抄一遍，抄错的概率比"少一次服务端校验"更大，而且会让"改一个字重新提交"
变成"从零再填一遍证件"。所以 `idCardNo` 在 DTO 里去掉了 `binding:"required"`，
由 service 按**库里有没有老记录**判定：有就沿用，没有才报错。填了仍然要过 MOD 11-2 校验位，
不是"可以随便传空"。

### 28. 提现审批拆成「通过」与「已打款」两步，钱的动作为状态让路

`pending → approved → paid`，驳回与打款失败是终态。状态机在 `model/finance.go` 的
`withdrawalTransitions`，流转一律走 CAS（`WHERE id = ? AND status = ?`），与其他状态机同一套。

刻意的三点：

- **「通过」不动钱**。只是从待审核挪到待打款，钱仍冻在 `frozen_cents` 里。做成「通过即打款」，
  等于把核对收款人的步骤省掉——钱一旦打出去就收不回来了。
- **先改状态，再动钱**。驳回要退冻结、打款要扣冻结，都是「CAS 成功之后」才执行。
  顺序反过来的话，两个人同时点驳回会退两次；而 CAS 失败的请求根本走不到动钱那一步。
  动钱失败时状态已经变了，这里选择「日志留足线索 + 返回可读错误」，宁可让金额暂时悬着等人工核对，
  也不能出现「状态还是待审核、钱已经退回余额」。
- **打款流水号必填**。不填就无法对账，事后只剩「已打款」三个字，查不出打给了谁、什么时候打的。

### 29. 封禁改账号，不动档案；三类误操作必须挡住

封禁只写 `users.status`，专家档案、订单、余额、提现记录一概不碰。解封后一切照旧——
这比"封禁时顺手把专家停牌"更好收拾：反向操作只有一步，不会出现"解封了但专家还是停牌"的半截状态。

前台专家列表因此要显式过滤 `users.status = 'active'`（`ExpertFilter.OnlyActiveUser`），
后台审核队列则**不能**过滤——被封禁者那份仍待审核的申请得让人看见，否则运营会以为它丢了。

三道闸：

- **不能对自己下手**（封禁 / 改角色）。管理员把自己封了或降级，后台就再没有入口，
  只能改数据库救回来。
- **升成专家必须已有审核通过的档案**。此前 `RequireApprovedExpert` 对空的 `expertStatus`
  一律放行，手工把角色改成 `expert` 就能直接进接单列表——这条捷径已经堵掉：
  空值只对管理员放行。
- **封禁必须写原因**，解封则把原因清空。原因存在 `users.status_reason`，
  历史留在 `audit_logs`（`audit_logs` 表建了很久，这是第一个真正往里写的动作）。
  不写原因的话，半年后有人申诉，客服只看到 `disabled` 两个字，结果往往是先解封再说。

最后「至少保留一个可用管理员」的判断（`CountActiveAdmins`）在正常路径上触发不到：
操作者本身就是一个可用管理员，目标也是，数量至少是 2。它是给并发留的兜底——
A 正在降 B 的时候，C 刚好把 A 降了。要做到严格互斥得在事务里锁 `users`，
对内部后台来说不值得，剩下的交给"另一个管理员用命令行改回来"这条运维预案。

### 30. 对账按四个时间口径分开算，别合成一个净额

一张订单会经过四个时点，每个时点都是一类财务事件：

| 事件 | 归日依据 | 钱在哪 |
|---|---|---|
| 收款 | `orders.paid_at` | 进平台账户，其中专家那部分只是**代管** |
| 退款 | `orders.refunded_at` | 从平台账户退出去 |
| 结算 | `settlements.settled_at` | 从代管挪进专家余额，**现金没动**，平台此时确认佣金 |
| 打款 | `withdrawals.paid_at` | 真正离开平台 |

把四件事合成"当日利润"必然出错：1 号收款、5 号退款时，退款算在 1 号会让 1 号虚高、5 号看不出少了钱；
而结算那天平台一分现金都没进，却常被当成"今天赚了佣金"。所以仓储里是**四段独立聚合**，
在 Go 里按日期拼成一行的四组列，页面上也只给 `净现金 = 收款 − 退款 − 打款` 这一个合成指标。

**按日聚合不能用 `DATE()` 统一处理。** SQLite 的 `date()` 会先把值转成 UTC 再取日期，而库里存的是带
偏移的本地时间：`date('2026-10-15 00:30:00+08:00')` 实测返回 `2026-10-14`——早 8 点前的每一笔钱都会
被算到前一天，日报表与银行流水永远差一截。SQLite 侧改成 `substr(col, 1, 10)`，取到的正是写入时的本地日期；
MySQL 的 `datetime` 不带时区，`DATE()` 才是对的。见 `FinanceReportRepo.dayExpr`。

**累计与按日是两个口径，不能共用一个条件。** `refunded_at` 是后加的字段，之前退掉的单子补不出真实时间：
按日统计进不去（归不到哪一天），但"一共退过多少钱"必须准，所以 `Totals` 按 `refund_cents > 0` 统计。
这件事在页面上作为 `warn` 明确列出，不静默漏掉——财务看到"这个月退款少了"会去找一笔不存在的差错。
历史数据能定位到具体日期的话可以回填：

```sql
-- 仅在能确认退款时间时执行；updated_at 对已终态的退款单通常就是退款那一刻
UPDATE orders SET refunded_at = updated_at
WHERE refunded_at IS NULL AND refund_cents > 0;
```

**校验是恒等式，不是提醒。** 七条检查里六条是硬的：佣金 + 专家实收 = 总额、实际到账 = 申请 − 手续费、
累计收入 = 可提余额 + 冻结 + 已打款、已结算订单必须有分账、进入服务必须已收款。
不平就意味着某条写路径只改了一半，`level=error` 直接标红，不给灰提示——灰的那条没人看。
唯一降级成 `warn` 的是"历史退款缺时间"，它破坏的是可归日性，不是金额守恒。

顺带修掉一个**演示数据污染账务**的问题：种子里 `BalanceCents` 与 `TotalIncomeCents` 原本是两个不相干的
数字（差额代表"历史上提过现"），却没有对应的提现流水。结果每个新库第一次打开对账页，三位专家全部报红，
红得毫无意义，真出问题时反而没人看。现在两者相等——种子不造流水，就不该声称提过钱。
**存量库**（包括 QA 机上的 test 档）要手工对齐一次，条件写成恒等式本身，跑几遍都安全：

```sql
UPDATE experts SET balance_cents = total_income_cents
WHERE total_income_cents <> balance_cents + frozen_cents + (
  SELECT COALESCE(SUM(w.amount_cents), 0) FROM withdrawals w
  WHERE w.expert_id = experts.user_id AND w.status = 'paid' AND w.deleted_at IS NULL
);
```

### 31. 抽佣比例放数据库，下单时实时查、不加缓存

抽佣比例在 `system_settings` 键值表里（`order.commission_rate_bp`），**下单那一刻读一次库并写进订单快照**，
之后改比例只影响新订单——已下单的按各自快照结算，绝不被追溯改账（这条与第 9 条「结算取快照」是同一件事的两端）。

三个刻意的取舍：

- **不缓存**。订单创建是低频操作，一次主键查询的成本可以忽略；缓存（哪怕 60 秒）意味着"改完比例，
  60 秒窗口内下的单还在按旧比例算钱"，多副本部署时各节点缓存还会不一致。
- **查库失败要让下单失败**。错一分钱对账时都要人工核，代价远高于让用户重试一次；
  但库里值本身坏了（手工改库改出非数字）记 error 后回落配置文件默认值——库是好的，
  只是那一行脏了，不值得让全站下单陪葬。
- **同值修改不写库也不留审计**。否则审计里很快堆满"点了但没变"的噪声，真正的变更被淹没。
  真实变更的审计带 `from` / `to`——只记终值的话，回看时不知道改之前是多少。

上限 5000 基点与 `config.Validate` 同口径：超过 50% 基本是把基点当百分比填了。

### 32. 技能字典：删除是例外，停用才是常态

技能被专家档案引用时**不能删**（409，提示改用停用）——删掉后专家页与搜索里会出现悬空 ID。
停用是无损的：入驻下拉框里消失，已有档案照常展示。真要删除时是**物理删除**（Unscoped）：
`skills.name` 上有唯一索引，软删的行会一直占着名字，同名重建会静默撞索引——
与第 8 条 ArticleLike 是同一类坑。

### 33. 免费问答与付费悬赏是两套表，绝不合并

`Question`（悬赏）是**订单的前置单据**：有钱、有接单、有状态机、有退款与纠纷。
`QaQuestion`（问答广场）是**社区帖**：没有金额字段，也不该有。

想合并的冲动很容易理解——"都是提问，为什么两张表"。但一旦问答可以悬赏，就等于在订单链路
之外长出第二条要结算、要退款、要处理纠纷的支线，而这条支线的每一分钱都得像订单一样对账。
反过来把悬赏降级成问答也不行：接了单的专家要有履约凭据。所以两套表、各自的状态机、互不引用。

由此带来的两条具体约定：

- 问答的 `status` 只有 `open / resolved / closed` 三态且基本单向，**刻意不设「回答中 / 待确认」**
  这类中间态。社区问答的价值在于沉淀，状态机越长，帖子越容易卡在没人处理的一格里。
- 问答**不做浏览量去重、不做热度加权**，排序就是 `last_answer_at` 倒序：新回答把帖子顶上来。
  没有这个字段，新帖子发出来五分钟就沉底，提问人拿不到回答就再也不来了。

### 34. 回答审核分层：新号先审，专家与管理员直发

社区问答最稀缺的不是内容，是**有人愿意回答**。一律先审会把这件事从"当场看到"拖成"隔天见"，
问答区在第一天就死了；一律不审则等于给灌水机器人开门。

所以按人分层：普通账号前 3 条回答进 `pending`（只有作者与管理员看得见），
管理员与**已过审专家**免审直发。阈值取 3 而不是 10，是因为真实用户前三条几乎都是认真答的，
而批量灌水的机器人不会只发三条。

专家的判定要求 `role=expert` **且** `expertStatus=approved`。只检查角色是不够的：
历史上存在"手工把 `users.role` 改成 `expert` 但没有任何档案"的脏值，那等于绕过了入驻审核直接拿到免审权。
这条与第 29 条里 `RequireApprovedExpert` 对空 `expertStatus` 的处理是同一个教训。

被判定的逻辑收在 `QaViewer.CanPublishDirectly()` 一处，投稿（`Contribute` 的 `directPublish` 参数）
也复用它——同一条规则不该有第二份实现。

### 35. 收起一个入口用开关，不要删路由

「专家广场」暂时不放在导航上，但 `#/experts` 与 `#/experts/:id` 两条路由**全部保留**，
由 `flags.js` 的 `FLAGS.expertPlaza` 控制导航项是否渲染。列表页另加一条「正在整理中」提示条。

原因很实际：专家主页的链接已经出现在订单详情、问答回答的专家角标、用户收藏里。
删掉路由，这些链接会一起 404，而"找不到页面"的用户不会区分"功能下线"和"网站坏了"。
开关只控制**入口是否出现**，不控制**页面是否存在**——收起与下线是两件事。

`service: false` 用的是同一套机制：收起「悬赏 / 订单 / 接单大厅」这组导航项，
`#/orders`、`#/orders/:orderNo`、`#/hall` 的路由与页面一个都没删。

注意**所有静态入口**都要过一遍开关：`index.html` 的页脚是纯静态标记，没法走模板条件，
启动时由 `app.js` 按 `FEATURES.expertPlaza` 摘掉对应链接与其分隔符（`.hidden` 类）。
第一版只收了导航、漏了页脚，是上线截图肉眼复核时发现的——收起入口这类需求，
验收标准应该是「整站每个角落都看不到它」，而不是「导航栏里没有它」。

### 36. Markdown 列表要吞掉「项与项之间的空行」

自研的 `renderMarkdown` 按行解析，最初「连续同类项合成一个 `<ol>`」的收集器一遇到空行就停——
而正常人写步骤就是每条之间空一行的：

```
1. 第一步

2. 第二步
```

按严格 Markdown 这算两个列表，各从 1 开始，页面上于是全是「1.」——内容是对的，
编号是错的，而且只在这种写法下出现，肉眼抽查很难碰到。收集器因此放宽为**宽松列表**：
空行后面如果还是**同类**标记（编号对编号、圆点对圆点）就继续收集；
无序列表后面跟编号行不会被吞成圆点（那会把作者的编号信息抹掉）。

教训与第 25 条同源：自研渲染器没有现成测试集兜底，**每个块级语法都要有一段真实样张**，
且验证必须走真实浏览器截图——字符串断言只看 `<ol>` 在不在，看不出编号错了。

### 37. 删问题要连回答一起软删，统计要 JOIN 活着的问题

删问题最初只软删 `qa_questions` 一行，回答留在表里成了**孤儿**：任何页面都走不到它
（详情 404、列表不可达），但它仍然是 `status=published`，于是——

- 问答列表顶部的「已回答」总数永远比看得见的回答多一条；
- 后台待审队列里会冒出标题为空、点进去 404 的行；
- `CountPublishedByUser` 虚高，用户删帖删得越多，后面本该进审的回答越容易直接免审发布。

三处同源，改法分两层：`DeleteQuestion` 在**一个事务里**先物理删点赞行、再软删回答、
最后软删问题（`QaAnswerLike` 是纯关系表，不物理删就会一直占着 `answer_id`）；
读侧再统一加 `liveAnswerJoin`（`JOIN qa_questions q ON q.id = ... AND q.deleted_at IS NULL`）
兜住历史孤儿。**写侧修的是根因，读侧加的是约束**——只做其中一半，线上那条脏数据还会继续显示。

这个 bug 是靠「删掉测试帖之后总数从 3 变 4」发现的，不是靠单测。
现在有 `TestQaDeleteQuestionCascadesAnswers` 锁住四条断言。

### 38. 批量导入是「全量校验、一次落库」，投稿固定 `pending`

题库的批量导入有两条路可以选：边校验边插（快的行先进去），或全量校验完再一次性插。
选了后者：**任一行有问题则一条都不写**，错误带行号一次给全。
半批入库是最难收拾的状态——用户不知道哪几条进去了，再传怕重复，而题库没有天然唯一键兜底。

为了让 200 行不查 200 次分类表，把单题录入的字段加工拆成了纯函数 `prepareQuizFields`，
导入时先取一次分类索引，再对每行调纯函数。

题目投稿与后台录题的差异被压到两个字段上：

- **请求体里没有 `status`**——让客户端传状态等于把发布权限交给浏览器，投稿固定 `pending`；
- **解析必填**——后台补历史题允许留空，投稿不允许：没有解析的题，答错的人不知道错在哪。

免审判定复用问答的 `CanPublishDirectly()`（认证专家/管理员直发并计 5 贡献分），
判定仍然只在一处。审核走 CAS（`WHERE id=? AND status='pending'`），通过加分用
`UniqKey = quiz.publish:<题目ID>` 保证重复审核不会重复加分。

### 39. 教程集只做目录组织，正文复用 articles

`ToolBook`/`ToolBookChapter` 两级封顶（不做无限层级：目录渲染、排序、面包屑三处同时变复杂）。
正文就是 articles 里的文章——编辑器、图片上传、审核流、SEO、版本回滚全都在那套里，
把正文搬进教程集等于再造一个立刻落后的编辑器。

三个随之而来的决定：

- **章节用大纲文本整体替换**（`# 章` / `## 节 | slug或外链 | 导语`），不做拖拽编辑器：
  运营的真实大纲本来写在文档里，可整段粘贴；整体替换天然避免「删了章、它的节还挂着
  指向空气的 parent_id」。子节点需要父节点自增 ID 的问题由 `ReplaceChapterTree`
  在一个事务里解决（先清、逐章插入拿 ID、子节点挂父 ID 再插）。
- **失效节点前台跳过、后台标红**：前台挂一个 404 链接比不展示更伤；后台看不到就改不了。
  前台章自己没正文且子节点全失效时，整章不展示。
- **章节表不嵌 `Base`（不软删除）**：目录是整体替换的结构，软删会让被删节点永远占行
  （与 `ToolGuide`/点赞表同一条纪律）。

大纲里写的引用是**文章 slug 不是 ID**——ID 在后台任何页面都不显示，
照着 ID 抄只会抄错。找不到的 slug 保留标题去掉链接并记入 `warnings`，不阻断保存。

## 五、配置

全部配置项见 `configs/config.example.yaml`，每一项都有中文注释。优先级：

```
内置默认值  <  configs/config.yaml  <  环境变量 / .env
```

常用环境变量：

| 变量 | 说明 | 默认 |
|---|---|---|
| `APP_ENV` | `dev` / `test` / `prod` | `dev` |
| `APP_ADDR` | 监听地址 | `:8080` |
| `DB_DRIVER` | `sqlite` / `mysql` | `sqlite` |
| `DB_DSN` | MySQL 连接串 | — |
| `DB_SQLITE_PATH` | SQLite 文件路径 | `./data/ops_expert.db` |
| `REDIS_ADDR` | 留空则降级内存缓存 | 空 |
| `JWT_SECRET` | ≥32 字节 | 开发默认值 |
| `JWT_ACCESS_TTL` | 访问令牌有效期 | `2h` |
| `JWT_REFRESH_TTL` | 刷新令牌有效期 | `168h` |
| `SMS_PROVIDER` | `mock` / `webhook` / `aliyun` / `tencent` | `mock` |
| `SMS_EXPOSE_CODE` | 是否回传验证码（**生产强制 false**） | `false` |
| `WECHAT_MOCK` | 微信 mock 开关（**生产强制 false**） | `true` |
| `SCHEDULER_ENABLED` | 是否在本进程跑定时任务 | `true` |
| `SCHEDULER_INTERVAL` | 扫描周期（1s–24h，超范围**启动失败**） | `1m` |
| `SCHEDULER_BATCH_SIZE` | 单轮每任务处理上限（1–1000） | `100` |
| `SCHEDULER_JOB_TIMEOUT` | 单个任务执行超时（1s–10m） | `30s` |
| `SCHEDULER_RUN_ON_START` | 启动时立刻跑一轮 | `true` |
| `PAYMENT_PAY_TIMEOUT` | 待支付有效期，超时由定时任务关单 | `15m` |
| `PAYMENT_AUTO_CONFIRM_AFTER` | 完成后多久自动确认 | `48h` |
| `PAYMENT_COMMISSION_RATE_BP` | 抽佣基点（1500 = 15%） | `1500` |

**生产环境的硬性校验**（`config.Validate` 会直接拒绝启动）：
- `jwt.secret` 必须替换且 ≥32 字节
- `sms.expose_code` 必须为 false
- `sms.provider` 不能是 `mock`
- `wechat.mock` 必须为 false

---

## 六、测试

```bash
cd server
go test ./... -count=1
```

覆盖内容：

| 测试文件 | 覆盖点 |
|---|---|
| `pkg/jwtx/jwtx_test.go` | 签发/解析、过期、错误签名、错误 issuer、访问令牌与刷新令牌不可互换 |
| `pkg/cache/cache_test.go` | 读写删、TTL 过期、自增窗口起始与重置、Close 幂等 |
| `pkg/errcode/errcode_test.go` | 未知错误归一、Wrap 追溯、`WithMsg` 不改原定义、**全部错误码唯一性** |
| `service/auth_service_test.go` | **真实 SQLite + repository 全链路**：验证码限流三档、一次性校验、失败次数作废、登录自动注册、令牌轮换防重放、登出吊销、资料脱敏与校验、微信建号与绑定手机号 |
| `service/article_service_test.go` | **真实 SQLite**：slug 自动生成与同标题避让、非法 slug 拒绝、草稿对前台不可见（slug 与 ID 两条路）、发布 → 重复发布被拒 → 下线 → 重新发布、空正文拒绝发布、点赞幂等与计数、阅读数自增、标签/关键词/分类过滤、分类下有文章禁删 |
| `service/content_validate_test.go` | 纯函数：长度按**字符**而非字节、URL 协议白名单（挡 `javascript:` / `data:` / `file:`）、标签忽略大小写去重、部署方式白名单、文件名消毒（路径穿越 / 控制字符 / 截断） |
| `service/quiz_service_test.go` | 纯函数与落库：**判分是集合相等不给部分分**、单选传多答案被拒、答案指向不存在的选项被拒、判断选项固定、多选答案不能覆盖全部选项、重复作答不重复累计（`fresh`）、整卷 2/3 → 67 分、单次提交内同题去重保首次作答、**错题本口径为「最近一次仍答错」**、草稿与下线题不可作答、随机抽题按分类/难度过滤、**批量导入是全量校验一次落库**（坏行让整批不入库且带行号）、投稿无解析被拒且固定 pending、审核驳回必填原因且不能重复审、通过加分只记一次 |
| `service/tool_book_service_test.go` | 大纲解析的常见写法（# / ## / - / 全角竖线 / 漏空格）、两级树落库并按文章标题覆盖、失效节点**前台跳过后台保留**、空目录不能发布、节出现在任何章之前被忽略、删书连章节一起物理删 |
| `service/resource_service_test.go` | 大小与 SHA256 从**落盘内容实测**（不采信请求）、非 `resource_file` 场景的 key 被拒、文件不存在/为空被拒、需登录资源对游客返回 40031、下载数自增、签名地址带 `e`/`s` 参数、**共用文件在两个引用者都在时不回收**、换文件后重新实测、草稿不进列表与筛选项 |
| `service/tool_service_test.go` | URL 白名单、非法部署方式被拒、负 Star 被拒、草稿对前台不可见（slug 与 ID 两条路）、详情浏览数自增、语言/协议/Star/关键词过滤与 Star 排序、筛选项只聚合已发布、slug 同名避让、局部更新保留未传字段、分类不存在被拒 |
| `service/search_query_test.go` | 纯函数：全角归一、疑问词与语气词剔除、最长匹配不切碎长词、未知串整串保留/超长丢弃、同组去重、英文停用词剔除、**中英混排按文字系统切段**、解析不出词时**回落整串而非不过滤**、词典无空词条且语气词按长度倒序 |
| `service/search_relevance_test.go` | **真实 SQLite 全链路**：问句命中正文、同义词展开能捞到词面不同的内容、标题命中排在正文命中之前、多概念先严格 AND、一个都不沾才放宽为 OR、只命中标签也能搜到、无关键词返回全部、**语气词输入返回空而不是全站内容** |
| `service/qa_service_test.go` | **真实 SQLite 全链路**：新账号前 3 条回答进 `pending` 且对游客不可见、专家与管理员免审直发、**角色是 expert 但档案没过审照样进 pending**、一人一答（唯一索引挡住第二条）、待审回答不能采纳、非提问人不能采纳、采纳后状态/采纳标记/+10 积分一次到位、**重复采纳被 CAS 挡住且积分不翻倍**、点赞幂等（取消再点不重复记分）、自赞计数加但不记分、点赞分记在回答作者头上、标题/正文长度校验、提问 10 分钟冷却（把上一条时间拨回 11 分钟验证窗口本身）、徽章阈值 |
| `service/order_service_test.go` | **真实 SQLite 全链路，不 mock 仓储**——订单最容易出错的地方恰恰在 SQL 那一层：悬赏与预约两种下单、预算与附件校验、**8 个 goroutine 抢同一单只有 1 个赢**、未知通道 400 / 未接通通道 501 / 线下通道留待支付、重复支付不改 `paid_at`、支付过期拦截、会话复用与生命周期、完成 → 确认 → 结算 → 余额增加、结算幂等、金额不平拒结算、**抽佣取订单快照而不是当前配置**、评价只能一次且仅限已完成、**争议结案后仍能从订单详情读到结论**、退款单不可评价、争议判完成自动结算、预约金额由服务端定价、挂起会话在完成时收口、访问控制与「我的/专家的」两侧列表、后台强制流转仍受状态机约束 |
| `router/router_test.go` | 路由树能构建（gin 的路由冲突只在启动时 panic，`go build` 看不出来）、**26 条订单路由逐条注册**、无重复注册、订单号形状只有一个 owner、前端静态托管与 `/api/v1` 共存互不吃掉对方 |
| `scheduler/scheduler_test.go` | **panic 就地隔离**（后续任务照常跑、进程不挂）、单任务超时被放弃且不拖累其他任务、`RunOnce` 按注册顺序执行、重复 `Start` 报错（多启一份会让日志里处理条数凭空翻倍）、`Stop` 等在跑的一轮结束且幂等、父 ctx 取消即停机、`enabled=false` 时仍允许手动补跑 |
| `config/config_test.go` | 默认配置必须能直接启动；定时任务的四个边界（周期/批量/超时/关闭时仍要合法）**一律报错而不是静默取默认值** |

带覆盖率：

```bash
go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1
```

### 端到端冒烟

三个套件，各守一块：

| 套件 | 断言 | 覆盖 |
|---|---|---|
| `scripts/smoke_kb.py` | 173 | 栏目导航 → 文章 → 开源工具 → 资源下载 → 运维题库 → 后台录入与校验 → 用完整问句检索 |
| `scripts/smoke_order.py` | 104 | 悬赏 → 抢单 → 支付 → 服务会话 → 完成 → 确认 → 结算 → 评价 → 争议 → 线下核账 |
| `scripts/smoke_sched.py` | 37 | 定时任务：待支付超时被关单、到期自动确认并结算、未接单的悬赏**不该**被误伤、后台人工操作与自动扫描不重复打钱 |

一条命令跑完（每个套件各配一个**全新的库与全新的服务进程**）：

```bash
cd server
go build -o bin/api.exe ./cmd/api     # Linux / macOS 去掉 .exe（或直接 make smoke）
bash scripts/smoke.sh
```

```
############ kb（库 kb.db）############
===== 知识库冒烟结果：173/173 通过 =====

############ order（库 order.db）############
===== 订单冒烟结果：104/104 通过 =====

############ sched（库 sched.db）############
===== 定时任务冒烟结果：37/37 通过 =====

===== 冒烟全绿 =====
```

`sched` 套件必须以**短周期**启动服务才有意义——它的断言全是「等一会儿，系统自己做了什么」。
`smoke.sh` 已经把这组变量钉死，手工跑时照抄：

```bash
SCHEDULER_INTERVAL=1s PAYMENT_PAY_TIMEOUT=4s PAYMENT_AUTO_CONFIRM_AFTER=6s \
  DB_SQLITE_PATH=./data/sched.db ./bin/api.exe &
python scripts/smoke_sched.py
```

用轮询而不是 `sleep` 固定时长：调度周期与系统抖动会让「刚好等 4 秒」变成随机的过或不过，
轮询到目标状态才既快又稳。

任一断言失败即以非 0 退出码结束，可直接接进 CI；失败时只回显失败项，完整输出与服务日志留在
`.smoke-run/` 下。

**为什么一个套件一个服务进程**：两个脚本都要跑在全新库上，且都登录 `13800007777` /
`13800000000` 这几个固定号码。共用一个进程时，第二个脚本的验证码请求会撞上「同一号码 60 秒
只能发一次」的保护——白等一分钟，还容易被误判成环境坏了。分开起进程，限流计数随进程一起归零。

单独跑某一个套件（调试时用）：

```bash
rm -f data/ops_expert.db
go run ./cmd/api > /tmp/api.log 2>&1 &
python scripts/smoke_kb.py /tmp/api.log        # 验证码从服务日志里取，这个参数必需
python scripts/smoke_order.py                  # dev 环境验证码直接回传，可省略
```

两个脚本取验证码的方式不同：`smoke_kb.py` 一律读服务日志里的 `phone=... code=...`；
`smoke_order.py` 优先用响应里回传的 `devCode`（dev 环境的 `sms.expose_code: true`），
只有关掉它之后才退回读日志——那时才需要把日志路径传进来。

两个脚本的断言都写明了它在守哪条业务规则（比如「需登录资源对游客被拒」「答对后不该再挂在错题本里」
「认识但没接通的支付通道返回 501，禁止静默降级」），所以它同时是一份可执行的接口说明。

断言总数必须**每次都一样**。曾经有一条断言挂在「随机抽一道题」的结果上——抽到多选题时它
静默不跑，于是总数在 168/169 之间随抽签浮动，看着像少验了一条却查不出原因。凡是依赖数据的
断言都要先显式取到符合条件的那条数据，不能靠抽签。

`scripts/smoke_twice.sh` 把这条纪律变成一条命令：连跑两趟（各自全新的库），并排比对每个套件的总数。

```bash
bash scripts/smoke_twice.sh
```

```
  kb     a=178/178    b=178/178    稳定
  order  a=104/104    b=104/104    稳定
  sched  a=37/37      b=37/37      稳定
RESULT: 两趟断言总数一致，全绿
```

（单独跑一趟只说明"这次没红"；只有比两趟才能发现"某条断言这次根本没跑"。）

### 前端自检

前端是原生 ESM，**没有构建步骤，所以也没有编译期检查**——`import` 了不存在的导出、
模板里字段名写错、转义漏了，浏览器里只表现为整页白屏加控制台一行红字，`curl` 拿到的仍是 200。
所以补了两个静态/运行时自检，改完前端顺手跑一遍：

```bash
# 需要服务已在跑，且库里要有种子数据
node scripts/depcheck.js        # ESM 导入符号逐个核对（默认查 web/static/assets/js）
node scripts/render_check.mjs   # 用真实接口数据驱动各页面渲染函数，271 项断言
```

`render_check.mjs` 的做法是给 `fetch` / `localStorage` / `document` 打桩，然后 `import` 真实的
渲染函数（它们不碰 DOM，只返回 HTML 字符串），所以能在 Node 里跑。它覆盖五类容易出事的地方：
**入口页**（`index.html` 引用的脚本与样式是否真的存在）、结构（页面里该有的容器在不在）、
转义（注入的 `<script>` 与 `javascript:` 链接有没有被拦）、检索页的三种状态
（有结果 / 零结果给建议词 / 空输入给热门词）、以及**订单详情的 9 种状态**。

第一类是从一次真实事故里补的：`index.html` 曾经写着 `/assets/app.js`，而文件实际在
`/assets/js/app.js`。所有模块级自检都直接 `import` 源文件，因此全绿；但浏览器一打开只有
一个转圈图标——**没有任何断言在看"入口页引用的东西是否存在"**。现在这条会直接红。

最后一条值得单说：详情页的模板抽成了纯函数 `orderDetailHTML(order, role)`，于是可以在 Node 里
把「状态 × 视角」11 种组合逐一渲染，断言每种组合下该出现的按钮出现了、不该出现的没有出现。
若不做这层拆分，验一个「争议已结案」的页面就得先在真库里造一笔单并走完整套后台流程——
成本高到没人会去写，最后就变成"改完模板靠肉眼点一遍"。

登录态那一段用**一次性手机号**（每次不同）注册，绕开"同一号码 60 秒只能发一次"的限流，
所以连跑多次也不会红。它会真发一条悬赏、渲染列表与详情、再取消掉，把
`api.js` → 路由 → service → 数据库这条链路也顺带验了一遍。

---

## 六·补充、部署到服务器

交付物是**一个可执行文件**：静态站点用 `go:embed` 打在里面，不需要 nginx、不需要 Node，
数据库默认 SQLite，零外部依赖。部署物只有三份文本（`deploy/install.sh`、配置模板、systemd 单元），
加一个二进制。完整步骤见 [`deploy/README.md`](deploy/README.md)，这里只讲清楚三件事。

### 1. 交叉编译（CGO 必须关）

```bash
cd server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.version=1.0.0" -o dist/ops-expert-api ./cmd/api
```

SQLite 驱动是纯 Go 实现（`glebarez/sqlite`），所以能关掉 CGO 交叉编译；
换成 `mattn/go-sqlite3` 就必须在目标机上编译，这一条会立刻失效。

### 2. 安装与升级是同一条命令

```bash
scp dist/ops-expert-api root@<host>:/tmp/
ssh root@<host> 'bash /opt/ops-expert/deploy/install.sh --binary /tmp/ops-expert-api \
    --env test --port 8080 --base-url http://<host>:8080'
```

脚本做六件事：建 `opsexpert` 系统账号 → 装二进制 → 渲染配置 → 装 systemd 单元 → 启动 → 探活。
**配置只在首次生成**，升级时不会被覆盖——否则线上手改的参数每次发版都被冲掉，
而"我明明改过配置，重启又变回去了"是最难查的一类问题。密钥用服务器本地 `openssl` 生成，
不经过仓库也不经过聊天记录。

### 3. 部署后跑一遍自检

冒烟脚本验证的是"代码对"（自己起进程、自己建库）；部署完要验证的是"这台机器上真的能用"：

```bash
python scripts/verify_deploy.py http://<host>:8080     # 52 项
```

它只发 HTTP，不碰进程和数据库，覆盖五段：静态站点与文档没被静态路由吞掉（含入口页脚本路径
这条回归断言）、读路径与种子数据、发布悬赏、专家抢单、支付通道与支付、后台可见。
会**真的创建一条标题带「部署自检」的悬赏**并走完抢单与支付——"能下单"才是部署成功的证据；
验证完可以重置库：`systemctl stop ops-expert && rm -f data/ops_expert.db* && systemctl start ops-expert`
（会重新执行种子数据）。

它连续跑两遍也不会红：固定号码（种子专家、管理员）撞上 60 秒限流时，脚本会把那次 429
当作**正常保护**等过窗口重试，且不计入断言总数——否则总数会随重试次数浮动，
而"断言总数每次都一样"是这个项目一直守的纪律。

> 当前部署是 **test 档**（`app.env: test`）：验证码直接回给前端、支付走 mock。
> 上生产要换的东西列在 `deploy/README.md` 的「上线前必须换掉的四件事」里，
> 其中 `app.env: prod` 会硬拦 mock 短信/mock 支付/默认 JWT 密钥——起不来而不是悄悄跑着。

---

## 七、本阶段未做的事（下一阶段）

明确列出来，避免误以为已完成：

- ❌ **支付对接**（微信 / 支付宝服务商分账）—— 通道骨架在 `internal/service/payment/`，
  `wechat` / `alipay` 目前明确返回 501，而不是假装成功或静默降级到 mock
- ❌ **专家主页的「服务案例」** —— 评价列表已完成（`GET /experts/:id/reviews`，含汇总 / 分布 / 匿名脱敏），
  案例精选（专家从自己完成过的订单里挑代表作展示）还没有接口
- ❌ **后台仍缺的一块**：数据看板的趋势图（概览目前只有待办数字；对账页已有日表，但没做折线图）。
  系统配置（抽佣比例、分类、技能标签）已完成
- ❌ **审计日志**目前有用户治理（封禁/解封/改角色）、抽佣比例变更（`setting.commission`）、
  内容审核（`article.review`）、问答审核（`qa.answer_review`）与问答转教程（`qa.to_article`）；
  订单强制流转与专家审核还没有写，补的时候直接用 `audit.go` 里的共享 `appendAudit`，别再各写一份
- ❌ **对账只到日粒度**。要按小时看、或按专家 / 按渠道拆分，还需要在聚合里再加维度；
  目前单日明细给了原始记录，够人工核，但不够做渠道成本分析
- ❌ 阿里云/腾讯云短信的真实发送实现；对象存储（OSS/COS）驱动仍返回明确错误而非静默降级
- ❌ WebSocket 实时推送（订单状态变化目前靠用户刷新；前端未做轮询，避免空转请求）
- ❌ 订单详情里的附件上传（悬赏的 `attachments` 接口支持，表单暂未接入上传控件）
- ❌ 知识库的全文检索（当前用 `LIKE`，文章量上十万时应换 MySQL FULLTEXT 或独立搜索服务）
- ❌ 知识库的阅读数缓存化（当前每次详情都写一次库，高频访问下会成为写热点）
- ❌ **资讯抓取的重试与退避**（失败只记一行 warn，下一轮再试；没有指数退避）
- ❌ **资源换文件时不回收旧文件**（`Update` 刻意保守，见上方说明）—— 长期运行会积累孤儿文件，需要离线清理兜底
- ❌ 题库的**题目导出**（导入已完成；导出用于迁移与备份，等真实题量起来再做）
- ❌ **教程集的「学习进度」**（按产品决定②本期不做；表结构已为它留了空间）
- ❌ **问答的举报与折叠** —— 现在只有管理员能删（`DELETE /admin/qa/questions/:id` 与待审队列），
  没有「用户举报 → 折叠」这条轻量路径。量上来之后需要，现在人少时人工看得过来
- ❌ **问答的标签页与技能筛选只做了 ID 过滤** —— 前端有 chip 多选，但没有「按技能看热门问题」
  这种聚合页；建议等真实问题过百条再做，那时候才有"热门"可言

### 建议的下一阶段顺序

1. **支付对接** —— `payment.Provider` 这层已经定型，补 `wechat` / `alipay` 的 Prepay 与回调即可；
   需要商户资质，可以并行推进申请。现在整条链路已经能自助跑通（入驻 → 审核 → 接单 → 确认 → 结算 → 提现审批），
   卡住的只剩"真收钱"这一步
2. **数据看板的趋势图** —— 概览页只有待办数字；对账页的日表数据已经在手，
   差的是折线图（收款 / 退款 / 净现金三条线就够，别上来就上图表库全家桶）。
   用户治理、财务对账、系统配置（抽佣 / 分类 / 技能）、专家主页评价都已完成
3. **定时任务拆成独立进程** —— 目前调度器跟着 API 进程跑（`SCHEDULER_ENABLED` 控制）。
   副本数上去以后应当单独部署一份（或全部关掉、只留一个），否则每个副本都在扫同一批数据。
   现在之所以能这么跑，是因为三个任务在订单侧都幂等；加不幂等的任务前必须先补分布式锁

---

## 八、常见问题

**Q：启动报 `jwt.secret 长度不足 32 字节`？**
配置里 `jwt.secret` 被改短了。开发环境直接删掉这行用默认值即可。

**Q：启动报 `生产环境禁止开启 sms.expose_code`？**
`APP_ENV=prod` 时这些开关会被拒绝。这是有意的——防止把开发开关带上线。

**Q：SQLite 报 `database is locked`？**
已把 SQLite 连接数收敛为 1 并开启 WAL，正常不会出现。若仍出现，说明有长事务，检查是否有未结束的手工事务。

**Q：`/docs` 页面空白？**
Swagger UI 从 CDN 加载，内网环境需要放行 `cdn.jsdelivr.net`。接口本身不受影响，`openapi.yaml` 可直接下载导入 Postman。

**Q：改了模型但表结构没变？**
`database.auto_migrate` 默认为 true，重启即生效。但 AutoMigrate **不会删列也不会改列类型**——上线前请换成正式迁移工具（如 `golang-migrate`），AutoMigrate 只适合开发期。
