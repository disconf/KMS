## OpenResty 的命令就是 nginx 的命令

服务名不同、路径不同，语义完全一致：

```bash
openresty -t                 # 校验配置（等价 nginx -t）
openresty -s reload          # 优雅重载
openresty -s quit            # 优雅停止
openresty -V                 # 版本与编译参数（确认 LuaJIT 已启用）
systemctl reload openresty   # systemd 方式优先
```

## resty 命令行：Lua 脚本的快速验证

```bash
# 单行验证（不用起服务）
resty -e 'ngx.say("lua ok, jit=", jit.version)'

# 跑一个脚本文件，可访问共享内存等 OpenResty API
resty --main-conf 'lua_shared_dict t 1m;' -e '
  local d = ngx.shared.t
  d:set("k", "v")
  ngx.say(d:get("k"))'
```

上线前用 `resty` 把 Lua 语法跑一遍，比 reload 后看 error.log 高效得多。

## 日志与排障

```bash
tail -f /usr/local/openresty/nginx/logs/error.log     # Lua 报错都在这里
grep -c "lua error" /usr/local/openresty/nginx/logs/error.log
ss -s | head -3                                       # 连接概况
```

Lua 阶段速查（排障时按请求顺序定位是哪一段出的问题）：
`init_by_lua`（启动）→ `ssl_certificate_by_lua` → `access_by_lua`（鉴权限流）→
`content_by_lua`（响应）→ `log_by_lua`（日志埋点）。

## 共享内存巡检

```lua
-- 放一个内部 location，或用 log_by_lua 定期上报
local d = ngx.shared.cache_store
ngx.say("used=", d:capacity() - d:free_space(), " capacity=", d:capacity())
```

## 组合拳：一次灰度切换的完整操作

```bash
# 1. 校验新 Lua 与配置
resty -e 'assert(loadfile("/opt/lua/router.lua")); print("syntax ok")'
openresty -t

# 2. 记录当前连接数作为基线
ss -ntp state established '( sport = :443 )' | wc -l

# 3. 优雅重载并观察错误日志
openresty -s reload
tail -f /usr/local/openresty/nginx/logs/error.log &

# 4. 回切 = 把 Git 里的配置 revert 后再来一遍 1-3
```

## 排障心法

1. **先看 error.log 再猜**：Lua 的运行时错误（attempt to index nil 等）都会带阶段与行号；
2. **区分配置错和代码错**：`openresty -t` 过了但 500，问题在 Lua；
3. **内存类问题看 shared dict**：no memory / 频繁 eviction 说明容量规划要重做。