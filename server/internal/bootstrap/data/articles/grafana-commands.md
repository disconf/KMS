## CLI 管理

```bash
grafana-cli plugins ls                                  # 已装插件
grafana-cli plugins install grafana-piechart-panel      # 装插件，装完要重启
grafana-cli plugins update-all
grafana-cli --pluginsDir /var/lib/grafana/plugins plugins install <id>
```

容器里装插件更简单：环境变量 `GF_INSTALL_PLUGINS=grafana-piechart-panel,grafana-clock-panel`。

## 配置校验与调试

```bash
grafana server --config /etc/grafana/grafana.ini --homepath /usr/share/grafana \
  --packaging=deb cfg:default.log.level=debug    # 前台 debug 起一次看日志
```

## 数据源连通性测试（API）

```bash
# 列数据源
curl -s -u admin:Pass localhost:3000/api/datasources | jq '.[].name'
# 测试某个数据源
curl -s -u admin:Pass -X POST localhost:3000/api/datasources/1/health | jq
# 用数据源跑一条查询（代理到后端）
curl -s -u admin:Pass -X POST localhost:3000/api/ds/proxy/1/api/v1/query \
  --data-urlencode 'query=up' | jq '.data.result | length'
```

## 大盘导入导出

```bash
# 导出全部大盘为 JSON（备份/迁移）
for id in $(curl -s -u admin:Pass 'localhost:3000/api/search?type=dash-db' | jq '.[].id'); do
  curl -s -u admin:Pass localhost:3000/api/dashboards/id/$id \
    | jq '.dashboard' > dash-$id.json
done

# 导入
curl -s -u admin:Pass -X POST localhost:3000/api/dashboards/db \
  -H 'Content-Type: application/json' \
  -d '{"dashboard":'"$(cat dash-1.json)"',"overwrite":true}'
```

## 「图打不开」排查清单

1. `curl /api/health`：database=ok 才是 Grafana 自己没问题；
2. 数据源 health 接口：连不通的是 Prometheus/ES 地址，不是 Grafana；
3. 面板报「no data」：在 Explore 里手跑同一条查询，区分查询问题与面板配置问题；
4. 时间范围与刷新频率：`now-15m` 的窗口里可能就是没有数据；
5. 慢查询超时：Grafana 代理超时一般跟数据源端查询超时一致，先优化查询再看代理日志。

## 日志位置

```bash
tail -f /var/log/grafana/grafana.log       # 虚机
docker logs -f grafana                     # 容器
journalctl -u grafana-server -f            # systemd
```