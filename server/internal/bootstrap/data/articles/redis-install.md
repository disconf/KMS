## 先定形态再谈安装

Redis 的「部署方式」其实是两种维度的组合：安装方式（怎么装）× 架构形态（怎么跑）。

| 形态 | 适用 | 代价 |
|---|---|---|
| 单机 | 缓存丢了能重建、量小 | 无高可用 |
| 主从 + 哨兵（Sentinel） | 中小规模，自动故障转移 | 3 个哨兵起，写仍是单点 |
| Cluster | 数据量大、写多、要水平扩 | 客户端要支持 cluster 协议，多 key 操作受限 |
| Docker / K8s Operator | 容器化环境 | 存储与网络要跟得上 |

## 编译安装（推荐，版本最新）

```bash
yum install -y gcc make
wget https://download.redis.io/releases/redis-7.2.5.tar.gz
tar xzf redis-7.2.5.tar.gz && cd redis-7.2.5
make -j$(nproc) && make install PREFIX=/usr/local/redis

# 自建 systemd 服务
useradd -r -s /sbin/nologin redis
mkdir -p /data/redis /etc/redis && chown -R redis:redis /data/redis
```

systemd 单元：`ExecStart=/usr/local/redis/bin/redis-server /etc/redis/redis.conf --supervised systemd`。

## 发行版包 / Docker

```bash
yum install -y redis          # 版本老，能不用就不用
# Docker（数据必须挂出来）
docker run -d --name redis --restart unless-stopped \
  -p 127.0.0.1:6379:6379 \
  -v /data/redis:/data \
  --sysctl net.core.somaxconn=1024 \
  --memory 2g redis:7.2 redis-server --requirepass 'StrongPass' --appendonly yes
```

## 离线安装

有 gcc 环境时把源码包拷进内网编译；没 gcc 就在外网同版本系统上
`yum install --downloadonly --downloaddir=./pkgs redis` 后离线装。
**gcc 版本太老编译 7.x 会报错**，先确认 `gcc --version >= 5`。

## 安装后的系统参数（必做）

```bash
cat >> /etc/sysctl.conf <<EOF
vm.overcommit_memory = 1
net.core.somaxconn = 1024
EOF
sysctl -p
echo never > /sys/kernel/mm/transparent_hugepage/enabled
# THP 写进 grub 或 rc.local 持久化，否则重启恢复
```

这三项不做，日志里会有 WARNING，而且 fork 时可能卡住整个实例。

## 验证

```bash
redis-cli -a 'StrongPass' ping      # PONG
redis-cli -a 'StrongPass' info server | grep redis_version
redis-benchmark -q -n 100000 -a 'StrongPass'   # 快速压一遍
```