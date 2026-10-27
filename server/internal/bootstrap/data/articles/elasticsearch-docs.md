## 文档入口

| 找什么 | 去哪 |
|---|---|
| Reference（当前版） | <https://www.elastic.co/guide/en/elasticsearch/reference/current/index.html> |
| REST API 参考 | Reference 内「REST APIs」章 |
| 查询 DSL 教程 | Reference 内「Query DSL」章 |
| 版本下载与生命周期 | <https://www.elastic.co/support/eol> |

文档默认显示当前版本，**左上角切版本**是第一个要养成的习惯——7 与 8 的 API 细节差异不少。

## 必读页面清单

1. **Query DSL**——bool 组合查询（must/should/filter）是所有检索的骨架，
   filter 上下文不算分可缓存，能不用 query 就不用；
2. **Cat API**（`_cat/nodes`、`_cat/shards`、`_cat/indices`）——运维排障的全套眼睛；
3. **Cluster health 与磁盘水位**——黄/红状态与 flood_stage 的语义；
4. **Index lifecycle management (ILM)**——日志场景 hot/warm/cold/删除的自动化；
5. **Aggregations**——不做搜索只做统计时，这一章就是你的 SQL GROUP BY。

## 推荐学习路径

1. **第 1 天**：单节点起服务 → 用 Dev Tools 跑通 index/文档 CRUD → `_cat` 全家桶过一遍；
2. **第 1 周**：建一个带 mapping 的索引（keyword vs text 的区别务必弄懂），
   练 bool+filter+聚合；模拟一次磁盘水位触发只读并用 API 解锁；
3. **第 1 个月**：搭 3 节点集群，动手做一次滚动升级与分片迁移（`_cluster/reroute`）；
   给日志索引配一套 ILM 策略。

## 版本选择与升级文档

- 升级手册：Reference 内「Upgrading Elasticsearch」，跨大版本必须先看
  **Breaking changes**（8.0 起安全配置默认启用，升级路径有强制顺序）；
- 滚动升级原则：禁分片分配 → 升 data 节点一个个来 → 最后升 master；
- Kibana 版本必须 ≥ ES 版本，先升 ES 后升 Kibana 的窗口要短。

## 社区与求助渠道

- 官方论坛：<https://discuss.elastic.co/>——回复质量高，官方工程师活跃；
- 提问带上：版本、`_cluster/health`、`_cat/shards` 输出、出问题的请求体与响应；
- 中文内容注意时效，7.x 之前的教程很多配置已改名（如 `type` 移除）。
