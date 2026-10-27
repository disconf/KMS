## 生产形态：Kubernetes 是唯一推荐

单机 Compose 是单 PostgreSQL、单 Redis、单 Kafka、单 OpenSearch，没有节点级高可用，
Vault 还要人工解封——只适合 POC。生产用 K8s：控制面 3×8C/16G，Worker 3×16C/32G 起步
（NVMe 数据盘 500G + 2T），Longhorn 3 副本存储。参考环境实际规模更大：
3×8C16G + 5×32C64G，PVC 申请约 2.9TB。

## 关键中间件的生产形态

| 组件 | 生产要求 |
|---|---|
| PostgreSQL | CloudNativePG 3 实例，每日基础备份到对象存储 + WAL 归档；数据目录禁 NFS |
| Redis | 3 主 3 从 Cluster，密码进 Secret，开持久化 |
| Kafka | KRaft 3 broker + 3 controller，每 broker ≥200G 数据盘；九个领域事件 topic 预建 |
| Keycloak | 2 副本 + 外部 PostgreSQL，启用 health/metrics |
| Vault | Raft 3 副本；解封密钥与 Root Token **离线保存，绝不进 Git** |
| MinIO | Operator Tenant ≥4 盘、100GiB/盘，独立 StorageClass，TLS + 密钥走 Secret |
| 控制面 | 打 taint 不跑业务；VIP + Keepalived/HAProxy 提供 apiserver 高可用 |

## 升级纪律

```bash
# K8s：先备份，后滚动，保留旧镜像
kubectl -n ops-qa set image deployment/gateway gateway=harbor.example.com/cmdb/gateway-bff:<new>
kubectl -n ops-qa rollout status deployment/gateway --timeout=300s
kubectl -n ops-qa rollout undo deployment/gateway   # 回滚

# 单机：改 .env 的 CMDB_VERSION 为旧版 + compose up -d
# 迁移不兼容时：停写 → backups/<label>/postgres.sql 恢复
```

数据库升级前必做：触发 PG 基础备份、确认 WAL 归档正常、测试环境验证迁移、
记录当前镜像版本、**回滚目标不用 latest**。
操作系统内核升级走逐台排空：`kubectl drain` → 升级 → `reboot` → `uncordon`。

## 备份与恢复目标

| 数据 | 方式 | RPO | RTO |
|---|---|---:|---:|
| PostgreSQL | 每日全备 + WAL 归档 | ≤15 分钟 | ≤2 小时 |
| MinIO | 版本化或跨桶复制 | ≤1 小时 | ≤2 小时 |
| Vault | Raft 快照 | ≤1 小时 | ≤1 小时 |
| Neo4j | 每日 dump | ≤24 小时 | ≤4 小时 |
| OpenSearch | 快照仓库 | ≤24 小时 | ≤4 小时 |

**备份必须复制到另一台服务器或对象存储**，唯一备份放同一块磁盘等于没备份。
至少每季度做一次恢复演练并记录耗时。

## 安全基线

- 证书提前 30 天续期；PG/Redis/Kafka/MinIO/Neo4j/Vault Token 每 90 天轮换；
- Agent 共享令牌与 Runner Token 轮换后要同步 Gateway、Agent、Runner 三方；
- Root Token 只用于初始化，日常用受限策略 Token；
- 高危命令走审批，命令黑白名单 + 执行前快照 + 全量审计是平台内置的，别绕过。

## 验收清单（上线前逐项打勾）

- [ ] 时间同步正常、证书有效期 >30 天、数据目录在本地 XFS/ext4
- [ ] PG/Redis/Kafka/Keycloak/Vault/MinIO 健康，Neo4j/OpenSearch 可连接
- [ ] Prometheus 抓到 Gateway/PG/Redis/主机指标，Alertmanager webhook 通
- [ ] Loki/Tempo/Grafana 有数据
- [ ] Web/Gateway/Runner Ready，Keycloak 登录与角色、项目组声明正常
- [ ] 资产、Agent、任务、工单、发布、知识库核心流程通过
- [ ] 备份文件已复制到独立存储，完成一次恢复演练
