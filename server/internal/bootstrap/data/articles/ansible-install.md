## 控制节点与被管节点的关系

Ansible 只装在**控制节点**上，被管机只要能 SSH、有 Python 3 即可——
这就是它比 agent 类工具好落地的原因。控制节点推荐 pip 安装（版本最新、可虚拟环境隔离）。

## pip 安装（推荐）

```bash
python3 -m venv /opt/ansible-venv
source /opt/ansible-venv/bin/activate
pip install ansible ansible-lint

ansible --version
# 装完第一件事：确认能连通被管机
ansible all -i '10.0.1.11,' -m ping -u root -k
```

## 发行版包安装

```bash
# EPEL（版本旧一点，胜在省事）
yum install -y epel-release ansible-core
apt install -y ansible        # Ubuntu 官方仓库
```

## 容器化执行（CI 友好）

```bash
docker run --rm -it \
  -v $PWD:/work -v ~/.ssh:/root/.ssh:ro \
  -w /work cytopia/ansible:latest-tools ansible-playbook site.yml
```

CI 里用容器镜像跑 Ansible 是最佳实践：版本锁死在镜像里，不随宿主机漂。

## 离线安装

内网机在外网 `pip download ansible -d pkgs/`，连同依赖拷过去
`pip install --no-index --find-links=pkgs/ ansible`。
**pip 源和系统 Python 版本要对齐**，否则依赖解析会给你惊喜。

## SSH 免密的准备（比装 Ansible 更花时间）

```bash
ssh-keygen -t ed25519
ssh-copy-id root@10.0.1.11
# 验证免密
ssh root@10.0.1.11 'hostname'
```

密钥没有直接权限的机器（跳板机后），用 ProxyJump：
`ansible_ssh_common_args='-o ProxyJump=bastion'`。

## 最小验证

```bash
mkdir -p /etc/ansible && cat > /etc/ansible/hosts <<EOF
[web]
10.0.1.11
10.0.1.12
EOF
ansible web -m ping
ansible web -m command -a 'uptime'
```