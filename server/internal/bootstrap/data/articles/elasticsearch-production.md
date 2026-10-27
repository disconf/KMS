## 角色分离：什么规模的集群都该做的事

```yaml
# 数据节点
node.roles: [ data, ingest ]
# 专用 master（3 台小规格即可）
node.roles: [ master ]
```

小集群（3 节点内）可以混合角色；**一旦超过 5 节点或写入量大，master 一定独立**，
否则一次大查询能把 master 选举搞挂。

## 分片规划：先算账再建索引

- 单分片建议 20~50GB，上限 50GB；
- 分片总数 ≈ 数据量 / 30GB，且不能超过 `数据节点数 × 20`（软上限）；
- 日志类索引按天滚动，用 ILM 自动管理：

```json
PUT _ilm/policy/logs_policy
{
  "policy": {
    "phases": {
      "hot":    { "actions": { "rollover": { "max_primary_shard_size": "50gb", "max_age": "1d" } } },
      "warm":   { "min_age": "3d", "actions": { "shrink": { "number_of_shards": 1 } } },
      "delete": { "min_age": "30d", "actions": { "delete": {} } }
    }
  }
}
```

没有 ILM 的日志集群，三个月后磁盘就会教做人。

## 快照备份（必须有仓库）

```json
PUT _snapshot/backup
{ "type": "fs", "settings": { "location": "/data/es-backup" } }
PUT _snapshot/backup/snap-2026.10.26?wait_for_completion=true
```

`path.repo` 先写进 elasticsearch.yml。快照是**增量**的，每天跑不贵，
但恢复演练必须做过——只备不演 = 没备。

## 安全配置

1. `xpack.security.enabled: true` 后第一件事改 elastic 内置账号密码；
2. 业务访问用独立账号 + 角色限定索引前缀（`logs-*` 只读）；
3. 节点间通信开 TLS（`bin/elasticsearch-certutil` 生成，8.0 起向导自动配）；
4. 9200 不对公网，必要走 Nginx 加一层鉴权。

## 验收清单

- [ ] 集群 health 为 green，`_cat/nodes` 角色分布符合规划
- [ ] ILM 生效：`_cat/indices` 里没有超过保留期的索引
- [ ] 每日快照成功且异机存储，恢复演练做过
- [ ] JVM 堆使用率 < 75%，GC 日志无长暂停
- [ ] 安全：内置密码已改、TLS 开启、无匿名访问