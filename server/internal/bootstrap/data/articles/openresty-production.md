## 动态 upstream：不停机改后端

OpenResty 最大的实战价值之一。用 `lua-resty-core` 配合共享内存或直接上
`ngx_dynamic_upstream`，把节点增删做成 API：

```nginx
location /upstream {
    content_by_lua_block {
        local http = require "resty.http"
        -- 授权后允许运维平台调用，实现「后台点一下，网关摘节点」
    }
}
```

没有现成方案时，退一步用「配置中心生成 conf.d 文件 + `openresty -s reload`」，
reload 是优雅的，毫秒级且不断连接。

## 限流：Lua 版比 limit_req 更灵活

```nginx
init_by_lua_block {
    limit_req = require "resty.limit.req"
    limiter = limit_req.new("my_limit_store", 200, 100)   -- 200 r/s 突发 100
}
access_by_lua_block {
    local delay, err = limiter:incoming("api:" .. ngx.var.binary_remote_addr, true)
    if not delay then
        if err == "rejected" then
            ngx.exit(429)
        end
    end
}
```

按业务维度（用户 ID、API key）限流必须走 Lua，`limit_req_zone` 只能按 IP / header。

## 灰度发布：按 Cookie / Header 分流

```nginx
location / {
    access_by_lua_block {
        if ngx.var.http_x_gray == "true" then
            ngx.var.upstream_uri = "http://gray_backend"
        end
    }
    proxy_pass http://stable_backend;
}
```

测试同事带 `X-Gray: true` 头就进灰度池，别的流量不受影响。

## 生产参数（在裸 nginx 基础上追加）

```nginx
lua_shared_dict cache_store 512m;     # 多 worker 共享的本地缓存
lua_shared_dict limit_store 64m;
lua_socket_log_errors off;
resolver 10.0.0.2 valid=10s;          # Lua 里动态解析域名时必须配
```

`lua_shared_dict` 大小按业务估算，超了写入报 no memory，监控要覆盖。

## 验收清单

- [ ] Lua 代码进 Git，发布走流水线（网关不是 playground）
- [ ] `init_by_lua` 里加载的配置有版本号，`/status` 能看到当前版本
- [ ] 共享内存命中率与 no memory 计数有监控
- [ ] reload 前跑 `resty -e` 语法检查 Lua 脚本
- [ ] 灰度规则有明确的退出路径（灰度池全量回切是一条命令的事）