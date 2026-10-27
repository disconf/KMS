## 启停与重载

```bash
nginx -t                       # 改配置后必跑，语法+路径检查
nginx                          # 启动
nginx -s reload                # 优雅重载：老 worker 处理完存量请求再退
nginx -s quit                  # 优雅停止
nginx -s stop                  # 立即停（FIN 直接发，可能断请求）
systemctl reload nginx         # systemd 管理时优先用这个
```

**改配置没生效的排查顺序**：`nginx -t` 是否通过 → reload 是否真的执行过 →
改的是不是被 include 的那份文件 → `nginx -T`（大写）输出最终合并配置确认。

## 版本与编译参数

```bash
nginx -v                       # 版本
nginx -V                       # 版本 + configure 参数（确认模块有没有编进去）
```

## 日志分析

```bash
# 实时错误
tail -f /var/log/nginx/error.log

# TOP 20 访问 IP
awk '{print $1}' access.log | sort | uniq -c | sort -rn | head -20

# 5xx 分布
awk '$9 ~ /^5/ {print $9, $7}' access.log | sort | uniq -c | sort -rn | head

# 慢请求（log_format 里加了 $request_time 时）
awk '$NF > 1 {print $7, $NF}' access.log | sort -k2 -rn | head

# 日志切割（不丢日志的标准做法）
mv access.log access.log.$(date +%F)
kill -USR1 $(cat /run/nginx.pid)    # 让 nginx 换到新文件继续写
```

## 连接与进程

```bash
ss -s | grep -i 'timewait\|estab'          # 连接概况
ss -ntp state established '( sport = :443 )' | wc -l   # 当前 443 长连接数
ps -eo pid,ppid,cmd | grep nginx           # master/worker 关系
kill -HUP  $(cat /run/nginx.pid)           # 等价 reload
kill -USR1 $(cat /run/nginx.pid)           # 重开日志文件
```

## 502 / 504 从哪查起

1. **502 Bad Gateway**：后端拒绝连接。先 `curl -v http://后端IP:端口`，
   通了再看 SELinux（`getenforce`，`setsebool -P httpd_can_network_connect 1`）；
2. **504 Gateway Timeout**：后端处理慢。调大 `proxy_read_timeout` 之前先想清楚
   是不是该给业务加异步，而不是把超时调成 300s；
3. **间歇性 502**：看 error.log 里的 `connect() failed` 与 `upstream prematurely closed`，
   前者是后端挂了，后者是后端处理中主动断开（常见于 keepalive 没配对）。