## 标签设计：Loki 成本与可用性的全部秘密

Loki 只索引标签。每多一个高基数标签，索引就多一份负担：

```yaml
# Promtail/Alloy 采集配置示例
scrape_configs:
- job_name: app
  static_configs:
  - targets: [localhost]
    labels:
      job: order-api          # 服务名（有界）
      env: prod               # 环境（有界）
      host: web-1             # 主机名（有界）
      __path__: /data/logs/app/*.log
```

**可以打的标签**：job、env、host、namespace、container 这类有界枚举。
**不能打的标签**：user_id、request_id、trace_id、URL 明细——
这些进日志正文，用 LogQL 过滤，不要进索引。

误把高基数标签打进去的表现：`ingester` 内存暴涨、查询越来越慢。
治理手段只有删标签重建流，所以上线前把标签清单评审一遍。

## 采集端：Promtail（旧）与 Alloy（新）

```yaml
# Promtail pipeline：从日志行里提取字段
pipeline_stages:
  - regex:
      expression: '(?P<time>\S+) (?P<level>\w+) (?P<msg>.*)'
  - labels:
      level:                    # level 是有界的，可以提为标签
  - timestamp:
      source: time
      format: RFC3339
```

K8s 场景直接用 Helm 里的 Grafana Alloy DaemonSet，自动带
`namespace / pod / container` 标签。

## 保留与压缩

```yaml
limits_config:
  retention_period: 720h        # 30 天
compactor:
  working_directory: /data/loki/compactor
  retention_enabled: true       # retention 生效必须显式开
```

日志量估算：日均 N GB × 保留天数 = 对象存储容量。
文件系统存储只适合验证，**生产接对象存储（COS/S3/MinIO）**，这是 Loki 成本优势的来源。

## 与告警集成

```yaml
# ruler 规则：错误率突增告警
ruler:
  storage: { type: local, directory: /data/loki/rules }
```

```yaml
# /data/loki/rules/fake/logs.yaml
groups:
  - name: app-errors
    rules:
      - alert: HighErrorRate
        expr: sum(count_over_time({job="order-api"} |= "ERROR" [5m])) > 100
        for: 5m
```

## 验收清单

- [ ] 标签基数有清单有评审，无 user/request 级标签
- [ ] 对象存储后端 + retention_enabled 开启
- [ ] Grafana 接入 Loki 数据源，日志能跳转 Trace
- [ ] 采集断流监控：`loki_distributor_lines_received_total` 速率掉零告警
- [ ] 查询延迟 P95 < 3s（超了先查标签设计，再谈扩容）