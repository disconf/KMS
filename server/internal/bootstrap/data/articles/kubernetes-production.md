## 高可用控制面的最小形态

三个 master + 外置或堆叠 etcd。堆叠（etcd 跟控制面同机）是 kubeadm 默认，
三个节点起；机器富余用外置 etcd，故障域隔离更干净。

```bash
# 堆叠 etcd 三 master：init 时加 --control-plane-endpoint 指向 LB/VIP
kubeadm init \
  --control-plane-endpoint="k8s-vip.internal:6443" \
  --upload-certs \
  --pod-network-cidr=10.244.0.0/16

# 第二、三个 master 用输出的 join --control-plane 命令加入
```

`k8s-vip` 用 keepalived 或 HAProxy（TCP 6443）承载，**这一层不可省**：
apiserver 地址写进每个组件的 kubeconfig，事后换地址等于重装。

## etcd 运维三件事

```bash
# 1. 健康（在任一 master 上）
ETCDCTL_API=3 etcdctl \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key endpoint health

# 2. 快照（放 cron，每天一次）
etcdctl ... snapshot save /backup/etcd-$(date +%F).db

# 3. 磁盘：etcd 对 fsync 延迟极敏感，必须 SSD，别和别的负载混盘
```

恢复是整集群级别的动作：`kubeadm reset` 所有节点后用 snapshot 恢复 etcd 再重建，
**平时没做过恢复演练的备份等于没有备份**。

## 证书：到期前轮换，别等报错

```bash
kubeadm certs check-expiration          # 看有效期
kubeadm certs renew all                 # 全部续一年（kubeadm 装的默认一年期）
systemctl restart kubelet kube-apiserver kube-controller-manager kube-scheduler
```

一年到期这件事没有告警就会变成事故，把 `check-expiration` 挂进巡检。

## 节点扩容与下线

```bash
# 扩容：新节点装好前置后 join（token 过期就重新生成）
kubeadm token create --print-join-command

# 下线：先驱逐再删除，确认 Pod 都漂走了再关机
kubectl drain node-3 --ignore-daemonsets --delete-emptydir-data
kubectl delete node node-3
```

## 资源与调度基线

1. 所有业务 Pod **必须**写 requests/limits，没有 requests 的 Pod 调度是盲的；
2. 关键组件（CNI、DNS、存储）打 `node-role.kubernetes.io/master` 污点隔离或独占节点；
3. `PodDisruptionBudget` 给有副本数要求的服务兜底，防止 drain 时一次杀光；
4. 集群升级一次只升一个小版本（1.28→1.29→1.30），先升工作节点再升控制面。

## 验收清单

- [ ] 三个 master 全 Ready，VIP 漂移演练通过
- [ ] etcd 快照在 cron 里，且在另一台机器做过恢复演练
- [ ] 证书有效期 > 90 天，巡检覆盖
- [ ] 任一 master 关机，业务发布与解析不中断
