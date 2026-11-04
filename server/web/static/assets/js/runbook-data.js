/* ============================================================================
   runbook-data.js — 排障剧本：把"现象 → 判断 → 下一步"固化成决策树。

   与文章、题库的分工：
     - 文章讲"是什么、为什么"，
     - 题库考"你记不记得"，
     - 剧本解决"现在线上炸了，先敲哪条命令"。
   所以每一步都必须是可执行的命令 + 明确的判断条件，不写"检查一下网络"
   这种废话；每条分支都要能落到下一个可执行的步骤上。

   数据结构：
     steps[] 每步有 id、title（在确认什么）、detail（为什么这么判）、
     cmd（照抄可跑）、branches（命中哪个现象走哪一步）；
     terminal 为 true 的步骤是结论节点，带 rootCause 与 fix 清单。

   改这份数据时注意：branches 里的 goto 必须指向同一剧本里真实存在的 id，
   pages-runbook.js 在渲染前会做一次全量校验，写错会直接抛错——宁可页面白屏
   也不要让运维在事故现场点进一个死胡同。
   ========================================================================= */

export const RUNBOOKS = [
  /* ------------------------------------------------------------------ 1 */
  {
    slug: 'crashloop',
    title: 'Pod 一直 CrashLoopBackOff',
    scene: 'Kubernetes',
    icon: 'package',
    severity: 'high',
    summary: '容器起来就跑、跑完就退，反复重启，日志还看不到几行',
    keywords: ['CrashLoopBackOff', '重启', '退出码', 'pod 起不来', 'ExitCode'],
    entry: 's1',
    steps: [
      {
        id: 's1',
        title: '先看上一次退出的退出码',
        detail: '退出码直接决定走哪条路：137 是 OOMKilled，143 是被 SIGTERM 干掉，'
          + '1/2 是应用自己报错退出。跳过这步去看日志，等于在有 30 行无关输出的日志里瞎找。',
        cmd: "kubectl get pod <pod> -n <ns> -o jsonpath='{.status.containerStatuses[*].lastState.terminated.exitCode}'",
        branches: [
          { when: '137 或 reason=OOMKilled', goto: 'end-oom', hint: '内存超 limit 被杀' },
          { when: '143（SIGTERM）', goto: 's4', hint: '被探针或驱逐终止' },
          { when: '1 / 2 / 其他非零', goto: 's2', hint: '应用自己退出' },
        ],
      },
      {
        id: 's2',
        title: '看上个容器的日志尾部',
        detail: '必须带 --previous，否则看到的是正在重启的这个容器（它可能还没来得及输出）。',
        cmd: 'kubectl logs <pod> -n <ns> --previous --tail=100',
        branches: [
          { when: '报 dial tcp / connection refused / no such host', goto: 's3', hint: '依赖连不上' },
          { when: '报 permission denied / no such file', goto: 'end-mount', hint: '挂载或权限问题' },
          { when: '日志为空或只有启动横幅', goto: 'end-entry', hint: '进程根本没跑起来' },
        ],
      },
      {
        id: 's3',
        title: '从同一个 namespace 里探依赖是否可达',
        detail: '不要在宿主机上测——宿主机能通不代表 Pod 网段能通，NetworkPolicy 只拦 Pod。',
        cmd: "kubectl run tmp-net --rm -it --image=busybox:1.36 --restart=Never -- sh -c 'nc -zv <svc> <port>'",
        branches: [
          { when: 'connection refused', goto: 'end-dep', hint: '依赖没起或端口不对' },
          { when: '超时无响应', goto: 'end-netpol', hint: '网络策略或 DNS' },
          { when: '能连上', goto: 'end-config', hint: '连接串/账号/库名配错了' },
        ],
      },
      {
        id: 's4',
        title: '143 通常是探针失败被 kubelet 重启',
        detail: 'liveness 探针连续失败会触发重启，而应用只是"启动慢"——这是最常见的误杀。'
          + '看 Events 里的 Killing 与 Unhealthy 记录，再对比探针参数。',
        cmd: 'kubectl describe pod <pod> -n <ns> | sed -n "/Events/,$p" | tail -20',
        branches: [
          { when: '出现 Liveness probe failed', goto: 'end-probe' },
          { when: '出现 Evicted / The node was low on resource', goto: 'end-oom', hint: '节点资源压力驱逐' },
        ],
      },

      {
        id: 'end-oom',
        title: '结论：内存触顶被杀',
        terminal: true,
        detail: '容器真实用量超过 limits.memory，内核 OOM Killer 直接干掉进程。'
          + '注意 137 不等于"一定泄漏"——也可能是 limit 给得太小、或 JVM 堆设得比 limit 还大。',
        rootCause: '内存 limit 过小 / 应用内存泄漏 / JVM 堆参数与 limit 不匹配',
        fix: [
          '先看真实峰值：kubectl top pod <pod>，对照 limits.memory',
          'JVM 应用必须设 -XX:MaxRAMPercentage=70 这类按比例的值，别写死 -Xmx2g 而 limit 只有 1Gi',
          '确认是泄漏（内存曲线单调上升）再谈扩容，否则扩容只是把崩溃往后拖',
        ],
        verify: 'kubectl get pod <pod> -o jsonpath="{.status.containerStatuses[*].restartCount}" 观察 30 分钟内不再增长',
      },
      {
        id: 'end-config',
        title: '结论：配置错了，网络本身是通的',
        terminal: true,
        detail: '依赖能连上却仍然报错，八成是连接串、库名、账号密码或环境变量没注入。'
          + '重点核对 ConfigMap/Secret 的 key 是否与代码读取的名字完全一致（大小写、下划线）。',
        rootCause: '配置项错误或环境变量缺失',
        fix: [
          'kubectl exec 进容器 env | grep 关键变量，确认注入生效',
          'Secret 用 stringData 写错了会 base64 编两遍，解出来是乱码',
          '改完配置记得 rollout restart，挂载的 ConfigMap 不会让进程自动重读',
        ],
        verify: 'kubectl rollout status deploy/<name> 后日志无连接错误',
      },
      {
        id: 'end-mount',
        title: '结论：挂载或文件权限问题',
        terminal: true,
        detail: '典型是挂载了 ConfigMap 但在容器里读不到，或非 root 用户跑容器而卷属主是 root。',
        rootCause: '卷未就绪 / 文件权限与运行用户不匹配 / subPath 写错',
        fix: [
          'kubectl describe pod 看 Mounts 段与实际路径是否一致',
          '容器以非 root 运行时设 securityContext.fsGroup，让卷可写',
          'ConfigMap 挂载是只读的，要写的目录绝不能挂 ConfigMap',
        ],
        verify: '进容器 ls -l 目标路径能看到文件且当前用户可读',
      },
      {
        id: 'end-entry',
        title: '结论：镜像入口就没跑起来',
        terminal: true,
        detail: '日志完全为空说明容器进程在输出第一行前就退出了：command/args 覆盖了镜像 ENTRYPOINT、'
          + '或入口解释器不存在（比如镜像里没有 bash 却写了 bash -c）。',
        rootCause: 'command/args 配置错误，或镜像入口脚本缺失',
        fix: [
          "kubectl get pod <pod> -o jsonpath='{.spec.containers[*].command}' 看实际覆盖的命令",
          '本地 docker run 同一个镜像，确认镜像本身能起来（排除是镜像问题）',
          '去掉 command/args 让镜像走默认入口，先验证能不能跑',
        ],
        verify: 'Pod 进入 Running 且日志有正常启动输出',
      },
      {
        id: 'end-dep',
        title: '结论：依赖服务没起来',
        terminal: true,
        detail: 'nc 直接被拒绝说明目标端口没人监听：依赖服务挂了、端口写错、或者 Service 的 targetPort 指错。',
        rootCause: '依赖服务未就绪或端口映射错误',
        fix: [
          'kubectl get endpoints <svc> 确认有后端地址（空的说明 selector 没匹配到 Pod）',
          '核对 Service 的 port 与 targetPort，容器里实际监听的是哪个',
          '启动顺序问题加 initContainer 或就绪探针，不要用 sleep 硬等',
        ],
        verify: 'nc -zv 返回 succeeded，且应用日志无连接错误',
      },
      {
        id: 'end-netpol',
        title: '结论：网络策略或 DNS 拦截',
        terminal: true,
        detail: '连接超时（不是拒绝）通常是被丢包：NetworkPolicy 默认拒绝、或 DNS 解析不出来。',
        rootCause: 'NetworkPolicy 阻断 / CoreDNS 异常',
        fix: [
          'kubectl get networkpolicy -n <ns> 看是否有默认 deny',
          '临时用 kubectl exec 测 nslookup <svc> 判断是解析还是连通问题',
          '策略要放行的是 Pod 的标签选择器，不是 IP 段',
        ],
        verify: '同 namespace 的临时 Pod 能连通目标服务',
      },
      {
        id: 'end-probe',
        title: '结论：探针把健康的应用杀掉了',
        terminal: true,
        detail: 'liveness 探针的语义是"不健康就重启"。启动慢的应用必须有 startupProbe，'
          + '否则在它还在初始化时 liveness 就已经超时了。',
        rootCause: '探针参数过紧，缺少 startupProbe',
        fix: [
          '给慢启动应用加 startupProbe（failureThreshold × periodSeconds 覆盖冷启动时间）',
          'liveness 只判"进程是否假死"，别在里面查数据库——依赖抖动会让整批 Pod 集体重启',
          'readiness 才用来摘流量，两者职责不要混',
        ],
        verify: '观察 10 分钟 restartCount 不再增长，Events 无 Unhealthy',
      },
    ],
  },

  /* ------------------------------------------------------------------ 2 */
  {
    slug: 'pod-pending',
    title: 'Pod 一直 Pending 调度不上',
    scene: 'Kubernetes',
    icon: 'server',
    severity: 'medium',
    summary: 'Pod 建出来了但没有节点接收，Events 里写着 0/N nodes are available',
    keywords: ['Pending', '调度', 'unschedulable', '资源不足', '污点', 'PVC'],
    entry: 's1',
    steps: [
      {
        id: 's1',
        title: '看 Events 里调度器给的第一条原因',
        detail: '调度失败的原因会直接写在 Events 里，且通常只有一条主因。先读它，别急着看监控。',
        cmd: 'kubectl describe pod <pod> -n <ns> | sed -n "/Events/,$p"',
        branches: [
          { when: 'insufficient cpu / insufficient memory', goto: 's2' },
          { when: 'node(s) had untolerated taint', goto: 's3' },
          { when: 'unbound immediate PersistentVolumeClaims', goto: 's4' },
          { when: "didn't match Pod's node affinity/selector", goto: 's5' },
        ],
      },
      {
        id: 's2',
        title: '看各节点还剩多少可分配资源',
        detail: '注意看的是 Allocated resources（按 request 算），不是 top 的实时用量。'
          + '调度器只认 request，节点上真实空闲再多，request 占满就调不进去。',
        cmd: 'kubectl describe nodes | grep -A 6 "Allocated resources"',
        branches: [
          { when: '确实 request 占满', goto: 'end-capacity' },
          { when: '节点空闲却仍报不足', goto: 'end-request' },
        ],
      },
      {
        id: 's3',
        title: '列出节点污点，确认是否该容忍',
        detail: '控制面节点默认带 NoSchedule 污点；专用节点（GPU、中间件）通常也打污点。'
          + '先判断这个工作负载该不该上这类节点，再决定加容忍还是换节点池。',
        cmd: 'kubectl get nodes -o custom-columns=NAME:.metadata.name,TAINTS:.spec.taints',
        branches: [
          { when: '是专用节点池，业务确实该跑上去', goto: 'end-toleration' },
          { when: '污点打错了（不该存在）', goto: 'end-taint-remove' },
        ],
      },
      {
        id: 's4',
        title: '看 PVC 卡在哪一步',
        detail: 'Pending 的 Pod 只是结果，真正卡住的是 PVC：没有 StorageClass、容量不够、'
          + '或访问模式与存储不支持（比如 ReadWriteMany 挂到只支持 RWO 的云盘）。',
        cmd: 'kubectl get pvc -n <ns> && kubectl describe pvc <pvc> -n <ns> | sed -n "/Events/,$p"',
        branches: [
          { when: 'ProvisioningFailed / no persistent volumes available', goto: 'end-pvc' },
          { when: 'PVC 已 Bound 但 Pod 仍 Pending', goto: 's5' },
        ],
      },
      {
        id: 's5',
        title: '核对节点选择器与亲和性',
        detail: 'nodeSelector 里写了一个节点上不存在的标签，是最常见的"静默失败"——'
          + '不报错，就是永远调不上。',
        cmd: "kubectl get pod <pod> -o jsonpath='{.spec.nodeSelector}{\"\\n\"}{.spec.affinity}'",
        branches: [
          { when: '标签在节点上不存在', goto: 'end-selector' },
          { when: '亲和性规则互相矛盾', goto: 'end-selector' },
        ],
      },

      {
        id: 'end-capacity',
        title: '结论：集群资源真的不够了',
        terminal: true,
        detail: '所有节点的可分配 request 都满了。这时候扩副本没有意义，只会多几个 Pending。',
        rootCause: '集群容量不足',
        fix: [
          '加节点（集群自动扩缩容要看是否有 pending pod 触发条件）',
          '把 request 从"拍脑袋的 2 核"改成按实际 P95 用量设，虚高的 request 会浪费整台机器',
          '长期低负载的副本先降下来，比加机器便宜',
        ],
        verify: 'Pod 进入 Running，且 kubectl describe node 显示 Allocated 有余量',
      },
      {
        id: 'end-request',
        title: '结论：单 Pod 的 request 超过了任何节点的可分配量',
        terminal: true,
        detail: '比如 request 8 核，但节点总容量 8 核、可分配只有 7.5 核——这种 Pod 永远调不上，'
          + '报错还是"资源不足"，很容易误判成集群没资源。',
        rootCause: 'request 值大于节点最大可分配量',
        fix: [
          '把 request 降到小于单节点可分配值（kubectl describe node 里的 Allocatable）',
          '确实需要这么大的话，用大规格节点池 + nodeAffinity 固定过去',
        ],
        verify: 'kubectl get pod 显示 Running',
      },
      {
        id: 'end-toleration',
        title: '结论：需要给工作负载加容忍',
        terminal: true,
        detail: '容忍只是"允许"调度上去，不等于"一定"上去；配合 nodeAffinity 才能钉在目标节点池。',
        rootCause: '缺少对应的 toleration',
        fix: [
          '按污点的 key/effect 加 tolerations（value 要对上，空值表示匹配任意）',
          '同时加 nodeAffinity 限定节点池，否则可能被调度到别处',
        ],
        verify: 'Pod 调度到目标节点：kubectl get pod -o wide 看 NODE 列',
      },
      {
        id: 'end-taint-remove',
        title: '结论：污点配置有误，去掉它',
        terminal: true,
        detail: '误打的污点会影响整个节点池的所有工作负载，去掉前先确认没有别的东西依赖它做隔离。',
        rootCause: '节点污点误配置',
        fix: [
          'kubectl taint nodes <node> <key>:<effect>- （末尾的减号表示删除）',
          '删完确认原有 Pod 没有被意外驱逐',
        ],
        verify: '目标 Pod 成功调度，节点上其他业务仍正常',
      },
      {
        id: 'end-pvc',
        title: '结论：存储卷创建不出来',
        terminal: true,
        detail: 'PVC 卡在 Pending 说明动态供应失败：没有默认 StorageClass、容量超限、'
          + '或存储插件（CSI）Pod 本身不健康。',
        rootCause: 'StorageClass 缺失 / 容量不足 / CSI 插件异常',
        fix: [
          'kubectl get sc 确认有默认 StorageClass（名字旁有 default 标记）',
          'kubectl -n kube-system get pod | grep csi 看存储插件是否 Running',
          '云盘类存储只支持 ReadWriteOnce，多副本共享要换 NFS/CephFS 这类 RWX 存储',
        ],
        verify: 'PVC 状态 Bound，Pod 随之 Running',
      },
      {
        id: 'end-selector',
        title: '结论：调度约束把人挡在门外',
        terminal: true,
        detail: 'nodeSelector / nodeAffinity / podAntiAffinity 里只要有一条无法满足，Pod 就永远 Pending，'
          + '而报错信息只说"didn\'t match"，不会告诉你是哪一条。',
        rootCause: '节点选择器或亲和性规则与实际节点标签不符',
        fix: [
          'kubectl get nodes --show-labels 核对标签是否存在、拼写是否一致',
          'podAntiAffinity 的 topologyKey 用 hostname 时，副本数不能超过节点数',
          '先用最简单的 nodeSelector 验证能调通，再逐步加复杂规则',
        ],
        verify: 'Pod Running 且落在预期节点',
      },
    ],
  },

  /* ------------------------------------------------------------------ 3 */
  {
    slug: 'nginx-502',
    title: 'Nginx 报 502 / 504',
    scene: 'Nginx',
    icon: 'shield',
    severity: 'high',
    summary: '网关返回 502 Bad Gateway 或 504 Gateway Timeout，用户侧只看到白屏',
    keywords: ['502', '504', 'Bad Gateway', 'Gateway Timeout', 'upstream', '反向代理'],
    entry: 's1',
    steps: [
      {
        id: 's1',
        title: '先分清 502 还是 504',
        detail: '这两个码含义完全不同：502 是 Nginx 连不上后端或后端提前断连，'
          + '504 是连上了但后端在超时时间内没回。搞混了会往完全错误的方向查。',
        cmd: 'tail -200 /var/log/nginx/error.log | grep -E "502|504|upstream"',
        branches: [
          { when: 'connect() failed / Connection refused', goto: 's2' },
          { when: 'upstream timed out (110)', goto: 's3' },
          { when: 'upstream prematurely closed connection', goto: 's4' },
        ],
      },
      {
        id: 's2',
        title: '确认后端端口是否有人监听',
        detail: '在 Nginx 所在机器上测。注意 Nginx 跑在容器里时，127.0.0.1 是容器自己，'
          + '必须用服务名或宿主机 IP——这是容器化后最经典的 502 来源。',
        cmd: 'curl -sv --max-time 3 http://<upstream_host>:<port>/healthz 2>&1 | tail -15',
        branches: [
          { when: 'Connection refused', goto: 'end-down' },
          { when: '能通但浏览器仍 502', goto: 'end-proxy-config' },
          { when: '超时', goto: 'end-firewall' },
        ],
      },
      {
        id: 's3',
        title: '504 要看后端到底慢在哪',
        detail: '先把 upstream 的耗时拆开：$upstream_response_time 是后端真实耗时，'
          + '$request_time 是 Nginx 视角的总耗时。前者接近超时值就是后端慢，不是 Nginx 的问题。',
        cmd: "awk '{print $0}' /var/log/nginx/access.log | grep -o 'upstream_response_time=[0-9.]*' | sort -t= -k2 -rn | head",
        branches: [
          { when: 'upstream 耗时本身就超过 proxy_read_timeout', goto: 'end-slow-app' },
          { when: '后端有慢查询', goto: 'end-slow-db' },
          { when: '耗时正常但仍 504（连接排队）', goto: 's5' },
        ],
      },
      {
        id: 's4',
        title: '后端提前断连：多半是它自己崩了或超了 keepalive',
        detail: '上游进程被 OOM 杀掉、或 worker 超时自己断开，在 Nginx 侧就表现为 premature close。'
          + '这种要去看后端服务的日志，Nginx 里查不出更多。',
        cmd: 'journalctl -u <backend_service> --since "10 min ago" | tail -50',
        branches: [
          { when: '后端有 OOM / 崩溃记录', goto: 'end-backend-crash' },
          { when: '后端日志正常', goto: 'end-keepalive' },
        ],
      },
      {
        id: 's5',
        title: '查 worker 连接数与上游连接池是否打满',
        detail: 'worker_connections 到了上限后新连接会排队而不是立刻失败，表现为"偶发 504"。'
          + '同时看 TIME_WAIT 堆积量，过多说明没开 keepalive 到上游。',
        cmd: 'ss -s && nginx -T 2>/dev/null | grep -E "worker_connections|keepalive"',
        branches: [
          { when: '连接数贴近上限', goto: 'end-capacity' },
          { when: 'TIME_WAIT 数量异常高', goto: 'end-keepalive' },
        ],
      },

      {
        id: 'end-down',
        title: '结论：后端服务没在跑',
        terminal: true,
        detail: '端口没人监听，Nginx 只能返回 502。先让后端活过来，再回头看它为什么挂。',
        rootCause: '后端进程未启动 / 崩溃退出 / 端口配错',
        fix: [
          'systemctl status 或 kubectl get pod 确认进程状态与退出原因',
          '核对后端实际监听端口与 Nginx upstream 配置是否一致',
          '确认监听地址是 0.0.0.0 而不是 127.0.0.1（容器内尤其常见）',
        ],
        verify: 'curl 后端健康检查返回 200，Nginx 侧 502 消失',
      },
      {
        id: 'end-proxy-config',
        title: '结论：代理配置指向了错误的位置',
        terminal: true,
        detail: '上游通了但 Nginx 仍 502，常见于 Host 头没透传、HTTPS 上游配成 http、'
          + '或 upstream 里写的是容器内不可解析的名字。',
        rootCause: 'proxy_pass / upstream 配置错误',
        fix: [
          'proxy_set_header Host $host 与 X-Forwarded-For 要补全',
          '后端是 HTTPS 时 proxy_pass 必须写 https://，且可能要 proxy_ssl_server_name on',
          '容器场景用服务名，并确认 Nginx 容器与后端在同一网络',
        ],
        verify: 'nginx -t 通过并 reload，接口返回 200',
      },
      {
        id: 'end-firewall',
        title: '结论：网络层挡住了',
        terminal: true,
        detail: '连接超时（而非拒绝）一般是包被丢弃：安全组、iptables、或跨 namespace 的策略。',
        rootCause: '安全组 / 防火墙 / NetworkPolicy 阻断',
        fix: [
          '检查云安全组入方向是否放行该端口',
          'iptables -S 与 firewall-cmd --list-all 看本机规则',
          'K8s 环境下确认 NetworkPolicy 允许了 Nginx 所在 namespace',
        ],
        verify: '从 Nginx 机器 nc -zv 后端端口返回 succeeded',
      },
      {
        id: 'end-slow-app',
        title: '结论：后端处理本身太慢',
        terminal: true,
        detail: 'upstream_response_time 逼近超时值，说明后端的业务逻辑或下游依赖慢，'
          + '调大超时只是把 504 变成"更慢的 200"，用户体验一样差。',
        rootCause: '后端性能问题（算法、外部调用、GC 停顿）',
        fix: [
          '先看后端自己的日志与 APM，定位最慢的那个环节',
          '外部调用加超时与降级，别让一个慢接口拖垮整条链路',
          '临时缓解可调大 proxy_read_timeout，但必须同时排期治本',
        ],
        verify: 'P95 响应时间回到基线内，504 归零',
      },
      {
        id: 'end-slow-db',
        title: '结论：慢在数据库',
        terminal: true,
        detail: '应用层看着慢，实际时间花在 SQL 上。这时候查 Nginx 和应用日志都是隔靴搔痒。',
        rootCause: '慢查询 / 缺索引 / 锁等待',
        fix: [
          '开慢查询日志，用 pt-query-digest 找 TOP SQL',
          'EXPLAIN 看是否走了全表扫描，补索引',
          '检查锁等待：information_schema.innodb_trx 里长事务要处理掉',
        ],
        verify: '慢查询数量下降，接口耗时回到基线',
      },
      {
        id: 'end-backend-crash',
        title: '结论：后端被 OOM 或崩溃重启',
        terminal: true,
        detail: '进程被杀时连接立刻断开，Nginx 侧就是 premature close。'
          + '这类故障往往周期性出现（内存缓慢上涨到触发点）。',
        rootCause: '后端进程 OOM / 崩溃',
        fix: [
          'dmesg -T | grep -i "killed process" 确认是否被 OOM Killer 干掉',
          '看内存曲线是否单调上升，是则按内存泄漏处理',
          '容器场景核对 limit 与运行时堆参数',
        ],
        verify: '进程稳定运行超过一个完整业务高峰且内存平稳',
      },
      {
        id: 'end-keepalive',
        title: '结论：连接复用没配好，端口被 TIME_WAIT 吃掉',
        terminal: true,
        detail: '到上游不开 keepalive，每个请求都新建连接，高并发下大量 TIME_WAIT 占满本地端口，'
          + '新连接失败表现为间歇性 502/504。',
        rootCause: 'upstream keepalive 未启用 / 连接池过小',
        fix: [
          'upstream 块里加 keepalive 32; 并在 location 里设 proxy_http_version 1.1 与 proxy_set_header Connection ""',
          '内核参数 net.ipv4.tcp_tw_reuse 可缓解，但不能替代连接复用',
        ],
        verify: 'ss -s 里 TIME_WAIT 明显下降，错误率归零',
      },
      {
        id: 'end-capacity',
        title: '结论：Nginx 自身到瓶颈了',
        terminal: true,
        detail: 'worker_connections × worker_processes 是理论上限，实际还受文件描述符限制。',
        rootCause: '连接数上限或 worker 数不足',
        fix: [
          'worker_connections 调到 10240 起，同时确认 worker_rlimit_nofile 足够',
          'worker_processes 设 auto，跟随 CPU 核数',
          '真到单机极限就上多层 Nginx 或换 LVS/SLB',
        ],
        verify: '压测下错误率归零，CPU 未打满',
      },
    ],
  },

  /* ------------------------------------------------------------------ 4 */
  {
    slug: 'disk-full',
    title: '磁盘写满了',
    scene: 'Linux',
    icon: 'server',
    severity: 'high',
    summary: 'df 显示 100%，服务写日志报 No space left on device',
    keywords: ['磁盘满', 'No space left', 'df', 'inode', 'deleted', '日志'],
    entry: 's1',
    steps: [
      {
        id: 's1',
        title: '确认是哪个挂载点满了',
        detail: '先看 df 的 Use% 与挂载点，别一上来就 du / ——在多挂载点的机器上会白跑几分钟。',
        cmd: "df -hT | awk 'NR==1 || $6==\"/\" || $5+0>=90'",
        branches: [
          { when: '某个分区 Use% 接近 100', goto: 's2' },
          { when: 'Use% 不高但写入仍报 No space', goto: 's4', hint: 'inode 或已删除文件' },
        ],
      },
      {
        id: 's2',
        title: '找占空间最多的目录',
        detail: '-x 表示不跨文件系统，避免把挂载的子盘算进来导致数字对不上。',
        cmd: 'du -xh --max-depth=2 / 2>/dev/null | sort -rh | head -20',
        branches: [
          { when: '是日志目录（/var/log、应用 logs）', goto: 'end-log' },
          { when: '是数据目录（数据库、上传文件）', goto: 'end-data' },
          { when: 'du 加起来远小于 df 的已用量', goto: 's3', hint: '有被删除但仍被占用的文件' },
        ],
      },
      {
        id: 's3',
        title: '找已删除但仍被进程占用的大文件',
        detail: '这是"df 说满了、du 却找不到"的唯一解释：文件被 rm 了但进程还握着 fd，'
          + '空间不会释放，直到进程重启或 fd 关闭。',
        cmd: 'lsof +L1 2>/dev/null | sort -k7 -rn | head -10',
        branches: [
          { when: '找到大文件且指向某进程', goto: 'end-deleted' },
          { when: '没有明显大文件', goto: 's4' },
        ],
      },
      {
        id: 's4',
        title: '检查 inode 是否用尽',
        detail: '小文件极多（比如百万级的 session 文件、缓存碎片）会把 inode 吃光，'
          + '此时磁盘还有空间也写不进去。',
        cmd: 'df -i | awk \'NR==1 || $5+0>=90\'',
        branches: [
          { when: 'IUse% 接近 100', goto: 'end-inode' },
          { when: 'inode 正常', goto: 'end-other' },
        ],
      },

      {
        id: 'end-log',
        title: '结论：日志没有轮转',
        terminal: true,
        detail: '最常见的一类。日志按天写却从不删除，几个月后就吃满整个盘。'
          + '清理只是止血，加轮转才是治本。',
        rootCause: '日志未配置轮转或保留期过长',
        fix: [
          'journalctl --vacuum-size=500M 或 --vacuum-time=7d 清系统日志',
          '应用日志配 logrotate：daily + rotate 7 + compress + copytruncate',
          '容器日志设 max-size/max-file（Docker 默认不限制，能写满宿主盘）',
        ],
        verify: 'df 使用率明显下降，且一周后不再持续上涨',
      },
      {
        id: 'end-deleted',
        title: '结论：文件被删了但空间没释放',
        terminal: true,
        detail: 'rm 只删目录项，进程持有的 fd 还在，内核不会回收数据块。'
          + '所以 df 满了而 du 找不到——这是最容易被误判成"磁盘假满"的情况。',
        rootCause: '进程持有已删除文件的句柄',
        fix: [
          '最稳：重启该进程（会释放 fd）',
          '不能重启时用 : > /proc/<pid>/fd/<n> 清空（注意会影响该进程正在写的数据）',
          '根治：日志用 copytruncate 轮转，避免 "rm 了但没人重开" ',
        ],
        verify: 'df 与实际 du 差距消失，写入恢复正常',
      },
      {
        id: 'end-data',
        title: '结论：数据目录增长超出预期',
        terminal: true,
        detail: '数据库 binlog、上传附件、备份文件是三大增长源。删之前必须确认能删——'
          + 'binlog 删错会直接破坏从库复制。',
        rootCause: '数据/备份文件累积',
        fix: [
          'MySQL：PURGE BINARY LOGS BEFORE 而不是直接 rm',
          '备份文件保留策略：本地 7 天 + 异地归档，别把备份存在同一块盘',
          '长期方案是数据盘独立挂载并做容量告警（80% 就告警，不要等 100%）',
        ],
        verify: '使用率回到 80% 以下并设置监控告警',
      },
      {
        id: 'end-inode',
        title: '结论：inode 用尽',
        terminal: true,
        detail: '常见于缓存目录、邮件队列、session 文件。清空小文件目录要注意量级，'
          + '直接 rm -rf 百万文件会拖死 IO。',
        rootCause: '海量小文件耗尽 inode',
        fix: [
          '定位小文件最多的目录：find /path -type f | wc -l 逐层排查',
          '清理用 find <dir> -type f -delete，分批或加 -print | head 先看',
          '根治：缓存设过期淘汰，或迁移到对象存储',
        ],
        verify: 'df -i 的 IUse% 回落到正常区间',
      },
      {
        id: 'end-other',
        title: '结论：空间被隐藏或预留占用',
        terminal: true,
        detail: '两种情况会让人看走眼：ext4 默认给 root 预留 5%；或挂载点被覆盖'
          + '（目录上又挂了一层，原内容被遮蔽但空间仍占用）。',
        rootCause: '文件系统预留 / 挂载点覆盖 / 配额限制',
        fix: [
          'tune2fs -m 1 /dev/xxx 把预留从 5% 降到 1%（数据盘可做）',
          'mount | grep 目标路径 检查是否有多层挂载',
          '确认没被 quota 或 cgroup 磁盘限制卡住',
        ],
        verify: '写入测试文件成功',
      },
    ],
  },

  /* ------------------------------------------------------------------ 5 */
  {
    slug: 'cpu-high',
    title: 'CPU 使用率飙高',
    scene: 'Linux',
    icon: 'activity',
    severity: 'medium',
    summary: '监控告警 CPU 打满，机器变慢但不知道是哪个进程、哪段代码',
    keywords: ['CPU', 'load', 'load average', '飙高', '火焰图', 'perf', '线程'],
    entry: 's1',
    steps: [
      {
        id: 's1',
        title: '先分清是 CPU 真忙还是在等',
        detail: 'load average 高不等于 CPU 忙：大量 D 状态（IO 等待）也会把 load 拉高。'
          + '先看 us/sy/wa 三个值，us 高才是算力问题。',
        cmd: 'uptime && top -bn1 | head -5',
        branches: [
          { when: 'us 高（用户态）', goto: 's2' },
          { when: 'sy 高（内核态）', goto: 's4' },
          { when: 'wa 高 / load 高但 CPU 不满', goto: 'end-io' },
        ],
      },
      {
        id: 's2',
        title: '定位到具体进程',
        detail: '按 CPU 排序取前 10，注意看的是 %CPU 而不是 %MEM。多核机器上单进程可能超过 100%。',
        cmd: 'ps -eo pid,ppid,user,pcpu,pmem,etime,cmd --sort=-pcpu | head -10',
        branches: [
          { when: '是 Java 进程', goto: 's3' },
          { when: '是数据库进程', goto: 'end-db' },
          { when: '是业务脚本/批处理', goto: 'end-batch' },
        ],
      },
      {
        id: 's3',
        title: 'Java：把最忙的线程翻译成堆栈',
        detail: 'top -Hp 看线程级 CPU，把十进制 tid 转成十六进制，再到 jstack 里搜 nid=0x...，'
          + '就能定位到具体是哪段代码在烧 CPU。',
        cmd: "top -Hp <pid> -bn1 | head -15; printf '%x\\n' <tid>; jstack <pid> | grep -A 20 'nid=0x<hex>'",
        branches: [
          { when: '堆栈停在 GC 线程', goto: 'end-gc' },
          { when: '堆栈停在业务方法', goto: 'end-hotcode' },
          { when: '堆栈反复变化（正常计算）', goto: 'end-capacity' },
        ],
      },
      {
        id: 's4',
        title: 'sy 高：看软中断与上下文切换',
        detail: 'sy 高常见于网络包处理（si 高）、频繁系统调用、或容器 CPU 限流。'
          + '先看是不是单核被打满（软中断不均衡）。',
        cmd: 'mpstat -P ALL 1 3; grep -E "ctxt|intr" /proc/stat',
        branches: [
          { when: '某个核的 %soft 特别高', goto: 'end-softirq' },
          { when: '上下文切换数量异常高', goto: 'end-ctxswitch' },
          { when: '容器内 CPU 被 throttle', goto: 'end-throttle' },
        ],
      },

      {
        id: 'end-hotcode',
        title: '结论：业务代码里有热点',
        terminal: true,
        detail: '堆栈稳定停在同一个业务方法上，说明是真在算（死循环、正则回溯、大对象序列化）。'
          + '下一步是采样出火焰图，而不是继续盯 top。',
        rootCause: '热点代码（循环、正则、序列化、加解密）',
        fix: [
          'async-profiler 采 30 秒火焰图：./profiler.sh -d 30 -f out.html <pid>',
          '重点看火焰图最宽的那一层，通常就是元凶',
          '死循环类问题看线程是否长时间停在同一行（连续三次 jstack 对比）',
        ],
        verify: '优化后同压力下 CPU 下降且 P99 稳定',
      },
      {
        id: 'end-gc',
        title: '结论：GC 太频繁',
        terminal: true,
        detail: 'CPU 烧在 GC 线程上，说明对象分配速率过高或堆太小。此时看 GC 日志比看堆栈有用。',
        rootCause: '堆偏小 / 分配速率高 / GC 参数不当',
        fix: [
          'jstat -gcutil <pid> 1000 看 YGC 频率与 Full GC 次数',
          'Young GC 频繁且 Old 增长慢 → 适当增大堆；Full GC 频繁 → 查内存泄漏',
          'G1 下关注 MaxGCPauseMillis 与 Humongous 对象（大数组直接进老年代）',
        ],
        verify: 'Full GC 频率降到小时级，CPU 回落',
      },
      {
        id: 'end-capacity',
        title: '结论：真的是业务量涨了',
        terminal: true,
        detail: 'CPU 高但堆栈分散、无热点、无 GC 异常——这就是正常的负载增长，'
          + '该做的是扩容，而不是继续找"故障"。',
        rootCause: '容量不足',
        fix: [
          '按当前 QPS 与单核处理能力算需要的副本数，留 30% 余量',
          '扩容前确认瓶颈在 CPU 而不是连接数或下游',
          '有峰谷特征的加 HPA，别 24 小时按峰值配置',
        ],
        verify: '扩容后 CPU 回落到 60% 以下',
      },
      {
        id: 'end-io',
        title: '结论：瓶颈在 IO 不在 CPU',
        terminal: true,
        detail: 'wa 高说明进程在等磁盘。常见于数据库刷盘、大文件拷贝、日志同步写。',
        rootCause: '磁盘 IO 瓶颈',
        fix: [
          'iostat -x 1 看 %util 与 await，确认是哪块盘',
          'iotop -o 找具体进程',
          '日志改异步刷盘、大文件操作错峰、必要时换 NVMe 或加 IO 调度优化',
        ],
        verify: 'iostat 的 %util 降到 70% 以下',
      },
      {
        id: 'end-softirq',
        title: '结论：软中断集中在单核',
        terminal: true,
        detail: '网卡中断默认全落在 CPU0，小包多的场景（比如大量短连接）会把那一个核打满，'
          + '整体 CPU 看着不高但服务已经卡了。',
        rootCause: '网卡中断未做多核分发',
        fix: [
          '开 RPS/RFS：echo f > /sys/class/net/eth0/queues/rx-0/rps_cpus',
          '或配 irqbalance 让中断分散',
          '长期解法是网卡多队列 + RSS',
        ],
        verify: '各核 %soft 分布均匀，单核不再打满',
      },
      {
        id: 'end-ctxswitch',
        title: '结论：上下文切换过多',
        terminal: true,
        detail: '线程数远超核数、锁竞争激烈、或大量短连接都会导致切换风暴。',
        rootCause: '线程/进程数过多或锁竞争',
        fix: [
          'pidstat -w 1 看 cswch/s 与 nvcswch/s',
          '线程池参数与核数匹配，避免几千线程抢几个核',
          '容器场景注意不要把 CPU limit 设得远小于实际需要',
        ],
        verify: 'cswch/s 明显下降，吞吐提升',
      },
      {
        id: 'end-throttle',
        title: '结论：被 cgroup CPU 限流',
        terminal: true,
        detail: '容器 CPU 用满 limit 后会被强制节流，表现是"CPU 只到 limit 就上不去、延迟抖动"，'
          + '而宿主机看整体 CPU 还很闲。',
        rootCause: '容器 CPU limit 过低',
        fix: [
          '读 /sys/fs/cgroup/cpu.stat，nr_throttled 持续增长就是被限流',
          '调高 limits.cpu，或改 request=limit 保证独占',
          'Java 应用注意 ActiveProcessorCount 会按 limit 算，GC 线程数会变少',
        ],
        verify: 'nr_throttled 不再增长，延迟抖动消失',
      },
      {
        id: 'end-db',
        title: '结论：数据库在烧 CPU',
        terminal: true,
        detail: '数据库占满 CPU 通常是慢 SQL 全表扫、或并发连接过多导致上下文切换。',
        rootCause: '慢查询 / 连接数过多',
        fix: [
          'information_schema.processlist 按 time 排序找长查询',
          'EXPLAIN 看执行计划，补索引消除全表扫描',
          '连接池上限对齐数据库 max_connections，别每台机器开 500',
        ],
        verify: '数据库 CPU 回落到 50% 以下，慢查询数下降',
      },
      {
        id: 'end-batch',
        title: '结论：批处理任务与在线业务抢资源',
        terminal: true,
        detail: '报表、备份、数据同步这类任务常被安排在业务高峰，一跑就把 CPU 吃光。',
        rootCause: '批处理未错峰或未限速',
        fix: [
          '错峰到低峰期（凌晨），或改成可中断的分批执行',
          '用 taskset 绑核、或 cpulimit 限速，把在线业务隔离开',
          '长期用独立的批处理节点池',
        ],
        verify: '业务高峰期 CPU 平稳，批处理仍在窗口内完成',
      },
    ],
  },

  /* ------------------------------------------------------------------ 6 */
  {
    slug: 'oom',
    title: '内存不足与 OOM Killer',
    scene: 'Linux',
    icon: 'alert',
    severity: 'high',
    summary: '进程莫名消失，dmesg 里有 Killed process，或容器反复 137 重启',
    keywords: ['OOM', '内存', 'Killed process', 'swap', '137', 'free'],
    entry: 's1',
    steps: [
      {
        id: 's1',
        title: '先确认是不是内核杀的',
        detail: '服务"自己挂了"和"被 OOM Killer 杀了"处理方式完全不同。dmesg 里 Killed process 是最硬的证据。',
        cmd: 'dmesg -T | grep -iE "out of memory|killed process|oom-kill" | tail -20',
        branches: [
          { when: '有 Killed process 记录', goto: 's2' },
          { when: '没有记录但进程消失', goto: 'end-not-oom' },
        ],
      },
      {
        id: 's2',
        title: '看被杀时系统内存的真实状态',
        detail: '关键看 available 而不是 free：页缓存会被回收，free 小很正常，available 才是能给应用用的。',
        cmd: 'free -m && cat /proc/meminfo | grep -E "MemAvailable|Committed_AS|SwapTotal"',
        branches: [
          { when: 'available 极低且 swap 也用满', goto: 'end-real-oom' },
          { when: 'available 尚可但仍被 OOM', goto: 'end-cgroup' },
        ],
      },
      {
        id: 's3',
        title: '找到内存消耗大户',
        detail: '按 RSS 排序。注意 RSS 会把共享库重复计算，容器场景更准的是看 cgroup 的 memory.current。',
        cmd: 'ps -eo pid,user,rss,pmem,cmd --sort=-rss | head -10',
        branches: [
          { when: '某个业务进程 RSS 异常大', goto: 's4' },
          { when: '多个进程各占一部分，总量超标', goto: 'end-capacity' },
        ],
      },
      {
        id: 's4',
        title: 'JVM 类进程：分清堆内还是堆外',
        detail: '容器 OOM 但堆没满，是 JVM 最坑的问题：元空间、直接内存、线程栈都不在 -Xmx 里。',
        cmd: 'jcmd <pid> VM.native_memory summary 2>/dev/null || jstat -gc <pid> 1000 5',
        branches: [
          { when: '堆已接近上限', goto: 'end-heap' },
          { when: '堆正常但进程 RSS 很大', goto: 'end-offheap' },
        ],
      },

      {
        id: 'end-real-oom',
        title: '结论：整机内存真的耗尽',
        terminal: true,
        detail: '物理内存 + swap 都见底，内核必须杀进程自保。这种要么扩容，要么把占用降下来。',
        rootCause: '整机内存容量不足',
        fix: [
          '先止血：重启占用最大的服务释放内存',
          '扩容内存或把服务拆到多台机器',
          '加监控：available < 10% 就告警，不要等 OOM 才被动',
        ],
        verify: 'available 稳定在 20% 以上，无新的 oom-kill 记录',
      },
      {
        id: 'end-cgroup',
        title: '结论：容器 limit 触顶，不是宿主机不够',
        terminal: true,
        detail: 'cgroup v2 下读 memory.max 与 memory.current，容器内存到 limit 就会被 cgroup OOM kill，'
          + '宿主机内存可能还有一大半空闲。',
        rootCause: '容器内存 limit 过小',
        fix: [
          'cat /sys/fs/cgroup/memory.current 与 memory.max 对比',
          '调高 limits.memory，同时保证 requests 合理以便调度',
          'Java 一定设 -XX:MaxRAMPercentage，让它知道 limit 是多少',
        ],
        verify: '容器内存曲线稳定在 limit 的 70% 以下，restartCount 不再增长',
      },
      {
        id: 'end-heap',
        title: '结论：堆内内存不够或泄漏',
        terminal: true,
        detail: '堆逼近上限后频繁 Full GC，最后 OOM。要区分"不够用"和"泄漏"：'
          + '看 GC 后老年代是否回落。',
        rootCause: '堆偏小 / 内存泄漏',
        fix: [
          'GC 后老年代仍占满 → 泄漏，jmap -histo 或 dump 堆分析',
          'GC 后能回落但很快又满 → 分配速率高，增大堆或减少对象创建',
          '别用 -Xmx 直接等于 limit，要留出堆外空间',
        ],
        verify: 'Full GC 后老年代使用率 < 60%，无 OOM',
      },
      {
        id: 'end-offheap',
        title: '结论：堆外内存泄漏',
        terminal: true,
        detail: '堆正常但 RSS 持续增长，典型是 Netty 直接内存、线程栈过多、或 JNI 分配未释放。'
          + '开启 Native Memory Tracking 才能看清。',
        rootCause: '直接内存 / 元空间 / 线程栈超限',
        fix: [
          '启动加 -XX:NativeMemoryTracking=summary，用 jcmd VM.native_memory summary 对比 Internal/Other 段',
          '设 -XX:MaxDirectMemorySize 限制直接内存',
          '-XX:MaxMetaspaceSize 防动态类生成把元空间撑爆',
        ],
        verify: 'RSS 曲线平稳不再单调上升',
      },
      {
        id: 'end-capacity',
        title: '结论：多进程累计超容量',
        terminal: true,
        detail: '没有单个进程异常，但所有服务加起来超过了物理内存，属于容量规划问题。',
        rootCause: '部署密度过高',
        fix: [
          '按内存 request 重新排布，别把大内存服务挤在一台',
          '调整 request 让调度器知道真实需求',
          '长期做容量水位管理（保持在 70% 左右）',
        ],
        verify: '整机内存使用率长期低于 75%',
      },
      {
        id: 'end-not-oom',
        title: '结论：不是内存问题，可能是主动退出或被编排系统驱逐',
        terminal: true,
        detail: '没有 OOM 记录却进程消失，方向要转到：容器被驱赶（节点压力）、健康检查失败重启、'
          + '或代码里调用了 System.exit。',
        rootCause: '驱逐 / 探针失败 / 主动退出',
        fix: [
          'kubectl describe pod 看 Events 是否有 Evicted',
          '查应用日志末尾是否有优雅退出记录',
          '确认部署更新导致的滚动重启（时间点是否与发布吻合）',
        ],
        verify: '进程稳定运行，无异常退出记录',
      },
    ],
  },

  /* ------------------------------------------------------------------ 7 */
  {
    slug: 'mysql-slow',
    title: 'MySQL 慢查询与连接打满',
    scene: 'MySQL',
    icon: 'layers',
    severity: 'high',
    summary: '接口超时、Too many connections、CPU 高但看不出哪条 SQL 慢',
    keywords: ['MySQL', '慢查询', 'Too many connections', 'max_connections', '锁等待', '索引'],
    entry: 's1',
    steps: [
      {
        id: 's1',
        title: '先看连接数与上限',
        detail: '连接打满时新请求直接被拒，应用侧报的是"连接池获取超时"，很容易被误判成应用问题。',
        cmd: "SHOW STATUS LIKE 'Threads_connected'; SHOW STATUS LIKE 'Threads_running'; SHOW VARIABLES LIKE 'max_connections';",
        branches: [
          { when: 'Threads_connected 接近 max_connections', goto: 's2' },
          { when: 'Threads_running 很高（并发查询多）', goto: 's3' },
          { when: '连接数正常但接口慢', goto: 's4' },
        ],
      },
      {
        id: 's2',
        title: '看谁在占连接、占着在干嘛',
        detail: '重点看 Command=Sleep 的连接：那是连接池里闲置但没释放的连接，'
          + '占着名额不干活，是最常见的"连接泄漏"表现。',
        cmd: "SELECT id,user,host,db,command,time,state,LEFT(info,60) AS sql_head FROM information_schema.processlist ORDER BY time DESC LIMIT 30;",
        branches: [
          { when: '大量 Sleep 且 time 很大', goto: 'end-pool-leak' },
          { when: '大量 Query 卡在同一状态', goto: 's3' },
        ],
      },
      {
        id: 's3',
        title: '查锁等待：很多慢其实是排队',
        detail: '一条长事务不提交，后面一堆查询全在等锁，看起来是"所有 SQL 都慢"，实际只有一个元凶。',
        cmd: "SELECT * FROM performance_schema.data_lock_waits\\G",
        branches: [
          { when: '有等待关系（WAITING 指向 BLOCKING）', goto: 'end-lock' },
          { when: '没有锁等待', goto: 's4' },
        ],
      },
      {
        id: 's4',
        title: '开慢日志并按耗时聚合',
        detail: '慢日志才是按 SQL 维度定位的唯一可靠来源。按"总耗时"排序，'
          + '单次 200ms 但每秒 500 次的 SQL 比单次 2 秒的更有杀伤力。',
        cmd: 'pt-query-digest /var/lib/mysql/slow.log | head -40',
        branches: [
          { when: 'TOP SQL 是全表扫描', goto: 'end-index' },
          { when: 'TOP SQL 已是索引查询但仍慢', goto: 'end-bigresult' },
          { when: '慢日志里没有明显慢 SQL', goto: 'end-io' },
        ],
      },
      {
        id: 's5',
        title: '确认执行计划是否走对索引',
        detail: 'EXPLAIN 看 type 与 rows：ALL 是全表扫，index 是全索引扫，range/ref 才是正常。'
          + '还要看 Extra 里有没有 Using filesort / Using temporary。',
        cmd: 'EXPLAIN SELECT ... ; EXPLAIN ANALYZE SELECT ...;',
        branches: [
          { when: 'type=ALL 或 rows 极大', goto: 'end-index' },
          { when: '走了索引但仍 filesort', goto: 'end-bigresult' },
        ],
      },

      {
        id: 'end-pool-leak',
        title: '结论：连接池泄漏或上限配得太小',
        terminal: true,
        detail: 'Sleep 连接长期不释放，通常是应用侧拿了连接没关（异常分支漏了 close），'
          + '或每台机器的池上限相加远超数据库上限。',
        rootCause: '连接泄漏 / 池大小与 max_connections 不匹配',
        fix: [
          '应用侧确认 try-with-resources / defer close 覆盖异常分支',
          '连接池设 maxLifetime 小于数据库 wait_timeout，避免用到被服务端断掉的死连接',
          '全部实例的连接池上限之和 ≤ max_connections × 0.8',
        ],
        verify: 'Threads_connected 稳定在合理水位，Sleep 连接数量下降',
      },
      {
        id: 'end-lock',
        title: '结论：长事务阻塞',
        terminal: true,
        detail: '找到 blocking 的那个事务（通常是忘了 commit 的批处理，或一个大 UPDATE），'
          + '杀掉它能立刻缓解，但要先确认能否回滚。',
        rootCause: '长事务持有行锁',
        fix: [
          'kill <blocking_thread_id> 前先看它在跑什么，确认可中断',
          '应用侧检查事务边界，别把 RPC 调用包在事务里',
          '设 innodb_lock_wait_timeout 让等待方快速失败，而不是一起挂住',
        ],
        verify: 'data_lock_waits 为空，慢查询数量回落',
      },
      {
        id: 'end-index',
        title: '结论：缺索引导致全表扫描',
        terminal: true,
        detail: '这是慢查询最常见的根因。加索引要注意：联合索引的最左前缀、区分度、'
          + '以及别在索引列上做函数运算。',
        fix: [
          '按 WHERE + ORDER BY 的组合建联合索引，把区分度高的列放前面',
          'EXPLAIN 确认 key 命中新索引且 rows 显著下降',
          '大表加索引用 online DDL 或 gh-ost，避免锁表',
        ],
        rootCause: '缺少合适索引',
        verify: 'EXPLAIN 的 rows 降到百级以内，接口 P95 回到基线',
      },
      {
        id: 'end-bigresult',
        title: '结论：结果集太大或排序代价高',
        terminal: true,
        detail: '索引命中了却仍然慢，常见于 SELECT *、深分页（LIMIT 100000,20）、'
          + '或需要临时表排序。',
        rootCause: '大结果集 / 深分页 / filesort',
        fix: [
          '只查需要的列，避免 SELECT *',
          '深分页改游标：WHERE id > 上次最大 id LIMIT 20',
          '排序字段纳入索引，消除 Using filesort',
        ],
        verify: '扫描行数与返回行数接近（比例不再悬殊）',
      },
      {
        id: 'end-io',
        title: '结论：磁盘 IO 或刷盘跟不上',
        terminal: true,
        detail: 'SQL 本身不慢但整体慢，看 innodb 的刷盘与磁盘 util。写入密集时 redo 刷盘会成为瓶颈。',
        rootCause: '磁盘 IO 瓶颈 / 刷盘参数不当',
        fix: [
          'iostat -x 1 看 %util 与 await',
          'innodb_flush_log_at_trx_commit=2 可缓解（牺牲 1 秒持久性，需业务确认）',
          '换 NVMe SSD，或把 binlog 与数据文件分盘',
        ],
        verify: '磁盘 %util < 70%，写入延迟下降',
      },
    ],
  },

  /* ------------------------------------------------------------------ 8 */
  {
    slug: 'dns-fail',
    title: 'DNS 解析失败或解析很慢',
    scene: '网络',
    icon: 'link',
    severity: 'high',
    summary: '服务名解析不出来、偶发 no such host、接口无缘无故多等几秒',
    keywords: ['DNS', 'no such host', 'resolv.conf', 'CoreDNS', 'ndots', '解析'],
    entry: 's1',
    steps: [
      {
        id: 's1',
        title: '先分清解析失败还是解析慢',
        detail: '两者根因完全不同：失败是配置/服务问题，慢通常是超时重试（每次 5 秒，'
          + '两个 nameserver 就是 10 秒），表现为"接口偶发多等 5 秒"。',
        cmd: 'time getent hosts <domain>; cat /etc/resolv.conf',
        branches: [
          { when: '立刻返回失败', goto: 's2' },
          { when: '要等 5 秒或 10 秒才出结果', goto: 's4', hint: '超时重试' },
          { when: '解析成功且很快', goto: 'end-not-dns' },
        ],
      },
      {
        id: 's2',
        title: '确认 nameserver 本身可用',
        detail: '直接对 nameserver 发查询，绕过系统的 search 域与缓存，能立刻分清是'
          + '"DNS 服务挂了"还是"我们配错了域名"。',
        cmd: 'dig @<nameserver_ip> <domain> +short +time=2 +tries=1',
        branches: [
          { when: '直接查也失败', goto: 'end-server-down' },
          { when: '直接查成功但系统解析失败', goto: 's3', hint: 'search 域或 nsswitch' },
        ],
      },
      {
        id: 's3',
        title: '查 search 域与域名拼接',
        detail: 'resolv.conf 里的 search 会让 foo 变成 foo.ns.svc.cluster.local 去查，'
          + '拼接出来不对就解析不了；外部域名加不加尾点也会有差别。',
        cmd: 'cat /etc/resolv.conf; getent hosts <domain>.',
        branches: [
          { when: '带尾点能解析、不带不能', goto: 'end-search' },
          { when: 'search 域里有错条目', goto: 'end-search' },
        ],
      },
      {
        id: 's4',
        title: 'K8s 环境：看 CoreDNS 状态',
        detail: 'CoreDNS 副本不足或某个副本异常时，1/3 的请求会超时重试，'
          + '表现出来就是"偶发慢"，非常难查。',
        cmd: 'kubectl -n kube-system get pod -l k8s-app=kube-dns -o wide; kubectl -n kube-system logs -l k8s-app=kube-dns --tail=50',
        branches: [
          { when: 'CoreDNS 有 Pod 不 Ready 或频繁重启', goto: 'end-coredns' },
          { when: 'CoreDNS 正常但仍有超时', goto: 's5' },
        ],
      },
      {
        id: 's5',
        title: '查 conntrack 与 ndots 放大',
        detail: 'ndots:5 意味着任何短域名都要先试 5 种拼接，外部域名每次多 5 次查询；'
          + 'conntrack 表满则会直接丢 UDP 包，表现为解析超时。',
        cmd: 'cat /proc/sys/net/netfilter/nf_conntrack_count; cat /proc/sys/net/netfilter/nf_conntrack_max; grep ndots /etc/resolv.conf',
        branches: [
          { when: 'conntrack 接近上限', goto: 'end-conntrack' },
          { when: 'ndots 过大且外部域名多', goto: 'end-ndots' },
        ],
      },

      {
        id: 'end-server-down',
        title: '结论：DNS 服务本身不可用',
        terminal: true,
        detail: '连直连 nameserver 都查不到，问题就在 DNS 服务侧，跟客户端无关。',
        rootCause: 'DNS 服务宕机 / 上游不可达 / 被限流',
        fix: [
          '检查 named/dnsmasq/CoreDNS 进程与端口 53',
          '确认上游转发地址可达（dig @上游）',
          '临时切备用 nameserver 止血，再修根本',
        ],
        verify: 'dig @nameserver 正常返回记录',
      },
      {
        id: 'end-search',
        title: '结论：search 域配置导致拼接错误',
        terminal: true,
        detail: '最常见的表现是"ping 短名不行、ping 全名可以"。外部域名末尾加 . 可以跳过 search 拼接。',
        rootCause: 'resolv.conf 的 search / ndots 配置不当',
        fix: [
          '修正 search 域为实际 namespace',
          '外部域名在代码里用绝对域名（带尾点）或减少 ndots',
          'K8s 里给 Pod 设 dnsConfig.options 的 ndots:2',
        ],
        verify: 'getent hosts 短名与全名都能解析',
      },
      {
        id: 'end-coredns',
        title: '结论：集群 DNS 副本不足或异常',
        terminal: true,
        detail: 'CoreDNS 默认 2 副本在节点多、Pod 多时会成为瓶颈；某个副本异常则表现为概率性失败。',
        rootCause: 'CoreDNS 副本不足 / 单副本异常',
        fix: [
          '扩副本：kubectl -n kube-system scale deploy coredns --replicas=4',
          '看 CoreDNS 日志里的 SERVFAIL / 上游超时',
          '给 CoreDNS 加反亲和，避免全挤在同一个节点',
        ],
        verify: '集群内 nslookup 服务名成功率高且延迟稳定',
      },
      {
        id: 'end-conntrack',
        title: '结论：conntrack 表被打满',
        terminal: true,
        detail: '表满后新连接的包被丢弃，UDP 的 DNS 查询尤其明显：超时、重试、再超时。',
        rootCause: 'nf_conntrack 容量不足',
        fix: [
          '临时调大：sysctl -w net.netfilter.nf_conntrack_max=1048576',
          '永久写入 /etc/sysctl.d/',
          '排查是否有短连接风暴（连接不复用）',
        ],
        verify: 'count 稳定在 max 的 50% 以下，解析超时消失',
      },
      {
        id: 'end-ndots',
        title: '结论：ndots 放大导致查询次数暴增',
        terminal: true,
        detail: 'ndots:5 时，一个外部域名要先尝试 5 个内部后缀，全失败了才查原域名，'
          + '每次都可能是 5 秒超时。',
        rootCause: 'ndots 默认值过高',
        fix: [
          'Pod 加 dnsConfig: options: [{name: ndots, value: "2"}]',
          '或代码里统一用带尾点的绝对域名',
          '减少不必要的短名调用',
        ],
        verify: '外部域名首次解析耗时降到 50ms 以内',
      },
      {
        id: 'end-not-dns',
        title: '结论：不是 DNS 问题',
        terminal: true,
        detail: '解析又快又准，问题在别处。回到 502/网络不通的剧本继续查链路。',
        rootCause: '排除 DNS',
        fix: [
          '换用「端口不通」剧本继续分层排查',
          '若表现为慢，去看后端耗时（Nginx 504 剧本）',
        ],
        verify: '定位到真实瓶颈',
      },
    ],
  },

  /* ------------------------------------------------------------------ 9 */
  {
    slug: 'port-unreachable',
    title: '端口不通 / 服务连不上',
    scene: '网络',
    icon: 'link',
    severity: 'high',
    summary: 'telnet 不通、连接超时，但服务看着是起来的',
    keywords: ['端口', '不通', 'telnet', 'connection refused', '超时', '防火墙', '安全组'],
    entry: 's1',
    steps: [
      {
        id: 's1',
        title: '分层排查：先确认本机在监听',
        detail: '永远从最内层往外查：本机监听 → 本机回环 → 同网段 → 跨网段 → 防火墙。'
          + '跳过第一步直接查防火墙，十次有六次是服务压根没起。',
        cmd: 'ss -lntp | grep :<port>; curl -v --max-time 3 telnet://127.0.0.1:<port>',
        branches: [
          { when: '没有监听记录', goto: 'end-not-listen' },
          { when: '监听在 127.0.0.1:port', goto: 'end-bind-local' },
          { when: '本机可通但远程不通', goto: 's2' },
        ],
      },
      {
        id: 's2',
        title: '从客户端看是哪一跳断了',
        detail: 'nc 的三种结果要分清：succeeded（通）、refused（有主机应答但端口没开）、'
          + 'timeout（包被丢弃，通常是防火墙或路由）。',
        cmd: 'nc -zv -w 3 <host> <port>; traceroute -T -p <port> <host>',
        branches: [
          { when: 'connection refused', goto: 'end-refused' },
          { when: 'timeout 无响应', goto: 's3' },
          { when: '能通但应用报错', goto: 'end-app-layer' },
        ],
      },
      {
        id: 's3',
        title: '查本机与云上防火墙',
        detail: '云主机的安全组在机器外部，iptables 看不到；两者都要查，顺序是安全组 → iptables。',
        cmd: 'iptables -S; firewall-cmd --list-all 2>/dev/null; nft list ruleset 2>/dev/null | head -40',
        branches: [
          { when: '本机有 DROP/REJECT 规则', goto: 'end-iptables' },
          { when: '本机规则干净', goto: 's4', hint: '可能是安全组或上游设备' },
        ],
      },
      {
        id: 's4',
        title: '抓包确认包到底有没有到',
        detail: '服务端抓包：只有 SYN 没有 SYN-ACK 说明回程被挡；连 SYN 都没有说明去程就断了。'
          + '这一条能直接把责任范围切一半。',
        cmd: 'tcpdump -ni any port <port> -c 20',
        branches: [
          { when: '看不到任何 SYN', goto: 'end-upstream' },
          { when: '有 SYN 无 SYN-ACK', goto: 'end-return-path' },
          { when: '三次握手完成但应用层没响应', goto: 'end-app-layer' },
        ],
      },
      {
        id: 's5',
        title: 'K8s 场景：看 Service 与 Endpoints',
        detail: 'Service 存在不代表有后端。Endpoints 为空说明 selector 没匹配到任何 Pod，'
          + '此时 ClusterIP 是通不了的（直接拒绝）。',
        cmd: 'kubectl get svc <svc> -n <ns>; kubectl get endpoints <svc> -n <ns> -o wide',
        branches: [
          { when: 'ENDPOINTS 为 <none>', goto: 'end-endpoints' },
          { when: 'Endpoints 有地址但连不上', goto: 'end-k8s-net' },
        ],
      },

      {
        id: 'end-not-listen',
        title: '结论：服务根本没在监听',
        terminal: true,
        detail: '最常见也最容易被忽略。进程可能挂了、启动失败、或配置里换了端口。',
        rootCause: '服务未启动 / 端口配置不一致',
        fix: [
          '看进程与日志：systemctl status / journalctl / 容器日志',
          '核对配置文件里的端口与实际期望是否一致',
          '确认不是启动到一半失败（日志里找 bind: address already in use 之类）',
        ],
        verify: 'ss -lntp 能看到目标端口',
      },
      {
        id: 'end-bind-local',
        title: '结论：只监听了 127.0.0.1',
        terminal: true,
        detail: '本机 curl 通、其他机器不通，八成是绑定了回环地址。容器内尤其常见：'
          + '服务绑 127.0.0.1 时，Nginx 在另一个容器里永远连不上。',
        rootCause: 'bind 地址为 127.0.0.1',
        fix: [
          '改配置绑 0.0.0.0（或 ::），注意安全组/防火墙只放行必要来源',
          '有些框架默认 localhost，要显式设 host',
        ],
        verify: 'ss 显示 *:port 或 0.0.0.0:port',
      },
      {
        id: 'end-refused',
        title: '结论：主机可达但端口没开',
        terminal: true,
        detail: 'refused 说明包到了主机、内核回了 RST，即目标端口没有监听。'
          + '区别于 timeout：那个是包被丢弃。',
        rootCause: '目标端口无监听 / 服务绑在别的端口',
        fix: [
          '到目标机 ss -lntp 对照实际端口',
          '确认服务是否只监听了 IPv6（v6only 时 IPv4 连不上）',
          '中间有端口映射（DNAT）时核对映射表',
        ],
        verify: 'nc -zv 返回 succeeded',
      },
      {
        id: 'end-iptables',
        title: '结论：本机防火墙拦截',
        terminal: true,
        detail: '注意 Docker/K8s 会自己写 iptables 链，手改规则容易与它们冲突，'
          + '改动前先备份规则。',
        rootCause: 'iptables / firewalld 规则阻断',
        fix: [
          'iptables -I INPUT -p tcp --dport <port> -j ACCEPT 先验证是否本机拦截',
          '确认后用持久化方式写入（iptables-save 或 firewall-cmd --permanent）',
          '云环境记得同步安全组',
        ],
        verify: '远程 nc 可通，且重启后规则仍生效',
      },
      {
        id: 'end-upstream',
        title: '结论：包在到达前就被丢弃',
        terminal: true,
        detail: '服务端完全看不到 SYN，问题在去程：安全组、路由、NAT、或中间设备。',
        rootCause: '安全组 / 路由 / 中间设备阻断',
        fix: [
          '云平台安全组入方向放行来源 IP 与端口',
          '检查子网路由表与 NACL',
          '跨 VPC / 跨云场景确认对等连接与路由',
        ],
        verify: '服务端 tcpdump 能看到 SYN',
      },
      {
        id: 'end-return-path',
        title: '结论：回程路径不通',
        terminal: true,
        detail: 'SYN 到了但没有 SYN-ACK，常见于多网卡机器回程走了错误的网关，'
          + '或中间设备只放行了单向。',
        rootCause: '路由不对称 / 回程被拦',
        fix: [
          'ip route get <client_ip> 确认回程走哪个网卡',
          '多网卡场景用策略路由固定回程',
          '确认中间防火墙是双向放行',
        ],
        verify: 'tcpdump 同时看到 SYN 与 SYN-ACK',
      },
      {
        id: 'end-app-layer',
        title: '结论：链路通了，是应用层不认',
        terminal: true,
        detail: 'TCP 能建连但请求无响应或报错，方向转到：协议不匹配（http 打 https 端口）、'
          + 'Host 头不对、TLS 版本不兼容、或应用内部异常。',
        rootCause: '应用层协议/配置问题',
        fix: [
          'curl -v 看完整握手与响应，确认协议对不对',
          'HTTPS 端口用 curl -k https:// 验证',
          '看服务端访问日志有无该请求记录',
        ],
        verify: 'curl 返回预期状态码',
      },
      {
        id: 'end-endpoints',
        title: '结论：Service 没有后端',
        terminal: true,
        detail: 'selector 与 Pod 标签不匹配是主要原因，其次是 Pod 都 NotReady（就绪探针没过）。',
        rootCause: 'selector 不匹配 / 后端 Pod 未就绪',
        fix: [
          'kubectl get pod --show-labels 对照 Service 的 selector',
          'kubectl describe svc 看 Endpoints 为空的原因',
          '检查就绪探针是否通过（未就绪的 Pod 不会进 Endpoints）',
        ],
        verify: 'kubectl get endpoints 显示后端地址',
      },
      {
        id: 'end-k8s-net',
        title: '结论：集群网络插件或策略问题',
        terminal: true,
        detail: 'Endpoints 有地址但连不上，看 CNI 插件状态与 NetworkPolicy。',
        rootCause: 'CNI 异常 / NetworkPolicy 阻断',
        fix: [
          'kubectl -n kube-system get pod | grep -E "calico|flannel|cilium"',
          'kubectl get networkpolicy -A 检查是否有默认拒绝',
          'kubectl exec 到同 namespace 的 Pod 里测，排除源端问题',
        ],
        verify: '集群内 Pod 能访问 Service',
      },
    ],
  },

  /* ----------------------------------------------------------------- 10 */
  {
    slug: 'cert-tls',
    title: 'HTTPS 证书过期 / TLS 握手失败',
    scene: '安全',
    icon: 'key',
    severity: 'high',
    summary: '浏览器报证书无效、curl 报 certificate has expired、部分客户端连不上',
    keywords: ['证书', 'TLS', 'SSL', 'expired', '握手失败', 'certificate', 'SNI'],
    entry: 's1',
    steps: [
      {
        id: 's1',
        title: '先看客户端报的真实错误',
        detail: 'TLS 错误的文案非常具体，直接决定方向：expired 是有效期，'
          + 'unable to get local issuer 是证书链，handshake failure 是协议/套件不匹配。',
        cmd: 'curl -vI https://<domain> 2>&1 | tail -20',
        branches: [
          { when: 'certificate has expired', goto: 's2' },
          { when: 'unable to get local issuer certificate', goto: 'end-chain' },
          { when: 'handshake failure / protocol version', goto: 'end-protocol' },
          { when: 'certificate does not match / hostname', goto: 'end-sni' },
        ],
      },
      {
        id: 's2',
        title: '从外部确认服务端实际下发的证书',
        detail: '必须带 -servername：SNI 场景下，不带 SNI 拿到的可能是默认证书，'
          + '而不是这个域名该有的那张，会得出错误结论。',
        cmd: 'openssl s_client -connect <domain>:443 -servername <domain> </dev/null 2>/dev/null | openssl x509 -noout -subject -issuer -dates',
        branches: [
          { when: '确实已过期', goto: 'end-expired' },
          { when: '未过期但客户端报过期', goto: 'end-clock' },
          { when: '下发的证书域名不对', goto: 'end-sni' },
        ],
      },
      {
        id: 's3',
        title: '核对证书与私钥是否配对',
        detail: '换证书时最容易犯的错：只换了 pem 没换 key，nginx -t 能通过但握手失败。'
          + '两个公钥指纹必须完全一致。',
        cmd: "openssl x509 -in cert.pem -pubkey -noout | openssl sha256; openssl pkey -in priv.key -pubout | openssl sha256",
        branches: [
          { when: '两个指纹不一致', goto: 'end-mismatch' },
          { when: '一致', goto: 'end-reload' },
        ],
      },

      {
        id: 'end-expired',
        title: '结论：证书过期了',
        terminal: true,
        detail: '续签后必须 reload 才生效——很多"续了还是过期"的情况，是文件换了但进程还在用内存里的旧证书。',
        rootCause: '证书到期未续签',
        fix: [
          '续签（acme.sh / certbot / 云厂商签发）',
          '替换文件后 nginx -t && nginx -s reload（容器内 nginx -s reload）',
          '设置到期前 30 天自动续签 + 到期告警，别靠人记',
        ],
        verify: 'openssl s_client 显示的 notAfter 是新日期',
      },
      {
        id: 'end-chain',
        title: '结论：证书链不完整',
        terminal: true,
        detail: '服务器只下发了叶子证书，没带中间证书。桌面浏览器可能自动补，'
          + '但 Java、Android、curl 会直接失败——"我这儿好好的"就是这么来的。',
        rootCause: '缺少中间证书',
        fix: [
          '把 fullchain（叶子 + 中间）一起配置到 ssl_certificate',
          'openssl s_client -showcerts 确认下发了两张以上',
          '中间证书顺序要在叶子之后',
        ],
        verify: 'openssl s_client -verify_return_error 返回 verify return:1',
      },
      {
        id: 'end-protocol',
        title: '结论：协议或加密套件不匹配',
        terminal: true,
        detail: '典型是服务端关了 TLS1.2 以下，而客户端只会 TLS1.0；或套件列表里没有客户端支持的。',
        rootCause: 'TLS 版本 / 加密套件不兼容',
        fix: [
          'nmap --script ssl-enum-ciphers -p 443 <domain> 看服务端支持范围',
          '保留 TLS1.2 兼容老客户端，逐步淘汰',
          '排查是否有中间设备（WAF、负载均衡）只支持旧协议',
        ],
        verify: '目标客户端握手成功',
      },
      {
        id: 'end-sni',
        title: '结论：SNI / 域名不匹配',
        terminal: true,
        detail: '一台机器挂多个域名时，没配 SNI 就会下发默认证书；或者证书 SAN 里没有这个域名。',
        rootCause: 'SNI 未配置 / 证书 SAN 不含该域名',
        fix: [
          'Nginx 每个 server 块配 server_name 与对应证书，开 ssl 的 default_server 也要有证书',
          '确认证书 SAN（Subject Alternative Name）包含实际访问的域名',
          '换证书时把主域名与 www 都加进 SAN',
        ],
        verify: 'openssl s_client -servername <domain> 返回的 subject/SAN 匹配',
      },
      {
        id: 'end-clock',
        title: '结论：客户端时间不对',
        terminal: true,
        detail: '证书未过期但客户端报过期，检查客户端系统时间与时区。容器里时间不同步也常见。',
        rootCause: '客户端时钟偏差',
        fix: [
          'chronyc tracking / timedatectl 确认时间同步',
          '容器挂载宿主 /etc/localtime 或开 NTP',
        ],
        verify: '客户端时间与服务端偏差在秒级内',
      },
      {
        id: 'end-mismatch',
        title: '结论：证书与私钥不配对',
        terminal: true,
        detail: 'nginx -t 只检查文件能读，不校验配对关系，所以这个错误能一路走到线上。',
        rootCause: '证书与私钥来自不同签发批次',
        fix: [
          '重新下载同一批次的证书与私钥',
          '替换后做一次指纹比对（见上一步命令）',
          'reload 后再从外部验证',
        ],
        verify: '两个 sha256 指纹一致且握手成功',
      },
      {
        id: 'end-reload',
        title: '结论：文件换了但进程没重载',
        terminal: true,
        detail: '证书与私钥匹配、文件也更新了，但服务还在用启动时加载进内存的旧证书。',
        rootCause: '未 reload',
        fix: [
          'nginx -t 后 nginx -s reload（不能 restart 的话 reload 是必须的）',
          '容器场景 docker exec <container> nginx -s reload',
          '确认 reload 的是正在跑的那个实例（多实例容易 reload 错）',
        ],
        verify: '外部 s_client 立即显示新证书',
      },
    ],
  },

  /* ----------------------------------------------------------------- 11 */
  {
    slug: 'kafka-lag',
    title: 'Kafka 消费堆积（Lag 持续上涨）',
    scene: '中间件',
    icon: 'layers',
    severity: 'medium',
    summary: '监控显示 consumer lag 一直涨，消息延迟越来越大但消费者进程还活着',
    keywords: ['Kafka', 'lag', '堆积', '消费', 'rebalance', '分区'],
    entry: 's1',
    steps: [
      {
        id: 's1',
        title: '看 lag 是全局涨还是某个分区涨',
        detail: '全局均匀上涨是消费能力不足；单分区暴涨是热点 key 或该分区消费者卡住。'
          + '两者的处理方式完全不同。',
        cmd: 'kafka-consumer-groups.sh --bootstrap-server <broker> --describe --group <group>',
        branches: [
          { when: '所有分区都在涨', goto: 's2' },
          { when: '只有个别分区涨', goto: 'end-hotkey' },
          { when: 'lag 不涨但一直不为 0', goto: 'end-stuck' },
        ],
      },
      {
        id: 's2',
        title: '确认消费者是否在频繁重平衡',
        detail: '重平衡期间整个 group 停止消费。频繁 rebalance（几分钟一次）会让消费能力'
          + '直接掉一半，日志里能看到 Preparing to rebalance。',
        cmd: 'grep -iE "rebalance|Revoking|Joining group" /var/log/<consumer>.log | tail -30',
        branches: [
          { when: '频繁 rebalance', goto: 'end-rebalance' },
          { when: '无 rebalance', goto: 's3' },
        ],
      },
      {
        id: 's3',
        title: '看消费者的处理耗时',
        detail: '消费能力 = 分区数 × 每分区每秒处理条数。处理慢在下游（写库、调接口）时，'
          + '加消费者没用——分区数才是并行度上限。',
        cmd: 'kafka-consumer-groups.sh --describe --group <group> | grep -E "CONSUMER-ID|LAG"; 观察单条处理日志耗时',
        branches: [
          { when: '单条处理耗时明显变长', goto: 'end-slow-consume' },
          { when: '处理正常但条数不够', goto: 's4' },
        ],
      },
      {
        id: 's4',
        title: '核对消费者实例数与分区数',
        detail: '消费者比分区多时，多出来的实例是空转的——这是"加了机器却没提速"的标准答案。',
        cmd: 'kafka-topics.sh --bootstrap-server <broker> --describe --topic <topic> | grep -c PartitionCount',
        branches: [
          { when: '消费者数 ≥ 分区数', goto: 'end-partition-cap' },
          { when: '消费者数 < 分区数', goto: 'end-instance-mismatch' },
        ],
      },

      {
        id: 'end-hotkey',
        title: '结论：单分区热点',
        terminal: true,
        detail: '某个 key 的消息量远超其他，全部落在一个分区里，一个消费者再怎么优化也追不上。',
        rootCause: '分区键分布不均',
        fix: [
          '换分区键（加随机后缀 / 用业务上更均匀的字段）',
          '热点 key 单独走一个 topic 并提高并行度',
          '注意换分区键会打乱同一 key 的顺序性，评估业务是否能接受',
        ],
        verify: '各分区 lag 分布均匀且回落',
      },
      {
        id: 'end-rebalance',
        title: '结论：频繁重平衡吃掉消费能力',
        terminal: true,
        detail: '常见原因：session.timeout.ms 太短而处理一批消息超过它；'
          + '或消费者实例数在弹性伸缩时反复增减。',
        rootCause: '会话超时过短 / 实例反复上下线',
        fix: [
          '调大 max.poll.interval.ms（要大于单批最长处理时间）',
          '用静态成员（group.instance.id）避免滚动重启触发重平衡',
          '把弹性伸缩的冷却时间设长，别一有 lag 就扩容又缩容',
        ],
        verify: '日志中重平衡次数降到小时级，lag 开始下降',
      },
      {
        id: 'end-slow-consume',
        title: '结论：消费逻辑慢',
        terminal: true,
        detail: '瓶颈在下游：慢 SQL、同步调外部接口、或单条处理里有重试。'
          + '加消费者不解决问题，因为并行度受分区数限制。',
        rootCause: '消费逻辑耗时长',
        fix: [
          '批处理 + 批量写库，别一条一条来',
          '外部调用改异步或加超时快速失败',
          '先定位慢在哪：打点记录单条处理各阶段耗时',
        ],
        verify: '单条平均处理耗时下降，lag 开始收敛',
      },
      {
        id: 'end-partition-cap',
        title: '结论：分区数就是并行度上限',
        terminal: true,
        detail: '消费者数量已经等于或超过分区数，再加实例也不会有任何提升。'
          + '扩分区是唯一出路，但要注意分区只能增不能减。',
        rootCause: '分区数不足',
        fix: [
          '扩分区：kafka-topics.sh --alter --partitions <新数量>',
          '扩分区后 key 的映射会变，同 key 的顺序性不再保证',
          '评估下游能否承受更高并发',
        ],
        verify: 'lag 持续下降至接近 0',
      },
      {
        id: 'end-instance-mismatch',
        title: '结论：消费者实例不足',
        terminal: true,
        detail: '分区还有富余却没有消费者来分，直接加实例即可（不超过分区数）。',
        rootCause: '消费者实例数少于分区数',
        fix: [
          '扩容消费者到分区数量级',
          '确认新实例分配到不同分区（describe 看 ASSIGNMENT）',
        ],
        verify: '每个分区都有活跃消费者，lag 下降',
      },
      {
        id: 'end-stuck',
        title: '结论：卡在某条消息上反复重试',
        terminal: true,
        detail: 'lag 不涨是因为没有新消息，但不为 0 说明有 offset 没提交——'
          + '通常是某条消息处理一直失败，被无限重试卡住。',
        rootCause: '毒消息导致消费卡死',
        fix: [
          '看消费者日志里重复出现的同一条消息',
          '加重试上限 + 死信队列，别无限重试',
          '确认业务上能否跳过该消息',
        ],
        verify: 'offset 正常推进，无重复报错',
      },
    ],
  },
];

/** 剧本总数，页面头部与测试断言共用。 */
export const RUNBOOK_COUNT = RUNBOOKS.length;

/**
 * 校验所有分支跳转都指向真实存在的步骤。
 *
 * 数据写错的代价是运维在事故现场点进一个死胡同，所以这里把校验放在渲染前，
 * 出错直接抛——页面白屏远好过给出错误的排查路径。
 */
export function validateRunbooks() {
  RUNBOOKS.forEach((rb) => {
    const ids = new Set(rb.steps.map((s) => s.id));
    if (!ids.has(rb.entry)) {
      throw new Error(`剧本 ${rb.slug} 的 entry 指向不存在的步骤 ${rb.entry}`);
    }
    rb.steps.forEach((s) => {
      if (!s.terminal && (!s.branches || !s.branches.length)) {
        throw new Error(`剧本 ${rb.slug} 的步骤 ${s.id} 既不是结论也没有分支，会走不下去`);
      }
      (s.branches || []).forEach((b) => {
        if (!ids.has(b.goto)) {
          throw new Error(`剧本 ${rb.slug} 的步骤 ${s.id} 分支指向不存在的 ${b.goto}`);
        }
      });
    });
  });
  return true;
}

export const SEVERITY_LABEL = { high: '高危', medium: '中危', low: '低危' };
