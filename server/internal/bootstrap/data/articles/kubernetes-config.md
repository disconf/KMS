## 配置文件在哪

| 组件 | 配置 |
|---|---|
| kubelet | `/var/lib/kubelet/config.yaml` + systemd drop-in（`--node-ip` 等） |
| API Server / etcd / controller | 静态 Pod manifests：`/etc/kubernetes/manifests/*.yaml` |
| kubeadm | `/etc/kubernetes/kubeadm-config.yaml`（初始化/升级用） |
| CNI | 各插件自己的 ConfigMap 或 /etc/cni/net.d/ |

K8s 的大部分「配置」不是文件，而是**API 对象**（ConfigMap/Secret/资源 spec）——
先分清「节点级配置」与「集群对象配置」两个世界。

## 参数怎么读

资源对象按 apiVersion/kind/metadata/spec/status 组织，`kubectl explain pod.spec`
能逐级看字段文档，比搜博客可靠——**explain 是内置的权威手册**。

## 核心参数分组

**资源申请与限制（每个负载都要有）**

```yaml
resources:
  requests: { cpu: 250m, memory: 256Mi }   # 调度依据
  limits:   { cpu: "1",  memory: 512Mi }   # 硬顶，内存超了 OOMKill
```

**探针（决定生死与流量）**

```yaml
readinessProbe:   # 没就绪就摘流量，不重启
  httpGet: { path: /healthz, port: 8080 }
livenessProbe:    # 失败就重启容器
  httpGet: { path: /healthz, port: 8080 }
startupProbe:     # 慢启动应用先给宽限期
  failureThreshold: 30
```

**滚动发布**

```yaml
strategy:
  rollingUpdate: { maxSurge: 1, maxUnavailable: 0 }
```

**kubelet 关键项**：`--node-ip`（多网卡必配）、`eviction-hard`（磁盘/内存驱逐线）、
`max-pods`（默认 110）。

## 常见误配

1. **不设 resources**——节点资源被无申请的 Pod 吃光，触发驱逐风暴；
2. **liveness 挂到依赖接口上**——数据库抖动导致全量 Pod 被重启，探针只测自身健康；
3. **镜 tag 用 latest**——发布不可复现，回滚都不知道回哪个；
4. **limits 压得太死**——CPU 被 throttle 表现为「接口莫名变慢」；
5. **只备份应用不备份 etcd**——集群级灾难时应用 YAML 救不了 control plane。

## 校验与生效

```bash
kubectl get events -A --sort-by=.lastTimestamp | tail   # 事件时间线，排障第一站
kubectl describe pod <name>    # 调度失败/探针失败原因都在 Events 里
kubectl top nodes/pods         # 实际用量与 requests 的差距
kubectl explain deployment.spec.strategy
etcdctl snapshot status <file> # 备份有效性
```
