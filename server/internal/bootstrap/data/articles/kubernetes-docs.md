## 文档入口

| 找什么 | 去哪 |
|---|---|
| 概念 / 任务 / 参考三大区 | <https://kubernetes.io/zh-cn/docs/home/> |
| API 对象字段权威手册 | `kubectl explain`（离线、永远与集群版本一致） |
| API 参考（在线版） | <https://kubernetes.io/docs/reference/kubernetes-api/> |
| 版本发布与升级 | <https://kubernetes.io/releases/> |

## 必读页面清单

1. **Concepts → Workloads → Pods/Deployments**——所有日常操作的语义基础；
2. **Concepts → Services, Load Balancing, and Networking**——Service 类型与
   kube-proxy 转发模式，排网络问题的根基；
3. **Tasks → Configure Pods and Containers**——资源、探针、ConfigMap/Secret 的实操入口；
4. **Set up → High Availability**（kubeadm 建高可用控制面的官方路径）；
5. **kubectl 备忘单**（Cheat Sheet）——高频命令组合，打印一份贴墙上。

## 推荐学习路径

1. **第 1 天**：minikube/kind 起本地集群 → 部署一个 Deployment + Service →
   `kubectl get/describe/logs/exec` 四件套用熟；
2. **第 1 周**：把探针、resources、滚动策略配齐；故意制造 OOM/探针失败/镜像拉取失败，
   练「describe + events」定位肌肉记忆；
3. **第 1 个月**：kubeadm 搭多节点集群 → 一次版本升级演练 → etcd 备份恢复演练；
   然后学 Ingress、HPA 与 Helm，按业务需要逐个引入。

## 版本选择与升级文档

- [发布节奏页](https://kubernetes.io/releases/)列出每版的 EOL 日期，维护窗口按它排；
- 升级文档：Tasks → Administration → Upgrading（kubeadm 升级是逐节点、
  一次一个小版本，跳版不支持）；
- 升级顺序：控制面先于节点，kubelet 版本永远 ≤ API Server。

## 社区与求助渠道

- Slack（#kubernetes 等频道）与 GitHub：<https://github.com/kubernetes/kubernetes>；
- 提问带上：集群版本、`kubectl version`、`kubectl describe` 的 Events 段、
  相关 YAML 与 `kubectl get events` 时间线；
- Discourse 论坛 <https://discuss.kubernetes.io/> 对新手问题更友好。
