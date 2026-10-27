## 单机 Compose 日常命令

```bash
cd /opt/cmdb-src/deploy/single-node

sudo ./scripts/bootstrap.sh      # 一键初始化（首装）
sudo ./scripts/verify.sh         # 全组件健康自检
sudo ./scripts/backup.sh [标签]   # 备份 PG/MinIO/Vault/Neo4j/OpenSearch 到 /opt/cmdb/backups/
sudo ./scripts/update.sh         # git pull 后升级
sudo ./scripts/unseal-vault.sh   # 主机重启后必做：解封 Vault

# 容器状态与日志
sudo docker compose --env-file .env -f docker-compose.yml ps
sudo docker compose --env-file .env -f docker-compose.yml logs -f gateway web

# 查初始管理员（密码在 .env 里，首次登录后去 Keycloak 重置）
sudo grep -E '^CMDB_ADMIN_(USERNAME|PASSWORD)=' .env
```

备份产物在 `/opt/cmdb/backups/<label>/`：`postgres.sql`、`minio.tar.gz`、
`vault.tar.gz`、`neo4j.tar.gz`、`opensearch.tar.gz`、`env.backup`。

## Kubernetes 日常命令

```bash
# 健康与容量
kubectl -n ops-qa get pods,svc,pvc
kubectl -n ops-qa get endpoints gateway web
kubectl -n monitoring get pods
curl -fsS http://<gateway>:30080/api/v1/health
curl -fsS http://<gateway>:30080/api/v1/ready

# 升级与回滚
kubectl -n ops-qa set image deployment/gateway gateway=<registry>/cmdb/gateway-bff:<new>
kubectl -n ops-qa rollout status deployment/gateway --timeout=300s
kubectl -n ops-qa rollout undo deployment/gateway

# 节点维护（逐台）
kubectl drain <node> --ignore-daemonsets --delete-emptydir-data
sudo reboot
kubectl uncordon <node>
```

## Vault 运维

```bash
kubectl -n ops-qa exec -it vault-0 -- vault operator init            # 首次，密钥离线保存
kubectl -n ops-qa exec -it vault-0 -- vault operator unseal <KEY_N>  # 解封（3 把中的任意 3 把/阈值）
kubectl -n ops-qa exec -it vault-0 -- vault login <ROOT_TOKEN>
kubectl -n ops-qa exec -it vault-0 -- vault secrets enable -path=secret -version=2 kv
```

单机版解封密钥在 `secrets/vault-init.json`，离线备份。

## 排障速查表

| 现象 | 排查顺序 |
|---|---|
| Gateway 全部 401 | `KEYCLOAK_ISSUER_URL` 与浏览器登录地址是否逐字符一致 |
| Gateway Ready 失败 | 查 `DATABASE_URL`、Redis、Kafka、Keycloak 连通性与 Secret |
| PG 启动失败/权限错误 | 数据目录是否本地文件系统、属主是否 UID 999 |
| 知识附件上传失败 | MinIO Health、`S3_*` 配置、Bucket 权限 |
| 任务执行失败 | Runner Token、SSH 密钥、known_hosts、目标主机权限 |
| 监控目标为空 | `AGENT_SHARED_TOKEN`、Prometheus HTTP SD 配置 |
| Vault Ready 失败 | 是否已解封 |
| 单机重启后业务不可用 | 先 `unseal-vault.sh`，再看容器 |
| K8s PVC Pending | Longhorn、StorageClass、节点磁盘、副本数 |
| 滚动更新探针偶发失败 | 节点负载、CPU/内存限制、`/api/v1/ready` 耗时 |
