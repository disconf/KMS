## 配置文件在哪

| 层级 | 文件 | 范围 |
|---|---|---|
| 系统 | `/etc/gitconfig` | 全机器 |
| 全局 | `~/.gitconfig` | 当前用户（99% 的配置写这） |
| 仓库 | `.git/config` | 单仓库（remote 等） |

`git config --list --show-origin` 能看到每项来自哪个文件；优先级仓库 > 全局 > 系统。

## 参数怎么读

git config 是 key-value，分节（user/core/remote/alias…）。多数「配置问题」
其实是身份（user.email）或协议（ssh vs https）没配对。

## 核心参数分组

**身份（必配的前两行）**

```bash
git config --global user.name  "运维组"
git config --global user.email "ops@example.com"
git config --global init.defaultBranch main
```

**行为（core/push/pull）**

```bash
git config --global core.editor vim
git config --global core.autocrlf input      # 跨平台换行统一
git config --global pull.rebase true          # pull 默认变基，历史线性
git config --global push.default current      # 只推当前分支
git config --global fetch.prune true          # fetch 顺带清理远端已删分支
```

**别名与 diff 增强**

```bash
git config --global alias.lg "log --oneline --graph --decorate -20"
git config --global alias.st "status -sb"
git config --global diff.mnemonicPrefix true
```

**凭据与代理**

```bash
git config --global credential.helper store        # 或 manager/cache
git config --global http.proxy http://127.0.0.1:7890   # 按需
```

## 常见误配

1. **user.email 不对**——贡献统计挂到别人头上，入职第一天核对；
2. **CRLF 折腾**——Windows/容器协作统一 autocrlf 策略，别每次改文件全量 diff；
3. **`git push -f` 到共享分支**——团队分支禁 force push（服务端保护），个人分支随意；
4. **reset --hard 前没看 stash**——未提交工作直接没了，先 `git stash` 兜底；
5. **大小写敏感踩坑**——`core.ignorecase` 在 Windows 上与 Linux 服务端不一致导致路径怪象。

## 校验与生效

```bash
git config --list --show-origin | grep 关键字
git status / git diff --stat      # 变更总览
git log --graph --oneline -10     # 历史形状
git reflog                        # 找回任何「丢失」的提交
```
