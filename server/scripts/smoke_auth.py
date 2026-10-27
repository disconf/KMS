#!/usr/bin/env python3
"""邮箱注册 / 登录与「只读 → 实名 → 可发言」权限链的端到端冒烟。

守的是这一串规则：
  邮箱验证码是唯一自助入口 → 注册出来的是只读账号 → 只读账号发不了任何内容
  → 交实名申请 → 管理员审核 → 通过后立刻能发言（且不需要重新登录）

与其它套件同一套口径：每条断言守一条业务规则，跑完打印 `xx/xx 通过`，
有失败以非 0 退出。必须跑在**全新的数据库 + 全新的服务进程**上：

  - 发送验证码有 60 秒间隔限制，跨套件复用进程会撞限流（本脚本每个邮箱只用一次，
    但"发送太频繁"这条规则本身要靠一个新进程才验得准）
  - 管理员口令取种子值 Admin@12345（生产会覆盖，所以这套只能在 dev/test 跑）

    go build -o bin/api.exe ./cmd/api
    bash scripts/smoke_auth.py 的启动方式见 scripts/smoke.sh（run_suite auth ...）

需要 mail.provider=mock + mail.expose_code=true：验证码从响应里直接取，
拿不到再退回读服务日志（日志里是 `to=<邮箱> code=<6位>`）。
"""
import base64
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

# 每个邮箱只用一次：发送间隔键按邮箱计，复用就得等 60 秒。
# 后缀统一用 example.com，真实域名收不到这些信也无所谓（mock 通道不发信）。
MAIL_MAIN = "readonly@example.com"
MAIL_WEAK = "weak@example.com"
MAIL_BADCODE = "badcode@example.com"
MAIL_TAKEN = "taken@example.com"
MAIL_CODE_LOGIN = "codelogin@example.com"
ADMIN_PHONE = "13800000000"
ADMIN_PASSWORD = "Admin@12345"
PASSWORD = "OpsExpert2026"


def check(cond, label=""):
    ok = bool(cond)
    results.append(ok)
    print(f"     [{'OK ' if ok else 'FAIL'}] {label}")
    return ok


def _send(method, path, body, token, raw_body=None):
    data = raw_body if raw_body is not None else (
        json.dumps(body).encode() if body is not None else None)
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def call(method, path, body=None, token=None, expect=None, label="", raw_body=None):
    code, raw = _send(method, path, body, token, raw_body)
    try:
        payload = json.loads(raw)
    except (json.JSONDecodeError, UnicodeDecodeError):
        payload = {"_raw": raw[:300]}
    ok = expect is None or code == expect
    results.append(ok)
    tag = f" {label}" if label else ""
    extra = "" if ok else f"  want={expect} body={json.dumps(payload, ensure_ascii=False)[:200]}"
    print(f"[{'OK ' if ok else 'FAIL'}] {method:6} {path[:52]:52} -> HTTP {code}{tag}{extra}")
    return code, payload


def biz(payload):
    """业务错误码。0 / 缺失都当成功。"""
    return int(payload.get("code") or 0)


# 1×1 的合法 PNG。上传接口按文件头识别类型，随便编几个字节过不了。
PNG_1PX = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8AAAwAB/wD/6f7ZAAAAAElFTkSuQmCC"
)


def upload_image(token, scene="article_image", data=PNG_1PX, filename="shot.png"):
    """按 multipart/form-data 发一个文件。返回 (HTTP 状态, JSON)。"""
    boundary = "----smoke" + str(int(time.time() * 1000))
    body = b"".join([
        f"--{boundary}\r\n".encode(),
        f'Content-Disposition: form-data; name="scene"\r\n\r\n{scene}\r\n'.encode(),
        f'--{boundary}\r\n'.encode(),
        f'Content-Disposition: form-data; name="file"; filename="{filename}"\r\n'.encode(),
        b"Content-Type: image/png\r\n\r\n",
        data,
        f"\r\n--{boundary}--\r\n".encode(),
    ])
    req = urllib.request.Request(BASE + "/upload", data=body, method="POST")
    req.add_header("Content-Type", f"multipart/form-data; boundary={boundary}")
    req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status, json.loads(r.read().decode())
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, {"_raw": raw[:200]}


def head_status(url):
    """只取状态码，用来验证静态图片能不能匿名访问。"""
    req = urllib.request.Request(url, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            return r.status, r.headers.get("Content-Type", "")
    except urllib.error.HTTPError as e:
        return e.code, ""
    except urllib.error.URLError:
        return 0, ""


def code_from_log(email):
    for _ in range(40):
        if LOG and LOG.exists():
            txt = LOG.read_text(encoding="utf-8", errors="ignore")
            m = re.findall(rf"to={re.escape(email)}\s+code=(\d{{6}})", txt)
            if not m:
                m = re.findall(rf"{re.escape(email)}.*?code=(\d{{6}})", txt)
            if m:
                return m[-1]
        time.sleep(0.4)
    raise SystemExit("未能从日志中取到验证码: " + email)


def send_code(email, scene="register", expect=200):
    code, p = call("POST", "/auth/email/send",
                   {"email": email, "scene": scene}, expect=expect,
                   label=f"发码 {email} ({scene})")
    dev = (p.get("data") or {}).get("devCode")
    if dev:
        return dev, p
    if expect == 200 and LOG:
        return code_from_log(email), p
    return dev, p


def login_password(phone, password):
    _, p = call("POST", "/auth/login/password",
                {"phone": phone, "password": password},
                expect=200, label=f"口令登录 {phone}")
    return p["data"]["token"]["accessToken"], p["data"]["user"]


print("--- 1. 注册：邮箱验证码 + 自设密码 ---")
code1, sendResp = send_code(MAIL_MAIN, "register")
check(bool(code1) and len(code1) == 6, "注册验证码是 6 位数字")
check(int((sendResp.get("data") or {}).get("interval") or 0) > 0,
      "响应带回重发间隔（前端倒计时要靠它）")

_, p = call("POST", "/auth/register", {
    "email": MAIL_MAIN, "code": code1, "password": PASSWORD, "nickname": "只读小张",
}, expect=200, label="注册")
reg = p.get("data") or {}
user = reg.get("user") or {}
token = ((reg.get("token") or {}).get("accessToken")) or ""
check(bool(token), "注册直接下发令牌（不用再登录一次）")
check(user.get("role") == "user", "新账号角色是 user，不是 expert/admin")
check(user.get("speakStatus") == "none", "新账号发言状态是 none（从未申请）")
check(user.get("canSpeak") is False, "新账号 canSpeak=false —— 注册出来的是只读账号")
check(user.get("email") == MAIL_MAIN, "资料里回显注册邮箱")

print("--- 2. 注册的拒绝路径 ---")
# 已注册的邮箱不该再收到注册码：既没意义，也白耗一次发信额度。
# 这一步同时守住"注册码的占用检查在发信之前"。
_, p = call("POST", "/auth/email/send",
            {"email": MAIL_MAIN, "scene": "register"}, expect=None,
            label="已注册邮箱请求注册码")
check(biz(p) == 20019, "已注册邮箱不再发注册码（20019 ErrEmailTaken）")

code2, _ = send_code(MAIL_BADCODE, "register")
_, p = call("POST", "/auth/register", {
    "email": MAIL_BADCODE, "code": "000001" if code2 != "000001" else "000002",
    "password": PASSWORD,
}, expect=None, label="错验证码注册")
check(biz(p) == 20021, "验证码不对时拒绝注册（20021 ErrEmailCodeInvalid）")

codeWeak, _ = send_code(MAIL_WEAK, "register")
_, p = call("POST", "/auth/register", {
    "email": MAIL_WEAK, "code": codeWeak, "password": "12345678",
}, expect=None, label="弱口令注册")
check(biz(p) != 0, f"纯数字口令被拒（强度校验），错误码={biz(p)}")

_, p = call("POST", "/auth/register", {
    "email": "not-an-email", "code": "123456", "password": PASSWORD,
}, expect=None, label="邮箱格式非法")
check(biz(p) == 20018, "邮箱格式非法（20018 ErrEmailInvalid）")

# 前面几个都没注册成功，这个地址应当是干净的：
# 一次"能注册成功"的对照，证明上面的失败是被规则拦的，不是验证码根本没发出来。
codeTaken, _ = send_code(MAIL_TAKEN, "register")
_, p = call("POST", "/auth/register", {
    "email": MAIL_TAKEN, "code": codeTaken, "password": PASSWORD,
}, expect=200, label="干净邮箱注册")
check(biz(p) == 0, "未被占用的邮箱可以正常注册")

print("--- 3. 两条登录通道 ---")
_, p = call("POST", "/auth/login/email",
            {"email": MAIL_MAIN, "password": PASSWORD}, expect=200, label="邮箱+密码登录")
token = p["data"]["token"]["accessToken"]
check(p["data"]["user"]["canSpeak"] is False, "登录回来仍是只读")

_, p = call("POST", "/auth/login/email",
            {"email": MAIL_MAIN, "password": "WrongPass123"},
            expect=None, label="错误密码登录")
check(biz(p) == 20014, "密码错误统一报「手机号或密码不正确」（20014，不泄露账号是否存在）")

codeLogin, _ = send_code(MAIL_CODE_LOGIN, "register")
_, p = call("POST", "/auth/register", {
    "email": MAIL_CODE_LOGIN, "code": codeLogin, "password": PASSWORD,
}, expect=200, label="给验证码登录准备账号")

# 同一邮箱注册时刚用过一次发信额度，登录码要等过发送间隔。
# 这里显式等一轮而不是换个邮箱：换邮箱验的就变成"另一个账号能不能登录"，
# 而这条断言要守的是"同一个账号可以换通道进来"。
print("     ... 等待 61s 让该邮箱的发信间隔过期")
time.sleep(61)
codeLogin2, _ = send_code(MAIL_CODE_LOGIN, "login")
_, p = call("POST", "/auth/login/email-code",
            {"email": MAIL_CODE_LOGIN, "code": codeLogin2},
            expect=200, label="邮箱+验证码登录")
codeToken = p["data"]["token"]["accessToken"]
check(bool(codeToken), "邮箱 + 验证码也能登录（忘记密码的退路）")

_, p = call("POST", "/auth/login/email-code",
            {"email": MAIL_CODE_LOGIN, "code": codeLogin2},
            expect=None, label="同一个验证码再用一次")
check(biz(p) == 20006,
      "验证码一次性：登录成功后立即作废，再用报「已过期」（20006）")

print("--- 4. 只读账号发不了任何内容 ---")
_, p = call("POST", "/qa/questions", {
    "title": "只读账号能不能提问",
    "description": "这是一条本该被拦住的提问，用来验证发言权限的拦截确实生效。",
}, token=token, expect=None, label="只读账号提问")
check(biz(p) == 20026, "问答提问被拦（20026 ErrSpeakNotAllowed）")

_, p = call("POST", "/kb/articles/contribute", {
    "title": "只读账号的投稿标题",
    "content": "x" * 80,
}, token=token, expect=None, label="只读账号投稿")
check(biz(p) == 20026, "文章投稿被拦（20026）")

# 悬赏走的是另一套 service，必须单独验一遍：它有金额，绕过去等于能挂单骗专家接单
_, p = call("POST", "/questions", {
    "title": "只读账号发悬赏",
    "description": "同样该被拦住：悬赏会进入接单大厅，是真金白银的单子。",
    "skillIds": ["1"], "budgetCents": 10000, "expectResponseMin": 30, "expireHours": 24,
}, token=token, expect=None, label="只读账号发悬赏")
check(biz(p) == 20026, "悬赏发布被拦（20026）")

print("--- 5. 实名申请 ---")
_, p = call("GET", "/me/speak-apply", token=token, expect=200, label="查询发言状态")
st = p.get("data") or {}
check(st.get("status") == "none" and st.get("canSpeak") is False, "初始状态 none / 不可发言")

_, p = call("POST", "/me/speak-apply", {
    "realName": "张", "contact": "13800000000",
}, token=token, expect=None, label="姓名太短")
check(biz(p) == 30007, "姓名不足 2 字被拒（30007 ErrRealNameInvalid）")

_, p = call("POST", "/me/speak-apply", {
    "realName": "张伟", "contact": "13800000000", "note": "做 K8s 运维，想回答集群相关的问题",
}, token=token, expect=200, label="提交实名申请")
st = p.get("data") or {}
check(st.get("status") == "pending", "申请后进入 pending")
check(st.get("canSpeak") is False, "待审期间仍然不能发言")
check(bool(st.get("appliedAt")), "申请时间已记录")

_, p = call("POST", "/me/speak-apply", {
    "realName": "张伟", "contact": "13800000000",
}, token=token, expect=None, label="重复提交申请")
check(biz(p) == 20027, "pending 期间不能重复提交（20027 ErrSpeakApplyDup）")

_, p = call("POST", "/qa/questions", {
    "title": "审核中能不能先提问",
    "description": "还没通过审核，这一步必须仍然被拦住，否则 pending 就等于已通过。",
}, token=token, expect=None, label="pending 期间提问")
check(biz(p) == 20026, "pending 不是豁免：仍然不能发言")

print("--- 6. 管理员审核 ---")
admin_token, admin = login_password(ADMIN_PHONE, ADMIN_PASSWORD)
check(admin.get("role") == "admin", "管理员登录成功且角色是 admin")

_, p = call("GET", "/admin/speak-applications?status=pending&pageSize=50",
            token=admin_token, expect=200, label="待审发言申请列表")
apps = (p.get("data") or {}).get("list") or []
mine = next((a for a in apps if a.get("email") == MAIL_MAIN), None)
check(mine is not None, "刚提交的申请出现在待审列表里")
if mine:
    check(mine.get("realName") == "张伟" and mine.get("contact") == "13800000000",
          "列表带回实名与联系方式（审核要据此核人）")
    check(mine.get("nickname") == "只读小张", "列表带回昵称，运营才知道是谁")
    uid = mine["userId"]

    _, p = call("POST", f"/admin/speak-applications/{uid}/review",
                {"approve": False}, token=admin_token, expect=None, label="驳回但不填原因")
    check(biz(p) == 30015, "驳回必须写原因（30015，与其它审核同一口径）")

    _, p = call("POST", f"/admin/speak-applications/{uid}/review",
                {"approve": True, "note": "已确认身份"}, token=admin_token,
                expect=200, label="通过申请")
    check(biz(p) == 0, "审核通过成功")

    _, p = call("POST", f"/admin/speak-applications/{uid}/review",
                {"approve": True}, token=admin_token, expect=None, label="重复审核")
    check(biz(p) == 20028, "非 pending 状态不能重复审核（20028 ErrSpeakApplyState）")

print("--- 7. 通过后立刻能发言（不换令牌） ---")
_, p = call("GET", "/me/speak-apply", token=token, expect=200, label="再查发言状态")
st = p.get("data") or {}
check(st.get("status") == "approved", "状态变成 approved")
check(st.get("canSpeak") is True, "canSpeak 变 true")
check(st.get("reviewNote") == "已确认身份", "审核备注回传给本人")

# 关键点：中间件按 uid 查库，所以同一个旧令牌下一跳就该放行。
# 若这里失败，说明有人在中间件里缓存了身份快照。
_, p = call("POST", "/qa/questions", {
    "title": "通过审核后的第一次提问",
    "description": "刚拿到发言权限，立刻提问应当成功：权限生效不该要求重新登录。",
}, token=token, expect=200, label="通过后提问（旧令牌）")
check(biz(p) == 0, "提问成功，不需要重新登录")
qid = (p.get("data") or {}).get("id")
check(bool(qid), "返回问题 id")

if qid:
    # 不能拿刚提的问题自问自答（服务端有专门一条 45003），
    # 所以让管理员再提一个，由这个刚通过审核的账号去回答。
    _, p = call("POST", "/qa/questions", {
        "title": "管理员提一个问题给新账号回答",
        "description": "用来验证回答接口吃的是同一套发言权限判断，而不是只有提问被守住。",
    }, token=admin_token, expect=200, label="管理员提问")
    admin_qid = (p.get("data") or {}).get("id")
    check(bool(admin_qid), "管理员提问成功（admin 天生有发言权）")
    if admin_qid:
        _, p = call("POST", f"/qa/questions/{admin_qid}/answers", {
            "content": "回答一条，验证回答接口也走同一套权限判断。",
        }, token=token, expect=200, label="通过后回答")
        check(biz(p) == 0, "回答接口同样放行")

print("--- 8. 发送限流与场景校验 ---")
# 先发一次（此时上一封已过间隔）再连发一次，两跳连着看限流是不是按邮箱计的。
_, p = call("POST", "/auth/email/send",
            {"email": MAIL_MAIN, "scene": "login"}, expect=200, label="换个场景再发码")
check(biz(p) == 0, "同一邮箱可以换场景再发（注册码与登录码互不影响）")

_, p = call("POST", "/auth/email/send",
            {"email": MAIL_MAIN, "scene": "login"}, expect=None, label="立刻再发一次")
check(biz(p) == 20022, "同一邮箱 60 秒内只能发一次（20022 ErrEmailSendTooFast）")

_, p = call("POST", "/auth/email/send",
            {"email": "whatever@example.com", "scene": "bogus"},
            expect=None, label="未知场景")
check(biz(p) != 0, "未知场景被拒（Scene 只认 login/bind/register）")

print("--- 9. 正文插图：上传与公开访问 ---")
# 只读账号连图片都不该能传：上传等于往公共存储里发布内容。
_, p = call("POST", "/auth/login/email",
            {"email": MAIL_TAKEN, "password": PASSWORD}, expect=200,
            label="只读账号登录（取令牌）")
ro_token = ((p.get("data") or {}).get("token") or {}).get("accessToken") or ""
code, p = upload_image(ro_token)
check(code == 403 and biz(p) == 20026,
      f"只读账号上传被拒（HTTP {code} / code {biz(p)}）")

code, p = upload_image(token)
data = p.get("data") or {}
check(code == 201 and biz(p) == 0, f"可发言账号上传图片成功（HTTP {code}）")
img_url = str(data.get("url", ""))
check("/uploads/article/image/" in img_url, f"返回公开访问地址（{img_url}）")
check(data.get("mime") == "image/png", f"按文件头识别出真实类型（{data.get('mime')}）")

# url 可能是绝对的（storage.public_base_url / app.base_url 有配置时），也可能相对。
root = BASE.rsplit("/api/v1", 1)[0]
full_url = img_url if img_url.startswith("http") else root + img_url
status, ctype = head_status(full_url)
check(status == 200 and "image/png" in ctype,
      f"图片可匿名访问且类型正确（HTTP {status} / {ctype}）")

# 私有场景的 key 不能被公开入口读到，否则资质证书等于公开。
status, _ = head_status(root + "/uploads/expert/cert/202610/0000000000000000.png")
check(status != 200, f"私有场景文件从公开入口读不到（HTTP {status}）")

# 路径穿越：任何 ../ 都不能把 key 带出存储根目录。
status, _ = head_status(root + "/uploads/article/image/..%2F..%2Fconfigs%2Fconfig.yaml")
check(status != 200, f"路径穿越被拒（HTTP {status}）")

total = len(results)
passed = sum(1 for r in results if r)
print()
print(f"===== 邮箱注册与发言权限冒烟：{passed}/{total} 通过 =====")
if passed != total:
    print("!! 有断言失败，见上面的 FAIL 行")
    sys.exit(1)
