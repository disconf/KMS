## 配置文件在哪

| 文件 | 作用 |
|---|---|
| `ansible.cfg` | 控制机行为（inventory 路径、并发、SSH 参数），按目录就近生效 |
| `inventory/` | 主机清单与组变量（`group_vars/`、`host_vars/`） |
| `~/.ansible.cfg` → `/etc/ansible/ansible.cfg` | 用户级 → 全局兜底 |

查找顺序：当前目录 `ansible.cfg` 优先——**项目内放一份 ansible.cfg 是标准做法**。

## 参数怎么读

变量优先级从低到高大致是：role defaults → inventory group_vars → host_vars →
play vars → extra vars（`-e`）。**-e 永远最高**，排查「变量没生效」从低往高找覆盖。

## 核心参数分组

**ansible.cfg 关键项**

```ini
[defaults]
inventory = ./inventory
forks = 20                    # 并发数，默认 5，机器多调大
gathering = smart             # facts 缓存，加速大清单
host_key_checking = False     # 内网装机场景
[ssh_connection]
pipelining = True             # 减少 SSH 往返，明显提速
```

**Playbook 安全阀**

```yaml
- hosts: web
  serial: "20%"               # 滚动执行，一次 20% 主机，失败即停
  any_errors_fatal: true
  max_fail_percentage: 10
```

**变量与加密**

```bash
ansible-vault create group_vars/prod/vault.yml
ansible-playbook site.yml --ask-vault-pass
```

**幂等写法要点**：能用 module 就别 `shell`；`shell` 任务加 `changed_when`/`creates`；
`--check --diff` 是干跑利器。

## 常见误配

1. **变量放错层级**——group_vars 目录名与 inventory 组名不一致，变量静默不加载；
2. **shell 里写「安装」不加 creates**——每次都跑，幂等性没了；
3. **forks 太小**——百台机器跑一小时，先看是不是并发瓶颈；
4. **vault 密码进 Git**——密码文件放密码管理器，仓库里只留 `.vault-pass` 的路径引用；
5. **在 handler 里放业务逻辑**——handler 只做「重启/重载」类收尾动作。

## 校验与生效

```bash
ansible-lint site.yml                 # 静态检查
ansible-playbook site.yml --check --diff --limit web   # 干跑看变更
ansible-inventory --graph             # 清单与变量加载是否正确
ansible web -m ping -f 20             # 连通性与并发
```
