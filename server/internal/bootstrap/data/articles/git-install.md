## 客户端安装

```bash
# 发行版包：版本通常落后一到两个大版本，够用
yum install -y git
apt install -y git

# 源码编译（要最新特性时）
yum install -y gcc make gettext-devel curl-devel openssl-devel zlib-devel
make prefix=/usr/local all && make prefix=/usr/local install

# Windows：Git for Windows 自带 bash 环境
```

版本检查：`git --version`。团队统一版本能避免一些协议行为差异。

## 全局配置（装完必做）

```bash
git config --global user.name "yourname"
git config --global user.email "you@example.com"
git config --global init.defaultBranch main
git config --global core.quotepath false      # 中文文件名不转义
git config --global pull.rebase true          # pull 默认 rebase，历史线性
git config --global credential.helper store   # 内网环境；公网机器用 ssh key
```

## 自建 Git 服务：Gitea 是性价比最高的选择

```bash
# Gitea 单二进制 + SQLite，十分钟一台内部 Git 服务
wget https://dl.gitea.com/gitea/1.22/gitea-1.22-linux-amd64 -O /usr/local/bin/gitea
chmod +x /usr/local/bin/gitea
useradd -r -s /sbin/nologin git
mkdir -p /data/gitea && chown git:git /data/gitea
sudo -u git gitea web --config /etc/gitea/app.ini
```

选型参考：小团队自托管选 Gitea；需要 CI/代码评审一体化且公司预算充足选 GitLab；
已经全面 K8s 可以考虑 Gitea + Actions Runner。

## 服务端裸仓库（最轻的「自建」）

```bash
git init --bare /srv/git/ops-scripts.git
# 客户端直接推
git remote add ops opsuser@10.0.1.11:/srv/git/ops-scripts.git
git push ops main
```

给 shell 账号加 `git-shell` 限制：`usermod -s /usr/bin/git-shell opsuser`，
只能走 Git 协议不能登录 shell。

## SSH 密钥接入

```bash
ssh-keygen -t ed25519 -C "you@company"
cat ~/.ssh/id_ed25519.pub   # 贴到 Git 服务的 SSH Keys
ssh -T git@git.internal     # 测试（看到欢迎语即通）
```