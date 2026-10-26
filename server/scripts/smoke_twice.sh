#!/usr/bin/env bash
# 连跑两趟冒烟，每趟都用全新的库，比对断言总数是否一致。
#
# 为什么需要它：断言总数必须每次都一样。曾经有一条断言挂在「随机抽一道题」的结果上，
# 抽到多选题时它静默不跑，于是总数在 168/169 之间随抽签浮动——看着像"少验了一条"，
# 但盯着单次输出永远查不出原因。**跑两趟比总数**是发现这类问题的唯一办法。
#
# 一趟 = smoke.sh（它自己会给每个套件起一个新的库与服务进程），所以这里只负责
# 把两趟的输出并排摆出来。
#
# 用法（在 server/ 目录下）：
#   bash scripts/smoke_twice.sh                      # 用 bin/api.exe
#   BIN=./bin/api.exe bash scripts/smoke_twice.sh     # 同上，显式指定
set -u

cd "$(dirname "$0")/.." || exit 1
ROOT=$(pwd)
PORT=${PORT:-8080}
SUITES="kb order sched"

# 与 smoke.sh 同一套识别逻辑：Windows 是 bin/api.exe，Makefile 的 build 目标是 bin/api。
# 这里解析一次并把结果显式传给 smoke.sh，两趟用的是同一个二进制。
if [ -z "${BIN:-}" ]; then
  for cand in "$ROOT/bin/api.exe" "$ROOT/bin/api"; do
    if [ -x "$cand" ]; then BIN="$cand"; break; fi
  done
  BIN=${BIN:-"$ROOT/bin/api.exe"}
fi

# 不要用 mktemp：Git Bash 下它返回的是 C:\Users\... 这种带反斜杠的 Windows 路径，
# 拼到 "$DB-shm" 上传给 rm 时会被判成"路径里嵌了盘符"而拒绝删除，
# 第二趟运行就会带着上一趟的库跑、断言全错。用项目内的固定目录，路径全是正斜杠。
WORK="$ROOT/.smoke-run"
rm -rf "$WORK"
mkdir -p "$WORK"

echo "工作目录: $WORK"
echo "可执行文件: $BIN"
[ -x "$BIN" ] || { echo "找不到 $BIN，先跑：go build -o bin/api.exe ./cmd/api"; exit 1; }

run_pass() {
  tag=$1
  echo
  echo "############ PASS $tag ############"
  BIN="$BIN" PORT="$PORT" WORK="$WORK/$tag" bash scripts/smoke.sh > "$WORK/pass_$tag.txt" 2>&1
  rc=$?
  grep -E "^\s*\[FAIL\]|^=====" "$WORK/pass_$tag.txt"
  echo "exit=$rc"
  return $rc
}

total_of() {
  # 一个套件的输出末尾是 "===== xx/yy 通过 ====="，取那一个分数
  grep -hoE '[0-9]+/[0-9]+' "$WORK/$1/$2.txt" 2>/dev/null | tail -1
}

run_pass a; rc_a=$?
run_pass b; rc_b=$?

echo
echo "===== 两趟对比 ====="
stable=1
for suite in $SUITES; do
  a=$(total_of a "$suite")
  b=$(total_of b "$suite")
  if [ -z "$a" ] || [ "$a" != "$b" ]; then stable=0; fi
  printf '  %-6s a=%-10s b=%-10s %s\n' "$suite" "${a:-缺失}" "${b:-缺失}" \
    "$([ -n "$a" ] && [ "$a" = "$b" ] && echo 稳定 || echo 不稳定)"
done

if [ "$rc_a" = 0 ] && [ "$rc_b" = 0 ] && [ "$stable" = 1 ]; then
  echo "RESULT: 两趟断言总数一致，全绿"
  rm -rf "$WORK"
  exit 0
fi
echo "RESULT: FAILED（a rc=$rc_a / b rc=$rc_b）"
echo "完整输出保留在 $WORK，别删"
exit 1
