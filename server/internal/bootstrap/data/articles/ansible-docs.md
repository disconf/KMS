## 文档入口

| 找什么 | 去哪 |
|---|---|
| 文档总入口 | <https://docs.ansible.com/ansible/latest/index.html> |
| 模块 / 插件索引 | <https://docs.ansible.com/ansible/latest/collections/index.html> |
| Playbook 关键字参考 | <https://docs.ansible.com/ansible/latest/reference_appendices/playbooks_keywords.html> |
| 变量优先级全表 | <https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_variables.html#variable-precedence> |

## 必读页面清单

1. **Variable precedence 全表**——变量为什么没生效，答案永远在这张表里；
2. **内置模块（ansible.builtin）**——copy/template/service/yum/shell 的参数表，
   写任务前先查有没有现成模块；
3. **Playbook best practices**（sample_setup 目录结构）——团队协作的项目骨架；
4. **Conditionals / Loops**——when 与 loop 的组合细节；
5. **ansible-vault**——敏感信息管理的唯一官方方案。

## 推荐学习路径

1. **第 1 天**：装好 → inventory 写两台机器 → ad-hoc 跑 ping/command →
   写一个三行 playbook 装个包，观察第二次执行的 changed=False；
2. **第 1 周**：写第一个 role（nginx 配置下发：template + handler reload）；
   用 --check --diff 体会幂等；把变量按环境拆进 group_vars；
3. **第 1 个月**：项目结构按 best practices 重组；引入 vault 管密码；
   接入 CI（ansible-lint + 语法检查）；用 serial 滚动策略上线一次真实变更。

## 版本选择与升级文档

- Ansible 核心与 collections 独立发版，[发布页](https://github.com/ansible/ansible/releases)；
- 升级看 [porting guides](https://docs.ansible.com/ansible/latest/porting_guides.html)——
  每个大版本的模块参数废弃都在里面；
- 用 `requirements.yml` 锁 collections 版本，团队版本一致是协作底线。

## 社区与求助渠道

- GitHub Discussions：<https://github.com/ansible/ansible/discussions>；
- Matrix/IRC #ansible 频道；
- 提问带上：ansible 版本、最小复现 playbook、`-vvv` 输出中报错前后 20 行；
- 「模块不存在/参数不认识」先确认 collection 装没装——2.10 之后八成是这个问题。
