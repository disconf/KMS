# -*- coding: utf-8 -*-
"""运维题库批量生成引擎。

数据与逻辑分离：bank/<方向>.py 只提供「事实」，本文件负责把事实组合成题目。
事实分五类，各走一套模板：

  cmd     命令/工具   → 「哪个命令做 X」「命令 X 做什么」
  flag    参数语义    → 「X 的 -Y 参数作用」
  symptom 故障现象    → 「现象 → 最可能原因」
  config  配置项语义  → 「配置项 X 的含义」
  concept 概念判断    → 陈述句判对/判错

每条事实出 2~4 道题（视角不同、干扰项不同），干扰项从同方向同 kind 的
事实池里抽——错的选项也是真命令/真概念，不会出现一眼假货。

输出 internal/bootstrap/data/quiz_bank.json，由 Go 种子灌库。

用法：
  python scripts/quizgen/generate.py            # 生成并打印各方向统计
  python scripts/quizgen/generate.py --min 1000 # 每方向不足 1000 报错退出
"""
import argparse
import importlib
import json
import pathlib
import random
import sys

ROOT = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
OUT = ROOT.parent.parent / "internal" / "bootstrap" / "data" / "quiz_bank.json"

# 每方向固定随机种子：同版本事实生成结果完全一致，方便 diff 与幂等。
SEED = 20261023

BANK_TAG = "系统题库"

# ---------------------------------------------------------------- 事实结构 --
# bank 文件约定导出三个变量：
#   CAT      = "linux"                      # 分类 slug
#   HAND     = [ (stem, opts, correct_idx, analysis, diff, tags), ... ]
#   FACTS    = [ {"kind":..., ...}, ... ]

DIFF_POOL = ("easy", "medium", "hard")


def pick_diff(rng, kind):
    """按事实类型给难度：cmd/flag 偏易，symptom/config 偏难，concept 中间。"""
    if kind in ("cmd", "flag"):
        return rng.choices(["easy", "easy", "medium", "hard"], weights=[3, 3, 3, 1])[0]
    if kind in ("symptom", "config"):
        return rng.choices(["easy", "medium", "medium", "hard"], weights=[1, 3, 3, 2])[0]
    return rng.choices(["easy", "medium", "hard"], weights=[2, 4, 1])[0]


def make_options(rng, correct_text, pool):
    """从 pool 抽 3 个干扰项。pool 元素是字符串；不足时允许重复使用但保证不同。"""
    opts, seen = [], {correct_text}
    shuffled = pool[:]
    rng.shuffle(shuffled)
    for t in shuffled:
        if t not in seen:
            opts.append(t)
            seen.add(t)
        if len(opts) == 3:
            break
    # 池子太小时兜底：补通用错误项，宁可朴素也不能选项少于 4 个。
    fallback = ["以上都不是", "该操作与题意无关", "需要重启后才能生效"]
    for t in fallback:
        if len(opts) == 3:
            break
        if t not in seen:
            opts.append(t)
            seen.add(t)
    return opts


KEYS = "ABCDEFGH"


def mc(rng, stem, correct_text, distractor_pool, analysis, diff, tags):
    """组装一道单选题，正确项位置随机。"""
    opts = make_options(rng, correct_text, distractor_pool)
    idx = rng.randrange(len(opts) + 1)
    opts.insert(idx, correct_text)
    return {
        "stem": stem,
        "type": "single",
        "options": [{"key": KEYS[i], "text": t} for i, t in enumerate(opts)],
        "correct": [KEYS[idx]],
        "analysis": analysis,
        "difficulty": diff,
        "tags": tags,
    }


def judge(rng, statement, analysis, diff, tags, negative=None, force=None):
    """判断题固定 A=正确 B=错误，答案是陈述真值。

    参数：
      statement  陈述本身
      negative   它的否定写法（可选）。早先是把肯定句扔给一条「是→不是」的字符串
                 替换去翻，遇到「…里全是 Sending data 的典型原因是 Y」这种句子，
                 第一个「是」在症状里，翻出来就是「里全不是 Sending data 的典型原因
                 是 Y」——症状本身被改坏，读完不知道在问什么。所以否定式必须由
                 模板自己写；给不出就只出肯定陈述（答案恒为「正确」）。
      force      强制真值。概念事实的 `wrong` 字段本身就是一句错话，答案必须是
                 「错误」；以前把这句错话再翻一次、再随机一次，等于把错误陈述
                 标成「正确」，是答案键错误，不只是文案难看。

    rng 照旧抽一次：不给否定式、或给了 force 也占掉一次随机数，保证同一份事实
    生成的其余题目不受影响（否则改一处模板会让整库题干全部漂移）。
    """
    roll = rng.random()
    if force is not None:
        truth = force
    elif negative is None:
        truth = True
    else:
        truth = roll < 0.5
    text = statement if truth else (negative or statement)
    return {
        "stem": text,
        "type": "judge",
        "options": [
            {"key": "A", "text": "正确"},
            {"key": "B", "text": "错误"},
        ],
        "correct": ["A" if truth else "B"],
        "analysis": analysis,
        "difficulty": diff,
        "tags": tags,
    }


# ---------------------------------------------------------------- 生成主体 --

def questions_from_fact(rng, fact, pools):
    """一条事实展开成若干道题。返回题目列表。"""
    kind = fact["kind"]
    topic = fact.get("topic", "")
    tags = [t for t in [topic, BANK_TAG] if t]
    out = []

    if kind == "cmd":
        subject, desc = fact["name"], fact["desc"]
        pool = [f"{f['name']}" for f in pools["cmd"] if f["name"] != subject]
        detail = fact.get("detail", desc)
        out.append(mc(rng, f"下列哪个命令/工具用于{desc}？", subject, pool,
                      f"{subject}：{detail}。其余选项是同领域其他命令，用途与此不符。",
                      pick_diff(rng, kind), tags))
        pool2 = [f["desc"] for f in pools["cmd"] if f["name"] != subject]
        out.append(mc(rng, f"命令 `{subject}` 的主要用途是？", desc, pool2,
                      f"{subject}：{detail}。",
                      pick_diff(rng, kind), tags))
        out.append(judge(rng, f"`{subject}` 用于{desc}。",
                         f"`{subject}` 的用途是{desc}。",
                         pick_diff(rng, kind), tags,
                         negative=f"`{subject}` 并不用于{desc}。"))
        if fact.get("detail"):
            # 干扰项用其他命令的 detail：都是「正确的命令要点」，
            # 张冠李戴本身就是干扰，不会出现「以上都不是」这种凑数选项。
            pool4 = [f["detail"] for f in pools["cmd"]
                     if f is not fact and f.get("detail")]
            if len(pool4) >= 3:
                out.append(mc(rng, f"使用 `{subject}` 时，下列说法正确的是？", detail,
                              pool4, f"`{subject}` 的要点：{detail}",
                              pick_diff(rng, kind), tags))

    elif kind == "flag":
        subject, flag, meaning = fact["name"], fact["flag"], fact["meaning"]
        full = f"{subject} {flag}"
        pool = [f"{f['name']} {f['flag']}" for f in pools["flag"] if f["flag"] != flag]
        out.append(mc(rng, f"`{full}` 这个参数的作用是？", meaning, pool,
                      f"{full}：{meaning}。", pick_diff(rng, kind), tags))
        # 「想…应该用哪个参数」必须在题干里点明是哪个命令/程序：
        # 只写「应当使用哪个参数」，而干扰项来自别的工具，题目就没有唯一解。
        # 干扰项也优先取同一命令的其他参数（同一套参数风格），不足再跨命令补。
        same_cmd = [f["flag"] for f in pools["flag"]
                    if f["name"] == subject and f["flag"] != flag]
        other_cmd = [f["flag"] for f in pools["flag"]
                     if f["name"] != subject and f["flag"] != flag]
        out.append(mc(rng, f"在 `{subject}` 中，想{fact.get('goal', meaning)}，应当使用哪个参数？",
                      flag, same_cmd + other_cmd,
                      f"{subject} 中，{flag} 表示「{meaning}」。",
                      pick_diff(rng, kind), tags))

    elif kind == "symptom":
        symptom, cause = fact["symptom"], fact["cause"]
        pool = [f["cause"] for f in pools["symptom"] if f["cause"] != cause]
        out.append(mc(rng, f"{symptom}，最可能的原因是？", cause, pool,
                      fact.get("analysis", f"{symptom} 对应的典型原因是：{cause}"),
                      pick_diff(rng, kind), tags))
        if fact.get("action"):
            pool3 = [f.get("action") for f in pools["symptom"]
                     if f.get("action") and f["cause"] != cause]
            if len(pool3) >= 3:
                out.append(mc(rng, f"{symptom}，下列哪个处置动作最合适？", fact["action"],
                              pool3, fact.get("analysis", f"该现象的正确处置是：{fact['action']}"),
                              pick_diff(rng, kind), tags))
        out.append(judge(rng, f"{symptom}的典型原因是{cause}。",
                         fact.get("analysis", f"{symptom} 的典型原因确实是：{cause}"),
                         pick_diff(rng, kind), tags,
                         negative=f"{symptom}的典型原因并不是{cause}。"))

    elif kind == "config":
        name, meaning = fact["name"], fact["meaning"]
        where = fact.get("where", "")
        q = f"配置项 `{name}`" + (f"（{where}）" if where else "")
        pool = [f["meaning"] for f in pools["config"] if f["name"] != name]
        out.append(mc(rng, f"{q}的作用是？", meaning, pool,
                      f"{name}：{meaning}", pick_diff(rng, kind), tags))
        out.append(judge(rng, f"{q}用于{meaning}。",
                         f"{name} 的作用是：{meaning}",
                         pick_diff(rng, kind), tags,
                         negative=f"{q}并不用于{meaning}。"))

    elif kind == "concept":
        statement, truth = fact["statement"], fact.get("truth", True)
        diff = pick_diff(rng, kind)
        if truth:
            out.append(judge(rng, statement, fact["analysis"], diff, tags, force=True))
            if fact.get("wrong"):
                # wrong 本身就是错误陈述：答案固定「错误」，不能再翻一次。
                out.append(judge(rng, fact["wrong"], fact["analysis"], diff, tags, force=False))
        # 概念事实不再出选择题：陈述句本身信息量足够，重复包装反而生硬。

    return out


def dedup(questions):
    seen, out = set(), []
    for q in questions:
        key = q["stem"]
        if key in seen:
            continue
        seen.add(key)
        out.append(q)
    return out


def expand_category(mod, rng):
    cat = mod.CAT
    facts = list(mod.FACTS)
    pools = {}
    for kind in ("cmd", "flag", "symptom", "config"):
        pools[kind] = [f for f in facts if f["kind"] == kind]

    questions = []
    # 精写题直接落。
    for stem, opts, correct_idx, analysis, diff, tags in mod.HAND:
        questions.append({
            "stem": stem,
            "type": "judge" if len(opts) == 2 and opts[0] in ("正确", "错误") else
                    ("multiple" if len(correct_idx) > 1 else "single"),
            "options": [{"key": KEYS[i], "text": t} for i, t in enumerate(opts)],
            "correct": [KEYS[i] for i in correct_idx],
            "analysis": analysis,
            "difficulty": diff,
            "tags": list(dict.fromkeys(list(tags) + [BANK_TAG]))[:5],
        })
    for fact in facts:
        questions.extend(questions_from_fact(rng, fact, pools))
    return cat, dedup(questions)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--min", type=int, default=0, help="每方向最少题数，达不到则报错")
    args = ap.parse_args()

    rng = random.Random(SEED)
    bank = {}
    for f in sorted((ROOT / "bank").glob("*.py")):
        if f.name.startswith("_"):
            continue
        mod = importlib.import_module(f"bank.{f.stem}")
        cat, questions = expand_category(mod, rng)
        # 同一方向可以拆多个文件（主文件 + 补充），合并到一起。
        if cat in bank:
            bank[cat].extend(questions)
        else:
            bank[cat] = questions

    # 统计与去重都放在所有文件读完之后：边读边打印会把前面文件的题目重复计入
    # （总数虚高一截），而题干重复（两条事实的 desc 写成同一句话，模板拼出来
    # 一模一样）也只有在合并后才看得出来。同一方向内同名题干只留先出现的——
    # 库里挂两条一模一样的题干，刷题的人会以为系统出错了。
    for cat in list(bank):
        seen, uniq = set(), []
        for q in bank[cat]:
            if q["stem"] in seen:
                continue
            seen.add(q["stem"])
            uniq.append(q)
        dropped = len(bank[cat]) - len(uniq)
        if dropped:
            print(f"  ! {cat} 丢弃 {dropped} 道题干重复的题（改 fact 的 desc 让它有区分度）",
                  file=sys.stderr)
        bank[cat] = uniq

    for cat, questions in sorted(bank.items()):
        hard = sum(1 for q in questions if q["difficulty"] == "hard")
        judge_n = sum(1 for q in questions if q["type"] == "judge")
        multi = sum(1 for q in questions if q["type"] == "multiple")
        print(f"{cat:16s} {len(questions):5d} 题  (多选 {multi} / 判断 {judge_n} / 难题 {hard})")

    # 最低题量校验放在合并之后：同一方向可能拆了多个文件（主文件 + 补充），
    # 按单文件校验会把主文件误判为不达标。
    if args.min:
        short = {c: len(qs) for c, qs in bank.items() if len(qs) < args.min}
        if short:
            for c, n in sorted(short.items()):
                print(f"  !! {c} 不足 {args.min} 题，缺 {args.min - n} 条——补 FACTS", file=sys.stderr)
            sys.exit(1)

    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(json.dumps(
        {"version": 1, "generated": "scripts/quizgen/generate.py", "categories": bank},
        ensure_ascii=False, separators=(",", ":")), encoding="utf-8")
    # 总数按合并后的每个方向的最终长度求和：同一方向拆了多个文件时，
    # 循环里累加会把前面文件的题目重复计入（表现为总数虚高一截）。
    total = sum(len(qs) for qs in bank.values())
    print(f"共 {total} 题 -> {OUT.relative_to(ROOT.parent.parent)} "
          f"({OUT.stat().st_size // 1024} KB)")


if __name__ == "__main__":
    main()
