/* ============================================================================
   pages-qa.js — 问答广场：列表 / 提问 / 详情。

   与「悬赏提问」(#/ask) 的分工要写在页面上，否则用户会以为这里是付费通道：
   悬赏是花钱找专家一对一，问答是免费社区互助。两者共用"描述问题"的形态，
   但一个产生订单、一个产生内容。

   三个页面共同遵守两条规则：
   1) 游客能读（搜索引擎落地页），写操作才要求登录；
   2) 权限判断全部来自服务端下发的 canAccept / canClose / myAnswerId，
      前端不自己按 userId 比较——那种判断在改了身份模型后必然失准。
   ========================================================================= */

import { api, isLoggedIn, currentUser, ApiError } from './api.js';
import { esc, fmtRelative, fmtNum, icon, renderMarkdown, setHashQuery } from './util.js';
import { avatarHTML, bindPager, emptyHTML, pagerHTML, toastError, toastOk } from './ui.js';

const PAGE_SIZE = 10;

// STATUS_META 与后端 model.QaStatus* 逐字对齐。
const STATUS_META = {
  open: { label: '待解答', tone: 'warn' },
  resolved: { label: '已解决', tone: 'ok' },
  closed: { label: '已关闭', tone: '' },
};

const TABS = [
  { key: '', label: '全部' },
  { key: 'open', label: '待解答' },
  { key: 'resolved', label: '已解决' },
];

/* ======================================================= 列表 ====== */

export async function qaListPage(ctx) {
  const params = ctx.params;
  const status = params.get('status') || '';
  const keyword = params.get('keyword') || '';
  const page = Math.max(1, Number(params.get('page') || 1));

  const [data, skills] = await Promise.all([
    api.qaQuestions({ status, keyword, page, pageSize: PAGE_SIZE }),
    // 技能字典取不到不影响列表：只是标签退化成「#12」这种编号。
    api.skills().catch(() => []),
  ]);

  const list = Array.isArray(data && data.list) ? data.list : [];
  const skillName = new Map((skills || []).map((s) => [Number(s.id), s.name]));
  const stats = data && data.stats;

  const body = list.length
    ? list.map((q) => questionCardHTML(q, skillName)).join('')
    : emptyHTML({
        iconName: 'chat',
        title: keyword ? '没有找到相关的问题' : '还没有人提问',
        desc: keyword
          ? '换个说法试试，或者直接把你的问题发出来——总会有人遇到过。'
          : '第一个提问的人往往也是第一个得到答案的人。',
        actionHTML: '<a class="btn btn-primary" href="#/qa/ask">我要提问</a>',
      });

  return {
    title: '问答广场',
    html: `
      <section class="qa-hero">
        <div class="qa-hero-main">
          <h1>问答广场</h1>
          <p>遇到问题就写下来。回答的人不必是专家，只要真的踩过这个坑。</p>
        </div>
        <div class="qa-hero-stats">
          <span><b>${fmtNum((stats && stats.open) || 0)}</b>待解答</span>
          <span><b>${fmtNum((stats && stats.resolved) || 0)}</b>已解决</span>
          <span><b>${fmtNum((stats && stats.answers) || 0)}</b>条回答</span>
        </div>
        <a class="btn btn-primary btn-lg qa-hero-ask" href="#/qa/ask">${icon('plus', 15)} 我要提问</a>
      </section>

      <div class="qa-toolbar">
        <div class="qa-tabs">
          ${TABS.map((t) => `<button class="qa-tab ${t.key === status ? 'is-active' : ''}"
             type="button" data-status="${esc(t.key)}">${esc(t.label)}</button>`).join('')}
        </div>
        <form class="qa-search" data-qa-search>
          <input class="input" name="keyword" value="${esc(keyword)}"
                 placeholder="搜问题，例如：磁盘满了" />
          <button class="btn" type="submit">搜索</button>
        </form>
      </div>

      <div class="qa-list">${body}</div>
      ${pagerHTML({ page, pageSize: PAGE_SIZE, total: (data && data.total) || 0, unit: '个问题' })}
    `,
    mount: (root) => {
      root.addEventListener('click', (e) => {
        const tab = e.target.closest('[data-status]');
        if (tab) {
          // 切 tab 要回到第一页：留在第 3 页看「已解决」，往往直接是空页。
          setHashQuery({ status: tab.getAttribute('data-status'), page: '' });
        }
      });

      const form = root.querySelector('[data-qa-search]');
      if (form) {
        form.addEventListener('submit', (e) => {
          e.preventDefault();
          setHashQuery({ keyword: form.elements.keyword.value.trim(), page: '' });
        });
      }

      bindPager(root, (next) => setHashQuery({ page: next }, { keepPage: true }));
    },
  };
}

function questionCardHTML(q, skillName) {
  const meta = STATUS_META[q.status] || STATUS_META.open;
  const tags = (q.skillIds || []).slice(0, 4)
    .map((id) => `<span class="chip">${esc(skillName.get(Number(id)) || `#${id}`)}</span>`)
    .join('');

  return `
    <a class="qa-card" href="#/qa/${q.id}">
      <div class="qa-card-main">
        <div class="qa-card-title">
          ${q.hasAccepted ? `<span class="qa-solved" title="已有采纳回答">${icon('check', 12)}</span>` : ''}
          <h2>${esc(q.title)}</h2>
        </div>
        ${q.excerpt ? `<p class="qa-card-excerpt">${esc(q.excerpt)}</p>` : ''}
        ${tags ? `<div class="qa-card-tags">${tags}</div>` : ''}
        <div class="qa-card-meta">
          ${avatarHTML(q.nickname, q.avatar, 20)}
          <span>${esc(q.nickname || '运维用户')}</span>
          ${q.authorRole === 'expert' ? '<span class="chip chip-brand">专家</span>' : ''}
          <span class="qa-dot">·</span>
          <span>${esc(fmtRelative(q.lastAnswerAt || q.createdAt))}</span>
        </div>
      </div>
      <div class="qa-card-side">
        <div class="qa-card-count ${q.answerCount > 0 ? 'is-hot' : ''}">
          <b>${fmtNum(q.answerCount)}</b><span>回答</span>
        </div>
        <div class="qa-card-views">${fmtNum(q.viewCount)} 浏览</div>
        <span class="chip ${meta.tone ? `chip-${meta.tone}` : ''}">${meta.label}</span>
      </div>
    </a>`;
}

/* ======================================================= 提问 ====== */

export async function qaAskPage(ctx) {
  if (!isLoggedIn()) {
    return {
      title: '我要提问',
      html: emptyHTML({
        iconName: 'chat',
        title: '登录后才能提问',
        desc: '问题会绑定账号，回答与采纳通知也都发到这个账号下。',
        actionHTML: '<button class="btn btn-primary" type="button" data-act="login">去登录</button>',
      }),
    };
  }

  const skills = await api.skills().catch(() => []);
  const top = (skills || []).slice(0, 24);

  return {
    title: '我要提问',
    html: `
      <nav class="crumb">
        <a href="#/">首页</a><span class="crumb-sep">/</span>
        <a href="#/qa">问答广场</a><span class="crumb-sep">/</span>
        <span>我要提问</span>
      </nav>

      <div class="detail-layout">
        <form class="card card-pad" id="qa-form" novalidate>
          <h1 class="block-title">把问题说清楚</h1>
          <p class="muted-note" style="margin-bottom:18px">
            写清「现象 + 环境 + 已经试过什么」。只说"打不开"没人能答，
            说"CentOS 7 上 nginx 502，后端 keepalive 设 65s 超时日志里是 upstream prematurely closed"
            十分钟就有人回。
          </p>

          <div class="field" style="margin-bottom:16px">
            <label for="qa-title">标题 <span class="hint">6–200 字，写故障现象而不是"求助"</span></label>
            <input class="input" id="qa-title" maxlength="200"
                   placeholder="例如：K8s Pod 频繁 OOMKilled，limit 已调到 2Gi" />
          </div>

          <div class="field" style="margin-bottom:16px">
            <label for="qa-desc">详细描述 <span class="hint">至少 10 字，支持 Markdown</span></label>
            <textarea class="input code-input" id="qa-desc" rows="12"
              placeholder="## 现象&#10;&#10;## 环境（系统 / 版本 / 部署方式）&#10;&#10;## 已尝试&nbsp;&#10;&#10;## 报错日志"></textarea>
          </div>

          <div class="field" style="margin-bottom:16px">
            <label>技术方向 <span class="hint">选填，最多 3 个，用于让答得上的人看到</span></label>
            <div class="qa-skill-pick" id="qa-skills">
              ${top.map((s) => `<button class="chip chip-pick" type="button"
                 data-skill="${esc(String(s.id))}">${esc(s.name)}</button>`).join('')
                || '<span class="muted-note">技能字典还是空的，可以先不选。</span>'}
            </div>
          </div>

          <p class="form-summary" id="qa-hint"></p>

          <div class="form-actions">
            <button class="btn btn-primary btn-lg" id="qa-submit" type="submit">发布问题</button>
            <a class="btn btn-lg" href="#/qa">取消</a>
          </div>
        </form>

        <aside>
          <div class="card side-card" style="margin-bottom:16px">
            <h3>发出去之后</h3>
            <ol class="steps">
              <li>问题立刻公开，游客也能看到</li>
              <li>有人回答会顶到列表前面</li>
              <li>你可以在回答里采纳一个作为答案</li>
              <li>回答被采纳的人会获得贡献积分</li>
            </ol>
          </div>
          <div class="card side-card">
            <h3>想让专家优先处理？</h3>
            <p class="muted-note">问答广场是免费互助，不保证有人答。
              急事与生产故障请走悬赏通道，由认证专家接单并承诺响应时间。</p>
          </div>
        </aside>
      </div>
    `,
    mount: (root) => {
      const form = root.querySelector('#qa-form');
      const titleEl = root.querySelector('#qa-title');
      const descEl = root.querySelector('#qa-desc');
      const hintEl = root.querySelector('#qa-hint');
      const submit = root.querySelector('#qa-submit');
      const picked = new Set();

      root.querySelectorAll('[data-skill]').forEach((btn) => {
        btn.addEventListener('click', () => {
          const id = Number(btn.getAttribute('data-skill'));
          if (picked.has(id)) {
            picked.delete(id);
            btn.classList.remove('is-on');
            return;
          }
          if (picked.size >= 3) {
            toastError('最多选 3 个技术方向');
            return;
          }
          picked.add(id);
          btn.classList.add('is-on');
        });
      });

      const refresh = () => {
        const t = titleEl.value.trim().length;
        const d = descEl.value.trim().length;
        const problems = [];
        if (t < 6) problems.push(`标题还差 ${6 - t} 字`);
        if (d < 10) problems.push(`描述还差 ${10 - d} 字`);
        hintEl.textContent = problems.length ? problems.join('，') + '。' : '可以发布了。';
      };
      titleEl.addEventListener('input', refresh);
      descEl.addEventListener('input', refresh);
      refresh();

      form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const title = titleEl.value.trim();
        const description = descEl.value.trim();
        if (title.length < 6) { toastError('标题至少 6 个字'); titleEl.focus(); return; }
        if (description.length < 10) { toastError('描述至少 10 个字'); descEl.focus(); return; }

        submit.disabled = true;
        const original = submit.textContent;
        submit.textContent = '发布中…';
        try {
          const q = await api.askQuestion({
            title, description, attachments: [], skillIds: Array.from(picked),
          });
          toastOk('问题已发布');
          // 直接进详情：用户下一步就是等回答，而详情页能自己复制链接分享。
          location.hash = `#/qa/${q.id}`;
        } catch (err) {
          toastError(err instanceof ApiError ? err.message : '发布失败，请稍后再试');
          submit.disabled = false;
          submit.textContent = original;
        }
      });
    },
  };
}

/* ======================================================= 详情 ====== */

export async function qaDetailPage(ctx, id) {
  let q;
  try {
    q = await api.qaQuestion(id);
  } catch (err) {
    return {
      title: '问题不存在',
      html: emptyHTML({
        iconName: 'chat',
        title: err instanceof ApiError ? err.message : '问题不存在',
        desc: '它可能已被作者删除。',
        actionHTML: '<a class="btn btn-primary" href="#/qa">回到问答广场</a>',
      }),
    };
  }

  const meta = STATUS_META[q.status] || STATUS_META.open;
  const canWrite = isLoggedIn();

  return {
    title: q.title,
    html: `
      <nav class="crumb">
        <a href="#/">首页</a><span class="crumb-sep">/</span>
        <a href="#/qa">问答广场</a><span class="crumb-sep">/</span>
        <span>#${q.id}</span>
      </nav>

      <div class="detail-layout">
        <div>
          <article class="card card-pad qa-question">
            <div class="qa-q-head">
              <h1>${esc(q.title)}</h1>
              <div class="qa-q-meta">
                ${avatarHTML(q.nickname, q.avatar, 24)}
                <span>${esc(q.nickname || '运维用户')}</span>
                ${q.authorRole === 'expert' ? '<span class="chip chip-brand">专家</span>' : ''}
                <span class="qa-dot">·</span>
                <span>${esc(fmtRelative(q.createdAt))}</span>
                <span class="qa-dot">·</span>
                <span>${fmtNum(q.viewCount)} 浏览</span>
                <span class="chip ${meta.tone ? `chip-${meta.tone}` : ''}" style="margin-left:auto">${meta.label}</span>
              </div>
            </div>

            <div class="prose qa-q-body">${renderMarkdown(q.description || '')}</div>

            ${(q.attachments || []).length
              ? `<p class="muted-note">${icon('link', 13)} 附带 ${q.attachments.length} 个日志/截图附件</p>`
              : ''}

            ${q.canAccept || q.canClose ? `<div class="qa-q-actions">
              ${q.canAccept ? '<span class="muted-note">在下面的回答里点「采纳」选定答案。</span>' : ''}
              ${q.canClose ? '<button class="btn btn-sm" type="button" data-close>关闭问题</button>' : ''}
            </div>` : ''}
          </article>

          <section class="card card-pad qa-answers">
            <div class="block-head">
              <h2 class="block-title">${fmtNum(q.answerCount)} 条回答</h2>
              ${q.status === 'resolved' ? '<span class="muted-note">提问人已采纳其中一条</span>' : ''}
            </div>

            ${(q.answers || []).length
              ? q.answers.map((a) => answerHTML(a, q)).join('')
              : `<p class="muted-note" style="padding:12px 0">还没有人回答。${canWrite ? '知道答案就写下来吧。' : '登录后可以回答。'}</p>`}

            <div class="qa-answer-form">
              ${canWrite
                ? `<h3 class="block-title">${q.myAnswerId ? '编辑我的回答' : '写下你的回答'}</h3>
                   <textarea class="input code-input" id="qa-answer" rows="7"
                     placeholder="说清「为什么」和「怎么验证修好了」。贴命令比贴结论有用。">${
                       esc((q.answers.find((a) => a.id === q.myAnswerId) || {}).content || '')
                     }</textarea>
                   <div class="form-actions" style="margin-top:12px">
                     <button class="btn btn-primary" type="button" data-submit-answer>${
                       q.myAnswerId ? '保存修改' : '提交回答'
                     }</button>
                     <span class="muted-note">一个人只能答一次，想补充就编辑原回答。</span>
                   </div>`
                : `<p class="muted-note">登录后可以回答这个问题。</p>
                   <button class="btn btn-primary" type="button" data-act="login">去登录</button>`}
            </div>
          </section>
        </div>

        <aside>
          <div class="card side-card">
            <h3>关于这个回答区</h3>
            <ul class="tips-list">
              <li>被采纳的回答会置顶并给作者 +10 贡献分</li>
              <li>点赞 +1 分，自己给自己点不加分</li>
              <li>回答可以编辑，不能删已采纳的那条</li>
              <li>新账号前 3 条回答需要人工审核</li>
            </ul>
          </div>
        </aside>
      </div>
    `,
    mount: (root) => {
      const submitBtn = root.querySelector('[data-submit-answer]');
      if (submitBtn) {
        submitBtn.addEventListener('click', async () => {
          const ta = root.querySelector('#qa-answer');
          const content = ta.value.trim();
          if (content.length < 10) { toastError('回答至少 10 个字'); ta.focus(); return; }
          submitBtn.disabled = true;
          const original = submitBtn.textContent;
          submitBtn.textContent = '提交中…';
          try {
            if (q.myAnswerId) {
              await api.updateAnswer(q.myAnswerId, { content });
              toastOk('已保存');
            } else {
              const a = await api.answerQuestion(q.id, { content });
              // 新账号的回答会先进审核：这里必须说清楚，否则用户会以为没发出去。
              toastOk(a.status === 'pending' ? '已提交，审核通过后对其他人生效' : '回答已发布');
            }
            window.__reloadPage();
          } catch (err) {
            toastError(err instanceof ApiError ? err.message : '提交失败');
            submitBtn.disabled = false;
            submitBtn.textContent = original;
          }
        });
      }

      root.addEventListener('click', async (e) => {
        const acceptBtn = e.target.closest('[data-accept]');
        if (acceptBtn) {
          const aid = acceptBtn.getAttribute('data-accept');
          try {
            await api.acceptAnswer(q.id, aid);
            toastOk('已采纳，感谢确认答案');
            window.__reloadPage();
          } catch (err) {
            toastError(err instanceof ApiError ? err.message : '采纳失败');
          }
          return;
        }

        const likeBtn = e.target.closest('[data-like]');
        if (likeBtn) {
          const aid = likeBtn.getAttribute('data-like');
          const on = likeBtn.classList.contains('is-on');
          try {
            const r = on ? await api.unlikeAnswer(aid) : await api.likeAnswer(aid);
            likeBtn.classList.toggle('is-on', !on);
            const num = likeBtn.querySelector('b');
            if (num) num.textContent = String(r.score);
          } catch (err) {
            toastError(err instanceof ApiError ? err.message : '操作失败');
          }
          return;
        }

        const closeBtn = e.target.closest('[data-close]');
        if (closeBtn) {
          try {
            await api.closeQuestion(q.id);
            toastOk('问题已关闭');
            window.__reloadPage();
          } catch (err) {
            toastError(err instanceof ApiError ? err.message : '关闭失败');
          }
        }
      });
    },
  };
}

function answerHTML(a, q) {
  const pending = a.status === 'pending';
  const rejected = a.status === 'rejected';
  const me = currentUser();
  const mine = me && Number(me.id) === Number(a.userId);

  return `
    <article class="qa-answer ${a.isAccepted ? 'is-accepted' : ''}">
      ${a.isAccepted ? `<div class="qa-accepted-tag">${icon('check', 12)} 已采纳</div>` : ''}
      <div class="qa-a-head">
        ${avatarHTML(a.nickname, a.avatar, 28)}
        <div class="qa-a-who">
          <b>${esc(a.nickname || '运维用户')}</b>
          <span class="qa-a-sub">
            ${a.isExpert ? '<span class="chip chip-brand">专家</span>' : ''}
            <span class="qa-badge">${esc(a.badge || '新手')}</span>
            <span>${esc(fmtRelative(a.createdAt))}</span>
          </span>
        </div>
        <div class="qa-a-ops">
          ${pending ? '<span class="chip chip-warn">审核中</span>' : ''}
          ${rejected ? '<span class="chip chip-danger">未通过</span>' : ''}
          ${q.canAccept && !a.isAccepted && !pending
            ? `<button class="btn btn-sm" type="button" data-accept="${a.id}">采纳</button>` : ''}
          <button class="btn btn-sm qa-like ${a.liked ? 'is-on' : ''}" type="button"
                  data-like="${a.id}" ${pending ? 'disabled' : ''}>
            ${icon('thumb', 13)}<b>${fmtNum(a.score)}</b>
          </button>
        </div>
      </div>

      <div class="prose qa-a-body">${renderMarkdown(a.content || '')}</div>

      ${rejected && mine && a.reviewNote
        ? `<p class="admin-hint admin-hint-warn">驳回原因：${esc(a.reviewNote)}</p>` : ''}
      ${pending && mine
        ? '<p class="muted-note">这条回答正在审核，只有你自己能看到。通过后会出现在其他人的页面上。</p>' : ''}
    </article>`;
}
