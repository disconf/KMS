## 生产部署架构

```text
浏览器
  │ HTTPS（TLS 在 Nginx/Caddy 终止）
  ▼
Nginx / Caddy / 内网负载均衡（IP 白名单）
  │ HTTP 8090
  ▼
license-center 容器（单副本）
  ├─ Ed25519 私钥只读挂载
  ├─ /data/licenses.json 持久化卷
  └─ 管理员会话保存在容器内存
```

签发中心**不需要** PostgreSQL、Redis、Kafka 或任何外部中间件，
单二进制 + 单 JSON 文件就是全部状态。也正因为单文件存储，**只跑单副本**——
不要为了高可用起两个实例写同一个卷。

## 资源规划

| 资源 | 最低 | 推荐 |
|---|---:|---:|
| CPU | 0.2 Core | 0.5 Core |
| 内存 | 64 MiB | 128 MiB |
| 数据磁盘 | 100 MiB | 1 GiB |
| 副本数 | 1 | 1 |

## 反向代理与 HTTPS

生产环境绝不裸暴露 8090。Nginx 参考配置要点：

```nginx
server {
    listen 443 ssl http2;
    server_name license.example.com;

    ssl_certificate     /etc/nginx/certs/tls.crt;
    ssl_certificate_key /etc/nginx/certs/tls.key;

    allow 10.0.0.0/8;   # 内网白名单
    deny all;

    location / {
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_pass http://127.0.0.1:8090;
    }
}
```

同时把 `.env` 里 `LICENSE_CENTER_BIND_ADDRESS=127.0.0.1`，
让容器端口只绑定到回环地址，杜绝绕过 Nginx 直连。

## 备份与恢复

```bash
# 备份命名卷
docker run --rm \
  -v cmdb-license-center-data:/data \
  -v "$PWD/backups:/backup" \
  alpine \
  sh -c 'tar czf /backup/license-center-$(date +%Y%m%d-%H%M%S).tar.gz -C /data .'

# 恢复
docker compose stop
docker run --rm \
  -v cmdb-license-center-data:/data \
  -v "$PWD/backups:/backup" \
  alpine \
  sh -c 'rm -rf /data/* && tar xzf /backup/<backup-file>.tar.gz -C /data && chown -R 65532:65532 /data'
docker compose start
```

恢复数据不会改变密钥，已签出的许可证全部继续有效——签发记录只是账本。

## 升级与回滚

```bash
# 升级（先备份命名卷）
git pull
docker compose up -d --build
docker compose ps
curl -fsS http://127.0.0.1:8090/healthz

# 回滚
git checkout <previous-commit>
docker compose up -d --build
```

密钥和数据卷在升级/回滚中都不需要变更。

## 上线验收清单

- [ ] `curl -fsS http://127.0.0.1:8090/healthz` 返回 `{"status":"ok"}`；
- [ ] `LICENSE_CENTER_ADMIN_PASSWORD` 已换成 ≥24 位随机值；
- [ ] `LICENSE_CENTER_BIND_ADDRESS=127.0.0.1`，Nginx HTTPS + IP 白名单生效；
- [ ] `go run ./cmd/key-check` 输出 `private and public keys match`；
- [ ] 备份命令实跑一次并确认 tar 包可解；
- [ ] 团队内已明确：`down -v` 和 `license-keygen` 是禁手。
