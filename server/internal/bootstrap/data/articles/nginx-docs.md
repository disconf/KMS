## 文档入口

| 找什么 | 去哪 |
|---|---|
| 全部模块文档（英文） | <https://nginx.org/en/docs/> |
| 指令字母序索引 | <https://nginx.org/en/docs/dirindex.html> |
| 变量索引 | <https://nginx.org/en/docs/varindex.html> |
| 中文翻译（社区） | <https://nginx.org/cn/docs/>（更新滞后，以英文为准） |

## 必读页面清单

按使用频率排序，前四个是日常必查：

1. **ngx_http_proxy_module**——proxy_pass、超时、缓冲全套，反向代理的一切；
2. **ngx_http_upstream_module**——调度策略、健康检查参数（max_fails/fail_timeout）；
3. **ngx_http_rewrite_module**——if、return、rewrite，重定向与路由改写；
4. **ngx_http_ssl_module**——证书与 TLS 参数；
5. **ngx_http_gzip_module / ngx_http_limit_req_module**——压缩与限流，按需查。

## 推荐学习路径

1. **第 1 天**：serve 一个静态目录 + 一条反向代理，理解 server/location 匹配顺序
   （精确 `=` → 前缀 `^~` → 正则 → 普通前缀，正则按文件顺序）；
2. **第 1 周**：配一个双节点 upstream，故意杀掉一个节点观察摘除与恢复；
   把 keepalive 六件套抄进配置，对比 TIME_WAIT 数量；
3. **第 1 个月**：独立完成「HTTPS 入口 + 按路径分流 + 限流 + 日志格式定制」一整套，
   之后遇到新需求基本都是查 dirindex 抄指令的层面了。

## 版本选择与升级文档

- 生产选偶数稳定版，[下载页](https://nginx.org/en/download.html) 标注了 stable/mainline；
- 升级看 [changelog](https://nginx.org/en/CHANGES)，重点搜 `incompatible` 与 `security`；
- 平滑升级可以二进制热替换（`kill -USR2` + `kill -WINCH`），但常规场景 reload 换包就够。

## 社区与求助渠道

- 官方 mailing list 存档：<https://forum.nginx.org/>；
- 提问带上 `nginx -V`（模块列表决定行为差异）、完整 server 块、`error.log` 对应行；
- 中文资料鱼龙混杂，一个自检法：任何博客方案先想「它依赖哪个模块、哪个版本」，
  拿着模块名去官方文档核对参数，再抄不迟。
