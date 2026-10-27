## 它是什么

Grafana 是**可视化与告警面板**：不存数据，连上 Prometheus/MySQL/Loki/ES 等几十种数据源，
把查询变成可共享的看板，并在指标越限时发告警。运维「出事时第一个打开的页面」通常就是它。

## 解决什么问题

监控数据存在 TSDB 里，`curl API` 查出来是人看不了的 JSON。Grafana 做三件事：
把查询变成图（变量、阈值、注释）、把图组织成看板（按服务/环境分屏）、
把超阈值变成通知（分级路由到不同群）。

## 架构与核心概念

| 概念 | 一句话 |
|---|---|
| Data Source | 数据源连接配置，一个 Grafana 可挂多个不同类型的库 |
| Dashboard / Panel | 看板 / 面板，每个面板绑定一条查询 |
| Variable | 看板变量（下拉框），一块大盘覆盖多环境多实例的关键 |
| Alerting | 告警规则 + 联络点（Contact Point）+ 通知策略（路由） |
| Provisioning | 用 YAML 文件声明数据源/看板，进 Git 版本管理 |

## 版本与生态

- OSS 版免费，企业版加 RBAC 等能力；10.x/11.x 主线，半年一个大版本；
- 部署形态：裸机 / Docker / Helm，配置入口有 `grafana.ini` 与环境变量两种；
- 插件生态：面板插件（时钟、流程图）与数据源插件按需装，生产尽量少装减少升级面。

## 适用与不适用

**适合**：指标大盘、日志检索界面（配 Loki）、告警可视化与路由、给业务方只读视图。
**不适合**：当报表系统给非技术团队做复杂表格（体验一般）、存储任何数据（它只是视图层）。

## 官方文档导读

- 总入口：<https://grafana.com/docs/grafana/latest/>
- 配置参考（grafana.ini 全字段）：<https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/>
- 告警文档：<https://grafana.com/docs/grafana/latest/alerting/>
