## 配置文件在哪

| 组件 | 配置 |
|---|---|
| prometheus | 启动参数 `--config.file=prometheus.yml`（默认工作目录） |
| 告警规则 | `rule_files:` 指向的 `*.yml` |
| Alertmanager | `alertmanager.yml` |
| systemd | `--storage.tsdb.*` 系列启动参数（不能写在 yml 里） |

yml 配置支持热加载：`kill -HUP <pid>` 或 `POST /-/reload`（需 `--web.enable-lifecycle`）。

## 参数怎么读

启动参数管存储与运行时（内存、保留期），yml 管抓取与规则——**保留期/内存这类
关键参数在命令行不在 yml**，是新手最常见的找错地方。

## 核心参数分组

**存储与保留（命令行）**

```bash
--storage.tsdb.retention.time=15d     # 本地保留期，按内存磁盘算
--storage.tsdb.no-lockfile
--web.enable-lifecycle                # 开热加载
```

**抓取（prometheus.yml）**

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
rule_files: ["rules/*.yml"]
scrape_configs:
  - job_name: node
    static_configs:
      - targets: ['10.0.0.1:9100']
  - job_name: app
    file_sd_configs: [{ files: ['sd/*.json'] }]   # 文件服务发现
```

**告警规则分层**

```yaml
groups:
  - name: instance
    rules:
      - alert: InstanceDown
        expr: up == 0
        for: 2m            # 持续 2 分钟才告警，防抖
        labels: { severity: critical }
        annotations: { summary: "{{ $labels.instance }} 掉线" }
```

## 常见误配

1. **标签基数爆炸**——把 user_id/trace_id 当标签，时序数百万条直接 OOM；
   标签原则：可枚举、低基数；
2. **保留期与磁盘不匹配**——按「每样本 1-2 字节 × 活跃序列 × 保留秒」估算，
   `prometheus_tsdb_head_series` 是现成依据；
3. **告警不加 for**——抖动直接打群里，告警疲劳比不告警更糟；
4. **只改 yml 忘 reload**——配置不生效还以为规则写错；
5. **rate 用在 gauge 上**——rate 只对 counter 有意义，gauge 用 avg_over_time。

## 校验与生效

```bash
promtool check config prometheus.yml    # 配置与规则校验，CI 里也该跑
promtool check rules rules/*.yml
curl -s localhost:9090/-/healthy
curl -s localhost:9090/api/v1/targets | jq '.data.activeTargets[] | {job,health}'  # 抓取健康
```
