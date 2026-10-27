## 它是什么

Argo CD 是**面向 Kubernetes 的声明式 GitOps 交付工具**：Git 仓库里存着集群的期望状态
（YAML/Helm/Kustomize），Argo CD 持续比对「Git 里的期望」与「集群里的现实」，漂移了就告警
（或自动拉回）。CNCF 毕业项目。

## 解决什么问题

kubectl apply 面临的经典问题：谁在什么时候改了什么？改回滚到哪一版？多集群怎么保证一致？
GitOps 把答案交给 Git——**提交即发布，回滚 = revert，审计 = git log**，
集群里谁手改了东西会被立刻发现。

## 架构与核心概念

| 概念 | 一句话 |
|---|---|
| Application | 一个应用 = Git 源（repo+path）+ 目标集群+ 命名空间 |
| AppProject | 应用的分组与边界（允许哪些 repo/集群/资源类型） |
| Sync / Prune / selfHeal | 同步到 Git 状态 / 删除 Git 里没有的资源 / 抵抗手工修改 |
| Health | Argo 对资源健康度的判定（Progressing/Healthy/Degraded） |
| ApplicationSet | 批量生成 Application（多集群多环境的模板化） |

## 版本与生态

- 2.x 主线；UI/API 兼容性在大版本间有破坏，升级看 breaking notes；
- 同类对比：Flux CD（更轻、组件化）；Argo 全家桶（Workflow/Rollouts/Events）可组合，
  Argo Rollouts 做金丝雀发布是常见搭配。

## 适用与不适用

**适合**：K8s 应用的多环境多集群发布、需要审计与回滚纪律的团队、配置即代码的落地。
**不适合**：非 K8s 目标（虚机/VM 部署要别的方式）、频繁交互式发布（GitOps 的仪式感反而慢）。

## 官方文档导读

- 总入口：<https://argo-cd.readthedocs.io/en/stable/>
- 声明式配置（Argo CD 自己也要 GitOps）：<https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/>
- 最佳实践（必读）：<https://argo-cd.readthedocs.io/en/stable/user-guide/best-practices/>
