## 它是什么

Ansible 是**无 agent 的自动化工具**：通过 SSH（Linux）或 WinRM（Windows）连到目标机器，
把配置、部署、批量操作写成 YAML（Playbook）执行。没有守护进程、不用装客户端，
控制机上装好就能管几百台机器。

## 解决什么问题

十台机器改个内核参数还能 ssh 敲命令，一百台就不行了。Ansible 的价值：
**幂等**的模块（同一个任务跑两遍结果一样）、**声明式**表达期望状态、
执行过程可审计（每个任务的输出都留痕）。批量改配置、发版、巡检、装软件，
全是它的主场。

## 架构与核心概念

| 概念 | 一句话 |
|---|---|
| Inventory | 主机清单（ini/yaml），可分组、分环境、带变量 |
| Module | 幂等执行单元（yum/copy/template/service…），上千个内置 |
| Task / Play / Playbook | 任务 → 针对一组主机的编排 → 编排文件 |
| Role | 可复用的任务包（task/handler/template/defaults 标准目录） |
| Facts | 执行前自动收集的目标机信息（ansible_facts） |
| Vault | 加密敏感变量的机制 |

**执行模型**：控制机渲染模板 → SSH 推到目标执行 → 收结果。没有中心数据库，
状态就是机器本身。

## 版本与生态

- 社区版 ansible（pip 安装）与企业版 AAP（带 UI/调度/RBAC）双轨；
- 2.10 起模块拆到 **collections**（`ansible.builtin.*`、`community.general.*`）；
- 配套：Molecule（role 测试）、ansible-lint（静态检查）、AWX（开源 Web 界面）。

## 适用与不适用

**适合**：批量配置管理、应用发布、巡检与整改、装机初始化。
**不适合**：秒级高频变更（SSH 推送模型有延迟）、需要持续 agent 上报的场景
（监控不是它的活）。

## 官方文档导读

- 总入口：<https://docs.ansible.com/ansible/latest/index.html>
- 模块索引（最常查）：<https://docs.ansible.com/ansible/latest/collections/index.html>
- 最佳实践（目录结构）：<https://docs.ansible.com/ansible/latest/tips_tricks/sample_setup.html>
