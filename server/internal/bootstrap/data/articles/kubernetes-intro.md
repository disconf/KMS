## 它是什么

Kubernetes（K8s）是**容器编排系统**：把一群机器抽象成资源池，声明式地调度容器、
自愈故障、弹性伸缩、滚动发布。Docker 解决「单个容器怎么跑」，K8s 解决
「几十上百个服务怎么协同地跑」。

## 解决什么问题

- **自愈**：Pod 挂了自动重建，节点宕机负载自动迁走；
- **声明式发布**：你描述「期望状态」（几个副本、什么镜像），控制器持续对齐现实；
- **服务发现与负载均衡**：Service/Ingress 把「IP 会变」这件事从运维问题变成平台能力；
- **伸缩**：HPA 按 CPU/自定义指标自动扩缩副本。

## 架构与核心概念

| 概念 | 一句话 |
|---|---|
| API Server | 唯一入口，一切读写都过它（kubectl 就是它的客户端） |
| etcd | 集群的唯一事实来源，备份它 = 备份整个集群 |
| Scheduler / kubelet | 调度器选节点，kubelet 在节点上真拉起容器 |
| Pod | 最小调度单位，一个或多个共享网络的容器 |
| Deployment / StatefulSet | 无状态副本 vs 有身份（稳定域名+存储）的工作负载 |
| Service / Ingress | 四层稳定访问入口 / 七层路由 |

## 版本与生态

- 一年三个小版本，每版维护约 14 个月——**升级是常态运维**，一年至少一次；
- CNI（Calico/Cilium）、Ingress（nginx-ingress）、CSI（存储）等生态组件独立发版；
- 发行版选择：kubeadm 自建 / K3s（边缘轻量）/ 云托管（EKS/TKE/AKS）。

## 适用与不适用

**适合**：多服务、多环境、需要滚动发布与自愈的中大型系统。
**不适合**：两三台机器跑几个单体服务——compose 更省心；强状态、强宿主机绑定的传统软件
（如某些老中间件）上 K8s 收益低风险高。

## 官方文档导读

- 概念文档（Concepts）：<https://kubernetes.io/zh-cn/docs/concepts/>
- 任务手册（Tasks）：<https://kubernetes.io/zh-cn/docs/tasks/>
- kubectl 命令参考：<https://kubernetes.io/zh-cn/docs/reference/kubectl/>
