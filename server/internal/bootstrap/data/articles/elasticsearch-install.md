## 前置检查（比安装本身重要）

```bash
# 1. 系统：vm.max_map_count 与 nofile，不改起不来
cat >> /etc/sysctl.conf <<EOF
vm.max_map_count = 262144
EOF
sysctl -p
ulimit -n 65536        # 持久化到 /etc/security/limits.conf

# 2. 内存：堆 <= 物理内存一半且 <= 31G（压缩指针失效线）
free -h

# 3. 禁 swap（或 swappiness=1），ES 的 GC 对 swap 极敏感
sysctl vm.swappiness=1
```

## 压缩包安装（推荐，最可控）

```bash
useradd -r -s /sbin/nologin elastic
tar xzf elasticsearch-8.14.0-linux-x86_64.tar.gz -C /opt/
mkdir -p /data/es && chown -R elastic:elastic /opt/elasticsearch-8.14.0 /data/es

# jvm 参数：Xms = Xmx，且不超过物理内存一半
sed -i 's/^-Xms4g/-Xms8g/;s/^-Xmx4g/-Xmx8g/' /opt/elasticsearch-8.14.0/config/jvm.options
```

`elasticsearch.yml` 最小配置：

```yaml
cluster.name: ops-logs
node.name: es-1
path.data: /data/es
network.host: 0.0.0.0
discovery.seed_hosts: ["es-1", "es-2", "es-3"]
cluster.initial_master_nodes: ["es-1", "es-2", "es-3"]
xpack.security.enabled: true
```

以 elastic 用户启动：`su - elastic -c '/opt/elasticsearch-8.14.0/bin/elasticsearch -d'`。

## rpm/deb 安装

```bash
rpm -i elasticsearch-8.14.0-x86_64.rpm
systemctl enable --now elasticsearch
# 配置在 /etc/elasticsearch/，数据目录默认 /var/lib/elasticsearch（改到数据盘）
```

## Docker 方式（单节点开发）

```bash
docker run -d --name es --restart unless-stopped \
  -p 9200:9200 -e discovery.type=single-node \
  -e xpack.security.enabled=false \
  -v /data/es:/usr/share/elasticsearch/data \
  -e ES_JAVA_OPTS="-Xms2g -Xmx2g" \
  elasticsearch:8.14.0
```

## 验证

```bash
curl -s localhost:9200 | jq '.version.number, .cluster_name'
curl -s localhost:9200/_cluster/health | jq '.status'    # green 才算完
```