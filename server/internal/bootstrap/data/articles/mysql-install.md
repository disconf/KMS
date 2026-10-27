## MySQL 8.0 官方仓库安装（CentOS）

```bash
yum install -y https://dev.mysql.com/get/mysql80-community-release-el7-11.noarch.rpm
# 国内网络可换阿里镜像源
yum install -y mysql-community-server
systemctl enable --now mysqld

# 8.0 首次启动自动生成 root 临时密码，在日志里
grep 'temporary password' /var/log/mysqld.log
mysql_secure_installation     # 改密码、删匿名账号、禁远程 root
```

## 二进制安装（多实例 / 定制路径）

```bash
tar xJf mysql-8.0.36-linux-glibc2.17-x86_64-minimal.tar.xz -C /usr/local/
ln -s /usr/local/mysql-8.0.36* /usr/local/mysql
useradd -r -s /sbin/nologin mysql
mkdir -p /data/mysql && chown mysql:mysql /data/mysql

/usr/local/mysql/bin/mysqld --initialize-insecure \
  --user=mysql --basedir=/usr/local/mysql --datadir=/data/mysql
```

`--initialize-insecure` 生成空密码 root，装完立刻设强密码；
`--initialize` 会生成临时密码（写到 error log）。

## Docker 方式（开发/测试友好）

```bash
docker run -d --name mysql8 --restart unless-stopped \
  -p 3306:3306 \
  -v /data/mysql:/var/lib/mysql \
  -v /etc/mysql/conf.d:/etc/mysql/conf.d:ro \
  -e MYSQL_ROOT_PASSWORD='StrongPass' \
  -e TZ=Asia/Shanghai \
  mysql:8.0
```

生产库不建议容器化：磁盘 IO、NUMA 与 OOM 管控都不如虚机直接。

## 多实例（3307/3308）要点

同一台机器跑多实例时用 `mysqld_multi` 或多个 systemd 单元，
**每个实例独立的 datadir / port / socket / server_id**，配置文件里写清楚。

## 安装后必做

```sql
-- 字符集与时区（8.0 默认 utf8mb4，确认即可）
SHOW VARIABLES LIKE 'character_set_server';
SHOW VARIABLES LIKE 'time_zone';
-- 时区跟随东八区：default-time-zone = '+8:00' 写进 my.cnf
```

```bash
# binlog 开启确认（8.0 默认开）
mysql -e "SHOW VARIABLES LIKE 'log_bin'"    # ON 才能做复制与恢复
```