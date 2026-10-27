## 三种安装

```bash
# 1. yum 仓库（推荐，版本新）
cat > /etc/yum.repos.d/grafana.repo <<EOF
[grafana]
baseurl=https://rpm.grafana.com
repo_gpgcheck=1
gpgcheck=1
gpgkey=https://rpm.grafana.com/gpg.key
EOF
yum install -y grafana && systemctl enable --now grafana-server

# 2. Docker
docker run -d --name grafana --restart unless-stopped \
  -p 3000:3000 \
  -v /data/grafana:/var/lib/grafana \
  -e GF_SECURITY_ADMIN_PASSWORD='StrongPass' \
  grafana/grafana:11.2.0

# 3. Helm（K8s）
helm install grafana grafana/grafana -n monitoring \
  --set admin.password='StrongPass' --set persistence.enabled=true
```

## 两种配置入口

配置文件 `/etc/grafana/grafana.ini` 与环境变量 `GF_<SECTION>_<KEY>` 等效，
**容器场景用环境变量，虚机用 ini**，别两边混着写（环境变量优先）。

```ini
[server]
root_url = https://grafana.example.com
domain = grafana.example.com

[security]
admin_user = admin
disable_initial_admin_creation = false

[auth.anonymous]
enabled = false           # 生产禁匿名
```

改配置后 `systemctl restart grafana-server`。

## 首次登录后必做

1. 改掉默认 admin 密码；
2. 装插件按需：`grafana-cli plugins install <id>` 后重启；
3. 配置数据源（Prometheus URL 指向本机 `http://localhost:9090`）；
4. 初始大盘：Prometheus 生态直接导入 dashboard ID（如 Node Exporter Full = 1860）。

## 离线安装

外网 `yum install --downloadonly --downloaddir=./pkgs grafana` 拷入内网，
或直接下 tar.gz 包（自带所有依赖，解压即用）：

```bash
tar xzf grafana-11.2.0.linux-amd64.tar.gz -C /opt/
/opt/grafana-v11.2.0/bin/grafana server --config /opt/grafana/conf/defaults.ini
```

## 验证

```bash
curl -s localhost:3000/api/health          # {"database":"ok",...}
systemctl status grafana-server --no-pager | head -5
tail -f /var/log/grafana/grafana.log       # 启动失败原因基本都在这
```