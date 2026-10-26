#!/usr/bin/env bash
# 一趟端到端冒烟：知识库 + 订单 + 定时任务，每个套件各配一个**全新的库与全新的服务进程**。
#
# 为什么要一个套件一个进程：前两个脚本都要求跑在全新库上，且都要登录
# 13800007777 / 13800000000 这几个固定号码。共用一个服务进程时，第二个脚本的
# 验证码请求会撞上「同一号码 60 秒只能发一次」的保护——白等一分钟，还容易被
# 误判成环境坏了。分开起进程，限流计数随进程一起归零。
#
# 第三个套件（定时任务）除了要新进程，还必须用**很短的周期与超时**启动：
# 断言的是"时间过去之后系统自己做了什么"，用默认的 15m/48h 一条都验不了。
#
# 用法（在 server/ 目录下）：
#   go build -o bin/api.exe ./cmd/api
#   bash scripts/smoke.sh
#   BIN=./bin/api.exe PORT=8080 bash scripts/smoke.sh
#   WORK=/tmp/smoke bash scripts/smoke.sh          # 换工作目录（smoke_twice.sh 会用）
#
# 失败时保留 $WORK 下的完整输出与服务日志，直接看 $WORK/<套件>.txt。
set -u

cd "$(dirname "$0")/.." || exit 1
ROOT=$(pwd)
PORT=${PORT:-8080}
WORK=${WORK:-"$ROOT/.smoke-run"}
PYTHON=${PYTHON:-python}

# 二进制名在各平台不一样（Windows 是 api.exe），Makefile 的 build 目标是 bin/api。
# 自己找一遍，省得每次都要显式传 BIN。
if [ -z "${BIN:-}" ]; then
  for cand in "$ROOT/bin/api.exe" "$ROOT/bin/api"; do
    if [ -x "$cand" ]; then BIN="$cand"; break; fi
  done
  BIN=${BIN:-"$ROOT/bin/api.exe"}
fi

[ -x "$BIN" ] || { echo "找不到 $BIN，先跑：go build -o bin/api.exe ./cmd/api"; exit 1; }

rm -rf "$WORK"
mkdir -p "$WORK"

# run_suite <套件名> <脚本> [额外的 环境变量=值 ...]
# 起库起服务、跑脚本、收服务，返回脚本的退出码。
run_suite() {
  name=$1
  script=$2
  shift 2
  db="$WORK/$name.db"
  log="$WORK/${name}_api.log"
  out="$WORK/$name.txt"
  rm -f "$db" "$db-shm" "$db-wal" "$log"

  ( env DB_SQLITE_PATH="$db" WEB_DIR="$ROOT/web/static" "$@" "$BIN" > "$log" 2>&1 ) &
  srv=$!

  up=0
  for _ in $(seq 1 40); do
    if curl -s -m 2 "http://localhost:$PORT/healthz" >/dev/null 2>&1; then up=1; break; fi
    sleep 1
  done
  if [ "$up" != 1 ]; then
    echo ">>> $name：服务未起来，日志尾部"
    tail -20 "$log"
    kill "$srv" 2>/dev/null
    return 1
  fi

  echo
  echo "############ $name（库 $name.db）############"
  "$PYTHON" -u "scripts/$script" "$log" > "$out" 2>&1
  rc=$?

  kill "$srv" 2>/dev/null
  wait "$srv" 2>/dev/null
  sleep 2                      # 让端口彻底释放，下一个套件才绑得上

  # 只回显汇总行与失败项，完整输出留在 $out
  grep -E "^\s*\[FAIL\]|^=====|^!!|Traceback" "$out" || echo "  （$name 没有汇总行输出，检查 $out）"
  return $rc
}

rc=0
# kb 套件关掉调度器：资讯抓取任务 run_on_start 会在服务启动几秒内把订阅源的内容写进
# ops-news 栏目，而套件里有一条断言假设「资讯当期为空」（第 5 节，tutorial/news 计数）。
# 抓取快慢取决于外网，抓到了这条就变红——曾出现 177/178 与 178/178 交替，
# 看起来像"少验了一条"，实际是后台任务抢在断言前面写了库。
run_suite kb smoke_kb.py SCHEDULER_ENABLED=false || rc=1
run_suite order smoke_order.py || rc=1
run_suite sched smoke_sched.py \
  SCHEDULER_INTERVAL=1s \
  PAYMENT_PAY_TIMEOUT=4s \
  PAYMENT_AUTO_CONFIRM_AFTER=6s || rc=1

echo
if [ "$rc" = 0 ]; then
  echo "===== 冒烟全绿 ====="
else
  echo "===== 冒烟有失败，完整输出见 $WORK ====="
fi
exit $rc

