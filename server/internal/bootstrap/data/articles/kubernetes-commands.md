## 日常查看

```bash
kubectl get nodes -o wide                  # 节点与版本
kubectl get pods -A --field-selector=status.phase!=Running   # 找不正常的 Pod
kubectl get events -A --sort-by=.lastTimestamp | tail -30    # 最近事件
kubectl top nodes && kubectl top pods -A   # 资源占用（需要 metrics-server）
kubectl get all -n prod                    # 一个命名空间的全家福
```

## Pod 排障组合拳

```bash
kubectl describe pod web-7d9c -n prod      # 事件在最后，先读 Events
kubectl logs web-7d9c -n prod --previous   # 上一次崩溃前的日志
kubectl logs web-7d9c -n prod -c sidecar   # 多容器时指定容器
kubectl exec -it web-7d9c -n prod -- sh    # 进容器
kubectl port-forward pod/web-7d9c 8080:80  # 本地直连调试
kubectl debug -it web-7d9c --image=busybox # 临时排障容器（镜像里没 sh 时用）
```

状态速查：`Pending` 看调度（资源不够/污点/PVC），`CrashLoopBackOff` 看 logs --previous，
`ImagePullBackOff` 看 describe 里的镜像名与凭证，`Evicted` 看节点压力。

## 资源操作

```bash
kubectl apply -f deploy.yaml               # 声明式交付
kubectl set image deploy/web web=app:v1.3 -n prod
kubectl rollout status deploy/web -n prod  # 盯发布进度
kubectl rollout undo deploy/web -n prod    # 回滚上一版
kubectl rollout history deploy/web -n prod # 看历史版本
kubectl scale deploy/web --replicas=5 -n prod
kubectl cordon node-2 && kubectl uncordon node-2
kubectl drain node-2 --ignore-daemonsets   # 维护前驱逐
```

## 调试与网络

```bash
kubectl run tmp --rm -it --image=nicolaka/netshoot -- /bin/bash  # 临时网络工具箱
kubectl get endpointslices -n prod         # Service 后端到底挂了谁
kubectl exec dns-tools -- nslookup web.prod.svc.cluster.local   # Service DNS
kubectl auth can-i create pods --as=system:serviceaccount:prod:ci   # 权限验证
```

## 节点与证书运维

```bash
kubectl get cs                             # 老版本控制面健康（新版本看 pod）
kubectl get --raw '/readyz?verbose'        # apiserver 就绪明细
kubeadm certs check-expiration             # 证书有效期
kubectl describe node node-1 | grep -A 20 "Allocated resources"
```

## 从告警到定位的标准路径

1. `kubectl get pods -n prod` 锁定异常 Pod；
2. `kubectl describe pod` 读 Events，区分调度失败 / 拉镜像失败 / 启动失败；
3. 启动失败 → `kubectl logs --previous`；重启风暴 → 看 OOMKilled 与探针配置；
4. 单 Pod 没问题但服务不通 → `kubectl get endpointslices` 看后端，再查 NetworkPolicy；
5. 全 namespace 异常 → `kubectl get events` 按时间看，通常是节点级或配额问题。