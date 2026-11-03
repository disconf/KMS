## 环境要求

- Linux 服务器；
- Docker Engine 24 或更高，Docker Compose v2（`docker compose version` 确认）；
- Go 1.26 仅源码测试与本地构建需要，容器部署用不到；
- 可通过内网访问的域名或 IP。

## 部署步骤（Docker Compose，唯一推荐方式）

```bash
# 1. 获取代码（私有仓库，需要仓库访问权限）
git clone https://github.com/disconf/CMDB-.git
cd CMDB-

# 2. 收紧密钥权限
chmod 700 keys
chmod 600 keys/license-private.pem

# 3. 验证固定密钥对匹配
go run ./cmd/key-check \
  -private keys/license-private.pem \
  -public keys/license-public.pem
# 预期输出：private and public keys match

# 4. 配置管理员
cp .env.example .env
vi .env   # 见下节参数说明

# 5. 启动
docker compose up -d --build

# 6. 验证
docker compose ps
curl -fsS http://127.0.0.1:8090/healthz
# 预期：{"status":"ok"}
```

浏览器访问 `http://服务器IP:8090`，用 `.env` 里的管理员账号登录即可进入签发工作台。

> 不要运行 `cmd/license-keygen` 生成新密钥——它只用于创建全新产品线，
> 在现有产品上跑一遍，所有已部署的 CMDB 平台都会验签失败。

## Compose 里的安全加固（已经替你做好）

| 配置 | 作用 |
|---|---|
| `read_only: true` | 容器根文件系统只读，写路径只剩 tmpfs 和数据卷 |
| `cap_drop: [ALL]` | 丢弃全部 Linux capabilities |
| `no-new-privileges: true` | 禁止容器内提权 |
| 非 root 运行 | 数据卷属主 `65532:65532` |
| 私钥只读挂载 | `keys/license-private.pem` → `/run/secrets/license-signing-key:ro`，不进镜像 |
| 命名卷持久化 | `cmdb-license-center-data` → `/data/licenses.json` |

国内构建可传 `GOPROXY`：`GOPROXY=https://goproxy.cn,direct docker compose build`。

## 停止、重启与红线

```bash
docker compose restart   # 重启，签发记录不丢
docker compose down      # 停止并删容器，命名卷保留
docker compose logs -f --tail=200
```

**两条红线**：

- **永远不要 `docker compose down -v`**——`-v` 会连命名卷一起删掉，签发记录全部丢失；
- **不要同时启动多个写同一数据卷的实例**——单个 JSON 文件不支持并发写。
