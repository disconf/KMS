## 同步策略：三个开关定生死

```yaml
syncPolicy:
  automated:
    prune: true        # Git 删了资源，集群跟着删——不开会攒孤儿，开了要防误删
    selfHeal: true     # 有人 kubectl edit 会被拉回 Git 状态——GitOps 的前提
  syncOptions:
    - CreateNamespace=true
    - ApplyOutOfSyncOnly=true
  retry:
    limit: 3
    backoff: { duration: 5s, factor: 2 }
```

排障期间临时改参数会被 selfHeal 覆盖，要提前和团队约定「紧急变更也走 Git」，
否则 GitOps 会在最关键的深夜失效。

## 权限与 SSO

```yaml
# argocd-rbac-cm：按团队拆权限，admin 只留给平台组
policy.csv: |
  p, role:dev, applications, sync, prod/*, allow
  p, role:dev, applications, get, prod/*, allow
  g, team-dev, role:dev
  p, role:sre, *, *, */*, allow
```

对接 OIDC（企业微信 / 飞书 / LDAP）后禁用本地 admin：
`admin.enabled: "false"`。**RBAC 按「项目 + 环境」划分**，别按人授权。

## 仓库凭证与多租户

- 凭证放 `argocd-repo-creds`（按 URL 前缀匹配），细粒度用 Project 限定
  `sourceRepos` / `destinations`；
- 私有仓库用部署密钥（只读）而不是个人 token，避免人员离职断链；
- ApplicationSet 按目录 / Git 生成应用，几百个服务不用手建 App。

## 灾备：Argo CD 本身丢了怎么办

Argo CD 的状态源是 Git + 目标集群，**重建实例本身不丢业务**。
需要备份的只有：`argocd-cm`（SSO/RBAC 配置）、仓库凭证、Project 定义。
把这三样 Helm values 化进 Git，整个控制面就是可重建的。

## 验收清单

- [ ] admin 直连关闭，SSO 登录跑通
- [ ] prune / selfHeal 的语义向全员宣讲过（尤其 prune 的删除风险）
- [ ] 镜像 tag 全部不可变（commit sha），回滚才可靠
- [ ] RBAC 策略进 Git，变更走 PR
- [ ] 控制面重建演练：新集群上用 Git 里的 values 十分钟拉起 Argo CD 并恢复应用同步