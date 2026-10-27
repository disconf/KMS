## 文档入口

OpenResty 的文档分散在三处，找东西之前先分清去哪找：

| 找什么 | 去哪 |
|---|---|
| 安装包、组件清单 | <https://openresty.org/en/> |
| `*_by_lua*` 指令、ngx API | [lua-nginx-module](https://github.com/openresty/lua-nginx-module) 的 README（就是文档本体） |
| Lua 库用法 | 各 `lua-resty-*` 仓库 README + 测试文件 `t/*.t` |

## 必读页面清单

1. **Getting Started**（openresty.org）——半小时过一遍目录结构和一个 hello world；
2. **lua-nginx-module README 的 Directives 章节**——按阶段顺序读一遍指令名，
   不求记住，混个脸熟，用到再回来查；
3. **ngx.tcp / ngx.req API 章节**——cosocket 与请求读写，写业务逻辑的高频区；
4. **`lua-resty-core` 文档**——新版本推荐走 FFI API，性能比传统 `ngx.*` 更好。

## 推荐学习路径

1. **第 1 天**：装好 → `content_by_lua_block` 输出 JSON → 理解请求进来经过哪些阶段；
2. **第 1 周**：用 `lua-resty-redis` 在 access 阶段做一次「查缓存决定是否转发」，
   顺手理解 cosocket 与连接池 `set_keepalive`；
3. **第 1 个月**：实现一个完整小网关（限流 + 鉴权 + 路由改写），这时再回头看
   README 里的 phase 列表，会发现当初不懂的指令都有了归属。

## 版本选择与升级文档

- 用 [最新稳定版](https://openresty.org/en/download.html)，别追 source 主干；
- 升级前看 [changelog](https://openresty.org/en/changelog.html) 里的 **Incompatible Changes** 段，
  OpenResty 升级偶有 Lua API 行为变更；
- 生产升级姿势与 nginx 相同：新版本起一套 → 灰度流量 → 全量，或者直接 `-s reload` 平滑替换。

## 社区与求助渠道

- 报 issue 前先在 [lua-nginx-module issues](https://github.com/openresty/lua-nginx-module/issues) 搜关键字，
  高频坑（如 `lua_code_cache`、子请求语义）都有长讨论；
- 提问模板：nginx -V 输出 + 最小复现配置 + error.log 片段，三样齐了回复率高得多。
