## 应用同步与回滚

```bash
argocd app list
argocd app get demo                      # 状态、同步版本、健康详情
argocd app sync demo                     # 手动同步
argocd app sync demo --revision <sha>    # 同步到指定 commit
argocd app diff demo                     # Git 期望 vs 集群实际
argocd app history demo                  # 部署历史
argocd app rollback demo 3               # 回到历史 ID=3 的版本
argocd app terminate-op demo             # 终止卡住的同步操作
```

自动同步开启时 rollback 会被自动同步顶回去——**回滚要走 Git revert**，
`argocd app rollback` 只在自动同步关闭时有效。

## 资源排查

```bash
argocd app resources demo                # 应用管到的所有 k8s 资源
argocd app logs demo --kind Pod --name web-7d9c   # 不用切 kubectl 也能看日志
argocd app manifests demo | less         # 渲染后的最终 manifest
argocd app events demo                   # 应用级事件流
```

## 仓库与集群管理

```bash
argocd repo list
argocd repo add git@internal:deploy.git --ssh-private-key-path ~/.ssh/deploy_key
argocd cluster list
argocd project list && argocd proj get default
```

## 管理 Argo CD 自身

```bash
kubectl get pods -n argocd
kubectl logs -n argocd deploy/argocd-application-controller --tail=100
argocd admin settings validate           # 配置校验
argocd admin proj update-role ...        # 项目权限维护
```

## 「同步卡在 Progressing」排查顺序

1. `argocd app get` 看 conditions：多半是健康检查没过（探针 / Job 未完成）；
2. `argocd app resources` 找到不健康的资源，再 `kubectl describe` 看真实原因；
3. Hook 任务（如 DB 迁移 Job）失败会卡住整个同步，看 Job 日志；
4. `argocd app diff` 确认是不是 live state 被 selfHeal 反复拉扯（人工改过的字段）；
5. 都不是 → 控制器日志：`kubectl logs deploy/argocd-application-controller`
   搜应用名，注意 `comparison error`（常见是仓库凭证失效）。