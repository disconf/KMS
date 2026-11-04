/* ============================================================================
   pages-exam.js — 模拟考：限时整卷，交卷才给答案，出成绩单与弱项分析。

   与练习模式（pages-quiz.js）的差别只有一条，但这条决定了整个交互：
   练习是"答一题判一题"，模拟考是"全部答完才判"。所以这里不能复用
   单题判分接口，只能最后一次性提交整卷——也因此中途改答案是被允许的，
   这一点跟真实笔试一致。
   ========================================================================= */

import { api, isLoggedIn, ApiError } from './api.js';
import {
  esc, icon, fmtDate, fmtDuration, fmtNum, DIFFICULTY_LABEL, QUIZ_TYPE_LABEL,
} from './util.js';
import { toastError, toastOk, openModal, closeModal } from './ui.js';

/**
 * 卷型定义。
 *
 * 题量上限 50 是服务端 /quiz/practice 的 count 上限，挑战卷贴着上限来；
 * 时间给的是"够用但不宽松"的配比（约 45~55 秒/题），比真实笔试稍紧，
 * 因为这里的题目以单行判断为主，不需要读长材料。
 */
export const EXAM_PAPERS = [
  { id: 'quick', name: '快速测', count: 10, minutes: 8, desc: '碎片时间摸个底' },
  { id: 'standard', name: '标准卷', count: 30, minutes: 20, desc: '模拟真实笔试节奏' },
  { id: 'challenge', name: '挑战卷', count: 50, minutes: 45, desc: '完整能力检验' },
];

/** 剩余时间进入最后两分钟时转为告警色。 */
const URGENT_SEC = 120;

/* ====================================================== 模拟考首页 ==== */

export async function examHomePage(ctx) {
  const logged = isLoggedIn();

  // 分类对游客同样有意义（能看出题库覆盖哪些方向），
  // 历史记录取不到就整块不渲染，不弹错。
  const [catRes, attemptRes] = await Promise.allSettled([
    api.quizCategories(),
    logged ? api.attempts({ page: 1, pageSize: 3 }) : null,
  ]);
  const cats = (pick(catRes, []) || []).filter((c) => c && c.name && Number(c.count) > 0);
  const attempts = (pick(attemptRes, { list: [] }).list || []).slice(0, 3);

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a>
      <span class="crumb-sep">/</span>
      <a href="#/quiz">运维题库</a>
      <span class="crumb-sep">/</span>
      <span>模拟考</span>
    </nav>

    <div class="page-head">
      <h1>模拟考</h1>
      <p class="lede">限时整卷、交卷才给答案。分数不是目的，弱项分析才是——
        成绩单会告诉你哪个方向拖了后腿，以及下一步该补哪一章。</p>
    </div>

    <div class="card card-pad exam-rules">
      <div class="exam-rule">
        <span class="exam-rule-ic">${icon('clock', 16)}</span>
        <div><strong>限时作答</strong><p>倒计时结束自动交卷，最后两分钟顶部计时会变红。</p></div>
      </div>
      <div class="exam-rule">
        <span class="exam-rule-ic">${icon('eye', 16)}</span>
        <div><strong>中途不给答案</strong><p>与练习模式不同，选完不打分，交卷后才有解析。</p></div>
      </div>
      <div class="exam-rule">
        <span class="exam-rule-ic">${icon('target', 16)}</span>
        <div><strong>弱项分析</strong><p>按方向统计正确率，标出最该补的方向。</p></div>
      </div>
    </div>

    <div class="section">
      <div class="section-head"><h2>选一份卷子</h2></div>
      <div class="exam-paper-grid" data-paper-grid>
        ${EXAM_PAPERS.map((p, i) => `
          <button class="exam-paper${i === 1 ? ' is-active' : ''}" type="button"
            data-paper="${esc(p.id)}" aria-pressed="${i === 1}">
            <span class="exam-paper-name">${esc(p.name)}</span>
            <span class="exam-paper-meta">${p.count} 题 · ${p.minutes} 分钟</span>
            <span class="exam-paper-desc">${esc(p.desc)}</span>
          </button>`).join('')}
      </div>
    </div>

    <div class="card card-pad" style="margin-bottom:24px">
      <div class="exam-opts">
        <label class="field" style="margin:0">
          <span class="label">考察方向</span>
          <select class="select" data-cat>
            <option value="">全部方向（混合出题）</option>
            ${cats.map((c) => `<option value="${esc(String(c.id))}">
              ${esc(c.name)}（${fmtNum(c.count)} 题）</option>`).join('')}
          </select>
        </label>
        <label class="field" style="margin:0">
          <span class="label">难度</span>
          <select class="select" data-diff>
            <option value="">不限难度</option>
            <option value="easy">简单</option>
            <option value="medium">中等</option>
            <option value="hard">困难</option>
          </select>
        </label>
        <button class="btn btn-primary btn-lg" type="button" data-start>
          ${icon('play', 15)}开始考试
        </button>
      </div>
      ${logged ? '' : `<p class="exam-guest">${icon('info', 13)}
        模拟考需要登录才能记录成绩与错题，点「开始考试」会先让你登录。
        只想随手练几题的话，<a href="#/quiz">去题库练习模式</a>不用登录也能玩。</p>`}
    </div>

    ${attempts.length ? `<div class="section">
      <div class="section-head">
        <h2>最近成绩</h2>
        <a class="more" href="#/quiz">查看全部历史</a>
      </div>
      ${attempts.map((a) => `<div class="card exam-history">
        <span class="badge">${esc(fmtDate(a.createdAt))}</span>
        <span>答对 ${fmtNum(a.correctCount)} / ${fmtNum(a.questionCount)} 题</span>
        ${a.durationSec ? `<span class="exam-history-time">${icon('clock', 12)}${esc(fmtDuration(a.durationSec))}</span>` : ''}
        <span class="chip ${scoreTone(a.score)}" style="margin-left:auto">${fmtNum(a.score)} 分</span>
      </div>`).join('')}
    </div>` : ''}
  `;

  const mount = (root) => {
    let paperId = 'standard';
    root.querySelectorAll('[data-paper]').forEach((btn) => {
      btn.addEventListener('click', () => {
        paperId = btn.getAttribute('data-paper');
        root.querySelectorAll('[data-paper]').forEach((b) => {
          const on = b === btn;
          b.classList.toggle('is-active', on);
          b.setAttribute('aria-pressed', String(on));
        });
      });
    });

    root.querySelector('[data-start]')?.addEventListener('click', () => {
      if (!isLoggedIn()) {
        window.dispatchEvent(new CustomEvent('opskb:need-login'));
        return;
      }
      const q = new URLSearchParams({ paper: paperId });
      const cat = root.querySelector('[data-cat]')?.value;
      const diff = root.querySelector('[data-diff]')?.value;
      if (cat) q.set('categoryId', cat);
      if (diff) q.set('difficulty', diff);
      // 带一个随机位：同一份卷子连考两次时 hash 若完全相同不会触发路由，
      // 页面就停在上一次的成绩单上，看起来像"点了没反应"。
      q.set('r', String(Date.now()));
      location.hash = `#/exam/run?${q.toString()}`;
    });
  };

  return { html, title: '模拟考', mount };
}

/* ====================================================== 答题页 ======== */

export async function examRunPage(ctx) {
  const paper = EXAM_PAPERS.find((p) => p.id === ctx.params.get('paper')) || EXAM_PAPERS[1];
  const categoryId = ctx.params.get('categoryId') || '';
  const difficulty = ctx.params.get('difficulty') || '';

  if (!isLoggedIn()) {
    return {
      html: needLoginHTML(paper),
      title: '模拟考',
      mount: (root) => {
        root.querySelector('[data-act="login"]')?.addEventListener('click', () => {
          window.dispatchEvent(new CustomEvent('opskb:need-login'));
        });
      },
    };
  }

  let questions = [];
  let loadError = '';
  try {
    const res = await api.practice({
      count: paper.count,
      categoryId: categoryId || undefined,
      difficulty: difficulty || undefined,
    });
    questions = res.list || [];
  } catch (err) {
    loadError = err instanceof ApiError ? err.message : '题目加载失败，请稍后再试';
  }

  if (loadError) {
    return {
      html: `<nav class="crumb"><a href="#/quiz">运维题库</a>
        <span class="crumb-sep">/</span><a href="#/exam">模拟考</a></nav>
        <div class="card card-pad">
          <p style="color:var(--danger)">${esc(loadError)}</p>
          <a class="btn" href="#/exam">重新选卷</a>
        </div>`,
      title: '模拟考',
    };
  }

  if (!questions.length) {
    return {
      html: `<nav class="crumb"><a href="#/quiz">运维题库</a>
        <span class="crumb-sep">/</span><a href="#/exam">模拟考</a></nav>
        <div class="card card-pad">
          <p>这个条件下没有可用的题目，换个方向或去掉难度限制试试。</p>
          <a class="btn" href="#/exam">重新选卷</a>
        </div>`,
      title: '模拟考',
    };
  }

  // 实际抽到的题数可能少于卷面题数（题库在该方向下不够），以实际为准。
  const html = `
    <div class="exam-bar" data-bar>
      <div class="exam-bar-left">
        <span class="exam-clock" data-clock>${fmtClock(paper.minutes * 60)}</span>
        <span class="exam-progress" data-progress>已答 0 / ${questions.length}</span>
      </div>
      <div class="exam-bar-right">
        <span class="exam-paper-tag">${esc(paper.name)}</span>
        <button class="btn btn-primary btn-sm" type="button" data-finish>交卷</button>
      </div>
    </div>

    <div class="exam-nums" data-nums></div>
    <div data-stage></div>

    <div class="exam-nav">
      <button class="btn" type="button" data-prev>${icon('left', 14)}上一题</button>
      <span class="exam-nav-hint" data-pos></span>
      <button class="btn" type="button" data-next>下一题${icon('right', 14)}</button>
    </div>
  `;

  const mount = (root) => runExam(root, questions, paper);
  return { html, title: `模拟考 · ${paper.name}`, mount };
}

/**
 * 考试状态机。
 *
 * 三处刻意的设计：
 *   - 全程不改 hash：改 hash 会触发路由重渲染，抽到另一批题，人还没答完卷子就换了；
 *   - 答案存在 Map 里而不是存在 DOM 上：题号导航来回跳时 DOM 会重绘，
 *     状态挂 DOM 里的实现一跳到别的题就丢答案；
 *   - 计时器每次 tick 检查容器是否还在 document 里，路由切走后自杀——
 *     页面模块没有 unmount 钩子，这是唯一不会泄漏的地方。
 */
function runExam(root, questions, paper) {
  const total = questions.length;
  const st = {
    index: 0,
    answers: new Map(),   // questionId -> string[]
    remain: paper.minutes * 60,
    done: false,
  };

  const stage = root.querySelector('[data-stage]');
  const nums = root.querySelector('[data-nums]');
  const clockEl = root.querySelector('[data-clock]');
  const progressEl = root.querySelector('[data-progress]');
  const posEl = root.querySelector('[data-pos]');
  const prevBtn = root.querySelector('[data-prev]');
  const nextBtn = root.querySelector('[data-next]');

  clearInterval(window.__examTimer);
  window.__examTimer = setInterval(() => {
    if (!document.body.contains(root) || st.done) {
      clearInterval(window.__examTimer);
      return;
    }
    st.remain -= 1;
    paintClock();
    if (st.remain <= 0) {
      clearInterval(window.__examTimer);
      toastError('时间到，已自动交卷');
      submit(false, true);
    }
  }, 1000);

  function paintClock() {
    if (!clockEl) return;
    clockEl.textContent = fmtClock(Math.max(0, st.remain));
    clockEl.classList.toggle('is-urgent', st.remain <= URGENT_SEC);
  }

  function answeredCount() {
    return questions.filter((q) => (st.answers.get(q.id) || []).length > 0).length;
  }

  function paintNums() {
    if (!nums) return;
    nums.innerHTML = questions.map((q, i) => {
      const has = (st.answers.get(q.id) || []).length > 0;
      const cls = ['exam-num'];
      if (has) cls.push('is-answered');
      if (i === st.index) cls.push('is-current');
      return `<button class="${cls.join(' ')}" type="button" data-jump="${i}"
        title="第 ${i + 1} 题${has ? '（已答）' : '（未答）'}">${i + 1}</button>`;
    }).join('');
    nums.querySelectorAll('[data-jump]').forEach((btn) => {
      btn.addEventListener('click', () => {
        st.index = Number(btn.getAttribute('data-jump'));
        draw();
        window.scrollTo({ top: 0, behavior: 'smooth' });
      });
    });
    if (progressEl) progressEl.textContent = `已答 ${answeredCount()} / ${total}`;
    if (posEl) posEl.textContent = `第 ${st.index + 1} / ${total} 题`;
    if (prevBtn) prevBtn.disabled = st.index === 0;
    if (nextBtn) {
      const last = st.index + 1 >= total;
      nextBtn.innerHTML = last ? `交卷${icon('check', 14)}` : `下一题${icon('right', 14)}`;
    }
  }

  function draw() {
    const q = questions[st.index];
    const picked = st.answers.get(q.id) || [];
    const multiple = q.type === 'multiple';

    stage.innerHTML = `<div class="card quiz-card">
      <div class="quiz-stem"><span class="qno">${st.index + 1}</span>${esc(q.stem)}</div>
      <div class="quiz-tags">
        <span class="chip chip-brand">${esc(QUIZ_TYPE_LABEL[q.type] || q.type)}</span>
        <span class="chip">${esc(DIFFICULTY_LABEL[q.difficulty] || q.difficulty || '')}</span>
        ${q.categoryName ? `<span class="chip">${esc(q.categoryName)}</span>` : ''}
        ${multiple ? `<span class="chip" style="color:var(--warn)">多选，全对才得分</span>` : ''}
      </div>
      <div class="opts">
        ${(q.options || []).map((opt) => `<button class="opt${picked.includes(opt.key) ? ' is-picked' : ''}"
          type="button" data-opt="${esc(opt.key)}" aria-checked="${picked.includes(opt.key)}">
          <span class="opt-mark">${esc(opt.key)}</span>
          <span class="opt-text">${esc(opt.text)}</span>
        </button>`).join('')}
      </div>
      <p class="exam-tip">${icon('info', 12)}现在不打分，可以先随便改；交卷后统一给答案与解析。</p>
    </div>`;

    stage.querySelectorAll('[data-opt]').forEach((btn) => {
      btn.addEventListener('click', () => {
        if (st.done) return;
        const key = btn.getAttribute('data-opt');
        const cur = st.answers.get(q.id) || [];
        const next = q.type === 'multiple'
          ? (cur.includes(key) ? cur.filter((k) => k !== key) : [...cur, key])
          : [key];
        if (next.length) st.answers.set(q.id, next);
        else st.answers.delete(q.id);
        // 只重绘选项与题号条，不整页重建，避免点选时页面跳动。
        stage.querySelectorAll('[data-opt]').forEach((b) => {
          const on = next.includes(b.getAttribute('data-opt'));
          b.classList.toggle('is-picked', on);
          b.setAttribute('aria-checked', String(on));
        });
        paintNums();
      });
    });

    paintNums();
  }

  function go(delta) {
    const next = st.index + delta;
    if (next < 0) return;
    if (next >= total) {
      askFinish();
      return;
    }
    st.index = next;
    draw();
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  prevBtn?.addEventListener('click', () => go(-1));
  nextBtn?.addEventListener('click', () => go(1));
  root.querySelector('[data-finish]')?.addEventListener('click', askFinish);

  /** 手动交卷：有未答题时先提醒，让人自己决定补答还是就这样交。 */
  function askFinish() {
    if (st.done) return;
    const blanks = [];
    questions.forEach((q, i) => {
      if (!(st.answers.get(q.id) || []).length) blanks.push(i + 1);
    });
    if (!blanks.length) {
      submit(true, false);
      return;
    }
    openModal(`
      <div class="modal-head"><h3>还有 ${blanks.length} 题没答</h3></div>
      <div class="modal-body">
        <p style="font-size:13.5px;line-height:1.7">
          未作答按错计。题目：<b>${blanks.slice(0, 20).join('、')}${blanks.length > 20 ? ' …' : ''}</b>
        </p>
        <p style="font-size:12.5px;color:var(--text-3);margin-top:10px">
          建议回去补完再交——模拟考的价值在弱项分析，缺题会让统计失真。
        </p>
      </div>
      <div class="modal-actions">
        <button class="btn" type="button" data-go-blank>去补答</button>
        <button class="btn btn-primary" type="button" data-confirm>就这样交卷</button>
      </div>
    `, (modal) => {
      modal.querySelector('[data-go-blank]')?.addEventListener('click', () => {
        closeModal();
        st.index = blanks[0] - 1;
        draw();
        window.scrollTo({ top: 0, behavior: 'smooth' });
      });
      modal.querySelector('[data-confirm]')?.addEventListener('click', () => {
        closeModal();
        submit(true, false);
      });
    });
  }

  async function submit(manual, auto) {
    if (st.done) return;
    st.done = true;
    clearInterval(window.__examTimer);

    const answered = questions.filter((q) => (st.answers.get(q.id) || []).length > 0);
    const blankCount = total - answered.length;
    const durationSec = paper.minutes * 60 - Math.max(0, st.remain);

    stage.innerHTML = `<div class="card"><div class="page-loading">
      <span class="spinner"></span><p>正在判卷…</p></div></div>`;
    if (nums) nums.innerHTML = '';

    // 一道都没答就直接交：没有可提交的内容，也不必打接口。
    if (!answered.length) {
      drawBlankResult();
      return;
    }

    const items = answered.map((q) => ({ questionId: q.id, chosen: st.answers.get(q.id) }));
    try {
      const res = await api.submitAttempt({ items, durationSec });
      drawResult(res, items.length, blankCount, durationSec);
    } catch (err) {
      // 判分只能靠服务端——正确答案从不下发到前端，本地算不出分。
      stage.innerHTML = `<div class="card card-pad">
        <h3 style="font-size:15px;margin-bottom:8px">交卷失败</h3>
        <p style="font-size:13.5px;color:var(--text-3);margin-bottom:12px">
          ${esc(err instanceof ApiError ? err.message : '网络异常，请稍后再试')}
          <br />你的作答还在本页，重试不会丢答案。
        </p>
        <button class="btn btn-primary" type="button" data-retry>重新交卷</button>
      </div>`;
      stage.querySelector('[data-retry]')?.addEventListener('click', () => {
        st.done = false;
        submit(manual, auto);
      });
    }
  }

  function drawBlankResult() {
    stage.innerHTML = `<div class="card card-pad" style="text-align:center">
      <h3 style="font-size:16px">整卷空白，没有可判的内容</h3>
      <p style="font-size:13.5px;color:var(--text-3);margin:8px 0 16px">
        至少答一道题才能出成绩单。
      </p>
      <a class="btn btn-primary" href="#/exam">重新选卷</a>
    </div>`;
  }

  function drawResult(res, answeredCount, blankCount, durationSec) {
    const items = res.items || [];
    const correctCount = items.filter((it) => it.correct).length;
    // 分数按**卷面总题数**算，不是按已答题数：漏答的题在真实考试里就是 0 分。
    // 服务端返回的 score 是"已答正确率"，两者口径不同，展示时要说清楚。
    const score = Math.round((correctCount / total) * 100);

    if (nums) nums.innerHTML = '';
    if (progressEl) progressEl.textContent = '已交卷';
    if (posEl) posEl.textContent = '';

    const weak = weakAnalysis(items, questions);

    stage.innerHTML = `
      <div class="card score-panel">
        <div class="score-ring" style="--pct:${score}">
          <div class="score-ring-inner"><strong>${score}</strong><span>分</span></div>
        </div>
        <h2 style="font-size:18px">${esc(scoreWord(score))}</h2>
        <p class="exam-result-sub">
          ${esc(paper.name)} · ${total} 题 · 用时 ${esc(fmtDuration(durationSec))}
          ${blankCount ? `<br /><span style="color:var(--warn)">有 ${blankCount} 题未作答，已按错计</span>` : ''}
        </p>
        <div class="score-summary">
          <div><strong>${correctCount}</strong><span>答对</span></div>
          <div><strong>${answeredCount}</strong><span>已答</span></div>
          <div><strong>${blankCount}</strong><span>未答</span></div>
        </div>
        <div class="exam-result-actions">
          <button class="btn btn-primary" type="button" data-again>${icon('refresh', 15)}再来一轮</button>
          <button class="btn" type="button" data-share>${icon('chat', 15)}复制成绩</button>
          <a class="btn" href="#/quiz">错题本</a>
          <a class="btn" href="#/exam">换份卷子</a>
        </div>
      </div>

      ${weak.length ? `<div class="section">
        <div class="section-head">
          <h2>弱项分析</h2>
          <span class="more">按方向正确率从低到高</span>
        </div>
        <div class="card card-pad">
          ${weak.map(weakRowHTML).join('')}
          <p class="exam-weak-hint">${icon('info', 12)}
            正确率低于 60% 的方向建议先回去刷一轮练习模式，再来考一次对比曲线。</p>
        </div>
      </div>` : ''}

      <div class="section">
        <div class="section-head">
          <h2>逐题回顾</h2>
          <span class="more">${items.filter((i) => !i.correct).length} 道错题</span>
        </div>
        ${items.map((it) => {
          // 题号用原卷位置，不是结果数组下标——漏答的题不在这份列表里，
          // 用下标会让"第 5 题"对不上卷子上的第 5 题。
          const no = questions.findIndex((q) => q.id === it.questionId) + 1;
          return reviewHTML(it, no, questions);
        }).join('')}
        ${blankCount ? `<div class="card exam-blank-note">${icon('alert', 14)}
          另有 ${blankCount} 题未作答，不计入逐题回顾。</div>` : ''}
      </div>
    `;

    stage.querySelector('[data-again]')?.addEventListener('click', () => {
      const p = new URLSearchParams({ paper: paper.id, r: String(Date.now()) });
      location.hash = `#/exam/run?${p.toString()}`;
    });
    stage.querySelector('[data-share]')?.addEventListener('click', async () => {
      const text = shareText({ score, correctCount, total, durationSec, weak, paper });
      const ok = await copyText(text);
      if (ok) toastOk('成绩文案已复制，去群里喊一嗓子');
      else toastError('复制失败，请手动选中文字');
    });

    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  draw();
  paintClock();
}

/* --------------------------------------------------------- 结果零件 -- */

/**
 * 按方向聚合正确率。
 *
 * 只统计已作答的题（items 里有的），未答的题没有方向信息之外的价值，
 * 计入分母反而会让"没来得及做"和"不会做"混在一起，误导复习方向。
 */
function weakAnalysis(items, questions) {
  const byId = new Map(questions.map((q) => [q.id, q]));
  const agg = new Map();
  items.forEach((it) => {
    const q = byId.get(it.questionId);
    if (!q) return;
    const name = q.categoryName || '未分类';
    const cur = agg.get(name) || { name, total: 0, correct: 0 };
    cur.total += 1;
    if (it.correct) cur.correct += 1;
    agg.set(name, cur);
  });
  return [...agg.values()]
    .map((x) => ({ ...x, pct: Math.round((x.correct / x.total) * 100) }))
    .sort((a, b) => a.pct - b.pct || b.total - a.total)
    .slice(0, 8);
}

function weakRowHTML(w) {
  const tone = w.pct >= 80 ? 'is-ok' : w.pct >= 60 ? 'is-warn' : 'is-bad';
  return `<div class="exam-weak-row">
    <span class="exam-weak-name">${esc(w.name)}</span>
    <span class="exam-weak-track"><i class="${tone}" style="width:${w.pct}%"></i></span>
    <span class="exam-weak-pct ${tone}">${w.pct}%</span>
    <span class="exam-weak-count">${w.correct}/${w.total}</span>
  </div>`;
}

function reviewHTML(item, no, questions) {
  const q = questions.find((x) => x.id === item.questionId);
  const options = q ? q.options : [];
  const correct = item.correctOptions || [];
  const chosen = item.chosen || [];
  const opts = (options || []).map((opt) => {
    const cls = ['opt', 'is-readonly'];
    if (correct.includes(opt.key)) cls.push('is-correct');
    else if (chosen.includes(opt.key)) cls.push('is-wrong');
    return `<div class="${cls.join(' ')}">
      <span class="opt-mark">${esc(opt.key)}</span>
      <span class="opt-text">${esc(opt.text)}</span>
    </div>`;
  }).join('');

  return `<div class="card wrong-item${item.correct ? ' is-pass' : ''}">
    <header>
      <span class="badge num">${no}</span>
      <span class="stem">${esc(q ? q.stem : `题目 #${item.questionId}`)}</span>
      <span class="chip ${item.correct ? 'chip-ok' : 'chip-danger'}">${item.correct ? '正确' : '错误'}</span>
    </header>
    ${opts ? `<div class="opts">${opts}</div>` : ''}
    <div class="answer-pair">
      <span>你的答案：<b>${esc(chosen.join('、') || '未作答')}</b></span>
      <span>正确答案：<i>${esc(correct.join('、'))}</i></span>
    </div>
    ${item.analysis ? `<div class="explain"><b>解析：</b>${esc(item.analysis)}</div>` : ''}
  </div>`;
}

function needLoginHTML(paper) {
  return `<nav class="crumb">
      <a href="#/quiz">运维题库</a><span class="crumb-sep">/</span><a href="#/exam">模拟考</a>
    </nav>
    <div class="card card-pad" style="text-align:center;padding:44px 24px">
      <span style="display:inline-flex;width:52px;height:52px;border-radius:16px;align-items:center;
        justify-content:center;background:var(--brand-soft);color:var(--brand);margin-bottom:14px">
        ${icon('quiz', 26)}
      </span>
      <h2 style="font-size:17px;margin-bottom:8px">登录后开始${esc(paper.name)}</h2>
      <p style="font-size:13.5px;color:var(--text-3);max-width:460px;margin:0 auto 20px;line-height:1.7">
        模拟考的成绩单、弱项分析和错题本都要挂在账号上，所以需要先登录。
        ${paper.count} 题 · ${paper.minutes} 分钟，答完立即出分。
      </p>
      <button class="btn btn-primary btn-lg" type="button" data-act="login">登录 / 注册</button>
    </div>`;
}

function shareText({ score, correctCount, total, durationSec, weak, paper }) {
  const weakTop = weak.slice(0, 3).map((w) => `${w.name} ${w.pct}%`).join('、');
  return [
    `我在运维专家服务平台做了「${paper.name}」：${score} 分`,
    `${total} 题答对 ${correctCount} 题，用时 ${fmtDuration(durationSec)}`,
    weakTop ? `弱项：${weakTop}` : '',
    `来测测你能拿多少分 → https://www.jinzhiqiang.com/#/exam`,
  ].filter(Boolean).join('\n');
}

async function copyText(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch { /* 落到下面的兼容分支 */ }
  try {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

function scoreWord(score) {
  if (score >= 90) return '这个分数可以直接去面了';
  if (score >= 75) return '基础扎实，个别方向还差一口气';
  if (score >= 60) return '刚过线，弱项值得再刷一轮';
  return '底子还要补，先按弱项分析补两章';
}

function scoreTone(score) {
  const n = Number(score) || 0;
  return n >= 80 ? 'chip-ok' : n >= 60 ? 'chip-warn' : 'chip-danger';
}

function fmtClock(sec) {
  const s = Math.max(0, Math.floor(sec));
  const m = Math.floor(s / 60);
  const r = s % 60;
  return `${String(m).padStart(2, '0')}:${String(r).padStart(2, '0')}`;
}

function pick(settled, fallback) {
  if (!settled || settled.status !== 'fulfilled') return fallback;
  return settled.value ?? fallback;
}
