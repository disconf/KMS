## 配置文件在哪

| 组件 | 配置 |
|---|---|
| Loki 服务端 | 启动 `loki -config.file=loki-config.yaml` |
| Promtail 采集端 | `promtail -config.file=promtail-config.yaml` |
| Helm 部署 | values.yaml 转译成上述 yaml |

单体模式一个 yaml 搞定；微服务模式（读/写/后端分离）用同一份 yaml 配不同 `target:`。

## 参数怎么读

yaml 分区块：`server`（HTTP 端口）、`ingester`（写入内存）、`limits_config`（全局限额）、
`storage_config`/`schema_config`（存哪、怎么组织）。**limits_config 是运维重点**，
磁盘爆和拒写大多源于这里。

## 核心参数分组

**存储与保留**

```yaml
schema_config:
  configs:
    - from: 2026-01-01
      store: tsdb          # 新部署用 tsdb
      object_store: s3     # 或 filesystem
      schema: v13
limits_config:
  retention_period: 720h   # 保留 30 天
compactor:
  retention_enabled: true  # 保留期生效的前提（compactor 开启删除）
```

**容量保护（limits_config）**

```yaml
  ingestion_rate_mb: 10        # 每秒写入上限
  ingestion_burst_size_mb: 20
  max_query_series: 500        # 单查询序列上限，防大查询打挂
  reject_old_samples: true     # 拒绝太旧的时间戳，防时钟漂移灌垃圾
```

**采集端（Promtail）**

```yaml
scrape_configs:
  - job_name: system
    static_configs:
      - targets: [localhost]
        labels: { job: varlogs, host: ${HOSTNAME} }   # 标签在此定，克制！
    pipeline_stages:        # 解析、提取字段、改标签
      - regex: { expression: '(?P<level>ERROR|WARN)' }
      - labels: { level: '' }
```

## 常见误配

1. **用 IP/时间戳当标签**——基数爆炸，ingester 内存打满；
2. **retention_period 配了但 compactor 没开 retention_enabled**——数据永远不删，磁盘只涨；
3. **把正文字段提进标签**——pipeline 提取 level/app 这类低基数字段可以，
   提 user_id 等于自杀；
4. **单盘 filesystem 部署不做容量告警**——先配磁盘告警再谈别的；
5. **查询不限定时间范围**——LogQL 默认窗口拉全量，先框时间再查。

## 校验与生效

```bash
loki -verify-config -config.file=loki-config.yaml   # 配置校验
curl -s localhost:3100/ready                        # 就绪态
curl -s localhost:3100/metrics | grep loki_ingester_streams  # 流数量（基数监控）
curl -s localhost:3100/loki/api/v1/labels           # 实际标签集
```
