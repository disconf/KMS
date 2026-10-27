## 它是什么

Prometheus 是**拉模式的时序监控数据库 + 告警引擎**：定期抓取（scrape）各目标暴露的
`/metrics` 指标，存成本地时序，用 PromQL 查询，配合 Alertmanager 发告警。
CNCF 毕业项目，云原生监控的事实标准。

## 解决什么问题

「系统现在和过去一会儿发生了什么」——CPU、QPS、延迟分布、队列积压，全部是
随时间变化的数值序列。Prometheus 用多维标签模型（指标名 + 一组 key=value）统一描述，
让「查 5 分钟内 error 率超过 1% 的接口」变成一条 PromQL。

## 架构与核心概念

| 概念 | 一句话 |
|---|---|
| 指标（metric） | `名字{标签=值} 数值`，计数器/仪表/直方图/摘要四类 |
| scrape | 定时（默认 15s）拉目标的 /metrics |
| PromQL | 瞬时/区间向量查询语言，聚合与函数都在这 |
| Alertmanager | 接收告警后做分组、抑制、静默、路由到通知渠道 |
| 本地存储 | 自带 TSDB，长期存储靠联邦/Thanos/VictoriaMetrics |

**拉 vs 推**：拉模式让监控端掌握采集节奏，目标列表可来自服务发现，
这是它对动态环境友好的原因。

## 版本与生态

- 每 6 周一个小版本，2.x 主线；升级通常是平滑的，看 changelog 的 breaking 段；
- 生态组件：node_exporter（主机）、blackbox_exporter（拨测）、Grafana（展示）、
  Alertmanager（告警）、kube-prometheus-stack（K8s 一键全家桶）；
- 兼容替代：VictoriaMetrics/Mimir 用于超大规模长期存储。

## 适用与不适用

**适合**：基础设施与服务的指标监控、SLO 告警、容量趋势。
**不适合**：日志（选 Loki/ELK）、事件追踪（选 OTel/Jaeger）、
要求单条数据不丢的计费（监控允许短暂缺口）。

## 官方文档导读

- 总入口：<https://prometheus.io/docs/>
- PromQL 教程：<https://prometheus.io/docs/prometheus/latest/querying/basics/>
- 配置参考：<https://prometheus.io/docs/prometheus/latest/configuration/configuration/>
