## CLI（在 controller 容器/机器上）

```bash
# 启用：管理 Jenkins → CLI 手册里下载 jenkins-cli.jar 或开 SSH CLI
java -jar jenkins-cli.jar -s http://localhost:8080 -auth user:token list-jobs
java -jar jenkins-cli.jar -s http://localhost:8080 -auth user:token build job-name
java -jar jenkins-cli.jar -s http://localhost:8080 -auth user:token console job-name 42
java -jar jenkins-cli.jar -s http://localhost:8080 -auth user:token reload-configuration
```

`-auth user:APIToken`（个人设置里生成），别用密码。

## Groovy 控制台（应急手术刀）

管理 Jenkins → Script Console，能改运行中实例的任何状态：

```groovy
// 解锁全部排队中的任务
Jenkins.get().queue.clear()

// 看某个 job 的最近构建状态
def j = Jenkins.get().getItemByFullName('order-api')
println "${j.lastBuild.number} ${j.lastBuild.result}"

// 批量把任务的描述写上 owner
Jenkins.get().getItems().each { it.setDescription("owner: sre-team") }
```

控制台权限等价 root，用完即走，别在脚本里留死循环。

## 日志与定位

```bash
# controller 主日志
tail -f /var/lib/jenkins/jenkins.log        # 虚机
docker logs -f jenkins                       # 容器

# 单次构建日志
cat /var/lib/jenkins/jobs/<job>/builds/42/log

# agent 连接失败先看这里
grep -i 'Remoting\|Channel' /var/lib/jenkins/jenkins.log | tail
```

## 构建队列堆积排查

1. 管理 Jenkins → Build Queue：看堵了多少、缺什么标签的 executor；
2. agent 掉线（离线原因会显示在节点页）：磁盘满 / SSH 断 / JNLP 端口不通；
3. 单个任务跑飞：Console Output 找到卡住的 stage，通常是等交互输入或下载超时；
4. 长期方案：给慢 stage 加 `timeout(time: 30, unit: 'MINUTES')`，给重构建任务分组专用 agent。

## 日常组合拳

```bash
# 巡检：离线节点 + 失败任务
java -jar jenkins-cli.jar ... list-computers | grep -B1 offline
curl -s 'localhost:8080/api/json?tree=jobs[name,lastBuild[result]]' | \
  jq -r '.jobs[] | select(.lastBuild.result=="FAILURE") | .name'
```

## 升级

```bash
# LTS 版本，升级 = 换 war / 换容器镜像，先备份
cp /usr/share/java/jenkins.war /usr/share/java/jenkins.war.bak
systemctl restart jenkins
# 插件升级在管理界面批量做，一次别升太多
```