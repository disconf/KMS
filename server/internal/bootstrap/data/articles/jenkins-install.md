## 四种安装

| 方式 | 适用 | 备注 |
|---|---|---|
| 发行版包（yum/apt） | 虚机直接跑，最常见 | Java 17 由包依赖自动带上 |
| WAR + 自备 Java | 已有 Tomcat 体系 | 灵活但升级要手动 |
| Docker | 单机验证 / 隔离要求高 | 数据卷与 Docker sock 挂载想清楚 |
| K8s + JCasC | 大规模、配置即代码 | 首选生产形态 |

## yum 安装（LTS 版本）

```bash
wget -O /etc/yum.repos.d/jenkins.repo \
  https://pkg.jenkins.io/redhat-stable/jenkins.repo
rpm --import https://pkg.jenkins.io/redhat-stable/jenkins.io-2023.key
yum install -y fontconfig java-17-openjdk jenkins
systemctl enable --now jenkins

# 初始密码
cat /var/lib/jenkins/secrets/initialAdminPassword
```

默认端口 8080，改 `JENKINS_PORT` 或 `/etc/sysconfig/jenkins`（新版本用
`/usr/lib/systemd/system/jenkins.service` 里的 `Environment`）。

## Docker 方式

```bash
docker run -d --name jenkins --restart unless-stopped \
  -p 8080:8080 -p 50000:50000 \
  -v /data/jenkins:/var/jenkins_home \
  -v /var/run/docker.sock:/var/run/docker.sock \
  jenkins/jenkins:2.452-lts
```

挂 docker.sock 让容器里能操作宿主 Docker（构建镜像用），
**代价是容器内进程等于宿主 root**，只在可信环境用。

## 初始化必做

1. 解锁后选「安装推荐插件」；
2. 创建管理员，**禁用匿名读**（全局安全配置 → 取消 Anonymous 可读）；
3. 装 blueocean / pipeline 等常用插件；
4. 改工作目录空间监控阈值，磁盘满会让所有构建排队。

## K8s + JCasC（配置即代码）

```bash
helm repo add jenkins https://charts.jenkins.io
helm install jenkins jenkins/jenkins -n jenkins --create-namespace \
  -f values.yaml     # JCasC configScripts 写在 values 里
```

JCasC 让 controller 的插件清单、用户、云配置全部进 Git——
**Jenkins 实例变成可重建的**，这是它值得上生产的前提。

## 验证

```bash
curl -sI localhost:8080 | head -3
journalctl -u jenkins -n 50 --no-pager
java -version    # 17+
```