/* ============================================================================
   pages-contribute.js — 内容投稿（教程 / 题目）与「我的投稿」。

   为什么要有这两页：平台的发布权限不是「有账号就能发」。任何登录用户都可以
   提交内容，但**是否上线由运营审核决定**——投稿落库固定为 pending，
   审核通过才变 published。前端的职责只有两件事：
   把「提交后会发生什么」说清楚，以及把驳回原因原样摆出来。

   为什么是「一个入口、两个表单」而不是两个页面：
   用户想贡献内容时并不知道平台内部把内容分成 articles 与 quiz_questions，
   他要的只是「我想交点什么」。所以入口统一在 #/contribute，
   用类型 tab 切换表单；提交后落到不同的审核队列，但用户看到的仍是
   同一个「我的投稿」列表（两个 tab）。

   这里刻意不做「保存草稿」：投稿是单向的提交动作，草稿是后台编辑的中间态。
   给投稿人一个草稿箱，意味着要再维护一套「草稿 → 提交 → 审核」的状态流转，
   而九成投稿人根本不会回来改——那套机制最后只剩维护成本。
   ========================================================================= */

import { api, isLoggedIn, ApiError } from './api.js';
import { esc, fmtNum, fmtDate, icon, setHashQuery, splitTags } from './util.js';
import { bindPager, emptyHTML, pagerHTML, toastError, toastOk } from './ui.js';
import { speakGateOrNull } from './auth-modal.js';
import { enableMarkdownImages } from './md-image.js';

const PAGE_SIZE = 10;

const TITLE_MIN = 6;
const TITLE_MAX = 200;
const CONTENT_MIN = 50;
const CONTENT_MAX = 50000;

// 选项标识。题库用字母做 key（后端 normalizeOptionKey 也只认单字母），
// 下标即 key，所以最多 8 个选项。
const OPTION_KEYS = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H'];

// STATUS_META 是投稿状态的展示口径，必须与后端 model.ContentStatus* 对齐。
// tone 决定徽标颜色：待审是中性、通过是绿、驳回是红、下线是灰。
const STATUS_META = {
  pending: { label: '待审核', tone: 'warn' },
  published: { label: '已发布', tone: 'ok' },
  rejected: { label: '已驳回', tone: 'danger' },
  draft: { label: '草稿', tone: '' },
  offline: { label: '已下线', tone: '' },
};

/* ======================================================= 投稿页 ====== */

export async function contributePage(ctx) {
  if (!isLoggedIn()) {
    return {
      html: pageHead('内容投稿', '把你踩过的坑写成可复用的步骤，审核通过后会发布到知识库。') +
        emptyHTML({
          iconName: 'book',
          title: '登录后才能投稿',
          desc: '投稿与「我的投稿」列表绑定账号，审核结果与驳回原因都发到这个账号下。',
          actionHTML: '<button class="btn btn-primary" type="button" data-act="login">去登录</button>',
        }),
      title: '内容投稿',
    };
  }

  // 投稿是"写内容"，只读账号一律拦在表单之前。
  const speakGate = speakGateOrNull('内容投稿', {
    head: '把你踩过的坑写成可复用的步骤，审核通过后会发布到知识库。',
    desc: '投稿需要先开通发言权限：通过实名申请后，文章与题目才能进入审核队列。',
  });
  if (speakGate) return speakGate;

  const kind = ctx.params.get('type') === 'quiz' ? 'quiz' : 'article';

  // 分类取不到时也要能打开页面：只是少了一个下拉的可选项，
  // 让整页 500 会把「投稿」这个入口彻底堵死。
  const categories = await api.categories().catch(() => []);

  let formHTML = '';
  let asideHTML = '';
  if (kind === 'article') {
    // 只允许往图文教程类栏目投稿。资讯是运营挑选 + 订阅源抓取的产物，
    // 开放投稿等于任何人都能往「行业动态」里塞软文（服务端也会再拒一次）。
    const channels = await api.channels().catch(() => []);
    const writable = channels.filter((c) => c.kind === 'article' && c.slug !== 'ops-news');
    formHTML = articleFormHTML(writable, categories);
    asideHTML = articleAsideHTML();
  } else {
    formHTML = quizFormHTML(categories);
    asideHTML = quizAsideHTML();
  }

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a>
      <span class="crumb-sep">/</span>
      <a href="#/contribute">内容投稿</a>
    </nav>

    ${pageHead('内容投稿', kind === 'article'
      ? '写清「场景 + 步骤 + 验证方式」。审核通过后会发布到对应栏目，署名保留你的账号。'
      : '把做过的那道题贡献出来：题干、选项、正确答案与解析。答案要经得起推敲，解析要能让错的人看懂。')}

    <div class="contrib-tabs" role="tablist">
      <a class="contrib-tab ${kind === 'article' ? 'is-active' : ''}" href="#/contribute">实战教程</a>
      <a class="contrib-tab ${kind === 'quiz' ? 'is-active' : ''}" href="#/contribute?type=quiz">运维题目</a>
    </div>

    <div class="detail-layout">
      ${formHTML}
      <aside>${asideHTML}</aside>
    </div>
  `;

  const mount = (root) => {
    if (kind === 'article') mountArticleForm(root);
    else mountQuizForm(root);
  };

  return { html, mount, title: '内容投稿' };
}

/* ------------------------------------------------------- 教程表单 -- */

function articleFormHTML(writable, categories) {
  return `
    <form class="card card-pad" id="c-form" novalidate>
      <div class="field" style="margin-bottom:18px">
        <label for="c-title">标题 <span class="hint">${TITLE_MIN}–${TITLE_MAX} 字</span></label>
        <input class="input" id="c-title" maxlength="${TITLE_MAX}"
               placeholder="例如：Nginx 502 排查：后端 keepalive 超时导致连接被复用" />
      </div>

      <div class="form-row">
        <div class="field">
          <label for="c-channel">投稿栏目</label>
          <select class="select" id="c-channel">
            ${writable.length
              ? writable.map((c) => `<option value="${esc(c.slug)}">${esc(c.name)}</option>`).join('')
              : '<option value="linux-tutorial">运维实战教程</option>'}
          </select>
        </div>
        <div class="field">
          <label for="c-category">分类</label>
          <select class="select" id="c-category">
            <option value="0">不指定分类</option>
            ${categories.map((c) => `<option value="${esc(String(c.id))}">${esc(c.name)}</option>`).join('')}
          </select>
        </div>
      </div>

      <div class="field" style="margin-bottom:18px">
        <label for="c-summary">摘要 <span class="hint">选填，不超过 500 字；留空由正文自动截取</span></label>
        <textarea class="input" id="c-summary" rows="3" maxlength="500"
          placeholder="一两句话说明这篇解决什么问题，会显示在列表和搜索结果里"></textarea>
      </div>

      <div class="field" style="margin-bottom:18px">
        <label for="c-content">正文 <span class="hint">至少 ${CONTENT_MIN} 字，支持 Markdown</span></label>
        <textarea class="input code-input" id="c-content" rows="18" maxlength="${CONTENT_MAX}"
          placeholder="## 现象&#10;&#10;## 排查过程&#10;&#10;## 根因&#10;&#10;## 修复步骤&#10;&#10;## 如何验证"></textarea>
      </div>

      <div class="field" style="margin-bottom:18px">
        <label for="c-tags">标签 <span class="hint">用逗号分隔，最多 5 个</span></label>
        <input class="input" id="c-tags" placeholder="nginx, keepalive, 502" />
      </div>

      <p class="form-summary" id="c-summary-line"></p>

      <div class="form-actions">
        <button class="btn btn-primary btn-lg" id="c-submit" type="submit">提交审核</button>
        <a class="btn btn-lg" href="#/me/contributions">我的投稿</a>
      </div>
    </form>
  `;
}

function articleAsideHTML() {
  return `
    <div class="card side-card" style="margin-bottom:16px">
      <h3>提交之后</h3>
      <ol class="steps">
        <li>内容进入待审核队列，不会立刻出现在知识库</li>
        <li>运营会看结构、命令是否可执行、有没有敏感信息</li>
        <li>通过即发布；驳回会写明原因，可在「我的投稿」里看到</li>
      </ol>
    </div>
    <div class="card side-card">
      <h3>容易被打回的情况</h3>
      <ul class="tips-list">
        <li>只有结论没有过程，读者无法复现</li>
        <li>贴了内网地址、密钥、真实账号</li>
        <li>整段搬运官方文档且没有自己的实践</li>
        <li>标题写成提问句（「这个怎么解决？」）</li>
      </ul>
    </div>
  `;
}

function mountArticleForm(root) {
  const form = root.querySelector('#c-form');
  const titleEl = root.querySelector('#c-title');
  const contentEl = root.querySelector('#c-content');
  const channelEl = root.querySelector('#c-channel');
  const categoryEl = root.querySelector('#c-category');
  const summaryEl = root.querySelector('#c-summary');
  const tagsEl = root.querySelector('#c-tags');
  const lineEl = root.querySelector('#c-summary-line');
  const submit = root.querySelector('#c-submit');

  // 正文支持粘贴 / 拖入图片，落库是标准 Markdown 图片语法。
  enableMarkdownImages(root, contentEl);

  const refresh = () => {
    const t = titleEl.value.trim().length;
    const c = contentEl.value.trim().length;
    const problems = [];
    if (t < TITLE_MIN) problems.push(`标题还差 ${TITLE_MIN - t} 字`);
    if (c < CONTENT_MIN) problems.push(`正文还差 ${CONTENT_MIN - c} 字`);
    lineEl.textContent = problems.length
      ? problems.join('，') + '。'
      : '看起来可以提交了。提交后进入待审核队列。';
  };

  [titleEl, contentEl].forEach((el) => el.addEventListener('input', refresh));
  refresh();

  form.addEventListener('submit', async (ev) => {
    ev.preventDefault();

    const title = titleEl.value.trim();
    const content = contentEl.value.trim();
    if (title.length < TITLE_MIN) {
      toastError(`标题至少 ${TITLE_MIN} 个字`);
      titleEl.focus();
      return;
    }
    if (content.length < CONTENT_MIN) {
      toastError(`正文至少 ${CONTENT_MIN} 个字`);
      contentEl.focus();
      return;
    }

    submit.disabled = true;
    const original = submit.textContent;
    submit.textContent = '提交中…';
    try {
      await api.contributeArticle({
        title,
        content,
        summary: summaryEl.value.trim(),
        channel: channelEl.value,
        categoryId: Number(categoryEl.value) || 0,
        tags: splitTags(tagsEl.value),
      });
      toastOk('已提交，等待审核');
      // 提交成功后跳到「我的投稿」而不是留在原地清空表单：
      // 用户下一步必然是想知道「它现在什么状态」。
      location.hash = '#/me/contributions';
    } catch (err) {
      toastError(err instanceof ApiError ? err.message : '提交失败，请稍后再试');
      submit.disabled = false;
      submit.textContent = original;
    }
  });
}

/* ------------------------------------------------------- 题目表单 -- */

function quizFormHTML(categories) {
  return `
    <form class="card card-pad" id="q-form" novalidate>
      <div class="form-row">
        <div class="field">
          <label for="q-type">题型</label>
          <select class="select" id="q-type">
            <option value="single">单选题</option>
            <option value="multiple">多选题</option>
            <option value="judge">判断题</option>
          </select>
        </div>
        <div class="field">
          <label for="q-difficulty">难度</label>
          <select class="select" id="q-difficulty">
            <option value="easy">简单</option>
            <option value="medium" selected>中等</option>
            <option value="hard">困难</option>
          </select>
        </div>
      </div>

      <div class="field" style="margin-bottom:16px">
        <label for="q-stem">题干 <span class="hint">一句话问清一件事，别把多个知识点塞进一道题</span></label>
        <textarea class="input" id="q-stem" rows="3" maxlength="2000"
          placeholder="例如：容器内 df -h 显示磁盘已满，但 du 求和只有 40%，最可能的原因是什么？"></textarea>
      </div>

      <div class="field" style="margin-bottom:8px">
        <label>选项 <span class="hint" id="q-opt-hint">2–8 个，最少两个有内容</span></label>
        <div class="q-opt-list" id="q-options"></div>
        <div class="q-opt-actions">
          <button class="btn btn-sm" type="button" id="q-add-opt">${icon('plus', 14)}添加选项</button>
          <button class="btn btn-sm" type="button" id="q-del-opt">删除最后一项</button>
        </div>
      </div>

      <div class="field" style="margin-bottom:16px">
        <label>正确答案 <span class="hint" id="q-answer-hint">单选只能选一个；多选至少选两个</span></label>
        <div class="q-answer-list" id="q-answer"></div>
      </div>

      <div class="field" style="margin-bottom:16px">
        <label for="q-analysis">解析 <span class="hint">必填：答错的人要能从这里看懂为什么</span></label>
        <textarea class="input" id="q-analysis" rows="4" maxlength="4000"
          placeholder="说明正确答案为什么对，以及几个干扰项分别错在哪"></textarea>
      </div>

      <div class="form-row">
        <div class="field">
          <label for="q-category">分类</label>
          <select class="select" id="q-category">
            <option value="0">不指定分类</option>
            ${categories.map((c) => `<option value="${esc(String(c.id))}">${esc(c.name)}</option>`).join('')}
          </select>
        </div>
        <div class="field">
          <label for="q-tags">标签 <span class="hint">用逗号分隔，最多 5 个</span></label>
          <input class="input" id="q-tags" placeholder="docker, disk, overlay" />
        </div>
      </div>

      <p class="form-summary" id="q-summary-line"></p>

      <div class="form-actions">
        <button class="btn btn-primary btn-lg" id="q-submit" type="submit">提交审核</button>
        <a class="btn btn-lg" href="#/me/contributions?type=quiz">我的题目投稿</a>
      </div>
    </form>
  `;
}

function quizAsideHTML() {
  return `
    <div class="card side-card" style="margin-bottom:16px">
      <h3>题目怎么算合格</h3>
      <ol class="steps">
        <li>题干只问一件事，不依赖上下文</li>
        <li>干扰项要像真的——从真实错解里挑</li>
        <li>解析说清「为什么对」和「其余为什么错」</li>
        <li>过审后进入题库，按分类与难度归档</li>
      </ol>
    </div>
    <div class="card side-card">
      <h3>过审后</h3>
      <ul class="tips-list">
        <li>题目发布到对应运维方向，计入题库总量</li>
        <li>你获得 5 贡献分，计入问答页的等级徽章</li>
        <li>解析与答案会原样展示，提交前请自己先做一遍</li>
      </ul>
    </div>
  `;
}

function mountQuizForm(root) {
  const form = root.querySelector('#q-form');
  const typeEl = root.querySelector('#q-type');
  const stemEl = root.querySelector('#q-stem');
  const analysisEl = root.querySelector('#q-analysis');
  const categoryEl = root.querySelector('#q-category');
  const difficultyEl = root.querySelector('#q-difficulty');
  const tagsEl = root.querySelector('#q-tags');
  const optionListEl = root.querySelector('#q-options');
  const answerListEl = root.querySelector('#q-answer');
  const optHintEl = root.querySelector('#q-opt-hint');
  const answerHintEl = root.querySelector('#q-answer-hint');
  const lineEl = root.querySelector('#q-summary-line');
  const submit = root.querySelector('#q-submit');

  // 状态集中在两个变量里，DOM 只是它的投影：选项文本数组 + 已选答案下标集合。
  // 每次改动整体重渲染选项与答案区，比就地改 DOM 更不容易出「删了一行但答案
  // 还指向老下标」这种错位。
  const state = {
    type: 'single',
    options: ['', '', '', ''],
    answer: new Set(),
  };

  const visibleOptions = () => {
    if (state.type === 'judge') return ['正确', '错误'];
    return state.options;
  };

  const renderOptions = () => {
    const opts = visibleOptions();
    if (state.type === 'judge') {
      optionListEl.innerHTML = opts.map((t, i) => `
        <div class="q-opt-row is-fixed">
          <span class="q-opt-key">${OPTION_KEYS[i]}</span>
          <span class="q-opt-text">${esc(t)}</span>
        </div>`).join('');
      return;
    }
    optionListEl.innerHTML = state.options.map((t, i) => `
      <div class="q-opt-row">
        <span class="q-opt-key">${OPTION_KEYS[i]}</span>
        <input class="input" data-opt="${i}" value="${esc(t)}" maxlength="300"
               placeholder="选项 ${OPTION_KEYS[i]}" />
      </div>`).join('');
  };

  const renderAnswer = () => {
    const opts = visibleOptions();
    const inputType = state.type === 'multiple' ? 'checkbox' : 'radio';
    answerListEl.innerHTML = opts.map((t, i) => {
      const key = OPTION_KEYS[i];
      const label = String(t || '').trim() || `选项 ${key}`;
      const checked = state.answer.has(key) ? 'checked' : '';
      return `<label class="q-answer-item">
        <input type="${inputType}" name="q-answer" data-key="${key}" ${checked} />
        <span class="q-answer-key">${key}</span>
        <span class="q-answer-text">${esc(label)}</span>
      </label>`;
    }).join('');
  };

  const refreshHint = () => {
    const problems = [];
    const filled = visibleOptions().filter((t) => String(t).trim()).length;
    const stem = stemEl.value.trim().length;
    if (stem < 8) problems.push(`题干还差 ${8 - stem} 字`);
    if (state.type !== 'judge' && filled < 2) problems.push('至少要有两个选项有内容');
    if (state.answer.size === 0) problems.push('还没选正确答案');
    if (state.type === 'multiple' && state.answer.size === 1) problems.push('多选题至少选两个答案');
    if (!analysisEl.value.trim()) problems.push('解析还没写');
    lineEl.textContent = problems.length
      ? problems.join('，') + '。'
      : '看起来可以提交了。提交后进入待审核队列。';
  };

  const syncTypeHints = () => {
    if (state.type === 'judge') {
      optHintEl.textContent = '判断题固定为「正确 / 错误」两项，由系统填写';
      answerHintEl.textContent = '选「正确」或「错误」';
    } else if (state.type === 'multiple') {
      optHintEl.textContent = '2–8 个，最少两个有内容';
      answerHintEl.textContent = '至少选两个；不能全选（全对就没有区分度）';
    } else {
      optHintEl.textContent = '2–8 个，最少两个有内容';
      answerHintEl.textContent = '单选题只能选一个';
    }
  };

  const rerender = () => {
    renderOptions();
    renderAnswer();
    syncTypeHints();
    refreshHint();
  };

  typeEl.addEventListener('change', () => {
    state.type = typeEl.value;
    state.answer.clear();
    rerender();
  });

  optionListEl.addEventListener('input', (ev) => {
    const idx = Number(ev.target.dataset.opt);
    if (Number.isNaN(idx)) return;
    state.options[idx] = ev.target.value;
    // 选项文本变了要同步答案区的显示文案（radio label 里显示的就是选项内容）
    renderAnswer();
    refreshHint();
  });

  answerListEl.addEventListener('change', (ev) => {
    const key = ev.target.dataset.key;
    if (!key) return;
    if (state.type === 'multiple') {
      if (ev.target.checked) state.answer.add(key);
      else state.answer.delete(key);
    } else {
      state.answer.clear();
      state.answer.add(key);
    }
    refreshHint();
  });

  root.querySelector('#q-add-opt').addEventListener('click', () => {
    if (state.type === 'judge') return;
    if (state.options.length >= OPTION_KEYS.length) {
      toastError(`最多 ${OPTION_KEYS.length} 个选项`);
      return;
    }
    state.options.push('');
    rerender();
  });

  root.querySelector('#q-del-opt').addEventListener('click', () => {
    if (state.type === 'judge') return;
    if (state.options.length <= 2) {
      toastError('至少要保留两个选项');
      return;
    }
    state.options.pop();
    // 删掉的那一项如果被选作答案，答案也要跟着失效，否则会提交一个
    // 指向不存在选项的 key，后端会直接拒掉且提示很难懂。
    const last = OPTION_KEYS[state.options.length];
    state.answer.delete(last);
    rerender();
  });

  [stemEl, analysisEl].forEach((el) => el.addEventListener('input', refreshHint));

  rerender();

  form.addEventListener('submit', async (ev) => {
    ev.preventDefault();

    const stem = stemEl.value.trim();
    if (stem.length < 8) {
      toastError('题干至少 8 个字，否则读者不知道在问什么');
      stemEl.focus();
      return;
    }
    if (!analysisEl.value.trim()) {
      toastError('请填写解析：没有解析的题，答错的人也不知道错在哪');
      analysisEl.focus();
      return;
    }

    let options = [];
    if (state.type !== 'judge') {
      options = state.options
        .map((t, i) => ({ key: OPTION_KEYS[i], text: String(t).trim() }))
        .filter((o) => o.text);
      if (options.length < 2) {
        toastError('至少要有两个选项有内容');
        return;
      }
    }
    if (state.answer.size === 0) {
      toastError('请选择正确答案');
      return;
    }
    if (state.type === 'multiple' && state.answer.size < 2) {
      toastError('多选题至少选两个正确答案');
      return;
    }
    if (state.type === 'multiple' && state.answer.size === options.length) {
      toastError('不能把所有选项都设为正确答案');
      return;
    }

    submit.disabled = true;
    const original = submit.textContent;
    submit.textContent = '提交中…';
    try {
      await api.contributeQuiz({
        stem,
        type: state.type,
        options,
        correct: [...state.answer].sort(),
        analysis: analysisEl.value.trim(),
        difficulty: difficultyEl.value,
        categoryId: Number(categoryEl.value) || 0,
        tags: splitTags(tagsEl.value),
      });
      toastOk('已提交，等待审核');
      location.hash = '#/me/contributions?type=quiz';
    } catch (err) {
      toastError(err instanceof ApiError ? err.message : '提交失败，请稍后再试');
      submit.disabled = false;
      submit.textContent = original;
    }
  });
}

/* =================================================== 我的投稿 ====== */

export async function myContributionsPage(ctx) {
  if (!isLoggedIn()) {
    return {
      html: pageHead('我的投稿', '查看投稿的审核状态，以及被驳回时需要改什么。') +
        emptyHTML({
          iconName: 'book',
          title: '登录后才能查看投稿',
          desc: '投稿与账号绑定，登录后这里会列出你提交过的全部内容。',
          actionHTML: '<button class="btn btn-primary" type="button" data-act="login">去登录</button>',
        }),
      title: '我的投稿',
    };
  }

  const kind = ctx.params.get('type') === 'quiz' ? 'quiz' : 'article';
  const page = Math.max(1, Number(ctx.params.get('page') || 1));

  // 两个来源是两张不同的表，各翻各的页：合并成一条跨表游标列表要处理
  // 「文章 30 篇、题目 200 道」时的交错排序，复杂度远大于收益。
  const data = kind === 'quiz'
    ? await api.myQuizContributions({ page, pageSize: PAGE_SIZE })
    : await api.myContributions({ page, pageSize: PAGE_SIZE });
  const list = Array.isArray(data && data.list) ? data.list : [];
  const total = Number(data && data.total) || 0;

  const unit = kind === 'quiz' ? '道' : '篇';
  const body = list.length
    ? `<div class="contrib-list">${list.map((item) => contribRowHTML(item, kind)).join('')}</div>
       ${pagerHTML({ page, pageSize: PAGE_SIZE, total })}`
    : emptyHTML({
        iconName: 'book',
        title: kind === 'quiz' ? '还没有投过题目' : '还没有投过稿',
        desc: kind === 'quiz'
          ? '把刷题时踩到的坑整理成一道题，比凭空出题更有价值——你知道错的人会错在哪。'
          : '把你解决过的问题写成步骤，是最快积累个人技术品牌的方式。',
        actionHTML: kind === 'quiz'
          ? '<a class="btn btn-primary" href="#/contribute?type=quiz">去投稿题目</a>'
          : '<a class="btn btn-primary" href="#/contribute">去投稿</a>',
      });

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a>
      <span class="crumb-sep">/</span>
      <span>我的投稿</span>
    </nav>

    ${pageHead('我的投稿', total ? `共 ${total} ${unit}，通过审核的会出现在对应栏目里。` : '')}

    <div class="contrib-tabs" role="tablist">
      <a class="contrib-tab ${kind === 'article' ? 'is-active' : ''}" href="#/me/contributions">实战教程</a>
      <a class="contrib-tab ${kind === 'quiz' ? 'is-active' : ''}" href="#/me/contributions?type=quiz">运维题目</a>
    </div>

    <div class="page-tools">
      <span class="muted-note">投稿只进待审队列，发布与否由运营决定。</span>
      <a class="btn btn-sm btn-primary" href="${kind === 'quiz' ? '#/contribute?type=quiz' : '#/contribute'}">
        ${icon('plus', 14)}新建投稿
      </a>
    </div>

    ${body}
  `;

  const mount = (root) => {
    // keepPage: true 不能省——setHashQuery 默认会先删掉 page 再写补丁，
    // 用它翻页会变成「永远停在第一页」。
    bindPager(root, (target) => setHashQuery({ page: target }, { keepPage: true }));
  };

  return { html, mount, title: '我的投稿' };
}

/* ------------------------------------------------------------ 内部 -- */

function pageHead(title, lede) {
  return `<div class="page-head" style="padding-top:8px">
    <h1>${esc(title)}</h1>
    ${lede ? `<p class="lede">${esc(lede)}</p>` : ''}
  </div>`;
}

function contribRowHTML(item, kind) {
  const meta = STATUS_META[item.status] || { label: item.status || '未知', tone: '' };
  const chip = `<span class="chip ${meta.tone ? 'chip-' + meta.tone : ''}">${esc(meta.label)}</span>`;

  // 已发布给可点的标题（能直接看读者看到的样子），未发布的给纯文本——
  // 未发布文章对非管理员是 404，做成链接只会让人点出一次报错。
  // 题目没有单题前台页，一律纯文本。
  const linkable = kind === 'article' && item.status === 'published' && item.slug;
  const head = linkable
    ? `<a class="contrib-title" href="${esc('#/article/' + encodeURIComponent(item.slug))}">${esc(item.title)}</a>`
    : `<span class="contrib-title">${esc(item.title)}</span>`;

  return `<div class="contrib-row">
    <div class="contrib-main">
      <div class="contrib-top">${head}${chip}</div>
      <div class="contrib-meta">
        <span class="meta-item">${icon('clock', 12)}提交于 ${esc(fmtDate(item.createdAt))}</span>
        ${item.publishedAt ? `<span class="meta-item">${icon('check', 12)}发布于 ${esc(fmtDate(item.publishedAt))}</span>` : ''}
        ${kind === 'article' && item.status === 'published'
          ? `<span class="meta-item">${icon('eye', 12)}${fmtNum(item.viewCount)} 次阅读</span>` : ''}
      </div>
      ${item.reviewNote ? `<div class="contrib-note">
        ${icon('info', 13)}<span><strong>审核意见：</strong>${esc(item.reviewNote)}</span>
      </div>` : ''}
    </div>
  </div>`;
}

// splitTags 见 util.js（后台批量导入也用同一份切分规则）。
