## 文档入口

| 找什么 | 去哪 |
|---|---|
| 文档总入口 | <https://grafana.com/docs/grafana/latest/> |
| 配置字段全表 | <https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/> |
| 告警（新版 Unified Alerting） | <https://grafana.com/docs/grafana/latest/alerting/> |
| 大盘/面板示例库 | <https://grafana.com/grafana/dashboards/> |

## 必读页面清单

1. **Configure Grafana**——ini/环境变量/默认值三者的优先级，配错的头号来源；
2. **Dashboards → Variables**——变量（query/interval/custom 类型）写法，
   一块大盘管多环境的核心技能；
3. **Provisioning**——datasources/dashboards 的 YAML 声明，看板即代码；
4. **Alerting → Configure notification policies**——告警路由树，
   理解了 label 匹配就理解了全部路由；
5. **Image rendering / Share**——导出 PNG 与快照分享，给非登录用户的视图。

## 推荐学习路径

1. **第 1 天**：连上 Prometheus → 用官方导入一条 Node Exporter Full 大盘（ID 1860）→
   改一个变量看看过滤怎么生效；
2. **第 1 周**：自建一块业务大盘（QPS/延迟/错误率三行）；配一条告警从
   rule → contact point → 钉钉/企微走通；把数据源和大盘写成 provisioning YAML 进 Git；
3. **第 1 个月**：接 SSO（OAuth/LDAP），按团队分 folder 与权限；
   治理一遍「三年没人看的大盘」——看板也有维护成本。

## 版本选择与升级文档

- What's new 页 <https://grafana.com/docs/grafana/latest/whatsnew/> 与升级指南
  （Setup → Upgrade Grafana）必读——**大版本升级有 breaking change**（如告警系统迁移）；
- 升级顺序：备份数据库 → 起新版只读验证 → 切流量；SQLite 用户升级前先整体备份文件；
- 插件兼容：升级主版本前核对每个插件的 supported versions。

## 社区与求助渠道

- 官方社区：<https://community.grafana.com/>；
- GitHub：<https://github.com/grafana/grafana/issues>；
- 提问带上：版本、部署方式、`grafana.log` 对应时间行、复现步骤；
- 大盘/查询类问题直接贴 JSON（面板导出），比截图有效十倍。
