## 文档入口

| 找什么 | 去哪 |
|---|---|
| 命令字典 | <https://redis.io/commands/>（可按数据结构/用途过滤） |
| 管理与运维专题 | <https://redis.io/docs/latest/operate/> |
| 配置文件注释版模板 | 源码包里的 `redis.conf`，比网页版更权威 |
| 数据类型教程 | <https://redis.io/docs/latest/develop/data-types/> |

## 必读页面清单

1. **Commands 页**——每条命令都有复杂度标注（O(1)/O(N)），用前先看复杂度，
   这是避开慢命令的第一道防线；
2. **Replication / Sentinel / Cluster 三篇管理文档**——高可用方案的唯一权威出处；
3. **Latency 一篇**（`docs/latest/operate/oss_and_mgmt/management/latency/`）——
   fork、swap、大 key 三大延迟来源都在里面；
4. **Persistence 一篇**——RDB 与 AOF 的取舍写得很清楚。

## 推荐学习路径

1. **第 1 天**：起单实例 → 把 String/Hash/ZSet 各玩一遍 → 用 `MONITOR` 看自己敲的命令；
2. **第 1 周**：配一套主从 + 哨兵，手动 kill 主库观察切换全流程；
   开 AOF 做一次「重启恢复」演练，理解 everysec 最多丢一秒意味着什么；
3. **第 1 个月**：压测（`redis-benchmark`）找单实例上限；治理一次真实大 key；
   然后再评估要不要上 Cluster——大多数业务到不了需要分片的量。

## 版本选择与升级文档

- 版本发布页 <https://github.com/redis/redis/releases>，看 **Upgrade notes**；
- 跨大版本升级（6→7）先在测试机过一遍数据迁移；
- 升级主从结构的标准姿势：逐台从库升 → 切主 → 旧主升，期间注意 `INFO replication` 的主从偏移；
- 许可证变化（7.4+）涉及商用合规，选型时把 Valkey 纳入对比。

## 社区与求助渠道

- GitHub Discussions：<https://github.com/redis/redis/discussions>；
- 提问带上 `redis-server --version`、`CONFIG GET *` 关键项、慢命令样本；
- 中文社区内容多偏「面试题」，实操问题优先信官方文档与 issue 区。
