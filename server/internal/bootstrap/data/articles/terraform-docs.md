## 文档入口

| 找什么 | 去哪 |
|---|---|
| 官方文档 | <https://developer.hashicorp.com/terraform/docs> |
| HCL 语言参考 | <https://developer.hashicorp.com/terraform/language> |
| Registry（provider/module） | <https://registry.terraform.io/> |
| CLI 命令参考 | <https://developer.hashicorp.com/terraform/cli> |

**Provider 文档在 Registry 各自的页面里**——aws/alicloud 每个资源的参数说明都在那，
总文档只讲语言与工作流，别在总文档里找某个云资源的参数。

## 必读页面清单

1. **Language → Resources / Variables / Outputs**——HCL 语法三件套；
2. **State**（remote state、locking、import）——state 是 Terraform 的命门；
3. **Modules**（create/use/standard structure）——复用与团队协作的单元；
4. **CLI → import / state 子命令**——把存量资源纳管的唯一途径；
5. **Registry 里所用 provider 的版本说明**——每个 provider 大版本都可能有 breaking。

## 推荐学习路径

1. **第 1 天**：装 tfenv → 在测试账号建一个 VPC+虚机 → plan/apply/destroy
   完整走一遍，盯着 plan 输出读懂每行 +/~/-；
2. **第 1 周**：上远端 state（OSS/S3 + 锁）；把一套现有资源 `import` 进来；
   抽第一个 module（如「标准 VPC」）并复用到两个环境；
3. **第 1 个月**：目录结构按环境分层；CI 里接 tflint + plan 审批；
   演练一次「代码在、账号空」的重建，检验灾备叙事是否成立。

## 版本选择与升级文档

- Terraform 核心升级看 [release notes](https://github.com/hashicorp/terraform/releases)；
  1.x 语义稳定，主要风险在 provider；
- **provider 升级纪律**：锁 minor，升级先跑 plan 看意外 diff；
- 许可证变化（BUSL）与 OpenTofu 迁移：state 兼容，评估迁移前备份 state。

## 社区与求助渠道

- HashiCorp Discuss：<https://discuss.hashicorp.com/c/terraform-core/>；
- GitHub：<https://github.com/hashicorp/terraform/issues>；
- 提问带上：版本、provider 版本、plan 输出、相关 .tf 片段（脱敏）；
- 「plan 出来一堆想不通的变更」先查 provider changelog——多数是 provider 升级
  引入的默认值变化，不是你写错了。
