## 配置文件在哪

| 文件 | 用途 |
|---|---|
| `/etc/docker/daemon.json` | daemon 全局配置（日志、镜像源、数据目录） |
| `/etc/docker/daemon-cert/` 或 service 文件 | TLS 远程访问相关 |
| `~/.docker/config.json` | CLI 客户端配置（登录凭据、代理） |
| systemd drop-in | `systemctl edit docker` 改启动参数（如 `--data-root`） |

daemon.json 改完要 `systemctl restart docker`——**所有容器都会重启**，生产先排窗口。

## 参数怎么读

daemon.json 是 dockerd 的启动配置；每个容器另有运行时配置（`docker run` 参数）。
分清「宿主机全局的」与「每容器的」，大部分困惑源于把后者写进了前者。

## 核心参数分组

**daemon.json 全局（每台宿主机必配）**

```json
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "50m", "max-file": "3" },
  "registry-mirrors": ["https://<你的镜像加速地址>"],
  "data-root": "/data/docker",
  "live-restore": true
}
```

- `log-opts` 不配是**全局无上限日志**，磁盘迟早被吃满，这是最值钱的一行；
- `live-restore: true` 让 dockerd 重启时容器不中断。

**容器运行时（docker run 级）**

```bash
docker run -d --name web \
  -m 2g --cpus=2 \                # 资源上限，裸跑必给
  --restart=unless-stopped \      # 自愈策略
  --memory-swap 2g \              # 等于 -m 则禁 swap
  -v /data/web:/var/www/html \    # 挂卷而非写容器层
  -p 80:8080 \
  nginx:1.27
```

## 常见误配

1. **不限内存的容器 OOM 整机**——`-m` 是裸跑容器的底线；
2. **数据写容器层**——容器一删就没，持久数据必须 `-v` 卷；
3. **改了 daemon.json 忘了重启在跑的容器**——旧容器不继承新 log-opts，要重建；
4. **`--restart` 与 K8s 混用**——托管容器交给编排器，别再设 restart 策略；
5. **同一个宿主机端口映射冲突**排查半天，其实是 `docker ps -a` 里有 stopped 容器占着。

## 校验与生效

```bash
docker info                     # daemon 生效配置与运行时状态
docker inspect <容器> | jq '.[0].HostConfig.LogConfig'   # 单容器日志策略
docker system df -v             # 磁盘占用明细
journalctl -u docker -n 100     # daemon 日志
```
