## 备份体系

- **PostgreSQL**：`scripts/production/backup-postgres.sh`，基础备份进对象存储
  （如 `s3://postgres-backup/postgres-ha-<日期>/`）+ WAL 持续归档；恢复用
  `restore-postgres.sh backups/xxx.sql.gz`。备份文件同时生成 SHA256 校验，
  **恢复前必须先在隔离环境验证**。
- **单机 Compose**：`sudo ./scripts/backup.sh [标签]`，一次打包
  `postgres.sql / minio.tar.gz / vault.tar.gz / neo4j.tar.gz / opensearch.tar.gz / env.backup`
  到 `/opt/cmdb/backups/<label>/`——**必须复制到另一台机器或对象存储，不能与数据同盘**。
- **Vault**：Raft 3 副本；解封密钥与 Root Token 离线保存（不进 Git）。

## 自动化恢复演练：dr-drill.sh

```bash
bash scripts/production/dr-drill.sh          # 全量恢复演练
SCHEMA_ONLY=1 bash scripts/production/dr-drill.sh   # 快速结构演练
NAMESPACE=ops-qa POD=postgres-ha-2 SOURCE_DB=ops_platform bash scripts/production/dr-drill.sh
```

脚本流程：主库导出 + SHA256 → 建临时库（随机后缀）→ 恢复 → 校验 public schema
表数与 `cmdb_assets` 记录数 → 自动清理。**生产库全程不被触碰**。
成功输出 `DR_DRILL_SUCCESS backup=... tables=... assets=... schema_only=0`。
建议每月至少一次，保留备份与 SHA256 文件。

## 上线前：生产就绪检查

「系统管理 → 生产就绪」逐项确认 PostgreSQL、pgvector、Redis、Kafka、Keycloak、Vault、
MinIO/S3、Prometheus、Alertmanager、Argo CD、Rancher。**阻断项必须全部解决**；
Neo4j/OpenSearch 已部署并通过检查，Loki/Thanos/Tempo 等可选项缺失只显示提示。

## 故障场景演练清单

| 场景 | 验证目标 |
|---|---|
| 停一个网关副本 | 服务仍可用（多副本分流） |
| Redis 节点故障 | 会话与远程终端降级策略生效 |
| Kafka broker 故障 | Outbox 积压在恢复后自动追平 |
| Prometheus/Loki 不可用 | 网关核心写入不受影响 |
| PG 主库故障 | 从备份/副本恢复到 3/3 Ready（RTO/RPO 记录进演练报告） |

## 压测与安全冒烟基线

```bash
k6 run -e BASE_URL=... -e ACCESS_TOKEN=... scripts/production/load-test-assets.js
k6 run -e BASE_URL=... -e AGENT_SHARED_TOKEN=... scripts/production/load-test-agent-heartbeat.js
VIEWER_TOKEN=... ADMIN_TOKEN=... bash scripts/production/security-smoke.sh
```

目标：资产查询 P95 < 800ms、错误率 < 1%；Agent 心跳 100 次/秒持续 2 分钟、
错误率 < 1%；未认证访问被拒、只读用户不能创建任务、管理员可访问生产就绪报告。

## 一次真实的恢复记录（2026-10-19）

- PostgreSQL 从 1/3 Ready 恢复到 3/3 Ready（新主库 + 两副本），备份目标切到
  `s3://postgres-backup/postgres-ha-20261019/`，旧前缀保留追溯；
- Vault `vault-2` 解封后集群回到 3/3 Ready；
- 恢复前的逻辑备份留存为 `ops-platform-pre-replica-rebuild-20261019.dump`，
  使用前在隔离环境完成 `pg_restore` 验证。

每次演练都产出同样的三件套：RTO/RPO 数字、演练报告、待修问题清单。
