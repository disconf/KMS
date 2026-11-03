## 适用边界

单机 Docker Compose 是最快的上手方式：一台 32C/64GB 级别的主机跑全套
（PG、Redis、Kafka、Keycloak、Vault、MinIO、Neo4j、OpenSearch、监控全家桶、
Gateway/Web/Runner）。**它没有节点级高可用**——单 PG、单 Redis、单 Kafka、单 OpenSearch，
主机一挂全停，不建议承载核心生产或强 SLA 业务；生产请走 Kubernetes 或外部高可用中间件。

部署包在仓库 `deploy/single-node/`，包含全部组件的 Compose 清单。
Rancher、K8s、Argo CD、Harbor、GitLab、外部 LLM 是集成目标，不在 Compose 内启动。

## 端口规划

| 端口 | 组件 | 暴露范围 |
|---:|---|---|
| 8088 | Web Portal | 业务网或反向代理 |
| 8080 | Gateway | 仅反向代理/内网 |
| 8081 | Keycloak | 业务网或统一入口 |
| 5432 / 6379 / 9092 | PostgreSQL / Redis / Kafka | 仅平台内网 |
| 8200 | Vault | 仅平台内网或运维网 |
| 9000/9001 | MinIO API/Console | API 仅内网；Console 仅运维网 |
| 7474/7687 | Neo4j | 仅平台内网 |
| 9200 | OpenSearch | 仅平台内网 |
| 9090/9093/3000 | Prometheus / Alertmanager / Grafana | 运维网（SSH 隧道） |

生产建议统一域名 + HTTPS 反向代理，对外只开 443。

## 第一步：操作系统初始化

以 Ubuntu 22.04、`/dev/sdb` 作为数据盘为例（**执行前确认磁盘设备名，误格式化不可逆**）：

```bash
sudo hostnamectl set-hostname cmdb-single
sudo timedatectl set-timezone Asia/Shanghai
sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg jq openssl openssh-client chrony
sudo systemctl enable --now chrony

# 数据盘独立 XFS，挂到 /opt/cmdb
sudo mkfs.xfs /dev/sdb
sudo mkdir -p /opt/cmdb
echo 'UUID='$(sudo blkid -s UUID -o value /dev/sdb)' /opt/cmdb xfs defaults,noatime 0 2' | sudo tee -a /etc/fstab
sudo mount -a

# Docker + Compose 插件（官方源）
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
```

PostgreSQL 数据目录**禁止 NFS/CIFS/SMB/FUSE**，只能本地 ext4/XFS。

## 第二步：配置与一键初始化

```bash
sudo rsync -a --delete /path/to/CMDB/ /opt/cmdb-src/
cd /opt/cmdb-src/deploy/single-node
sudo cp .env.example .env
sudo vi .env
```

`.env` 至少改三项地址（其余密码 `CHANGE_ME_*` 不用手动填，
`bootstrap.sh` 会自动生成随机密钥）：

```dotenv
SERVER_HOST=192.168.10.20
KEYCLOAK_PUBLIC_URL=http://192.168.10.20:8081
CMDB_PUBLIC_URL=http://192.168.10.20:8088
CMDB_DATA_DIR=/opt/cmdb
```

```bash
sudo ./scripts/bootstrap.sh
```

脚本做的事：生成全套随机密钥 → 建 `/opt/cmdb` 并修正 PG 权限 →
生成 `secrets/*.env` 与 Agent 共享令牌 → 起中间件与监控 → 初始化并解封 Vault →
创建 Keycloak Realm `cmdb`、客户端 `cmdb-web` 与初始管理员 → 起业务服务 → 等 Gateway Ready。

初始管理员账号密码写回 `.env`，**不要在终端/聊天里传播，首次登录后去 Keycloak 重置**。

## 第三步：校验与访问

```bash
sudo docker compose --env-file .env -f docker-compose.yml ps
sudo ./scripts/verify.sh
```

平台 `http://IP:8088`、Keycloak `http://IP:8081`；Grafana/Prometheus 只绑回环，
用 SSH 隧道：`ssh -L 3000:127.0.0.1:3000 -L 9090:127.0.0.1:9090 ops@服务器IP`。

## 日常运维三件事

```bash
# 主机重启后（顺序不能反：先起容器，再解封 Vault）
sudo docker compose --env-file .env -f docker-compose.yml up -d
sudo ./scripts/unseal-vault.sh
sudo ./scripts/verify.sh

# 升级（先备份！）
sudo ./scripts/backup.sh before-v1.0.232
cd /opt/cmdb-src && sudo git pull --ff-only
cd deploy/single-node && sudo ./scripts/update.sh

# 备份产物
/opt/cmdb/backups/<label>/{postgres.sql,minio.tar.gz,vault.tar.gz,neo4j.tar.gz,opensearch.tar.gz,env.backup}
```

回滚：`.env` 把 `CMDB_VERSION` 改回旧版 → `docker compose up -d`；
迁移不兼容时停写并用 `postgres.sql` 恢复。解封密钥在 `secrets/vault-init.json`，**必须离线备份**；
备份产物必须复制到另一台机器或对象存储，不能和数据同盘。
