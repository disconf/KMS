## 文档入口

| 找什么 | 去哪 |
|---|---|
| 总入口（Engine/CLI/Compose） | <https://docs.docker.com/> |
| CLI 命令参考 | <https://docs.docker.com/reference/cli/docker/> |
| daemon.json 字段全表 | <https://docs.docker.com/reference/cli/dockerd/#daemon-configuration-file> |
| Dockerfile 最佳实践 | <https://docs.docker.com/build/building/best-practices/> |

## 必读页面清单

1. **Get Started 系列**——镜像/容器/仓库三个概念走通一遍，半小时的事；
2. **Container networking**——bridge/host/overlay 三种模式的行为差异，排网络问题的根基；
3. **Resource constraints**（`docker run` 的 `--memory/--cpus` 章节）——裸机容器的资源治理；
4. **Prune 命令族**（`docker system prune` 等）——磁盘回收的标准动作与危险边界；
5. **Dockerfile best-practices**——层缓存、多阶段构建、最小镜像。

## 推荐学习路径

1. **第 1 天**：run/busybox → 写个 Dockerfile 构建自己的镜像 → 推上私有仓库；
   理解 `docker build` 每层缓存在什么时候失效；
2. **第 1 周**：配 daemon.json（日志上限 + 加速 + data-root）并演练一次 data-root 迁移；
   用 `docker system df` + `prune` 完成一次真实磁盘回收；
3. **第 1 个月**：用 compose 编排一组服务（app+db+redis）跑通网络互访与健康检查；
   然后评估该不该上 K8s——单机多服务 compose 足够时别过早引入编排复杂度。

## 版本选择与升级文档

- Engine 版本看 <https://docs.docker.com/engine/release-notes/>；
- 升级顺序：yum 升包 → `systemctl restart docker`（live-restore 开着则容器不中断）→
  `docker version` 确认 client/server 一致；
- 版本兼容：K8s 对 containerd/runtime 有版本要求，混用前查 [兼容矩阵](https://kubernetes.io/zh-cn/docs/setup/production-environment/container-runtimes/)。

## 社区与求助渠道

- GitHub issues：<https://github.com/moby/moby/issues>（先搜，高频问题都有讨论）；
- 提问带上：`docker version`、`docker info`、完整 run 命令与 `journalctl -u docker` 片段；
- 排网络问题先说清模式（bridge/host）与 DNS 需求，不然每句回答都得先反问。
