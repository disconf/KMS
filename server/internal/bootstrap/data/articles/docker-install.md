## 先说结论：别用发行版仓库里的 docker.io

CentOS / Ubuntu 自带仓库里的 Docker 版本老、更新慢，出问题时官方文档对不上。
生产环境一律用 **Docker 官方仓库**（`download.docker.com`）或其国内镜像，版本跟随官方 stable。

| 方式 | 适用场景 | 升级路径 |
|---|---|---|
| 官方 yum/apt 仓库 | 绝大多数物理机与虚机，首选 | `yum update` 即可 |
| 二进制 tgz | 无 root、内网无仓库、临时验证 | 替换文件重启 |
| 离线 rpm/deb | 完全隔离的内网，包先在外网下载 | 上传后 rpm -Uvh |
| Docker 容器跑 Docker（dind） | 只在 CI 里用 | 换镜像 tag |

## 官方仓库安装（CentOS / RHEL / openEuler）

```bash
# 1. 卸掉旧版（包名不同，都要清）
yum remove -y docker docker-client docker-common docker-engine docker.io containerd runc

# 2. 装 yum 工具并加官方仓库（国内可把 baseurl 换成 mirrors.aliyun.com/docker-ce）
yum install -y yum-utils
yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo

# 3. 安装（containerd 会作为依赖一起装上）
yum install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# 4. 启动 + 开机自启
systemctl enable --now docker

# 5. 验证
docker version && docker run --rm hello-world
```

## 官方仓库安装（Ubuntu / Debian）

```bash
apt-get update && apt-get install -y ca-certificates curl gnupg
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | \
  gpg --dearmor -o /etc/apt/keyrings/docker.gpg

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" \
  > /etc/apt/sources.list.d/docker.list

apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
```

## 二进制安装（无仓库环境）

```bash
# 从 download.docker.com/linux/static/stable/x86_64/ 下载 tgz 后：
tar xzf docker-27.*.tgz
cp docker/* /usr/local/bin/

# 二进制方式没有 systemd 单元，自己写一个（ExecStart 直接拉 dockerd）
useradd --system --home /var/lib/docker --shell /sbin/nologin dockerd 2>/dev/null || true
```

systemd 单元最小写法：

```ini
[Service]
ExecStart=/usr/local/bin/dockerd
Restart=always
RestartSec=5
LimitNOFILE=1048576
```

## 离线安装

在外网同版本系统上 `yum install --downloadonly --downloaddir=./pkgs docker-ce`，
把整个目录拷进内网，`rpm -Uvh ./pkgs/*.rpm`。**依赖包一个都不能少**，
containerd、runc、libseccomp 都在依赖列表里，漏一个安装就断。

## 装完必做的两件事

```bash
# 1. 让运维账号免 sudo 用 docker（docker 组等价 root，只给可信账号）
usermod -aG docker opsuser

# 2. 写 /etc/docker/daemon.json（日志限制 + 镜像加速），见生产实战篇
mkdir -p /etc/docker
```

不做日志限制，默认 json-file 会把磁盘写满——这是 Docker 上生产前最常见的事故。
