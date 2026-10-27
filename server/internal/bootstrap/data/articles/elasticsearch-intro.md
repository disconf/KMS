## 它是什么

Elasticsearch 是基于 **Lucene** 的分布式搜索与分析引擎：把数据建成倒排索引，
支持全文检索、聚合分析与横向扩展。整套日志方案（ELK）里的 E，也是站内搜索、
风控特征查询的常见底座。

## 解决什么问题

关系库的 LIKE '%x%' 扫全表且不懂分词；ES 把文本切词后建倒排索引，检索相关度排序、
高亮、聚合（ facets/统计）都是原生能力。日志场景里它把「grep 十亿行」变成「毫秒级条件查询」。

## 架构与核心概念

| 概念 | 一句话 |
|---|---|
| Index | 逻辑数据集（类似表），物理上由多个 shard 组成 |
| Shard | Lucene 实例，主分片数建索引时定死，副本分片可调 |
| Node / Cluster | 节点按角色分工（master/data/ingest），集群靠选主协调 |
| Mapping | 字段类型定义，倒排索引怎么建由它决定 |
| 倒排索引 | 词 → 文档列表，检索快的原因；写入即建索引，这是写入成本 |

## 版本与生态

- 8.x 主线，7.17 是 7 系收尾版；**许可证**：7.11 起默认 Apache 2.0 回归（2024 年 Elastic 宣布）；
- 同族组件：Kibana（可视化）、Logstash/Filebeat（采集）；替代品 OpenSearch（AWS 分支）；
- JDK 内置（8.x 起），不再需要单独装 Java。

## 适用与不适用

**适合**：全文检索、日志检索与分析、指标聚合、地理查询。
**不适合**：主存储与事务（没有 ACID）、高频更新文档（更新 = 重建索引段）、
强一致查询（近实时 NRT，写入后约 1 秒可查）。

## 官方文档导读

- 总入口：<https://www.elastic.co/guide/en/elasticsearch/reference/current/index.html>
- REST API 索引：<https://www.elastic.co/guide/en/elasticsearch/reference/current/rest-apis.html>
- 集群管理（分片/磁盘水位）：<https://www.elastic.co/guide/en/elasticsearch/reference/current/cluster-management.html>
