## 一句话结论

一个 K8s 节点的 conntrack 表被打满，内核开始丢包（`nf_conntrack: table full, dropping packet`），kubelet 与 API Server 的心跳因此超时，节点被判 NotReady，节点上的 Pod 被批量驱逐。

## 影响范围

- 受影响业务：该节点上的 43 个 Pod（含 2 个有状态副本）
- 起止时间：14:20 节点 NotReady，15:05 恢复调度，15:40 有状态副本数据追平
- 影响面：无状态服务靠多副本扛住；有状态服务单副本短暂不可用（这也暴露了"有状态服务单副本"本身就是隐患）

## 时间线

| 时间 | 发生了什么 | 谁在处理 |
| --- | --- | --- |
| 14:20 | 监控告警：节点 `node-07` NotReady | 值班 |
| 14:24 | `kubectl describe node node-07` 显示 `Kubelet stopped posting node status` | 值班 |
| 14:26 | SSH 登节点发现响应极慢，`ping` 丢包 12% | 值班 |
| 14:31 | `dmesg -T \| tail -50` 发现大量 `nf_conntrack: table full, dropping packet` | 值班 |
| 14:38 | `nf_conntrack_count` 已到 `nf_conntrack_max`（65536） | 值班 |
| 14:45 | 临时调大 `nf_conntrack_max` 到 262144，丢包停止 | 值班 |
| 14:52 | kubelet 心跳恢复，节点状态回到 Ready | 值班 |
| 15:40 | 有状态副本完成数据同步 | DBA |

## 排查过程

节点 NotReady 有三类常见原因，先按成本从低到高排：

1. **kubelet 自己挂了** —— `systemctl status kubelet`
2. **节点资源耗尽**（磁盘、内存、PID）—— `df -h`、`free -g`、`ps -eLf | wc -l`
3. **网络不通**（到 API Server 或 CNI 有问题）—— 这时候 Pod 还活着但控制面联系不上

本次 `systemctl status kubelet` 显示进程活着、重启次数 0，磁盘与内存都正常。但节点 `ping` 有丢包、SSH 卡顿——说明是**网络层**。

内核查丢包原因：

```bash
dmesg -T | grep -i -E "conntrack|drop" | tail -30
```

关键输出：

```
nf_conntrack: table full, dropping packet
```

再看用量：

```bash
cat /proc/sys/net/netfilter/nf_conntrack_count
cat /proc/sys/net/netfilter/nf_conntrack_max
```

`count` 顶到 `max`（65536）。到这个状态，新连接一律被丢——包括 kubelet 到 API Server 的 10250 心跳。

**这时候要回答的是"谁占了这么多连接"**，而不是急着调大上限：

```bash
ss -s                 # 总量与各状态分布
ss -tn state established '( dport = :443 or sport = :443 )' | wc -l
ss -tn state time-wait | wc -l
```

结果：`time-wait` 数量异常高，且集中在到上游 API 的 443 端口——大量短连接，没有复用。

顺着查业务配置：健康检查探针 `periodSeconds: 1`、`timeoutSeconds: 1`，节点上 43 个 Pod × 2 个探针 = 每秒 86 次 HTTP 请求，每次都是新连接；加上业务侧 HTTP 客户端没开 keep-alive，QPS 高峰时连接创建速率直接压垮 conntrack。

## 根因

- **直接原因**：conntrack 表满，内核丢包，kubelet 心跳超时，节点被判 NotReady。
- **深层原因**：探针频率过高（1 秒一次）+ 客户端不复用连接，导致连接创建速率远超节点默认容量；而 `nf_conntrack_max` 用的是内核默认值 65536，从未按节点规格调整过。
- **为什么没早发现**：节点监控只采 CPU / 内存 / 磁盘，没有采 `nf_conntrack_count / max`；也没有"节点 NotReady 前兆"这类预警指标。

## 修复与恢复

```bash
# 临时生效（重启失效）
sysctl -w net.netfilter.nf_conntrack_max=262144

# 持久化
echo "net.netfilter.nf_conntrack_max=262144" > /etc/sysctl.d/99-conntrack.conf
sysctl -p /etc/sysctl.d/99-conntrack.conf
```

同时缩短已建立连接的保留时间，加快表项回收：

```bash
sysctl -w net.netfilter.nf_conntrack_tcp_timeout_established=600
```

验证：`nf_conntrack_count` 稳定在 max 的 60% 以下，`dmesg` 不再出现 table full，`kubectl get node` 回到 Ready，节点上 Pod 不再被驱逐。

## 改进项

| 改进 | 类型 | 负责人 | 期限 |
| --- | --- | --- | --- |
| 节点监控新增 `nf_conntrack_count / max` 使用率，超过 70% 预警 | 监控 | 平台 | 三天 |
| 探针频率规范：liveness 不低于 10s、readiness 不低于 5s，除非有明确理由 | 流程 | 平台 | 一周 |
| 应用侧 HTTP 客户端统一开启 keep-alive 与连接池（默认 `MaxIdleConnsPerHost` 不要用 2） | 代码 | 业务 | 两周 |
| 节点规格与 conntrack 上限挂钩，在节点初始化脚本里按内存大小设置 | 配置 | 运维 | 一周 |
| 有状态服务补副本或改成 Operator 管理，避免单副本节点故障即不可用 | 架构 | 业务 | 一个月 |

## 一条值得记住的判断链

节点 NotReady 不一定是"节点死了"。这次的节点业务 Pod 大多还在跑，只是**控制面联系不上**。区分方法：

- 能 SSH 进去 → 节点活着，问题在 kubelet 或网络
- `ping` 丢包但能登录 → 大概率是内核层丢包（conntrack / 软中断 / 网卡队列）
- `dmesg` 里只要有 `dropping packet`，先查 conntrack 和 `netdev_max_backlog`

先确认"控制面为什么联系不上"，再决定是重启 kubelet 还是修网络。直接重启 kubelet 能让节点短暂变 Ready，但 conntrack 还是满的，几十分钟后照样复发。

---

本文按常见生产故障的真实形态整理，用于演示复盘写法；节点名与数字为示例。
