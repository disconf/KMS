## 内存与淘汰策略：最先想清楚的两件事

```conf
maxmemory 4gb                       # 上限留足给 fork 与复制缓冲，别等于物理内存
maxmemory-policy allkeys-lru        # 缓存场景；会话存储用 volatile-lru
```

缓存的值要**可重建**才能放心让 Redis 淘汰。存会话这类「丢了就登录态失效」的
数据时用 `volatile-lru` 并保证 key 都设了 TTL。

## 持久化取舍

| 方式 | 特点 | 生产建议 |
|---|---|---|
| RDB | 定时快照，恢复快，可能丢最后几分钟 | 单独用只接受丢数据 |
| AOF（everysec） | 最多丢 1 秒，文件大恢复慢 | **生产默认** |
| 两者都开 | 重启时优先用 AOF | 数据敏感场景 |

```conf
appendonly yes
appendfsync everysec
auto-aof-rewrite-percentage 100
auto-aof-rewrite-min-size 64mb
```

AOF rewrite 会 fork，`vm.overcommit_memory=1` 与内存余量就是为这一刻准备的。

## 主从 + 哨兵最小生产形态

```conf
# 从库配置
replicaof 10.0.0.11 6379
masterauth 'SamePassAsMaster'        # 主库有密码时必配，否则复制断
replica-read-only yes
```

哨兵三节点（可与业务机混部，但不要和主从同机）：

```conf
sentinel monitor mymaster 10.0.0.11 6379 2    # 2 = 3 票里 2 票判定下线
sentinel down-after-milliseconds mymaster 5000
sentinel failover-timeout mymaster 60000
sentinel auth-pass mymaster SamePassAsMaster
```

客户端要用哨兵协议拿主库地址，**写死主库 IP 的客户端在故障转移时会写挂**。

## 慢查询与热 key 治理

```conf
slowlog-log-slower-than 10000       # 10ms 进慢日志
slowlog-max-len 256
```

大 key 扫描：`redis-cli --bigkeys`（低峰跑）；热 key：`redis-cli --hotkeys`（LFU 策略下有效）。
大 key 的解法是拆（hash 分桶、list 分段），不是加大内存。

## 验收清单

- [ ] `info memory` 的 used_memory_human < maxmemory 的 80%
- [ ] `info persistence` 无最近 fork 卡顿（latest_fork_usec < 20ms）
- [ ] 主从 `master_link_status: up`，哨兵脑裂参数 `min-replicas-to-write` 评估过
- [ ] 慢日志与 bigkeys 巡检在 cron 里
- [ ] 故障转移演练做过一次：杀主库，观察哨兵切换与客户端恢复时间