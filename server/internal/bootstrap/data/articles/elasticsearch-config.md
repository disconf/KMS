## 配置文件在哪

| 文件 | 用途 |
|---|---|
| `config/elasticsearch.yml` | 集群与节点核心配置 |
| `config/jvm.options` | JVM 堆（Xms=Xmx，且 ≤31GB） |
| `/etc/sysctl.d/elasticsearch.conf` | `vm.max_map_count=262144`（必调内核参数） |
| `/etc/security/limits.conf` | memlock/nofile 提额 |

容器与 rpm 部署的路径略不同，`elasticsearch.yml` 优先级永远高于环境变量。

## 参数怎么读

yml 是扁平 key-value。改完必须重启进程才生效（除少数 cluster settings 可通过
API 热调：`PUT /_cluster/settings`，优先用 API 改集群级参数）。

## 核心参数分组

**集群标识与发现**

```yaml
cluster.name: logs-prod          # 同集群必须一致
node.name: es-01
discovery.seed_hosts: ["10.0.0.1:9300","10.0.0.2:9300"]
cluster.initial_master_nodes: ["es-01","es-02","es-03"]  # 仅首次组建集群有效
network.host: 0.0.0.0            # 生产建议绑内网地址
```

**数据路径与角色**

```yaml
path.data: /data/es
node.roles: [ data, ingest ]     # 专用 master 节点: [ master ]
```

**集群级热调（API）——磁盘水位三兄弟**

```json
PUT /_cluster/settings
{"persistent":{
  "cluster.routing.allocation.disk.watermark.low":  "85%",
  "cluster.routing.allocation.disk.watermark.high": "90%",
  "cluster.routing.allocation.disk.watermark.flood_stage": "95%"
}}
```

flood_stage 触发后索引变只读，这是「磁盘满了以后全站写入失败」的直接原因。

## 常见误配

1. **堆大于 31GB**——超过压缩指针阈值，实际可用内存反而变小；
2. **堆与 Lucene 抢内存**——Lucene 靠系统页缓存，机器内存至少给堆留一半以上的空间不进堆；
3. **分片数拍脑袋**——单分片建议 10~50GB，几千个小分片会让 master 状态管理压垮；
4. **`cluster.initial_master_nodes` 留在已建成集群里**——重启会试图重新选主，把它从生产配置删掉；
5. **swapping**——`bootstrap.memory_lock: true` + memlock 提额，锁住堆不进交换区。

## 校验与生效

```bash
curl -s localhost:9200/_cluster/health?pretty    # status 绿/黄/红
curl -s localhost:9200/_cat/nodes?v              # 角色、堆、磁盘一览
curl -s localhost:9200/_cat/shards?v | head      # 分片分布
curl -s "localhost:9200/_nodes/stats/jvm?pretty" # GC 情况
```
