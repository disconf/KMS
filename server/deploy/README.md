# 服务器部署

面向一台 Linux 服务器（已在 CentOS 7 / x86_64 上验证）。交付物是**一个可执行文件**：
静态站点已经用 `go:embed` 打进二进制，不需要 nginx、不需要 Node、不需要单独发前端包。
数据库默认用 SQLite，零外部依赖；有 MySQL 时改一个参数即可。

## 部署物

```
deploy/
  install.sh            安装/升级脚本，在服务器上以 root 执行，可反复跑
  config.template.yaml  配置模板，首次安装时渲染成 /opt/ops-expert/configs/config.yaml
  ops-expert.service    systemd 单元
```

服务器上的布局：

```
/opt/ops-expert/
  bin/ops-expert-api          可执行文件
  configs/config.yaml         运行时配置（640 root:opsexpert，含密钥，不入库）
  data/ops_expert.db          SQLite 库（用 MySQL 时这个文件不生成）
  data/uploads/               上传文件（公开资源 + 私有资源/资质证书）
  logs/                       预留给文件日志，当前日志走 journald
  deploy/                     上面三份部署物的副本，供 install.sh 自取
```

## 首次安装

1. 本地交叉编译（在 `server/` 下，CGO 必须关：SQLite 驱动是纯 Go 实现，关掉才能交叉编译）：

   ```bash
   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
     go build -trimpath -ldflags "-s -w -X main.version=1.0.0" -o dist/ops-expert-api ./cmd/api
   ```

2. 传上去（`scp` 或任何方式），把三份部署物放到 `/opt/ops-expert/deploy/`：

   ```bash
   ssh root@<host> 'mkdir -p /opt/ops-expert/deploy'
   scp deploy/{install.sh,config.template.yaml,ops-expert.service} root@<host>:/opt/ops-expert/deploy/
   scp dist/ops-expert-api root@<host>:/tmp/
   ```

3. 安装：

   ```bash
   ssh root@<host> 'bash /opt/ops-expert/deploy/install.sh --binary /tmp/ops-expert-api \
       --env test --port 8080 --base-url http://<host>:8080'
   ```

脚本会做六件事：建 `opsexpert` 系统账号 → 装二进制 → 渲染配置（**仅在文件不存在时**）→
装 systemd 单元 → 启动 → 探 `/healthz` 和 `/readyz`。任一步失败会打日志并退出非 0。

## 升级

同一条命令，换二进制即可：

```bash
scp dist/ops-expert-api root@<host>:/tmp/
ssh root@<host> 'bash /opt/ops-expert/deploy/install.sh --binary /tmp/ops-expert-api'
```

- **配置文件不会被覆盖**。要改配置就直接改 `/opt/ops-expert/configs/config.yaml` 再 `systemctl restart ops-expert`。
- 二进制先写 `.new` 再 `mv`：直接覆盖正在运行的可执行文件会报 `Text file busy`。
- 数据库迁移随启动自动执行（`auto_migrate: true`）。当前阶段可以，上真实数据前应换成 golang-migrate：
  AutoMigrate 只加列不删列、不改列类型，回滚时它帮不上忙。

## 常用运维命令

```bash
systemctl status ops-expert
journalctl -u ops-expert -f            # 实时日志
journalctl -u ops-expert -n 200         # 最近 200 行
systemctl restart ops-expert
curl -s localhost:8080/healthz          # 进程活着
curl -s localhost:8080/readyz           # 依赖就绪（数据库通）
```

`/healthz` 与 `/readyz` 的区别值得记住：进程还在、但数据库连不上时，`healthz` 返 200 而 `readyz` 失败。
负载均衡/监控应该看 `readyz`，否则会出现"节点健康但每个请求都 500"。

## 备份

SQLite 开了 WAL，**不能只 `cp` 主库文件**：最新事务可能还在 `-wal` 里，拷出来的库会缺数据。

```bash
# 一致性备份（在线，不需要停服）
sqlite3 /opt/ops-expert/data/ops_expert.db ".backup '/backup/ops_expert-$(date +%F).db'"
# 上传文件单独打包
tar czf /backup/uploads-$(date +%F).tar.gz -C /opt/ops-expert/data uploads
```

服务器上没有 `sqlite3` 时，退而求其次：`systemctl stop ops-expert` → 拷 `ops_expert.db*`
（三个文件一起：`.db`、`.db-wal`、`.db-shm`）→ 启回来。

## 上线前必须换掉的四件事

当前这套配置是**可用的测试环境**，不是生产：它故意留了几个"能让人立刻用起来"的口子。
上线前逐条确认：

| 项 | 现状 | 上线前 |
|---|---|---|
| 短信 | `sms.provider: mock` + `expose_code: true`，验证码直接回给前端 | 接阿里云/腾讯云，`expose_code: false`，`app.env: prod`（prod 会硬拦 mock，启动即失败） |
| 支付 | `payment.provider: mock`，点支付立刻变已支付 | 换 `offline`（线下转账 + 人工核账）或接通微信/支付宝 |
| 缓存 | `redis.addr` 为空，进程内缓存 | 填真实 Redis：否则**重启会把所有人踢下线**，短信限流计数也清零 |
| 数据库 | SQLite 单连接 | 换 MySQL，并按业务量配连接池；多实例部署时 SQLite 直接不可用 |

另外：`app.env: prod` 会顺带关掉种子数据（管理员 `13800000000 / Admin@12345` 不再创建），
所以切 prod 之前要先准备好正式管理员账号，否则后台进不去。

## 排查经验

- **`Text file busy`**：直接 `cp`/`install` 覆盖正在运行的可执行文件。先写临时名再 `mv`。
- **systemd 起来就退出**：`journalctl` 里通常是配置校验失败。`config.Validate` 对越界值一律**启动失败**
  而不是退回默认值——比如 `scheduler.interval: 30m` 本意是 30s 时它不会报错也不违反下限，
  只会让超时单在库里多躺半小时，从外部完全看不出来。宁可起不来。
- **`readyz` 不通但进程在**：多为 SQLite 数据目录不可写（`ProtectSystem` + 目录属主）。
  确认 `/opt/ops-expert/data` 属主是 `opsexpert`。
- **页面白屏、资源 404**：确认二进制是重新编译的（静态站点内嵌在里面）。
  `web.dir` 留空表示用内嵌副本；填了路径就改成读磁盘，服务器上通常没有这个目录。
