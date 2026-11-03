## 服务日常命令

```bash
cd CMDB-   # 仓库根目录

docker compose ps                        # 容器状态
docker compose logs -f --tail=200        # 跟日志
docker compose restart                   # 重启（签发记录不丢，需重新登录）
curl -fsS http://127.0.0.1:8090/healthz  # 健康检查，预期 {"status":"ok"}

make up       # = docker compose up -d --build
make logs
make ps
make key-check  # 验证固定密钥对，预期 private and public keys match
```

## 密钥相关命令

```bash
# 公私钥匹配校验（换服务器、升级、任何怀疑密钥的场合先跑这个）
go run ./cmd/key-check \
  -private keys/license-private.pem \
  -public keys/license-public.pem

# 私钥权限收紧
chmod 700 keys && chmod 600 keys/license-private.pem

# 本地测试
go test ./... && go vet ./...
docker compose config   # 校验 compose 文件语法
```

## 签发一张许可证（页面操作）

1. 登录授权中心，进入签发工作台；
2. 向客户索取 Deployment ID（客户平台：**系统管理 → 授权与用量 → 授权申请信息**）；
3. 填写字段：

| 字段 | 要求 |
|---|---|
| 客户名称 | 客户公司或项目名 |
| 部署实例 ID | 客户的 Deployment ID，逐字符核对 |
| 产品版本 | Enterprise / Professional |
| 资产额度 | 必须 **大于免费版 200** |
| 授权天数 | 1~3650；0 表示永久 |
| 功能包 | 逗号分隔：`cmdb,monitoring,automation,knowledge` |
| 签发备注 | 合同号、销售负责人、续费说明 |

4. 点击「签发许可证」→ 复制许可证文本或下载 `.license` 文件；
5. 客户在「系统管理 → 授权与用量」粘贴到「导入商业许可证」→「验证并激活」，
   校验签名/结构/Deployment ID/有效期后**即时生效，无需重启平台**。

续期和扩容：直接签新证让客户导入覆盖，旧证记录留在授权中心供审计。

## 备份恢复速查

```bash
# 备份（tar 命名卷）
docker run --rm -v cmdb-license-center-data:/data -v "$PWD/backups:/backup" \
  alpine sh -c 'tar czf /backup/license-center-$(date +%Y%m%d-%H%M%S).tar.gz -C /data .'
```

恢复流程见「生产环境实战部署」一篇，注意恢复后 `chown -R 65532:65532 /data`。

## 排障速查表

| 现象 | 原因 |
|---|---|
| 客户激活报「许可证签名或内容无效」 | 许可证被改动，或复制时截断不完整 |
| 客户激活报「部署实例与当前平台不一致」 | Deployment ID 填错，对不上客户实例 |
| 客户激活报「许可证已过期」 | 导入的是过期证，回授权中心签新证 |
| 页面提示「私钥不存在」 | `keys/license-private.pem` 缺失或被误删 |
| 签发报「许可证无法签名」 | 私钥损坏，或与 `license-public.pem` 不匹配（跑 key-check 确认） |
| 升级后 502 | 容器没起来，`docker compose logs` 看构建/启动错误 |
