## 集群规划

已验证的生产形态（最小生产参考取下行）：

| 角色 | 验证环境 | 最小生产参考 |
|---|---|---|
| Control Plane | 3 × 8C/16GB/200GB | 3 × 8C/16GB/200GB SSD |
| Worker | 5 × 32C/64GB/1TB+ | 3 × 16C/32GB/500GB NVMe + 2TB 数据盘 |
| 组件 | K8s v1.28.15 + containerd + Calico + Longhorn | 同左 |

**控制平面必须打 taint**，业务和有状态中间件不许调度上去；VIP 用 Keepalived/HAProxy
或云负载均衡指向三个控制平面（`controlPlaneEndpoint: VIP:6443`）。

## 系统与网络前置

所有节点：时间同步（误差 <1s）、关 swap、加载 `overlay`/`br_netfilter`、
`vm.max_map_count=262144`（OpenSearch 需要）。防火墙按角色放通：

- Control Plane：6443、2379-2380、10250-10252；
- Worker：10250、30000-32767（NodePort）；
- Longhorn：3260、9500-9504；Calico 按 IPIP/VXLAN/BGP 放通。

kubeadm 初始化要点：

```bash
sudo kubeadm init --config kubeadm-config.yaml --upload-certs
# kubeadm-config.yaml: controlPlaneEndpoint + podSubnet/serviceSubnet
kubectl apply -f calico.yaml   # v3.28.2
```

## 基础平台组件（按顺序装）

| 组件 | 部署方式 | 生产要点 |
|---|---|---|
| Longhorn | Helm，`defaultReplicaCount=3` | 所有节点 `iscsid` 启动，StorageClass `longhorn` 可用 |
| cert-manager | Helm，`crds.enabled=true` | 给 Ingress/TLS 用 |
| PostgreSQL | CloudNativePG Operator | 3 实例 + 每日基础备份到对象存储 + WAL 归档 |
| MinIO | MinIO Operator Tenant | ≥4 盘、100GiB/盘起步、独立 StorageClass、TLS+密钥走 Secret |
| Redis | Cluster 3 主 3 从 | 密码入 Secret、开持久化（单机 Compose 才用单实例） |
| Kafka | KRaft 3 broker + 3 controller | 每 broker ≥200GiB；建齐 9 个平台 topic（见下） |
| Keycloak | 2 副本 + 外部 PG | Realm `cmdb`、Client `cmdb-web`（PKCE）、平台权限角色、复合角色 `platform-admin`、组声明 `project_groups` |
| Vault | Raft 3 副本（Helm `server.ha.*`） | `vault operator init` 后三把 key 解封；**解封密钥与 Root Token 离线保存，不进 Git** |

Kafka 平台 topic 清单：`cmdb.asset.changed.v1`、`cmdb.relation.changed.v1`、
`discovery.resource.found.v1`、`monitor.target.changed.v1`、`alert.state.changed.v1`、
`job.execution.changed.v1`、`ticket.state.changed.v1`、`release.state.changed.v1`、
`audit.operation.created.v1`。

## 部署业务服务

```bash
kubectl apply -f deploy/k8s/neo4j-opensearch.yaml        # 拓扑图 + 全文检索
kubectl apply -f deploy/monitoring/k8s/workloads.yaml    # Prometheus 全家桶
kubectl apply -f deploy/k8s/observability-*.yaml         # 日志/链路/Thanos
kubectl apply -f deploy/k8s/cmdb.yaml
kubectl apply -f deploy/k8s/ops-qa-platform.yaml
kubectl apply -f deploy/runner/k8s/runner.yaml
kubectl -n ops-qa rollout status deployment/gateway --timeout=300s
```

**平台 Secret 全部从 Vault 或 CI/CD 渲染，不写进 YAML**：
`cmdb-gateway-env`（`--from-env-file=secrets/gateway.env`）至少要含
`DATABASE_URL`、`REDIS_URL`、`KAFKA_BROKERS`、`KEYCLOAK_ISSUER_URL`、
`SESSION_SECRET`、`AGENT_SHARED_TOKEN`、`NEO4J_*`、`OPENSEARCH_URL`、
`S3_*`、`VAULT_ADDR/VAULT_TOKEN` 等 20+ 变量，另有 `cmdb-registry` 镜像拉取密钥。

## 验收

```bash
kubectl -n ops-qa get pods,svc,pvc
curl -fsS http://<gateway>:30080/api/v1/health
curl -fsS http://<gateway>:30080/api/v1/ready
```

浏览器走一遍：Keycloak 登录、角色/项目组声明正确、资产/告警/任务/知识库页面加载、
附件上传成功、RAG 只引用有权知识、Grafana 有日志与指标。其余对照部署验收清单
（时间同步、证书 >30 天、数据目录本地文件系统、各中间件健康、备份已异地、完成一次恢复演练）。

## 升级与回滚

```bash
kubectl -n ops-qa set image deployment/gateway gateway=<registry>/gateway-bff:<new>
kubectl -n ops-qa rollout status deployment/gateway --timeout=300s
kubectl -n ops-qa rollout undo deployment/gateway      # 回滚
```

数据库升级前四件事：触发 PG 基础备份、确认 WAL 归档正常、测试环境验证迁移、
**记录当前 ReplicaSet 镜像与应用版本**。生产回滚目标必须是具体版本标签，禁用 `latest`。
