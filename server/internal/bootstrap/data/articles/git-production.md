## 分支模型：简单可执行优先

团队常用的是简化版 Git Flow：

```
main        生产分支，受保护，只接受 PR/MR
release/x.y 发布分支，小修复进这里
feature/*   功能分支，从 main 拉出
hotfix/*    紧急修复，从 main 拉出，修完合回 main 和 release
```

规则少但必须硬：**main 禁止直推**、PR 必须有 review、CI 红了不能合。
分支策略不为时髦服务，为「出问题能快速定位该 revert 哪个提交」服务。

## 钩子与 CI 门禁

服务端钩子（Gitea/GitLab 支持）：`pre-receive` 里做大文件、提交信息、分支保护检查：

```bash
# hooks/pre-receive：拦截超 50M 的文件
while read old new ref; do
  if [ $(git cat-file -s $new) -gt 52428800 ]; then
    echo "reject: file too large in $ref" && exit 1
  fi
done
```

客户端钩子（`pre-commit`）装 lint/格式化，配合 CI 的 `pre-commit run --all-files`
保证钩子不是摆设。

## 大文件与历史瘦身

```bash
# 找历史里最大的对象
git rev-list --objects --all | git cat-file --batch-check='%(objecttype) %(objectname) %(objectsize) %(rest)' \
  | awk '$1=="blob"' | sort -k3 -rn | head

# 从历史中彻底移除误提交的大文件/密钥（全团队要重新 clone）
git filter-repo --path data/dump.sql --invert-paths
```

误提交的密钥不只是删掉就行——**先轮换密钥**，再清历史。二进制产物用 Git LFS，
构建产物永远不进 Git。

## 服务端备份

```bash
# 裸仓库的备份就是整个目录，每日 rsync/cp 即可
rsync -a /srv/git/ /backup/git/
# 或用 git 原生
git clone --mirror /srv/git/ops-scripts.git /backup/mirrors/ops-scripts.git
```

镜像 clone 恢复时 `git push --mirror` 回去，账号权限需要在平台上重建。

## 验收清单

- [ ] main 分支保护 + PR review 流程真实生效
- [ ] pre-commit 与 CI lint 门禁都在跑
- [ ] 仓库无 >50M 文件，二进制走 LFS 或制品库
- [ ] 服务端每日备份且恢复演练过
- [ ] 提交信息有规范（约定式提交或同等标准），能按提交生成 changelog