## 配置文件在哪

| 安装方式 | 主配置 |
|---|---|
| 源码/二进制 | 启动参数指定的 `redis.conf` |
| 发行版包 | `/etc/redis/redis.conf`（Debian）或 `/etc/redis.conf`（CentOS） |
| 容器 | 挂载 conf 后 `redis-server /usr/local/etc/redis/redis.conf` |

`CONFIG GET *` 能在线看全部生效值，`CONFIG SET` 可热改——但热改不落盘，
改 conf 文件才算持久，两者要一起做。

## 参数怎么读

redis.conf 是纯 key-value，每项官方注释都很长——**那份注释本身就是最好的文档**，
升级版本时 diff 一份新模板的注释，能直接看到新增了哪些参数。

## 核心参数分组

**内存与淘汰（最重要的一组）**

```conf
maxmemory 4gb                      # 上限必设，不设等于把内存命运交给 OOM killer
maxmemory-policy allkeys-lru       # 纯缓存用 allkeys-lru；有持久数据用 volatile-lru
```

**持久化**

```conf
appendonly yes                     # 要数据就开 AOF
appendfsync everysec               # 每秒刷盘，性能与安全折中
save 900 1                         # RDB 快照条件，AOF 开了也可保留
```

**网络与安全**

```conf
bind 127.0.0.1 内网IP              # 绝不 0.0.0.0 裸奔
requirepass 强口令                  # 或用 ACL（6.0+）做账号级权限
protected-mode yes
rename-command FLUSHALL ""         # 生产可注释掉危险命令
```

**慢命令治理**

```conf
slowlog-log-slower-than 10000      # 微秒，10ms 记一条
slowlog-max-len 128
```

## 常见误配

1. **不设 maxmemory**，缓存越积越多最后整台机器 OOM；
2. **缓存与持久数据混用一个实例**又配了 `allkeys-lru`，持久数据会被淘汰——
   两种用途分实例或至少分库并改用 volatile-*；
3. **KEYS \* 上线**——O(N) 阻塞全局，用 SCAN 分批；
4. **大 key 不治理**：`redis-cli --bigkeys` 定期跑，几百 KB 的 value 迟早出事；
5. **AOF 开了却把 appendfsync 设 always**，每次写都刷盘，QPS 掉一个量级。

## 校验与生效

```bash
redis-cli ping                     # 连通性
redis-cli CONFIG GET maxmemory     # 看生效值
redis-cli --latency                # 延迟基线
redis-cli SLOWLOG GET 10           # 慢命令
redis-cli INFO memory              # 内存碎片率、淘汰计数
```
