## 它是什么

OpenResty = **Nginx + LuaJIT + 一批 C 模块**。内核就是 nginx，但在里面嵌了一个 Lua 运行时，
让你能用几行脚本改写请求的每个阶段：改写 URI、查缓存、调后端、限流计数，都不用重新编译 nginx。

## 解决什么问题

纯 nginx 是配置驱动的：location、rewrite、proxy_pass 写在 conf 里，逻辑稍微复杂就得装第三方模块
或者退到应用层。OpenResty 把「每请求的小段逻辑」放进 nginx 进程里跑，收益有三条：

- **快**：LuaJIT 是最快的 Lua 实现，热逻辑跑在 C 级别，比绕一圈到后端服务快一个量级；
- **稳**：跑在 worker 进程内，不引入额外网络跳数，也不会像第三方 C 模块那样牵一发动全身；
- **生态**：lua-resty-* 系列库（redis、mysql、jwt、limit-req…）基本覆盖了网关层常见需求。

## 架构与核心概念

| 概念 | 一句话 |
|---|---|
| master / worker | 与 nginx 相同：master 读配置管 worker，worker 真正处理请求 |
| 阶段（phase） | `init_by_lua`（启动）、`access_by_lua`（进业务前）、`content_by_lua`（出响应）、`log_by_lua`（记日志），脚本挂在阶段上执行 |
| cosocket | Lua 里发起的非阻塞网络调用（`ngx.socket.tcp`），这是「在 nginx 里连 Redis」而不卡住 worker 的基础 |
| 共享内存 zone | `lua_shared_dict` 声明的跨 worker 缓存，限流、计数都靠它 |

理解「阶段 + cosocket + shared dict」三件事，OpenResty 的绝大多数用法都能推出来。

## 版本与生态

- 版本号跟随内置的 nginx 主版本（如 1.25.x），升级 OpenResty 大约等于升级 nginx；
- 上游周边：Kong、APISIX 两个开源网关都构建在 OpenResty 之上，直接用网关也是一条路；
- Lua 库都在 [openresty/lua-resty-*](https://github.com/openresty) 下维护，文档就在各仓库 README。

## 适用与不适用

**适合**：API 网关、WAF、限流认证、边缘缓存、把多个后端粘合成一个入口。
**不适合**：重业务逻辑——Lua 脚本一旦超过几百行、开始有复杂状态，就该沉到真正的应用服务里，
入口层保持薄才好维护。

## 官方文档导读

- 入门与组件列表：<https://openresty.org/en/>
- nginx 指令的 OpenResty 扩展（`*_by_lua*` 全在这）：<https://github.com/openresty/lua-nginx-module>
- Lua 库清单：<https://openresty.org/en/components.html>
