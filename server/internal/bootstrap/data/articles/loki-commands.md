## LogQL 查询语法

```bash
# 基本选择：标签 + 行过滤
{job="order-api"}
{job="order-api", env="prod"} |= "timeout"      # 包含
{job="order-api"} != "healthcheck"              # 排除
{job="order-api"} |~ "level=(error|fatal)"      # 正则匹配
{job="order-api"} | logfmt                       # 结构化解析
{job="order-api"} | json | level="error"        # JSON 行解析后过滤

# 计数与聚合
sum(count_over_time({job="order-api"} |= "ERROR" [5m]))       # 5 分钟错误条数
sum by (host) (count_over_time({job=~"api.*"} [1h]))
rate({job="access-log"} | logfmt | status>=500 [5m])          # 5xx 速率

# 从行里提取数字做聚合（延迟分析）
quantile_over_time(0.95,
  {job="order-api"} | json | unwrap duration_ms [5m]) by (host)
```

## 命令行 logcli

```bash
logcli query '{job="order-api"} |= "ERROR"' --since=1h --limit=50
logcli query --output=raw '{job="order-api"}' --from="2026-10-26T09:00:00+08:00" \
  --to="2026-10-26T10:00:00+08:00" > errors.log       # 拉一段日志到本地
logcli series --match='{job="order-api"}'             # 这个流有哪些标签组合
logcli labels job                                      # 所有 job 值
logcli stats '{job="order-api"}' --since=24h           # 流量统计
logcli instant '{job="order-api"} |= "panic"'          # 单点即时查询
```

`--addr` 指定 Loki 地址：`logcli --addr=http://loki:3100 ...`。

## API 直调

```bash
curl -sG 'http://loki:3100/loki/api/v1/query_range' \
  --data-urlencode 'query={job="order-api"} |= "ERROR"' \
  --data-urlencode 'since=1h' --data-urlencode 'limit=100' | jq '.data.result[].values'

curl -s 'http://loki:3100/loki/api/v1/labels' | jq '.data[]'   # 全部标签名
curl -s 'http://loki:3100/metrics' | grep -E 'received_total|reject'   # 摄入与拒绝
```

## 「日志查不到」排查清单

1. `logcli labels job`：标签值和你写的一致吗（大小写、路径差异）；
2. 时间范围：Loki 按时间分区，`--since` 覆盖到没有数据的时间段自然为空；
3. 采集端活着吗：看 Promtail/Alloy 的 `positions.yaml` 前进没、target 是不是 down；
4. 查询打到 `Ingester` 了吗：刚写入的数据有短暂不可查窗口（默认未压缩前不暴露）；
5. 流太多导致查询超时：加标签收窄范围，或给查询加 `| json` 前先 `|= "关键字"` 行过滤。