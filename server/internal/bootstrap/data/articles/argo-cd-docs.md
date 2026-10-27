## 文档入口

| 找什么 | 去哪 |
|---|---|
| 文档总入口 | <https://argo-cd.readthedocs.io/en/stable/> |
| Operator Manual（运维向） | <https://argo-cd.readthedocs.io/en/stable/operator-manual/> |
| User Guide（应用同步） | <https://argo-cd.readthedocs.io/en/stable/user-guide/> |
| 升级与兼容 | ReadTheDocs 左侧「Upgrade」页 |

文档按 stable/latest 两个通道发布，引用时**固定 stable 链接**避免内容漂移。

## 必读页面清单

1. **Best Practices**——仓库结构、凭证管理、自动同步开不开，官方立场都在这；
2. **Declarative Setup**——Argo CD 自身配置即对象，用 kubectl/Helm 管理；
3. **Sync Options & Sync Waves**——`argocd.argoproj.io/sync-wave` 控制资源先后
   （先建库再起应用），实战高频；
4. **RBAC**（operator-manual/rbac）——policy.csv 语法与内置角色；
5. **ApplicationSet**——多集群多环境的模板化生成。

## 推荐学习路径

1. **第 1 天**：装好 → port-forward 进 UI → 用官方 guestbook 仓库建第一个 app，
   手动 sync 一次，看 diff 与资源树；
2. **第 1 周**：把一个真实服务的清单进 Git，开自动同步（先不开 prune）；
   故意在集群里 scale 一次，观察 OutOfSync——理解「Git 是唯一事实」的含义；
3. **第 1 个月**：拆 repo 结构（per-env 目录或 overlay）、配 RBAC 与 SSO、
   上 ApplicationSet 管多环境；演练一次「revert 提交 = 回滚发布」。

## 版本选择与升级文档

- 升级页：ReadTheDocs「Upgrade」逐大版本说明（2.8→2.9→…不可跳级）；
- 升级顺序：升级 CRD → 升组件 manifests → 检查 `argocd-cm` 的废弃项；
- UI/API 兼容：argocd CLI 版本要与服务端大版本匹配，CLI 随服务端一起升。

## 社区与求助渠道

- GitHub Discussions：<https://github.com/argoproj/argo-cd/discussions>；
- CNCF Slack #argo-cd 频道；
- 提问带上：服务端与 CLI 版本、`argocd app get` 输出、repo-server 日志片段；
- 「同步失败」类问题先跑 `argocd app diff` 与 `argocd app manifests <app>`，
  十有八九是渲染（Helm/Kustomize）层的问题而不是同步层。
