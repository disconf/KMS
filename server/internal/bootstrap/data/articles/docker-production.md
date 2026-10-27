## daemon.json：生产环境的唯一配置入口

所有全局配置都收在 `/etc/docker/daemon.json`，改完 `systemctl reload docker`（优雅重载）
或 `systemctl restart docker`。一份能上生产的基线：

```json
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "50m", "max-file": "3" },
  "registry-mirrors": ["https://docker.m.daocloud.io"],
  "data-root": "/data/docker",
  "live-restore": true,
  "default-ulimits": { "nofile": { "Name": "nofile", "Soft": 65535, "Hard": 65535 } },
  "insecure-registries": ["harbor.internal:5000"]
}
```

- `log-opts` 只对**新创建**的容器生效，存量容器要重建；
- `live-restore: true` 让 dockerd 重启时容器不中断，升级 daemon 靠它；
- `data-root` 改完要 `systemctl stop docker` 后**手动迁移** `/var/lib/docker` 的数据再启动。

## 日志限制：不做迟早写满磁盘

默认 json-file **不限大小**。高 QPS 服务一天能写几十 GB。除了 daemon.json 全局限制，
compose 里也能按服务覆盖：

```yaml
services:
  app:
    logging:
      driver: json-file
      options: { max-size: "20m", max-file: "5" }
```

已经写满的机器按顺序回收：`docker system prune -af --volumes` 前先确认没有要保留的卷。

## 数据目录迁移实战

```bash
systemctl stop docker
rsync -aP /var/lib/docker/ /data/docker/
# 改 daemon.json 的 data-root 后再启动，确认容器都在再删旧目录
systemctl start docker && docker ps
```

镜像加速只解决拉取慢，**私有仓库（Harbor）才是生产标配**：镜像不进公网、有扫描、有保留策略。

## 安全基线

1. 容器进程别用 root 跑：`user:` 字段或 Dockerfile 里 `USER`；
2. 只暴露必要端口，`-p 127.0.0.1:8080:80` 比 `-p 8080:80` 安全；
3. 挂载 docker.sock 等于给容器 root，除非是 CI 专用节点，否则禁止；
4. 镜像用不可变 tag（版本号 / commit sha），`:latest` 在回滚时是个定时炸弹；
5. 开启 `live-restore` 之前先确认应用有重连机制，否则重启 dockerd 后容器是活的但网络是断的。

## 验收清单

- [ ] `docker info` 显示 log-driver 带限制、data-root 指向数据盘
- [ ] `df -h` 数据盘余量 > 30%
- [ ] `docker run --rm hello-world` 通过，私有仓库能拉能推
- [ ] 重启 dockerd 后容器自动回来（`--restart unless-stopped`）
- [ ] 容器内时间、时区、DNS 与宿主机预期一致
