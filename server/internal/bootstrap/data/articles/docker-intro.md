## 它是什么

Docker 是**容器运行时的标准化封装**：把应用与依赖打包成镜像，用内核的 namespace/cgroup
做进程隔离与资源限制。一句话：让「在我机器上是好的」变成「在任何机器上都一样」。

## 解决什么问题

部署的历史难题是依赖与环境不一致。镜像把 OS 库、运行时、应用、配置固化成一个不可变制品，
发布就变成「拉镜像、跑容器」；同一台宿主机上多个容器互不干扰，资源上限可用 cgroup 精确到 CPU/内存。

## 架构与核心概念

| 概念 | 一句话 |
|---|---|
| 镜像 / 容器 | 镜像是只读分层模板，容器是它跑起来的实例（加一层可写层） |
| daemon（dockerd） | 常驻服务，CLI 只是它的客户端 |
| Registry | 镜像仓库，Docker Hub / Harbor / 云厂商 ACR |
| volume | 数据卷，绕过容器可写层的持久化 |
| 网络 | bridge（默认 NAT）、host、overlay（跨主机） |

理解「分层镜像 + 联合文件系统」就理解了为什么构建慢、拉取快、层可缓存。

## 版本与生态

- 版本号 20.10 / 23.0 起按年份，跟随 [moby](https://github.com/moby/moby) 主项目；
- 运行时分层：containerd（容器生命周期）+ runc（OCI 运行时），Docker 是它们之上的易用层；
- K8s 1.24 起不再内置 dockershim，但镜像规范（OCI）通用，Docker 构建的镜像任何运行时都能跑。

## 适用与不适用

**适合**：应用交付与运行环境标准化、CI 构建、单机多服务隔离。
**不适合**：把「用了 Docker」当成高可用——单机容器的编排、自愈、扩缩容是 K8s 或
compose/swarm 的事，容器本身只解决「跑起来」。

## 官方文档导读

- 总入口：<https://docs.docker.com/>
- Engine 配置参考（daemon.json）：<https://docs.docker.com/reference/cli/dockerd/#daemon-configuration-file>
- 网络专题：<https://docs.docker.com/engine/network/>
