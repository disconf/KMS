#!/usr/bin/env python3
"""订单模块端到端冒烟：悬赏 → 抢单 → 支付 → 履约 → 确认 → 结算 → 评价 → 争议。

与 smoke_kb.py 同一套口径：每条断言都在守一条业务规则，跑完打印 `xx/xx 通过`，
有失败以非 0 退出。**必须跑在全新的数据库上**——多处断言依赖种子数据的确定状态：
  - 两个已通过审核且开放接单的专家（13900000001 / 13900000002）
  - 专家钱包的初始余额（脚本只断言"结算后增加了 expertAmountCents"，不写死数值）
  - 后台管理员（13800000000）

    rm -f data/ops_expert.db
    go run ./cmd/api > /tmp/api.log 2>&1 &
    python scripts/smoke_order.py                  # dev 环境验证码直接回传，不用读日志
    python scripts/smoke_order.py /tmp/api.log      # 关掉 expose_code 时从服务日志里取

为什么单独一个脚本而不是并进 smoke_kb.py：两个脚本的种子契约不同，且共用登录号码。
每个脚本配一个**全新的服务进程**跑，短信发送间隔的计数才不会被上一轮带着。
"""
import json
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

BASE = "http://localhost:8080/api/v1"
LOG = Path(sys.argv[1]) if len(sys.argv) > 1 else None
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
        # 非 JSON 响应（502 网关页、panic 栈）也要能读出下文，否则排查时只剩一个状态码
        payload = {"_raw": raw[:300]}
    ok = expect is None or code == expect
    results.append(ok)
    tag = f" {label}" if label else ""
    extra = "" if ok else f"  want={expect} body={json.dumps(payload, ensure_ascii=False)[:200]}"
    print(f"[{'OK ' if ok else 'FAIL'}] {method:6} {path[:56]:56} -> HTTP {code}{tag}{extra}")
    return code, payload


def soft_call(method, path, body=None, token=None, label=""):
    """只发请求、打印结果，不把非 2xx 记成断言。

    用于"失败也在预期内"的调用。目前只有获取验证码：同一号码 60 秒内只能发一次，
    跨脚本复用同一个服务进程时会撞上，那是被测系统的正常保护，不该算演练失败。
    """
    code, raw = _send(method, path, body, token)
    try:
        payload = json.loads(raw)
    except (json.JSONDecodeError, UnicodeDecodeError):
        payload = {"_raw": raw[:300]}
    print(f"[{'warn' if code >= 300 else 'OK  '}] {method:6} {path[:56]:56} -> HTTP {code} {label}")
    return code, payload


def code_from_log(phone):
    for _ in range(40):
        txt = LOG.read_text(encoding="utf-8", errors="ignore")
        m = re.findall(rf"phone={phone} code=(\d{{6}})", txt)
        if m:
            return m[-1]
        time.sleep(0.4)
    raise SystemExit("未能从日志中取到验证码: " + phone)


def login(phone, nickname=None):
    """拿到令牌。优先用响应里回传的 devCode，没有再退回读服务日志。"""
    for attempt in range(2):
        code, p = soft_call("POST", "/auth/sms/send", {"phone": phone, "scene": "login"},
                            label=f"获取验证码 {phone}")
        if code == 200:
            dev = (p.get("data") or {}).get("devCode")
            if dev:
                return _login_with(phone, dev, nickname)
            break
        print("     ... 撞上发送间隔限制，等待 61s 后重试")
        time.sleep(61)
    if LOG:
        return _login_with(phone, code_from_log(phone), nickname)
    raise SystemExit(f"取不到验证码：{phone}（请打开 sms.expose_code，或把服务日志路径作为第一个参数）")


def _login_with(phone, code, nickname):
    body = {"phone": phone, "code": code}
    if nickname:
        body["nickname"] = nickname
    _, p = call("POST", "/auth/login/sms", body, expect=200, label=f"登录 {phone}")
    return p["data"]["token"]["accessToken"], p["data"]["user"]


def jget(payload, *keys):
    cur = payload
    for k in keys:
        cur = cur[k]
    return cur


print("--- 1. 登录：下单人 / 专家 / 管理员 ---")
user_token, user = login("13800007777", "运维老王")
print(f"     下单人 id={user['id']} role={user['role']}")
expert_token, expert = login("13900000001")
print(f"     专家   id={expert['id']} role={expert['role']}")

_, p = call("GET", "/experts?pageSize=50", expect=200, label="专家列表")
cards = jget(p, "data", "list")
me = next((c for c in cards if c["userId"] == expert["id"]), None)
check(me is not None, "种子专家已通过审核并对外可见（大厅/预约都依赖它）")
if me is None:
    raise SystemExit("示例专家未出现在专家列表里，后面的预约与抢单都无从验起")
print(f"     档案 id={me['id']} userId={me['userId']} 服务模式={me['serviceModes']} 时薪={me['pricePerHourCents']}分")

_, p = call("GET", "/skills", expect=200, label="技能字典")
skills = jget(p, "data", "list")
# 技能字典里的 id 是数字，而 SkillIDs 收的是字符串数组——两侧类型不一致，显式转换，
# 别靠 Python 的隐式行为糊过去，否则换个语言写客户端就会踩坑。
skill_id = str(skills[0]["id"] if isinstance(skills[0], dict) else skills[0])
print(f"     取技能 {skill_id!r}")

print("--- 2. 发布悬赏 ---")
_, p = call("POST", "/questions", {
    "title": "K8s 节点频繁 NotReady 排查",
    "description": "三个 worker 节点每隔几小时就 NotReady，kubelet 日志里有 PLEG 相关报错，需要有人帮我定位根因。",
    "skillIds": [skill_id],
    "budgetCents": 20000,
    "expectResponseMin": 30,
    "expireHours": 72,
}, token=user_token, expect=201, label="发布悬赏")
q1 = p["data"]
q_order_no, q_id = q1["orderNo"], q1["id"]
print(f"     问题 id={q_id} 订单 {q_order_no} 问题状态={q1['status']} 订单状态={q1['orderStatus']}")
check(q1["status"] == "open", "悬赏进入待响应")
check(q1["orderStatus"] == "pending_accept", "同一事务里建出的订单进入待接单")
check(q1["budgetCents"] == 20000, "预算原样落库")
check(bool(q1.get("expiredAt")), "悬赏带过期时间（到期由定时任务收口）")

# 金额恒等式在订单上验，问题详情不带金额字段——它只负责把用户引到订单去。
_, p = call("GET", f"/orders/{q_order_no}", token=user_token, expect=200, label="刚建出的订单详情")
check(p["data"]["amountCents"] == 20000, "订单金额等于预算")
check(p["data"]["commissionCents"] + p["data"]["expertAmountCents"] == p["data"]["amountCents"],
      "佣金 + 专家实收 == 订单总额（金额恒等式）")

print("--- 3. 大厅只该看见未指派的悬赏 ---")
_, p = call("GET", "/orders/hall", token=expert_token, expect=200, label="接单大厅")
hall = jget(p, "data", "list")
hit = next((h for h in hall if h["orderNo"] == q_order_no), None)
check(hit is not None, "刚发布的悬赏出现在大厅里")
if hit:
    check(hit["orderId"] > 0 and hit["orderNo"] == q_order_no,
          "大厅条目自带订单标识（否则前端无从发起抢单）")

print("--- 4. 抢单 ---")
_, p = call("POST", f"/orders/{q_order_no}/grab", token=expert_token, expect=200, label="专家抢单")
g = p["data"]
print(f"     状态={g['status']} 专家={g['expertId']} 应付={g['amountCents']}分 "
      f"佣金={g['commissionCents']}分 实收={g['expertAmountCents']}分")
check(g["status"] == "pending_pay" and g["expertId"] == expert["id"], "抢单后转入待支付并绑定专家")
check(bool(g.get("payExpireAt")), "抢单同时写入支付截止时间（否则超时关单永远扫不到这一单）")

print("--- 5. 抢单冲突：第二个人再抢应被拒 ---")
other_token, other = login("13900000002")
call("POST", f"/orders/{q_order_no}/grab", token=other_token, expect=409, label="第二人抢单被拒")

print("--- 6. 支付：通道语义 ---")
_, p = call("GET", "/orders/pay-channels", token=user_token, expect=200, label="可选支付通道")
avail = [c["name"] for c in jget(p, "data", "list") if c["available"]]
print(f"     已接通通道={avail}")
check("mock" in avail, "开发环境默认通道出现在可选列表里")
call("POST", f"/orders/{q_order_no}/pay", {"channel": "nonsense"}, token=user_token,
     expect=400, label="不认识的通道 -> 400（当成参数错）")
call("POST", f"/orders/{q_order_no}/pay", {"channel": "wechat"}, token=user_token,
     expect=501, label="认识但未接通的通道 -> 501（禁止静默降级）")
_, p = call("POST", f"/orders/{q_order_no}/pay", {"channel": "mock"}, token=user_token,
            expect=200, label="mock 支付")
check(p["data"]["paid"] is True and p["data"]["manual"] is False, "mock 通道即时到账")
check(p["data"]["channel"] == "mock", "回执里带回实际使用的通道，前端据此提示")
_, p = call("GET", f"/orders/{q_order_no}", token=user_token, expect=200, label="支付后的订单")
check(p["data"]["status"] == "in_service", "支付后订单转入服务中")
check(p["data"]["payChannel"] == "mock", "订单记下了实际支付通道（对账要认这个字段）")
call("POST", f"/orders/{q_order_no}/pay", {"channel": "mock"}, token=user_token,
     expect=409, label="已支付后重复支付被拒")

print("--- 7. 服务会话 ---")
call("POST", f"/orders/{q_order_no}/sessions?channel=video", token=expert_token,
     expect=200, label="开始会话")
_, p = call("POST", f"/orders/{q_order_no}/sessions", token=expert_token,
            expect=200, label="不带通道重复进入")
check(p["data"]["channel"] == "video", "复用已有会话时沿用原通道，而不是报通道不合法")
_, p = call("POST", f"/orders/{q_order_no}/sessions/end", token=expert_token, expect=200, label="结束会话")
check(p["data"]["status"] == "ended" and p["data"].get("durationSec") is not None,
      "结束后给出结束状态与时长")
call("POST", f"/orders/{q_order_no}/sessions", token=expert_token, expect=400,
     label="已结束后再进、且没给通道 -> 400")

print("--- 8. 完成 → 确认 → 结算 ---")
_, p = call("GET", "/expert/wallet", token=expert_token, expect=200, label="专家钱包（结算前）")
before = jget(p, "data", "balanceCents")

call("POST", f"/orders/{q_order_no}/finish", token=expert_token, expect=200, label="专家标记完成")
_, p = call("GET", f"/orders/{q_order_no}", token=user_token, expect=200, label="订单详情（待确认）")
check(p["data"]["status"] == "pending_confirm", "完成标记后转入待确认")
check(bool(p["data"].get("confirmDeadline")), "待确认订单带确认截止时间（自动确认靠它兜底）")
check(all("Status" not in k or k == "status" for k in p["data"]), "订单字段名里没有第二个状态位")
tl = p["data"].get("timeline") or []
check(len(tl) >= 4 and all(n["status"].isascii() for n in tl),
      "时间轴按英文枚举返回且已记录关键节点，中文标签由前端映射")

call("POST", f"/orders/{q_order_no}/confirm", token=user_token, expect=200, label="用户确认完成")
_, p = call("GET", f"/orders/{q_order_no}", token=user_token, expect=200, label="订单详情（结算后）")
d = p["data"]
print(f"     状态={d['status']} 分账={json.dumps(d.get('settlement'), ensure_ascii=False)}")
check(d["status"] == "settled", "确认后自动结算，状态为 settled")
settle = d.get("settlement") or {}
check(settle.get("expertAmountCents") == d["expertAmountCents"], "分账明细里的专家实收与订单一致")
check(settle.get("commissionCents") == d["commissionCents"], "分账明细里的佣金与订单一致")

_, p = call("GET", "/expert/wallet", token=expert_token, expect=200, label="专家钱包（结算后）")
after = jget(p, "data", "balanceCents")
print(f"     余额 {before} -> {after}（+{after - before}），期望 +{d['expertAmountCents']}")
check(after - before == d["expertAmountCents"], "结算后余额恰好增加专家实收")

print("--- 9. 评价 ---")
call("POST", f"/orders/{q_order_no}/review", {
    "score": 5, "professionalScore": 5, "speedScore": 4, "communicationScore": 5,
    "content": "定位很快，问题根因说得很清楚，还顺手帮我改了 kubelet 的配置。",
    "recommend": True,
}, token=user_token, expect=201, label="提交评价")
call("POST", f"/orders/{q_order_no}/review", {"score": 1, "content": "重复评价应被唯一索引拒绝。"},
     token=user_token, expect=409, label="重复评价被拒（唯一索引兜底）")
call("POST", f"/orders/{q_order_no}/review/reply", {"reply": "感谢认可，后续有需要随时找我。"},
     token=expert_token, expect=200, label="专家回复评价")

print("--- 10. 越权与参数守卫 ---")
call("GET", f"/orders/{q_order_no}", token=other_token, expect=403, label="无关用户看订单被拒")
call("POST", f"/orders/{q_order_no}/confirm", token=other_token, expect=403, label="无关用户确认被拒")
call("GET", "/orders/123", token=user_token, expect=400, label="订单号形状不合法 -> 400")
call("GET", "/orders/hall", token=user_token, expect=403, label="非专家访问大厅被拒")
call("GET", "/orders/hall", expect=401, label="游客访问大厅被拒")
call("GET", "/orders/counts", token=user_token, expect=200, label="订单数统计")
call("POST", "/questions", {"title": "缺预算", "description": "补充说明足够长的一段文字。"},
     token=user_token, expect=400, label="缺预算的悬赏被拒")

print("--- 11. 预约下单（价格由服务端算） ---")
_, p = call("POST", "/orders", {
    "expertUserId": expert["id"],
    "serviceMode": me["serviceModes"][0],
    "durationMin": 90,
    "remark": "希望先讲一下备份策略",
}, token=user_token, expect=201, label="预约下单")
b = p["data"]
expect_amount = (me["pricePerHourCents"] * 90 + 59) // 60   # 向上取整到分
print(f"     订单 {b['orderNo']} 金额={b['amountCents']}分 期望={expect_amount}分 状态={b['status']}")
check(b["amountCents"] == expect_amount, "金额按专家时薪在服务端计算，不采信前端")
check(b["commissionCents"] + b["expertAmountCents"] == b["amountCents"],
      "预约单的佣金与专家实收之和与总额一致")
check(b["expertId"] == expert["id"], "预约单绑定的是档案里的用户 ID")
call("POST", "/orders", {
    "expertUserId": expert["id"],
    "serviceMode": me["serviceModes"][0],
}, token=expert_token, expect=400, label="专家给自己下单被拒（自交易）")

print("--- 12. 取消未支付订单 ---")
call("POST", f"/orders/{b['orderNo']}/cancel", {"reason": "时间冲突，改天再约"},
     token=user_token, expect=200, label="取消预约单")
_, p = call("GET", f"/orders/{b['orderNo']}", token=user_token, expect=200, label="取消后的详情")
check(p["data"]["status"] == "cancelled", "取消后状态为 cancelled")
call("POST", f"/orders/{b['orderNo']}/cancel", {"reason": "再取消一次"},
     token=user_token, expect=409, label="重复取消被状态机拒绝")

print("--- 13. 争议链路 ---")
_, p = call("POST", "/questions", {
    "title": "Nginx 502 反复出现需要定位",
    "description": "上游是 php-fpm，压测到 200 并发就开始 502，想找人看一下 fastcgi 的超时与进程数配置。",
    "skillIds": [skill_id],
    "budgetCents": 15000,
    "expireHours": 24,
}, token=user_token, expect=201, label="发布第二个悬赏")
q2 = p["data"]["orderNo"]
call("POST", f"/orders/{q2}/grab", token=expert_token, expect=200, label="抢第二单")
call("POST", f"/orders/{q2}/pay", {"channel": "mock"}, token=user_token, expect=200, label="支付第二单")
_, p = call("POST", f"/orders/{q2}/dispute",
            {"reason": "服务内容与约定不一致，希望能退一部分。"},
            token=user_token, expect=201, label="发起争议")
dispute_id = p["data"]["id"]
check(p["data"]["status"] == "open", "新发起的争议处于处理中")
call("POST", f"/orders/{q2}/dispute", {"reason": "重复发起争议这件事本身应该被拒绝掉。"},
     token=user_token, expect=409, label="重复发起争议被拒")
call("POST", f"/orders/{q2}/confirm", token=user_token, expect=409, label="争议中不能确认完成")
_, p = call("GET", f"/orders/{q2}", token=user_token, expect=200, label="争议中的订单详情")
check((p["data"].get("dispute") or {}).get("status") == "open", "订单详情能读到进行中的争议")

print("--- 14. 后台处理争议（判退） ---")
admin_token, admin = login("13800000000")
print(f"     管理员 id={admin['id']} role={admin['role']}")
call("GET", "/admin/orders?pageSize=50", token=admin_token, expect=200, label="后台订单列表")
call("GET", f"/admin/orders/{q2}", token=admin_token, expect=200, label="后台订单详情")
call("GET", "/admin/orders", token=expert_token, expect=403, label="普通用户访问后台被拒")
call("POST", f"/admin/disputes/{dispute_id}/resolve",
     {"resolution": "refunded", "result": "经核实服务确有缺失，按约定全额退款。"},
     token=admin_token, expect=200, label="后台判定退款")
_, p = call("GET", f"/orders/{q2}", token=user_token, expect=200, label="退款后的订单")
d2 = p["data"]
check(d2["status"] == "refunded", "争议判退后订单状态为 refunded")
# 结案后的争议必须仍然可见，否则用户只看到"已退款"却找不到处理结论。
check((d2.get("dispute") or {}).get("status") == "resolved", "争议结案后仍能在订单详情里看到")
check((d2.get("dispute") or {}).get("result", "").startswith("经核实"), "结案结论随争议一起返回")
check("settlement" not in d2 or not d2.get("settlement"), "退款订单不该有分账记录")
call("POST", f"/orders/{q2}/review", {"score": 1, "content": "这条评价不该被允许，因为订单是退款收尾的。"},
     token=user_token, expect=409, label="退款订单不能评价")

print("--- 15. 线下转账 + 后台人工确认收款 ---")
_, p = call("POST", "/questions", {
    "title": "数据库主从延迟持续增大",
    "description": "从库延迟从 1 秒涨到 300 秒，binlog 量不大，想排查是不是大事务导致的。",
    "skillIds": [skill_id],
    "budgetCents": 30000,
    "expireHours": 24,
}, token=user_token, expect=201, label="发布第三个悬赏")
q3 = p["data"]["orderNo"]
call("POST", f"/orders/{q3}/grab", token=expert_token, expect=200, label="抢第三单")
_, p = call("POST", f"/orders/{q3}/pay", {"channel": "offline"}, token=user_token,
            expect=200, label="线下转账（不即时到账）")
check(p["data"]["manual"] is True and p["data"]["paid"] is False,
      "offline 通道标记为人工核账，不即时置为已支付")
check(bool(p["data"].get("instruction")), "线下通道给出下一步说明，而不是拉起收银台")
_, p = call("GET", f"/orders/{q3}", token=user_token, expect=200, label="线下支付后订单状态")
check(p["data"]["status"] == "pending_pay", "订单仍停在待支付，等后台核账")
_, p = call("POST", f"/admin/orders/{q3}/mark-paid",
            {"thirdPartyNo": "BANK202610150001", "remark": "对公转账已到账"},
            token=admin_token, expect=200, label="后台确认收款")
check(p["data"]["paid"] is True, "后台核账接口回报已收款")
_, p = call("GET", f"/orders/{q3}", token=user_token, expect=200, label="核账后的订单")
check(p["data"]["status"] == "in_service", "核账后订单才从待支付转入服务中")
check(p["data"]["payChannel"] == "offline", "通道记录为 offline，人工确认不影响通道归属")
call("POST", f"/admin/orders/{q3}/mark-paid", {"thirdPartyNo": "BANK202610150002"},
     token=admin_token, expect=409, label="重复确认收款被拒（不能重复进入服务中）")

print()
passed, total = sum(1 for r in results if r), len(results)
print(f"===== 订单冒烟结果：{passed}/{total} 通过 =====")
if passed != total:
    sys.exit(1)
