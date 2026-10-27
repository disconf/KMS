## 它是什么

Git 是**分布式版本控制系统**：每个开发者手里都是完整仓库（含全部历史），
提交在本地完成、离线可用，仓库间靠 push/pull 同步。Linus 为管 Linux 内核写的，
如今是一切代码协作的底座。

## 解决什么问题

- **历史可追溯**：每次提交是一个不可变快照，谁改的、为什么改、改回哪一版都查得到；
- **并行协作**：分支让多人在不同方向同时工作，合并机制负责汇流；
- **分发与备份**：任何一份 clone 都是全量备份，GitHub/GitLab 只是「协作中心」这一角色。

## 架构与核心概念

| 概念 | 一句话 |
|---|---|
| 对象库（.git/objects） | blob（文件）/tree（目录）/commit（快照）/tag，内容寻址（SHA） |
| 暂存区（index） | 下次提交的内容清单，add 就是往里写 |
| 分支 / HEAD | 分支只是指向 commit 的可变指针，HEAD 指当前所在位置 |
| 远程（remote） | 别的仓库别名，origin 是惯例 |
| reflog | HEAD 的移动日志，**所有「丢失」的提交都能在这找回** |

理解「分支是指针、提交是不可变快照」，80% 的恐惧（rebase/reset）就消失了。

## 版本与生态

- Git 本身 2.x 主线，语义稳定多年；
- 托管平台：GitHub / GitLab / Gitee；自建首选 Gitea（轻）或 GitLab（全家桶）；
- 周边：pre-commit（钩子框架）、Git LFS（大文件）、gh/gitlab-cli（命令行协作）。

## 适用与不适用

**适合**：一切文本资产的版本管理——代码、配置、文档、IaC、教程。
**不适合**：二进制大文件频繁变更（用 LFS 或对象存储）、把 Git 当数据库做查询
（它不是搜索引擎）。

## 官方文档导读

- 官方书（中文版免费）：<https://git-scm.com/book/zh/v2>——唯一必读；
- 命令参考：<https://git-scm.com/docs>；
- GitHub 的交互式学习分支：<https://learngitbranching.js.org/?locale=zh_CN>
