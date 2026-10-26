/* ============================================================================
   pages-quiz.js — 运维题库：首页（战绩 / 错题本 / 历史）与练习页（抽题判分）。
   ========================================================================= */

import { api, isLoggedIn, ApiError } from './api.js';
import { esc, fmtNum, fmtDuration, fmtDate, icon, DIFFICULTY_LABEL, QUIZ_TYPE_LABEL } from './util.js';
import { emptyHTML, toastError } from './ui.js';

/* ====================================================== 题库首页 ====== */

export async function quizHomePage(ctx) {
  const logged = isLoggedIn();
  const channel = findChannel(ctx, 'ops-quiz');

  // 未登录时这三项都取不到，直接不请求，省掉三次必然 401 的往返
  // 分类卡对游客同样有意义（能看出题库覆盖了哪些运维方向），所以不受登录态影响；
  // 未登录时服务端把个人进度置 0，前端不必再判断一次。
  const [stats, wrongBook, attempts, categoryRes] = await Promise.allSettled([
    logged ? api.quizStats() : null,
    logged ? api.wrongBook({ page: 1, pageSize: 10 }) : null,
    logged ? api.attempts({ page: 1, pageSize: 10 }) : null,
    api.quizCategories(),
  ]);

  const statsData = pickValue(stats, null);
  const wrongItems = pickValue(wrongBook, { list: [] }).list || [];
  const attemptItems = pickValue(attempts, { list: [] }).list || [];
  const categoryItems = (pickValue(categoryRes, []) || []).filter((c) => c && c.name);

  const html = `
    <div class="page-head">
      <h1>${esc(channel.name)}</h1>
      <p class="lede">${esc(channel.desc || '')}。多选题按集合判分——少选、多选都不得分，
        因为现场少执行一步就是事故。</p>
    </div>

    <div class="card card-pad" style="margin-bottom:24px">
      <div style="display:flex;align-items:center;gap:20px;flex-wrap:wrap">
        <div style="flex:1;min-width:220px">
          <h2 style="font-size:15.5px;margin-bottom:6px">开始一轮练习</h2>
          <p style="font-size:13px;color:var(--text-3)">
            随机抽题、逐题判分，答完给出成绩单与逐题解析。
          </p>
        </div>
        <div style="display:flex;align-items:center;gap:10px;flex-wrap:wrap">
          <div class="seg" role="group" aria-label="抽题数量" data-count-seg>
            ${[5, 10, 20].map((n) => `<button type="button" data-count="${n}"
              class="${n === 10 ? 'is-active' : ''}">${n} 题</button>`).join('')}
          </div>
          <select class="select" data-difficulty aria-label="难度">
            <option value="">不限难度</option>
            <option value="easy">简单</option>
            <option value="medium">中等</option>
            <option value="hard">困难</option>
          </select>
          <button class="btn btn-primary btn-lg" type="button" data-start>
            ${icon('play', 15)}开始练习
          </button>
        </div>
      </div>
    </div>

    ${categorySectionHTML(categoryItems)}

    ${logged ? statsHTML(statsData) : guestHTML()}

    ${logged ? `<div class="tabs" role="tablist">
      <button class="tab is-active" type="button" data-tab="wrong" role="tab">
        错题本${wrongItems.length ? `（${wrongItems.length}）` : ''}
      </button>
      <button class="tab" type="button" data-tab="history" role="tab">练习记录</button>
    </div>

    <div data-panel="wrong">
      ${wrongItems.length
        ? wrongItems.map(wrongItemHTML).join('')
        : emptyHTML({
            iconName: 'check',
            title: '错题本是空的',
            desc: '最近一次作答全对的题目不会出现在这里。保持住。',
          })}
    </div>

    <div data-panel="history" hidden>
      ${attemptItems.length
        ? `<div class="post-list">${attemptItems.map(attemptItemHTML).join('')}</div>`
        : emptyHTML({ iconName: 'list', title: '还没有练习记录', desc: '完成一轮练习后，成绩会留在这里。' })}
    </div>` : ''}
  `;

  const mount = (root) => {
    let count = 10;
    const countSeg = root.querySelector('[data-count-seg]');
    if (countSeg) {
      countSeg.querySelectorAll('[data-count]').forEach((btn) => {
        btn.addEventListener('click', () => {
          count = Number(btn.getAttribute('data-count'));
          countSeg.querySelectorAll('[data-count]').forEach((b) => b.classList.remove('is-active'));
          btn.classList.add('is-active');
        });
      });
    }

    // 分类卡直接开练：不先进分类列表再点第二次，
    // 用户点「网络」时想做的就是开始做题。
    root.querySelectorAll('[data-cat-id]').forEach((btn) => {
      btn.addEventListener('click', () => {
        if (!isLoggedIn()) {
          window.dispatchEvent(new CustomEvent('opskb:need-login'));
          toastError('练习需要登录，这样成绩和错题才能存下来');
          return;
        }
        const categoryId = btn.getAttribute('data-cat-id') || '';
        const n = clampInt(Number(btn.getAttribute('data-cat-count')), 1, 20);
        location.hash = `#/quiz/practice${queryOf({ count: n, categoryId })}`;
      });
    });

    const startBtn = root.querySelector('[data-start]');
    if (startBtn) {
      startBtn.addEventListener('click', () => {
        if (!isLoggedIn()) {
          window.dispatchEvent(new CustomEvent('opskb:need-login'));
          toastError('练习需要登录，这样成绩和错题才能存下来');
          return;
        }
        const difficulty = root.querySelector('[data-difficulty]')?.value || '';
        location.hash = `#/quiz/practice${queryOf({ count, difficulty })}`;
      });
    }

    root.querySelectorAll('[data-tab]').forEach((tab) => {
      tab.addEventListener('click', () => {
        const target = tab.getAttribute('data-tab');
        root.querySelectorAll('[data-tab]').forEach((t) => t.classList.toggle('is-active', t === tab));
        root.querySelectorAll('[data-panel]').forEach((p) => {
          p.hidden = p.getAttribute('data-panel') !== target;
        });
      });
    });
  };

  return { html, mount };
}

/* ------------------------------------------------------------ 分类卡 -- */

// 分类卡把「题库有哪些方向」摊开给人看。
// 0 题的分类不隐藏而是标成「筹备中」：这既是内容规划的信号，
// 也避免用户以为题库只覆盖了寥寥几个方向。
function categorySectionHTML(items) {
  if (!items.length) return '';
  const ready = items.filter((c) => Number(c.count) > 0).length;
  const total = items.reduce((sum, c) => sum + (Number(c.count) || 0), 0);
  return `<section class="quiz-cats">
    <div class="section-head">
      <h2>按方向刷题</h2>
      <span class="more">${ready} 个方向 · 共 ${fmtNum(total)} 题</span>
    </div>
    <div class="quiz-cat-grid">
      ${items.map(categoryCardHTML).join('')}
    </div>
  </section>`;
}

function categoryCardHTML(c) {
  const total = Number(c.count) || 0;
  const answered = Number(c.answered) || 0;
  const correct = Number(c.correct) || 0;
  const empty = total <= 0;
  // 进度按**题目去重**口径，与服务端一致：刷三遍同一题不该走到 300%。
  const pct = total > 0 ? Math.min(100, Math.round((answered / total) * 100)) : 0;
  const hint = empty
    ? '题库正在补充这个方向'
    : answered > 0
      ? `已做 ${fmtNum(answered)} 题 · 对 ${fmtNum(correct)} 题`
      : '还没开始';

  return `<button class="quiz-cat${empty ? ' is-empty' : ''}" type="button"
    data-cat-id="${esc(String(c.id || ''))}"
    data-cat-count="${Math.min(total || 10, 20)}"
    ${empty ? 'disabled aria-disabled="true"' : ''}>
    <span class="quiz-cat-top">
      <span class="quiz-cat-name">${esc(c.name)}</span>
      <span class="quiz-cat-count">${empty ? '筹备中' : `${fmtNum(total)} 题`}</span>
    </span>
    <span class="quiz-cat-bar"><i style="width:${pct}%"></i></span>
    <span class="quiz-cat-hint">${esc(hint)}</span>
  </button>`;
}

function statsHTML(stats) {
  const s = stats || { totalAnswered: 0, totalCorrect: 0, accuracy: 0, wrongCount: 0, attemptCount: 0 };
  const accuracy = Math.round(Number(s.accuracy) || 0);
  return `<div class="stat-grid">
    ${statCard('已作答', fmtNum(s.totalAnswered), '道题', 'target', 'var(--brand)')}
    ${statCard('正确率', String(accuracy), '%', 'check', accuracy >= 60 ? 'var(--ok)' : 'var(--warn)')}
    ${statCard('待复习错题', fmtNum(s.wrongCount), '道', 'alert', s.wrongCount > 0 ? 'var(--danger)' : 'var(--text-3)')}
    ${statCard('练习次数', fmtNum(s.attemptCount), '次', 'activity', 'var(--info)')}
  </div>`;
}

function statCard(label, value, unit, iconName, color) {
  return `<div class="stat-card">
    <div class="stat-label" style="color:${color}">${icon(iconName, 14)}<span style="color:var(--text-3)">${esc(label)}</span></div>
    <div class="stat-value">${esc(value)}<small>${esc(unit)}</small></div>
  </div>`;
}

function guestHTML() {
  return `<div class="card card-pad" style="margin-bottom:24px;display:flex;align-items:center;gap:16px;flex-wrap:wrap">
    <span style="width:42px;height:42px;border-radius:12px;display:flex;align-items:center;justify-content:center;background:var(--brand-soft);color:var(--brand);flex:0 0 auto">
      ${icon('user', 21)}
    </span>
    <div style="flex:1;min-width:200px">
      <strong style="font-size:14.5px">登录后开启成绩记录</strong>
      <p style="font-size:13px;color:var(--text-3);margin-top:4px">
        登录后可以查看正确率、错题本与历史练习记录。现在的你可以先随便练一练。
      </p>
    </div>
    <button class="btn btn-primary" type="button" data-act="login">登录</button>
  </div>`;
}

function wrongItemHTML(item) {
  const chosen = (item.chosen || []).join('、') || '未作答';
  const correct = (item.correctOptions || []).join('、');
  return `<div class="card wrong-item">
    <header>
      <span class="chip chip-danger">${esc(DIFFICULTY_LABEL[item.difficulty] || item.difficulty || '')}</span>
      <span class="stem">${esc(item.stem)}</span>
    </header>
    ${optionsReadonlyHTML(item.options, item.correctOptions, item.chosen)}
    <div class="answer-pair">
      <span>你的答案：<b>${esc(chosen)}</b></span>
      <span>正确答案：<i>${esc(correct)}</i></span>
      <span style="margin-left:auto">${esc(fmtDate(item.lastWrongAt))}</span>
    </div>
    ${item.analysis ? `<div class="explain"><b>解析：</b>${esc(item.analysis)}</div>` : ''}
  </div>`;
}

function attemptItemHTML(item) {
  const score = Number(item.score) || 0;
  const tone = score >= 80 ? 'chip-ok' : score >= 60 ? 'chip-warn' : 'chip-danger';
  return `<div class="card" style="padding:15px 18px;display:flex;align-items:center;gap:16px;flex-wrap:wrap">
    <span class="badge">${esc(fmtDate(item.createdAt))}</span>
    <span style="font-size:13.5px">答对 ${item.correctCount} / ${item.questionCount} 题</span>
    ${item.durationSec ? `<span class="meta-item" style="font-size:12.5px;color:var(--text-3)">
      ${icon('clock', 13)}${esc(fmtDuration(item.durationSec))}</span>` : ''}
    <span class="chip ${tone}" style="margin-left:auto">${score} 分</span>
  </div>`;
}

/* ====================================================== 练习页 ======== */

export async function quizPracticePage(ctx) {
  const params = ctx.params;
  const count = clampInt(Number(params.get('count') || 10), 1, 50);
  const difficulty = params.get('difficulty') || '';
  const type = params.get('type') || '';
  const categoryId = params.get('categoryId') || '';

  const result = await api.practice({
    count,
    difficulty: difficulty || undefined,
    type: type || undefined,
    categoryId: categoryId || undefined,
  });
  const questions = result.list || [];

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a>
      <span class="crumb-sep">/</span>
      <a href="#/quiz">运维题库</a>
      <span class="crumb-sep">/</span>
      <span>练习</span>
    </nav>

    <div class="quiz-progress" data-progress>
      <span data-progress-text>准备开始</span>
      <span class="bar"><i data-progress-bar style="width:0%"></i></span>
      <span data-progress-score></span>
    </div>

    <div data-stage></div>
  `;

  const mount = (root) => {
    runPractice(root, questions);
  };

  return { html, mount };
}

/**
 * 练习状态机。
 *
 * 整个过程刻意不改 hash：改 hash 会触发路由重渲染，等于把刚抽到的题
 * 换成另一批，用户答到一半题就变了。所以这里只做局部重绘。
 */
function runPractice(root, questions) {
  const st = {
    index: 0,
    picked: [],
    results: [],
    judged: null,      // 当前题的判分结果
    finished: false,
    startedAt: Date.now(),
  };

  const stage = root.querySelector('[data-stage]');
  const progressText = root.querySelector('[data-progress-text]');
  const progressBar = root.querySelector('[data-progress-bar]');
  const progressScore = root.querySelector('[data-progress-score]');

  if (!questions.length) {
    stage.innerHTML = emptyHTML({
      iconName: 'quiz',
      title: '题库里还没有符合条件的题目',
      desc: '换个难度或分类再试试。',
      actionHTML: '<a class="btn btn-sm" href="#/quiz">返回题库</a>',
    });
    if (progressText) progressText.textContent = '';
    return;
  }

  function drawProgress() {
    const done = st.results.filter((r) => r.correct).length;
    const total = questions.length;
    const pos = Math.min(st.index + 1, total);
    if (progressText) progressText.textContent = `第 ${pos} / ${total} 题`;
    if (progressBar) progressBar.style.width = `${(st.results.length / total) * 100}%`;
    if (progressScore) progressScore.textContent = `已答对 ${done} 题`;
  }

  function drawQuestion() {
    const q = questions[st.index];
    const judge = st.judged;
    const multiple = q.type === 'multiple';

    stage.innerHTML = `<div class="card quiz-card">
      <div class="quiz-stem">
        <span class="qno">${st.index + 1}</span>${esc(q.stem)}
      </div>

      <div class="quiz-tags">
        <span class="chip chip-brand">${esc(QUIZ_TYPE_LABEL[q.type] || q.type)}</span>
        <span class="chip">${esc(DIFFICULTY_LABEL[q.difficulty] || q.difficulty || '')}</span>
        ${q.categoryName ? `<span class="chip">${esc(q.categoryName)}</span>` : ''}
        ${q.answerCount ? `<span class="chip">已有 ${fmtNum(q.answerCount)} 人作答</span>` : ''}
      </div>

      ${multiple ? `<p style="font-size:12.5px;color:var(--warn);margin-bottom:12px">
        ${icon('info', 13)} 多选题需要全部选对才得分，漏选、多选都不得分。
      </p>` : ''}

      <div class="opts" data-opts>
        ${(q.options || []).map((opt) => optionHTML(opt, st.picked, judge, multiple)).join('')}
      </div>

      ${judge ? verdictHTML(judge) : ''}

      <div style="display:flex;gap:10px;margin-top:18px;align-items:center">
        ${judge
          ? `<button class="btn btn-primary" type="button" data-next>
              ${st.index + 1 >= questions.length ? '交卷看成绩' : '下一题'} ${icon('right', 14)}
             </button>`
          : `<button class="btn btn-primary" type="button" data-submit
              ${st.picked.length ? '' : 'disabled'}>提交本题</button>
             <span style="font-size:12.5px;color:var(--text-4)">
               ${st.picked.length ? '已选 ' + st.picked.join('、') : '请先选择答案'}
             </span>`}
      </div>
    </div>`;

    bindQuestion();
    drawProgress();
  }

  function bindQuestion() {
    const q = questions[st.index];

    stage.querySelectorAll('[data-opt]').forEach((btn) => {
      btn.addEventListener('click', () => {
        if (st.judged) return;   // 已判分就不允许改答案
        const key = btn.getAttribute('data-opt');
        if (q.type === 'multiple') {
          st.picked = st.picked.includes(key)
            ? st.picked.filter((k) => k !== key)
            : [...st.picked, key];
        } else {
          st.picked = [key];
        }
        drawQuestionKeepScroll();
      });
    });

    const submitBtn = stage.querySelector('[data-submit]');
    if (submitBtn) submitBtn.addEventListener('click', submitAnswer);

    const nextBtn = stage.querySelector('[data-next]');
    if (nextBtn) nextBtn.addEventListener('click', goNext);
  }

  /** 重绘选项时保留滚动位置：不然每次点选整页都会跳回顶部。 */
  function drawQuestionKeepScroll() {
    const y = window.scrollY;
    drawQuestion();
    window.scrollTo({ top: y });
  }

  async function submitAnswer() {
    const q = questions[st.index];
    const btn = stage.querySelector('[data-submit]');
    if (btn) btn.disabled = true;
    try {
      const res = await api.answer(q.id, st.picked);
      st.judged = res;
      st.results.push({
        questionId: q.id,
        chosen: st.picked.slice(),
        correct: res.correct,
      });
      drawQuestionKeepScroll();
    } catch (err) {
      if (btn) btn.disabled = false;
      if (err instanceof ApiError && err.status === 401) {
        toastError('登录状态已失效，请重新登录');
        window.dispatchEvent(new CustomEvent('opskb:need-login'));
        return;
      }
      toastError(err instanceof ApiError ? err.message : '提交失败，请稍后再试');
    }
  }

  function goNext() {
    if (st.index + 1 >= questions.length) {
      finish();
      return;
    }
    st.index += 1;
    st.picked = [];
    st.judged = null;
    drawQuestion();
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  async function finish() {
    st.finished = true;
    const durationSec = Math.round((Date.now() - st.startedAt) / 1000);

    stage.innerHTML = `<div class="card"><div class="page-loading">
      <span class="spinner"></span><p>正在生成成绩单…</p>
    </div></div>`;

    try {
      const res = await api.submitAttempt({
        items: st.results.map((r) => ({ questionId: r.questionId, chosen: r.chosen })),
        durationSec,
      });
      drawScore(res);
    } catch (err) {
      // 成绩单拿不到不该把本地算出来的结果也丢掉，先给一份本地汇总
      const correct = st.results.filter((r) => r.correct).length;
      drawScore({
        questionCount: st.results.length,
        correctCount: correct,
        score: st.results.length ? Math.round((correct / st.results.length) * 100) : 0,
        durationSec,
        items: [],
        offline: true,
        message: err instanceof ApiError ? err.message : '',
      });
    }
  }

  function drawScore(res) {
    const score = Number(res.score) || 0;
    const items = res.items || [];
    if (progressText) progressText.textContent = '本轮完成';
    if (progressBar) progressBar.style.width = '100%';
    if (progressScore) progressScore.textContent = '';

    stage.innerHTML = `
      <div class="card score-panel">
        <div class="score-ring" style="--pct:${score}">
          <div class="score-ring-inner">
            <strong>${score}</strong>
            <span>分</span>
          </div>
        </div>
        <h2 style="font-size:18px">${scoreText(score)}</h2>
        <p style="color:var(--text-3);font-size:13px;margin-top:6px">
          ${res.offline
            ? '成绩单没能存到服务器' + (res.message ? `（${esc(res.message)}）` : '') + '，以下是本次作答的本地统计。'
            : '成绩已记录，可以到题库首页查看历史与错题本。'}
        </p>
        <div class="score-summary">
          <div><strong>${res.correctCount}</strong><span>答对</span></div>
          <div><strong>${res.questionCount}</strong><span>总题数</span></div>
          <div><strong>${esc(fmtDuration(res.durationSec))}</strong><span>用时</span></div>
        </div>
        <div style="display:flex;gap:10px;justify-content:center;flex-wrap:wrap">
          <a class="btn btn-primary" href="#/quiz/practice?count=${questions.length}">
            ${icon('refresh', 15)}再练一轮
          </a>
          <a class="btn" href="#/quiz">返回题库</a>
        </div>
      </div>

      ${items.length ? `<div class="section" style="margin-top:26px">
        <div class="section-head"><h2>逐题回顾</h2></div>
        ${items.map((it, i) => reviewItemHTML(it, i, questions)).join('')}
      </div>` : ''}
    `;
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }

  drawQuestion();
}

/* ------------------------------------------------------- 题目零件 -- */

function optionHTML(opt, picked, judge, multiple) {
  const isPicked = picked.includes(opt.key);
  const correctOptions = judge ? (judge.correctOptions || []) : [];
  const isCorrectKey = correctOptions.includes(opt.key);

  const classes = ['opt'];
  if (judge) {
    classes.push('is-readonly');
    if (isCorrectKey) classes.push('is-correct');
    else if (isPicked) classes.push('is-wrong');
  } else if (isPicked) {
    classes.push('is-picked');
  }

  const shape = multiple ? 'checkbox' : 'radio';
  return `<button class="${classes.join(' ')}" type="button" data-opt="${esc(opt.key)}"
    ${judge ? 'disabled' : ''} aria-checked="${isPicked}" role="${shape}">
    <span class="opt-mark">${esc(opt.key)}</span>
    <span class="opt-text">${esc(opt.text)}</span>
  </button>`;
}

/** 只读选项（错题本、逐题回顾用）。 */
function optionsReadonlyHTML(options, correctOptions, chosen) {
  const correct = correctOptions || [];
  const picked = chosen || [];
  return `<div class="opts">${(options || []).map((opt) => {
    const classes = ['opt', 'is-readonly'];
    if (correct.includes(opt.key)) classes.push('is-correct');
    else if (picked.includes(opt.key)) classes.push('is-wrong');
    return `<div class="${classes.join(' ')}">
      <span class="opt-mark">${esc(opt.key)}</span>
      <span class="opt-text">${esc(opt.text)}</span>
    </div>`;
  }).join('')}</div>`;
}

function verdictHTML(judge) {
  const correct = judge.correct;
  return `<div class="verdict ${correct ? 'is-correct' : 'is-wrong'}">
    <div class="verdict-head">
      ${icon(correct ? 'check' : 'close', 17)}
      ${correct ? '答对了' : '答错了'}
      ${judge.fresh === false ? '<span class="chip" style="margin-left:6px;font-weight:400">重复作答，不计入统计</span>' : ''}
    </div>
    <div class="verdict-body">
      ${correct ? '' : `正确答案：${(judge.correctOptions || []).map((k) => `<span class="k">${esc(k)}</span>`).join('')}<br />`}
      ${judge.analysis ? `<b>解析：</b>${esc(judge.analysis)}` : ''}
    </div>
  </div>`;
}

function reviewItemHTML(item, index, questions) {
  const q = questions.find((x) => x.id === item.questionId);
  const stem = q ? q.stem : `题目 #${item.questionId}`;
  const options = q ? q.options : [];
  return `<div class="card wrong-item">
    <header>
      <span class="badge num">${index + 1}</span>
      <span class="stem">${esc(stem)}</span>
      <span class="chip ${item.correct ? 'chip-ok' : 'chip-danger'}">
        ${item.correct ? '正确' : '错误'}
      </span>
    </header>
    ${options.length ? optionsReadonlyHTML(options, item.correctOptions, item.chosen) : ''}
    <div class="answer-pair">
      <span>你的答案：<b>${esc((item.chosen || []).join('、') || '未作答')}</b></span>
      <span>正确答案：<i>${esc((item.correctOptions || []).join('、'))}</i></span>
    </div>
    ${item.analysis ? `<div class="explain"><b>解析：</b>${esc(item.analysis)}</div>` : ''}
  </div>`;
}

function scoreText(score) {
  if (score >= 90) return '几乎全对，状态很好';
  if (score >= 75) return '掌握得不错';
  if (score >= 60) return '刚过线，错题值得再看一遍';
  return '基础还需要再巩固一下';
}

/* ------------------------------------------------------------ 内部 -- */

function pickValue(settled, fallback) {
  if (!settled || settled.status !== 'fulfilled') return fallback;
  return settled.value ?? fallback;
}

function clampInt(value, min, max) {
  const n = Number.isFinite(value) ? Math.floor(value) : min;
  return Math.min(Math.max(n, min), max);
}

function queryOf(obj) {
  const sp = new URLSearchParams();
  Object.entries(obj || {}).forEach(([k, v]) => {
    if (v !== '' && v !== null && v !== undefined) sp.set(k, String(v));
  });
  const s = sp.toString();
  return s ? `?${s}` : '';
}

function findChannel(ctx, slug) {
  const hit = (ctx.state.channels || []).find((c) => c.slug === slug);
  return hit || { slug, name: '运维题库', desc: '' };
}
