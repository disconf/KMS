## 配置文件在哪

| 安装方式 | 主配置 |
|---|---|
| yum/apt（OpenResty） | `/usr/local/openresty/nginx/conf/nginx.conf` |
| 源码编译 | `--prefix` 下的 `conf/nginx.conf` |
| 容器 | 镜像内 `/usr/local/openresty/nginx/conf/nginx.conf`，用挂载覆盖 |

nginx 的惯例是主配置里 `include conf.d/*.conf`，业务配置按域名一文件放 `conf.d/`，
别把所有 server 块堆在主配置里。

## 参数怎么读

nginx 指令作用于**层级**：`http` → `server` → `location`，内层未显式配置的指令继承外层。
看别人配置时先认层级，再认指令，顺序反了会误读很多「为什么不生效」。

## 核心参数分组

**进程与连接（全局）**

```nginx
worker_processes auto;          # 等于 CPU 核数，别手写数字
worker_rlimit_nofile 65535;     # 每个 worker 能开的文件描述符上限
events {
    worker_connections 10240;   # 每个 worker 的并发连接数，与 ulimit 联动
    use epoll;                  # Linux 上显式指定
}
```

**超时三件套（http/server）**——出问题最多的就是这三个：

```nginx
client_body_timeout   15s;      # 客户端发请求体的间隔超时
proxy_connect_timeout 5s;       # 连后端的握手超时，必须短
proxy_read_timeout    60s;      # 两次读后端响应的间隔，长轮询接口要单独调大
```

**OpenResty 专属**：

```nginx
lua_shared_dict limits 10m;             # 共享内存区，限流计数用
lua_code_cache on;                      # 生产必须 on；off 会每次请求重读脚本
init_worker_by_lua_block { ... }        # 定时任务、连接池预热挂这里
```

## 常见误配

1. **改了 conf 不 reload 就测**——先 `nginx -t` 再 `-s reload`；
2. **`worker_connections` 大于系统 ulimit**，报 `worker_connections are not enough` 或 accept 失败；
3. **`lua_code_cache off` 带上生产**，QPS 一下来还以为是被打了；
4. **超时层层继承看错层级**：server 里改了 `proxy_read_timeout`，location 里又写了一遍，
   生效的是 location 那份。

## 校验与生效

```bash
openresty -t                 # 语法检查，改配置后必跑
openresty -s reload          # 平滑重载，worker 逐个替换，不断连接
tail -f logs/error.log       # reload 失败与 Lua 运行时报错都在这里
```
