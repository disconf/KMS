# 生产部署（Docker Compose）

单机编排：MySQL + Redis + API + Nginx，全部走内部网络，只有 Nginx 暴露 80/443。
配置与密钥在 `.env` / `config.yaml`（首次部署自动生成，均不入库）。

## 首次部署

```bash
# 服务器上（Ubuntu 22.04，需 Docker 与 Docker Compose v2）
git clone https://github.com/disconf/KMS.git /opt/kms
cd /opt/kms/deploy/prod
sudo bash bootstrap.sh
```

脚本依次完成：生成 `.env`（随机密钥）→ 渲染 `config.yaml` → 构建镜像 →
起 MySQL/Redis → 幂等初始化（建表 + 种子内容 + 管理员）→ 启动全部服务 →
停用宿主机 nginx（80/443 交给容器）。

结束时终端会打印管理员初始密码，**首次登录后立即在右上角钥匙图标里改掉**，
然后把 `.env` 里的 `SEED_ADMIN_PASSWORD` 清空。

## 日常更新

```bash
cd /opt/kms/deploy/prod && sudo bash update.sh
```

## 证书续期

证书挂载自宿主机 `/etc/nginx/ssl/`（只读）。更换文件后：

```bash
docker compose restart nginx
```

当前证书有效期至 **2026-11-03**。

## 管理后台

- 后台地址：`https://<域名>/#/admin`，账号 `13800000000`，走"管理员密码登录"。
- 普通用户仍是手机验证码登录；短信通道（`sms.provider`）未接通前新用户无法注册，
  拿到资质后在 `config.yaml` 配置 `sms` 段并 `docker compose restart api`。

## 日常运维

```bash
docker compose ps                    # 各服务状态（api 有 healthcheck）
docker compose logs -f api           # 应用日志
docker compose logs -f mysql         # 慢查询已开（>1s 落日志）
docker compose exec mysql mysqldump -uroot -p$(grep MYSQL_ROOT_PASSWORD .env | cut -d= -f2) kms > backup-$(date +%F).sql
```

## 内存说明

整机 2GB 时已按 `innodb-buffer-pool-size=256M`、Redis `maxmemory=128mb` 收紧。
若加内存，优先调大 MySQL 缓冲池。
