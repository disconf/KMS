#!/usr/bin/env python3
"""部署自检：对一个**已经在运行**的实例做端到端验证。

与 smoke_*.py 的区别：那三个脚本自己拉起进程、自己建库，验证的是"代码对"；
这个脚本不碰进程和数据库，只发 HTTP，验证的是"这台机器上跑起来了，而且真的能用"：
静态站点是从二进制里出来的、/docs 没被静态路由吞掉、验证码能收到、写路径能落库、
支付通道能切到 provider。

    python scripts/verify_deploy.py                      # 默认 http://127.0.0.1:8080
    python scripts/verify_deploy.py http://1.2.3.4:8080

会真的创建一条悬赏订单（标题带"部署自检"），因为"能下单"才是部署成功的证据。
"""
import json
import os
import random
import sys
import time
import urllib.error
import urllib.request

BASE = (sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:8080").rstrip("/")
API = BASE + "/api/v1"
results = []
skips = []


def check(cond, label=""):
    ok = bool(cond)
    results.append(ok)
    print("     [%s] %s" % ("OK " if ok else "FAIL", label))
    return ok


def skip(label, reason=""):
    """不可用能力显式跳过：宁可写清"没验证"，也不要伪装成通过。

    生产上短信通道尚未接通（webhook 仍是占位 URL），依赖验证码登录的
    下单链路本来就跑不了。把它记成 FAIL 只会让人慢慢无视这个脚本。
    """
    skips.append(label)
    print("     [SKIP] %s%s" % (label, (" —— " + reason) if reason else ""))


def call(method, path, body=None, token=None, expect=None, label="", root=False, record=True):
    # root=True 表示这是站根路径（/healthz、/assets、/docs），不能拼 /api/v1
    if path.startswith("http"):
        url = path
    elif root:
        url = BASE + path
    else:
        url = API + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            code, raw = r.status, r.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        code, raw = e.code, e.read().decode("utf-8", "replace")
    except urllib.error.URLError as e:
        raise SystemExit("连不上 %s：%s" % (BASE, e))

    try:
        payload = json.loads(raw)
    except (json.JSONDecodeError, UnicodeDecodeError):
        payload = {"_raw": raw}

    ok = expect is None or code == expect
    # record=False 用于"要先试探、失败会重试"的调用（发送验证码撞限流）：
    # 中间那次 429 不该出现在断言总数里，否则总数会随重试次数浮动。
    if record:
        results.append(ok)
    extra = "" if ok else "  want=%s body=%s" % (expect, raw[:200])
    print("[%s] %-6s %-48s -> %s %s%s" % (
        "OK " if ok else "FAIL", method, path[:48], code, label, extra))
    return code, payload, raw


def ok_data(payload):
    """取出统一响应体里的 data；失败时返回 None，避免后面连串 KeyError。"""
    if isinstance(payload, dict) and payload.get("code") == 0:
        return payload.get("data")
    return None


def login(phone):
    """拿验证码并登录。

    同一号码 60 秒只能发一次是**被测系统的正常保护**，不是失败：
    连续跑两遍自检时必然会撞上（专家与管理员的号码是固定种子账号）。
    撞上就等过窗口重试，而不是把 429 记成一条失败——否则这个脚本在 CI 里
    第二遍永远红，慢慢就没人看了。
    """
    for attempt in range(3):
        code, body, _ = call("POST", "/auth/sms/send", {"phone": phone}, expect=200,
                             label="发送验证码", record=False)
        if code == 200:
            check(True, "发送验证码 %s" % phone)
            break
        if code == 429 and attempt < 2:
            print("      距上次发送不足 60 秒，等 62 秒后重试（这是限流保护，不是故障）")
            time.sleep(62)
            continue
        raise SystemExit("发送验证码失败：HTTP %s" % code)

    data = ok_data(body) or {}
    dev = data.get("devCode") or data.get("code")
    if not dev:
        raise SystemExit("响应里没有 devCode：实例可能关掉了 sms.expose_code。"
                         "请改用读服务日志的方式，或临时打开 expose_code。")
    check(len(str(dev)) == 6, "验证码为 6 位（%s）" % dev)
    code, body, _ = call("POST", "/auth/login/sms", {"phone": phone, "code": dev},
                         expect=200, label="验证码登录")
    data = ok_data(body) or {}
    # 响应形状是 {token:{accessToken,...}, user:{...}}
    token = (data.get("token") or {}).get("accessToken") if isinstance(data.get("token"), dict) else None
    check(bool(token), "拿到 access token")
    return token, data


def main():
    print("== 0. 基础探针（静态站点与文档不能被路由吞掉）==")
    # 生产默认关闭 /docs 与 /openapi.yaml（config.ExposeAPIDocs），测试环境仍开放。
    # 先探一次再断言，两种状态都算通过：开放就校验内容，关闭就确认真的 404。
    # 探测本身 record=False，不占断言总数。
    docs_code, _, _ = call("GET", "/docs", expect=None, label="/docs 探测", root=True, record=False)
    docs_on = docs_code == 200
    docs_want = 200 if docs_on else 404
    for path, want in [("/healthz", 200), ("/readyz", 200), ("/", 200),
                       ("/assets/js/app.js", 200), ("/assets/app.css", 200),
                       ("/assets/js/orders.js", 200),
                       ("/docs", docs_want), ("/openapi.yaml", docs_want)]:
        call("GET", path, expect=want, label=path, root=True)

    code, body, raw = call("GET", "/", expect=200, root=True)
    check("/assets/js/app.js" in raw, "入口页引用的脚本路径正确（白屏事故的回归断言）")
    if docs_on:
        code, body, raw = call("GET", "/openapi.yaml", expect=200, root=True)
        check("openapi:" in raw, "openapi.yaml 内容为 YAML 文档")
    else:
        code, body, raw = call("GET", "/openapi.yaml", expect=404, root=True)
        check(code == 404, "生产已关闭 API 文档（/openapi.yaml 返回 404）")
    call("GET", "/assets/js/not-exists.js", expect=404, label="不存在的静态资源应 404", root=True)
    code, body, _ = call("GET", "/api/v1/not-exists", expect=404)
    check(isinstance(body, dict) and body.get("code") not in (0, None),
          "不存在的接口返回业务错误码而不是 HTML")

    print("\n== 1. 读路径与种子数据 ==")
    code, body, _ = call("GET", "/channels", expect=200)
    channels = (ok_data(body) or {}).get("list") or []
    check(len(channels) >= 5, "栏目数 ≥ 5（实际 %d）" % len(channels))
    code, body, _ = call("GET", "/kb/articles?pageSize=3", expect=200)
    articles = (ok_data(body) or {}).get("list") or []
    check(len(articles) > 0, "知识库文章非空（种子数据已写入）")
    code, body, _ = call("GET", "/skills", expect=200)
    skills = (ok_data(body) or {}).get("list") or []
    check(len(skills) > 0, "技能字典非空（%d 项）" % len(skills))
    skill_id = skills[0]["id"] if skills else None

    # 短信通道若还是占位配置（webhook 未填 URL），发码会 502/90003。
    # 这是"还没拿到短信资质"的已知状态，不该把整份自检拖红——探一次记下结论，
    # 后面依赖验证码登录的段落显式跳过。探测本身 record=False，不占断言总数。
    probe_phone = "139" + "".join(random.choice("0123456789") for _ in range(8))
    code, body, _ = call("POST", "/auth/sms/send",
                         {"phone": probe_phone, "scene": "login"},
                         expect=None, label="短信通道探测", record=False)
    sms_ok = code == 200
    if not sms_ok:
        print("     短信通道不可用（HTTP %s / code=%s），验证码登录相关段落将跳过"
              % (code, (body or {}).get("code")))

    print("\n== 2. 邮件通道（注册与邮箱登录的唯一通道）==")
    # 非法邮箱：本地校验就挡掉，不会真的发信（业务码 20018 / HTTP 400）
    code, body, _ = call("POST", "/auth/email/send",
                         {"email": "not-an-email", "scene": "register"},
                         expect=None, label="非法邮箱格式")
    check((body or {}).get("code") == 20018, "非法邮箱被拒（20018）")

    # 形状合法但没先取码：通道就绪时校验会走到"验证码不存在/不对"（20006/20021）；
    # 通道没配 SMTP 时在发码前就返回 20024。两者都说明接口行为正确，
    # 区别只在"能不能真发信"，所以分开断，别混成一条通过。
    probe_mail = "verify-probe@example.com"
    code, body, _ = call("POST", "/auth/register",
                         {"email": probe_mail, "code": "000000", "password": "Verify@12345"},
                         expect=None, label="注册探针（不取码直接提交）")
    mail_biz = (body or {}).get("code")
    if mail_biz in (20006, 20021):
        check(True, "邮件通道就绪：走到验证码校验（%s）" % mail_biz)
    elif mail_biz == 20024:
        check(True, "邮件通道未配置：注册按设计返回 20024（不是静默失败）")
        skip("真实发信与邮箱注册", "实例未配置 SMTP")
    else:
        check(False, "注册探针返回了意料之外的业务码 %s" % mail_biz)
        skip("真实发信与邮箱注册", "探针结论不明确")

    print("\n== 3. 用户侧：登录 → 只读拦截 → 实名申请 → 发布悬赏 ==")
    admin_token = None
    if not sms_ok:
        # 短信不通时后台并非不可验证：管理员还能用口令登录。
        # 口令从环境变量读，不写进脚本（脚本会进仓库）。
        admin_pwd = os.environ.get("KMS_ADMIN_PASSWORD")
        if admin_pwd:
            code, body, _ = call("POST", "/auth/login/password",
                                 {"phone": "13800000000", "password": admin_pwd},
                                 expect=200, label="管理员密码登录")
            admin_token = ((ok_data(body) or {}).get("token") or {}).get("accessToken")
            check(bool(admin_token), "拿到管理员令牌（口令登录，不依赖短信）")
        for lbl in ("用户登录（短信验证码）", "新账号只读拦截", "实名申请与后台审批",
                    "发布悬赏", "专家抢单与抽佣", "支付通道切换"):
            skip(lbl, "短信通道未接通")
    else:
        admin_token, admin_data = login("13800000000")   # 种子管理员
        owner_phone = "139" + "".join(random.choice("0123456789") for _ in range(8))
        owner_token, owner_data = login(owner_phone)
        owner_id = ((owner_data or {}).get("user") or {}).get("id")
        call("GET", "/orders?side=user&pageSize=10", token=owner_token, expect=200,
             label="我的订单")

        # 新注册账号默认只读：先确认发布被挡，再走实名申请 → 审批放行。
        bounty = {
            "title": "【部署自检】K8s 节点 NotReady 排查",
            "description": "部署验证自动创建，可忽略或删除。节点状态 NotReady，kubelet 日志反复重启。",
            "skillIds": [str(skill_id)],
            "budgetCents": 20000,
            "expectResponseMin": 30,
        }
        code, body, _ = call("POST", "/questions", bounty, token=owner_token,
                             expect=403, label="只读账号发悬赏")
        check((body or {}).get("code") == 20026, "只读账号发悬赏被挡下（20026）")

        code, body, _ = call("POST", "/me/speak-apply",
                             {"realName": "部署自检", "contact": owner_phone},
                             token=owner_token, expect=200, label="提交实名申请")
        check(((ok_data(body) or {}).get("status")) == "pending", "申请落到 pending 待审")

        call("POST", "/admin/speak-applications/%s/review" % owner_id, {"approve": True},
             token=admin_token, expect=200, label="管理员审批通过")

        code, body, _ = call("POST", "/questions", bounty, token=owner_token,
                             expect=201, label="发布悬赏（审批后）")
        created = ok_data(body) or {}
        order_no = created.get("orderNo")
        check(bool(order_no), "返回订单号（%s）" % order_no)
        check(created.get("orderStatus") == "pending_accept",
              "新悬赏状态为 pending_accept（实际 %s）" % created.get("orderStatus"))
        code, body, _ = call("GET", "/orders/" + str(order_no), token=owner_token, expect=200)
        detail = ok_data(body) or {}
        check(detail.get("orderNo") == order_no, "订单详情可回读")
        check(detail.get("amountCents") == 20000, "金额按分落库（20000）")

        print("\n== 4. 专家侧：大厅 → 抢单 ==")
        expert_token, _ = login("13900000001")   # 种子专家，已审核且开放接单
        call("GET", "/orders/hall", token=expert_token, expect=200, label="接单大厅")
        call("POST", "/orders/%s/grab" % order_no, None, token=expert_token, expect=200,
             label="抢单")
        code, body, _ = call("GET", "/orders/" + str(order_no), token=owner_token, expect=200)
        detail = ok_data(body) or {}
        check(detail.get("status") == "pending_pay",
              "抢单后转 pending_pay（实际 %s）" % detail.get("status"))
        expert_amount = detail.get("expertAmountCents")
        check(isinstance(expert_amount, int) and 0 < expert_amount < 20000,
              "专家实收已按抽佣算好（%s < 20000）" % expert_amount)

        print("\n== 5. 支付通道与支付 ==")
        code, body, _ = call("GET", "/orders/pay-channels", token=owner_token, expect=200)
        chans = (ok_data(body) or {}).get("list") or []
        check(len(chans) > 0, "存在可选支付通道（%d 个）" % len(chans))
        avail = [c for c in chans if c.get("available")]
        if avail:
            check(True, "至少一个通道可用（%s）" % ",".join(c["name"] for c in avail))
            code, body, _ = call("POST", "/orders/%s/pay" % order_no,
                                 {"channel": avail[0]["name"]}, token=owner_token,
                                 expect=200, label="发起支付（%s）" % avail[0]["name"])
            code, body, _ = call("GET", "/orders/" + str(order_no), token=owner_token,
                                 expect=200)
            detail = ok_data(body) or {}
            check(detail.get("status") in ("in_service", "pending_pay"),
                  "支付后状态推进（实际 %s）" % detail.get("status"))
        else:
            # offline 通道下"没有可在线支付的通道"是预期状态，不是故障。
            check(True, "当前无可在线支付通道（offline 线下收款，符合预期）")
            skip("在线支付推进", "未接通微信/支付宝，provider=offline")

    print("\n== 6. 运营侧：后台可见 ==")
    if admin_token:
        call("GET", "/admin/orders?pageSize=5", token=admin_token, expect=200,
             label="后台订单列表")
        call("GET", "/admin/speak-applications?status=pending&pageSize=5", token=admin_token,
             expect=200, label="后台发言申请队列")
    else:
        skip("后台订单列表", "没有管理员令牌")
        skip("后台发言申请队列", "没有管理员令牌")

    total = len(results)
    passed = sum(1 for r in results if r)
    tail = "，%d 项跳过（%s）" % (len(skips), "、".join(skips[:3]) + ("…" if len(skips) > 3 else "")) if skips else ""
    print("\n%d/%d 通过%s" % (passed, total, tail))
    if passed != total:
        print("自检未通过：先看 journalctl -u ops-expert -n 100，或 docker logs kms-api")
        sys.exit(1)


if __name__ == "__main__":
    main()
