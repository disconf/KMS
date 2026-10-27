## 部署形态怎么选

| 形态 | 最低资源 | 适用 | 结论 |
|---|---|---|---|
| 本地开发 | 笔记本即可 | 二次开发、联调 | 无外部依赖时自动用内存数据 |
| 单机 Docker Compose | 16C / 64GB / 1TB NVMe | POC、测试、演示、边缘节点 | 功能完整但无高可用，不建议承载核心生产 |
| Kubernetes | 3×8C16G（控制面）+ 3×16C32G（Worker）+ 2TB 存储 | 生产 / 准生产 | 推荐，配合 Longhorn + CloudNativePG |

## 方式一：本地开发启动

需要 Node.js 24+、npm 11+、Go 1.26+：

```powershell
# 后端（默认监听 8080，未配置 PostgreSQL 时使用内存数据）
cd apps/gateway-bff
go run ./cmd/server

# 前端（Vite 代理 /api 到本地 Go 服务）
cd apps/web-portal
npm install
npm run dev
# 浏览器访问 http://localhost:5173
```

本地内存验证账号：`admin/admin123`（管理员）、`viewer/viewer123`（只读）。
线上环境不走这两个账号，统一 Keycloak。

## 方式二：单机 Docker Compose 一键部署

部署包在仓库 `deploy/single-node/`，包含业务三件套（Gateway/Web/Runner）+
核心中间件（PostgreSQL/Redis/Kafka/Keycloak/Vault/MinIO/Neo4j/OpenSearch）+
可观测全家桶（Prometheus/Alertmanager/Grafana/Loki/Tempo）+ 各类 Exporter。
Rancher、Argo CD、Harbor、GitLab、外部 LLM 是集成目标，不在 Compose 内启动。

```bash
# 1. 操作系统初始化（Ubuntu 22.04，数据盘独立挂到 /opt/cmdb，XFS）
sudo hostnamectl set-hostname cmdb-single
sudo timedatectl set-timezone Asia/Shanghai
sudo mkfs.xfs /dev/sdb   # 先 lsblk 确认盘符，别格式化错了

# 2. 安装 Docker 与 Compose 插件（官方源，略）

# 3. 准备配置
cd /opt/cmdb-src/deploy/single-node
sudo cp .env.example .env
sudo vi .env
# 至少改：SERVER_HOST / KEYCLOAK_PUBLIC_URL / CMDB_PUBLIC_URL / CMDB_DATA_DIR
# 不要继续用 .env.example 里的 CHANGE_ME_* 占位密码

# 4. 一键初始化：生成随机密钥、起中间件、初始化并解封 Vault、
#    创建 Keycloak Realm/角色/初始管理员、启动业务服务
sudo ./scripts/bootstrap.sh

# 5. 校验
sudo docker compose --env-file .env -f docker-compose.yml ps
sudo ./scripts/verify.sh
```

初始管理员账号密码写回 `.env`（`CMDB_ADMIN_USERNAME/PASSWORD`），
首次登录后应在 Keycloak 里重置。

**重启后必做**：Vault 是文件存储且默认封存，主机重启后必须先解封再验：

```bash
sudo docker compose --env-file .env -f docker-compose.yml up -d
sudo ./scripts/unseal-vault.sh
sudo ./scripts/verify.sh
```

Grafana/Prometheus 只绑本机回环，用 SSH 隧道访问：
`ssh -L 3000:127.0.0.1:3000 -L 9090:127.0.0.1:9090 ops@服务器IP`。

## 方式三：Kubernetes 部署

集群规划：控制面 3 节点（8C/16G/200G）+ Worker 若干（生产建议 16C/32G 起步、NVMe 数据盘），
控制面打 taint 不跑业务。软件栈：containerd + kubeadm v1.28 + Calico + Longhorn。

部署顺序（清单都在仓库 `deploy/` 下）：

```bash
# 基础平台组件
helm upgrade --install longhorn longhorn/longhorn -n longhorn-system --create-namespace
helm upgrade --install cert-manager jetstack/cert-manager -n cert-manager --create-namespace --set crds.enabled=true
helm upgrade --install cnpg cnpg/cloudnative-pg -n cnpg-system --create-namespace   # PostgreSQL
helm upgrade --install minio-operator minio-operator/operator -n minio-operator --create-namespace
# Redis Cluster（3主3从）、Kafka（KRaft 3 broker）、Keycloak（2 副本 + 外部 PG）

# Vault Raft 3 副本：init → unseal×3 → login → enable kv v2

# 图库/搜索与可观测性
kubectl apply -f deploy/k8s/neo4j-opensearch.yaml
kubectl apply -f deploy/monitoring/k8s/workloads.yaml
kubectl apply -f deploy/k8s/observability-logging.yaml
kubectl apply -f deploy/k8s/observability-tracing.yaml
kubectl apply -f deploy/k8s/observability-thanos.yaml

# 业务服务（先 Secret 后负载）
kubectl -n ops-qa create secret generic cmdb-gateway-env --from-env-file=secrets/gateway.env
kubectl apply -f deploy/k8s/cmdb.yaml
kubectl apply -f deploy/k8s/ops-qa-platform.yaml
kubectl apply -f deploy/runner/k8s/runner.yaml
kubectl -n ops-qa rollout status deployment/gateway --timeout=300s
```

Kafka 需预建九个领域事件 topic（`cmdb.asset.changed.v1`、`alert.state.changed.v1`、
`job.execution.changed.v1` 等），完整清单见部署手册。

验收：`curl -fsS http://<gateway>:30080/api/v1/health` 与 `/api/v1/ready` 返回正常，
浏览器完成 Keycloak 登录、资产/任务/知识库核心流程走通。

完整步骤（含系统初始化、内核参数、防火墙端口表）见仓库
`docs/deployment/平台部署手册-单机与Kubernetes.md`，本节只是主线。
