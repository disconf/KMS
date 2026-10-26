# KMS · 个人工作经验分享

个人运维与开发经验分享站点：Linux 实战教程、运维题库（10 方向 10000+ 题）、
开源工具推荐与教程集、运维资讯、资源下载，以及面向运维专家的接单与问答板块。

- 技术栈：Go (Gin + GORM) + 原生 ESM 前端（无构建链），MySQL 8 / Redis，前端静态资源 `go:embed` 进二进制。
- 站点：https://www.jinzhiqiang.com （京ICP备2026051452号）
- 详细设计文档见 [server/README.md](server/README.md)。

## 目录结构

```
server/            Go 后端 + 内嵌前端（cmd/api 入口，internal/ 分层）
  deploy/          Dockerfile、开发用 compose、systemd 单机部署（SQLite 方案）
  scripts/         冒烟、渲染自检、题库生成器
  data/articles/   教程集站内正文（go:embed）
deploy/prod/       生产部署（Docker Compose：MySQL+Redis+API+Nginx，见其 README）
```

## 本地开发

```bash
cd server
go run ./cmd/api            # SQLite + 内存缓存，管理员 13800000000 / Admin@12345
# 验证码在发送接口响应的 devCode 字段里（仅 dev/test）
```

## 生产部署

见 [deploy/prod/README.md](deploy/prod/README.md)：一条 `bootstrap.sh` 完成
密钥生成、镜像构建、建库、种子初始化与 HTTPS 反代（80 → 443）。

## 登录说明

- 普通用户：手机验证码登录（短信通道未接通前新用户无法注册）。
- 管理员：`/auth/login/password` 手机号 + 密码（bcrypt，15 分钟内错 5 次锁定），
  前端在登录弹窗切"管理员密码登录"。

## License

[MIT](LICENSE)
