## 配置文件在哪

| 来源 | 主配置 |
|---|---|
| 发行版包（CentOS） | `/etc/nginx/nginx.conf` |
| 发行版包（Debian/Ubuntu） | `/etc/nginx/nginx.conf`，站点在 `sites-enabled/` |
| 官方源 / 源码编译 | `--prefix` 下 `conf/nginx.conf` |
| 容器 | `/etc/nginx/nginx.conf`，挂载覆盖 |

Debian 系的 `sites-available/ + sites-enabled/ 软链` 是好习惯：下线一个站点删软链即可，
配置本体还在，可回滚。

## 参数怎么读

nginx 指令按 `http → server → location` 层级继承，内层显式配置覆盖外层。
排「配置不生效」先确认三件事：改的是哪个层级、有没有更内层的覆盖、reload 了没有。

## 核心参数分组

**进程与连接**

```nginx
worker_processes auto;
events { worker_connections 10240; }
```

**反向代理标准结构（每台都要有的六行）**

```nginx
proxy_http_version 1.1;
proxy_set_header Host              $host;
proxy_set_header X-Real-IP         $remote_addr;
proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
proxy_set_header Connection        "";
proxy_read_timeout                 60s;
```

**upstream 调度**

```nginx
upstream app {
    least_conn;                       # 或 ip_hash / weight=2
    server 10.0.0.1:8000 max_fails=3 fail_timeout=10s;
    server 10.0.0.2:8000 backup;      # 全挂了才顶上
    keepalive 32;                     # 与 proxy_http_version 1.1 + Connection "" 配套
}
```

**限流**

```nginx
limit_req_zone  $binary_remote_addr  zone=api:10m  rate=10r/s;
limit_req       zone=api burst=20 nodelay;
```

## 常见误配

1. **`proxy_pass` 末尾有没有 `/`**：`proxy_pass http://app/;` 会替换 location 前缀，
   不带斜杠则是原样拼接——路径多一层少一层全看这个字符；
2. **忘配 `proxy_http_version 1.1`** 导致 upstream keepalive 无效，TIME_WAIT 堆积；
3. **`client_max_body_size` 默认 1m**，上传接口直接 413；
4. **gzip 只压了 text/html**，`gzip_types` 不加 application/json 等于白开；
5. **改完不 `nginx -t` 直接 reload**，语法错会让新 worker 起不来而旧配置还在跑，问题被掩盖。

## 校验与生效

```bash
nginx -t                # 语法 + 语义检查
nginx -s reload         # 平滑重载
nginx -T | grep 关键字   # 打印合并后的完整配置，查继承覆盖问题最快
tail -f /var/log/nginx/error.log
```
