#!/usr/bin/env bash
# 生产环境首次部署 / 重复部署（幂等）。
#
# 做五件事：
#   1. 生成 .env（已有则沿用，绝不覆盖手工改过的口令）
#   2. 从 config.template.yaml 渲染 config.yaml（已有则沿用）
#   3. 构建应用镜像
#   4. 起 MySQL/Redis，等健康后跑一次 init（建表 + 种子，幂等）
#   5. 起全部服务；宿主机 nginx 关闭（80/443 由容器接管）
#
# 用法：sudo bash bootstrap.sh
set -euo pipefail

cd "$(dirname "$0")"
DOMAIN_DEFAULT="www.jinzhiqiang.com"

# ---------------------------------------------------------------- .env --
if [ ! -f .env ]; then
  cp .env.example .env
  sed -i "s|^MYSQL_ROOT_PASSWORD=.*|MYSQL_ROOT_PASSWORD=$(openssl rand -hex 16)|" .env
  sed -i "s|^MYSQL_APP_PASSWORD=.*|MYSQL_APP_PASSWORD=$(openssl rand -hex 16)|" .env
  sed -i "s|^JWT_SECRET=.*|JWT_SECRET=$(openssl rand -hex 32)|" .env
  sed -i "s|^SIGN_SECRET=.*|SIGN_SECRET=$(openssl rand -hex 32)|" .env
  echo "[env] 已生成 .env（含随机密钥）"
else
  echo "[env] .env 已存在，沿用其中配置"
fi

# shellcheck disable=SC1091
set -a; source .env; set +a

# 管理员口令：生产必须显式提供。这是种子阶段写入的第一口令，
# 不给随机值的话公网上等于用 Admin@12345 开门。
if [ -z "${SEED_ADMIN_PASSWORD:-}" ]; then
  SEED_ADMIN_PASSWORD="$(openssl rand -base64 15 | tr -d '/+=' | cut -c1-16)A1"
  sed -i "s|^SEED_ADMIN_PASSWORD=.*|SEED_ADMIN_PASSWORD=${SEED_ADMIN_PASSWORD}|" .env
  echo "[env] SEED_ADMIN_PASSWORD 未设置，已随机生成并写回 .env"
fi

# ------------------------------------------------------------- config --
if [ ! -f config.yaml ]; then
  sed -e "s|__DOMAIN__|${APP_DOMAIN:-$DOMAIN_DEFAULT}|g" \
      -e "s|__MYSQL_APP_PASSWORD__|${MYSQL_APP_PASSWORD}|g" \
      -e "s|__MYSQL_DATABASE__|${MYSQL_DATABASE:-kms}|g" \
      -e "s|__JWT_SECRET__|${JWT_SECRET}|g" \
      -e "s|__SIGN_SECRET__|${SIGN_SECRET}|g" \
      config.template.yaml > config.yaml
  chmod 600 config.yaml
  echo "[config] 已渲染 config.yaml"
else
  echo "[config] config.yaml 已存在，沿用（手工修改不会丢）"
fi

# -------------------------------------------------------------- build --
echo "[build] 构建应用镜像（首次约 3-5 分钟）"
docker compose build api

# ----------------------------------------------------------- database --
echo "[db] 启动 MySQL / Redis 并等待健康"
docker compose up -d mysql redis
for i in $(seq 1 60); do
  st="$(docker inspect --format '{{.State.Health.Status}}' kms-mysql 2>/dev/null || echo starting)"
  [ "$st" = "healthy" ] && break
  sleep 3
done
[ "$(docker inspect --format '{{.State.Health.Status}}' kms-mysql 2>/dev/null || echo none)" = "healthy" ] \
  || { echo "[db] MySQL 未在期限内健康"; docker compose logs mysql --tail 30; exit 1; }

# 应用账号 kms：MySQL 镜像只在数据目录为空时执行初始化 SQL，
# 所以这里用一次性容器补建（幂等：已存在则跳过建语句、只确保权限）。
docker compose exec -T mysql mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" <<SQL || true
CREATE DATABASE IF NOT EXISTS \`${MYSQL_DATABASE:-kms}\` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'kms'@'%' IDENTIFIED BY '${MYSQL_APP_PASSWORD}';
GRANT ALL PRIVILEGES ON \`${MYSQL_DATABASE:-kms}\`.* TO 'kms'@'%';
FLUSH PRIVILEGES;
SQL

echo "[db] 初始化表结构与种子数据（幂等，可重复执行）"
docker compose run --rm init

# ---------------------------------------------------------------- up ---
echo "[up] 启动全部服务"
docker compose up -d

# 宿主机 nginx 若在跑会抢占 80/443；证书目录仍归它管，只是监听交出来。
systemctl disable --now nginx >/dev/null 2>&1 || true

docker compose ps
echo
echo "=============================================================="
echo " 管理员账号：13800000000"
echo " 初始密码：${SEED_ADMIN_PASSWORD}"
echo " 首次登录后请立即在页面右上角（钥匙图标）修改密码，"
echo " 然后把 .env 里的 SEED_ADMIN_PASSWORD 清空。"
echo "=============================================================="
