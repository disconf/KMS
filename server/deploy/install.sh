#!/usr/bin/env bash
# 运维专家服务平台 - 服务器安装/升级脚本
#
# 在**服务器上**以 root 执行。幂等：第一次装、以后升级都跑同一条命令。
#
#   ./install.sh --binary ./ops-expert-api                    # 首次安装（test 环境，SQLite）
#   ./install.sh --binary ./ops-expert-api                    # 升级：只换二进制并重启
#   ./install.sh --binary ./ops-expert-api --env prod --sms-provider aliyun
#
# 设计取舍（改之前先读）：
#   1. 配置文件**只在首次生成**。升级时重渲染会把线上手改过的值冲掉，
#      而"我明明改过配置，重启又变回去了"是最难查的一类问题。
#   2. 密钥由服务器本地 openssl 生成，不经过仓库、不经过聊天记录。
#   3. 二进制先落 .new 再 mv。直接 install 覆盖正在运行的文件会报 Text file busy。
#   4. 进程以 ops-expert 专用账号运行（非 root）。它只写 /opt/ops-expert/data，
#      被拿下时损失有限——这个服务会存手机号、身份证号和收款账号，值得这一步。
set -euo pipefail

APP_NAME="ops-expert"
APP_DIR="/opt/ops-expert"
SERVICE="${APP_NAME}.service"
RUN_USER="opsexpert"

BINARY=""
ENV_NAME="test"
PORT="8080"
BASE_URL=""
DB_DRIVER="sqlite"
MYSQL_DSN=""
REDIS_ADDR=""
SMS_PROVIDER="mock"
SMS_EXPOSE_CODE="true"
PAY_PROVIDER="mock"
WECHAT_MOCK="true"

usage() {
  sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
  cat <<'EOF'

参数：
  --binary PATH         必填，linux/amd64 可执行文件
  --env NAME            dev|test|prod（默认 test；prod 会禁用 mock 短信/支付）
  --port N              监听端口（默认 8080）
  --base-url URL        对外访问地址（默认 http://<本机IP>:<port>）
  --db-driver NAME      sqlite|mysql（默认 sqlite）
  --mysql-dsn DSN       当 db-driver=mysql 时必填
  --redis-addr HOST:PORT 留空=进程内缓存
  --sms-provider NAME   mock|aliyun|tencent|webhook
  --sms-expose-code BOOL mock 下是否把验证码回给前端（prod 强制 false）
  --pay-provider NAME   mock|offline|wechat|alipay
  --wechat-mock BOOL    prod 强制 false
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary) BINARY="${2:-}"; shift 2 ;;
    --env) ENV_NAME="${2:-}"; shift 2 ;;
    --port) PORT="${2:-}"; shift 2 ;;
    --base-url) BASE_URL="${2:-}"; shift 2 ;;
    --db-driver) DB_DRIVER="${2:-}"; shift 2 ;;
    --mysql-dsn) MYSQL_DSN="${2:-}"; shift 2 ;;
    --redis-addr) REDIS_ADDR="${2:-}"; shift 2 ;;
    --sms-provider) SMS_PROVIDER="${2:-}"; shift 2 ;;
    --sms-expose-code) SMS_EXPOSE_CODE="${2:-}"; shift 2 ;;
    --pay-provider) PAY_PROVIDER="${2:-}"; shift 2 ;;
    --wechat-mock) WECHAT_MOCK="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知参数: $1" >&2; usage >&2; exit 2 ;;
  esac
done

[[ -n "$BINARY" ]] || { echo "缺少 --binary" >&2; exit 2; }
[[ -f "$BINARY" ]] || { echo "找不到文件: $BINARY" >&2; exit 2; }
[[ $EUID -eq 0 ]] || { echo "请用 root 执行" >&2; exit 2; }

if [[ -z "$BASE_URL" ]]; then
  ip=$(hostname -I 2>/dev/null | awk '{print $1}')
  BASE_URL="http://${ip:-127.0.0.1}:${PORT}"
fi

# prod 不允许"看起来能跑但其实谁都能登录"的组合，提前收敛掉，
# 不要等应用启动时才被 Validate 拦下来（那时已经装了一半）。
if [[ "$ENV_NAME" == "prod" ]]; then
  SMS_EXPOSE_CODE="false"
  WECHAT_MOCK="false"
  if [[ "$SMS_PROVIDER" == "mock" ]]; then
    echo "prod 环境不能用 mock 短信：没人能收到验证码。请配 --sms-provider（并使用真实通道凭据）" >&2
    exit 2
  fi
  if [[ "$PAY_PROVIDER" == "mock" ]]; then
    echo "prod 环境不能用 mock 支付：点支付会把订单直接置为已支付。请用 --pay-provider offline" >&2
    exit 2
  fi
fi

echo "==> 1/6 创建运行账号与目录"
if ! id -u "$RUN_USER" >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /sbin/nologin "$RUN_USER"
  echo "    已创建系统账号 $RUN_USER"
fi
install -d -m 0755 "$APP_DIR" "$APP_DIR/bin" "$APP_DIR/configs" "$APP_DIR/deploy"
install -d -m 0750 -o "$RUN_USER" -g "$RUN_USER" "$APP_DIR/data" "$APP_DIR/logs"

echo "==> 2/6 安装二进制"
# 先写 .new 再 mv：目标文件正在被进程映射时，直接覆盖会 Text file busy
install -m 0755 "$BINARY" "$APP_DIR/bin/${APP_NAME}-api.new"
mv -f "$APP_DIR/bin/${APP_NAME}-api.new" "$APP_DIR/bin/${APP_NAME}-api"
BIN_VERSION=$("$APP_DIR/bin/${APP_NAME}-api" -version 2>/dev/null || echo unknown)

echo "==> 3/6 渲染配置"
CONF="$APP_DIR/configs/config.yaml"
if [[ -f "$CONF" ]]; then
  echo "    已存在，保留原配置（不覆盖线上改动）"
else
  [[ -f "$APP_DIR/deploy/config.template.yaml" ]] || { echo "缺少 $APP_DIR/deploy/config.template.yaml" >&2; exit 2; }
  [[ "$DB_DRIVER" == "mysql" && -z "$MYSQL_DSN" ]] && { echo "--db-driver=mysql 需要 --mysql-dsn" >&2; exit 2; }

  JWT_SECRET=$(openssl rand -hex 32)
  SIGN_SECRET=$(openssl rand -hex 32)

  # 替换串里可能有 & | \，sed 会当成特殊语法，必须先转义
  esc() { printf '%s' "$1" | sed -e 's/[\\&|]/\\&/g'; }

  sed -e "s|__ENV__|$(esc "$ENV_NAME")|g" \
      -e "s|__PORT__|$(esc "$PORT")|g" \
      -e "s|__BASE_URL__|$(esc "$BASE_URL")|g" \
      -e "s|__DB_DRIVER__|$(esc "$DB_DRIVER")|g" \
      -e "s|__MYSQL_DSN__|$(esc "$MYSQL_DSN")|g" \
      -e "s|__REDIS_ADDR__|$(esc "$REDIS_ADDR")|g" \
      -e "s|__SMS_PROVIDER__|$(esc "$SMS_PROVIDER")|g" \
      -e "s|__SMS_EXPOSE_CODE__|$(esc "$SMS_EXPOSE_CODE")|g" \
      -e "s|__PAY_PROVIDER__|$(esc "$PAY_PROVIDER")|g" \
      -e "s|__WECHAT_MOCK__|$(esc "$WECHAT_MOCK")|g" \
      -e "s|__JWT_SECRET__|$(esc "$JWT_SECRET")|g" \
      -e "s|__SIGN_SECRET__|$(esc "$SIGN_SECRET")|g" \
      "$APP_DIR/deploy/config.template.yaml" > "$CONF"
  # 640 root:opsexpert：进程能读、同机其他账号读不到（里面有 JWT 与文件签名密钥）
  chown root:"$RUN_USER" "$CONF"
  chmod 0640 "$CONF"
  echo "    已生成 $CONF（密钥随机生成，未写入任何仓库）"
fi

echo "==> 4/6 安装 systemd 单元"
install -m 0644 "$APP_DIR/deploy/${APP_NAME}.service" "/etc/systemd/system/${SERVICE}"
systemctl daemon-reload
systemctl enable "$SERVICE" >/dev/null 2>&1 || true

echo "==> 5/6 启动"
systemctl restart "$SERVICE"

echo "==> 6/6 健康检查"
ok=0
for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null 2>&1; then ok=1; break; fi
  if ! systemctl is-active --quiet "$SERVICE"; then break; fi
  sleep 1
done
if [[ $ok -ne 1 ]]; then
  echo "启动失败，最近日志：" >&2
  journalctl -u "$SERVICE" -n 40 --no-pager >&2 || true
  exit 1
fi
wait_ready=0
for _ in $(seq 1 20); do
  if curl -fsS "http://127.0.0.1:${PORT}/readyz" >/dev/null 2>&1; then wait_ready=1; break; fi
  sleep 1
done

echo
echo "================= 部署完成 ================="
echo "版本      : $BIN_VERSION"
echo "访问地址  : $BASE_URL"
echo "运行账号  : $RUN_USER"
echo "配置      : $CONF"
echo "数据目录  : $APP_DIR/data"
echo "healthz   : $(curl -s "http://127.0.0.1:${PORT}/healthz" || echo 无响应)"
echo "readyz    : $([[ $wait_ready -eq 1 ]] && echo 就绪 || echo 未就绪（检查数据库）)"
echo "日志      : journalctl -u $SERVICE -f"
echo "============================================"
