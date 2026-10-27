## 文档入口

| 找什么 | 去哪 |
|---|---|
| 总文档 | <https://prometheus.io/docs/introduction/overview/> |
| PromQL 函数全表 | <https://prometheus.io/docs/prometheus/latest/querying/functions/> |
| 命令行参数（含存储） | <https://prometheus.io/docs/prometheus/latest/command-line/prometheus/> |
| 最佳实践合集 | <https://prometheus.io/docs/practices/naming/> |

## 必读页面清单

1. **PromQL Basics**——瞬时向量 vs 区间向量，90% 的查询错误源于没分清这两者；
2. **Functions**——rate/increase/histogram_quantile 三件套的语义与坑；
3. **Configuration**——scrape_config 里各服务发现（file_sd/dns_sd/kubernetes_sd）；
4. **Storage**——内存占用估算与长期存储方案的选型依据；
5. **Alerting rules + Alertmanager 配置**——分组/抑制/静默三个概念。

## 推荐学习路径

1. **第 1 天**：装上 → 抓自己的 node_exporter → 写三条 PromQL
   （当前 CPU、5 分钟 rate、内存可用百分比）；
2. **第 1 周**：写第一组告警规则接上 Alertmanager，故意触发一次 down 告警走完
   「触发 → 分组 → 通知」全链路；理解 up 指标与 scrape 失败的关系；
3. **第 1 个月**：给自己业务埋直方图指标（P95 延迟），做一张 Grafana 大盘；
   核对一次内存/磁盘与预估公式的偏差，形成自己的容量参数。

## 版本选择与升级文档

- Release notes：<https://github.com/prometheus/prometheus/releases>，重点看
  breaking changes 与 TSDB 兼容说明；
- 升级即换二进制重启（数据目录兼容时），先 `promtool tsdb` 校验备份；
- Alertmanager 与 Prometheus 版本解耦，各自独立升级。

## 社区与求助渠道

- GitHub Discussions/Issues：<https://github.com/prometheus/prometheus>；
- CNCF 官方 Slack #prometheus 频道；
- 提问带上：版本、prometheus.yml 相关段、`/api/v1/targets` 输出、
  出问题的 PromQL 与数据样本——查询类问题没数据样本基本猜不出来。
