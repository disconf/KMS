## 容量：保留期与内存怎么算

每秒采集样本数 ≈ 活跃序列数 / 采集间隔。内存粗算：
`内存 ≈ 每秒样本 × 保留天数 × 2 字节 × 冗余`。16G 内存的机器扛 100 万活跃序列 +
15s 间隔 + 15d 保留已经接近极限，**先算再买机器**。

```bash
--storage.tsdb.retention.time=15d      # 时间上限
--storage.tsdb.retention.size=400GB    # 磁盘上限，两者取先到
--query.max-samples=50000000           # 防止大查询打爆内存
```

长期数据（月级趋势）交给远端存储：Thanos / VictoriaMetrics / Mimir，
Prometheus 本地只当实时层。

## 联邦 vs 远端存储

- **联邦（federation）**：上级 Prometheus 拉下级的聚合指标，适合多机房汇聚，
  不要用来搬原始明细；
- **远端存储**：`remote_write` 流式推送，查询聚合在远端做，是主流方案。

## 告警规则分层

```yaml
groups:
- name: base
  rules:
  - alert: InstanceDown
    expr: up == 0
    for: 3m                          # 消抖：瞬断不报警
    labels: { severity: critical }
    annotations:
      summary: "{{ $labels.instance }} 采集断开"
- name: business
  rules:
  - alert: OrderErrorRateHigh
    expr: sum(rate(http_requests_total{code=~"5.."}[5m])) by (service)
        / sum(rate(http_requests_total[5m])) by (service) > 0.01
    for: 5m
```

原则：**基础设施告警看 for，业务告警看率**。没有 `for` 的告警规则上线一周就会被当成狼来了。

## 基数治理（生产事故第一来源）

```bash
# Top 10 序列数最多的指标
topk(10, count by (__name__)({__name__=~".+"}))
# 某个指标的标签基数
count(http_request_duration_seconds_bucket) 
```

把 `user_id`、`trace_id`、URL 明细打进标签就是内存定时炸弹。
标签必须**有界**：状态码、方法、实例这类枚举值。

## 验收清单

- [ ] `up == 0` 告警能收到（人为停一个 exporter 演练）
- [ ] 保留期 / 磁盘上限配置生效，`tsdb status` 页能看到真实用量
- [ ] 高基数巡检每周跑，Top 指标有 owner
- [ ] remote_write 断网演练：本地缓冲 `--storage.tsdb.out-of-order-time-window` 评估过