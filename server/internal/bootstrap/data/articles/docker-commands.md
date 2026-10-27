## 镜像操作

```bash
docker images                                    # 本地镜像列表
docker pull nginx:1.27-alpine                    # 拉指定版本，别用 latest
docker build -t registry.internal/app:v1.2.3 .   # 构建并打仓库 tag
docker push registry.internal/app:v1.2.3         # 推送
docker save app:v1.2.3 -o app.tar                # 导出（离线环境搬运）
docker load -i app.tar                           # 导入
docker tag app:v1.2.3 app:latest                 # 补一个 tag
docker rmi app:v1.2.3                            # 删除镜像
docker history app:v1.2.3                        # 看每层做了什么（排查体积）
```

## 容器生命周期

```bash
docker run -d --name web --restart unless-stopped \
  -p 8080:80 -v /data/web:/usr/share/nginx/html \
  -e TZ=Asia/Shanghai nginx:1.27-alpine

docker ps                       # 运行中
docker ps -a                    # 全部（含退出）
docker stop web && docker rm web
docker restart web
docker exec -it web sh          # 进容器（alpine 是 sh 不是 bash）
docker inspect web --format '{{.State.Status}} {{.State.StartedAt}}'
docker update --restart=always web   # 改重启策略不用重建
```

## 日志与排障

```bash
docker logs -f --tail 200 web        # 跟踪最近 200 行
docker logs --since 30m web          # 最近 30 分钟
docker stats                          # 实时 CPU / 内存（注意是瞬时值）
docker top web                        # 容器内进程列表
docker exec web df -h                 # 进容器看磁盘
docker port web                       # 端口映射确认
docker diff web                       # 容器里改了哪些文件
```

容器反复退出时先看这两处，别急着进容器：

```bash
docker inspect web --format '{{.State.ExitCode}} {{.State.OOMKilled}}'
docker logs --previous web 2>/dev/null || docker logs web | tail -50
```

## 清理（磁盘满了按这个顺序来）

```bash
docker container prune               # 删所有已停止容器
docker image prune -a                # 删无容器引用的镜像（-a 连没 tag 的一起删）
docker volume ls -f dangling=true    # 找无主数据卷
docker volume prune                  # 删无主卷——确认没有要留的数据再执行
docker system df                     # 先看磁盘都被谁占了
docker system prune -af              # 一把梭（不带 --volumes，卷不删）
```

## 网络

```bash
docker network ls
docker network create --driver bridge appnet
docker run -d --network appnet --name app myapp:v1
docker network inspect appnet        # 看谁在这个网络里
docker exec web curl -s app:8080/healthz   # 同网络内容器直接用名字互访
```

## 一分钟定位「容器起不来」

1. `docker ps -a` 看退出码：137 多半是 OOM 或被 kill，1 是应用自己崩；
2. `docker logs` 看最后的报错；
3. `docker run --rm -it 镜像 sh` 用同样参数手动起，绕开 entrypoint 逐条试命令。
