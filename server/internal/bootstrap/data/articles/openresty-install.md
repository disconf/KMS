## OpenResty 与 nginx 的关系

OpenResty = nginx + LuaJIT + lua-nginx-module + 一批 C 模块。安装方式上，
**官方 yum 仓库优先**，源码编译仅当需要定制组件时。已装好裸 nginx 的机器
不必卸载，但两者不能同时监听 80/443。

## 官方仓库安装（CentOS）

```bash
# 加 openresty 仓库（openresty.com 提供 repo 包）
wget https://openresty.org/package/centos/openresty.repo \
  -O /etc/yum.repos.d/openresty.repo
yum install -y openresty
systemctl enable --now openresty

# 注意：服务名是 openresty 不是 nginx，二进制在 /usr/local/openresty/nginx/sbin/
openresty -v
```

## 源码编译

```bash
# OpenResty 的 configure 会把自己的一套模块全部编进去，参数比裸 nginx 简单
./configure --prefix=/usr/local/openresty \
  --with-http_ssl_module --with-luajit
make -j$(nproc) && make install
```

## 目录约定（与裸 nginx 不同，先记牢）

```bash
/usr/local/openresty/nginx/conf/nginx.conf   # 主配置
/usr/local/openresty/nginx/conf/conf.d/      # 业务配置（自己 include）
/usr/local/openresty/lualib/                 # Lua 库搜索路径
/usr/local/openresty/site/lualib/            # 自研 Lua 代码放这里
```

## 容器化

```bash
docker run -d --name openresty \
  -p 80:80 -p 443:443 \
  -v /opt/openresty/conf.d:/etc/nginx/conf.d:ro \
  -v /opt/openresty/lua:/usr/local/openresty/site/lualib:ro \
  openresty/openresty:alpine
```

## 第一个 Lua 接口（验证 LuaJIT 生效）

```nginx
location /hello {
    content_by_lua_block {
        ngx.say("hello from openresty, ", ngx.worker.pid())
    }
}
```

`curl http://127.0.0.1/hello` 有输出即 Lua 执行链路通了。

## 与裸 nginx 的选型

| 需求 | 选择 |
|---|---|
| 纯反代 / 静态资源 | 裸 nginx，简单可控 |
| 网关逻辑：鉴权、限流、动态路由 | OpenResty（Lua 在请求阶段内嵌执行，无额外跳数） |
| 已有 WAF / API 网关产品 | 它们底层多是 OpenResty，直接用产品 |

选了 OpenResty 就要接受它的纪律：**Lua 代码也要进版本管理、也要 code review**，
网关层的坑会放大到全站。