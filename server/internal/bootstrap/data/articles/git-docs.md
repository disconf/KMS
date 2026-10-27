## 文档入口

| 找什么 | 去哪 |
|---|---|
| Pro Git 全书（中文） | <https://git-scm.com/book/zh/v2> |
| 命令手册 | <https://git-scm.com/docs>（每条命令一页） |
| 可视化练分支 | <https://learngitbranching.js.org/?locale=zh_CN> |
| 托管平台文档 | GitHub Docs / GitLab Docs（工作流类问题在这查） |

## 必读页面清单

1. **Pro Git 第 2、3 章**（基础与分支）——对象模型与分支指针，读懂后命令不用背；
2. **Pro Git 第 7 章**（Git 工具）——stash、bisect、worktree、submodule、refspec；
3. **git-rebase / git-reset 手册页**——「改历史三件套」的精确语义；
4. **git-svn / large-file**（按需）——迁移与大文件场景。

## 推荐学习路径

1. **第 1 天**：init/add/commit/log 四件套 → 在 learngitbranching 上把 main 系列关卡过了；
2. **第 1 周**：分支工作流（feature → PR/MR → merge）跑通；练熟
   `stash`、`cherry-pick`、`revert`；制造一次冲突并手工解决；
3. **第 1 个月**：理解 rebase 与 merge 的历史差异并选定团队约定；
   用 bisect 定位一次引入 bug 的提交；把 alias 配齐形成肌肉记忆。

## 版本选择与升级文档

- Git 服务端/客户端语义多年稳定，升级一般无风险，看 [release notes](https://github.com/git/git/releases)；
- 平台侧（GitHub/GitLab）升级另有专门升级文档与版本路线，生产 GitLab 升级
  必须按官方 upgrade path 逐版本走，不可跳级；
- 大文件场景引入 LFS 前先评估仓库体量——半路引入需要改写历史。

## 社区与求助渠道

- Stack Overflow 的 git 标签（历史问题覆盖极全，先搜）；
- Pro Git 书末的「附录：命令速查」；
- 提问带上：`git version`、出错的完整命令与输出、`git status` 与 `git log --oneline -5`；
- 「我提交丢了」的答案永远是 reflog——先跑 `git reflog` 再问人。
