## 仓库与文档地图

源码仓库：<https://github.com/disconf/CMDB>（Go + Vue 3，main 分支）

```text
apps/gateway-bff             Go 聚合网关与业务 API
apps/web-portal              Vue 3 运维门户
apps/cmdb-agent              Linux/Windows 主机 Agent
apps/ansible-runner-service  Ansible 任务执行器
deploy/single-node           单机 Docker Compose 一键部署包
deploy/k8s                   Kubernetes 部署清单（含可观测性 YAML）
docs/deployment/             部署手册与环境发布记录
docs/manuals/                平台使用手册（含全部模块截图）
docs/operations/             备份、灾备、生产就绪与演练
docs/design/                 各模块设计文档（资产、Agent、AIOps、权限等）
docs/roadmap/milestones.md   里程碑与进度
scripts/                     构建、压测、运维脚本
```

## 必读文档（按这个顺序读）

1. **`docs/deployment/平台部署手册-单机与Kubernetes.md`**——最重要的一份：
   架构、端口表、Compose 与 K8s 全流程、升级备份、常见故障，本教程集多数内容源于它。
2. **`docs/manuals/平台使用手册.md`**——九大模块的操作说明与截图，给最终用户看。
3. **`docs/deployment/v1.5-ops-qa-deployment.md`**——真实环境的部署形态、镜像版本与发布步骤。
4. **`docs/operations/生产就绪与灾备演练.md`**——RPO/RTO 目标、压测结论、演练记录。
5. **`docs/design/`**——要做二次开发前按模块挑着读（如 `Agent与发现模块设计`、
   `CMDB与Prometheus监控拓扑结合设计`、`系统用户与角色权限设计`）。

## 学习路径

- **第 1 天**：本地 `go run` + `npm run dev` 起起来，用内存账号把九大模块点一遍；
- **第 1 周**：找台 16C/64G 的机器走单机 Compose 部署，重点体会 `.env`、
  `bootstrap.sh` 做了哪些初始化、Vault 解封为什么是重启后的第一件事；
- **第 1 个月**：按 K8s 手册搭一套准生产，把资产模型（CI 类型、项目组、标签体系）
  按自己的组织结构设计好——这是整个平台的地基，然后接 Agent、看监控自动纳管。

## 线上环境参考

| 项 | 地址 |
|---|---|
| 平台门户 | <https://cmdb.jzq.com:31443> |
| API 网关 | <https://cmdb-api.jzq.com:31443> |
| Keycloak | <https://keycloak.jzq.com:31443> |
| 账号 | `admin`（全部模块）、`viewer`(只读)；密码不在文档中传播，向平台管理员索取 |

> 该环境使用自签 CA：其他同事首次访问需安装 CA 证书（向管理员索取 `ca.crt`，
> Windows 双击安装到「受信任的根证书颁发机构」），或等正式证书下发。

## 二次开发与反馈

- 后端：`apps/gateway-bff`，`go test ./... && go vet ./...`；
- 前端：`apps/web-portal`，`npm test && npm run build`；
- 问题与需求走 GitHub Issue：<https://github.com/disconf/CMDB/issues>。
