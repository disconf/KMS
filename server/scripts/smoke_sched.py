#!/usr/bin/env python3
"""定时任务端到端验证：支付超时自动关单、已付订单不被误伤、到期自动确认并结算。

这个套件与 kb / order 的不同之处：它断言的是"时间过去了之后系统自己做了什么"，
所以服务必须以**很短的周期与很短的超时**启动，否则一条断言要等 15 分钟。
smoke.sh 的 sched 套件已经固定了这组环境变量：

    SCHEDULER_INTERVAL=1s          # 扫描周期压到 1 秒
    PAYMENT_PAY_TIMEOUT=4s         # 待支付有效期 4 秒
    PAYMENT_AUTO_CONFIRM_AFTER=6s  # 完成到自动确认 6 秒

为什么值得单独一个套件而不是塞进 smoke_order.py：smoke_order 的断言全部是
"我发一个请求，看它立刻返回什么"，而这里的关键是"我什么都不做，系统会不会自己动"。
两者的失败含义完全不同——后者挂了通常意味着有人把钱或名额卡住了，需要立刻处理。

用法（由 smoke.sh 调用，不要直接跑）：
    SCHEDULER_INTERVAL=1s PAYMENT_PAY_TIMEOUT=4s PAYMENT_AUTO_CONFIRM_AFTER=6s \
        ./bin/api.exe > sched_api.log 2>&1 &
    python scripts/smoke_sched.py sched_api.log      # 必须有日志路径：要验证调度器确实起来了
"""
import json
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

BASE = "http://localhost:8080/api/v1"

if len(sys.argv) < 2:
    raise SystemExit("必须把服务日志路径作为第一个参数传进来（要验证调度器确实启动）")
LOG = Path(sys.argv[1])

results = []


def check(cond, label=""):
    """记录一条断言。label 说明这条断的是哪条业务规则。"""
    ok = bool(cond)
    results.append(ok)
    print(f"     [{'OK ' if ok else 'FAIL'}] {label}")
    return ok


def _send(method, path, body, token):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def call(method, path, body=None, token=None, expect=None, label=""):
    code, raw = _send(method, path, body, token)
    try:
        payload = json.loads(raw)
    except (json.JSONDecodeError, UnicodeDecodeError):
        payload = {"_raw": raw[:300]}
    ok = expect is None or code == expect
    results.append(ok)
    tag = f" {label}" if label else ""
    extra = "" if ok else f"  want={expect} body={json.dumps(payload, ensure_ascii=False)[:200]}"
    print(f"[{'OK ' if ok else 'FAIL'}] {method:6} {path[:56]:56} -> HTTP {code}{tag}{extra}")
    return code, payload


def soft_call(method, path, body=None, token=None, label=""):
    """只发请求、不计断言。用于获取验证码（撞限流是正常保护，不算失败）。"""
    code, raw = _send(method, path, body, token)
    try:
        payload = json.loads(raw)
    except (json.JSONDecodeError, UnicodeDecodeError):
        payload = {"_raw": raw[:300]}
    print(f"[{'warn' if code >= 300 else 'OK  '}] {method:6} {path[:56]:56} -> HTTP {code} {label}")
    return code, payload


def login(phone, nickname=None):
    for attempt in range(2):
        code, p = soft_call("POST", "/auth/sms/send", {"phone": phone, "scene": "login"},
                            label=f"获取验证码 {phone}")
        if code == 200:
            dev = (p.get("data") or {}).get("devCode")
            if dev:
                break
        print("     ... 撞上发送间隔限制，等待 61s 后重试")
        time.sleep(61)
    else:
        raise SystemExit(f"取不到验证码：{phone}")

    body = {"phone": phone, "code": dev}
    if nickname:
        body["nickname"] = nickname
    _, p = call("POST", "/auth/login/sms", body, expect=200, label=f"登录 {phone}")
    return p["data"]["token"]["accessToken"], p["data"]["user"]


def first_skill_id():
    """取一个技能 ID。悬赏的 skillIds 是 required,min=1，空数组会被 400 挡回来。"""
    _, p = call("GET", "/skills", expect=200, label="技能字典")
    items = p["data"]["list"]
    # 字典里的 id 是数字，而 skillIds 收字符串数组——两侧类型不一致，显式转换。
    return str(items[0]["id"] if isinstance(items[0], dict) else items[0])


def create_question(token, title, skill_id, budget=20000):
    _, p = call("POST", "/questions", {
        "title": title,
        "description": "定时任务验证用的悬赏，描述需要够长以通过校验，这里补一句根因待定位。",
        "skillIds": [skill_id],
        "budgetCents": budget,
        "expectResponseMin": 30,
        "expireHours": 72,
    }, token=token, expect=201, label=f"发布悬赏《{title}》")
    return p["data"]["orderNo"]


def order_status(order_no, token):
    """静默查一次订单，**不记断言**。

    它会按轮询次数被反复调用。如果记成断言，总数就会随网络快慢浮动——
    而"断言总数每次都一样"是这套脚本的硬约束，比多一条断言重要得多。
    """
    code, raw = _send("GET", f"/orders/{order_no}", None, token)
    if code != 200:
        raise SystemExit(f"查询订单 {order_no} 失败: HTTP {code} {raw[:200]}")
    try:
        return json.loads(raw)["data"]
    except (json.JSONDecodeError, KeyError, TypeError):
        raise SystemExit(f"查询订单 {order_no} 返回结构异常: {raw[:200]}")


def wait_status(order_no, token, want, timeout_sec):
    """轮询到目标状态。返回 (payload, 实际耗时)。

    用轮询而不是 sleep 固定时长：调度器的扫描周期与系统的调度抖动都会让
    "刚好等 N 秒"变得不可靠——等短了会误判成"没生效"，等长了整套冒烟白白变慢。
    """
    started = time.time()
    last = None
    while time.time() - started < timeout_sec:
        last = order_status(order_no, token)
        if last["status"] == want:
            elapsed = time.time() - started
            print(f"     订单 {order_no} 在 {elapsed:.1f}s 后变为 {want}")
            return last, elapsed
        time.sleep(0.5)
    print(f"     订单 {order_no} 等了 {timeout_sec}s 仍是 {last['status'] if last else '?'}，期望 {want}")
    return last, time.time() - started


print("--- 0. 调度器确实启动了 ---")
# 先看日志，再验行为：只看行为的话，"调度器在跑"和"别的东西刚好改了状态"
# 这两种情况分不开，而它们的排查方向完全不同。
log_text = LOG.read_text(encoding="utf-8", errors="ignore")
check("msg=定时任务已启动" in log_text, "启动日志里有『定时任务已启动』")
for job in ("cancel-expired-payments", "auto-confirm-expired", "settle-completed"):
    check(job in log_text, f"注册了任务 {job}")
check("定时任务未启用" not in log_text, "scheduler.enabled 没有被关掉")

print("--- 1. 登录 ---")
user_token, user = login("13800007777", "运维老王")
expert_token, expert = login("13900000001")
skill_id = first_skill_id()
print(f"     下单人 id={user['id']} 专家 id={expert['id']} 技能 {skill_id!r}")

print("--- 2. 待支付超时：应被自动关单 ---")
no_a = create_question(user_token, "超时关单验证：节点 NotReady", skill_id)
_, p = call("POST", f"/orders/{no_a}/grab", token=expert_token, expect=200, label="专家抢单")
check(p["data"]["status"] == "pending_pay", "抢单后进入待支付")
check(bool(p.get("data", {}).get("payExpireAt")), "抢单同时写入支付截止时间")

# 立刻查一次：4 秒的窗口里不该被提前关掉。这条挡的是"WHERE 条件写错，
# 把还没到期的单也扫进来"这类错误——它会让用户点开支付页就发现订单已取消。
first = order_status(no_a, user_token)
check(first["status"] == "pending_pay", "刚抢到的单不会被立刻关掉（截止时间还没到）")

a, waited = wait_status(no_a, user_token, "cancelled", 25)
check(a["status"] == "cancelled", "支付超时后订单被自动取消")
check("超时" in (a.get("cancelReason") or ""), "详情里说明了取消原因是超时（用户要知道为什么没了）")
tl = a.get("timeline") or []
check(bool(tl) and tl[-1]["status"] == "cancelled", "时间轴末尾记录了取消节点")

# 关单是一次性动作：再等几轮，状态与取消原因都不应再变。
time.sleep(3)
again = order_status(no_a, user_token)
check(again["status"] == "cancelled" and again.get("cancelReason") == a.get("cancelReason"),
      "再跑几轮调度器也不会重复处理同一单（幂等）")

_, p = call("GET", "/orders/hall", token=expert_token, expect=200, label="接单大厅")
hall_nos = [h["orderNo"] for h in (p["data"].get("list") or [])]
check(no_a not in hall_nos, "已取消的悬赏不会回到大厅（否则会被别人抢走一笔死单）")

print("--- 3. 已支付订单不应被超时任务误伤 ---")
no_b = create_question(user_token, "自动确认验证：磁盘告警", skill_id)
_, p = call("POST", f"/orders/{no_b}/grab", token=expert_token, expect=200, label="专家抢单")
check(p["data"]["status"] == "pending_pay", "第二单进入待支付")
_, p = call("POST", f"/orders/{no_b}/pay", {"channel": "mock"}, token=user_token,
            expect=200, label="mock 支付")
check(p["data"]["paid"] is True, "支付即时到账")

# 等过 pay_timeout：关单任务的 WHERE 若只按时间过滤而不看状态，这一单就会被误取消。
time.sleep(6)
b = order_status(no_b, user_token)
check(b["status"] == "in_service", "已支付的订单在超时点之后仍是服务中（关单只认 pending_pay）")

print("--- 4. 到期自动确认并结算 ---")
_, p = call("GET", "/expert/wallet", token=expert_token, expect=200, label="专家钱包（结算前）")
before = p["data"]["balanceCents"]

call("POST", f"/orders/{no_b}/finish", token=expert_token, expect=200, label="专家标记完成")
pending = order_status(no_b, user_token)
check(pending["status"] == "pending_confirm", "完成后进入待确认")
check(bool(pending.get("confirmDeadline")), "待确认订单带自动确认截止时间")

# 还没到截止时间：手动触发也不能提前确认。这条挡的是"自动确认不看 deadline"，
# 那会让用户失去在期限内提出异议的机会。
time.sleep(1)
still = order_status(no_b, user_token)
check(still["status"] == "pending_confirm", "未到截止时间不会被提前自动确认")

settled, waited = wait_status(no_b, user_token, "settled", 30)
check(settled["status"] == "settled", "到期后自动确认并结算，状态为 settled")
check(bool(settled.get("settlement")), "详情里带分账凭证")
settlement = settled.get("settlement") or {}
check(settlement.get("expertAmountCents") == settled["expertAmountCents"],
      "分账里的专家实收与订单一致")

_, p = call("GET", "/expert/wallet", token=expert_token, expect=200, label="专家钱包（结算后）")
after = p["data"]["balanceCents"]
print(f"     余额 {before} -> {after}（+{after - before}），期望 +{settled['expertAmountCents']}")
check(after - before == settled["expertAmountCents"],
      "自动确认后钱真的到了专家账上（这是这条链路唯一不可出错的地方）")

print("--- 5. 未指派的悬赏不该被关单任务碰 ---")
no_c = create_question(user_token, "不接单就该一直挂着", skill_id)
time.sleep(6)
c = order_status(no_c, user_token)
check(c["status"] == "pending_accept",
      "待接单的悬赏不会被支付超时任务取消（它没有支付截止时间，过期由 expireHours 管）")

ok = sum(1 for r in results if r)
total = len(results)
print()
print(f"===== 定时任务冒烟结果：{ok}/{total} 通过 =====")
if ok != total:
    print("!! 失败项见上方 [FAIL] 行与服务日志 " + str(LOG))
sys.exit(0 if ok == total else 1)
