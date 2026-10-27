## 文档入口

| 找什么 | 去哪 |
|---|---|
| Reference Manual（唯一权威） | <https://dev.mysql.com/doc/refman/8.0/en/> |
| 系统变量索引 | Manual 内「Server System Variables」一章 |
| sys schema 文档 | <https://dev.mysql.com/doc/refman/8.0/en/sys-schema.html> |
| 错误码手册 | Manual 内「Server Error Message Reference」 |

## 必读页面清单

1. **InnoDB 锁与事务模型**（Locks/Transactions 一章）——死锁分析绕不开，
   `SELECT ... FOR UPDATE` 加了什么锁这篇讲得最细；
2. **Replication 一章**——GTID、半同步、延迟排查的全套机制；
3. **Server System Variables**——每个参数的默认值/取值范围/是否动态，
   改配置前在这核对一遍比看博客可靠；
4. **EXPLAIN 输出格式**——看懂 type/key/rows/Extra 四列，慢查询就懂了一大半。

## 推荐学习路径

1. **第 1 天**：装好 → 建库建用户 → `SHOW VARIABLES` 对照本机 my.cnf，
   把上面「核心参数分组」逐个找到对应值；
2. **第 1 周**：搭一套 GTID 主从；用 sys.schema + EXPLAIN 治理一条慢查询，
   走完「慢日志 → explain → 加索引 → 复测」完整闭环；
3. **第 1 个月**：用 XtraBackup 做一次全量 + 增量备份与恢复演练；
   模拟一次主从延迟，练习从 `SHOW SLAVE STATUS` 的 Seconds_Behind 反查原因。

## 版本选择与升级文档

- 生命周期看 <https://www.mysql.com/support/supportedplatforms/database.html>；
- 5.7→8.0 用官方 `mysqlshell util check-for-server-upgrade` 先扫不兼容项
  （保留字、默认字符集、sql_mode 变化）；
- 升级顺序：从库先升 → 切主 → 旧主升，回滚窗口是主从切换的时间。

## 社区与求助渠道

- 官方论坛与 bugs 库：<https://bugs.mysql.com/>（很多「诡异行为」其实是有登记的 bug）；
- Percona 博客：<https://www.percona.com/blog/>——运维向干货密度最高；
- 提问带上：版本 + `SHOW CREATE TABLE` + `EXPLAIN` + my.cnf 相关段，
  四样齐了基本能得到确定答案。
