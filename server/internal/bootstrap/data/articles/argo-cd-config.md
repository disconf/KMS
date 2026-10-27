## 配置文件在哪

| 内容 | 位置 |
|---|---|
| 安装清单 | `argocd-install.yaml`（官方 manifests 或 Helm values） |
| 核心组件参数 | 安装清单里各 Deployment 的 args/env（repo-server、controller） |
| 密码/SSO | `argocd-secret` + `argocd-cm` ConfigMap |
| RBAC | `argocd-rbac-cm`（policy.csv） |
| 仓库凭证 | `argocd-repo-*` Secret（或声明式 repo Secret） |

Argo CD 自身的配置就是一组 K8s 对象——改它用的还是 kubectl/Helm，别去容器里改文件。

## 参数怎么读

核心参数都在组件启动 args 里。改完 rollout 重启对应 Deployment 即可；
`argocd-cm` 里的配置改完要等 controller 热加载或手动重启。

## 核心参数分组

**Application 关键字段（spec）**

```yaml
syncPolicy:
  automated: { prune: true, selfHeal: true }   # 自动同步；两个开关都要想清楚
  syncOptions: [CreateNamespace=true]
  retry: { limit: 5, backoff: { duration: 5s } }
source:
  targetRevision: main          # 永远显式写分支/tag，别依赖默认 HEAD
```

**性能调优（repo-server / controller args）**

```yaml
--status-processors: "20"
--operation-processors: "10"
--repo-server--parallelismlimit: "10"    # 大量 Helm 渲染时调
```

**RBAC（policy.csv）**

```csv
p, role:dev, applications, sync, dev-project/*, allow
p, role:dev, applications, delete, dev-project/*, deny
g, dev-team, role:dev
```

## 常见误配

1. **prune 无脑开**——有人把资源从 Git 删了想「先在集群里留着」，下一轮 sync 直接清掉；
   上自动同步前先想好误删恢复路径；
2. **selfHeal 与手工紧急变更打架**——线上救火 kubectl 改了，Argo 马上改回去；
   紧急通道要么走 Git hotfix，要么临时关 app 的自动同步；
3. **Health 当业务可用性**——Pod Running 不等于业务正常，Health 是资源级判定；
4. **大仓库单 Application**——path 拆细、按服务分 app，渲染与同步才快；
5. **凭证明文进 Git**——用 Secret 管理 + SealedSecrets/Vault 一类方案。

## 校验与生效

```bash
argocd app list
argocd app diff <app>        # Git 与集群的差异（同步前必看）
argocd app get <app>         # sync/health 状态与最近事件
kubectl logs deploy/argocd-repo-server -n argocd | tail   # 渲染报错在这
```
