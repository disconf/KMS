## 生命周期命令

```bash
terraform init                 # 初始化/换后端后必跑
terraform validate             # 语法与引用检查
terraform fmt -recursive       # 格式化，PR 前必跑
terraform plan -out=tf.plan    # 预演并落盘
terraform apply tf.plan        # 执行落盘的计划（不会重新计算）
terraform apply -replace=aws_instance.web    # 强制重建某资源
terraform destroy              # 销毁整个 state 里的资源（高危）
terraform destroy -target=module.cache      # 只销毁某模块
```

`-target` 是手术刀：应急用，别养成习惯（部分执行会让 state 与代码偏差）。

## state 操作

```bash
terraform state list                        # 管理中的资源
terraform state show aws_instance.web       # 资源当前属性
terraform state rm aws_instance.web         # 解除管理（资源不动，只移出账本）
terraform state mv module.old module.new    # 重构代码后迁移地址
terraform state pull > backup-$(date +%F).tfstate
```

`import`（1.5+ 声明式）：

```hcl
import {
  to = aws_instance.web
  id = "i-0abc123"
}
```

跑一次 `terraform plan` 生成导入配置，再 review diff。

## 变量与输出

```bash
terraform plan -var 'instance_type=c5.large' -var-file=prod.tfvars
terraform output                    # 输出值（如生成的 IP、密钥）
terraform output -json              # 脚本消费用 JSON
terraform console                   # 交互式求值表达式
```

## 应急场景

```bash
# apply 中断：锁没释放
terraform force-unlock <LOCK_ID>    # 确认没人真在跑再解锁

# plan 报 perpetual diff：手动改过资源
terraform apply -refresh-only      # 只刷新 state 到真实状态

# provider 版本漂移：锁版本
# required_providers 里写 version = "~> 1.81"，别裸用 >=
```

## 日常心法

1. **永远 plan 先行**，apply 执行的一定是看过的那份计划文件；
2. 改动小步提交，一个 PR 一类资源，diff 可读；
3. state 备份常态化：`state pull` 进 CI 产物或对象存储定时归档；
4. 敏感变量标 `sensitive = true`，输出里就不会明文回显（但 state 里仍有，桶权限是底线）。