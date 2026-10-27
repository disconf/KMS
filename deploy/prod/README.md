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

## 用户注册与权限

- 用户自助注册走**邮箱验证码 + 自设密码**，登录支持"邮箱+密码"与"邮箱+验证码"两种。
- 新注册账号一律**只读**：能浏览全部内容、进个人中心，不能提问/回答/投稿/发悬赏。
- 想发言要在「我的」页提交实名申请（真实姓名 + 联系方式），后台
  `#/admin/speak` 审核通过后**无需重新登录**即可发言；驳回必须写原因，会回显给用户。
- 手机号验证码登录仍保留，但依赖短信通道（`sms.provider`）接通；未接通时
  发码返回 502/90003，只影响手机号登录，不影响邮箱注册。拿到短信资质后配置
  `sms` 段并 `docker compose restart api`。

### 邮件通道（SMTP）

注册与邮箱登录唯一依赖。配置在 `.env`，改完 `docker compose up -d api` 生效：

```bash
MAIL_PROVIDER=smtp
MAIL_SMTP_HOST=smtp.163.com       # QQ: smtp.qq.com / 企业邮: smtp.exmail.qq.com
MAIL_SMTP_PORT=465                # 465 隐式 TLS，587/25 走 STARTTLS
MAIL_SMTP_USERNAME=you@163.com
MAIL_SMTP_PASSWORD=<授权码>        # 不是登录密码
MAIL_SMTP_FROM=you@163.com        # 必须与发件账号一致，否则被服务商拒
MAIL_SMTP_FROM_NAME=个人工作经验分享
MAIL_SMTP_TLS=true
```

未配置时注册接口返回 501/20024（明确报错，不假装已发送），站点其余部分不受影响。
生产环境禁止 `MAIL_PROVIDER=mock` 与 `MAIL_EXPOSE_CODE=true`，启动校验会拒绝。

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
