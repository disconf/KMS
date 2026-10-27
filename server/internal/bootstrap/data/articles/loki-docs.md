## 文档入口

| 找什么 | 去哪 |
|---|---|
| 总文档 | <https://grafana.com/docs/loki/latest/> |
| LogQL 参考 | <https://grafana.com/docs/loki/latest/query/> |
| 采集端（Alloy/Promtail） | <https://grafana.com/docs/loki/latest/send-data/> |
| 存储与 schema | <https://grafana.com/docs/loki/latest/storage/> |

## 必读页面清单

1. **Get started with labels**——标签设计心法，读 Loki 其他任何文档之前先读这篇；
2. **LogQL**——管道操作符（`|=` 行过滤、`| json` 解析、`| unwrap` 提数值）全表；
3. **Storage / schema**——tsdb + 对象存储是生产标配，理解 period 与 bucket 的关系；
4. **Operations**（compactor/retention 一节）——保留期真正生效的开关在这里；
5. **Alloy 文档**——新一代采集端，配置语法与 Promtail 有差异。

## 推荐学习路径

1. **第 1 天**：docker-compose 起 Loki+Grafana+Promtail → 收 `/var/log` →
   在 Grafana Explore 里写 `{job="varlogs"}` 查出内容；
2. **第 1 周**：给自己的应用日志配 pipeline（提 level 到标签）→ 写 LogQL 统计
   「5 分钟 ERROR 数」→ 把这张图配成告警；理解流（stream）与标签基数的关系；
3. **第 1 个月**：迁到对象存储 + tsdb schema → 演练保留期删除 →
   压测写入量并调整 rate/burst 限额 → 建立「流数量」监控看板。

## 版本选择与升级文档

- 升级指南：文档内「Upgrade」页，**schema/store 升级有严格顺序**（先扩 schema 版本再动服务）；
- 2.x→3.x 有 breaking change（配置项改名、默认值变化），按升级指南逐项过；
- Promtail 维护模式声明后新功能都进 Alloy，新项目直接选 Alloy 少走回头路。

## 社区与求助渠道

- Grafana Labs 社区：<https://community.grafana.com/c/grafana-loki/>；
- GitHub：<https://github.com/grafana/loki/issues>；
- 提问带上：部署形态（单体/微服务）、配置 yaml 相关段、`/metrics` 里的
  `loki_ingester_streams` 与写入报错原文；
- 「查不到日志」类问题九成是标签不匹配或时间范围没选对，先自我排查这两项。
