## 三种方式

| 方式 | 适用 | 说明 |
|---|---|---|
| 二进制 + systemd | 传统虚机，最常见 | 自己管配置与升级 |
| Docker | 单机快速起，采集端多时不够 | 数据卷必须挂出来 |
| kube-prometheus-stack | K8s 集群监控 | Helm 一键带 Grafana / 告警规则 / 采集配置 |

## 二进制安装

```bash
wget https://github.com/prometheus/prometheus/releases/download/v2.53.0/prometheus-2.53.0.linux-amd64.tar.gz
tar xzf prometheus-*.tar.gz && cd prometheus-*
mv prometheus promtool /usr/local/bin/
mkdir -p /etc/prometheus /data/prometheus
cp prometheus.yml /etc/prometheus/

useradd -r -s /sbin/nologin prometheus
chown -R prometheus:prometheus /data/prometheus
```

systemd 单元（资源限制在这里给）：

```ini
[Service]
User=prometheus
ExecStart=/usr/local/bin/prometheus \
  --config.file=/etc/prometheus/prometheus.yml \
  --storage.tsdb.path=/data/prometheus \
  --storage.tsdb.retention.time=15d \
  --web.enable-lifecycle
Restart=always
LimitNOFILE=65536
```

`--web.enable-lifecycle` 打开后 `curl -X POST localhost:9090/-/reload` 可热加载配置。

## Docker 方式

```bash
docker run -d --name prom --restart unless-stopped \
  -p 9090:9090 \
  -v /etc/prometheus:/etc/prometheus:ro \
  -v /data/prometheus:/prometheus \
  prom/prometheus:v2.53.0 \
  --config.file=/etc/prometheus/prometheus.yml \
  --storage.tsdb.retention.time=15d
```

## K8s（kube-prometheus-stack）

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install mon prometheus-community/kube-prometheus-stack -n monitoring --create-namespace \
  --set prometheus.prometheusSpec.retention=15d \
  --set prometheus.prometheusSpec.resources.limits.memory=4Gi
```

## 验证

```bash
promtool check config /etc/prometheus/prometheus.yml   # 配置校验
curl -s localhost:9090/-/healthy                        # healthy
curl -s 'localhost:9090/api/v1/targets' | grep -o '"health":"[a-z]*"' | sort | uniq -c
```

第一个指标：`up` 查询里所有 target 的值都该是 1，`up == 0` 就是采集断了。