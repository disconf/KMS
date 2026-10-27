## 它是什么

MySQL 是全球部署最广的**开源关系型数据库**（GPL 双许可，Oracle 持有），默认存储引擎 InnoDB
提供事务、行锁与崩溃恢复。运维语境里的 MySQL 通常指「InnoDB + 主从复制 + SQL 层」这一整套。

## 解决什么问题

需要**事务一致性**与**结构化查询**的数据都放这：订单、账户、库存、用户关系。
它不是最快的，但 ACID、丰富的工具链（备份、监控、慢查询分析）与最大的人才池
让它成为业务主库的默认答案。

## 架构与核心概念

| 概念 | 一句话 |
|---|---|
| Server 层 | 连接管理、SQL 解析、优化器、执行器 |
| InnoDB | 存储引擎：B+ 树、事务（redo/undo）、行锁、MVCC |
| binlog | Server 层日志，复制与时间点恢复都靠它 |
| 主从复制 | 从库拉主库 binlog 重放，读写分离与容灾的基础 |
| GTID | 全局事务标识，让主从切换/搭建从库不再数 binlog 位点 |

## 版本与生态

- **8.0 是当前主线**（8.4 LTS 已发布），5.7 已 EOL，别再新建 5.7 实例；
- 分支生态：Percona Server（增强工具与审计）、MariaDB（社区分支，注意兼容性差异）；
- 周边必备：Percona XtraBackup（物理备份）、pt-toolkit（pt-query-digest 等）、orchestrator（拓扑管理）。

## 适用与不适用

**适合**：事务型业务主库、中小规模读多写少（配缓存后覆盖绝大多数业务）。
**不适合**：单表数十亿行的分析型负载（选 OLAP 引擎）、无结构海量写入（选时序/文档库）——
但主库之上挂一个 CDC（如 Canal）同步出去，两条路就都有了。

## 官方文档导读

- Reference Manual（唯一权威）：<https://dev.mysql.com/doc/refman/8.0/en/>
- InnoDB 专题：<https://dev.mysql.com/doc/refman/8.0/en/innodb-storage-engine.html>
- 复制专题：<https://dev.mysql.com/doc/refman/8.0/en/replication.html>
