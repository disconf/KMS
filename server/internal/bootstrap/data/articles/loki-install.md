## 单机（monolithic）模式：大多数团队的正确起点

Loki 与 Prometheus 思路相反：它**不做全文索引**，只索引标签（label），
存储与计算分离，成本可以低一个量级。标签设计好之前不要扩规模。

```bash
# 二进制
wget https://github.com/grafana/loki/releases/download/v3.1.0/loki-linux-amd64.zip
unzip loki-linux-amd64.zip -d /usr/local/bin/
mkdir -p /etc/loki /data/loki

loki -config.file /etc/loki/loki.yml     # 默认 3100 端口
```

单机配置骨架（文件系统存储）：

```yaml
auth_enabled: false
server: { http_listen_port: 3100 }
common:
  path_prefix: /data/loki
  storage:
    filesystem: { chunks_directory: /data/loki/chunks, rules_directory: /data/loki/rules }
  replication_factor: 1
schema_config:
  configs:
    - from: 2026-01-01
      store: tsdb
      object_store: filesystem
      schema: v13
      index: { prefix: index_, period: 24h }
limits_config: { retention_period: 30d }
```

## Docker 方式

```bash
docker run -d --name loki --restart unless-stopped \
  -p 3100:3100 -v /data/loki:/loki \
  grafana/loki:3.1.0 -config.file=/etc/loki/local-config.yaml
```

## K8s（Helm）

```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm install loki grafana/loki -n monitoring --create-namespace \
  --set deploymentMode=SingleBinary \
  --set loki.commonConfig.replication.factor=1 \
  --set loki.storage.type=filesystem \
  --set singleBinary.replicas=1
```

## 微服务模式什么时候才需要

读/写/后端分离（`deploymentMode: Distributed`）在**日吞吐 > 数百 GB** 或
读写负载互相影响时才值得。先单机+对象存储跑半年，瓶颈出现再拆。

## 验证

```bash
curl -s localhost:3100/ready            # ready
curl -s localhost:3100/metrics | grep loki_build_info
logcli query '{job="varlogs"}' --limit 5 --since=1h
```