## 数据库与高可用

Grafana 自带 SQLite，生产换成 MySQL/PostgreSQL 才能多实例：

```ini
[database]
type = mysql
host = 10.0.2.11:3306
name = grafana
user = grafana
password = GrafanaPass

[unified_alerting]
enabled = true
ha_peers = grafana-1:9094,grafana-2:9094   # 多实例告警去重
```

两个 Grafana 实例 + 同一数据库 + `ha_peers`，就是告警高可用的最小形态。

## 大盘即代码（Provisioning）

大盘、数据源、告警全部用文件声明，启动自动加载，进 Git 版本管理：

```yaml
# /etc/grafana/provisioning/datasources/prom.yml
apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    url: http://prometheus:9090
    isDefault: true
```

```yaml
# /etc/grafana/provisioning/dashboards/provider.yml
apiVersion: 1
providers:
  - name: default
    options: { path: /etc/grafana/dashboards }
```

手工在 UI 里改过的大盘要导出 JSON 回 Git，否则下次 provision 会被覆盖——
**团队约定 UI 只读，改动走 Git**，这条纪律比工具本身重要。

## 权限分层

- Org 隔离大环境（生产/预发各自 Org），Folder + Team 控制」粒度；
- Viewer 只读、Editor 能改大盘、Admin 管数据源——默认三层够用；
- `viewers_can_edit` 打开可以让只读用户在「探索」里自查，不必给 Editor。

## 匿名访问治理

对内展示墙可用匿名只读（`auth.anonymous.enabled=true` + org_role=Viewer），
**绝不能给 Editor/Admin**，且要配合网络层限制访问来源。

## 验收清单

- [ ] 数据库外置，两实例负载均衡，告警不重复发送
- [ ] 大盘/数据源全部 provisioning，Git 单一事实源
- [ ] admin 密码非默认，SSO（OIDC/LDAP）接入
- [ ] 告警通道（钉钉/企微/电话）测试通过
- [ ] 升级演练：新版本在测试环境跑一周再上生产（Grafana 升级偶有 breaking change）