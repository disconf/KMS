## 日常提交

```bash
git status && git diff          # 提交前看清楚改了什么
git add -p                      # 分块暂存，只提交该提交的
git commit -m "fix: 排除已下线节点的健康检查误报"
git commit --amend              # 补充上一个提交（已 push 的别 amend）
git log --oneline --graph -20   # 分支图
git show <sha> --stat           # 某次提交改了哪些文件
```

## 回退与撤销（背熟这张表）

| 场景 | 命令 |
|---|---|
| 撤销工作区修改 | `git restore <file>` |
| 撤销已暂存 | `git restore --staged <file>` |
| 撤销最后一次提交（保留改动） | `git reset --soft HEAD~1` |
| 彻底丢弃最后 N 次提交 | `git reset --hard HEAD~N`（危险） |
| 已推送的错误提交 | `git revert <sha>`（生成反向提交，别 force push） |
| 挑一个提交到当前分支 | `git cherry-pick <sha>` |

## 分支与变基

```bash
git switch -c feature/x origin/main     # 建分支
git switch main
git merge feature/x                     # 合并（保留合并节点）
git rebase main                         # feature 上追平 main，历史线性
git rebase -i HEAD~5                    # 交互式整理提交
git push origin feature/x --force-with-lease   # rebase 后推送（比 -f 安全）
```

**团队共享分支永远 merge，不要 rebase**；只有自己独占的 feature 分支才 rebase。

## 远端与暂存

```bash
git remote -v
git fetch --prune                       # 拉取并清理已删远端分支
git stash push -m "wip: 改一半"         # 收纳工作区
git stash list && git stash pop
git worktree add ../hotfix main         # 多目录并行多分支，免 stash
```

## 标签与发布

```bash
git tag -a v1.4.0 -m "release 1.4.0"
git push origin v1.4.0
git tag -d v1.4.0 && git push origin :refs/tags/v1.4.0   # 删标签
```

## 找是谁改的

```bash
git blame -L 10,20 file.py              # 逐行归属
git log -p --follow file.py             # 文件完整历史（含改名）
git log --grep='健康检查' --oneline      # 按提交信息搜
git bisect start; git bisect bad; git bisect good v1.3.0   # 二分定位引入 bug 的提交
```