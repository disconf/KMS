## 配置文件在哪

| 文件 | 作用 |
|---|---|
| `main.tf` / `*.tf` | 资源与逻辑（按模块拆文件） |
| `backend "s3" {}` | state 存储与锁（写在 terraform 块） |
| `terraform.tfvars` | 变量赋值（按环境放 `envs/prod/`） |
| `~/.terraformrc` | CLI 配置（镜像源、凭据 helper） |
| `.terraform/` | provider 插件缓存（自动生成，不进 Git） |

约定优于配置：provider 版本、变量定义、backend 各就各位，团队才能维护同一套代码。

## 参数怎么读

HCL 是声明式的——你写「要什么」，执行顺序 Terraform 自己算（依赖图）。
显式依赖用 `depends_on`，引用赋值（`a.id`）自动构成隐式依赖。

## 核心参数分组

**版本与 provider 锁定（每个项目第一块）**

```hcl
terraform {
  required_version = "~> 1.9"
  required_providers {
    alicloud = { source = "aliyun/alicloud", version = "~> 1.235" }
  }
  backend "oss" { bucket = "tf-state-prod" prefix = "infra" }
}
```

**变量与输出**

```hcl
variable "instance_type" { type = string, default = "ecs.c7.xlarge" }
variable "env" { type = string, validation { condition = contains(["staging","prod"], var.env) } }
output "eip" { value = alicloud_eip.main.ip }
```

**资源与生命周期**

```hcl
resource "alicloud_instance" "web" {
  count = var.env == "prod" ? 3 : 1
  lifecycle {
    create_before_destroy = true        # 替换型变更先建后删
    prevent_destroy       = true        # 关键资源防误删
    ignore_changes        = [tags.LastModified]
  }
}
```

## 常见误配

1. **state 放本地**——没人能协作、丢失即失忆，第一周就该上远端 state + 锁；
2. **state 进 Git**——里面有明文凭据与敏感字段，进对象存储加密桶；
3. **不跑 plan 直接 apply**——plan 是唯一的安全网，CI 里把 plan 输出贴进 PR；
4. **手动改云资源**——下次 apply 全部被拉回（或标 drift），改资源只走代码；
5. **provider 版本不锁**——一次自动升级让所有资源 plan 出意外 diff。

## 校验与生效

```bash
terraform fmt -recursive     # 格式统一
terraform validate           # 语法与引用校验
terraform plan -out=tfplan   # 预览并固化 plan，apply 用同一份
terraform state list         # 现在管着哪些资源
tflint                       # 更深的静态检查
```
