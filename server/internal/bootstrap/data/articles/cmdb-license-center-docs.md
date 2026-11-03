## 仓库地图

```text
CMDB-/
├── cmd/
│   ├── key-check/          # 验证现有公私钥是否匹配
│   ├── license-center/     # 授权签发网站主程序
│   └── license-keygen/     # 仅用于全新产品线，当前勿运行
├── internal/
│   ├── license/            # 签名协议和密钥解析（token.go + 测试）
│   └── licensecenter/      # 登录、签发、记录存储和静态页面
├── keys/
│   ├── license-private.pem # 固定签发私钥，仅限私有仓库，权限 0600
│   └── license-public.pem  # 固定产品公钥（内置进 CMDB 平台）
├── compose.yaml            # 单机部署：只读根文件系统 + cap_drop ALL + 命名卷
├── Dockerfile
├── Makefile                # test / build / up / down / logs / ps / key-check
└── README.md               # 全部运维知识都在这一个文件里
```

## 必读顺序

1. **README §2 固定密钥对**——先搞懂「为什么私钥只在这里、仓库为什么必须私有」，
   这是整个产品线的安全底线；
2. **README §5 Docker Compose 部署**——从 clone 到 healthz 通过的全流程；
3. **README §7 使用说明**——签发工作台的日常操作（登录、取 Deployment ID、签发、客户激活）；
4. **README §12 安全约束**——十条禁令，团队里每个碰过这台服务器的人都该读一遍。

站内对应教程：本教程集的六章正好按「介绍 → 部署 → 配置 → 生产 → 命令 → 文档」展开，
初次上手按顺序读完即可独立值班。

## 学习路径

- **第 1 天**：本地或测试机 Compose 部署跑通 healthz，签发一张 1 天期的测试许可证，
  拿到 CMDB 平台「授权与用量」里激活走通全闭环；
- **第 1 周**：演练一次备份恢复（tar 出卷、清卷、还原、确认签发记录还在），
  按生产篇清单加固 Nginx HTTPS + IP 白名单 + `BIND_ADDRESS=127.0.0.1`。

## API 一览

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/healthz` | 健康检查，无需会话 |
| `POST` | `/api/login` | 管理员登录 |
| `POST` | `/api/logout` | 退出登录 |
| `GET` | `/api/session` | 当前会话 |
| `GET` | `/api/stats` | 授权统计 |
| `GET` | `/api/licenses` | 签发记录 |
| `POST` | `/api/licenses` | 签发许可证 |
| `GET` | `/api/licenses/{id}/download` | 下载许可证 |

除健康检查和登录外，所有 API 都要求管理员会话 Cookie。
日常签发走页面即可，API 主要给自动化留口子。

## 安全约束速记

- 私钥只存在私有仓库 + 受控服务器；仓库转 Public 必须立即轮换密钥；
- 管理员密码不能用示例值；授权中心限制在内网或 VPN，生产必须 HTTPS；
- 定期备份 `/data/licenses.json`；
- 禁手三件：`docker compose down -v`、多实例写同一卷、`license-keygen` 换现有产品密钥。

## 相关平台

签出的许可证在 **CMDB 智能运维平台**（本站工具库另有一条收录）中激活：
客户管理员进入「系统管理 → 授权与用量」，粘贴许可证验证激活。
商业版解锁 Enterprise/Professional 功能包与超过免费版 200 的资产额度。
