## 账号与权限

```sql
-- 最小授权原则：按库给，不用 %，不带 ALL
CREATE USER 'app'@'10.0.1.%' IDENTIFIED BY 'AppPass';
GRANT SELECT, INSERT, UPDATE, DELETE ON appdb.* TO 'app'@'10.0.1.%';

CREATE USER 'readonly'@'10.0.%' IDENTIFIED BY 'RoPass';
GRANT SELECT ON appdb.* TO 'readonly'@'10.0.%';

SHOW GRANTS FOR 'app'@'10.0.1.%';       -- 定期审计
ALTER USER 'app'@'10.0.1.%' PASSWORD EXPIRE INTERVAL 90 DAY;
```

## 备份与恢复

```bash
# 全量
mysqldump --single-transaction --all-databases --source-data=2 | gzip > full.sql.gz
# 单库 / 单表
mysqldump appdb orders > orders.sql

# 恢复
mysql -e "CREATE DATABASE appdb"
zcat full.sql.gz | mysql
mysql appdb < orders.sql

# binlog 做时间点恢复（PITR）：全量之后重放 binlog 到目标时刻
mysqlbinlog --start-datetime="2026-10-26 10:00:00" \
            --stop-datetime="2026-10-26 10:30:00" binlog.000123 | mysql
```

## 主从巡检与处理

```sql
SHOW REPLICA STATUS\G
-- 延迟秒数看 Seconds_Behind.Source；断开看 Last_IO_Error / Last_SQL_Error
STOP REPLICA; SET GLOBAL replica_parallel_workers = 4; START REPLICA;  -- 提并行回放
-- 跳过一条坏事务（GTID 模式先注入空事务，再 START REPLICA）
```

## 巡检命令

```sql
SHOW PROCESSLIST;                       -- 当前连接在干嘛
SHOW ENGINE INNODB STATUS\G             -- 锁等待与死锁（看 LATEST DETECTED DEADLOCK）
SELECT count(*) FROM information_schema.processlist WHERE command='Sleep' AND time>60;
SHOW VARIABLES LIKE 'max_connections';
```

## 「CPU 打满」的排查路径

1. `SHOW PROCESSLIST`：找 `Sending data` / `Copying to tmp table` 的长查询；
2. `EXPLAIN` 这批查询：全表扫描 → 补索引或改写；
3. 不是查询问题就查连接数：应用连接池泄漏会让 MySQL 空转大量线程；
4. 紧急止血 `KILL <id>`，别用 `SET GLOBAL max_connections` 掩盖问题；
5. 复盘进慢日志体系：`long_query_time=1` + 每周 review。

## 日常组合拳

```bash
# 每日巡检脚本骨架
mysql -e "SHOW REPLICA STATUS\G" | grep -E 'Running|Behind'
mysql -e "SHOW GLOBAL STATUS" | grep -E 'Threads_connected|Threads_running|Aborted_connects'
mysql -e "SELECT count(*) FROM information_schema.tables WHERE table_schema='appdb' AND engine!='InnoDB'"
```