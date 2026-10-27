## 配置入口有两层

单机 Compose 走 `.env` + 脚本生成的 `secrets/gateway.env`；
K8s 走 Secret（`cmdb-gateway-env`）+ 清单。**密码不要写进 YAML 提交仓库**，
推荐从 Vault 或 CI/CD Secret 渲染。

## .env 核心参数（单机 Compose）

```dotenv
SERVER_HOST=192.168.10.20          # 服务器内网 IP，服务发现与回调都靠它
KEYCLOAK_PUBLIC_URL=http://192.168.10.20:8081   # 浏览器访问 Keycloak 的地址
CMDB_PUBLIC_URL=http://192.168.10.20:8088       # 浏览器访问平台的地址
CMDB_DATA_DIR=/opt/cmdb            # 数据目录，必须是本地盘
CMDB_VERSION=1.0.232               # 升级/回滚改这里
CMDB_ENABLE_DEMO_DATA=false        # 生产必须 false，不生成演示资产/告警/工单
```

接入外部 LLM（AIOps 问答用）时再填 `AI_LLM_BASE_URL / AI_LLM_MODEL / AI_LLM_API_KEY`
与 `AI_EMBEDDING_*`（向量维度 `AI_EMBEDDING_DIMENSIONS` 要与模型一致）。

## Gateway Secret 清单（K8s）

`secrets/gateway.env` 至少包含以下键，`kubectl create secret generic cmdb-gateway-env --from-env-file` 注入：

```text
DATABASE_URL / REDIS_URL / KAFKA_BROKERS
KEYCLOAK_ISSUER_URL / KEYCLOAK_ADMIN_URL / KEYCLOAK_ADMIN_USERNAME / KEYCLOAK_ADMIN_PASSWORD
SESSION_SECRET / AGENT_SHARED_TOKEN
ANSIBLE_RUNNER_URL / ANSIBLE_RUNNER_TOKEN
PROMETHEUS_URL / ALERTMANAGER_URL / LOKI_URL / TEMPO_URL / THANOS_URL
NEO4J_URL / NEO4J_USERNAME / NEO4J_PASSWORD / OPENSEARCH_URL
S3_ENDPOINT_URL / S3_BUCKET / S3_ACCESS_KEY_ID / S3_SECRET_ACCESS_KEY
VAULT_ADDR / VAULT_TOKEN
```

其中 `AGENT_SHARED_TOKEN` 是 Agent 上报的共享令牌，Prometheus 的 HTTP 服务发现
（`agent-token`）也依赖它——监控目标为空先查这个。

## Keycloak 侧配置

必须创建的东西（`bootstrap.sh` 在单机模式下会自动做，K8s 手工建）：

- Realm：`cmdb`；
- Client：`cmdb-web`，启用 PKCE + 标准授权码流程；
- Realm 角色：平台权限名（`cmdb:view`、`monitor:manage`、`aiops:execute` 等），
  复合角色 `platform-admin` 打包全部权限；
- 组映射声明：`project_groups`（前端按项目组切数据范围的依据）。

## 端口与暴露范围

| 端口 | 组件 | 暴露范围 |
|---:|---|---|
| 8088 | Web Portal | 业务网或反向代理 |
| 8080 | Gateway | 仅反代/内网 |
| 8081 | Keycloak | 业务网或统一入口 |
| 5432 / 6379 / 9092 / 8200 / 9200 | PG / Redis / Kafka / Vault / OpenSearch | 仅平台内网，**绝不暴露公网** |
| 9090 / 9093 / 3000 | Prometheus / Alertmanager / Grafana | 仅运维网 |
| 8090 | Ansible Runner | 仅 Gateway 内网 |

生产建议统一域名 + HTTPS 反代，只暴露 443。

## 五条常见误配

1. **`KEYCLOAK_ISSUER_URL` 与浏览器登录地址不一致** → Gateway 全部 401。
   两个地址必须逐字符相同（含 http/https 与端口）。
2. **PostgreSQL 数据目录放 NFS/CIFS/FUSE** → 启动失败或数据损坏，
   只能本地 ext4/XFS，目录属主 UID 999。
3. **生产忘了关 `CMDB_ENABLE_DEMO_DATA`** → 库里混进演示资产、告警、工单，治理成本很高。
4. **`.env.example` 的 `CHANGE_ME_*` 没换** → `bootstrap.sh` 会自动生成随机密码，
   但手工改配置时容易漏掉其中一两个。
5. **Vault 重启后没解封** → 表现为「单机重启后业务不可用」，
   先 `unseal-vault.sh` 再查容器状态，别急着重启服务。
