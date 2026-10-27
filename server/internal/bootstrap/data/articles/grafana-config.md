## 配置文件在哪

| 部署 | 主配置 |
|---|---|
| rpm/deb | `/etc/grafana/grafana.ini` |
| Docker | 镜像内 `/etc/grafana/grafana.ini`，或全用 `GF_*` 环境变量 |
| Helm | values.yaml 里 `grafana.ini` 段 |

两种写法等价：ini 的 `[server] root_url` 等于环境变量 `GF_SERVER_ROOT_URL`。
环境变量优先，容器化部署基本全走 `GF_*`。

## 参数怎么读

ini 用分号注释——**默认文件里几乎所有行都是注释掉的默认值**，改哪行就生效哪行，
diff 上游模板能看到版本新增项。

## 核心参数分组

**服务与访问（[server] [security]）**

```ini
[server]
root_url = https://grafana.example.com   # 反代后必须配，否则跳转错乱

[security]
admin_user = admin
admin_password = 首次登录改掉            # 或启动时用环境变量给
disable_gravatar = true
```

**数据库与高可用（[database]）**

```ini
[database]
type = mysql
host = 10.0.0.5:3306
name = grafana
user = grafana
password = ***
# 多副本高可用的前提：会话/看板都在外部数据库
```

**认证（[auth.*]）**

```ini
[auth.anonymous]
enabled = false          # 大屏公开展示才开，且配 org_role=Viewer

[auth.ldap] / [auth.generic_oauth]
# 接入企业账号体系，SSO 统一入口
```

## 常见误配

1. **反代后 root_url 没改**——登录后跳回 localhost；
2. **SQLite 多副本**——两个实例各写各的库，看板「时有时无」；
3. **匿名开成 Editor**——大盘被人改了都不知道；
4. **告警测试只发给自己**——Contact Point 的路由（policy）没配，真实告警没人收到；
5. **升级后插件失效**——插件版本与 Grafana 主版本要匹配，升级前查插件兼容表。

## 校验与生效

```bash
grafana cli plugins ls            # 已装插件
systemctl restart grafana-server  # ini 改动需重启
curl -s localhost:3000/api/health
curl -s -H "Authorization: Bearer $TOKEN" localhost:3000/api/datasources  # API 校验
tail -f /var/log/grafana/grafana.log
```
