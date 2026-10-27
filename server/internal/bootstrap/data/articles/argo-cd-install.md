## 两种部署形态

| 形态 | 适用 | 说明 |
|---|---|---|
| 非高可用（install.yaml） | POC、小团队、单集群 | 一个副本，资源占用小 |
| 高可用（ha/install.yaml） | 生产 | API/仓库服务多副本，redis 哨兵内置 |

## 标准安装（命名空间 argocd）

```bash
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# 等全部 Running
kubectl get pods -n argocd -w
```

高可用版把 URL 换成 `manifests/ha/install.yaml` 即可，组件名一致。

## CLI 安装与登录

```bash
# 客户端
curl -sSL -o argocd https://github.com/argoproj/argo-cd/releases/latest/download/argocd-linux-amd64
install -m 555 argocd /usr/local/bin/argocd

# 初始 admin 密码（存在 argocd-initial-admin-secret）
kubectl -n argocd get secret argocd-initial-admin-secret \
  -o jsonpath='{.data.password}' | base64 -d && echo

argocd login argocd.example.com --username admin --grpc-web
# 立刻改密码并接 SSO，admin 长期直用是安全隐患
```

## Ingress 暴露

gRPC 走同域名需要 ingress 能透传 HTTP/2（nginx ingress 加
`nginx.ingress.kubernetes.io/backend-protocol: "GRPC"` 注解），或 CLI 用 `--grpc-web`。

## Helm / Operator 方式

```bash
helm repo add argo https://argoproj.github.io/argo-helm
helm install argocd argo/argo-cd -n argocd \
  --set configs.params."server\.insecure"=true    # TLS 交给 ingress
```

需要管理多个 Argo CD 实例或 ApplicationSet 集成时再考虑 Operator，常规场景 Helm 足够。

## 注册第一个集群与仓库

```bash
argocd cluster add kind-k8s-prod --name prod      # 拉取目标集群 kubeconfig
argocd repo add https://git.internal/deploy.git \
  --username ci --password $GIT_TOKEN
```

## 验证

```bash
argocd app create demo --repo https://git.internal/deploy.git \
  --path overlays/prod --dest-server https://kubernetes.default.svc \
  --dest-namespace demo
argocd app sync demo && argocd app get demo
```