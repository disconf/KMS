## 配置文件在哪

| 内容 | 位置 |
|---|---|
| 主目录 | `/var/lib/jenkins/`（JENKINS_HOME，备份即备份它） |
| 系统配置 | JENKINS_HOME 下 `config.xml` 及各插件 xml |
| JCasC | `casc.yaml`（装 configuration-as-code 插件后） |
| JVM 参数 | systemd `jenkins.service` 的 `JAVA_OPTS`（堆大小在这调） |

UI 改的每项配置最终都落在 JENKINS_HOME 的 xml 里——**两种改法别混用**，
JCasC 一旦启用就以它为准。

## 参数怎么读

Jenkins 配置分三层：JVM/启动参数（systemd）→ 全局系统配置（Manage Jenkins）→
Job 级配置（Jenkinsfile）。资源类问题先查第一层，行为类问题看后两层。

## 核心参数分组

**JVM 与启动（systemd）**

```ini
JAVA_OPTS="-Xmx4g -Xms4g -Djava.awt.headless=true -Dhudson.model.DirectoryBrowserSupport.CSP="
```

**Jenkinsfile 骨架（声明式）**

```groovy
pipeline {
  agent { label 'docker' }
  options { timestamps(); disableConcurrentBuilds(); timeout(time: 30, unit: 'MINUTES') }
  stages {
    stage('Build')  { steps { sh './build.sh' } }
    stage('Deploy') { steps { withCredentials([string(credentialsId: 'deploy-token', variable: 'TOK')]) { sh './deploy.sh' } } }
  }
  post { always { archiveArtifacts 'dist/**'; junit 'reports/**/*.xml' } }
}
```

**agent 与安全**

- 动态 agent：Kubernetes 插件按需起 Pod 跑构建，用完即焚；
- 全局安全：Matrix 授权策略 + 「匿名只读」按需开。

## 常见误配

1. **master 上跑构建**——构建内存把 UI 打挂，master 只做调度；
2. **凭据写在 Jenkinsfile 里**——用 Credentials Binding 按引用取，日志还会自动脱敏；
3. **没配日志与构建保留策略**——JENKINS_HOME 无限膨胀，磁盘爆掉；
4. **并发构建打架**——同一 job 未设 `disableConcurrentBuilds`，两个构建互踩工作区；
5. **插件全量升级**——一次升一个， Jenkins 插件互相依赖，出问题要能定位到哪次升级。

## 校验与生效

```bash
curl -s localhost:8080/login   # 服务健康
java -jar jenkins-cli.jar -s http://localhost:8080 version
tail -f /var/lib/jenkins/jenkins.log   # 或 journalctl -u jenkins
Manage Jenkins → System Log   # 插件冲突与启动报错
```
