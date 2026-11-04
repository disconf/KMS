/* ============================================================================
   paths-data.js — 运维成长路径的内容。

   为什么把路线写在前端数据里而不是后端表：路径是「内容编排」，不是用户数据。
   它随教程/题库的增补一起改，跟着代码走能进版本库、能 review、能回滚；
   放进数据库只会多一套只在后台改一次的编辑界面，而内容团队并不常改它。

   节点里的 href 一律指向站内真实路由（教程文章 / 工具页 / 速查 / 剧本 / 题库），
   不写教程集的数字 id——那是自增的，换库就失效。写死失效链接比不给链接更糟：
   用户点进去看到 404，会以为整条路径都是摆设。

   kind 决定节点左侧的图标与颜色：
     read 读教程 | sheet 查速查 | drill 练题 | runbook 走剧本 | tool 用工具 | write 动手写
   ========================================================================= */

export const RESOURCE_KIND = {
  read:    { label: '读', icon: 'book',     tone: 'brand' },
  sheet:   { label: '查', icon: 'terminal', tone: 'info' },
  drill:   { label: '练', icon: 'quiz',     tone: 'warn' },
  runbook: { label: '排', icon: 'activity', tone: 'danger' },
  tool:    { label: '用', icon: 'tools',    tone: 'ok' },
  write:   { label: '写', icon: 'edit',     tone: 'brand' },
};

export const PATH_LEVEL = {
  beginner: { label: '入门', tone: 'ok' },
  advanced: { label: '进阶', tone: 'brand' },
  expert:   { label: '深入', tone: 'info' },
};

/**
 * 六条路线，按「先能上手 → 再能负责一块 → 最后能对稳定性负责」的顺序排。
 * 每条路线的阶段 (stage) 都有明确的过关标准 (goal)，
 * 因为「看完一篇教程」不等于学会了，goal 是让人自己判断能不能往下走。
 */
export const PATHS = [
  {
    id: 'linux-base',
    name: 'Linux 运维基本功',
    level: 'beginner',
    icon: 'terminal',
    hours: 6,
    tagline: '先能独立处理一台机器',
    summary: '刚接手服务器时最该会的那几件事：看清资源水位、读懂日志、定位端口与 DNS，'
      + '以及在磁盘写满之前把它处理掉。这条路线的每一节都能当天用上。',
    outcome: '接到「机器慢 / 连不上 / 磁盘满」这类报障，能自己先查一轮再决定要不要上报。',
    stages: [
      {
        id: 's1',
        name: '摸清一台机器的家底',
        goal: '能在一分钟内说清这台机器的 CPU、内存、磁盘、负载分别是多少。',
        resources: [
          { kind: 'sheet', label: 'Linux 高频命令速查', href: '#/cheatsheet?c=linux', note: 'top / df / free / ss 的常用写法' },
          { kind: 'read', label: '一键健康检查脚本', href: '#/article/linux-health-check-scripts', note: '把散落的命令收成一个可交付的脚本' },
        ],
      },
      {
        id: 's2',
        name: '服务与开机自启',
        goal: '能说清一个服务为什么起不来，并让它在重启后自己回来。',
        resources: [
          { kind: 'sheet', label: 'systemd 速查', href: '#/cheatsheet?c=systemd', note: 'status / journalctl / enable 的正确用法' },
          { kind: 'read', label: '运维实战教程栏目', href: '#/channel/linux-tutorial', note: '按主题补基础，不用从头读到尾' },
        ],
      },
      {
        id: 's3',
        name: '网络与 DNS 不通怎么查',
        goal: '拿到「访问不了」的报障，能按 端口 → 链路 → DNS → 证书 的顺序排除。',
        resources: [
          { kind: 'sheet', label: '网络排查速查', href: '#/cheatsheet?c=network', note: 'ss / nc / curl / dig 的判定写法' },
          { kind: 'runbook', label: '剧本：端口不通', href: '#/runbook/port-unreachable', note: '从本机到对端逐段验证' },
          { kind: 'runbook', label: '剧本：DNS 解析失败', href: '#/runbook/dns-fail', note: '区分「解析慢」和「解析错」' },
        ],
      },
      {
        id: 's4',
        name: '磁盘与性能初查',
        goal: '磁盘告警时能判断是日志、是临时文件还是业务数据，并安全清理。',
        resources: [
          { kind: 'runbook', label: '剧本：磁盘写满', href: '#/runbook/disk-full', note: '先止血再治理，别直接 rm' },
          { kind: 'sheet', label: '性能排查速查', href: '#/cheatsheet?c=perf', note: 'top / iostat / pidstat 看什么指标' },
        ],
      },
      {
        id: 's5',
        name: '自测一轮，找出薄弱项',
        goal: '做完一轮练习，知道自己哪一块最需要补。',
        resources: [
          { kind: 'drill', label: '随机练习 10 题', href: '#/quiz', note: '答完看解析，错题会自动进错题本' },
          { kind: 'drill', label: '限时模拟考', href: '#/exam', note: '整卷交完才给答案，成绩单按方向列弱项' },
        ],
      },
    ],
  },

  {
    id: 'web-gateway',
    name: 'Nginx 与站点接入',
    level: 'advanced',
    icon: 'server',
    hours: 8,
    tagline: '把站点稳稳接进来',
    summary: '站点接入是最常见的运维活：装、配、上生产、出事能立刻判断是 502 还是证书问题。'
      + '这条路线以 Nginx 为主线，顺带把 OpenResty 的扩展方式过一遍。',
    outcome: '独立完成一个新站点的接入与上线，并在 5 分钟内定位 502 / 504 / 证书三类故障。',
    stages: [
      {
        id: 's1',
        name: '选型与安装',
        goal: '知道 Nginx 与 OpenResty 的差别，能在目标系统上完成安装并验证版本。',
        resources: [
          { kind: 'read', label: 'Nginx 入门：它到底做什么', href: '#/article/nginx-intro', note: '反向代理 / 负载均衡 / 静态资源' },
          { kind: 'read', label: 'Nginx 安装', href: '#/article/nginx-install', note: '包管理与编译安装的取舍' },
          { kind: 'read', label: 'OpenResty 入门', href: '#/article/openresty-intro', note: '需要写 Lua 逻辑时再选它' },
        ],
      },
      {
        id: 's2',
        name: '配置结构与调优',
        goal: '能读懂一份生产 nginx.conf，知道 include 从哪里来、worker 该开几个。',
        resources: [
          { kind: 'read', label: 'Nginx 配置详解', href: '#/article/nginx-config', note: 'server / location 的匹配优先级' },
          { kind: 'tool', label: 'Nginx 工具页与教程集', href: '#/tools/nginx', note: '六章教程集，从入门到上线' },
        ],
      },
      {
        id: 's3',
        name: '上生产：基线先行',
        goal: '按基线清单过一遍：日志切分、超时、限流、压缩、安全响应头。',
        resources: [
          { kind: 'read', label: 'Nginx 生产实践', href: '#/article/nginx-production', note: '参数背后的原因，不是抄配置' },
          { kind: 'read', label: 'Nginx 生产基线清单', href: '#/article/nginx-production-baseline', note: '逐项对照，缺一项补一项' },
        ],
      },
      {
        id: 's4',
        name: '502 / 504 怎么排',
        goal: '看到 502 不慌：先分清是后端没起、后端超时，还是连接被打满。',
        resources: [
          { kind: 'runbook', label: '剧本：Nginx 502', href: '#/runbook/nginx-502', note: '每步带命令与判断分支' },
          { kind: 'read', label: 'Nginx 常用命令', href: '#/article/nginx-commands', note: 'reload 与 restart 的区别必须清楚' },
        ],
      },
      {
        id: 's5',
        name: 'HTTPS 与证书',
        goal: '能独立完成证书签发、部署、reload，并说清为什么换了证书还报旧证书。',
        resources: [
          { kind: 'sheet', label: 'SSL 排查速查', href: '#/cheatsheet?c=ssl', note: 'openssl s_client 看真实链路' },
          { kind: 'runbook', label: '剧本：证书告警与 TLS 失败', href: '#/runbook/cert-tls', note: '到期 / 链不全 / 未 reload 三种原因' },
          { kind: 'read', label: '复盘：换了证书为什么还报旧证书', href: '#/article/postmortem-cert-not-reloaded', note: '容器里 reload 与宿主机不是一回事' },
        ],
      },
    ],
  },

  {
    id: 'database',
    name: 'MySQL 与 Redis 运维',
    level: 'advanced',
    icon: 'package',
    hours: 10,
    tagline: '扛住有状态的服务',
    summary: '数据库是唯一「重启不一定能修好」的那一层。这条路线讲安装、参数、备份与主从，'
      + '重点是遇到慢查询和主从延迟时，知道该看哪几个指标。',
    outcome: '能独立完成一次 MySQL 上线、备份校验与主从延迟定位。',
    stages: [
      {
        id: 's1',
        name: '安装与初始化',
        goal: '能装出一套可以直接上业务的 MySQL，字符集、时区、目录都设对。',
        resources: [
          { kind: 'read', label: 'MySQL 安装', href: '#/article/mysql-install', note: '版本选择与初始化参数' },
          { kind: 'read', label: 'MySQL 配置', href: '#/article/mysql-config', note: 'innodb 与连接数怎么定' },
        ],
      },
      {
        id: 's2',
        name: '生产参数与备份',
        goal: '有一套能落地的备份策略，并验证过恢复（没验证过的备份等于没有）。',
        resources: [
          { kind: 'read', label: 'MySQL 生产实践', href: '#/article/mysql-production', note: '慢日志、binlog、连接池' },
          { kind: 'read', label: 'MySQL 常用命令', href: '#/article/mysql-commands', note: 'show processlist 的正确看法' },
        ],
      },
      {
        id: 's3',
        name: '慢查询与主从延迟',
        goal: '能从「慢」和「不一致」两类现象出发，定位到具体语句或复制线程。',
        resources: [
          { kind: 'read', label: '主从延迟的常见原因', href: '#/article/mysql-replication-lag-causes', note: '大事务 / 无主键 / 单线程回放' },
          { kind: 'read', label: '主从延迟定位实战', href: '#/article/mysql-replication-lag-diagnose', note: '按指标往下钻' },
          { kind: 'runbook', label: '剧本：MySQL 慢查询', href: '#/runbook/mysql-slow', note: '先看是不是锁等待' },
        ],
      },
      {
        id: 's4',
        name: 'Redis 不能只会 set',
        goal: '知道内存淘汰策略、持久化方式与热 key 的处理办法。',
        resources: [
          { kind: 'read', label: 'Redis 生产实践', href: '#/article/redis-production', note: 'maxmemory 与淘汰策略' },
          { kind: 'sheet', label: 'Redis 速查', href: '#/cheatsheet?c=redis', note: 'info / bigkeys / memory usage' },
          { kind: 'tool', label: 'Redis 工具页与教程集', href: '#/tools/redis', note: '六章教程集串起主从与集群' },
        ],
      },
      {
        id: 's5',
        name: '故障演练',
        goal: '在测试环境演练一次磁盘写满，确认备份与告警都真的会响。',
        resources: [
          { kind: 'runbook', label: '剧本：磁盘写满', href: '#/runbook/disk-full', note: '数据库场景要格外小心 binlog' },
          { kind: 'read', label: '复盘：MySQL 数据盘写满', href: '#/article/postmortem-mysql-disk-full', note: '一次真实的连锁反应拆解' },
        ],
      },
    ],
  },

  {
    id: 'k8s-production',
    name: 'Kubernetes 生产运维',
    level: 'advanced',
    icon: 'layers',
    hours: 12,
    tagline: '从会 kubectl 到能值班',
    summary: '装起来只是开始。真正难的是探针、资源限制、滚动发布与「Pod 一直重启」的三分钟判断。'
      + '这条路线按「装 → 配 → 上线 → 排障 → 观测」推进。',
    outcome: '接到「Pod 起不来 / 一直重启 / 发布后流量异常」能独立排查，并给出可执行的修复动作。',
    stages: [
      {
        id: 's1',
        name: '集群与对象模型',
        goal: '能说清 Pod / Deployment / Service / Ingress 各自解决什么问题。',
        resources: [
          { kind: 'read', label: 'Kubernetes 入门', href: '#/article/kubernetes-intro', note: '对象之间的关系图' },
          { kind: 'read', label: 'Kubernetes 安装', href: '#/article/kubernetes-install', note: '单机与集群的取舍' },
        ],
      },
      {
        id: 's2',
        name: '配置与持续发布',
        goal: '能用声明式配置管理应用，并让发布可回滚。',
        resources: [
          { kind: 'read', label: 'Kubernetes 配置', href: '#/article/kubernetes-config', note: 'ConfigMap / Secret / 探针' },
          { kind: 'tool', label: 'Argo CD 工具页', href: '#/tools/argo-cd', note: 'GitOps 方式发布' },
          { kind: 'read', label: 'Argo CD 灰度发布', href: '#/article/argocd-canary-release', note: '逐步放量的实操' },
        ],
      },
      {
        id: 's3',
        name: '生产实践',
        goal: '资源 request/limit、健康探针、优雅退出三项都配到位。',
        resources: [
          { kind: 'read', label: 'Kubernetes 生产实践', href: '#/article/kubernetes-production', note: '最常被忽略的三个参数' },
          { kind: 'read', label: 'Kubernetes 配置详解', href: '#/article/kubernetes-config', note: '回到配置确认细节' },
        ],
      },
      {
        id: 's4',
        name: 'kubectl 与日常操作',
        goal: '能不看文档完成 describe / logs / exec / rollout 的常用组合。',
        resources: [
          { kind: 'sheet', label: 'kubectl 速查', href: '#/cheatsheet?c=kubectl', note: '排查时最高频的十来条' },
          { kind: 'read', label: 'Kubernetes 常用命令', href: '#/article/kubernetes-commands', note: '带输出解读' },
        ],
      },
      {
        id: 's5',
        name: 'Pod 异常三分钟判断',
        goal: '面对 CrashLoopBackOff / Pending / OOMKilled，能在三分钟内定位到根因方向。',
        resources: [
          { kind: 'runbook', label: '剧本：Pod 一直重启', href: '#/runbook/crashloop', note: '看事件还是看日志，先用哪个' },
          { kind: 'runbook', label: '剧本：Pod 一直 Pending', href: '#/runbook/pod-pending', note: '调度失败的五种原因' },
          { kind: 'runbook', label: '剧本：OOM 被杀', href: '#/runbook/oom', note: '区分容器超限与节点压力' },
          { kind: 'read', label: 'Pod 重启排查指南', href: '#/article/k8s-pod-restart-troubleshooting', note: '完整案例复盘' },
        ],
      },
      {
        id: 's6',
        name: '可观测性收尾',
        goal: '出事时先看板子：知道该看哪个指标，而不是先登录机器。',
        resources: [
          { kind: 'tool', label: 'Prometheus 工具页', href: '#/tools/prometheus', note: '指标采集与告警规则' },
          { kind: 'tool', label: 'Loki 工具页', href: '#/tools/loki', note: '日志按标签检索，比 grep 快' },
        ],
      },
    ],
  },

  {
    id: 'observability',
    name: '监控与日志体系',
    level: 'advanced',
    icon: 'activity',
    hours: 8,
    tagline: '让问题自己冒出来',
    summary: '监控的价值不在于画了多少张图，而在于「有事时能第一时间知道，并且知道去看哪」。'
      + '这条路线从指标采集讲到告警与日志检索。',
    outcome: '能为一套业务搭出可用的指标 + 告警 + 日志三件套，并控制住告警噪音。',
    stages: [
      {
        id: 's1',
        name: '指标体系入门',
        goal: '理解 counter / gauge / histogram 的差别，知道自己要采集什么。',
        resources: [
          { kind: 'read', label: 'Prometheus 入门', href: '#/article/prometheus-intro', note: '四种指标类型的用途' },
          { kind: 'tool', label: 'Prometheus 工具页与教程集', href: '#/tools/prometheus', note: '六章教程集' },
        ],
      },
      {
        id: 's2',
        name: '抓取与存储',
        goal: '能写出正确的 scrape 配置，并知道数据保留多久合适。',
        resources: [
          { kind: 'read', label: 'Prometheus 配置', href: '#/article/prometheus-config', note: 'relabel 是最容易写错的地方' },
          { kind: 'read', label: 'Prometheus 生产实践', href: '#/article/prometheus-production', note: '远程存储与容量估算' },
        ],
      },
      {
        id: 's3',
        name: '查询与告警',
        goal: '能写出「真的会在出事时响」的告警规则，而不是天天被吵。',
        resources: [
          { kind: 'read', label: 'Prometheus 常用查询', href: '#/article/prometheus-commands', note: 'PromQL 常用写法' },
          { kind: 'runbook', label: '剧本：CPU 使用率飙升', href: '#/runbook/cpu-high', note: '告警响了之后按这个走' },
        ],
      },
      {
        id: 's4',
        name: '看板与可视化',
        goal: '做出一张「一眼能看出哪里不对」的看板，而不是指标堆砌。',
        resources: [
          { kind: 'read', label: 'Grafana 入门', href: '#/article/grafana-intro', note: '面板与变量的组织方式' },
          { kind: 'read', label: 'Grafana 配置', href: '#/article/grafana-config', note: '数据源与告警通道' },
        ],
      },
      {
        id: 's5',
        name: '日志聚合',
        goal: '定位问题时能按标签在几秒内捞出相关日志，而不是 ssh 上去 grep。',
        resources: [
          { kind: 'tool', label: 'Loki 工具页', href: '#/tools/loki', note: '标签设计决定检索效率' },
          { kind: 'read', label: 'Loki 生产实践', href: '#/article/loki-production', note: '保留策略与查询加速' },
        ],
      },
    ],
  },

  {
    id: 'sre-stability',
    name: 'SRE 稳定性与复盘',
    level: 'expert',
    icon: 'shield',
    hours: 10,
    tagline: '对结果负责的那一层',
    summary: '到了这一层，拼的不是命令熟不熟，而是「有没有流程」：出事先按剧本走，事后再写复盘，'
      + '把一次故障变成组织记忆。这条路线的产出物是可以拿给团队用的。',
    outcome: '能主导一次故障的处置与复盘，并输出可被他人复用的排障路径与改进项。',
    stages: [
      {
        id: 's1',
        name: '先学会读事故',
        goal: '读完一篇复盘，能说出「直接原因 / 根本原因 / 改进项」三者的区别。',
        resources: [
          { kind: 'read', label: '事故复盘栏目', href: '#/channel/ops-postmortem', note: '按时间线还原的写法' },
          { kind: 'read', label: '复盘：conntrack 表满', href: '#/article/postmortem-conntrack-full', note: '最隐蔽的一类连接问题' },
        ],
      },
      {
        id: 's2',
        name: '把排障标准化',
        goal: '手上有一份能交给同事照着走的排障流程，而不是只在脑子里。',
        resources: [
          { kind: 'runbook', label: '排障剧本总览', href: '#/runbook', note: '11 个高频场景的决策树' },
          { kind: 'runbook', label: '剧本：Kafka 消费堆积', href: '#/runbook/kafka-lag', note: '先分清是生产快还是消费慢' },
        ],
      },
      {
        id: 's3',
        name: '日常巡检与交接',
        goal: '建立日检 / 周检 / 月检节奏，交接班有据可依。',
        resources: [
          { kind: 'tool', label: '值班巡检清单', href: '#/checklist', note: '日检 12 项，勾完可导出交接记录' },
          { kind: 'sheet', label: '速查手册', href: '#/cheatsheet', note: '清单里的命令都在这里' },
        ],
      },
      {
        id: 's4',
        name: '写一份能用的复盘',
        goal: '按模板写出一篇复盘：时间线、影响面、根因、改进项与责任人。',
        resources: [
          { kind: 'write', label: '投稿页（套用复盘模板）', href: '#/contribute?channel=ops-postmortem', note: '模板已内置，四段撑起全文' },
          { kind: 'read', label: '复盘：证书未 reload', href: '#/article/postmortem-cert-not-reloaded', note: '看别人怎么把「人为疏忽」写清楚' },
        ],
      },
      {
        id: 's5',
        name: '节奏与自证',
        goal: '用周报和模拟考持续证明自己覆盖了哪些面，还差哪一块。',
        resources: [
          { kind: 'tool', label: '周报生成器', href: '#/weekly', note: '五段式草稿，本地保存' },
          { kind: 'drill', label: '限时模拟考', href: '#/exam', note: '按方向列弱项，指导下一轮补什么' },
        ],
      },
    ],
  },
];

/** 按 id 取路线。找不到返回 null，由调用方决定显示 404 还是兜底。 */
export function findPath(id) {
  return PATHS.find((p) => p.id === id) || null;
}

/** 一条路线的总阶段数，进度分母统一走这里，避免各处自己数。 */
export function stageCountOf(path) {
  return (path && path.stages ? path.stages.length : 0);
}
