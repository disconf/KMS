## inventory 分层：生产环境的第一道秩序

```ini
[web]
web-1 ansible_host=10.0.1.11
web-2 ansible_host=10.0.1.12

[db]
db-1 ansible_host=10.0.2.11

[prod:children]
web
db

[prod:vars]
ansible_user=ops
ansible_ssh_common_args='-o ProxyJump=bastion.internal'
```

按「业务角色 × 环境」组织，`group_vars/prod.yml` 放环境差异。
**所有环境一份 playbook，只换 inventory**，否则改配置要改三处。

## ansible.cfg 调优（控制节点侧）

```ini
[defaults]
inventory = ./inventories/prod
forks = 20                    # 并发数，默认 5 太慢
gathering = smart             # facts 缓存
fact_caching = jsonfile
fact_caching_connection = ./.facts
fact_caching_timeout = 86400
host_key_checking = False     # 内网可关，公网必须开

[ssh_connection]
pipelining = True             # 减 SSH 往返，大批量提升明显
ssh_args = -o ControlMaster=auto -o ControlPersist=600s
```

## 变量覆盖顺序（背下来能省一半排障时间）

优先级从低到高：role defaults → inventory group_vars → inventory host_vars →
play vars → extra-vars（命令行）。**extra-vars 永远赢**，
排查「明明写了变量为什么不生效」先确认是不是被更高层覆盖。

## 批量操作的安全阀

```bash
ansible-playbook site.yml --check          # 干跑，只报会改什么
ansible-playbook site.yml --diff           # 连文件差异一起看
ansible-playbook site.yml --limit web-1    # 先打一台
ansible-playbook site.yml --start-at-task="安装 nginx"
ansible-playbook site.yml --step           # 逐任务确认
```

放量顺序永远是：`--check` → 单台 → 全量。这个顺序省下的每一次事故都值回票价。

## Playbook 的工程纪律

1. 幂等：用 `copy/template/systemd` 模块而不是 `shell`，`shell` 要自带幂等判断；
2. `changed_when/failed_when` 显式声明，让输出可信；
3. 密码进 `ansible-vault`，文件头 `---` 加 `vault` 提示；
4. role 化：三个 playbook 用同一段逻辑时就该抽成 role；
5. `serial: 2` 滚动执行，防止一次性改挂全部节点。

## 验收清单

- [ ] `ansible-lint` 过，playbook 进 Git
- [ ] vault 密钥不入库，用环境变量传
- [ ] 干跑 + 单台 + 全量的放量流程写进团队规范
- [ ] facts 缓存生效（第二次跑明显变快）