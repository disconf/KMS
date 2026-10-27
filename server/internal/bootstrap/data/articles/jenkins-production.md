## Agent 分布式：controller 只做调度

构建全部打到 agent 上，controller 不跑构建（磁盘、CPU 都留给调度）：

```groovy
// 管理 Jenkins → Nodes：静态 agent 走 JNLP 或 SSH
// pipeline 里指定标签
pipeline {
  agent { label 'docker && linux' }
  stages { stage('build') { steps { sh 'make build' } } }
}
```

- **SSH agent**：普通虚机，Jenkins 自动管理连接；
- **Docker agent**：每次构建起一次性容器，环境干净；
- **K8s cloud**：Pod 按需起，弹性最好。

标签体系要规划（`docker`、`node18`、`arm64`），标签即构建资源的分类账。

## 凭据与权限

凭据全部进 Credentials（不要写进 Jenkinsfile）：

```groovy
withCredentials([string(credentialsId: 'harbor-token', variable: 'TOKEN')]) {
  sh 'docker login harbor.internal -u ci -p $TOKEN'
}
```

- RBAC 插件按「文件夹 + 角色」授权，开发者只读自己项目的构建；
- `Anonymous` 一律不给 build 权限；
- 凭据用 Vault/Secrets 管理插件同步，**Jenkins 不是密码保险箱**。

## 备份与迁移

```bash
# 最小备份集：配置 + 凭据 + 任务定义（workspace 和构建历史可以不备）
tar czf jenkins-conf-$(date +%F).tgz \
  /var/lib/jenkins/{config.xml,credentials.xml,jobs/*/config.xml,*.xml} \
  /var/lib/jenkins/secrets/

# JCasC 场景：整个实例 = values.yaml + 插件清单，重建就是 helm install
```

构建历史要归档就同步 `jobs/*/builds/`，量大的话单独放。

## Pipeline 即代码

Jenkinsfile 进业务仓库（`declarative`），共享逻辑进 Shared Library：

```groovy
@Library('ops-pipeline') _
standardJavaRelease { service = 'order-api' }
```

**UI 里手改的任务（freestyle）都是技术债**：不可评审、不可回滚、不可迁移。

## 验收清单

- [ ] controller 无构建任务，agent 标签规划清晰
- [ ] 全部流水线声明式 + Jenkinsfile 入库
- [ ] 凭据零明文，RBAC 覆盖所有文件夹
- [ ] 配置每日备份且做过一次恢复演练
- [ ] 插件锁版本（JCasC 的 plugins.txt），升级先在测试实例验证