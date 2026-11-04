## 一句话结论

MySQL 从库数据盘被 binlog 与 relay log 写满，SQL 线程因无法写 relay log 而停止，主从延迟从 0 涨到 3000 秒；根因是一次大表 DDL 产生了 40GB binlog，而数据盘水位没有任何告警。

## 影响范围

- 受影响业务：订单查询、报表看板（读从库）
- 起止时间：09:12 发现延迟告警，11:40 恢复同步，实际数据延迟约 1 小时
- 影响面：读请求返回了约 40 分钟的旧数据，未产生写入错误（写主库正常）

**为什么"读旧数据"比"报错"更危险**：报错会立刻有人来问，读到旧数据没人发现。当天是客服反馈"订单列表里看不到刚下的单"才暴露的。

## 时间线

| 时间 | 发生了什么 | 谁在处理 |
| --- | --- | --- |
| 08:55 | 数据盘水位从 78% 快速上涨到 96% | 无人察觉（无告警） |
| 09:12 | 从库 `Seconds_Behind_Master` 从 0 跳到 1800，告警触发 | 值班 |
| 09:20 | 确认磁盘 100%，`df -h` 显示 `/data` 满 | 值班 |
| 09:35 | 尝试 `PURGE BINARY LOGS` 失败：磁盘满时该操作同样需要写文件 | 值班 + DBA |
| 10:05 | 扩容数据盘 200G → 400G（云盘在线扩容 + 文件系统扩容） | DBA |
| 10:40 | 磁盘降到 62%，清掉 7 天前的 binlog，SQL 线程恢复 | DBA |
| 11:40 | 延迟归零，`pt-heartbeat` 校验一致 | DBA |

## 排查过程

先确认是复制慢了还是复制断了。两个信号完全不同：

```bash
mysql -e "SHOW SLAVE STATUS\G" | egrep "Slave_IO_Running|Slave_SQL_Running|Seconds_Behind_Master|Last_SQL_Error"
```

输出里 `Slave_SQL_Running: Yes` 但 `Seconds_Behind_Master: 3120`，说明**线程活着但在追**——不是网络断，也不是权限问题，而是"处理不过来"或"写不下去"。

接着看从库磁盘：

```bash
df -h /data
du -sh /data/mysql/*  | sort -h | tail -10
```

`/data` 100%，占大头的是 `mysql/binlog`（22G）与 relay log（18G）。到这里根因链条已经清楚：磁盘满 → relay log 无法继续写 → SQL 线程停 → 延迟累积。

再反查 binlog 为什么突然涨了 40G：

```bash
ls -lh /data/mysql/binlog/ | tail -20   # 看生成速度
mysql -e "SHOW BINLOG EVENTS IN '<文件名>' LIMIT 20"
```

某个 binlog 文件单个体积 1G（`max_binlog_size`），连续 40 个在 40 分钟内生成——对应的是前一天晚上的 `ALTER TABLE order_item ADD COLUMN ...`，在 800 万行的表上执行了 52 分钟。

**这一步容易被跳过**：很多人看到"磁盘满"就去删日志，删完恢复，然后等下次再满。不追到"谁在写"就不算复盘。

## 根因

- **直接原因**：`/data` 分区写满，relay log 无法落盘，SQL 线程停止。
- **深层原因**：binlog 保留策略是"保留 7 天"，没有体积上限；一次大表 DDL 的 binlog 量远超日常（52 分钟的 row 模式变更 ≈ 40GB），瞬间吃掉了半个月的余量。
- **为什么没早发现**：监控只有"磁盘使用率 > 95% 告警"，而 78% → 96% 只用了 17 分钟——按 5 分钟采集间隔，最多只来得及报一次警，且发生在已经满之后。

## 修复与恢复

1. 在线扩容云盘并扩容文件系统（`growpart` + `xfs_growfs`），先让写入能继续。
2. 清理过期 binlog：

```bash
mysql -e "PURGE BINARY LOGS BEFORE DATE_SUB(NOW(), INTERVAL 3 DAY);"
```

3. 确认 SQL 线程自行恢复；若未恢复，`START SLAVE SQL_THREAD;`。
4. 用 `pt-heartbeat` 校验延迟真实归零，而不是只看 `Seconds_Behind_Master`（后者在长时间停止后可能给出误导性数字）。

验证方式：从库执行 `SELECT MAX(id) FROM orders` 与主库对比，并抽查 5 分钟内的新订单在读从库可查。

## 改进项

| 改进 | 类型 | 负责人 | 期限 |
| --- | --- | --- | --- |
| 磁盘水位分两级告警：80% warning / 90% critical，且按"预测写满时间 < 2 小时"再报一次 | 监控 | DBA | 一周 |
| binlog 保留改为"3 天且总量不超过 30G"，用 `binlog_expire_logs_seconds` + 定时清理脚本双保险 | 配置 | DBA | 一周 |
| 大表 DDL 上线前必须估算 binlog 量（`表行数 × 行平均变更大小`），超过 5G 走变更窗口并提前扩容 | 流程 | 平台 | 两周 |
| 从库延迟告警加上"磁盘水位"作为附带信息，避免下一次还要手动登录确认 | 监控 | 值班 | 一周 |
| 业务读从库的关键接口加"数据新鲜度"校验（比对主库最新 ID），避免静默读旧数据 | 代码 | 业务 | 一个月 |

---

本文按常见生产故障的真实形态整理，用于演示复盘写法；具体数字与时间线为示例。
