## my.cnf 生产基线（4C16G 参考值）

```ini
[mysqld]
innodb_buffer_pool_size = 12G        # 物理内存的 60~70%，这是第一参数
innodb_flush_log_at_trx_commit = 1   # 主库 1（不丢事务），从库可 2
sync_binlog = 1                      # 主库 1，双 1 才是真正的强持久
max_connections = 1000               # 配合连接池，别开到上万
innodb_redo_log_capacity = 2G        # 8.0.30+ 用这个，老版本 innodb_log_file_size
binlog_expire_logs_seconds = 604800  # binlog 留 7 天
long_query_time = 1
slow_query_log = ON
character-set-server = utf8mb4
default-time-zone = '+8:00'
server_id = 11                       # 复制体系内全局唯一
```

参数没有万能值：buffer pool 的正确大小是「热数据全装下」，
换入了再调大只是推迟问题。

## 主从复制搭建

```sql
-- 主库：建复制账号
CREATE USER 'repl'@'10.0.%' IDENTIFIED BY 'ReplPass' REQUIRE SSL;
GRANT REPLICATION SLAVE ON *.* TO 'repl'@'10.0.%';

-- 记下位点（8.0.22+ 推荐查 SHOW BINARY LOG STATUS）
SHOW MASTER STATUS;
```

```sql
-- 从库：指向主库并启动复制（8.0 推荐语法）
CHANGE REPLICATION SOURCE TO
  SOURCE_HOST='10.0.0.11', SOURCE_USER='repl', SOURCE_PASSWORD='ReplPass',
  SOURCE_AUTO_POSITION=1, GET_SOURCE_PUBLIC_KEY=1;
START REPLICA;
SHOW REPLICA STATUS\G    -- Replica_IO_Running 与 Replica_SQL_Running 都要 Yes
```

`SOURCE_AUTO_POSITION=1` 走 GTID，从库不需要手动对位点。

## 备份策略（没有备份演练的库等于裸奔）

```bash
# 每日全量（InnoDB 用 single-transaction，不锁表）
mysqldump --single-transaction --source-data=2 --triggers --routines \
  --all-databases | gzip > /backup/mysql-$(date +%F).sql.gz

# 或物理备份（大库用 XtraBackup，恢复快一个量级）
xtrabackup --backup --target-dir=/backup/base --user=backup -p
```

binlog 连续保留 + 每日全量，恢复才能做到任意时间点（PITR）。

## 慢查询治理

```sql
-- 1. 找出慢 SQL
SELECT * FROM mysql.slow_log ORDER BY query_time DESC LIMIT 10;
-- 2. 看执行计划
EXPLAIN SELECT ...;      -- type 是 ALL 且 rows 巨大 = 全表扫描
-- 3. 补索引后验证
EXPLAIN ANALYZE SELECT ...;
```

## 验收清单

- [ ] 双 1 配置（主库），binlog 开启且定期清理
- [ ] 主从延迟监控（`SHOW REPLICA STATUS` 的秒数）< 1s
- [ ] 每日备份成功且在异机存储，恢复演练做过
- [ ] 业务账号按库授权，没有 `GRANT ALL ON *.*`
- [ ] 慢查询日志开着的，且有每周 review