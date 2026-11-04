/* ============================================================================
   cheatsheet-data.js — 速查手册数据。

   纯静态数据，不碰网络：速查的价值在于"工位常开、随搜随抄"，
   数据放前端让搜索与过滤完全本地化，零延迟也不吃接口配额。
   新增分类往 SHEETS 里加一项即可，页面与搜索自动跟上。

   条目纪律：cmd 必须是可直接复制执行的写法（不带占位符的尽量不带）；
   desc 一句话说清"什么时候用它"，不写百科式长文——详解是站内文章的职责。
   ========================================================================= */

export const SHEETS = [
  {
    slug: 'linux',
    name: 'Linux 基础',
    groups: [
      {
        name: '文件与目录',
        items: [
          { cmd: 'du -sh * | sort -rh | head -20', desc: '当前目录下谁最占空间（按大小排序取前 20）' },
          { cmd: 'df -hT', desc: '各挂载点容量与文件系统类型' },
          { cmd: 'find / -xdev -size +500M -type f 2>/dev/null', desc: '全盘找超过 500M 的大文件' },
          { cmd: 'lsof +L1', desc: '文件已删除但空间未释放（进程仍持有）的经典排查' },
          { cmd: 'ls -lht --time-style=long-iso', desc: '按修改时间倒序列出文件' },
          { cmd: 'rsync -avzP --partial src/ dst/', desc: '断点续传式拷贝/同步目录' },
          { cmd: 'ln -s /real/path /link/path', desc: '建软链接' },
          { cmd: 'tar -czf backup.tar.gz dir/ && tar -xzf backup.tar.gz -C /tmp', desc: '打包与解包到指定目录' },
        ],
      },
      {
        name: '权限与用户',
        items: [
          { cmd: 'chown -R app:app /data/app && chmod -R 750 /data/app', desc: '递归改属主与权限' },
          { cmd: 'chmod +x script.sh', desc: '给脚本加执行权限' },
          { cmd: 'useradd -m -s /bin/bash deploy && passwd deploy', desc: '新建可登录用户并设密码' },
          { cmd: 'usermod -aG wheel deploy', desc: '把用户加进 sudo 组（wheel）' },
          { cmd: 'visudo', desc: '安全地编辑 sudo 规则（语法错误会拒绝保存）' },
          { cmd: 'last | head -20', desc: '最近的登录记录，安全审计第一眼' },
        ],
      },
      {
        name: '进程与任务',
        items: [
          { cmd: 'ps aux --sort=-%mem | head -10', desc: '内存占用最高的 10 个进程' },
          { cmd: 'ps aux --sort=-%cpu | head -10', desc: 'CPU 占用最高的 10 个进程' },
          { cmd: 'kill -9 $(pgrep -f bad_process)', desc: '按名称批量杀进程' },
          { cmd: 'nohup command > out.log 2>&1 &', desc: '后台运行并记录输出' },
          { cmd: 'crontab -l && crontab -e', desc: '查看/编辑当前用户的定时任务' },
          { cmd: 'timeout 60 slow_command', desc: '限时执行，超时自动杀掉' },
        ],
      },
    ],
  },
  {
    slug: 'text',
    name: '文本三剑客',
    groups: [
      {
        name: 'grep',
        items: [
          { cmd: 'grep -rn "ERROR" /var/log/app/ --include="*.log"', desc: '递归搜关键字并显示行号' },
          { cmd: 'grep -C 5 "panic" app.log', desc: '命中行的前后各 5 行上下文' },
          { cmd: 'grep -v "^#" /etc/ssh/sshd_config | grep -v "^$"', desc: '过滤掉注释与空行，看有效配置' },
          { cmd: 'grep -E "error|fail|timeout" app.log', desc: '多关键词任一命中（扩展正则）' },
          { cmd: 'zgrep "2026-11-03" app.log.2.gz', desc: '直接搜压缩日志，不用先解压' },
        ],
      },
      {
        name: 'awk',
        items: [
          { cmd: 'awk \'{print $1}\' access.log | sort | uniq -c | sort -rn | head', desc: '访问日志里 IP 出现次数 Top N' },
          { cmd: 'awk -F: \'{print $1, $3}\' /etc/passwd', desc: '指定冒号分隔符取第 1、3 列' },
          { cmd: 'awk \'$9>=500 {print $9, $7}\' access.log | sort | uniq -c', desc: '统计 5xx 请求的接口分布' },
          { cmd: 'awk \'END{print NR}\' big.txt', desc: '统计文件总行数（大文件比 wc 慢但可加条件）' },
        ],
      },
      {
        name: 'sed',
        items: [
          { cmd: 'sed -n \'100,120p\' app.log', desc: '只看第 100-120 行' },
          { cmd: 'sed -i \'s/old.example.com/new.example.com/g\' app.yml', desc: '原地批量替换（先备份再执行）' },
          { cmd: 'sed -i \'30a\\server 10.0.0.5 iburst\' /etc/chrony.conf', desc: '在第 30 行后插入一行' },
          { cmd: 'sed -i "/^#PermitRootLogin/d" /etc/ssh/sshd_config', desc: '删除匹配行' },
        ],
      },
    ],
  },
  {
    slug: 'network',
    name: '网络排查',
    groups: [
      {
        name: '连通性',
        items: [
          { cmd: 'curl -svk --connect-timeout 5 https://target:443 -o /dev/null', desc: '看 TLS 握手细节，判断证书/网络问题' },
          { cmd: 'curl -o /dev/null -s -w "DNS:%{time_namelookup} 连接:%{time_connect} 首字节:%{time_starttransfer} 总:%{time_total}\\n" https://site.com', desc: '分段计时定位慢在哪一环' },
          { cmd: 'mtr -rwz target.com', desc: '路由逐跳丢包报告（比 traceroute 信息多）' },
          { cmd: 'ping -c 4 -i 0.3 target && traceroute -n target', desc: '基础连通与路径' },
          { cmd: 'ip route get 10.0.0.5', desc: '查这个目标地址实际走哪张网卡/网关' },
        ],
      },
      {
        name: '端口与连接',
        items: [
          { cmd: 'ss -tlnp | grep :8080', desc: '谁在监听 8080（现代版 netstat）' },
          { cmd: 'ss -s', desc: '连接状态总览，TIME_WAIT 暴涨时先看这个' },
          { cmd: 'ss -tnp state established \'( dport = :3306 )\'', desc: '哪些进程连着 3306' },
          { cmd: 'lsof -i :443 -nP', desc: '按端口反查进程' },
        ],
      },
      {
        name: '抓包',
        items: [
          { cmd: 'tcpdump -ni eth0 port 443 -c 100', desc: '抓 100 个 443 包（-n 不解析域名）' },
          { cmd: 'tcpdump -ni any host 10.0.0.5 and port 3306 -w dump.pcap', desc: '落盘后用 Wireshark 分析' },
          { cmd: 'tcpdump -ni eth0 "tcp[tcpflags] & (tcp-rst) != 0"', desc: '只抓 RST，排查连接被重置' },
          { cmd: 'dig +short example.com @8.8.8.8', desc: '指定 DNS 验证解析结果' },
          { cmd: 'dig +trace example.com', desc: '完整解析链路，判断是哪一级 DNS 的问题' },
        ],
      },
    ],
  },
  {
    slug: 'kubectl',
    name: 'Kubernetes',
    groups: [
      {
        name: '日常操作',
        items: [
          { cmd: 'kubectl get pods -A -o wide | grep -v Running', desc: '全集群找非 Running 状态的 Pod' },
          { cmd: 'kubectl describe pod <pod> -n <ns>', desc: '定位 Pod 异常的第一入口（Events 在最下面）' },
          { cmd: 'kubectl logs -f --tail=200 <pod> -n <ns> [-c <容器>]', desc: '跟日志，多容器 Pod 要指定 -c' },
          { cmd: 'kubectl logs <pod> -n <ns> --previous', desc: '看上一次崩溃实例的日志（CrashLoopBackOff 必看）' },
          { cmd: 'kubectl exec -it <pod> -n <ns> -- sh', desc: '进容器排障（镜像没有 bash 用 sh）' },
          { cmd: 'kubectl get events -A --sort-by=.lastTimestamp | tail -20', desc: '集群最近发生的事件按时间排序' },
          { cmd: 'kubectl top pods -A --sort-by=memory | head', desc: '内存大户 Pod 排行' },
        ],
      },
      {
        name: '排查与发布',
        items: [
          { cmd: 'kubectl rollout restart deployment/<name> -n <ns>', desc: '滚动重启（改了 ConfigMap 后常用）' },
          { cmd: 'kubectl rollout status deployment/<name> -n <ns>', desc: '盯发布进度，卡住立即能看出来' },
          { cmd: 'kubectl rollout undo deployment/<name> -n <ns>', desc: '回滚到上一版' },
          { cmd: 'kubectl get pod <pod> -o jsonpath=\'{.spec.containers[*].images}\'', desc: '查 Pod 实际在跑的镜像' },
          { cmd: 'kubectl auth can-i create deployments --as=system:serviceaccount:ns:sa-name', desc: '检查某服务账号有没有权限' },
          { cmd: 'kubectl drain <node> --ignore-daemonsets --delete-emptydir-data', desc: '腾空节点（维护前必做）' },
          { cmd: 'kubectl get csr | grep Pending', desc: '节点加入集群时证书没批，先看这里' },
        ],
      },
    ],
  },
  {
    slug: 'docker',
    name: 'Docker',
    groups: [
      {
        name: '容器操作',
        items: [
          { cmd: 'docker ps -a --format "table {{.Names}}\\t{{.Status}}\\t{{.Image}}"', desc: '全部容器（含已退出）整齐列出' },
          { cmd: 'docker logs -f --tail 100 --since 30m <容器>', desc: '看最近 30 分钟日志' },
          { cmd: 'docker stats --no-stream', desc: '容器资源占用快照' },
          { cmd: 'docker exec -it <容器> sh', desc: '进容器' },
          { cmd: 'docker inspect --format \'{{.State.ExitCode}} {{.State.Error}}\' <容器>', desc: '查退出码与错误信息' },
          { cmd: 'docker system df', desc: '镜像/容器/卷占了多少磁盘' },
          { cmd: 'docker system prune -af --volumes', desc: '大扫除。⚠️ 会删停止容器与未用卷，生产先确认' },
        ],
      },
      {
        name: '镜像与编排',
        items: [
          { cmd: 'docker build -t app:1.2.0 . && docker save app:1.2.0 | gzip > app.tar.gz', desc: '构建并导出（内网机器没外网时这么搬镜像）' },
          { cmd: 'docker load -i app.tar.gz', desc: '导入镜像包' },
          { cmd: 'docker compose up -d --build', desc: '重建并后台启动整套编排' },
          { cmd: 'docker compose logs -f --tail=100 api', desc: '只看某个服务的日志' },
          { cmd: 'docker compose down && docker compose down -v', desc: '停止；加 -v 连数据卷一起删，⚠️ 不可逆' },
        ],
      },
    ],
  },
  {
    slug: 'systemd',
    name: 'systemd 与日志',
    groups: [
      {
        name: '服务管理',
        items: [
          { cmd: 'systemctl status <svc> --no-pager -l', desc: '服务状态全量输出（不截断）' },
          { cmd: 'systemctl restart <svc> && systemctl enable <svc>', desc: '重启并设置开机自启' },
          { cmd: 'systemctl list-units --state=failed', desc: '列出所有失败的服务' },
          { cmd: 'systemctl daemon-reload', desc: '改过 unit 文件后必须执行，否则不生效' },
          { cmd: 'systemd-analyze verify /etc/systemd/system/app.service', desc: '改完 unit 文件先做语法校验' },
          { cmd: 'systemd-run --unit=build1 --collect --property=Type=oneshot bash build.sh', desc: '把长任务交给 systemd 托管，SSH 断了也照跑' },
        ],
      },
      {
        name: 'journalctl',
        items: [
          { cmd: 'journalctl -u <svc> -f --since "10 min ago"', desc: '跟最近 10 分钟的服务日志' },
          { cmd: 'journalctl -u <svc> --since today -p err', desc: '只看今天的报错级别日志' },
          { cmd: 'journalctl -b -1 -u <svc>', desc: '看上一次开机期间的服务日志（重启后丢现场时用）' },
          { cmd: 'journalctl --disk-usage && journalctl --vacuum-size=1G', desc: '日志占了多少、压到 1G 以内' },
        ],
      },
    ],
  },
  {
    slug: 'nginx',
    name: 'Nginx',
    groups: [
      {
        name: '配置与热更',
        items: [
          { cmd: 'nginx -t && nginx -s reload', desc: '改配置的标准动作：先验证再热加载' },
          { cmd: 'nginx -T | grep -n "server_name"', desc: '把全部生效配置导出来查' },
          { cmd: 'tail -f /var/log/nginx/access.log | grep -v " 200 "', desc: '实时看非 200 请求' },
          { cmd: 'awk \'$9>=500 {print $7}\' access.log | sort | uniq -c | sort -rn | head', desc: '5xx 集中在哪些接口' },
        ],
      },
      {
        name: '性能与限流',
        items: [
          { cmd: 'worker_processes auto;', desc: 'worker 数随 CPU，别手写死 4' },
          { cmd: 'worker_connections 10240;', desc: '单 worker 连接数，配合 worker_rlimit_nofile 用' },
          { cmd: 'limit_req zone=api burst=20 nodelay;', desc: '接口限流：超出直接拒绝不排队' },
          { cmd: 'keepalive_timeout 65; keepalive_requests 1000;', desc: '长连接保活，反向代理场景必调' },
          { cmd: 'client_max_body_size 20m;', desc: '上传被 413 时改这里' },
          { cmd: 'proxy_read_timeout 120s;', desc: '后端慢接口被 504 时改这里' },
        ],
      },
    ],
  },
  {
    slug: 'mysql',
    name: 'MySQL',
    groups: [
      {
        name: '诊断',
        items: [
          { cmd: 'SHOW FULL PROCESSLIST;', desc: '当前所有连接在干什么，长事务/慢查询一眼见' },
          { cmd: 'SELECT * FROM information_schema.innodb_trx ORDER BY trx_started;', desc: '事务列表，锁等待先查这里' },
          { cmd: 'SHOW ENGINE INNODB STATUS\\G', desc: '死锁日志、缓冲池状态全在里面' },
          { cmd: 'KILL <Id>;', desc: '杀掉 PROCESSLIST 里的某条连接' },
          { cmd: 'EXPLAIN SELECT ...\\G', desc: '执行计划，type=ALL 且 rows 巨大就是全表扫' },
        ],
      },
      {
        name: '维护',
        items: [
          { cmd: 'mysqldump -uroot -p --single-transaction --routines db > db_$(date +%F).sql', desc: '在线备份（不锁表，InnoDB 适用）' },
          { cmd: 'mysql -uroot -p db < db_2026-11-03.sql', desc: '恢复' },
          { cmd: 'SHOW VARIABLES LIKE \'%slow%\';', desc: '确认慢日志开关与位置' },
          { cmd: 'SELECT table_name, ROUND(data_length/1024/1024) AS mb FROM information_schema.tables WHERE table_schema=\'db\' ORDER BY data_length DESC LIMIT 10;', desc: '最大的 10 张表' },
        ],
      },
    ],
  },
  {
    slug: 'redis',
    name: 'Redis',
    groups: [
      {
        name: '诊断',
        items: [
          { cmd: 'redis-cli --no-auth-warning -a <pwd> info memory | grep -E "used_memory_human|mem_fragmentation_ratio"', desc: '内存用量与碎片率（>1.5 该整理了）' },
          { cmd: 'redis-cli --no-auth-warning -a <pwd> slowlog get 10', desc: '最近 10 条慢命令' },
          { cmd: 'redis-cli --no-auth-warning -a <pwd> --bigkeys', desc: '扫描大 key（线上用，逐类型采样）' },
          { cmd: 'redis-cli --no-auth-warning -a <pwd> client list | awk \'{print $2}\' | sort | uniq -c | sort -rn | head', desc: '按客户端地址汇总连接数' },
          { cmd: 'redis-cli --no-auth-warning -a <pwd> config set maxmemory-policy allkeys-lru', desc: '在线改淘汰策略' },
        ],
      },
      {
        name: '备份与恢复',
        items: [
          { cmd: 'redis-cli --no-auth-warning -a <pwd> bgsave', desc: '后台落 RDB（不阻塞主线程）' },
          { cmd: 'redis-cli --no-auth-warning -a <pwd> lastsave', desc: '上次 RDB 成功的时间戳' },
          { cmd: 'redis-cli --no-auth-warning -a <pwd> config get dir', desc: 'RDB/AOF 落盘目录，拷备份先确认这里' },
        ],
      },
    ],
  },
  {
    slug: 'perf',
    name: '性能分析',
    groups: [
      {
        name: '系统层',
        items: [
          { cmd: 'top -bcn1 | head -30', desc: '一次性快照，命令行最长者优先' },
          { cmd: 'vmstat 1 5', desc: '看 si/so（swap 抖动）与 r/b（运行/阻塞队列）' },
          { cmd: 'iostat -xz 1 5', desc: '磁盘 util 与 await，%util 长期 100 就是盘顶不住' },
          { cmd: 'pidstat -u -r -d -p <pid> 1 5', desc: '单个进程的 CPU/内存/IO 分维度看' },
          { cmd: 'free -h && cat /proc/meminfo | grep -E "Cached|Available"', desc: '内存总量与可用量（available 才是真能用的）' },
          { cmd: 'strace -cp <pid>', desc: '进程系统调用耗时分布（futex 高=锁竞争）' },
          { cmd: 'dmesg -T | tail -30', desc: '内核最近消息，OOM 杀进程会写在这里' },
        ],
      },
      {
        name: '定位口诀',
        items: [
          { cmd: 'uptime', desc: 'load 远大于核数 → 先看 CPU；配合 vmstat 区分 CPU/IO 等待' },
          { cmd: 'cat /proc/pressure/{cpu,memory,io}', desc: ' PSI 压力指标，modern 内核判断瓶颈最直接的数字' },
          { cmd: 'perf top -p <pid>', desc: '热点函数实时排行' },
        ],
      },
    ],
  },
  {
    slug: 'ssl',
    name: '证书与安全',
    groups: [
      {
        name: '证书',
        items: [
          { cmd: 'echo | openssl s_client -connect site.com:443 -servername site.com 2>/dev/null | openssl x509 -noout -subject -dates', desc: '线上证书还剩几天，续期前必看' },
          { cmd: 'openssl x509 -in cert.pem -noout -dates -fingerprint -sha256', desc: '核对新证书有效期与指纹' },
          { cmd: 'openssl x509 -in cert.pem -pubkey -noout | openssl sha256 && openssl pkey -in cert.key -pubout 2>/dev/null | openssl sha256', desc: '两段哈希一致 → 证书与私钥配对' },
          { cmd: 'openssl req -new -newkey rsa:2048 -nodes -keyout a.key -out a.csr -subj "/CN=site.com"', desc: '生成 CSR 与私钥' },
        ],
      },
      {
        name: 'SSH 加固',
        items: [
          { cmd: 'sshd -T | grep -E "passwordauthentication|permitrootlogin"', desc: '验证 sshd 实际生效的配置（不是文件里写的）' },
          { cmd: 'ssh -vvv user@host', desc: '三层详细日志，认证失败先看卡在哪一步' },
          { cmd: 'ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub', desc: '看主机密钥指纹，防中间人比对用' },
          { cmd: 'fail2ban-client status sshd', desc: '爆破封禁情况' },
        ],
      },
    ],
  },
];
