## 四条路径，先选对再动手

| 方式 | 适用场景 | 代价 |
|---|---|---|
| kubeadm | 标准自建集群，生产首选 | 自己管 etcd / 证书 / 升级 |
| K3s | 边缘节点、小规模、资源紧张 | 功能裁剪，etcd 换 SQLite（可切） |
| 二进制手搓 | 深度定制 / 教学理解原理 | 升级是手工活，不推荐生产 |
| 云托管（ACK/EKS/TKE） | 不想管控制面 | 控制面免运维，节点还是要管 |

## kubeadm 标准安装（v1.29，容器运行时用 containerd）

所有节点的前置（控制面 + 工作节点）：

```bash
# 关 swap（kubelet 强制要求）
swapoff -a && sed -ri 's/.*swap.*/#&/' /etc/fstab

# 内核模块与网络参数
cat > /etc/modules-load.d/k8s.conf <<EOF
overlay
br_netfilter
EOF
modprobe overlay && modprobe br_netfilter

cat > /etc/sysctl.d/k8s.conf <<EOF
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF
sysctl --system

# 安装 kubeadm / kubelet / kubectl（阿里云镜像源示例）
cat > /etc/yum.repos.d/kubernetes.repo <<EOF
[kubernetes]
baseurl=https://mirrors.aliyun.com/kubernetes-new/core/stable/v1.29/rpm/
enabled=1
gpgcheck=1
gpgkey=https://mirrors.aliyun.com/kubernetes-new/core/stable/v1.29/rpm/repodata/repomd.xml.key
EOF
yum install -y kubelet kubeadm kubectl
systemctl enable --now kubelet
```

初始化控制面（只在 master 执行）：

```bash
kubeadm init \
  --apiserver-advertise-address=10.0.0.11 \
  --image-repository registry.aliyuncs.com/google_containers \
  --pod-network-cidr=10.244.0.0/16 \
  --service-cidr=10.96.0.0/12

# 按输出提示配置 kubeconfig
mkdir -p $HOME/.kube && cp /etc/kubernetes/admin.conf $HOME/.kube/config
```

工作节点用 init 输出的 `kubeadm join ...` 加入。**join 命令 24 小时过期**，
过期后用 `kubeadm token create --print-join-command` 重新生成。

## 装 CNI（不装集群是 NotReady）

```bash
# Flannel（简单、够用）
kubectl apply -f https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml
# Calico（需要 NetworkPolicy 时选它）
kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.28/manifests/calico.yaml
```

## 验证

```bash
kubectl get nodes -o wide        # 全部 Ready 才算完
kubectl get pods -n kube-system  # 核心 Pod 全 Running
kubectl run nettest --rm -it --image=busybox -- ping 10.244.0.2
```

## 常见翻车点

1. **节点 NotReady 且 kubelet 起不来**：九成是 swap 没关或 containerd 没配对；
2. **Pod 起不来报 sandbox 镜像拉取失败**：国内环境给 containerd 配 pause 镜像镜像源；
3. **join 失败提示端口不通**：6443 / 2379-2380 / 10250，先查防火墙再查证书。