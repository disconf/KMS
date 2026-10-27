## 它是什么

CMDB 智能运维平台是一套**以 CMDB 为唯一资产数据源**的运维自动化平台（当前 V1.5）：
资产台账、自动发现、监控告警、任务执行、工单、CI/CD、远程运维、AIOps、知识库
九大能力围绕同一份运维数据协作，避免「监控一套 IP、任务一套 IP、CMDB 又一套 IP」的脱节。

核心设计原则：CMDB 是唯一资产数据源，但其他模块不直接访问 CMDB 数据库；
模块之间通过 **API + Kafka 事件总线**协作；采集、监控和任务执行下沉到分布式 Agent，
避免中心节点成为瓶颈；AI 只做分析与建议，高风险操作必须审批。

## 架构与核心概念

```text
浏览器 → Web Portal (Vue 3) → Gateway BFF (Go) → 各业务域
                                    │
        PostgreSQL / Redis / Kafka / Keycloak / Vault / MinIO
        Neo4j(拓扑投影) / OpenSearch(全文检索)
        Prometheus / Alertmanager / Thanos / Loki / Tempo / Grafana
                                    │
                          Ansible Runner → Linux / Windows 主机（Agent）
```

| 概念 | 一句话 |
|---|---|
| Gateway BFF | Go 写的聚合网关：统一认证（Keycloak OIDC）、API、权限、业务编排 |
| Web Portal | Vue 3 运维门户，模块化可插拔 |
| CI 模型 | 统一资源模型：位置（机房/机柜/U位）、计算（物理机/VM/云主机）、网络、应用四类对象 + 关系 |
| Agent 与发现 | Linux/Windows Agent（mTLS、签名上报、隔离吊销）+ SSH/SNMP/Redfish/VMware/云平台/K8s 采集 |
| 事件总线 | Kafka 承载资产/告警/任务/工单/发布等领域事件，模块解耦 |
| Ansible Runner | 真实任务执行器：Playbook、审批、定时调度、实时日志 |
| 拓扑投影 | PostgreSQL 是资产主数据，Neo4j 只做关系查询投影——避免两个「真相来源」 |

## 九大能力模块

| 模块 | 能力 |
|---|---|
| CMDB | CI 模型、资产台账、机房机柜 U 位、关系拓扑、变更历史、CSV 导入、责任治理、监控联动 |
| Agent 与发现 | 主机 Agent、采集计划、健康巡检、多协议/多云/多 K8s 采集 |
| 监控与可观测性 | Prometheus + Alertmanager + Thanos 长期存储 + Loki 日志 + Tempo 链路 + Grafana |
| 拓扑与影响分析 | Neo4j/OpenSearch 资产投影、上下游依赖、故障影响路径 |
| 任务与工单 | Ansible Runner 真实执行、审批、SLA、告警转单 |
| CI/CD 与发布 | Argo CD / Harbor / GitLab 集成、发布门禁、回滚、漂移检测 |
| 远程运维 | WebSSH、文件传输、批量执行、高危命令审批、录像回放 |
| AIOps | 知识库、RAG 问答、LLM 网关、Runbook；只调用白名单注册的平台工具且全量审计 |
| 系统管理 | Keycloak 认证、RBAC、数据范围、审计、生产就绪检查 |

## 技术栈

Go（Gateway/Agent）+ Vue 3 + TypeScript（门户）；PostgreSQL 主数据、Redis 缓存、
Kafka 事件、OpenSearch 检索、Neo4j 图关系、MinIO 对象存储、Vault 密钥；
可观测性全套 Prometheus 生态；认证统一走 Keycloak（OIDC，可对接 LDAP/AD）。

## 适用与不适用

**适合**：千台级服务器、多集群、多云环境的资产治理与运维自动化；
想让监控目标、任务执行、发布记录都挂在同一份资产数据上的团队。

**不适合**：只有几台机器的小环境（一套 Compose + 监控就够了）；
期望「开箱即用零配置」的场景——资产模型与标签体系需要按自己的组织结构先设计好，
这个底座不稳，后面的监控、任务、AI 都接不住。

## 项目仓库

- 源码：<https://github.com/disconf/CMDB>
- 部署形态：单机 Docker Compose（POC/演示）与 Kubernetes（生产）两套，见本教程集后续章节。
