## ad-hoc：两分钟干完的事别写 playbook

```bash
ansible web -m ping                        # 连通性
ansible web -m command -a 'df -h /data'
ansible web -m shell -a 'free -m | grep Mem'
ansible web -m service -a 'name=nginx state=reloaded'
ansible web -m copy -a 'src=/etc/motd dest=/etc/motd backup=yes'
ansible web -m yum -a 'name=htop state=present'
ansible web -m setup -a 'filter=ansible_memtotal_mb'   # 查 facts
ansible all -m ping -o                     # 一行一台，方便截图
```

`command` 不支持管道，要管道用 `shell` 模块。

## playbook 执行

```bash
ansible-playbook site.yml
ansible-playbook site.yml --check --diff
ansible-playbook site.yml --limit 'web-1'
ansible-playbook site.yml --tags nginx --skip-tags backup
ansible-playbook site.yml -e 'app_version=v1.4.0'   # extra-vars 优先级最高
ansible-playbook site.yml --vault-password-file ~/.vault_pass
```

## 加密与敏感信息

```bash
ansible-vault create group_vars/prod/vault.yml    # 新建加密文件
ansible-vault edit group_vars/prod/vault.yml      # 编辑
ansible-vault rekey group_vars/prod/vault.yml     # 换密码
ansible-vault encrypt_string 'StrongPass' --name db_password   # 只加密一个值
```

## 调试三板斧

```bash
ansible web -m debug -a 'var=ansible_default_ipv4.address'
ansible-playbook site.yml -vvv              # 最详细，看模块最终执行的命令
ansible-inventory --graph --vars            # 看 inventory 最终解析结果
ansible-doc -s yum                          # 看模块参数（比搜索引擎快）
```

## 常见「不生效」排查

1. **改了 inventory 没效果**：`ansible-inventory --graph` 看解析结果，
   多半是同 IP 在两个组里或 yaml/ini 格式混用；
2. **变量被覆盖**：`ansible-playbook site.yml -e '{}'` 逐层排除，
   或临时加 debug 任务打出实际值；
3. **任务总显示 changed**：`shell/command` 模块没有幂等判断，加 `changed_when`；
4. **只有一台机器失败**：先看 `/root/.ansible/` 下 facts 是否缓存了坏数据，
   删掉重跑；再看那台机器的 SSH 与 Python 版本。