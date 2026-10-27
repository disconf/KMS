## 反向代理标准结构

一个能上生产的 server 块长这样，逐行都有理由：

```nginx
upstream app_backend {
    server 10.0.1.11:8080 max_fails=3 fail_timeout=10s;
    server 10.0.1.12:8080 max_fails=3 fail_timeout=10s;
    keepalive 32;                     # 到后端的长连接，高 QPS 必开
}

server {
    listen 443 ssl;
    http2 on;
    server_name api.example.com;

    ssl_certificate     /etc/nginx/certs/api.example.com.pem;
    ssl_certificate_key /etc/nginx/certs/api.example.com.key;
    ssl_protocols TLSv1.2 TLSv1.3;

    client_max_body_size 20m;         # 默认 1m，传文件必改
    access_log /var/log/nginx/api.access.log main;

    location / {
        proxy_pass http://app_backend;
        proxy_http_version 1.1;       # keepalive 要求 1.1
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_connect_timeout 3s;
        proxy_read_timeout 30s;
    }
}
```

## 限流与限连

```nginx
limit_req_zone  $binary_remote_addr zone=api:20m rate=20r/s;
limit_conn_zone $binary_remote_addr zone=conn:20m;

location /api/ {
    limit_req  zone=api  burst=40 nodelay;
    limit_conn conn 20;
}
```

`burst` 是排队容量，`nodelay` 让突发直接放行而不是排队等待——防刷开 nodelay，
保护后端则去掉它。

## HTTPS 证书轮转不中断

```bash
# acme.sh 自动续期（以阿里云 DNS API 为例）
acme.sh --issue --dns dns_ali -d api.example.com
acme.sh --install-cert -d api.example.com \
  --key-file /etc/nginx/certs/api.example.com.key \
  --fullchain-file /etc/nginx/certs/api.example.com.pem \
  --reloadcmd "nginx -s reload"
```

证书更新和 reload 绑在同一条命令里，才不会出现「证书续了但 nginx 没加载」。

## 健康检查与灰度

开源版 nginx 的 `max_fails/fail_timeout` 是**被动**检查，出过错才摘节点。
主动健康检查在 Plus 版或用 `nginx_upstream_check_module` 补丁。灰度用
两套 upstream + weight 过渡：

```nginx
upstream app_backend {
    server 10.0.1.11:8080 weight=9;   # 新版本小流量先 9:1
    server 10.0.1.13:8080 weight=1;
}
```

## 验收清单

- [ ] `nginx -t` 通过且配置纳入版本管理
- [ ] X-Forwarded-For 链路完整，后端取到的不是 nginx 自己的 IP
- [ ] 413（body 过大）、504（后端超时）的阈值与业务确认过
- [ ] 证书到期监控 + 自动续期跑通一轮
- [ ] keepalive 生效：压测时后端 TIME_WAIT 不随 QPS 线性增长