## PromQL 日常查询

```promql
# 存活与离线目标
up == 0
# CPU 使用率（node_exporter）
100 - avg by(instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100
# 内存可用
(node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100
# 磁盘使用率（按挂载点）
(1 - node_filesystem_avail_bytes{mountpoint="/data"} / node_filesystem_size_bytes{mountpoint="/data"}) * 100
# 服务 QPS 与错误率
sum(rate(http_requests_total{service="api"}[5m])) by (handler)
sum(rate(http_requests_total{service="api",code=~"5.."}[5m]))
  / sum(rate(http_requests_total{service="api"}[5m]))
# P95 延迟
histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le))
```

## API 用法（脚本/巡检程序用）

```bash
# 即时查询
curl -sG 'http://localhost:9090/api/v1/query' \
  --data-urlencode 'query=up == 0' | jq '.data.result[].metric.instance'

# 区间查询
curl -sG 'http://localhost:9090/api/v1/query_range' \
  --data-urlencode 'query=sum(rate(node_cpu_seconds_total{mode="user"}[5m]))' \
  --data-urlencode 'start=2026-10-26T00:00:00Z' \
  --data-urlencode 'end=2026-10-26T08:00:00Z' \
  --data-urlencode 'step=300s'

# 目标与告警状态
curl -s localhost:9090/api/v1/targets | jq '.data.activeTargets[] | select(.health!="up") | .labels'
curl -s localhost:9090/api/v1/alerts | jq '.data.alerts[] | select(.state=="firing") | .labels.alertname'
```

## TSDB 运维

```bash
promtool tsdb status /data/prometheus     # 离线分析数据目录
promtool check rules /etc/prometheus/rules/*.yml
promtool query instant http://localhost:9090 'up'        # 命令行查
curl -X POST localhost:9090/-/reload                     # 热加载
```

## 「查不到数据」排查清单

1. `up == 0`？先看 target 页：target 挂了还是 scrape timeout；
2. target 是 up 但查不到：**时间范围不对**（图表右上角的时间窗）；
3. exporter 单机正常但聚合查不到：标签拼写不一致（`job` / `instance` 大小写）；
4. rule 不触发：`promtool check rules` + `curl /api/v1/rules` 看规则是否加载、
   `for` 是否还没满足；
5. 数据在但内存压力大：查高基数指标（`count by (__name__)(... topk)`）。