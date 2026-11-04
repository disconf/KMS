/* ============================================================================
   checklist-data.js — 值班巡检清单（日检 / 周检 / 月检）。

   与「速查手册」的分工：
     速查回答「这个命令怎么写」，巡检回答「这一班该看哪几项、看到什么算正常」。
   所以每一项都必须带判定标准（expect），否则清单就退化成命令列表，
   新人勾完一圈仍然不知道有没有问题。

   critical 标记的是「漏了会出事」的项：页面上会高亮，并在未完成时单独提醒。
   宁可少标，标多了等于没标。

   runbook 指向排障剧本，异常时不用回搜索框重新找。
   ========================================================================= */

export const CHECKLISTS = [
  {
    id: 'daily',
    name: '日检',
    cadence: '每班一次 · 约 8 分钟',
    desc: '交接班时先过一遍：水位、服务、备份、证书。这四项没有异常，再去看业务报障。',
    groups: [
      {
        id: 'res',
        name: '资源水位',
        items: [
          {
            id: 'load', name: 'CPU 负载与核数', critical: true,
            cmd: 'uptime && nproc',
            expect: '1 分钟负载持续高于核数才算异常；单次尖峰不用处理。',
            runbook: '#/runbook/cpu-high',
            runbookText: 'CPU 高',
          },
          {
            id: 'mem', name: '内存与 swap', critical: true,
            cmd: 'free -h',
            expect: 'available 保持 10% 以上；swap 有使用不可怕，持续增长才可怕。',
            runbook: '#/runbook/oom',
            runbookText: 'OOM',
          },
          {
            id: 'disk', name: '磁盘使用率', critical: true,
            cmd: "df -hP | awk 'NR==1 || $5+0>=80'",
            expect: '输出只有表头 = 全部低于 80%。有分区 ≥ 90% 立即处理。',
            runbook: '#/runbook/disk-full',
            runbookText: '磁盘满',
          },
          {
            id: 'inode', name: 'inode 使用率',
            cmd: "df -iP | awk 'NR==1 || $5+0>=80'",
            expect: '和磁盘一样看百分比。inode 满时报错是「设备上没有空间」，但 df -h 显示还有空间。',
          },
        ],
      },
      {
        id: 'svc',
        name: '服务与端口',
        items: [
          {
            id: 'failed', name: '失败的 systemd 单元', critical: true,
            cmd: 'systemctl --failed --no-pager',
            expect: '输出「0 loaded units listed」。列出来的每一个都要说清是干什么的。',
          },
          {
            id: 'port', name: '关键端口监听',
            cmd: "ss -lntp | grep -E ':(80|443|3306|6379|9090)\\b'",
            expect: '期望的端口都在 LISTEN，且进程名对得上。少一个就是故障。',
            runbook: '#/runbook/port-unreachable',
            runbookText: '端口不通',
          },
          {
            id: 'health', name: '健康检查接口',
            cmd: "curl -sS -k -o /dev/null -w '%{http_code}  %{time_total}s\\n' https://127.0.0.1/healthz",
            expect: '状态 200，耗时 < 1s。只看状态码会漏掉「能通但慢」。',
          },
          {
            id: 'errlog', name: '近 24 小时错误日志',
            cmd: 'journalctl -p err -S -24h --no-pager | tail -50',
            expect: '没有新增的重复报错。同一个错误刷屏，往往比一次性崩溃更值得处理。',
          },
        ],
      },
      {
        id: 'data',
        name: '数据与凭据',
        items: [
          {
            id: 'backup', name: '备份产物', critical: true,
            cmd: 'ls -lht /data/backup | head -5',
            expect: '当天有文件、大小与昨天同一量级。「有文件」不等于「能恢复」，周检里会真恢复一次。',
          },
          {
            id: 'cert', name: '证书剩余天数', critical: true,
            cmd: "echo | openssl s_client -connect 127.0.0.1:443 -servername localhost 2>/dev/null | openssl x509 -noout -enddate",
            expect: '剩余 > 15 天。换过证书还要确认 reload 生效，否则对外仍是旧证书。',
            runbook: '#/runbook/cert-tls',
            runbookText: '证书告警',
          },
          {
            id: 'cron', name: '定时任务失败',
            cmd: 'journalctl -u cron -u crond -S -24h -p err --no-pager | tail -20',
            expect: '没有失败记录。静默失败的定时任务是最常见的数据丢失来源。',
          },
          {
            id: 'logsize', name: '日志目录体积',
            cmd: 'du -sh /var/log && ls -lS /var/log | head -5',
            expect: '总量与昨天相比无异常增长；单个文件超过 1G 说明轮转没生效。',
          },
        ],
      },
    ],
  },

  {
    id: 'weekly',
    name: '周检',
    cadence: '每周一次 · 约 40 分钟',
    desc: '日检看「现在有没有事」，周检看「会不会出事」：容量趋势、备份可恢复性、权限与证书链。',
    groups: [
      {
        id: 'capacity',
        name: '容量与趋势',
        items: [
          {
            id: 'grow', name: '磁盘增长最快的目录',
            cmd: 'du -xhd1 / 2>/dev/null | sort -h | tail -10',
            expect: '结合一周前的数据估算增速，算出「按这个速度还能撑几天」。',
            runbook: '#/runbook/disk-full',
            runbookText: '磁盘满',
          },
          {
            id: 'rotate', name: '日志轮转是否生效',
            cmd: 'logrotate -d /etc/logrotate.conf 2>&1 | grep -iE "error|considering"',
            expect: '没有 error。留意「log does not need rotating」是否出现在本该轮转的文件上。',
          },
          {
            id: 'tablespace', name: '数据库表空间 Top 10',
            cmd: "mysql -N -e \"SELECT table_schema, ROUND(SUM(data_length+index_length)/1048576) mb FROM information_schema.tables GROUP BY 1 ORDER BY 2 DESC LIMIT 10;\"",
            expect: 'Top 表的增长与业务量对得上；没有单表突然翻倍。',
          },
        ],
      },
      {
        id: 'reliability',
        name: '可靠性与备份',
        items: [
          {
            id: 'restore', name: '抽样恢复一次备份',
            cmd: 'ls -lht /data/backup | head -3',
            expect: '挑最新一份恢复到临时实例，校验行数与主库一致。没恢复过的备份等于没有备份。',
            critical: true,
          },
          {
            id: 'silence', name: '告警静默项复核',
            cmd: 'grep -rn "silence" /etc/alertmanager/ 2>/dev/null | head',
            expect: '没有过期但忘记解除的静默。静默是临时手段，长期静默等于关掉告警。',
          },
          {
            id: 'single', name: '单点组件盘点',
            cmd: 'kubectl get pods -A -o wide 2>/dev/null | awk \'$2 ~ /^1\\//\' | head -20',
            expect: '列出只有 1 副本的关键组件，逐个确认能否接受它挂掉的后果。',
          },
          {
            id: 'peak', name: '一周资源峰值',
            cmd: 'uptime',
            expect: '峰值不超过 70%，留出故障切换与突发流量的余量。',
          },
        ],
      },
      {
        id: 'security',
        name: '安全与访问',
        items: [
          {
            id: 'chain', name: '证书链完整性',
            cmd: "openssl s_client -connect localhost:443 -servername localhost -showcerts </dev/null 2>/dev/null | grep 'Verify return code'",
            expect: '返回 (0 (ok))。非 0 说明中间证书缺失，浏览器与 curl 的表现会不一致。',
            runbook: '#/runbook/cert-tls',
            runbookText: '证书问题',
          },
          {
            id: 'uid0', name: 'UID 0 账号复核',
            cmd: "awk -F: '$3==0{print $1}' /etc/passwd",
            expect: '只有 root 一个。多出来的必须是明确审批过的。',
          },
          {
            id: 'updates', name: '安全更新待办',
            cmd: 'dnf check-update --security -q 2>/dev/null | wc -l',
            expect: '有数字就排期，不要攒到下次版本升级一起做。',
          },
        ],
      },
    ],
  },

  {
    id: 'monthly',
    name: '月检',
    cadence: '每月一次 · 约半天',
    desc: '月检解决「体系问题」：容量规划、依赖生命周期、演练与复盘归档。多数事故的种子在这一层。',
    groups: [
      {
        id: 'plan',
        name: '规划与生命周期',
        items: [
          {
            id: 'forecast', name: '容量预测',
            cmd: 'df -h',
            expect: '按近 30 天增长推算剩余可用天数，低于 90 天就要提前扩。',
          },
          {
            id: 'eol', name: '依赖版本是否仍在支持期',
            cmd: 'uname -r && nginx -v 2>&1 && mysql -V',
            expect: '内核、中间件、数据库都不在 EOL 列表里。EOL 之后出的漏洞没有补丁。',
          },
          {
            id: 'monitor', name: '新服务监控覆盖',
            cmd: 'curl -s localhost:9090/api/v1/targets | head -c 400',
            expect: '本月新上线的服务都有指标与告警，没有「裸奔」的。',
          },
        ],
      },
      {
        id: 'drill',
        name: '演练与复盘',
        items: [
          {
            id: 'restore-drill', name: '完整恢复演练',
            cmd: '# 在隔离环境按备份恢复流程走一遍',
            expect: '记录恢复耗时（RTO）与丢失窗口（RPO），与承诺值对比。',
            critical: true,
          },
          {
            id: 'runbook-drill', name: '抽一个剧本走一遍',
            cmd: '# 挑一个高频场景，在测试环境按剧本逐步执行',
            expect: '剧本里的命令在当前版本上仍然有效；失效的步骤当天就改。',
            runbook: '#/runbook',
            runbookText: '排障剧本',
          },
          {
            id: 'postmortem', name: '本月复盘归档',
            cmd: '# 检查事故是否都有复盘，改进项是否都有负责人',
            expect: '每起 P1/P2 都有复盘，改进项有 owner 与截止时间，且真的在推进。',
            runbook: '#/channel/ops-postmortem',
            runbookText: '复盘栏目',
          },
        ],
      },
      {
        id: 'access',
        name: '权限与交接',
        items: [
          {
            id: 'rotate', name: '密钥与凭据轮换',
            cmd: '# 清理离职账号、过期 token、共享口令',
            expect: '没有离职人员仍可登录；共享口令已改；临时授权已回收。',
          },
          {
            id: 'handover', name: '值班手册与实际一致',
            cmd: '# 对照值班手册走一遍，把不适用的步骤删掉',
            expect: '手册里写的路径、命令、联系人全部有效。过期手册比没有手册更危险。',
          },
        ],
      },
    ],
  },
];

/** 按 id 取清单。 */
export function findChecklist(id) {
  return CHECKLISTS.find((c) => c.id === id) || null;
}

/** 某份清单的项总数，进度分母统一从这里出。 */
export function itemCountOf(list) {
  if (!list) return 0;
  return list.groups.reduce((sum, g) => sum + g.items.length, 0);
}

/** 全部清单的项数，首页/入口处展示用。 */
export const TOTAL_ITEMS = CHECKLISTS.reduce((n, c) => n + itemCountOf(c), 0);
