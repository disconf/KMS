## 连接与巡检

```bash
redis-cli -h 10.0.0.11 -a 'Pass' --no-auth-warning   # 交互式
redis-cli ping                                        # 存活
redis-cli info memory | grep -E 'used_memory_human|maxmemory_human|mem_fragmentation_ratio'
redis-cli info stats | grep instantaneous            # 当前 QPS
redis-cli info replication | grep -E 'role|master_link_status|slave0'
redis-cli slowlog get 10                              # 最近 10 条慢查询
redis-cli client list | awk '{print $2}' | sort | uniq -c | sort -rn | head   # 连接来源
```

`mem_fragmentation_ratio > 1.5` 说明碎片严重，4.0 后可 `activedefrag yes`。

## Key 操作

```bash
redis-cli dbsize
redis-cli --scan --pattern 'session:*' | head       # 千万别用 KEYS *
redis-cli ttl user:1001                              # 剩余过期时间
redis-cli type user:1001                             # 数据类型
redis-cli del user:1001                              # 大 key 删除会阻塞，用 UNLINK
redis-cli unlink user:1001                           # 异步删
redis-cli --bigkeys                                  # 大 key 扫描（低峰跑）
```

## 批量与脚本

```bash
redis-cli -x set bigvalue < file.bin                 # 从 stdin 读值
cat cmds.txt | redis-cli                             # 逐行执行
redis-cli --eval del_by_prefix.lua 'session:*' , 0   # 执行 Lua
```

## 主从与哨兵

```bash
redis-cli replicaof 10.0.0.11 6379        # 临时变从库（配置文件才是永久的）
redis-cli replicaof no one                # 升为主
redis-cli -p 26379 sentinel get-master-addr-by-name mymaster   # 当前主库是谁
redis-cli -p 26379 sentinel master mymaster | head -20
redis-cli -p 26379 sentinel failover mymaster    # 手动触发切换
```

## 备份

```bash
redis-cli bgsave                          # 后台 RDB
redis-cli lastsave                        # 上次成功落盘时间戳
# AOF 场景：直接拷 appendonly.aof + 触发 bgrewriteaof 收缩
redis-cli bgrewriteaof
```

## 「缓存突然变慢」排查顺序

1. `slowlog get`：是哪些命令慢（keys/hgetall 大 key 是常客）；
2. `info memory`：是否逼近 maxmemory 触发淘汰抖动；
3. `info stats | grep rejected_connections`：连接数打满；
4. `latency history` 与 `latency latest`：fork / expire 事件；
5. 服务器侧 `vmstat`、`iostat -x 1`：SWAP 与磁盘 IO 是隐藏杀手。