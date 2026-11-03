## .env 配置参数

| 变量 | 默认 | 说明 |
|---|---|---|
| `LICENSE_CENTER_ADMIN_USERNAME` | 无（必填） | 管理员用户名，缺失时 Compose 直接拒绝启动 |
| `LICENSE_CENTER_ADMIN_PASSWORD` | 无（必填） | 管理员密码，**必须换成至少 24 位随机值**，不能用示例值 |
| `LICENSE_CENTER_ISSUER` | `CMDB Licensing Authority` | 签发机构名，写进每张许可证 |
| `LICENSE_CENTER_PORT` | `8090` | 对外端口 |
| `LICENSE_CENTER_BIND_ADDRESS` | `0.0.0.0` | 监听地址；生产前置 Nginx 时**必须改 `127.0.0.1`** |
| `LICENSE_CENTER_SESSION_HOURS` | `12` | 管理员会话时长（小时） |

容器内另有三个由 compose.yaml 注入的环境变量，一般不动：
`HTTP_ADDR=":8090"`、`LICENSE_CENTER_DATA_FILE=/data/licenses.json`、
`LICENSE_SIGNING_KEY_FILE=/run/secrets/license-signing-key`。

会话保存在容器内存：重启后需要重新登录，但签发记录在命名卷里不受影响。

## 密钥纪律（比配置项更重要）

私钥 `keys/license-private.pem` 的管理要求：

- 只保存在授权签发服务器，文件权限 `0600`；
- 仓库必须始终保持 Private——**仓库可见性或访问权限一旦变化，立即轮换密钥并重新签发所有许可证**；
- 私钥不进 Docker 镜像（compose 只做只读挂载）；
- 不通过聊天、邮件或公共网盘传播；
- 至少保留两份加密离线备份；
- 不挂载到正式 CMDB 网关或门户。

仓库自带两个命令行工具配合密钥使用：

```bash
# 验证现有公私钥是否匹配（日常可用，Makefile 里 make key-check）
go run ./cmd/key-check -private keys/license-private.pem -public keys/license-public.pem

# 仅用于创建全新产品线——当前产品严禁运行
go run ./cmd/license-keygen
```

## 典型误配清单

| 误配 | 后果 |
|---|---|
| 管理员密码用 `.env.example` 示例值 | 等于把签发中心敞开给全网 |
| `BIND_ADDRESS` 保持 `0.0.0.0` 又不上 HTTPS | 8090 裸奔，许可证可被任意人签发 |
| 运行 `license-keygen` | 现有产品线密钥对被换，所有已部署平台验签失败 |
| `docker compose down -v` | 命名卷被删，签发记录全丢 |
| 起两个实例写同一个数据卷 | `licenses.json` 并发写坏 |
| 仓库转 Public 后没轮换密钥 | 私钥泄露，等于把印钞机交出去 |

改完 `.env` 后用 `docker compose up -d --build`（或先 `docker compose config` 校验语法）重新生效。
