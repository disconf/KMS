## 配置文件在哪

| 安装方式 | 主配置 |
|---|---|
| yum/apt | `/etc/my.cnf` + `/etc/my.cnf.d/` |
| 二进制 | `basedir/my.cnf` 或 `--defaults-file` 指定 |
| 容器 | 挂载 `/etc/mysql/conf.d/my.cnf` |

读生效值用 `SHOW VARIABLES LIKE 'xxx'`；改配置 = 改文件 + 重启（部分参数
`SET GLOBAL` 可热改，但同样不持久）。

## 参数怎么读

my.cnf 按 `[mysqld]` 段组织。**参数分动态与只读**两类：
`SET GLOBAL` 立即生效的写进文件防重启丢失；只读参数（如 innodb_buffer_pool_size
在 8.0 仍不可在线调）改动必须排重启窗口。

## 核心参数分组

**内存（第一优先级）**

```ini
innodb_buffer_pool_size = 物理内存的 50%~70%   # 最重要的一行
innodb_log_file_size   = 1G                   # 写密集场景调大，减少 checkpoint 压力
```

**连接**

```ini
max_connections        = 500        # 别无脑调大，每个连接都有内存成本
wait_timeout           = 600        # 空闲连接回收，防连接池泄漏拖死实例
max_allowed_packet     = 64M        # 大 SQL/大字段按需调
```

**日志与复制**

```ini
log_bin                = mysql-bin  # 8.0 默认开启
binlog_format          = ROW        # 主流选择
binlog_expire_logs_seconds = 604800 # binlog 保留 7 天，磁盘紧张先调它
gtid_mode              = ON
enforce_gtid_consistency = ON
slow_query_log         = ON
long_query_time        = 1
```

**InnoDB 行为**

```ini
innodb_flush_log_at_trx_commit = 1   # 1=每次提交刷盘（默认，最安全）
sync_binlog            = 1           # 与上一条凑成「双 1」，金融级 durability
innodb_flush_method    = O_DIRECT    # 避免双写缓存
```

## 常见误配

1. **buffer_pool 调到内存 90%**，留下不够系统与连接开销，反而频繁 swap；
2. **双 1 改成 2/0 图性能**——宕机丢事务，除非明确接受，否则别动；
3. **max_connections 调到 5000**——瓶颈在锁与 IO，不是连接数，先查慢查询；
4. **没开慢查询日志**，出问题只能靠猜；
5. **utf8mb4 项目里建了 utf8（3 字节）**，emoji 直接报错。

## 校验与生效

```sql
SHOW VARIABLES LIKE 'innodb_buffer_pool_size';
SHOW STATUS LIKE 'Threads_connected';
SELECT * FROM sys.innodb_buffer_stats_by_table;  -- sys schema 是现成的诊断视图
EXPLAIN <慢SQL>;                                  -- 索引是否走上的唯一判据
```
