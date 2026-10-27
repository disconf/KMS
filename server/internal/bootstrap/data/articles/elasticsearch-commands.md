## _cat API：运维的第一入口

```bash
# 集群概览
curl -s 'localhost:9200/_cat/health?v'
curl -s 'localhost:9200/_cat/nodes?v&h=name,heap.percent,disk.used_percent,cpu,role'
curl -s 'localhost:9200/_cat/indices?v&s=store.size:desc' | head -15
curl -s 'localhost:9200/_cat/shards?v&h=index,shard,prirep,state,store,node' | grep -v STARTED
curl -s 'localhost:9200/_cat/allocation?v'
curl -s 'localhost:9200/_cat/thread_pool/search?v&h=node_name,active,queue,rejected'
```

## 集群健康诊断

```bash
curl -s 'localhost:9200/_cluster/health?pretty' | grep -E 'status|unassigned|delayed'
# 看未分配分片的原因（红/黄的第一现场）
curl -s 'localhost:9200/_cluster/allocation/explain?pretty' | grep -A5 'explanation'
# 磁盘水位导致只读时（ flood_stage 触发 index.blocks.read_only_allow_delete）
curl -s -XPUT 'localhost:9200/logs-2026.10.26/_settings' \
  -H 'Content-Type: application/json' -d '{"index.blocks.read_only_allow_delete": null}'
```

## 查询 DSL 速查

```bash
# 精确匹配 + 范围
curl -s 'localhost:9200/logs-*/_search?pretty' -H 'Content-Type: application/json' -d '{
  "query": {"bool": {"must": [{"match": {"service": "api"}}],
    "filter": [{"range": {"@timestamp": {"gte": "now-15m"}}}]}},
  "size": 5, "sort": [{"@timestamp": "desc"}]}'

# 聚合：按服务统计错误数
curl -s 'localhost:9200/logs-*/_search' -H 'Content-Type: application/json' -d '{
  "size": 0, "aggs": {"by_svc": {"terms": {"field": "service"},
    "aggs": {"errs": {"filter": {"term": {"level": "ERROR"}}}}}}}'

# 全文检索高亮
curl -s 'localhost:9200/logs-*/_search' -d '{"query":{"match":{"message":"timeout"}},
  "highlight":{"fields":{"message":{}}}}'
```

## 集群变黄 / 变红的处理

1. **黄**：副本分片未分配。通常是节点数不足或磁盘水位，`_cat/allocation` 看哪台满了；
2. **红**：主分片丢失。`_cluster/allocation/explain` 看原因，
   数据节点真挂了等它回来，确认不要了用 `_cluster/reroute` 的 `allocate_stale_primary`；
3. 磁盘满导致只读：清索引/扩容后重置 `read_only_allow_delete`；
4. 大查询拖垮集群：`_cat/thread_pool` 看 rejected，找出慢查询来源并限制并发。