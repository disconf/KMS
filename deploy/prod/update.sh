#!/usr/bin/env bash
# 日常更新：拉最新代码 → 重建镜像 → 滚动重启。
# 数据在 named volume 里，重建不影响。
set -euo pipefail
cd "$(dirname "$0")"

cd ../.. && git pull --ff-only && cd deploy/prod

docker compose build api
docker compose up -d api
docker compose ps
