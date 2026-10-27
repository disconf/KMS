## 文档入口

| 找什么 | 去哪 |
|---|---|
| 文档总入口 | <https://www.jenkins.io/doc/> |
| Pipeline Syntax（语法权威） | <https://www.jenkins.io/doc/book/pipeline/syntax/> |
| Pipeline Steps Reference | <https://www.jenkins.io/doc/pipeline/steps/>（装了什么插件就能查到什么 step） |
| 插件中心 | <https://plugins.jenkins.io/> |

## 必读页面清单

1. **Pipeline Syntax**——declarative 与 scripted 两种语法的边界；
   新手统一用 declarative，需要灵活时在 step 里嵌 script 块；
2. **Managing Jenkins**——备份、升级、监控的官方姿势；
3. **Distributed builds**——agent 的接入方式（JNLP/SSH）与 label 匹配；
4. **Credentials**——凭据类型（secret text/ssh key/文件）与作用域；
5. **JCasC**（plugin 页）——配置即代码的落地路径。

## 推荐学习路径

1. **第 1 天**：docker 起 Jenkins → 建一个自由风格 job 跑通拉码+构建 →
   把同一件事改写成 Jenkinsfile；
2. **第 1 周**：接 Webhook 触发；加一个 agent 节点并用 label 分流；
   把构建凭据迁进 Credentials，体验日志脱敏；
3. **第 1 个月**：写一条完整流水线（构建→测试→镜像→部署→通知）；
   上 JCasC 把系统配置纳入 Git；演练一次 Jenkins 数据迁移（整目录搬家）。

## 版本选择与升级文档

- 只用 LTS 线：<https://www.jenkins.io/changelog-stable/>；
- 升级前看「Upgrade Guide」与插件兼容说明；**先备份 JENKINS_HOME** 再升级；
- 升级顺序：先升核心 → 再逐个升插件；UI 里「插件升级不兼容」会显式列出，
  别忽略告警硬升。

## 社区与求助渠道

- 官方论坛：<https://community.jenkins.io/>；
- GitHub：<https://github.com/jenkinsci/jenkins/issues>；
- 提问带上：Jenkins 与插件版本、Jenkinsfile 片段、构建日志报错段；
- Groovy 报错先分清是 Jenkins DSL 限制还是 Groovy 语法问题——
  declarative 里很多 Groovy 特性被沙箱禁用。
