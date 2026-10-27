## 它是什么

Jenkins 是**最老牌的开源 CI/CD 服务器**：Java 编写、插件生态上千个，
把「拉代码 → 构建 → 测试 → 部署」编排成流水线（Pipeline）。几乎每种语言、
每套发布系统都有现成插件——这是它二十年来仍是企业 CI 主力的原因。

## 解决什么问题

- **自动化触发**：Git push / 定时 / 手工参数化触发构建；
- **流水线即代码**：Jenkinsfile 进仓库，构建逻辑跟代码一起评审和版本化；
- **分布式构建**：master 调度，agent 分担构建负载，不同环境的构建各自有专属节点；
- **过程可视化**：每次构建的日志、产物、测试报告有完整留痕。

## 架构与核心概念

| 概念 | 一句话 |
|---|---|
| Master（controller） | 调度与 UI，**不跑业务构建** |
| Agent | 执行节点，按 label 匹配任务 |
| Job / Pipeline | 构建单元；Pipeline 用 Jenkinsfile（Groovy DSL）描述 |
| Stage / Step | 流水线的阶段与最小动作 |
| Credentials | 凭据仓库，流水线里按 ID 引用 |
| JCasC | 用 YAML 声明 Jenkins 配置，配置即代码 |

## 版本与生态

- 每周发版、 LTS（长支持）线约 4 周一版，**生产用 LTS**；
- 关键插件：Pipeline、Git、Credentials Binding、Blue Ocean（可选 UI）、
  Kubernetes（动态 agent）；
- 定期看插件中心的安全告警：<https://www.jenkins.io/security/advisories/>。

## 适用与不适用

**适合**：传统多语言企业的 CI 中枢、复杂构建编排、历史资产延续。
**不适合**：追求极简的新团队（CI 用 GitLab CI/GitHub Actions 更省心）；
把 Jenkins 当调度器跑非构建任务（cron 是更好的选择）。

## 官方文档导读

- 总入口：<https://www.jenkins.io/doc/>
- Pipeline 语法：<https://www.jenkins.io/doc/book/pipeline/syntax/>
- 管理（备份/升级/agent）：<https://www.jenkins.io/doc/book/managing/>
