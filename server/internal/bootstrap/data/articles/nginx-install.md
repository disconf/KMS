## 两个 nginx 是不一样的

发行版仓库里的 nginx（`yum install nginx`）版本老、模块固定；官方源 `nginx.org/packages`
跟主线版本。**有模块定制需求就走编译，纯反代用官方源足够**。OpenResty 则是
「nginx + LuaJIT + 一堆 C 模块」的发行版，需要写 Lua 逻辑时才选它，别为了用而用。

## 官方源安装（CentOS）

```bash
cat > /etc/yum.repos.d/nginx.repo <<EOF
[nginx-stable]
baseurl=http://nginx.org/packages/centos/\$releasever/\$basearch/
enabled=1
gpgcheck=1
gpgkey=https://nginx.org/keys/nginx_signing.key
EOF
yum install -y nginx && systemctl enable --now nginx
nginx -v
```

## 编译安装（要第三方模块时）

```bash
# 依赖
yum install -y gcc make pcre-devel zlib-devel openssl-devel

# 编译参数的原则：只留需要的模块。每多一个模块，升级时就要多对齐一次
./configure \
  --prefix=/usr/local/nginx \
  --with-http_ssl_module \
  --with-http_v2_module \
  --with-http_stub_status_module \
  --with-threads
make -j$(nproc) && make install

# 平滑升级：只换二进制不重启（先备份旧的）
cp /usr/local/nginx/sbin/nginx /usr/local/nginx/sbin/nginx.old
make install                       # 新二进制落位
kill -USR2 $(cat /usr/local/nginx/logs/nginx.pid)   # 起新进程
kill -WINCH <旧worker.pid>                          # 老连接放完再退
```

## Docker 方式

```bash
docker run -d --name nginx --restart unless-stopped \
  -p 80:80 -p 443:443 \
  -v /etc/nginx/conf.d:/etc/nginx/conf.d:ro \
  -v /data/www:/usr/share/nginx/html:ro \
  -v /etc/nginx/certs:/etc/nginx/certs:ro \
  nginx:1.27-alpine
```

容器版配置校验：`docker exec nginx nginx -t`，重载：`docker exec nginx nginx -s reload`。

## 离线安装

官方源机器上 `yumdownloader --resolve nginx`，拷到内网 `rpm -Uvh nginx-*.rpm`。
编译版则把 configure 好的 make 产物 `make install` 到相同 prefix 打包搬运。

## 装完验证

```bash
nginx -t                          # 配置语法
curl -sI http://127.0.0.1 | head -3   # 默认页 200
ss -lntp | grep -E ':80|:443'     # 端口在听
```

目录约定要固定下来：`conf.d/` 放业务配置、`certs/` 放证书、日志统一 `/var/log/nginx/`，
多实例或多团队共管时这三条是秩序的基础。