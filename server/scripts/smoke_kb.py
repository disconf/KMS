"""
知识库端到端冒烟：栏目导航、文章、开源工具、资源下载、运维题库、后台录入。

必须跑在**全新的数据库**上。脚本里有多处断言依赖种子数据的确定状态：
  - 分类数是 5（少了或多了都会让断言失真）
  - 资讯栏目当期为空（0 条）
  - 题库章节用固定手机号注册考生，统计从零开始

换个库重跑：
    rm -f data/ops_expert.db
    go run ./cmd/api > /d/Devops/gocache/api.log 2>&1 &
    python scripts/smoke_kb.py /d/Devops/gocache/api.log

第二个参数是服务日志路径：开发环境短信走 mock，验证码只出现在日志里。
"""
import json, re, sys, time, urllib.parse, urllib.request

def enc(s):
    """路径参数可能含汉字（slug 保留汉字），必须先做百分号编码。"""
    return urllib.parse.quote(str(s), safe="")
from pathlib import Path

BASE = "http://localhost:8080/api/v1"
LOG = Path(sys.argv[1] if len(sys.argv) > 1 else "/tmp/api2.log")
results = []


def check(cond, label=""):
    """记录一条断言结果。label 用于说明这条断的是哪条业务规则。"""
    ok = bool(cond)
    results.append(ok)
    print(f"     [{'OK ' if ok else 'FAIL'}] {label}")
    return ok


def call(method, path, body=None, token=None, expect=None, label=""):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            code, raw = r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        code, raw = e.code, e.read().decode()
    payload = json.loads(raw)
    ok = expect is None or code == expect
    results.append(ok)
    tag = f" {label}" if label else ""
    print(f"[{'OK ' if ok else 'FAIL'}] {method:6} {path[:62]:62} -> HTTP {code} code={payload.get('code')}{tag}")
    return code, payload


def code_from_log(phone):
    """开发环境短信走 mock，验证码只出现在服务日志里。"""
    for _ in range(40):
        txt = LOG.read_text(encoding="utf-8", errors="ignore")
        m = re.findall(rf"phone={phone} code=(\d{{6}})", txt)
        if m:
            return m[-1]
        time.sleep(0.4)
    raise RuntimeError("未能从日志中取到验证码: " + phone)


def login(phone, nickname=None):
    call("POST", "/auth/sms/send", {"phone": phone, "scene": "login"}, expect=200)
    code = code_from_log(phone)
    body = {"phone": phone, "code": code}
    if nickname:
        body["nickname"] = nickname
    _, p = call("POST", "/auth/login/sms", body, expect=200)
    return p["data"]["token"]["accessToken"], p["data"]["user"]


print("--- 0. 健康检查 ---")
with urllib.request.urlopen("http://localhost:8080/healthz", timeout=5) as r:
    print("     healthz ->", r.read().decode())

print("--- 1. 知识库：游客可读 ---")
_, p = call("GET", "/kb/categories", expect=200)
cats = p["data"]["list"]
print("     分类:", [(c["slug"], c["name"], c["articleCount"]) for c in cats])
# 不锁死总数：分类是运营会持续增补的维度，写死 5 会在每次加方向时误报。
# 要守的是"六个基础方向都在"，这才是产品承诺。
required_dirs = {"network", "linux", "cloud-native", "windows-server", "middleware", "open-source"}
results.append(required_dirs.issubset({c["slug"] for c in cats}))

_, p = call("GET", "/kb/tags", expect=200)
print("     标签:", [(t["name"], t["count"]) for t in p["data"]["list"]][:5])

_, p = call("GET", "/kb/articles", expect=200)
d = p["data"]
print(f"     列表: total={d['total']} page={d['page']} pageSize={d['pageSize']}")
first = d["list"][0]
print(f"     首条: {first['title']} | 分类={first['categoryName']} | 标签={first['tags']} | 作者={first['authorName']} | 阅读={first['viewCount']}")
results.append("content" not in first)
results.append(len(first["tags"]) > 0)

_, p = call("GET", "/kb/articles?categorySlug=database", expect=200)
# 断言"过滤正确"，而不是"total 恰好 1"：系统题库/教程文章的种子会把 MySQL
# 三篇归进 database 分类，写死总数等于给种子数据上锁（种子每加一篇就红一次）。
dblist = p["data"]["list"]
results.append(p["data"]["total"] >= 1
               and all(a.get("categorySlug") == "database" for a in dblist))
print(f"     分类过滤 database -> total={p['data']['total']}")

_, p = call("GET", "/kb/articles?" + urllib.parse.urlencode({"keyword": "重启"}), expect=200)
# 断言"目标文章命中"，而不是"总数恰好 2"：每加一篇提到「重启」的种子文章，
# 总数就会变，而检索其实完全正常——写死总数等于给种子数据上了锁。
hits = [a["slug"] for a in p["data"]["list"]]
results.append("k8s-pod-restart-troubleshooting" in hits)
print(f"     关键词“重启” -> total={p['data']['total']} hits={hits}")

_, p = call("GET", "/kb/articles?tag=MySQL", expect=200)
# 同上：断言标签过滤本身正确（每条命中都带 MySQL 标签），不写死总数。
results.append(p["data"]["total"] >= 1
               and all("MySQL" in (a.get("tags") or []) for a in p["data"]["list"]))
print(f"     标签 MySQL -> total={p['data']['total']}")

_, p = call("GET", "/kb/articles?sort=hot", expect=200)
views = [a["viewCount"] for a in p["data"]["list"]]
results.append(views == sorted(views, reverse=True))
print(f"     热门排序阅读数: {views}")

slug = "k8s-pod-restart-troubleshooting"
_, p = call("GET", f"/kb/articles/" + enc(slug) + "", expect=200)
det = p["data"]
print(f"     详情: {det['title']} | seo.title={det['seo']['title'][:20]} | liked={det['liked']} | related={len(det['related'])} | 阅读={det['viewCount']}")
results.append(det["content"].startswith("##"))
results.append(det["seo"]["keywords"] != "")
results.append(det["liked"] is False)

# 用数字 ID 访问同一条详情，验证双路径
_, p = call("GET", f"/kb/articles/" + enc(det['id']) + "", expect=200)
results.append(p["data"]["slug"] == slug)
print(f"     按 ID {det['id']} 取详情 -> slug={p['data']['slug']}")

call("GET", "/kb/articles/not-exist-at-all", expect=404)
call("POST", f"/kb/articles/" + enc(slug) + "/like", expect=401, label="未登录点赞被拒")

print("--- 2. 点赞（普通用户） ---")
user_token, user = login("13800007777", "运维老王")
print(f"     用户: id={user['id']} role={user['role']} phone={user['phone']}")

_, p = call("POST", f"/kb/articles/" + enc(slug) + "/like", token=user_token, expect=200)
results.append(p["data"]["liked"] is True and p["data"]["likeCount"] == 33)
print(f"     首次点赞 -> liked={p['data']['liked']} count={p['data']['likeCount']}")

_, p = call("POST", f"/kb/articles/" + enc(slug) + "/like", token=user_token, expect=200)
results.append(p["data"]["liked"] is False and p["data"]["likeCount"] == 32)
print(f"     再次点击 -> liked={p['data']['liked']} count={p['data']['likeCount']}（取消）")

call("POST", f"/kb/articles/" + enc(slug) + "/like", token=user_token, expect=200)
_, p = call("GET", f"/kb/articles/" + enc(slug) + "", token=user_token, expect=200)
results.append(p["data"]["liked"] is True)
print(f"     登录态详情 liked={p['data']['liked']}")

print("--- 3. 后台：权限与内容管理 ---")
call("GET", "/admin/kb/articles", token=user_token, expect=403, label="普通用户访问后台被拒")

admin_token, admin = login("13800000000")
print(f"     管理员: id={admin['id']} role={admin['role']}")
_, p = call("GET", "/admin/kb/articles", token=admin_token, expect=200)
print(f"     后台列表 total={p['data']['total']}")
results.append(p["data"]["list"][0]["status"] == "published")

_, p = call("POST", "/admin/kb/articles", {
    "title": "Redis 大 key 排查",
    "content": "## 现象\n\n延迟毛刺，p99 抖动。\n\n## 定位\n\n`redis-cli --bigkeys`。",
    "categoryId": 2,
    "tags": ["Redis", "性能", "redis"],
}, token=admin_token, expect=201)
draft = p["data"]
print(f"     建草稿 id={draft['id']} slug={draft['slug']} status={draft['status']}")
results.append(draft["status"] == "draft")
results.append(draft["slug"] == "redis-大-key-排查")
results.append(draft["tags"] == ["Redis", "性能"])
print(f"     标签（忽略大小写去重）: {draft['tags']}")

call("GET", f"/kb/articles/" + enc(draft['slug']) + "", expect=404, label="草稿对前台不可见")

_, p = call("POST", f"/admin/kb/articles/" + enc(draft['id']) + "/publish", token=admin_token, expect=200)
results.append(p["data"]["status"] == "published" and bool(p["data"]["publishedAt"]))
print(f"     发布 -> status={p['data']['status']} seo.keywords={p['data']['seo']['keywords']}")

call("GET", f"/kb/articles/" + enc(draft['slug']) + "", expect=200, label="发布后前台可见")
call("POST", f"/admin/kb/articles/" + enc(draft['id']) + "/publish", token=admin_token, expect=409, label="重复发布被状态机拒绝")

_, p = call("PUT", f"/admin/kb/articles/" + enc(draft['id']) + "", {
    "title": "Redis 大 key 排查（修订）",
    "summary": "手动摘要",
}, token=admin_token, expect=200)
results.append(p["data"]["title"].endswith("（修订）") and p["data"]["summary"] == "手动摘要")
results.append(p["data"]["slug"] == draft["slug"])
print(f"     slug 未被标题变更影响 -> {p['data']['slug']}")
print(f"     局部更新 -> title={p['data']['title']} slug={p['data']['slug']} summary={p['data']['summary']}")

call("PUT", f"/admin/kb/articles/" + enc(draft['id']) + "", {"title": "短"}, token=admin_token, expect=400, label="标题过短被拒")
call("POST", "/admin/kb/articles", {"title": "占用测试", "slug": "redis-大-key-排查", "content": "x"},
     token=admin_token, expect=409, label="显式 slug 冲突")

_, p = call("POST", f"/admin/kb/articles/" + enc(draft['id']) + "/offline", token=admin_token, expect=200)
results.append(p["data"]["status"] == "offline")
call("GET", f"/kb/articles/" + enc(draft['slug']) + "", expect=404, label="下线后前台不可见")
call("POST", f"/admin/kb/articles/" + enc(draft['id']) + "/publish", token=admin_token, expect=200, label="已下线可重新发布")
call("DELETE", f"/admin/kb/articles/" + enc(draft['id']) + "", token=admin_token, expect=200)
call("GET", f"/admin/kb/articles/" + enc(draft['id']) + "", token=admin_token, expect=404, label="删除后不可读")
call("GET", f"/kb/articles/" + enc(draft['slug']) + "", expect=404)

print("--- 4. 后台：分类管理 ---")
# 刻意用一个没被种子占用的名字，否则 201 会变成 409
_, p = call("POST", "/admin/kb/categories", {"name": "服务治理", "sort": 60}, token=admin_token, expect=201)
newcat = p["data"]
print(f"     新建分类 id={newcat['id']} slug={newcat['slug']} enabled={newcat['enabled']}")
results.append(newcat["slug"] == "服务治理" and newcat["enabled"])

call("POST", "/admin/kb/categories", {"name": "服务治理"}, token=admin_token, expect=409, label="分类重名被拒")
# slug 从库里现取一条，别写死：分类改过名（k8s -> cloud-native），
# 写死旧 slug 会让这条断言在改名后变成"创建成功"，红得莫名其妙。
_, p = call("GET", "/kb/categories", expect=200)
taken_slug = p["data"]["list"][0]["slug"]
call("POST", "/admin/kb/categories", {"name": "另一个分类", "slug": taken_slug}, token=admin_token, expect=409, label="分类 slug 冲突被拒")
call("DELETE", "/admin/kb/categories/1", token=admin_token, expect=409, label="分类下有文章禁删")

_, p = call("PUT", "/admin/kb/categories/" + enc(newcat["id"]), {"enabled": False, "sort": 99}, token=admin_token, expect=200)
results.append(p["data"]["enabled"] is False)
print(f"     停用分类 -> enabled={p['data']['enabled']} sort={p['data']['sort']}")

_, p = call("GET", "/kb/categories", expect=200)
slugs = [c["slug"] for c in p["data"]["list"]]
results.append("服务治理" not in slugs)
print(f"     前台分类（应不含已停用）: {slugs}")

call("DELETE", "/admin/kb/categories/" + enc(newcat["id"]), token=admin_token, expect=200, label="空分类可删")
call("GET", "/admin/kb/articles/999999", token=admin_token, expect=404)
call("GET", "/kb/articles?sort=bogus", expect=400, label="非法排序参数被拒")

print()
print("--- 5. 栏目导航 ---")
_, p = call("GET", "/channels", expect=200)
channels = p["data"]["list"]
print("     栏目:", [(c["slug"], c["kind"], c["count"]) for c in channels])
results.append([c["slug"] for c in channels] == [
    "linux-tutorial", "ops-quiz", "ops-news", "open-source-tools", "resources",
])
results.append(all(c["kind"] in ("article", "quiz", "tool", "resource") for c in channels))
results.append(all(c["count"] >= 0 for c in channels))
# 教程与资讯共用文章表，靠 channel 字段区分
_, p = call("GET", "/kb/articles?channel=linux-tutorial", expect=200)
tutorial_total = p["data"]["total"]
_, p = call("GET", "/kb/articles?channel=ops-news", expect=200)
news_total = p["data"]["total"]
print(f"     教程 {tutorial_total} 篇 / 资讯 {news_total} 条（资讯本期只做人工录入）")
# 这里假设资讯栏目为空：新库没有种子资讯，且本套件由 smoke.sh 以 SCHEDULER_ENABLED=false 启动，
# 不会跑到一半被抓取任务写进几十条。若单独手工起服务跑本脚本，抓取任务会把这条断言弄红。
results.append(tutorial_total >= 1 and news_total == 0)

print("--- 6. 开源工具推荐 ---")
_, p = call("GET", "/tools", expect=200)
tools = p["data"]["list"]
print(f"     列表 total={p['data']['total']} 首个={tools[0]['name']} ({tools[0]['language']})")
check("content" not in tools[0], "列表不该带正文")
_, p = call("GET", "/tools/filters", expect=200)
filters = p["data"]
print(f"     筛选项 languages={filters['languages']} licenses={filters['licenses']}")
results.append(len(filters["deployModes"]) == 6)
results.append(len(filters["languages"]) >= 1)

_, p = call("GET", "/tools?language=Go&sort=stars", expect=200)
stars = [t["stars"] for t in p["data"]["list"]]
results.append(stars == sorted(stars, reverse=True))
print(f"     language=Go 按 Star 降序: {stars}")

tool_slug = tools[0]["slug"]
_, p = call("GET", "/tools/" + enc(tool_slug), expect=200)
det = p["data"]
print(f"     详情: {det['name']} 阅读={det['viewCount']} related={len(det['related'])}")
_, p = call("GET", "/tools/" + enc(tool_slug), expect=200)
check(p["data"]["viewCount"] == det["viewCount"] + 1, "浏览数自增")
call("GET", "/tools/not-exist-tool", expect=404)

print("--- 7. 后台录入：工具 ---")
_, p = call("POST", "/admin/kb/tools", {
    "name": "Terraform",
    "summary": "基础设施即代码，用声明式配置管理云资源。",
    "content": "## 为什么用它\n\n把控制台里点出来的资源变成可评审、可回滚的代码。",
    "repoUrl": "https://github.com/hashicorp/terraform",
    "homepageUrl": "https://developer.hashicorp.com/terraform",
    "license": "BUSL-1.1",
    "language": "Go",
    "deployModes": ["binary", "docker", "cloud"],
    "tags": ["IaC", "云原生"],
    "status": "published",
}, token=admin_token, expect=201)
newtool = p["data"]
print(f"     新建 id={newtool['id']} slug={newtool['slug']} status={newtool['status']}")
results.append(newtool["status"] == "published")
results.append(newtool["deployModes"] == ["binary", "docker", "cloud"])

_, p = call("GET", "/tools/filters", expect=200)
check("Go" in p["data"]["languages"], "新语言应自动进入筛选项")
results.append("BUSL-1.1" in p["data"]["licenses"])

call("POST", "/admin/kb/tools", {"name": "恶意工具", "repoUrl": "javascript:alert(1)"},
     token=admin_token, expect=400, label="javascript: 协议被拒")
call("POST", "/admin/kb/tools", {"name": "写错部署方式", "deployModes": ["k8s"]},
     token=admin_token, expect=400, label="非白名单部署方式被拒")
call("POST", "/admin/kb/tools", {"name": "无正文直接发布", "content": "", "status": "published"},
     token=admin_token, expect=400, label="空正文不能直接发布")
call("POST", "/admin/kb/tools", {"name": "普通用户不能建工具"}, token=user_token, expect=403)

# 建的测试工具用完即删，避免重复跑脚本时越积越多
call("DELETE", "/admin/kb/tools/" + enc(newtool["id"]), token=admin_token, expect=200, label="清理测试工具")

print("--- 8. 资源下载 ---")
_, p = call("GET", "/resources", expect=200)
resources = p["data"]["list"]
print(f"     列表 total={p['data']['total']} 首个={resources[0]['title']}")
check(all("fileKey" not in r for r in resources), "列表不该暴露存储 key")
_, p = call("GET", "/resources/file-exts", expect=200)
print(f"     文件类型: {p['data']['list']}")
results.append(len(p["data"]["list"]) >= 1)

# 资源现在有两种形态：托管文件的、只给官网/镜像外链的。
# 下载相关断言必须挑**真的有文件**的那类，否则会去请求一份本来就没文件的资源。
link_only = next((r for r in resources if not r.get("hasFile")), None)
hosted = [r for r in resources if r.get("hasFile")]
open_res = next((r for r in hosted if not r["needLogin"]), None)
locked_res = next((r for r in hosted if r["needLogin"]), None)

if open_res:
    _, p = call("GET", "/resources/" + enc(open_res["slug"]) + "/download", expect=200)
    dl = p["data"]
    print(f"     免登录资源 -> {dl['fileName']} {dl['fileSize']}B checksum={dl['checksum'][:16]}…")
    results.append(len(dl["checksum"]) == 64)
    results.append("s=" in dl["url"] and "e=" in dl["url"])
else:
    results.append(False)
    print("     [warn] 没有免登录资源可测")

if locked_res:
    # 未登录取下载地址应被拒（业务码 40031），这正是私有存储的意义
    call("GET", "/resources/" + enc(locked_res["slug"]) + "/download",
         expect=401, label="需登录资源对游客被拒")
    _, p = call("GET", "/resources/" + enc(locked_res["slug"]) + "/download",
                token=user_token, expect=200)
    results.append("s=" in p["data"]["url"])
    print(f"     需登录资源 -> 登录后放行，签名地址已签发")
else:
    results.append(False)
    print("     [warn] 没有 needLogin 资源可测")

if link_only:
    # 只给外链的资源不该能签发下载地址，且提示要指向"去点链接"而不是"重新上传"——
    # 用户按"重新上传"去操作是做不到的，那是运营的动作。
    _, p = call("GET", "/resources/" + enc(link_only["slug"]) + "/download", expect=400)
    results.append("外部下载地址" in p.get("message", ""))
    print(f"     仅外链资源 -> 拒发下载地址：{p.get('message')}")
    _, p = call("GET", "/resources/" + enc(link_only["slug"]), expect=200)
    results.append((p["data"].get("links") or []) != [])
    print(f"     仅外链资源 -> 外链 {len(p['data'].get('links') or [])} 条")
else:
    results.append(False)
    results.append(False)
    print("     [warn] 没有仅外链的资源可测")

_, p = call("POST", "/admin/kb/resources", {"title": "没有文件的资源"}, token=admin_token,
            expect=400, label="缺 fileKey 被拒")
call("POST", "/admin/kb/resources", {"title": "伪造场景", "fileKey": "cert/202510/whatever.txt"},
     token=admin_token, expect=400, label="非资源场景的 key 被拒")

print("--- 9. 运维题库：抽题与判分 ---")
# 用一个全新账号，统计从零开始，断言才能写确切数字
quiz_token, quiz_user = login("13800008888", "刷题的人")
print(f"     考生: id={quiz_user['id']}")

_, p = call("GET", "/quiz/practice?count=3", expect=200)
questions = p["data"]["list"]
print(f"     抽到 {len(questions)} 题，题型={[q['type'] for q in questions]}")
results.append(len(questions) == 3)
check(all("correct" not in q and "analysis" not in q for q in questions), "抽题结果不能带答案")
q1, q2 = questions[0], questions[1]

# 后台能看到答案，前台看不到——这正是两条路径该有的差别
_, p = call("GET", "/admin/kb/quiz/questions/" + enc(q1["id"]), token=admin_token, expect=200)
correct1 = p["data"]["correct"]
print(f"     后台可见答案 q{q1['id']} -> {correct1}")
results.append(len(correct1) >= 1)
check(q1["type"] != "multiple" or len(correct1) < len(q1["options"]), "多选答案不该覆盖全部选项")
wrong1 = [o["key"] for o in q1["options"] if o["key"] not in correct1]

_, p = call("POST", "/quiz/questions/" + enc(q1["id"]) + "/answer",
            {"chosen": correct1}, token=quiz_token, expect=200)
ans = p["data"]
print(f"     答对 -> correct={ans['correct']} fresh={ans['fresh']} 解析={ans['analysis'][:14]}…")
results.append(ans["correct"] is True and ans["fresh"] is True)
results.append(ans["analysis"] != "")

_, p = call("GET", "/quiz/stats", token=quiz_token, expect=200)
print(f"     统计 {p['data']}")
results.append(p["data"]["totalAnswered"] == 1 and p["data"]["totalCorrect"] == 1)
results.append(p["data"]["accuracy"] == 100)

_, p = call("POST", "/quiz/questions/" + enc(q1["id"]) + "/answer",
            {"chosen": wrong1[:1]}, token=quiz_token, expect=200)
print(f"     改答错 -> correct={p['data']['correct']} fresh={p['data']['fresh']}")
check(p["data"]["correct"] is False and p["data"]["fresh"] is False, "重复作答不该再标记 fresh")
_, p = call("GET", "/quiz/stats", token=quiz_token, expect=200)
print(f"     统计 {p['data']}")
# 同一题刷两遍仍只算一题；最近一次答错，所以错题数为 1
results.append(p["data"]["totalAnswered"] == 1 and p["data"]["totalCorrect"] == 0)
results.append(p["data"]["wrongCount"] == 1)

_, p = call("GET", "/quiz/wrong-book", token=quiz_token, expect=200)
results.append(p["data"]["total"] == 1)
print(f"     错题本 total={p['data']['total']} 首条选项数={len(p['data']['list'][0]['options'])}")
check(len(p["data"]["list"][0]["options"]) >= 2, "错题本应自带选项便于复习")
# 改对之后应从错题本消失
call("POST", "/quiz/questions/" + enc(q1["id"]) + "/answer", {"chosen": correct1}, token=quiz_token, expect=200)
_, p = call("GET", "/quiz/wrong-book", token=quiz_token, expect=200)
check(p["data"]["total"] == 0, "答对后不该再挂在错题本里")
print(f"     改对后错题本 total={p['data']['total']}")

_, p = call("GET", "/admin/kb/quiz/questions/" + enc(q2["id"]), token=admin_token, expect=200)
correct2 = p["data"]["correct"]

_, p = call("POST", "/quiz/attempts", {
    "durationSec": 95,
    "items": [
        {"questionId": q1["id"], "chosen": correct1},
        {"questionId": q2["id"], "chosen": correct2},
    ],
}, token=quiz_token, expect=201)
sheet = p["data"]
print(f"     整卷 2 题 -> {sheet['correctCount']}/{sheet['questionCount']} 得分 {sheet['score']}")
results.append(sheet["questionCount"] == 2 and sheet["correctCount"] == 2 and sheet["score"] == 100)

_, p = call("POST", "/quiz/attempts", {
    "items": [
        {"questionId": q1["id"], "chosen": correct1},
        {"questionId": q2["id"], "chosen": [o["key"] for o in q2["options"] if o["key"] not in correct2][:1]},
    ],
}, token=quiz_token, expect=201)
results.append(p["data"]["questionCount"] == 2 and p["data"]["score"] == 50)
print(f"     一题对一题错 -> 得分 {p['data']['score']}")

_, p = call("GET", "/quiz/attempts", token=quiz_token, expect=200)
results.append(len(p["data"]["list"]) == 2)
print(f"     历史 {len(p['data']['list'])} 条")

call("POST", "/quiz/questions/" + enc(q1["id"]) + "/answer", {"chosen": ["Z"]},
     token=quiz_token, expect=400, label="不存在的选项被拒")

# 「单选传两个答案必须被拒」这条规则不能挂在随机抽题的结果上——
# 抽到多选题时它就悄悄不跑了，断言总数会随抽签在 168/169 之间浮动，
# 看起来像"少验了一条"却查不出原因。这里显式取一道单选题来验。
_, p = call("GET", "/quiz/questions?" + urllib.parse.urlencode({"type": "single", "pageSize": 1}),
            expect=200)
single = p["data"]["list"][0]
call("POST", "/quiz/questions/" + enc(single["id"]) + "/answer",
     {"chosen": [o["key"] for o in single["options"][:2]]},
     token=quiz_token, expect=400, label="单选题传两个答案被拒")

call("POST", "/quiz/questions/" + enc(q1["id"]) + "/answer", {"chosen": correct1},
     expect=401, label="未登录不能作答")

print("--- 10. 后台录入：题目 ---")
# 判断题整个省略 options：选项固定是「正确 / 错误」，由服务端填
_, p = call("POST", "/admin/kb/quiz/questions", {
    "stem": "判断：Kubernetes 中 Pod 是最小的可调度单元。",
    "type": "judge",
    "correct": ["A"],
    "analysis": "正确。Pod 是 K8s 中最小的可部署与调度单元，容器运行在 Pod 之内。",
    "difficulty": "easy",
    "status": "published",
}, token=admin_token, expect=201)
judge = p.get("data")
if isinstance(judge, dict):
    print(f"     判断题 id={judge['id']} 选项={[o['text'] for o in judge['options']]}")
    check([o["text"] for o in judge["options"]] == ["正确", "错误"],
          "判断题选项固定为正确/错误")

call("POST", "/admin/kb/quiz/questions", {
    "stem": "多选题答案覆盖全部选项应被拒",
    "type": "multiple",
    "options": [{"key": "A", "text": "甲"}, {"key": "B", "text": "乙"}],
    "correct": ["A", "B"],
}, token=admin_token, expect=400, label="多选答案覆盖全部选项被拒")

call("POST", "/admin/kb/quiz/questions", {
    "stem": "答案指向不存在的选项应被拒",
    "type": "single",
    "options": [{"key": "A", "text": "甲"}, {"key": "B", "text": "乙"}],
    "correct": ["Z"],
}, token=admin_token, expect=400, label="答案指向不存在的选项被拒")

call("POST", "/admin/kb/quiz/questions", {
    "stem": "单选题只给一个选项应被拒",
    "type": "single",
    "options": [{"key": "A", "text": "甲"}],
    "correct": ["A"],
}, token=admin_token, expect=400, label="选项数量不足被拒")

if isinstance(judge, dict):
    call("DELETE", "/admin/kb/quiz/questions/" + enc(judge["id"]), token=admin_token,
         expect=200, label="清理测试题目")

print("--- 11. 检索：用完整问句也能搜到 ---")
_, p = call("GET", "/kb/search/suggest?q=" + enc("磁盘满了怎么办"), expect=200)
plan = p["data"]
print(f"     terms={plan['terms']} expanded={plan['expanded']}")
check(plan["terms"] == ["磁盘"], "疑问词与语气词被剔除，只留下「磁盘」")
check(isinstance(plan["expanded"], list) and len(plan["expanded"]) > 1, "同义词被展开")
check(isinstance(plan["hot"], list) and len(plan["hot"]) > 0, "附带热门关键词")

# 核心回归：改之前整句 LIKE 一条都搜不到，只能得到空页面。
_, p = call("GET", "/kb/articles?keyword=" + enc("磁盘满了怎么办"), expect=200)
question_hits = p["data"]["total"]
_, p = call("GET", "/kb/articles?keyword=" + enc("磁盘"), expect=200)
token_hits = p["data"]["total"]
print(f"     问句命中 {question_hits} 条 / 关键词命中 {token_hits} 条")
check(question_hits > 0, "完整问句能搜到内容（不再是空页）")

# 中英混排是最容易漏的一类：整串按汉字查词典时，英文前缀会被静默跳过，
# 「cpu 占用高」里的 cpu 就提不出来。切开各按各的规则走才对。
_, p = call("GET", "/kb/search/suggest?q=" + enc("cpu占用高怎么办"), expect=200)
print(f"     terms={p['data']['terms']}")
check("cpu" in p["data"]["terms"], "中英混排里的英文部分被提出来")
check("占用" in p["data"]["terms"], "「占用高」被切成「占用」，不是整串保留")

# 五个栏目都要吃到这层解析，任意一路漏接都会在这里露出来。
# 题库用「内存告警」：库里有 Redis 内存告警与 OOMKilled 两道题，命中数必须大于 0，
# 只断言 total 是个整数的话，接了但接错（永远返回 0）也看不出来。
for path, label in [
    ("/tools?keyword=" + enc("k8s集群怎么部署"), "工具"),
    ("/resources?keyword=" + enc("nginx配置怎么写"), "资源"),
    ("/quiz/questions?keyword=" + enc("内存告警怎么处理"), "题目"),
]:
    _, p = call("GET", path, expect=200)
    hits = p["data"]["total"]
    print(f"     {label}：命中 {hits} 条")
    check(hits > 0, f"{label}栏目检索可用")

# 英文问句同样要能落地：停用词剔掉，剩下的词展开成中文。
_, p = call("GET", "/kb/search/suggest?q=" + enc("how to fix oom"), expect=200)
print(f"     terms={p['data']['terms']} expanded={p['data']['expanded'][:5]}")
check("how" not in p["data"]["terms"], "英文停用词被剔除")
check("内存" in p["data"]["expanded"], "英文同义词展开成中文")

# 解析不出检索词时必须落到「整串匹配」，而不是「不过滤」——
# 后者会让语气词输入静默返回全站内容，比搜不到更容易被误判成搜索坏了。
_, p = call("GET", "/kb/articles?keyword=" + enc("的了吗呢"), expect=200)
print(f"     语气词输入命中 {p['data']['total']} 条")
check(p["data"]["total"] == 0, "全是语气词的输入返回空结果，而不是全站内容")

print()
passed, total = sum(1 for r in results if r), len(results)
print(f"===== 知识库冒烟结果：{passed}/{total} 通过 =====")
if passed != total:
    sys.exit(1)
