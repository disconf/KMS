## 它是什么

Terraform 是 HashiCorp 出的**基础设施即代码（IaC）工具**：用 HCL 语言声明想要的
云资源（虚机、网络、DNS、数据库），`apply` 时算出与现状的 diff 并执行变更。
状态存在 state 文件里，它是「现实与代码的桥梁」。

## 解决什么问题

控制台点选式建资源的问题：不可复制、不可评审、没人说得清现在到底有什么。
Terraform 让基础设施变更走代码流程——**plan 是变更预览（评审点），
apply 是执行，代码是唯一事实**，环境间复制就是换个变量跑一遍。

## 架构与核心概念

| 概念 | 一句话 |
|---|---|
| HCL | 声明式配置语言，资源块（resource）+ 变量 + 输出 |
| Provider | 各云的适配插件（aws/alibaba/azurerm…），按需下载 |
| State | 已管资源的记录，本地文件或远端（S3+DynamoDB 锁 / OSS） |
| Plan / Apply | 预览 diff → 执行变更 |
| Module | 可复用的资源包，像函数一样带输入输出 |
| Workspace / 目录 | 多环境隔离的两种主流组织法 |

## 版本与生态

- **1.6 起许可变更**：HashiCorp 改 BUSL，社区分支 **OpenTofu**（Linux 基金会，
  MPL，兼容 HCL/state）——合规敏感团队选型时注意；
- Registry：<https://registry.terraform.io/> 找 provider 与 module；
- 周边：tfenv（版本管理）、tflint（静态检查）、 Atlantis（PR 自动 plan/apply）。

## 适用与不适用

**适合**：云资源的创建与变更、多环境一致性复制、灾备重建（代码在，环境就能重来）。
**不适合**：应用层发布（那是 CI/CD 的活）、高频 K8s 对象管理（用 Argo/Flux）；
k8s_manifests 类资源只适合低频平台级变更。

## 官方文档导读

- 总入口：<https://developer.hashicorp.com/terraform/docs>
- HCL 语言文档：<https://developer.hashicorp.com/terraform/language>
- CLI 参考：<https://developer.hashicorp.com/terraform/cli>
