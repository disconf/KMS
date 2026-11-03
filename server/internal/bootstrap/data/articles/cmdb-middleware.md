## 总原则：CMDB 是唯一底账

PostgreSQL 是资产主数据；Neo4j 只做关系图投影（`(:Asset)-[:RELATED]->(:Asset)`，
支撑拓扑与影响面分析），OpenSearch 只做资产全文检索（索引 `cmdb-assets`）。
两者都是**可重建的副本**——丢了不慌，网关一键重建投影即可，不存在第二个「真相来源」。

## 组件清单与资源

| 组件 | 作用 | 单机 Compose | K8s 生产 |
|---|---|---|---|
| PostgreSQL + pgvector | 资产主数据、知识库向量 | 单实例 | CloudNativePG 3 实例 + WAL 归档 |
| Redis | 缓存/会话/锁 | 单实例 | Cluster 3 主 3 从 |
| Kafka | 领域事件总线 | 单 broker | KRaft 3+3，每 broker ≥200GiB |
| MinIO | 附件与备份对象存储 | 单实例 | Operator Tenant ≥4 盘 |
| Neo4j | 关系图投影 | 单实例（7474/7687） | StatefulSet，10Gi 数据 + 2Gi 日志卷 |
| OpenSearch | 资产全文检索 | 单实例（9200） | StatefulSet，20Gi 卷 |

## Neo4j 与 OpenSearch 的四个部署细节（都是踩过的坑）

1. **`enableServiceLinks: false` 必须设置**——K8s 注入的 Service 环境变量会被 Neo4j
   当成配置项解析，直接 `CrashLoopBackOff`；
2. **OpenSearch 要 `fsGroup/runAsUser/runAsGroup: 1000`**——Longhorn 卷属主不匹配
   会导致容器无法写入；
3. **单节点 OpenSearch 用 `discovery.type=single-node`，索引副本数设 0**——
   单节点配 1 副本索引会长期 yellow；
4. **镜像先入库内 Harbor**（Neo4j 5.26-community 约 335MiB、OpenSearch 2.19.1 约 981MiB），
   从可联网节点 pull → tag → push，集群节点不必访问外网。

## 网关如何连接

凭据经 Secret `cmdb-gateway-env` 以 `envFrom` 注入（YAML 里不写明文）：

```text
NEO4J_URL=http://neo4j.ops-qa.svc.cluster.local:7474
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=***
OPENSEARCH_URL=http://opensearch.ops-qa.svc.cluster.local:9200
```

## 投影重建与验证

```bash
# 管理员令牌触发全量重建（先清 :RELATED 再按当前关系重建）
curl -X POST http://<gateway>:30080/api/v1/system/projections/rebuild \
  -H "Authorization: Bearer $TOKEN"
# => {"assets":567,"neo4j":567,"openSearch":567}

curl -H "Authorization: Bearer $TOKEN" http://<gateway>:30080/api/v1/system/projections
curl 'http://opensearch:9200/_cat/indices/cmdb-assets?v'
kubectl -n ops-qa exec neo4j-0 -- cypher-shell -u neo4j -p '***' "MATCH (a:Asset) RETURN count(a);"
```

系统管理中心「拓扑投影」页签可看配置地址与投影数量，并提供重建按钮。

## 投影实现要点

- OpenSearch 走 `_bulk` 批量写入：资产 ID 形如 `k8s-pod/ops-qa/gateway-xxxx`，
  含 `/` 直接拼 URL 会被解析成多级路径报 `400 Bad Request`；
- Neo4j 用 `UNWIND` 批量事务；唯一约束 `asset_id_unique` 必须在**独立事务**创建，
  和写事务混用会触发 `ForbiddenDueToTransactionType`；
- 写入失败会把下游原始响应正文带回接口，便于定位。

## 故障排查速查

| 现象 | 原因 | 处理 |
|---|---|---|
| 重建报 `opensearch returned 400` | 资产 ID 拼进了 URL | 升级到 `_bulk` 版网关 |
| 重建报 `ForbiddenDueToTransactionType` | 约束与写语句同事务 | 约束单独提交 |
| Neo4j `CrashLoopBackOff` | Service 环境变量被解析 | `enableServiceLinks: false` |
| OpenSearch 启动权限错误 | 卷属主 root | `fsGroup/runAsUser/runAsGroup: 1000` |
| 索引长期 yellow | 单节点 + 1 副本 | 副本数改 0 |
| PostgreSQL 启动失败/权限错误 | 数据目录不在本地文件系统或属主不是 UID 999 | 本地 ext4/XFS + 修属主 |
