## 状态文件是 Terraform 的命根子

`terraform.tfstate` 记录「我管理哪些资源」。本地状态的团队协作等于盲飞，
**远端状态 + 锁是生产的第一前提**：

```hcl
terraform {
  backend "cos" {
    bucket = "ops-terraform-state"
    prefix = "prod/order-api"
  }
}
```

- 状态桶开版本控制 + 服务端加密，误删可回滚；
- 锁机制防止两人同时 apply（COS 锁 / S3 DynamoDB 锁 / Consul）；
- **state 文件含明文敏感信息**：桶权限收紧到 CI 与平台组。

## 目录结构：环境隔离用目录，不用 workspace

```
infra/
├── modules/vpc/      # 可复用模块
├── prod/
│   ├── main.tf       # module "vpc" { source = "../modules/vpc" }
│   └── backend.tf    # 指向 prod 前缀
└── staging/
    └── backend.tf    # 指向 staging 前缀
```

workspace 共享同一份代码同一套 provider 版本，环境差异容易漏；
**目录隔离让 prod 和 staging 的变更彻底解耦**。

## plan 审批流

```bash
terraform plan -out=prod.tfplan    # 计划落盘，apply 用同一份
terraform apply prod.tfplan
```

CI 流水线：PR 触发 `plan` 并把 diff 贴到评审页，merge 后 `apply` 计划文件。
**自动 apply 只允许在 staging**，prod 永远人工确认。

## 漂移治理

有人在控制台手改资源，Terraform 下次 plan 就会「纠正」回去。两种策略：

1. 严管：IaC 仓库外禁止改资源，控制台只读账号给日常运维；
2. 识别后导入：`terraform import`（1.5+ 有 `import` 块）把手工资源收编进管理。

## 验收清单

- [ ] 状态远端存储 + 锁，桶开版本控制
- [ ] prod/staging 目录隔离，backend 前缀不重叠
- [ ] PR 有 plan diff，apply 有审批记录
- [ ] `terraform state pull` 备份进 CI 产物（每周）
- [ ] 所有资源带标签（owner / env / cost-center），账单能归组