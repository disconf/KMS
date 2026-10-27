/* ============================================================================
   pages-admin.js — 后台管理。

   这一块解决的是「内容有了，运营还在用 curl 维护」的问题：
   审核队列、资讯订阅源、内容上下线、工具四类教程、资源外链、专家审核、订单处理。

   两个刻意的约定：

   1. **前端的 role 判断只决定"入口摆不摆"，不是权限。**
      所有 /admin 接口在服务端都有 RequireRole(admin)，改一行 JS 绕不过去。
      这里做判断是为了别让人点进来才发现一片 403。

   2. **写操作成功后一律 __reloadPage()，不在前端自己改状态。**
      订单、审核、上下线都有服务端的条件更新（CAS），前端照着改一遍
      只会造出"界面显示已通过、库里还是待审"的假象。
   ========================================================================= */

import { api, currentUser, isLoggedIn, ApiError } from './api.js';
import {
  esc, icon, fmtNum, fmtDate, fmtDateTime, fmtRelative, setHashQuery, qs, splitTags,
  DIFFICULTY_LABEL, QUIZ_TYPE_LABEL, BOOK_LEVEL_LABEL,
} from './util.js';
import {
  emptyHTML, pagerHTML, bindPager, toastOk, toastError, openModal, closeModal,
} from './ui.js';
import {
  ORDER_STATUS, ORDER_TRANSITIONS, ORDER_TYPE_LABEL, fmtCents, statusChipHTML,
} from './orders.js';
import { enableMarkdownImages } from './md-image.js';

/* ------------------------------------------------------------ 公共 -- */

const STATUS_LABEL = {
  draft: '草稿',
  pending: '待审核',
  published: '已发布',
  rejected: '已驳回',
  offline: '已下线',
};

const STATUS_TONE = {
  draft: '',
  pending: 'chip-warn',
  published: 'chip-ok',
  rejected: 'chip-danger',
  offline: 'chip-info',
};

function statusChip(status) {
  const label = STATUS_LABEL[status] || status || '—';
  const tone = STATUS_TONE[status] || '';
  return `<span class="chip ${tone}">${esc(label)}</span>`;
}

/** 后台子导航。badge 只有"待办"才给，别把每个菜单都挂个数字。 */
const ADMIN_NAV = [
  { href: '#/admin',           label: '概览',     test: (p) => p === '/admin' },
  { href: '#/admin/review',    label: '待审内容', test: (p) => p.startsWith('/admin/review') },
  { href: '#/admin/news',      label: '资讯源',   test: (p) => p.startsWith('/admin/news') },
  { href: '#/admin/articles',  label: '文章',     test: (p) => p.startsWith('/admin/articles') },
  { href: '#/admin/tools',     label: '工具教程', test: (p) => p.startsWith('/admin/tools') },
  { href: '#/admin/resources', label: '资源外链', test: (p) => p.startsWith('/admin/resources') },
  { href: '#/admin/quiz',      label: '题库',     test: (p) => p.startsWith('/admin/quiz') },
  { href: '#/admin/qa',        label: '问答',     test: (p) => p.startsWith('/admin/qa') },
  { href: '#/admin/experts',   label: '专家审核', test: (p) => p.startsWith('/admin/experts') },
  { href: '#/admin/withdrawals', label: '提现审批', test: (p) => p.startsWith('/admin/withdrawals') },
  { href: '#/admin/finance',   label: '对账',     test: (p) => p.startsWith('/admin/finance') },
  { href: '#/admin/orders',    label: '订单',     test: (p) => p.startsWith('/admin/orders') },
  { href: '#/admin/users',     label: '用户',     test: (p) => p.startsWith('/admin/users') },
  { href: '#/admin/speak',     label: '发言权限', test: (p) => p.startsWith('/admin/speak') },
  { href: '#/admin/settings',  label: '配置',     test: (p) => p.startsWith('/admin/settings') },
];

/**
 * 后台页面外壳。
 *
 * 子导航做成后端式的二级菜单而不是塞进顶栏：顶栏是给读者看内容的，
 * 后台是另一套信息架构，混在一起会让读者也看见"审核队列"这种词。
 */
function adminShell(path, { title, desc = '', actions = '', body }) {
  const nav = ADMIN_NAV.map((n) => `
    <a class="admin-nav-item ${n.test(path) ? 'is-active' : ''}" href="${n.href}"
       ${n.test(path) ? 'aria-current="page"' : ''}>${esc(n.label)}</a>`).join('');

  return `<div class="admin">
    <header class="admin-head">
      <div class="admin-head-main">
        <div class="admin-kicker">${icon('shield', 13)}运营后台</div>
        <h1 class="admin-title">${esc(title)}</h1>
        ${desc ? `<p class="admin-desc">${esc(desc)}</p>` : ''}
      </div>
      ${actions ? `<div class="admin-actions">${actions}</div>` : ''}
    </header>
    <nav class="admin-subnav" aria-label="后台导航">${nav}</nav>
    ${body}
  </div>`;
}

/** 未登录 / 非管理员：给出说明，而不是一片 403 的空白。 */
function adminGate() {
  if (!isLoggedIn()) {
    return {
      title: '运营后台',
      html: adminShell('/admin', {
        title: '需要先登录',
        desc: '后台只对平台运营开放，登录后如果仍然看不到，请确认账号角色。',
        body: emptyHTML({
          iconName: 'shield',
          title: '请先登录运营账号',
          desc: '登录后如果你有后台权限，这里会显示待审内容、订阅源与内容管理。',
          actionHTML: '<button class="btn btn-primary" type="button" data-need-login>登录</button>',
        }),
      }),
      mount: (root) => {
        const btn = root.querySelector('[data-need-login]');
        if (btn) btn.addEventListener('click', () => window.dispatchEvent(new CustomEvent('opskb:need-login')));
      },
    };
  }

  const user = currentUser() || {};
  if (user.role !== 'admin') {
    return {
      title: '无权访问',
      html: adminShell('/admin', {
        title: '没有后台权限',
        desc: '当前账号不是平台运营账号。',
        body: emptyHTML({
          iconName: 'shield',
          title: '这个账号进不了后台',
          desc: '后台权限由服务端按角色判定。需要开通请联系平台管理员。',
          actionHTML: '<a class="btn" href="#/">回到首页</a>',
        }),
      }),
    };
  }
  return null;
}

/** 栏目展示名。栏目是代码里的注册表，前端不抄一份名称。 */
function channelName(ctx, slug) {
  const ch = (ctx.state.channels || []).find((c) => c.slug === slug);
  return ch ? ch.name : (slug || '—');
}

/** 后台的日期列：新内容用相对时间更好读，超过 30 天 fmtRelative 会自己退成日期。 */
const when = (iso) => (iso ? `<time title="${esc(fmtDateTime(iso))}">${esc(fmtRelative(iso))}</time>` : '—');

/* ------------------------------------------------------------ 概览 -- */

export async function adminDashboardPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const [pendingArts, pendingExperts, payOrders, sources, wdPending, wdApproved, speakPending] = await Promise.allSettled([
    api.adminArticles({ status: 'pending', pageSize: 5 }),
    api.adminExperts({ status: 'pending', pageSize: 1 }),
    api.adminOrders({ status: 'pending_pay', pageSize: 1 }),
    api.newsSources(),
    api.adminWithdrawals({ status: 'pending', pageSize: 1 }),
    api.adminWithdrawals({ status: 'approved', pageSize: 1 }),
    api.adminSpeakApplications({ status: 'pending', pageSize: 1 }),
  ]);

  const artPage = pendingArts.status === 'fulfilled' ? pendingArts.value : { list: [], total: 0 };
  const expertTotal = pendingExperts.status === 'fulfilled' ? pendingExperts.value.total : 0;
  const payTotal = payOrders.status === 'fulfilled' ? payOrders.value.total : 0;
  const wdPendingTotal = wdPending.status === 'fulfilled' ? wdPending.value.total : 0;
  const wdApprovedTotal = wdApproved.status === 'fulfilled' ? wdApproved.value.total : 0;
  const speakTotal = speakPending.status === 'fulfilled' ? speakPending.value.total : 0;
  const srcList = sources.status === 'fulfilled' ? sources.value : [];
  const brokenSrc = srcList.filter((s) => s.lastStatus === 'failed');
  const pendingNews = srcList.reduce((max, s) => Math.max(max, Number(s.pendingTotal) || 0), 0);

  const statCard = (label, value, hint, href, tone = '') => `
    <a class="admin-stat ${tone}" href="${href}">
      <span class="admin-stat-label">${esc(label)}</span>
      <strong class="admin-stat-value">${fmtNum(value)}</strong>
      <span class="admin-stat-hint">${esc(hint)}</span>
    </a>`;

  const recent = (artPage.list || []).map((it) => `
    <a class="admin-mini-row" href="#/admin/review">
      <span class="admin-mini-title">${esc(it.title)}</span>
      <span class="admin-mini-meta">${esc(channelName(ctx, it.channel))} · ${when(it.createdAt)}</span>
    </a>`).join('');

  const broken = brokenSrc.map((s) => `
    <div class="admin-mini-row is-static">
      <span class="admin-mini-title">${esc(s.name)}</span>
      <span class="admin-mini-meta is-danger">${esc(s.lastError || '抓取失败')}</span>
    </div>`).join('');

  const body = `
    <div class="admin-stats">
      ${statCard('待审内容', artPage.total || 0, '投稿与抓取进来的内容', '#/admin/review', (artPage.total ? 'is-warn' : ''))}
      ${statCard('待审专家', expertTotal, '入驻申请等待处理', '#/admin/experts?status=pending')}
      ${statCard('待审提现', wdPendingTotal, '钱还冻在专家账户里', '#/admin/withdrawals?status=pending', (wdPendingTotal ? 'is-warn' : ''))}
      ${statCard('待打款', wdApprovedTotal, '已通过，等你线下转账', '#/admin/withdrawals?status=approved', (wdApprovedTotal ? 'is-warn' : ''))}
      ${statCard('待审发言', speakTotal, '实名通过后才有发言权', '#/admin/speak?tab=pending', (speakTotal ? 'is-warn' : ''))}
      ${statCard('待支付订单', payTotal, '超时会自动关闭', '#/admin/orders?status=pending_pay')}
      ${statCard('订阅源异常', brokenSrc.length, `${srcList.length} 个源在跑`, '#/admin/news', (brokenSrc.length ? 'is-danger' : ''))}
    </div>

    <div class="admin-cols">
      <section class="card card-pad">
        <div class="admin-sec-head">
          <h2>最近待审</h2>
          <a class="link-sm" href="#/admin/review">全部 ${fmtNum(artPage.total || 0)} 条${icon('right', 11)}</a>
        </div>
        ${recent || '<p class="muted-note">没有待审内容。订阅源抓回来的新动态会先进这里。</p>'}
      </section>

      <section class="card card-pad">
        <div class="admin-sec-head">
          <h2>订阅源状态</h2>
          <a class="link-sm" href="#/admin/news">管理${icon('right', 11)}</a>
        </div>
        ${broken || `<p class="muted-note">${srcList.length} 个源最近一次抓取都正常${
          pendingNews ? `；待采用 ${pendingNews} 条` : ''}。</p>`}
      </section>
    </div>`;

  return {
    title: '运营后台',
    html: adminShell('/admin', {
      title: '运营概览',
      desc: '今天需要处理的事情，以及内容流水线的状态。',
      body,
    }),
  };
}

/* -------------------------------------------------------- 待审内容 -- */

const REVIEW_TABS = [
  { key: 'article', label: '文章投稿' },
  { key: 'tool', label: '工具' },
  { key: 'resource', label: '资源' },
  { key: 'quiz', label: '题目' },
];

export async function adminReviewPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const tab = REVIEW_TABS.some((t) => t.key === ctx.params.get('tab'))
    ? ctx.params.get('tab') : 'article';
  const page = Math.max(1, Number(ctx.params.get('page')) || 1);
  const pageSize = 20;

  const tabsHTML = `<div class="seg admin-tabs">${REVIEW_TABS.map((t) => `
    <a href="#/admin/review?tab=${t.key}" class="${t.key === tab ? 'is-active' : ''}">${esc(t.label)}</a>`).join('')}</div>`;

  let body = '';
  let total = 0;

  if (tab === 'article') {
    const data = await api.adminArticles({ status: 'pending', page, pageSize });
    total = data.total || 0;
    body = articleReviewList(ctx, data.list || []);
  } else if (tab === 'tool') {
    const data = await api.adminTools({ status: 'pending', page, pageSize });
    total = data.total || 0;
    body = simpleReviewList(data.list || [], 'tool');
  } else if (tab === 'resource') {
    const data = await api.adminResources({ status: 'pending', page, pageSize });
    total = data.total || 0;
    body = simpleReviewList(data.list || [], 'resource');
  } else {
    const data = await api.adminQuestions({ status: 'pending', page, pageSize });
    total = data.total || 0;
    body = simpleReviewList(data.list || [], 'quiz');
  }

  const pager = pagerHTML({ page, pageSize, total });
  const html = adminShell('/admin/review', {
    title: '待审内容',
    desc: tab === 'article'
      ? '投稿与抓取进来的内容都在这里。通过即发布，驳回必须写清原因——作者要据此修改。'
      : '工具 / 资源 / 题目没有独立的审核意见，直接发布或删除。',
    actions: tabsHTML,
    body: `<div class="admin-list">${body}${pager ? `<div class="admin-pager">${pager}</div>` : ''}</div>`,
  });

  return {
    title: '待审内容',
    html,
    mount: (root) => bindReviewActions(root, tab),
  };
}

function articleReviewList(ctx, items) {
  if (!items.length) {
    return emptyHTML({ iconName: 'check', title: '没有待审的内容', desc: '投稿与抓取进来的内容会先落到这里。' });
  }
  return items.map((it) => `
    <article class="admin-row card">
      <div class="admin-row-main">
        <div class="admin-row-title">
          ${esc(it.title)}
          ${it.sourceName ? `<span class="chip chip-source">${icon('link', 11)}${esc(it.sourceName)}</span>` : '<span class="chip chip-brand">用户投稿</span>'}
        </div>
        <div class="admin-row-meta">
          <span>${esc(channelName(ctx, it.channel))}</span>
          <span>${esc(it.categoryName || '未分类')}</span>
          <span>提交于 ${when(it.createdAt)}</span>
          ${it.authorId ? `<span>作者 #${it.authorId}</span>` : ''}
        </div>
        ${it.summary ? `<p class="admin-row-summary">${esc(it.summary)}</p>` : ''}
      </div>
      <div class="admin-row-ops">
        <button class="btn btn-sm" type="button" data-view="${it.id}">看正文</button>
        <button class="btn btn-sm btn-primary" type="button" data-approve="${it.id}">通过并发布</button>
        <button class="btn btn-sm btn-danger" type="button" data-reject="${it.id}">驳回</button>
      </div>
    </article>`).join('');
}

/** 工具 / 资源 / 题目：没有审核意见字段，只能发布或删除。 */
function simpleReviewList(items, kind) {
  if (!items.length) {
    return emptyHTML({ iconName: 'check', title: '没有待审的内容' });
  }
  const titleOf = (it) => it.name || it.title || it.stem || `#${it.id}`;
  const metaOf = (it) => it.categoryName || (kind === 'quiz' ? QUIZ_TYPE_LABEL[it.type] || it.type : '');

  return items.map((it) => `
    <article class="admin-row card">
      <div class="admin-row-main">
        <div class="admin-row-title">${esc(titleOf(it))}</div>
        <div class="admin-row-meta">
          <span>${esc(metaOf(it) || '未分类')}</span>
          <span>#${it.id}</span>
          <span>${when(it.createdAt)}</span>
        </div>
      </div>
      <div class="admin-row-ops">
        <button class="btn btn-sm btn-primary" type="button" data-publish-kind="${kind}" data-id="${it.id}">发布</button>
        <button class="btn btn-sm btn-danger" type="button" data-delete-kind="${kind}" data-id="${it.id}">删除</button>
      </div>
    </article>`).join('');
}

function bindReviewActions(root, tab) {
  root.addEventListener('click', async (e) => {
    const btn = e.target.closest('button');
    if (!btn) return;

    const viewId = btn.getAttribute('data-view');
    if (viewId) {
      await showArticlePreview(viewId);
      return;
    }

    const approveId = btn.getAttribute('data-approve');
    if (approveId) {
      btn.disabled = true;
      try {
        await api.reviewArticle(approveId, { action: 'approve' });
        toastOk('已通过并发布');
        window.__reloadPage();
      } catch (err) {
        btn.disabled = false;
        toastError(err.message || '审核失败');
      }
      return;
    }

    const rejectId = btn.getAttribute('data-reject');
    if (rejectId) {
      openRejectModal(rejectId);
      return;
    }

    const pubKind = btn.getAttribute('data-publish-kind');
    if (pubKind) {
      btn.disabled = true;
      try {
        await publishByKind(pubKind, btn.getAttribute('data-id'));
        toastOk('已发布');
        window.__reloadPage();
      } catch (err) {
        btn.disabled = false;
        toastError(err.message || '发布失败');
      }
      return;
    }

    const delKind = btn.getAttribute('data-delete-kind');
    if (delKind && window.confirm('删除后不可恢复，确认删除这条内容？')) {
      btn.disabled = true;
      try {
        await deleteByKind(delKind, btn.getAttribute('data-id'));
        toastOk('已删除');
        window.__reloadPage();
      } catch (err) {
        btn.disabled = false;
        toastError(err.message || '删除失败');
      }
    }
  });
}

const PUBLISH_API = {
  tool: api.publishTool,
  resource: api.publishResource,
  quiz: api.publishQuestion,
};

const DELETE_API = {
  tool: api.deleteTool,
  resource: api.deleteResource,
  quiz: api.deleteQuestion,
};

const publishByKind = (kind, id) => PUBLISH_API[kind](id);
const deleteByKind = (kind, id) => DELETE_API[kind](id);

async function showArticlePreview(id) {
  const detail = await api.adminArticle(id);
  const content = String(detail.content || '');
  openModal(`
    <div class="modal-head">
      <div>
        <h2>${esc(detail.title)}</h2>
        <p>${esc((detail.seo && detail.seo.description) || detail.summary || '（无摘要）')}</p>
      </div>
    </div>
    <div class="modal-body">
      <pre class="admin-preview">${esc(content)}</pre>
      <p class="muted-note">正文按 Markdown 原文显示：审的是"他说了什么"，不是渲染后的样子。</p>
    </div>
  `, null, { className: 'is-wide' });
}

function openRejectModal(id) {
  openModal(`
    <div class="modal-head">
      <div>
        <h2>驳回这篇内容</h2>
        <p>原因会直接显示给作者。写清"缺什么"比写"质量不行"有用——否则他只能反复试。</p>
      </div>
    </div>
    <div class="modal-body">
      <div class="field">
        <label for="reject-note">驳回原因（必填）</label>
        <textarea class="input code-input" id="reject-note" rows="4"
          placeholder="例如：缺少复现步骤与版本号；命令没有说明执行环境"></textarea>
      </div>
      <div class="modal-actions">
        <button class="btn" type="button" data-modal-close>取消</button>
        <button class="btn btn-danger" type="button" data-do-reject>确认驳回</button>
      </div>
    </div>
  `, (modal) => {
    const btn = modal.querySelector('[data-do-reject]');
    btn.addEventListener('click', async () => {
      const note = modal.querySelector('#reject-note').value.trim();
      if (!note) {
        toastError('请填写驳回原因');
        return;
      }
      btn.disabled = true;
      try {
        await api.reviewArticle(id, { action: 'reject', note });
        closeModal();
        toastOk('已驳回并通知作者');
        window.__reloadPage();
      } catch (err) {
        btn.disabled = false;
        toastError(err.message || '操作失败');
      }
    });
  });
}

/* ------------------------------------------------------ 资讯订阅源 -- */

export async function adminNewsPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const sources = await api.newsSources();
  const pending = sources.reduce((max, s) => Math.max(max, Number(s.pendingTotal) || 0), 0);

  const rows = sources.map((s) => {
    const failed = s.lastStatus === 'failed';
    return `<tr data-source-row="${s.id}">
      <td>
        <div class="admin-cell-title">${esc(s.name)}</div>
        <div class="admin-cell-sub">${esc(s.url)}</div>
      </td>
      <td><span class="chip chip-mono">${esc(s.kind)}</span></td>
      <td>${s.autoPublish
        ? '<span class="chip chip-ok">自动发布</span>'
        : '<span class="chip">先进待审</span>'}</td>
      <td>${s.enabled ? '<span class="chip chip-info">启用</span>' : '<span class="chip">停用</span>'}</td>
      <td>
        ${s.lastFetchedAt ? esc(fmtRelative(s.lastFetchedAt)) : '<span class="muted-note">从未抓取</span>'}
        <div class="admin-cell-sub">${failed
          ? `<span class="is-danger">${esc(s.lastError || '抓取失败')}</span>`
          : (s.lastStatus === 'ok' ? '最近一次成功' : '')}</div>
      </td>
      <td class="admin-num">${fmtNum(s.fetchedCount)}</td>
      <td class="admin-ops">
        <button class="btn btn-sm" type="button" data-fetch="${s.id}">${icon('refresh', 13)}抓取</button>
        <button class="btn btn-sm" type="button" data-edit="${s.id}">编辑</button>
        <button class="btn btn-sm btn-danger" type="button" data-del="${s.id}">删除</button>
      </td>
    </tr>`;
  }).join('');

  const body = `
    <div class="admin-note-bar">
      <span>${icon('info', 14)}抓取默认进「待采用」池，由运营决定是否上线${
        pending ? `；当前待采用 <strong>${fmtNum(pending)}</strong> 条` : ''}。</span>
      <a class="link-sm" href="#/admin/articles?status=pending&channel=ops-news">去看待采用${icon('right', 11)}</a>
    </div>
    <div class="card admin-table-wrap">
      <table class="admin-table">
        <thead><tr>
          <th>名称 / 地址</th><th>类型</th><th>发布策略</th><th>状态</th>
          <th>上次抓取</th><th class="admin-num">累计</th><th class="admin-ops">操作</th>
        </tr></thead>
        <tbody>${rows || '<tr><td colspan="7"><p class="muted-note">还没有订阅源。</p></td></tr>'}</tbody>
      </table>
    </div>`;

  return {
    title: '资讯订阅源',
    html: adminShell('/admin/news', {
      title: '资讯订阅源',
      desc: '运维资讯靠订阅源自动抓取。RSS 用 feed 地址；GitHub 填 owner/repo，走 Releases API。',
      actions: '<button class="btn btn-primary btn-sm" type="button" data-new>新增订阅源</button>',
      body,
    }),
    mount: (root) => {
      const byId = new Map(sources.map((s) => [String(s.id), s]));
      root.querySelector('[data-new]')?.addEventListener('click', () => openSourceModal(null, ctx));
      root.addEventListener('click', async (e) => {
        const btn = e.target.closest('button');
        if (!btn) return;

        const editId = btn.getAttribute('data-edit');
        if (editId) {
          openSourceModal(byId.get(editId), ctx);
          return;
        }

        const fetchId = btn.getAttribute('data-fetch');
        if (fetchId) {
          btn.disabled = true;
          try {
            const res = await api.fetchNewsSource(fetchId);
            const item = Array.isArray(res) ? res[0] : res;
            if (item && item.error) toastError(`抓取失败：${item.error}`);
            else toastOk(`抓取完成，新写入 ${fmtNum((item && item.written) || 0)} 条`);
            window.__reloadPage();
          } catch (err) {
            btn.disabled = false;
            toastError(err.message || '抓取失败');
          }
          return;
        }

        const delId = btn.getAttribute('data-del');
        if (delId && window.confirm('删除订阅源不会删掉已经抓进来的内容，确认删除？')) {
          btn.disabled = true;
          try {
            await api.deleteNewsSource(delId);
            toastOk('已删除订阅源');
            window.__reloadPage();
          } catch (err) {
            btn.disabled = false;
            toastError(err.message || '删除失败');
          }
        }
      });
    },
  };
}

function openSourceModal(src, ctx) {
  const isNew = !src;
  const s = src || { name: '', url: '', kind: 'rss', autoPublish: false, enabled: true, sort: 0 };
  openModal(`
    <div class="modal-head">
      <div><h2>${isNew ? '新增订阅源' : '编辑订阅源'}</h2></div>
    </div>
    <div class="modal-body">
    <div class="admin-form">
      <div class="field">
        <label for="src-name">名称</label>
        <input class="input" id="src-name" value="${esc(s.name)}" placeholder="例如：Kubernetes Blog">
      </div>
      <div class="field">
        <label for="src-url">地址</label>
        <input class="input" id="src-url" value="${esc(s.url)}"
          placeholder="rss 填 https://…/feed.xml；github 填 owner/repo">
      </div>
      <div class="field-row">
        <div class="field">
          <label for="src-kind">类型</label>
          <select class="select" id="src-kind">
            <option value="rss" ${s.kind === 'rss' ? 'selected' : ''}>RSS / Atom</option>
            <option value="github" ${s.kind === 'github' ? 'selected' : ''}>GitHub Releases</option>
          </select>
        </div>
        <div class="field">
          <label for="src-sort">排序</label>
          <input class="input" id="src-sort" type="number" value="${Number(s.sort) || 0}">
        </div>
      </div>
      <label class="admin-check">
        <input type="checkbox" id="src-auto" ${s.autoPublish ? 'checked' : ''}>
        <span>抓回来的内容直接发布<em>（官方源可以开；聚合站不要开，软文要人工过一遍）</em></span>
      </label>
      <label class="admin-check">
        <input type="checkbox" id="src-enabled" ${s.enabled ? 'checked' : ''}>
        <span>启用抓取</span>
      </label>
    </div>
    <div class="modal-actions">
      <button class="btn" type="button" data-modal-close>取消</button>
      <button class="btn btn-primary" type="button" data-save>保存</button>
    </div>
    </div>
  `, (modal) => {
    modal.querySelector('[data-save]').addEventListener('click', async (e) => {
      const payload = {
        name: modal.querySelector('#src-name').value.trim(),
        url: modal.querySelector('#src-url').value.trim(),
        kind: modal.querySelector('#src-kind').value,
        autoPublish: modal.querySelector('#src-auto').checked,
        enabled: modal.querySelector('#src-enabled').checked,
        sort: Number(modal.querySelector('#src-sort').value) || 0,
      };
      if (!payload.name || !payload.url) {
        toastError('名称与地址都要填');
        return;
      }
      const btn = e.currentTarget;
      btn.disabled = true;
      try {
        if (isNew) await api.createNewsSource(payload);
        else await api.updateNewsSource(s.id, payload);
        closeModal();
        toastOk(isNew ? '已添加订阅源' : '已保存');
        window.__reloadPage();
      } catch (err) {
        btn.disabled = false;
        toastError(err.message || '保存失败');
      }
    });
  });
}

/* ------------------------------------------------------------ 文章 -- */

export async function adminArticlesPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const page = Math.max(1, Number(ctx.params.get('page')) || 1);
  const pageSize = 20;
  const status = ctx.params.get('status') || '';
  const channel = ctx.params.get('channel') || '';
  const keyword = ctx.params.get('keyword') || '';

  const [data, cats] = await Promise.all([
    api.adminArticles({ status, channel, keyword, page, pageSize }),
    api.adminCategories().catch(() => []),
  ]);

  const statusOpts = ['', 'pending', 'published', 'draft', 'rejected', 'offline'].map((v) => `
    <option value="${v}" ${v === status ? 'selected' : ''}>${v ? esc(STATUS_LABEL[v]) : '全部状态'}</option>`).join('');
  const channelOpts = [{ slug: '', name: '全部栏目' }, ...(ctx.state.channels || [])].map((c) => `
    <option value="${esc(c.slug)}" ${c.slug === channel ? 'selected' : ''}>${esc(c.name)}</option>`).join('');

  const catName = new Map((cats || []).map((c) => [String(c.id), c.name]));

  const rows = (data.list || []).map((it) => `<tr>
    <td>
      <a class="admin-cell-title" href="#/admin/articles/${it.id}">${esc(it.title)}</a>
      <div class="admin-cell-sub">${esc(it.slug)}</div>
    </td>
    <td>${esc(channelName(ctx, it.channel))}</td>
    <td>${esc(catName.get(String(it.categoryId)) || it.categoryName || '未分类')}</td>
    <td>${statusChip(it.status)}${it.reviewNote ? `<div class="admin-cell-sub is-danger">${esc(it.reviewNote)}</div>` : ''}</td>
    <td class="admin-num">${fmtNum(it.viewCount)}</td>
    <td>${it.publishedAt ? esc(fmtDate(it.publishedAt)) : '—'}</td>
    <td class="admin-ops">
      <a class="btn btn-sm" href="#/admin/articles/${it.id}">编辑</a>
      ${it.status === 'published'
        ? `<button class="btn btn-sm" type="button" data-offline="${it.id}">下线</button>`
        : `<button class="btn btn-sm btn-primary" type="button" data-publish="${it.id}">发布</button>`}
      <button class="btn btn-sm btn-danger" type="button" data-del="${it.id}">删除</button>
    </td>
  </tr>`).join('');

  const body = `
    <form class="toolbar admin-filters" data-filter>
      <select class="select" name="status">${statusOpts}</select>
      <select class="select" name="channel">${channelOpts}</select>
      <input class="input" name="keyword" value="${esc(keyword)}" placeholder="搜标题或正文" style="max-width:240px">
      <button class="btn btn-sm" type="submit">${icon('search', 14)}筛选</button>
      <span class="toolbar-spacer"></span>
      <a class="btn btn-sm btn-primary" href="#/admin/articles/new">${icon('plus', 14)}新建文章</a>
    </form>
    <div class="card admin-table-wrap">
      <table class="admin-table">
        <thead><tr>
          <th>标题</th><th>栏目</th><th>分类</th><th>状态</th>
          <th class="admin-num">浏览</th><th>发布时间</th><th class="admin-ops">操作</th>
        </tr></thead>
        <tbody>${rows || '<tr><td colspan="7"><p class="muted-note">没有匹配的内容。</p></td></tr>'}</tbody>
      </table>
    </div>
    <div class="admin-pager">${pagerHTML({ page, pageSize, total: data.total || 0 })}</div>`;

  return {
    title: '文章管理',
    html: adminShell('/admin/articles', {
      title: '文章管理',
      desc: '运维实战教程与运维资讯都在这张表里，用栏目区分。',
      body,
    }),
    mount: (root) => {
      bindFilter(root);
      bindPager(root, (p) => setHashQuery({ page: p > 1 ? String(p) : '' }, { keepPage: true }));
      root.addEventListener('click', async (e) => {
        const btn = e.target.closest('button');
        if (!btn) return;
        const pubId = btn.getAttribute('data-publish');
        const offId = btn.getAttribute('data-offline');
        const delId = btn.getAttribute('data-del');
        if (!pubId && !offId && !delId) return;

        if (delId && !window.confirm('删除后前台将不可见（软删除），确认？')) return;

        btn.disabled = true;
        try {
          if (pubId) await api.publishArticle(pubId);
          else if (offId) await api.offlineArticle(offId);
          else await api.deleteArticle(delId);
          toastOk('已更新');
          window.__reloadPage();
        } catch (err) {
          btn.disabled = false;
          toastError(err.message || '操作失败');
        }
      });
    },
  };
}

/** 筛选表单：写成 hash query，刷新与后退都能回到同一视图。 */
function bindFilter(root) {
  const form = root.querySelector('[data-filter]');
  if (!form) return;
  form.addEventListener('submit', (e) => {
    e.preventDefault();
    const fd = new FormData(form);
    setHashQuery({
      status: String(fd.get('status') || ''),
      channel: String(fd.get('channel') || ''),
      keyword: String(fd.get('keyword') || '').trim(),
    });
  });
}

export async function adminArticleEditPage(ctx, id) {
  const gate = adminGate();
  if (gate) return gate;

  const isNew = id === 'new';
  const [cats, detail] = await Promise.all([
    api.adminCategories().catch(() => []),
    isNew ? Promise.resolve(null) : api.adminArticle(id),
  ]);

  const a = detail || { title: '', channel: '', categoryId: 0, summary: '', content: '', tags: [], status: 'draft', seo: {} };
  const seo = a.seo || {};

  const channelOpts = (ctx.state.channels || [])
    .filter((c) => c.kind === 'article')
    .map((c) => `<option value="${esc(c.slug)}" ${c.slug === a.channel ? 'selected' : ''}>${esc(c.name)}</option>`).join('');
  const catOpts = (cats || []).map((c) => `
    <option value="${c.id}" ${Number(c.id) === Number(a.categoryId) ? 'selected' : ''}>${esc(c.name)}</option>`).join('');

  const body = `
    <form class="card card-pad admin-editor" data-editor>
      ${!isNew && a.status === 'published' ? `<div class="notice is-warn">
        <strong>这篇已经发布，保存后立即对访客生效</strong>
        <p>修改不会重新进审核队列，也不会改变发布时间；要临时藏起来请用下方「下线」，下线后仍可在这里改完再发布。</p>
      </div>` : ''}
      <div class="field">
        <label for="f-title">标题</label>
        <input class="input" id="f-title" value="${esc(a.title)}" placeholder="一句话说清这篇解决什么问题">
      </div>

      <div class="field-row">
        <div class="field">
          <label for="f-channel">栏目</label>
          <select class="select" id="f-channel">${channelOpts}</select>
        </div>
        <div class="field">
          <label for="f-category">分类</label>
          <select class="select" id="f-category">
            <option value="0" ${!a.categoryId ? 'selected' : ''}>未分类</option>${catOpts}
          </select>
        </div>
        <div class="field">
          <label for="f-tags">标签<em>（逗号分隔）</em></label>
          <input class="input" id="f-tags" value="${esc((a.tags || []).join(','))}" placeholder="nginx,502,keepalive">
        </div>
      </div>

      <div class="field">
        <label for="f-summary">摘要<em>（列表页与分享卡片用）</em></label>
        <input class="input" id="f-summary" value="${esc(a.summary)}">
      </div>

      <div class="field">
        <label for="f-content">正文<em>（Markdown）</em></label>
        <textarea class="input code-input" id="f-content" rows="18">${esc(a.content || '')}</textarea>
      </div>

      <details class="admin-seo">
        <summary>SEO 字段（可选）</summary>
        <div class="field"><label for="f-seo-title">SEO 标题</label>
          <input class="input" id="f-seo-title" value="${esc(seo.title || '')}"></div>
        <div class="field"><label for="f-seo-desc">SEO 描述</label>
          <input class="input" id="f-seo-desc" value="${esc(seo.description || '')}"></div>
        <div class="field"><label for="f-seo-kw">SEO 关键词</label>
          <input class="input" id="f-seo-kw" value="${esc(seo.keywords || '')}"></div>
      </details>

      <div class="admin-editor-foot">
        <a class="btn" href="#/admin/articles">返回列表</a>
        <span class="toolbar-spacer"></span>
        ${!isNew && a.status === 'published'
          ? `<button class="btn" type="button" data-offline>下线</button>` : ''}
        <button class="btn btn-primary" type="button" data-save>${isNew ? '创建' : '保存'}</button>
      </div>
    </form>`;

  return {
    title: isNew ? '新建文章' : '编辑文章',
    html: adminShell('/admin/articles', {
      title: isNew ? '新建文章' : '编辑文章',
      desc: isNew ? '创建后默认是草稿，确认无误再发布。' : `#${a.id} · ${a.slug || ''}`,
      body,
    }),
    mount: (root) => {
      const form = root.querySelector('[data-editor]');

      // 后台改正文是最高频的编辑场景，同样要能贴图。
      enableMarkdownImages(root, form.querySelector('#f-content'));

      form.querySelector('[data-save]').addEventListener('click', async (e) => {
        const payload = {
          title: form.querySelector('#f-title').value.trim(),
          channel: form.querySelector('#f-channel').value,
          categoryId: Number(form.querySelector('#f-category').value) || 0,
          summary: form.querySelector('#f-summary').value.trim(),
          content: form.querySelector('#f-content').value,
          tags: form.querySelector('#f-tags').value.split(',').map((s) => s.trim()).filter(Boolean),
          seoTitle: form.querySelector('#f-seo-title').value.trim(),
          seoDescription: form.querySelector('#f-seo-desc').value.trim(),
          seoKeywords: form.querySelector('#f-seo-kw').value.trim(),
        };
        if (!payload.title) {
          toastError('标题不能为空');
          return;
        }
        const btn = e.currentTarget;
        btn.disabled = true;
        try {
          if (isNew) {
            await api.createArticle({ ...payload, status: 'draft' });
            toastOk('已创建草稿');
            location.hash = '#/admin/articles';
          } else {
            await api.updateArticle(a.id, payload);
            toastOk('已保存');
            window.__reloadPage();
          }
        } catch (err) {
          btn.disabled = false;
          toastError(err.message || '保存失败');
        }
      });

      const off = form.querySelector('[data-offline]');
      if (off) {
        off.addEventListener('click', async () => {
          off.disabled = true;
          try {
            await api.offlineArticle(a.id);
            toastOk('已下线');
            window.__reloadPage();
          } catch (err) {
            off.disabled = false;
            toastError(err.message || '下线失败');
          }
        });
      }
    },
  };
}

/* ------------------------------------------------------ 工具四类教程 -- */

export async function adminToolsPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const page = Math.max(1, Number(ctx.params.get('page')) || 1);
  const pageSize = 20;
  const status = ctx.params.get('status') || '';
  const keyword = ctx.params.get('keyword') || '';

  const data = await api.adminTools({ status, keyword, page, pageSize });
  const statusOpts = ['', 'pending', 'published', 'draft', 'offline'].map((v) => `
    <option value="${v}" ${v === status ? 'selected' : ''}>${v ? esc(STATUS_LABEL[v]) : '全部状态'}</option>`).join('');

  const rows = (data.list || []).map((it) => `<tr>
    <td>
      <div class="admin-cell-title">${esc(it.name)}</div>
      <div class="admin-cell-sub">${esc(it.slug)} · ${esc(it.language || '')} ${it.license ? '/ ' + esc(it.license) : ''}</div>
    </td>
    <td>${esc(it.categoryName || '未分类')}</td>
    <td>${it.featured ? '<span class="chip chip-brand">推荐</span>' : ''}</td>
    <td>${statusChip(it.status)}</td>
    <td class="admin-num">${fmtNum(it.viewCount)}</td>
    <td class="admin-ops">
      <a class="btn btn-sm" href="#/admin/tools/${it.id}/guides">维护教程</a>
      <a class="btn btn-sm" href="#/admin/tools/${it.id}/books">系统教程</a>
      ${it.status === 'published'
        ? `<button class="btn btn-sm" type="button" data-offline="${it.id}">下线</button>`
        : `<button class="btn btn-sm btn-primary" type="button" data-publish="${it.id}">发布</button>`}
      <button class="btn btn-sm btn-danger" type="button" data-del="${it.id}">删除</button>
    </td>
  </tr>`).join('');

  const body = `
    <form class="toolbar admin-filters" data-filter>
      <select class="select" name="status">${statusOpts}</select>
      <input class="input" name="keyword" value="${esc(keyword)}" placeholder="搜工具名" style="max-width:240px">
      <button class="btn btn-sm" type="submit">${icon('search', 14)}筛选</button>
    </form>
    <div class="card admin-table-wrap">
      <table class="admin-table">
        <thead><tr><th>工具</th><th>分类</th><th>推荐</th><th>状态</th><th class="admin-num">浏览</th><th class="admin-ops">操作</th></tr></thead>
        <tbody>${rows || '<tr><td colspan="6"><p class="muted-note">没有匹配的工具。</p></td></tr>'}</tbody>
      </table>
    </div>
    <div class="admin-pager">${pagerHTML({ page, pageSize, total: data.total || 0 })}</div>`;

  return {
    title: '工具教程',
    html: adminShell('/admin/tools', {
      title: '开源工具',
      desc: '每个工具可挂部署 / 实施 / 优化 / 集群构建四类教程，站内文章或外部文档都行。',
      body,
    }),
    mount: (root) => {
      bindFilter(root);
      bindPager(root, (p) => setHashQuery({ page: p > 1 ? String(p) : '' }, { keepPage: true }));
      bindStandardRowActions(root, {
        publish: api.publishTool,
        offline: api.offlineTool,
        remove: api.deleteTool,
      });
    },
  };
}

/** 工具 / 资源列表里的三个动作完全同构，抽一份，避免两处各写一遍再各漏一处。 */
function bindStandardRowActions(root, fns) {
  root.addEventListener('click', async (e) => {
    const btn = e.target.closest('button');
    if (!btn) return;
    const pubId = btn.getAttribute('data-publish');
    const offId = btn.getAttribute('data-offline');
    const delId = btn.getAttribute('data-del');
    if (!pubId && !offId && !delId) return;
    if (delId && !window.confirm('删除后前台将不可见，确认删除？')) return;

    btn.disabled = true;
    try {
      if (pubId) await fns.publish(pubId);
      else if (offId) await fns.offline(offId);
      else await fns.remove(delId);
      toastOk('已更新');
      window.__reloadPage();
    } catch (err) {
      btn.disabled = false;
      toastError(err.message || '操作失败');
    }
  });
}

export async function adminToolGuidesPage(ctx, id) {
  const gate = adminGate();
  if (gate) return gate;

  const [tool, guides, articles] = await Promise.all([
    api.adminTool(id),
    api.toolGuides(id),
    // 站内文章下拉：只取已发布的。挂一篇草稿等于给读者一个 404 的入口。
    api.articles({ pageSize: 100, page: 1 }).catch(() => ({ list: [] })),
  ]);

  // 传数组而不是拼好的 option 字符串：需要给"当前这条挂的文章"加 selected，
  // 在字符串上做替换既脆又难读（换个引号就失效）。
  const articleList = (articles.list || []).map((a) => ({ id: a.id, title: a.title }));

  const sections = [
    { kind: 'deploy', label: '部署教程', hint: '从零装起来' },
    { kind: 'implement', label: '实施教程', hint: '接到真实业务上' },
    { kind: 'optimize', label: '优化教程', hint: '性能与成本' },
    { kind: 'cluster', label: '集群构建', hint: '高可用与扩容' },
  ].map((sec) => {
    const rows = (guides || []).filter((g) => g.kind === sec.kind)
      .map((g) => guideRowHTML(g, articleList)).join('');
    return `<section class="admin-guide-sec card">
      <header class="admin-guide-head">
        <h3>${esc(sec.label)}<em>${esc(sec.hint)}</em></h3>
        <button class="btn btn-sm" type="button" data-add="${sec.kind}">${icon('plus', 13)}添加</button>
      </header>
      <div class="admin-guide-rows" data-kind="${sec.kind}">${rows}</div>
    </section>`;
  }).join('');

  const body = `
    <div class="admin-note-bar">
      <span>${icon('info', 14)}整组保存：页面上改完一次性提交，服务端在同一个事务里替换，不会出现"改了一半"。</span>
    </div>
    <div class="admin-guides" data-guides data-tool="${esc(id)}">
      ${sections}
    </div>
    <div class="admin-editor-foot">
      <a class="btn" href="#/admin/tools">返回工具列表</a>
      <span class="toolbar-spacer"></span>
      <button class="btn btn-primary" type="button" data-save-guides>保存全部教程</button>
    </div>`;

  return {
    title: `${tool.name} · 教程维护`,
    html: adminShell('/admin/tools', {
      title: `${tool.name} · 四类教程`,
      desc: '每条教程要么选一篇站内文章，要么填一个外部链接。两者只能取其一。',
      body,
    }),
    mount: (root) => {
      const wrap = root.querySelector('[data-guides]');

      wrap.addEventListener('click', (e) => {
        const add = e.target.closest('[data-add]');
        if (add) {
          const kind = add.getAttribute('data-add');
          wrap.querySelector(`[data-kind="${kind}"]`)
            .insertAdjacentHTML('beforeend', guideRowHTML(null, articleList));
          return;
        }
        const del = e.target.closest('[data-remove-row]');
        if (del) del.closest('.admin-guide-row').remove();
      });

      // 站内 / 外链二选一：切换时把另一侧的输入清空，
      // 否则用户先填了 URL 再改回站内，提交上去会出现"文章和链接都有"，
      // 服务端只能靠约定丢弃其中一个——那是最难排查的一类问题。
      wrap.addEventListener('change', (e) => {
        const sel = e.target.closest('[data-mode]');
        if (!sel) return;
        const row = sel.closest('.admin-guide-row');
        const isArticle = sel.value === 'article';
        row.querySelector('[data-f-article]').hidden = !isArticle;
        row.querySelector('[data-f-url]').hidden = isArticle;
        if (isArticle) row.querySelector('[data-url-input]').value = '';
        else row.querySelector('[data-article-select]').selectedIndex = 0;
      });

      root.querySelector('[data-save-guides]').addEventListener('click', async (e) => {
        const payload = [];
        wrap.querySelectorAll('.admin-guide-row').forEach((row, i) => {
          const kind = row.closest('[data-kind]').getAttribute('data-kind');
          const title = row.querySelector('[data-f-title]').value.trim();
          const summary = row.querySelector('[data-f-summary]').value.trim();
          const url = row.querySelector('[data-url-input]').value.trim();
          const articleId = Number(row.querySelector('[data-article-select]').value) || 0;
          if (!url && !articleId) return; // 空行直接忽略，别让"多加了一行没填"变成保存失败
          payload.push({
            kind,
            title,
            summary,
            url,
            articleId: url ? 0 : articleId,
            sort: (i + 1) * 10,
          });
        });

        const btn = e.currentTarget;
        btn.disabled = true;
        try {
          await api.setToolGuides(id, payload);
          toastOk(`已保存 ${payload.length} 条教程`);
          window.__reloadPage();
        } catch (err) {
          btn.disabled = false;
          toastError(err.message || '保存失败');
        }
      });
    },
  };
}

function guideRowHTML(g, articles) {
  const articleId = g && g.articleId ? Number(g.articleId) : 0;
  // 只有「没填链接」才算站内文章：历史数据里两者可能都有，以 URL 为空作为判据，
  // 与后端 SetGuides 的校验口径保持一致。
  const mode = g && g.url ? 'url' : 'article';

  const opts = ['<option value="0">（选择一篇已发布文章）</option>']
    .concat((articles || []).map((a) =>
      `<option value="${a.id}" ${a.id === articleId ? 'selected' : ''}>${esc(a.title)}</option>`))
    .join('');

  return `<div class="admin-guide-row">
    <div class="field">
      <label>类型</label>
      <select class="select" data-mode>
        <option value="article" ${mode === 'article' ? 'selected' : ''}>站内文章</option>
        <option value="url" ${mode === 'url' ? 'selected' : ''}>外部链接</option>
      </select>
    </div>
    <div class="field" data-f-article ${mode === 'article' ? '' : 'hidden'}>
      <label>文章</label>
      <select class="select" data-article-select>${opts}</select>
    </div>
    <div class="field" data-f-url ${mode === 'url' ? '' : 'hidden'}>
      <label>链接</label>
      <input class="input" data-url-input value="${esc(g && g.url ? g.url : '')}" placeholder="https://…">
    </div>
    <div class="field">
      <label>标题<em>（可空，留空用文章标题）</em></label>
      <input class="input" data-f-title value="${esc(g && g.title ? g.title : '')}">
    </div>
    <div class="field">
      <label>说明</label>
      <input class="input" data-f-summary value="${esc(g && g.summary ? g.summary : '')}">
    </div>
    <button class="icon-btn admin-row-del" type="button" data-remove-row title="删除这条">${icon('trash', 15)}</button>
  </div>`;
}

/* -------------------------------------------------------- 系统教程集 -- */

/**
 * adminToolBooksPage 一个工具下的教程集管理。
 *
 * 一个页面两种视图，用 `?book=<id>` 切换：
 *   不带参数 —— 这个工具下的书列表；
 *   带 book  —— 那本书的编辑视图（信息表单 + 大纲编辑 + 目录预览）。
 *
 * 为什么不拆成两个路由：两页共享「工具」这个上下文（面包屑、返回、
 * 新建时的 toolID），拆开后每次都得重新取一次工具，还要在两个地方
 * 各写一遍"这本书属于哪个工具"的校验。
 */
export async function adminToolBooksPage(ctx, toolId) {
  const gate = adminGate();
  if (gate) return gate;

  const bookId = Number(ctx.params.get('book')) || 0;
  if (bookId) return adminBookEditPage(ctx, toolId, bookId);

  const [tool, books] = await Promise.all([api.adminTool(toolId), api.adminToolBooks(toolId)]);
  const list = books || [];
  const base = `#/admin/tools/${encodeURIComponent(toolId)}/books`;

  const rows = list.map((b) => `<tr>
    <td>
      <div class="admin-cell-title">${esc(b.title)}</div>
      <div class="admin-cell-sub">${esc(b.summary || '未写简介')}</div>
    </td>
    <td>${esc(BOOK_LEVEL_LABEL[b.level] || b.level || '—')}</td>
    <td class="admin-num">${fmtNum(b.chapterCount)}</td>
    <td>${statusChip(b.status)}</td>
    <td class="admin-ops">
      <a class="btn btn-sm" href="${base}?book=${b.id}">编辑目录</a>
      ${b.status === 'published'
        ? `<button class="btn btn-sm" type="button" data-book-offline="${b.id}">下线</button>`
        : `<button class="btn btn-sm btn-primary" type="button" data-book-publish="${b.id}">发布</button>`}
      <button class="btn btn-sm btn-danger" type="button" data-book-del="${b.id}">删除</button>
    </td>
  </tr>`).join('');

  const body = `
    <div class="admin-note-bar">
      <span>${icon('info', 14)}教程集是「按顺序读的学习路径」，与上面的四类教程并存：
        那边是散装好文推荐，这里是成套的目录。</span>
    </div>
    <div class="card admin-table-wrap">
      <table class="admin-table">
        <thead><tr><th>教程集</th><th>级别</th><th class="admin-num">章节</th><th>状态</th><th class="admin-ops">操作</th></tr></thead>
        <tbody>${rows || '<tr><td colspan="5"><p class="muted-note">这个工具还没有教程集。新建一本，把大纲粘贴进去即可。</p></td></tr>'}</tbody>
      </table>
    </div>
    <div class="admin-editor-foot">
      <a class="btn" href="#/admin/tools">返回工具列表</a>
      <span class="toolbar-spacer"></span>
      <button class="btn btn-primary" type="button" data-book-new>${icon('plus', 14)}新建教程集</button>
    </div>`;

  return {
    title: `${tool.name} · 系统教程集`,
    html: adminShell('/admin/tools', {
      title: `${tool.name} · 系统教程集`,
      desc: '一本书是一串有先后顺序的章节。目录用大纲文本整体维护，正文复用已发布文章。',
      body,
    }),
    mount: (root) => {
      root.querySelector('[data-book-new]')?.addEventListener('click', () => openBookCreateModal(toolId));

      root.addEventListener('click', async (ev) => {
        const btn = ev.target.closest('button');
        if (!btn) return;
        const pub = btn.getAttribute('data-book-publish');
        const off = btn.getAttribute('data-book-offline');
        const del = btn.getAttribute('data-book-del');
        if (!pub && !off && !del) return;
        if (del && !window.confirm('删除后目录一并清除，且无法恢复。确认删除？')) return;

        btn.disabled = true;
        try {
          if (pub) await api.publishToolBook(pub);
          else if (off) await api.offlineToolBook(off);
          else await api.deleteToolBook(del);
          toastOk('已更新');
          window.__reloadPage();
        } catch (err) {
          btn.disabled = false;
          toastError(err instanceof ApiError ? err.message : '操作失败');
        }
      });
    },
  };
}

/** 新建教程集：只填基本信息，目录进去再粘。 */
function openBookCreateModal(toolId) {
  openModal(`
    <div class="modal-head"><h2>新建教程集</h2></div>
    <div class="modal-body">
      <form class="admin-form" data-book-form>
        <div class="field">
          <label for="b-title">标题 <span class="hint">例如「Nginx 从装到集群」</span></label>
          <input class="input" id="b-title" name="title" required maxlength="120" />
        </div>
        <div class="field">
          <label for="b-summary">简介</label>
          <textarea class="input" id="b-summary" name="summary" rows="2" maxlength="300"
            placeholder="一句话说清读完能得到什么"></textarea>
        </div>
        <div class="form-row">
          <div class="field">
            <label for="b-level">级别</label>
            <select class="select" id="b-level" name="level">
              <option value="beginner">入门</option>
              <option value="advanced">进阶</option>
              <option value="expert">深入</option>
            </select>
          </div>
          <div class="field">
            <label for="b-sort">排序 <span class="hint">小的在前</span></label>
            <input class="input" id="b-sort" name="sort" type="number" value="0" />
          </div>
        </div>
        <p class="muted-note">新建后是草稿状态，填完目录再发布。空目录不允许发布。</p>
        <div class="modal-actions">
          <button class="btn" type="button" data-modal-close>取消</button>
          <button class="btn btn-primary" type="submit">创建并编辑目录</button>
        </div>
      </form>
    </div>
  `, (modal) => {
    const form = modal.querySelector('[data-book-form]');
    form.addEventListener('submit', async (ev) => {
      ev.preventDefault();
      const submit = form.querySelector('button[type="submit"]');
      submit.disabled = true;
      try {
        const created = await api.createToolBook(toolId, {
          title: form.elements.title.value.trim(),
          summary: form.elements.summary.value.trim(),
          level: form.elements.level.value,
          sort: Number(form.elements.sort.value) || 0,
        });
        closeModal();
        toastOk('已创建，接着把大纲粘进来');
        location.hash = `#/admin/tools/${encodeURIComponent(toolId)}/books?book=${created.id}`;
      } catch (err) {
        submit.disabled = false;
        toastError(err instanceof ApiError ? err.message : '创建失败');
      }
    });
  });
}

/** 单本教程集的编辑视图。 */
async function adminBookEditPage(ctx, toolId, bookId) {
  const [tool, book, doc] = await Promise.all([
    api.adminTool(toolId),
    api.adminToolBook(bookId),
    api.toolBookOutline(bookId),
  ]);
  const base = `#/admin/tools/${encodeURIComponent(toolId)}/books`;

  const body = `
    <div class="detail-layout" style="margin-top:0">
      <div>
        <section class="card card-pad" style="margin-bottom:16px">
          <h2 style="font-size:15px;margin:0 0 12px">目录大纲</h2>
          <div class="field">
            <label for="bk-outline">格式：<code># 章</code> 与 <code>## 节 | 文章slug或外链 | 导语</code></label>
            <textarea class="input code-input outline-input" id="bk-outline"
              placeholder="# 第一章 安装与编译&#10;## 1.1 源码编译 | nginx-compile | 从 configure 到 make&#10;## 1.2 目录结构 | nginx-dirs&#10;# 第二章 反向代理&#10;## 2.1 upstream | https://nginx.org/en/docs/http/ngx_http_upstream_module.html">${esc(doc.outline || '')}</textarea>
            <p class="muted-note">
              整段粘贴即可，保存时整体替换——不会出现「删了章、它的节还挂着」的中间态。
              第二列写站内文章的 slug，或 http(s) 外链；找不到的 slug 会保留标题去掉链接并给出提示。
            </p>
          </div>
          <div id="bk-warnings"></div>
          <div class="admin-editor-foot" style="margin-top:14px;padding-top:0;border-top:0">
            <span class="muted-note">当前 ${fmtNum(doc.count)} 个节点（章 + 节）</span>
            <span class="toolbar-spacer"></span>
            <button class="btn btn-primary" type="button" data-save-outline>保存目录</button>
          </div>
        </section>

        <section class="card card-pad">
          <h2 style="font-size:15px;margin:0 0 12px">目录预览</h2>
          <div class="book-toc" data-book-tree>${adminBookTreeHTML(book.chapters)}</div>
        </section>
      </div>

      <aside>
        <div class="card side-card" style="margin-bottom:16px">
          <h3>基本信息</h3>
          <form class="admin-form" data-book-info>
            <div class="field">
              <label for="bi-title">标题</label>
              <input class="input" id="bi-title" name="title" value="${esc(book.title)}" maxlength="120" />
            </div>
            <div class="field">
              <label for="bi-summary">简介</label>
              <textarea class="input" id="bi-summary" name="summary" rows="3" maxlength="300">${esc(book.summary || '')}</textarea>
            </div>
            <div class="field">
              <label for="bi-cover">封面图 URL</label>
              <input class="input" id="bi-cover" name="cover" value="${esc(book.cover || '')}" placeholder="可空">
            </div>
            <div class="field">
              <label for="bi-level">级别</label>
              <select class="select" id="bi-level" name="level">
                ${['beginner', 'advanced', 'expert'].map((v) =>
                  `<option value="${v}" ${book.level === v ? 'selected' : ''}>${BOOK_LEVEL_LABEL[v]}</option>`).join('')}
              </select>
            </div>
            <button class="btn" type="submit" data-save-info>保存信息</button>
          </form>
        </div>

        <div class="card side-card">
          <h3>发布</h3>
          <p class="muted-note" style="margin-bottom:10px">
            当前状态：${esc(STATUS_LABEL[book.status] || book.status)}。草稿与已下线的书在前台 404。
          </p>
          ${book.status === 'published'
            ? '<button class="btn btn-block" type="button" data-book-state="offline">下线</button>'
            : '<button class="btn btn-primary btn-block" type="button" data-book-state="publish">发布</button>'}
        </div>
      </aside>
    </div>

    <div class="admin-editor-foot">
      <a class="btn" href="${base}">返回教程集列表</a>
      <a class="btn" href="#/tools/${encodeURIComponent(tool.slug)}/book/${bookId}" target="_blank" rel="noopener">前台预览</a>
      <span class="toolbar-spacer"></span>
      <button class="btn btn-danger" type="button" data-book-delete>删除这本书</button>
    </div>`;

  return {
    title: `${tool.name} · ${book.title}`,
    html: adminShell('/admin/tools', {
      title: `${book.title}`,
      desc: `属于工具「${tool.name}」。目录整体替换，正文仍在文章库里维护。`,
      body,
    }),
    mount: (root) => {
      root.querySelector('[data-save-outline]').addEventListener('click', async (ev) => {
        const btn = ev.currentTarget;
        btn.disabled = true;
        btn.textContent = '保存中…';
        try {
          const res = await api.saveToolBookOutline(bookId, root.querySelector('#bk-outline').value);
          // 警告只提示不阻断：某篇文章被下线了不该让整份目录保存失败，
          // 但运营必须看到"这条现在没正文"。
          root.querySelector('#bk-warnings').innerHTML = renderOutlineWarnings(res.warnings);
          root.querySelector('[data-book-tree]').innerHTML = adminBookTreeHTML(res.chapters);
          btn.textContent = `已保存 ${fmtNum(res.count)} 个节点`;
        } catch (err) {
          btn.disabled = false;
          btn.textContent = '保存目录';
          toastError(err instanceof ApiError ? err.message : '保存失败');
        }
      });

      root.querySelector('[data-book-info]').addEventListener('submit', async (ev) => {
        ev.preventDefault();
        const form = ev.currentTarget;
        const btn = form.querySelector('[data-save-info]');
        btn.disabled = true;
        try {
          await api.updateToolBook(bookId, {
            title: form.elements.title.value.trim(),
            summary: form.elements.summary.value.trim(),
            cover: form.elements.cover.value.trim(),
            level: form.elements.level.value,
          });
          toastOk('已保存');
          window.__reloadPage();
        } catch (err) {
          btn.disabled = false;
          toastError(err instanceof ApiError ? err.message : '保存失败');
        }
      });

      root.querySelector('[data-book-state]')?.addEventListener('click', async (ev) => {
        const btn = ev.currentTarget;
        const act = btn.getAttribute('data-book-state');
        btn.disabled = true;
        try {
          if (act === 'publish') await api.publishToolBook(bookId);
          else await api.offlineToolBook(bookId);
          toastOk(act === 'publish' ? '已发布' : '已下线');
          window.__reloadPage();
        } catch (err) {
          btn.disabled = false;
          // 空目录发布会被后端拒掉，这里把原因原样告诉运营，
          // 而不是让他对着一个没反应的按钮猜。
          toastError(err instanceof ApiError ? err.message : '操作失败');
        }
      });

      root.querySelector('[data-book-delete]').addEventListener('click', async (ev) => {
        if (!window.confirm('删除后目录一并清除，且无法恢复。确认删除？')) return;
        const btn = ev.currentTarget;
        btn.disabled = true;
        try {
          await api.deleteToolBook(bookId);
          toastOk('已删除');
          location.hash = base;
        } catch (err) {
          btn.disabled = false;
          toastError(err instanceof ApiError ? err.message : '删除失败');
        }
      });
    },
  };
}

// adminBookTreeHTML 后台的目录预览：与前台不同，**失效节点保留并标红**。
// 前台挂一个 404 链接比不展示更伤，所以那边直接丢弃；但后台看不到就改不了，
// 所以这里必须摆出来，用 is-broken 标出来。
function adminBookTreeHTML(chapters) {
  const list = Array.isArray(chapters) ? chapters : [];
  if (!list.length) {
    return '<p class="muted-note">目录还是空的。把大纲粘贴到上面保存，这里会显示结构。</p>';
  }
  return list.map((ch, i) => {
    const subs = Array.isArray(ch.children) ? ch.children : [];
    return `<div class="book-chapter" style="margin-bottom:6px">
      <div class="book-chapter-head" style="display:flex;align-items:center;gap:10px;padding:10px 12px">
        <span class="book-chapter-no">${i + 1}</span>
        <span class="book-chapter-title">${esc(ch.title)}</span>
        ${subs.length ? `<span class="book-chapter-count">${subs.length} 节</span>` : ''}
      </div>
      ${subs.length ? `<ul class="book-sections" style="padding-left:46px">
        ${subs.map((s, j) => `<li class="book-sec${s.broken ? ' is-broken' : ''}">
          <div style="display:flex;align-items:baseline;gap:8px">
            <span class="book-sec-no">${i + 1}.${j + 1}</span>
            <span>${esc(s.title)}</span>
            ${s.broken ? '<span class="chip chip-danger">正文失效</span>' : ''}
          </div>
          ${s.summary ? `<span class="book-sec-summary">${esc(s.summary)}</span>` : ''}
        </li>`).join('')}
      </ul>` : ''}
    </div>`;
  }).join('');
}

function renderOutlineWarnings(warnings) {
  const list = Array.isArray(warnings) ? warnings : [];
  if (!list.length) return '';
  return `<div class="outline-warn">
    <strong>${list.length} 条需要留意（不影响保存）</strong>
    <ul>${list.map((w) => `<li>${esc(w)}</li>`).join('')}</ul>
  </div>`;
}

/* -------------------------------------------------------- 资源外链 -- */

export async function adminResourcesPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const page = Math.max(1, Number(ctx.params.get('page')) || 1);
  const pageSize = 20;
  const status = ctx.params.get('status') || '';
  const keyword = ctx.params.get('keyword') || '';

  const data = await api.adminResources({ status, keyword, page, pageSize });
  const statusOpts = ['', 'pending', 'published', 'draft', 'offline'].map((v) => `
    <option value="${v}" ${v === status ? 'selected' : ''}>${v ? esc(STATUS_LABEL[v]) : '全部状态'}</option>`).join('');

  const rows = (data.list || []).map((it) => `<tr>
    <td>
      <div class="admin-cell-title">${esc(it.title)}</div>
      <div class="admin-cell-sub">${esc(it.slug)}</div>
    </td>
    <td>${esc(it.categoryName || '未分类')}</td>
    <td>${it.hasFile
      ? '<span class="chip chip-ok">平台托管</span>'
      : '<span class="chip chip-info">只给外链</span>'}</td>
    <td>${statusChip(it.status)}</td>
    <td class="admin-num">${fmtNum(it.downloadCount)}</td>
    <td class="admin-ops">
      <a class="btn btn-sm" href="#/admin/resources/${it.id}/links">维护外链</a>
      ${it.status === 'published'
        ? `<button class="btn btn-sm" type="button" data-offline="${it.id}">下线</button>`
        : `<button class="btn btn-sm btn-primary" type="button" data-publish="${it.id}">发布</button>`}
      <button class="btn btn-sm btn-danger" type="button" data-del="${it.id}">删除</button>
    </td>
  </tr>`).join('');

  const body = `
    <form class="toolbar admin-filters" data-filter>
      <select class="select" name="status">${statusOpts}</select>
      <input class="input" name="keyword" value="${esc(keyword)}" placeholder="搜资源名" style="max-width:240px">
      <button class="btn btn-sm" type="submit">${icon('search', 14)}筛选</button>
    </form>
    <div class="card admin-table-wrap">
      <table class="admin-table">
        <thead><tr><th>资源</th><th>分类</th><th>形态</th><th>状态</th><th class="admin-num">下载</th><th class="admin-ops">操作</th></tr></thead>
        <tbody>${rows || '<tr><td colspan="6"><p class="muted-note">没有匹配的资源。</p></td></tr>'}</tbody>
      </table>
    </div>
    <div class="admin-pager">${pagerHTML({ page, pageSize, total: data.total || 0 })}</div>`;

  return {
    title: '资源外链',
    html: adminShell('/admin/resources', {
      title: '资源下载',
      desc: '安装包不自托管的，就只配下载入口：官网、镜像、包管理器、代码仓库、容器镜像。',
      body,
    }),
    mount: (root) => {
      bindFilter(root);
      bindPager(root, (p) => setHashQuery({ page: p > 1 ? String(p) : '' }, { keepPage: true }));
      bindStandardRowActions(root, {
        publish: api.publishResource,
        offline: api.offlineResource,
        remove: api.deleteResource,
      });
    },
  };
}

const LINK_KINDS = [
  { key: 'official', label: '官网' },
  { key: 'release', label: '发布页' },
  { key: 'mirror', label: '镜像' },
  { key: 'package', label: '包管理器' },
  { key: 'repo', label: '代码仓库' },
  { key: 'image', label: '容器镜像' },
  { key: 'docs', label: '文档' },
  { key: 'other', label: '其他' },
];

export async function adminResourceLinksPage(ctx, id) {
  const gate = adminGate();
  if (gate) return gate;

  const [res, links] = await Promise.all([api.adminResource(id), api.resourceLinks(id)]);

  const rows = (links || []).map((l) => linkRowHTML(l)).join('');
  const body = `
    <div class="admin-note-bar">
      <span>${icon('info', 14)}最多 8 条。不认识的类型会归到「其他」，不会因此保存失败。</span>
    </div>
    <div class="admin-guides" data-links data-res="${esc(id)}">
      <section class="admin-guide-sec card">
        <header class="admin-guide-head">
          <h3>下载入口<em>${res.hasFile ? '这份资源同时有托管文件' : '这份资源只有外链'}</em></h3>
          <button class="btn btn-sm" type="button" data-add-link>${icon('plus', 13)}添加</button>
        </header>
        <div class="admin-guide-rows" data-link-rows>${rows}</div>
      </section>
    </div>
    <div class="admin-editor-foot">
      <a class="btn" href="#/admin/resources">返回资源列表</a>
      <span class="toolbar-spacer"></span>
      <button class="btn btn-primary" type="button" data-save-links>保存外链</button>
    </div>`;

  return {
    title: `${res.title} · 外链维护`,
    html: adminShell('/admin/resources', {
      title: `${res.title} · 下载入口`,
      desc: '用户点的是这些地址。填之前先自己点一遍——链接 404 比没有链接更让人恼火。',
      body,
    }),
    mount: (root) => {
      const wrap = root.querySelector('[data-links]');

      wrap.addEventListener('click', (e) => {
        if (e.target.closest('[data-add-link]')) {
          wrap.querySelector('[data-link-rows]').insertAdjacentHTML('beforeend', linkRowHTML(null));
          return;
        }
        const del = e.target.closest('[data-remove-row]');
        if (del) del.closest('.admin-guide-row').remove();
      });

      root.querySelector('[data-save-links]').addEventListener('click', async (e) => {
        const payload = [];
        wrap.querySelectorAll('.admin-guide-row').forEach((row, i) => {
          const label = row.querySelector('[data-l-label]').value.trim();
          const url = row.querySelector('[data-l-url]').value.trim();
          if (!label || !url) return;
          payload.push({
            label,
            url,
            kind: row.querySelector('[data-l-kind]').value,
            sort: (i + 1) * 10,
          });
        });

        const btn = e.currentTarget;
        btn.disabled = true;
        try {
          await api.setResourceLinks(id, payload);
          toastOk(`已保存 ${payload.length} 条外链`);
          window.__reloadPage();
        } catch (err) {
          btn.disabled = false;
          toastError(err.message || '保存失败');
        }
      });
    },
  };
}

function linkRowHTML(l) {
  const kindOpts = LINK_KINDS.map((k) => `
    <option value="${k.key}" ${l && l.kind === k.key ? 'selected' : ''}>${esc(k.label)}</option>`).join('');
  return `<div class="admin-guide-row">
    <div class="field">
      <label>名称</label>
      <input class="input" data-l-label value="${esc(l ? l.label : '')}" placeholder="例如：官方安装文档">
    </div>
    <div class="field">
      <label>类型</label>
      <select class="select" data-l-kind>${kindOpts}</select>
    </div>
    <div class="field admin-grow">
      <label>地址</label>
      <input class="input" data-l-url value="${esc(l ? l.url : '')}" placeholder="https://…">
    </div>
    <button class="icon-btn admin-row-del" type="button" data-remove-row title="删除这条">${icon('trash', 15)}</button>
  </div>`;
}

/* ------------------------------------------------------------ 题库 -- */

export async function adminQuizPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const page = Math.max(1, Number(ctx.params.get('page')) || 1);
  const pageSize = 20;
  const status = ctx.params.get('status') || '';
  const keyword = ctx.params.get('keyword') || '';

  const data = await api.adminQuestions({ status, keyword, page, pageSize });
  const statusOpts = ['', 'pending', 'published', 'draft', 'rejected', 'offline'].map((v) => `
    <option value="${v}" ${v === status ? 'selected' : ''}>${v ? esc(STATUS_LABEL[v] || v) : '全部状态'}</option>`).join('');

  const rows = (data.list || []).map((it) => {
    // 待审的题来自用户投稿，操作是「通过 / 驳回」；运营自己录的题走
    // 草稿 → 发布，两条路径的语义不同，按钮也就不能共用一套。
    const ops = it.status === 'pending'
      ? `<button class="btn btn-sm btn-primary" type="button" data-quiz-pass="${it.id}">通过</button>
         <button class="btn btn-sm btn-danger" type="button" data-quiz-reject="${it.id}">驳回</button>`
      : (it.status === 'published'
        ? `<button class="btn btn-sm" type="button" data-offline="${it.id}">下线</button>`
        : `<button class="btn btn-sm btn-primary" type="button" data-publish="${it.id}">发布</button>`);
    return `<tr>
    <td>
      <div class="admin-cell-title">${esc(it.stem)}</div>
      <div class="admin-cell-sub">
        ${esc(QUIZ_TYPE_LABEL[it.type] || it.type)} · 答案 ${esc((it.correct || []).join(''))}
        · 答对率 ${it.correctRate ? Number(it.correctRate).toFixed(1) + '%' : '—'}
      </div>
    </td>
    <td>${esc(it.categoryName || '未分类')}</td>
    <td>${esc(DIFFICULTY_LABEL[it.difficulty] || it.difficulty || '—')}</td>
    <td>${statusChip(it.status)}${it.reviewNote
      ? `<div class="admin-cell-sub">${esc(it.reviewNote)}</div>` : ''}</td>
    <td class="admin-num">${fmtNum(it.answerCount)}</td>
    <td class="admin-ops">
      ${ops}
      <button class="btn btn-sm btn-danger" type="button" data-del="${it.id}">删除</button>
    </td>
  </tr>`;
  }).join('');

  const body = `
    <div class="admin-toolbar">
      <form class="toolbar admin-filters" data-filter>
        <select class="select" name="status">${statusOpts}</select>
        <input class="input" name="keyword" value="${esc(keyword)}" placeholder="搜题干" style="max-width:240px">
        <button class="btn btn-sm" type="submit">${icon('search', 14)}筛选</button>
      </form>
      <button class="btn btn-sm btn-primary" type="button" data-quiz-import>
        ${icon('layers', 14)}批量导入
      </button>
    </div>
    <div class="card admin-table-wrap">
      <table class="admin-table">
        <thead><tr><th>题干</th><th>方向</th><th>难度</th><th>状态</th><th class="admin-num">作答</th><th class="admin-ops">操作</th></tr></thead>
        <tbody>${rows || '<tr><td colspan="6"><p class="muted-note">没有匹配的题目。</p></td></tr>'}</tbody>
      </table>
    </div>
    <div class="admin-pager">${pagerHTML({ page, pageSize, total: data.total || 0 })}</div>`;

  return {
    title: '题库管理',
    html: adminShell('/admin/quiz', {
      title: '题库',
      desc: '按方向组织。支持批量导入（Excel 直接粘贴或 JSON）与用户投稿审核；导入的题一律先落草稿。',
      body,
    }),
    mount: (root) => {
      bindFilter(root);
      bindPager(root, (p) => setHashQuery({ page: p > 1 ? String(p) : '' }, { keepPage: true }));
      bindStandardRowActions(root, {
        publish: api.publishQuestion,
        offline: api.offlineQuestion,
        remove: api.deleteQuestion,
      });

      root.querySelector('[data-quiz-import]')?.addEventListener('click', openQuizImportModal);

      root.addEventListener('click', async (ev) => {
        const btn = ev.target.closest('button');
        if (!btn) return;
        const passId = btn.getAttribute('data-quiz-pass');
        const rejectId = btn.getAttribute('data-quiz-reject');
        if (!passId && !rejectId) return;

        if (rejectId) {
          openQuizRejectModal(rejectId);
          return;
        }
        btn.disabled = true;
        try {
          await api.reviewQuestion(passId, { action: 'pass' });
          toastOk('已通过并发布');
          window.__reloadPage();
        } catch (err) {
          btn.disabled = false;
          toastError(err instanceof ApiError ? err.message : '操作失败');
        }
      });
    },
  };
}

/** 驳回题目：必须填原因，否则投稿人只能反复试错。 */
function openQuizRejectModal(id) {
  openModal(`
    <div class="modal-head"><h2>驳回题目</h2></div>
    <div class="modal-body">
      <form class="admin-form" data-quiz-reject-form>
        <div class="field">
          <label for="quiz-note">驳回原因</label>
          <textarea class="input" id="quiz-note" name="note" rows="4" maxlength="200" required
            placeholder="例如：正确答案标错了，C 才是对的；或解析与题干对不上"></textarea>
          <p class="muted-note">原因会显示给投稿人，帮助他改对。</p>
        </div>
        <div class="modal-actions">
          <button class="btn" type="button" data-modal-close>取消</button>
          <button class="btn btn-danger" type="submit">确认驳回</button>
        </div>
      </form>
    </div>
  `, (modal) => {
    const form = modal.querySelector('[data-quiz-reject-form]');
    form.addEventListener('submit', async (ev) => {
      ev.preventDefault();
      const note = form.elements.note.value.trim();
      if (!note) { toastError('请填写驳回原因'); return; }
      const submit = form.querySelector('button[type="submit"]');
      submit.disabled = true;
      try {
        await api.reviewQuestion(id, { action: 'reject', note });
        closeModal();
        toastOk('已驳回');
        window.__reloadPage();
      } catch (err) {
        submit.disabled = false;
        toastError(err instanceof ApiError ? err.message : '操作失败');
      }
    });
  });
}

/* -------------------------------------------------------- 批量导入 -- */

const QUIZ_IMPORT_PLACEHOLDER = `# 方式一：从 Excel 直接复制（含表头也行）
题干	题型	选项A	选项B	选项C	选项D	正确答案	解析	难度	分类ID
容器内 df 显示磁盘已满但 du 求和只有 40%，最可能的原因是什么？	单选	有进程持有已删除的文件	磁盘没挂上	du 统计有 bug	inode 用尽	A	被删除但仍被进程打开的文件会继续占用空间，lsof +L1 可以查到。	中等	3

# 方式二：JSON 数组（字段同导入接口）
[{"stem":"...","type":"single","options":[{"key":"A","text":"..."}],"correct":["A"],"analysis":"...","difficulty":"easy","categoryId":3}]`;

/**
 * openQuizImportModal 批量导入题目。
 *
 * 支持两种粘贴格式：
 *  1. TSV / CSV —— 运营的真实工作流是"在 Excel 里整理好，全选复制"，
 *     要求他们先转成 JSON 等于凭空加一道工序；
 *  2. JSON 数组 —— 脚本生成或接口调试时用。
 *
 * 转换在浏览器做，后端只处理一种结构。这样后端校验逻辑与单题录入完全一致，
 * 不会出现"导入的题比手录的少校验几项"这种漏洞。
 */
function openQuizImportModal() {
  openModal(`
    <div class="modal-head"><h2>批量导入题目</h2></div>
    <div class="modal-body">
      <div class="field">
        <label for="qi-text">粘贴内容 <span class="hint">单次最多 200 道</span></label>
        <textarea class="input code-input" id="qi-text" rows="10"
          placeholder="${esc(QUIZ_IMPORT_PLACEHOLDER)}"></textarea>
        <p class="muted-note">
          表格列顺序：题干 / 题型（单选·多选·判断）/ 选项A-D / 正确答案（如 A 或 AB）/ 解析 / 难度 / 分类ID / 标签。
          判断题的选项留空即可。
        </p>
      </div>
      <div class="admin-import-result" id="qi-result"></div>
      <div class="modal-actions">
        <button class="btn" type="button" data-modal-close>取消</button>
        <button class="btn btn-primary" type="button" id="qi-submit">开始导入</button>
      </div>
    </div>
  `, (modal) => {
    const textEl = modal.querySelector('#qi-text');
    const resultEl = modal.querySelector('#qi-result');
    const submit = modal.querySelector('#qi-submit');

    submit.addEventListener('click', async () => {
      let items;
      try {
        items = parseQuizImportText(textEl.value);
      } catch (err) {
        toastError(err.message || '格式无法识别');
        return;
      }
      if (!items.length) {
        toastError('没有解析出任何题目');
        return;
      }
      if (items.length > 200) {
        toastError('单次最多导入 200 道题，请分批提交');
        return;
      }

      submit.disabled = true;
      submit.textContent = '导入中…';
      try {
        const res = await api.importQuestions({ items });
        if (!res.errors || !res.errors.length) {
          closeModal();
          toastOk(`已导入 ${fmtNum(res.imported)} 道题（草稿状态，确认后发布）`);
          window.__reloadPage();
          return;
        }
        // 有错误行时一条都没写进去，逐行摆出来让运营一次改完。
        resultEl.innerHTML = `
          <div class="admin-import-errors">
            <p><strong>${res.errors.length} 行有问题，本次一条都没有导入。</strong>
              改完再提交一次即可（不会重复）。</p>
            <ul>
              ${res.errors.map((e) => `<li><span class="chip chip-danger">第 ${e.row} 行</span>
                ${e.stem ? esc(e.stem) + ' — ' : ''}${esc(e.message)}</li>`).join('')}
            </ul>
          </div>`;
        submit.disabled = false;
        submit.textContent = '重新导入';
      } catch (err) {
        submit.disabled = false;
        submit.textContent = '开始导入';
        toastError(err instanceof ApiError ? err.message : '导入失败');
      }
    });
  });
}

/**
 * parseQuizImportText 把粘贴内容转成导入接口的 items 数组。
 *
 * 表格列（缺列按空处理，多余的列忽略）：
 *   0 题干 | 1 题型 | 2-5 选项A-D | 6 正确答案 | 7 解析 | 8 难度 | 9 分类ID | 10 标签
 */
function parseQuizImportText(text) {
  const raw = String(text || '').trim();
  if (!raw) throw new Error('请先粘贴内容');

  if (raw.startsWith('[')) {
    let arr;
    try {
      arr = JSON.parse(raw);
    } catch (err) {
      throw new Error('JSON 解析失败：' + err.message);
    }
    if (!Array.isArray(arr)) throw new Error('JSON 顶层必须是数组');
    return arr;
  }

  const lines = raw.split(/\r?\n/).filter((l) => l.trim());
  const out = [];
  lines.forEach((line, idx) => {
    const cells = line.includes('\t') ? line.split('\t') : splitCSVLine(line);
    // 表头：第一列是「题干」就跳过。别用"第一行一定不是数据"来判——
    // 有人不写表头，那样会白丢一道题。
    if (idx === 0 && /^(题干|stem)$/i.test(String(cells[0] || '').trim())) return;

    const stem = String(cells[0] || '').trim();
    if (!stem) return;

    const typeRaw = String(cells[1] || '').trim();
    const type = /多选|multiple/i.test(typeRaw) ? 'multiple'
      : /判断|judge/i.test(typeRaw) ? 'judge'
        : 'single';

    const options = [];
    if (type !== 'judge') {
      ['A', 'B', 'C', 'D'].forEach((key, i) => {
        const t = String(cells[2 + i] || '').trim();
        if (t) options.push({ key, text: t });
      });
    }

    const answerRaw = String(cells[6] || '').trim();
    let correct = [];
    if (type === 'judge') {
      // 判断题的答案允许写「对/错」「正确/错误」「√/×」，统一成 A/B。
      // 只认字母会让写"对"的人莫名其妙被拒。
      if (/^(对|正确|√|是|true|a)$/i.test(answerRaw)) correct = ['A'];
      else if (/^(错|错误|×|否|false|b)$/i.test(answerRaw)) correct = ['B'];
    } else {
      correct = answerRaw.toUpperCase().replace(/[^A-H]/g, '').split('').slice(0, 8);
    }

    const difficultyRaw = String(cells[8] || '').trim();
    const difficulty = /简单|easy/i.test(difficultyRaw) ? 'easy'
      : /困难|难|hard/i.test(difficultyRaw) ? 'hard'
        : 'medium';

    const catRaw = String(cells[9] || '').trim();
    const categoryId = /^\d+$/.test(catRaw) ? Number(catRaw) : 0;

    out.push({
      stem,
      type,
      options,
      correct,
      analysis: String(cells[7] || '').trim(),
      difficulty,
      categoryId,
      tags: splitTags(String(cells[10] || '').trim()),
    });
  });
  return out;
}

/** splitCSVLine 解析一行 CSV，支持双引号包裹与外层引号转义。 */
function splitCSVLine(line) {
  const cells = [];
  let cur = '';
  let quoted = false;
  for (let i = 0; i < line.length; i += 1) {
    const ch = line[i];
    if (quoted) {
      if (ch === '"' && line[i + 1] === '"') { cur += '"'; i += 1; }
      else if (ch === '"') quoted = false;
      else cur += ch;
    } else if (ch === '"') {
      quoted = true;
    } else if (ch === ',') {
      cells.push(cur);
      cur = '';
    } else {
      cur += ch;
    }
  }
  cells.push(cur);
  return cells;
}

/* -------------------------------------------------------- 专家审核 -- */

export async function adminExpertsPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const page = Math.max(1, Number(ctx.params.get('page')) || 1);
  const pageSize = 20;
  const status = ctx.params.get('status') || 'pending';

  const data = await api.adminExperts({ status, page, pageSize });
  const statusOpts = ['pending', 'approved', 'rejected', 'suspended', ''].map((v) => `
    <option value="${v}" ${v === status ? 'selected' : ''}>${v ? esc({
      pending: '待审核', approved: '已通过', rejected: '已驳回', suspended: '已暂停',
    }[v]) : '全部'}</option>`).join('');

  const rows = (data.list || []).map((it) => `<tr>
    <td>
      <div class="admin-cell-title">${esc(it.nickname || `用户 #${it.userId}`)}</div>
      <div class="admin-cell-sub">${esc(it.realName || '未填实名')} · ${esc(it.maskedIdCard || '—')}</div>
    </td>
    <td>${(it.skills || []).slice(0, 4).map((s) => `<span class="chip">${esc(s.name)}</span>`).join('') || '—'}</td>
    <td class="admin-num">${it.pricePerHourCents ? esc(fmtCents(it.pricePerHourCents)) : '—'}</td>
    <td>${it.appliedAt ? esc(fmtDate(it.appliedAt)) : '—'}</td>
    <td>${statusChip(it.status)}${it.auditRemark ? `<div class="admin-cell-sub">${esc(it.auditRemark)}</div>` : ''}</td>
    <td class="admin-ops">
      ${it.status === 'pending' ? `
        <button class="btn btn-sm btn-primary" type="button" data-audit="${it.id}" data-approved="1">通过</button>
        <button class="btn btn-sm btn-danger" type="button" data-audit="${it.id}" data-approved="0">驳回</button>` : '—'}
    </td>
  </tr>`).join('');

  const body = `
    <form class="toolbar admin-filters" data-filter>
      <select class="select" name="status">${statusOpts}</select>
      <button class="btn btn-sm" type="submit">${icon('filter', 14)}筛选</button>
    </form>
    <div class="card admin-table-wrap">
      <table class="admin-table">
        <thead><tr><th>专家</th><th>技能</th><th class="admin-num">时价</th><th>申请时间</th><th>状态</th><th class="admin-ops">操作</th></tr></thead>
        <tbody>${rows || '<tr><td colspan="6"><p class="muted-note">没有符合条件的申请。</p></td></tr>'}</tbody>
      </table>
    </div>
    <div class="admin-pager">${pagerHTML({ page, pageSize, total: data.total || 0 })}</div>`;

  return {
    title: '专家审核',
    html: adminShell('/admin/experts', {
      title: '专家入驻审核',
      desc: '通过后才能接单。驳回要写原因，申请人会看到。',
      body,
    }),
    mount: (root) => {
      bindFilter(root);
      bindPager(root, (p) => setHashQuery({ page: p > 1 ? String(p) : '' }, { keepPage: true }));
      root.addEventListener('click', (e) => {
        const btn = e.target.closest('[data-audit]');
        if (!btn) return;
        openExpertAuditModal(btn.getAttribute('data-audit'), btn.getAttribute('data-approved') === '1');
      });
    },
  };
}

function openExpertAuditModal(id, approved) {
  openModal(`
    <div class="modal-head">
      <div><h2>${approved ? '通过入驻申请' : '驳回入驻申请'}</h2></div>
    </div>
    <div class="modal-body">
      <div class="field">
        <label for="audit-remark">${approved ? '备注（可选）' : '驳回原因（必填）'}</label>
        <textarea class="input" id="audit-remark" rows="3"
          placeholder="${approved ? '例如：资质齐全' : '例如：证书照片模糊，无法核对姓名'}"></textarea>
      </div>
      <div class="modal-actions">
        <button class="btn" type="button" data-modal-close>取消</button>
        <button class="btn ${approved ? 'btn-primary' : 'btn-danger'}" type="button" data-do-audit>确认</button>
      </div>
    </div>
  `, (modal) => {
    modal.querySelector('[data-do-audit]').addEventListener('click', async (e) => {
      const remark = modal.querySelector('#audit-remark').value.trim();
      if (!approved && !remark) {
        toastError('驳回需要填写原因');
        return;
      }
      const btn = e.currentTarget;
      btn.disabled = true;
      try {
        await api.auditExpert(id, { approved, remark });
        closeModal();
        toastOk('已处理');
        window.__reloadPage();
      } catch (err) {
        btn.disabled = false;
        toastError(err.message || '操作失败');
      }
    });
  });
}

/* ------------------------------------------------------------ 提现审批 -- */

/**
 * 提现状态文案。与专家端工作台保持同一套字：
 * 同一个人在两处看到「待打款」和「审核通过」会以为是两件事。
 */
const WITHDRAW_STATUS = {
  pending:  { label: '待审核', tone: 'chip-warn' },
  approved: { label: '待打款', tone: 'chip-info' },
  paid:     { label: '已打款', tone: 'chip-ok' },
  rejected: { label: '已驳回', tone: 'chip-danger' },
  failed:   { label: '打款失败', tone: 'chip-danger' },
};

const WITHDRAW_TABS = [
  { key: 'pending', label: '待审核' },
  { key: 'approved', label: '待打款' },
  { key: 'paid', label: '已打款' },
  { key: 'rejected', label: '已驳回' },
  { key: 'failed', label: '打款失败' },
  { key: 'all', label: '全部' },
];

const ACCOUNT_TYPE_LABEL = { bank: '银行卡', alipay: '支付宝', wechat: '微信零钱' };

function withdrawChip(status) {
  const meta = WITHDRAW_STATUS[status] || { label: status || '—', tone: '' };
  return `<span class="chip ${meta.tone}">${esc(meta.label)}</span>`;
}

export async function adminWithdrawalsPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const tabKey = WITHDRAW_TABS.some((t) => t.key === ctx.params.get('status'))
    ? ctx.params.get('status') : 'pending';
  // 「全部」用一个显式值 all：空串会被 setHashQuery 删掉，刷新后又跳回待审核。
  const status = tabKey === 'all' ? '' : tabKey;
  const page = Math.max(1, Number(ctx.params.get('page')) || 1);
  const pageSize = 20;

  const data = await api.adminWithdrawals({ status, page, pageSize });
  const list = data.list || [];

  const tabsHTML = `<div class="seg admin-tabs">${WITHDRAW_TABS.map((t) => `
    <a href="#/admin/withdrawals?status=${t.key}" class="${t.key === tabKey ? 'is-active' : ''}">${esc(t.label)}</a>`).join('')}</div>`;

  const rows = list.map((it) => {
    const amount = fmtCents(it.amountCents);
    const account = `${ACCOUNT_TYPE_LABEL[it.accountType] || it.accountType || '—'} · ${it.accountName || '—'} · ${it.accountNoMasked || '—'}`;
    return `<tr>
      <td>
        <div class="admin-cell-title mono">${esc(it.withdrawalNo)}</div>
        <div class="admin-cell-sub">${when(it.createdAt)}</div>
      </td>
      <td>
        <div class="admin-cell-title">${esc(it.expertName || it.expertNick || `用户 #${it.expertId}`)}</div>
        <div class="admin-cell-sub">${esc(it.expertNick || '—')} · 余额 ${esc(fmtCents(it.expertBalanceCents || 0))} · 冻结 ${esc(fmtCents(it.expertFrozenCents || 0))}</div>
      </td>
      <td class="admin-num">${esc(amount)}</td>
      <td>
        <div class="admin-cell-sub">${esc(account)}</div>
        <div class="admin-cell-sub">实际到账 ${esc(fmtCents(it.actualCents || 0))}</div>
      </td>
      <td>${withdrawChip(it.status)}${it.remark ? `<div class="admin-cell-sub">${esc(it.remark)}</div>` : ''}</td>
      <td class="admin-ops">${withdrawOpsHTML(it)}</td>
    </tr>`;
  }).join('');

  const body = `
    <p class="admin-hint">
      通过只是同意提现，钱仍冻在专家账户里；线下转账完成后点「已打款」才会真正扣掉冻结。
      驳回或打款失败会把金额退回专家的可提现余额。
    </p>
    <div class="card admin-table-wrap">
      <table class="admin-table">
        <thead><tr>
          <th>提现单</th><th>专家</th><th class="admin-num">申请金额</th><th>收款账户</th><th>状态</th><th class="admin-ops">操作</th>
        </tr></thead>
        <tbody>${rows || `<tr><td colspan="6"><p class="muted-note">${
          tabKey === 'pending' ? '没有待审核的提现申请。' : '没有符合条件的提现记录。'}</p></td></tr>`}</tbody>
      </table>
    </div>
    <div class="admin-pager">${pagerHTML({ page, pageSize, total: data.total || 0 })}</div>`;

  return {
    title: '提现审批',
    html: adminShell('/admin/withdrawals', {
      title: '提现审批',
      desc: '审核通过后去线下转账，回来再标记已打款。每一步都能追到人。',
      actions: tabsHTML,
      body,
    }),
    mount: (root) => {
      bindPager(root, (p) => setHashQuery({ page: p > 1 ? String(p) : '' }, { keepPage: true }));
      root.addEventListener('click', (e) => {
        const btn = e.target.closest('[data-wd]');
        if (!btn) return;
        const item = list.find((x) => String(x.id) === btn.getAttribute('data-wd'));
        if (item) openWithdrawModal(item, btn.getAttribute('data-act'));
      });
    },
  };
}

function withdrawOpsHTML(it) {
  if (it.status === 'pending') {
    return `<button class="btn btn-sm btn-primary" type="button" data-wd="${it.id}" data-act="approve">通过</button>
      <button class="btn btn-sm btn-danger" type="button" data-wd="${it.id}" data-act="reject">驳回</button>`;
  }
  if (it.status === 'approved') {
    return `<button class="btn btn-sm btn-primary" type="button" data-wd="${it.id}" data-act="paid">已打款</button>
      <button class="btn btn-sm" type="button" data-wd="${it.id}" data-act="fail">打款失败</button>`;
  }
  return '<span class="muted-note">—</span>';
}

function openWithdrawModal(item, action) {
  const cfg = {
    approve: {
      title: '通过提现申请',
      label: '备注（可选）',
      ph: '例如：收款人与实名一致，已核对',
      ok: '确认通过',
      btn: 'btn-primary',
      required: false,
      effect: `通过后这笔钱仍冻结在专家账户里（${fmtCents(item.amountCents)}），等你线下转账后再点「已打款」。`,
    },
    reject: {
      title: '驳回提现申请',
      label: '驳回原因（必填）',
      ph: '例如：收款人与实名人不是同一人，请重新填写',
      ok: '确认驳回',
      btn: 'btn-danger',
      required: true,
      effect: `驳回会把 ${fmtCents(item.amountCents)} 退回专家的可提现余额，他可以修改账户后重新申请。`,
    },
    paid: {
      title: '标记已打款',
      label: '打款流水号 / 备注（必填）',
      ph: '例如：招行 2026102109300012345',
      ok: '确认已打款',
      btn: 'btn-primary',
      required: true,
      effect: `确认后将从专家冻结金额中扣掉 ${fmtCents(item.amountCents)}。请确认已经完成转账——这一步不可撤销。`,
    },
    fail: {
      title: '标记打款失败',
      label: '失败原因（必填）',
      ph: '例如：银行卡号已注销，渠道退回',
      ok: '确认失败并退回余额',
      btn: 'btn-danger',
      required: true,
      effect: `打款失败会把 ${fmtCents(item.amountCents)} 退回专家的可提现余额，他可以重新申请。`,
    },
  }[action];
  if (!cfg) return;

  openModal(`
    <div class="modal-head">
      <div>
        <h2>${esc(cfg.title)}</h2>
        <p class="modal-sub">${esc(item.withdrawalNo)} · ${esc(item.expertName || '')} · ${esc(fmtCents(item.amountCents))}</p>
      </div>
    </div>
    <div class="modal-body">
      <p class="admin-hint">${esc(cfg.effect)}</p>
      <div class="field">
        <label for="wd-remark">${esc(cfg.label)}</label>
        <textarea class="input" id="wd-remark" rows="3" placeholder="${esc(cfg.ph)}"></textarea>
      </div>
      <div class="modal-actions">
        <button class="btn" type="button" data-modal-close>取消</button>
        <button class="btn ${cfg.btn}" type="button" data-do-wd>${esc(cfg.ok)}</button>
      </div>
    </div>
  `, (modal) => {
    modal.querySelector('[data-do-wd]').addEventListener('click', async (e) => {
      const remark = modal.querySelector('#wd-remark').value.trim();
      if (cfg.required && !remark) {
        toastError('这一项必须填写');
        return;
      }
      const btn = e.currentTarget;
      btn.disabled = true;
      try {
        if (action === 'approve' || action === 'reject') {
          await api.adminWithdrawalReview(item.id, { action, remark });
        } else if (action === 'paid') {
          await api.adminWithdrawalPaid(item.id, { remark });
        } else {
          await api.adminWithdrawalFail(item.id, { remark });
        }
        closeModal();
        toastOk('已处理');
        // 不在前端自己改状态：钱的流转以服务端为准，刷新一遍最稳。
        window.__reloadPage();
      } catch (err) {
        btn.disabled = false;
        toastError(err.message || '操作失败');
      }
    });
  });
}

/* ------------------------------------------------------------ 订单 -- */

export async function adminOrdersPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const page = Math.max(1, Number(ctx.params.get('page')) || 1);
  const pageSize = 20;
  const status = ctx.params.get('status') || '';
  const keyword = ctx.params.get('keyword') || '';

  const data = await api.adminOrders({ status, keyword, page, pageSize });
  const statusOpts = [''].concat(Object.keys(ORDER_STATUS)).map((v) => `
    <option value="${v}" ${v === status ? 'selected' : ''}>${v ? esc(ORDER_STATUS[v].label) : '全部状态'}</option>`).join('');

  const rows = (data.list || []).map((o) => `<tr>
    <td>
      <div class="admin-cell-title admin-mono">${esc(o.orderNo)}</div>
      <div class="admin-cell-sub">${esc(o.title || '—')}</div>
    </td>
    <td>${esc(ORDER_TYPE_LABEL[o.type] || o.type)}</td>
    <td>${esc(o.userName || '#' + o.userId)} → ${esc(o.expertName || '#' + o.expertId)}</td>
    <td class="admin-num">${esc(fmtCents(o.amountCents))}</td>
    <td>${statusChipHTML(o.status)}</td>
    <td>${when(o.createdAt)}</td>
    <td class="admin-ops">${adminOrderOpsHTML(o)}</td>
  </tr>`).join('');

  const body = `
    <form class="toolbar admin-filters" data-filter>
      <select class="select" name="status">${statusOpts}</select>
      <input class="input" name="keyword" value="${esc(keyword)}" placeholder="搜订单号 / 标题" style="max-width:240px">
      <button class="btn btn-sm" type="submit">${icon('search', 14)}筛选</button>
    </form>
    <div class="card admin-table-wrap">
      <table class="admin-table">
        <thead><tr>
          <th>订单</th><th>类型</th><th>用户 → 专家</th><th class="admin-num">金额</th>
          <th>状态</th><th>创建</th><th class="admin-ops">操作</th>
        </tr></thead>
        <tbody>${rows || '<tr><td colspan="7"><p class="muted-note">没有匹配的订单。</p></td></tr>'}</tbody>
      </table>
    </div>
    <div class="admin-pager">${pagerHTML({ page, pageSize, total: data.total || 0 })}</div>`;

  return {
    title: '订单处理',
    html: adminShell('/admin/orders', {
      title: '订单处理',
      desc: '后台只能推进状态机里存在的下一步——这不是限制，是为了不造出对不上账的订单。',
      body,
    }),
    mount: (root) => {
      bindFilter(root);
      bindPager(root, (p) => setHashQuery({ page: p > 1 ? String(p) : '' }, { keepPage: true }));
      root.addEventListener('click', async (e) => {
        const btn = e.target.closest('button');
        if (!btn) return;

        const paidNo = btn.getAttribute('data-mark-paid');
        if (paidNo) {
          btn.disabled = true;
          try {
            await api.adminMarkPaid(paidNo, { remark: '后台确认线下收款' });
            toastOk('已标记收款');
            window.__reloadPage();
          } catch (err) {
            btn.disabled = false;
            toastError(err.message || '操作失败');
          }
          return;
        }

        const settleNo = btn.getAttribute('data-settle');
        if (settleNo && window.confirm('结算会把款项按分账结果结给专家，确认？')) {
          btn.disabled = true;
          try {
            await api.adminSettle(settleNo);
            toastOk('已结算');
            window.__reloadPage();
          } catch (err) {
            btn.disabled = false;
            toastError(err.message || '结算失败');
          }
          return;
        }

        const moveNo = btn.getAttribute('data-move');
        if (moveNo) {
          const to = btn.getAttribute('data-to');
          if (!window.confirm(`把订单推进到「${ORDER_STATUS[to] ? ORDER_STATUS[to].label : to}」？`)) return;
          btn.disabled = true;
          try {
            await api.adminTransition(moveNo, { to, remark: '后台强制流转' });
            toastOk('已流转');
            window.__reloadPage();
          } catch (err) {
            btn.disabled = false;
            toastError(err.message || '流转失败');
          }
        }
      });
    },
  };
}

function adminOrderOpsHTML(o) {
  const out = [];
  if (o.status === 'pending_pay' && o.payChannel === 'offline') {
    out.push(`<button class="btn btn-sm btn-primary" type="button" data-mark-paid="${esc(o.orderNo)}">确认收款</button>`);
  }
  if (o.status === 'completed') {
    out.push(`<button class="btn btn-sm btn-primary" type="button" data-settle="${esc(o.orderNo)}">结算</button>`);
  }
  // 只列状态机允许的下一步：下拉里出现一个必然报错的选项，比不给选项更糟。
  (ORDER_TRANSITIONS[o.status] || []).filter((s) => s !== 'cancelled').slice(0, 2).forEach((to) => {
    out.push(`<button class="btn btn-sm" type="button" data-move="${esc(o.orderNo)}" data-to="${esc(to)}"
      >→ ${esc(ORDER_STATUS[to] ? ORDER_STATUS[to].label : to)}</button>`);
  });
  return out.join('') || '<span class="muted-note">—</span>';
}

/* -------------------------------------------------------- 用户治理 -- */

const USER_ROLE_LABEL = { user: '普通用户', expert: '专家', admin: '管理员' };
const USER_STATUS_LABEL = { active: '正常', disabled: '已封禁' };
const USER_STATUS_TONE = { active: 'chip-ok', disabled: 'chip-danger' };
const EXPERT_AUDIT_LABEL = {
  pending: '待审核', approved: '已通过', rejected: '已驳回', suspended: '已暂停',
};
const AUDIT_ACTION_LABEL = {
  'user.disable': '封禁账号',
  'user.enable': '解封账号',
  'user.role': '修改角色',
};

function userRoleLabel(role) { return USER_ROLE_LABEL[role] || role || '—'; }

function userStatusChip(status) {
  return `<span class="chip ${USER_STATUS_TONE[status] || ''}">${esc(USER_STATUS_LABEL[status] || status || '—')}</span>`;
}

export async function adminUsersPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const page = Math.max(1, Number(ctx.params.get('page')) || 1);
  const pageSize = 20;
  const keyword = ctx.params.get('keyword') || '';
  const role = ctx.params.get('role') || '';
  const status = ctx.params.get('status') || '';

  const data = await api.adminUsers({ keyword, role, status, page, pageSize });
  const me = currentUser() || {};

  const roleOpts = [''].concat(Object.keys(USER_ROLE_LABEL)).map((v) => `
    <option value="${v}" ${v === role ? 'selected' : ''}>${v ? esc(USER_ROLE_LABEL[v]) : '全部角色'}</option>`).join('');
  const statusOpts = [''].concat(Object.keys(USER_STATUS_LABEL)).map((v) => `
    <option value="${v}" ${v === status ? 'selected' : ''}>${v ? esc(USER_STATUS_LABEL[v]) : '全部状态'}</option>`).join('');

  const rows = (data.list || []).map((u) => {
    const isSelf = Number(u.id) === Number(me.id);
    const expertNote = u.expertId
      ? `<div class="admin-cell-sub">专家 · ${esc(EXPERT_AUDIT_LABEL[u.expertStatus] || u.expertStatus || '—')}</div>` : '';
    return `<tr>
      <td>
        <div class="admin-cell-title">${esc(u.nickname || '未设昵称')}${isSelf ? ' <span class="chip chip-info">我</span>' : ''}</div>
        <div class="admin-cell-sub">#${u.id} · ${esc(u.maskedPhone || '未绑手机号')}${u.email ? ' · ' + esc(u.email) : ''}</div>
      </td>
      <td>${esc(userRoleLabel(u.role))}</td>
      <td>${userStatusChip(u.status)}${
        u.statusReason ? `<div class="admin-cell-sub" title="${esc(u.statusReason)}">${esc(u.statusReason)}</div>` : ''}</td>
      <td>${u.expertId ? esc(EXPERT_AUDIT_LABEL[u.expertStatus] || u.expertStatus || '—') : '<span class="muted-note">—</span>'}</td>
      <td>${esc(fmtDate(u.createdAt))}</td>
      <td>${u.lastLoginAt ? esc(fmtDateTime(u.lastLoginAt)) : '<span class="muted-note">从未登录</span>'}</td>
      <td class="admin-ops">
        <button class="btn btn-sm" type="button" data-user-detail="${u.id}">详情</button>
        ${isSelf ? '' : (u.status === 'disabled'
          ? `<button class="btn btn-sm btn-primary" type="button" data-user-unban="${u.id}">解封</button>`
          : `<button class="btn btn-sm btn-danger" type="button" data-user-ban="${u.id}">封禁</button>`)}
      </td>
    </tr>`;
  }).join('');

  const body = `
    <form class="toolbar admin-filters" data-user-filter>
      <select class="select" name="role">${roleOpts}</select>
      <select class="select" name="status">${statusOpts}</select>
      <input class="input" name="keyword" value="${esc(keyword)}" maxlength="64"
             placeholder="完整手机号 / 昵称 / 邮箱" style="max-width:260px">
      <button class="btn btn-sm" type="submit">${icon('search', 14)}筛选</button>
    </form>
    <p class="admin-hint">
      列表里的手机号是脱敏的。要按人查，请输入完整的 11 位手机号——精确匹配才能找到人，
      也不会把同号段的账号整批捞出来。封禁立即生效：对方手上的令牌下一次请求就会被拒。
    </p>
    <div class="card admin-table-wrap">
      <table class="admin-table">
        <thead><tr>
          <th>用户</th><th>角色</th><th>状态</th><th>专家</th><th>注册</th><th>最近登录</th><th class="admin-ops">操作</th>
        </tr></thead>
        <tbody>${rows || '<tr><td colspan="7"><p class="muted-note">没有匹配的用户。</p></td></tr>'}</tbody>
      </table>
    </div>
    <div class="admin-pager">${pagerHTML({ page, pageSize, total: data.total || 0 })}</div>`;

  return {
    title: '用户治理',
    html: adminShell('/admin/users', {
      title: '用户治理',
      desc: '封禁改的是账号能不能用，不动订单与资金记录——解封后一切照旧。',
      body,
    }),
    mount: (root) => {
      const form = root.querySelector('[data-user-filter]');
      if (form) {
        form.addEventListener('submit', (e) => {
          e.preventDefault();
          const fd = new FormData(form);
          setHashQuery({
            role: String(fd.get('role') || ''),
            status: String(fd.get('status') || ''),
            keyword: String(fd.get('keyword') || '').trim(),
          });
        });
      }
      bindPager(root, (p) => setHashQuery({ page: p > 1 ? String(p) : '' }, { keepPage: true }));

      root.addEventListener('click', (e) => {
        const btn = e.target.closest('button');
        if (!btn) return;

        const detailId = btn.getAttribute('data-user-detail');
        if (detailId) { openUserDetail(Number(detailId)); return; }

        const banId = btn.getAttribute('data-user-ban');
        if (banId) { openUserBanModal(Number(banId), true); return; }

        const unbanId = btn.getAttribute('data-user-unban');
        if (unbanId) { openUserBanModal(Number(unbanId), false); }
      });
    },
  };
}

/** 用户详情：账号、资金与订单、专家档案、操作留痕。 */
async function openUserDetail(userId) {
  let d;
  try {
    d = await api.adminUser(userId);
  } catch (err) {
    toastError(err.message || '读取用户失败');
    return;
  }

  const stats = d.orderStats || {};
  const expert = d.expert;
  const audits = d.auditLogs || [];

  const auditRows = audits.map((a) => {
    const detail = a.detail || {};
    let note = '';
    if (detail.reason) note = detail.reason;
    else if (detail.from && detail.to) note = `${detail.from} → ${detail.to}`;
    return `<li class="admin-audit-item">
      <div class="admin-cell-title">${esc(AUDIT_ACTION_LABEL[a.action] || a.action)}</div>
      <div class="admin-cell-sub">${esc(fmtDateTime(a.createdAt))} · ${esc(a.userRole || '—')}${a.ip ? ' · ' + esc(a.ip) : ''}</div>
      ${note ? `<div class="admin-cell-sub">${esc(note)}</div>` : ''}
    </li>`;
  }).join('');

  const expertHTML = expert ? `
    <section class="admin-detail-block">
      <h3>专家档案</h3>
      <dl class="admin-kv">
        <dt>实名</dt><dd>${esc(expert.realName || '—')}</dd>
        <dt>审核状态</dt><dd>${esc(EXPERT_AUDIT_LABEL[expert.status] || expert.status)}${expert.auditRemark ? ' · ' + esc(expert.auditRemark) : ''}</dd>
        <dt>评分 / 完成单</dt><dd>${esc(String(expert.rating))} · ${fmtNum(expert.orderCount)} 单</dd>
        <dt>余额 / 冻结</dt><dd>${esc(fmtCents(expert.balanceCents || 0))} / ${esc(fmtCents(expert.frozenCents || 0))}</dd>
        <dt>累计收入</dt><dd>${esc(fmtCents(expert.totalIncomeCents || 0))}</dd>
        <dt>接单开关</dt><dd>${expert.acceptingOrder ? '已开启' : '已关闭'}</dd>
      </dl>
    </section>` : '';

  const activeWarn = Number(stats.activeCount) > 0
    ? `<p class="admin-hint admin-hint-warn">该用户还有 ${fmtNum(stats.activeCount)} 笔在途订单。封禁后这些单子不会自动取消，
       仍需在订单页逐笔处理，否则会卡在服务中。</p>` : '';

  openModal(`
    <div class="modal-head">
      <div>
        <h2>${esc(d.nickname || '未设昵称')}</h2>
        <p class="modal-sub">#${d.id} · ${esc(userRoleLabel(d.role))} · ${esc(d.maskedPhone || '未绑手机号')}</p>
      </div>
    </div>
    <div class="modal-body">
      ${d.status === 'disabled'
        ? `<p class="admin-hint admin-hint-warn">账号已封禁${d.statusReason ? '：' + esc(d.statusReason) : ''}${
            d.statusChangedAt ? '（' + esc(fmtDateTime(d.statusChangedAt)) + '）' : ''}</p>` : ''}
      ${activeWarn}

      <section class="admin-detail-block">
        <h3>订单与资金</h3>
        <dl class="admin-kv">
          <dt>作为用户</dt><dd>${fmtNum(stats.asUserTotal)} 单 · 已付 ${esc(fmtCents(stats.asUserPaidCents || 0))}</dd>
          <dt>作为专家</dt><dd>${fmtNum(stats.asExpertTotal)} 单 · 收入 ${esc(fmtCents(stats.asExpertIncomeCents || 0))}</dd>
          <dt>在途订单</dt><dd>${fmtNum(stats.activeCount)} 笔</dd>
          <dt>注册时间</dt><dd>${esc(fmtDateTime(d.createdAt))}</dd>
        </dl>
      </section>

      ${expertHTML}

      <section class="admin-detail-block">
        <h3>操作记录</h3>
        ${auditRows ? `<ul class="admin-audit">${auditRows}</ul>`
          : '<p class="muted-note">这个账号还没有后台操作记录。</p>'}
      </section>

      <div class="modal-actions">
        <button class="btn" type="button" data-modal-close>关闭</button>
        <button class="btn" type="button" data-change-role>修改角色</button>
        ${d.status === 'disabled'
          ? '<button class="btn btn-primary" type="button" data-toggle-ban="active">解封账号</button>'
          : '<button class="btn btn-danger" type="button" data-toggle-ban="disabled">封禁账号</button>'}
      </div>
    </div>
  `, (modal) => {
    const banBtn = modal.querySelector('[data-toggle-ban]');
    if (banBtn) {
      banBtn.addEventListener('click', () => {
        const next = banBtn.getAttribute('data-toggle-ban');
        closeModal();
        // 复用同一个弹窗函数：封禁要填原因，解封一句话确认。
        openUserBanModal(d.id, next === 'disabled', d.nickname);
      });
    }
    const roleBtn = modal.querySelector('[data-change-role]');
    if (roleBtn) {
      roleBtn.addEventListener('click', () => {
        closeModal();
        openUserRoleModal(d);
      });
    }
  });
}

/** 封禁（需填原因）/ 解封（一次确认）。 */
function openUserBanModal(userId, ban, nickname = '') {
  if (!ban) {
    openModal(`
      <div class="modal-head"><h2>解封账号</h2></div>
      <div class="modal-body">
        <p class="admin-hint">解封后该账号可以立即登录，专家仍按原审核状态接单。</p>
        <div class="modal-actions">
          <button class="btn" type="button" data-modal-close>取消</button>
          <button class="btn btn-primary" type="button" data-do-ban>确认解封</button>
        </div>
      </div>
    `, (modal) => {
      modal.querySelector('[data-do-ban]').addEventListener('click', async (e) => {
        e.currentTarget.disabled = true;
        try {
          await api.adminUserStatus(userId, { status: 'active' });
          toastOk('已解封');
          window.__reloadPage();
        } catch (err) {
          e.currentTarget.disabled = false;
          toastError(err.message || '解封失败');
        }
      });
    });
    return;
  }

  openModal(`
    <div class="modal-head">
      <div>
        <h2>封禁账号</h2>
        <p class="modal-sub">#${userId}${nickname ? ' · ' + esc(nickname) : ''}</p>
      </div>
    </div>
    <div class="modal-body">
      <p class="admin-hint admin-hint-warn">
        封禁立即生效，对方不必等登录过期就会被拦下。订单、余额、提现记录都不会被删改，
        解封后原样恢复。
      </p>
      <div class="field">
        <label for="ban-reason">封禁原因（必填，只有后台可见）</label>
        <textarea class="input" id="ban-reason" rows="3"
          placeholder="例如：多次发布虚假悬赏并拒绝付款，用户投诉工单 #1024"></textarea>
      </div>
      <div class="modal-actions">
        <button class="btn" type="button" data-modal-close>取消</button>
        <button class="btn btn-danger" type="button" data-do-ban>确认封禁</button>
      </div>
    </div>
  `, (modal) => {
    modal.querySelector('[data-do-ban]').addEventListener('click', async (e) => {
      const reason = modal.querySelector('#ban-reason').value.trim();
      if (!reason) {
        toastError('请填写封禁原因');
        return;
      }
      e.currentTarget.disabled = true;
      try {
        await api.adminUserStatus(userId, { status: 'disabled', reason });
        toastOk('已封禁');
        window.__reloadPage();
      } catch (err) {
        e.currentTarget.disabled = false;
        toastError(err.message || '封禁失败');
      }
    });
  });
}

/** 修改角色。升专家受服务端约束：必须已有审核通过的档案。 */
function openUserRoleModal(d) {
  const opts = Object.keys(USER_ROLE_LABEL).map((v) => `
    <option value="${v}" ${v === d.role ? 'selected' : ''}>${esc(USER_ROLE_LABEL[v])}</option>`).join('');

  openModal(`
    <div class="modal-head">
      <div>
        <h2>修改角色</h2>
        <p class="modal-sub">#${d.id} · ${esc(d.nickname || '未设昵称')}</p>
      </div>
    </div>
    <div class="modal-body">
      <p class="admin-hint">
        角色立即生效，不需要对方重新登录。改成「专家」要求这个人已经通过专家入驻审核——
        否则等于手工绕开审核直接进接单列表。
      </p>
      <div class="field">
        <label for="new-role">目标角色</label>
        <select class="select" id="new-role">${opts}</select>
      </div>
      <div class="modal-actions">
        <button class="btn" type="button" data-modal-close>取消</button>
        <button class="btn btn-primary" type="button" data-do-role>确认修改</button>
      </div>
    </div>
  `, (modal) => {
    modal.querySelector('[data-do-role]').addEventListener('click', async (e) => {
      const role = modal.querySelector('#new-role').value;
      if (role === d.role) {
        toastError('角色没有变化');
        return;
      }
      e.currentTarget.disabled = true;
      try {
        await api.adminUserRole(d.id, { role });
        toastOk('已修改角色');
        window.__reloadPage();
      } catch (err) {
        e.currentTarget.disabled = false;
        toastError(err.message || '修改失败');
      }
    });
  });
}

/* -------------------------------------------------------- 发言权限 -- */

const SPEAK_TABS = [
  { key: 'pending', label: '待审核' },
  { key: 'approved', label: '已通过' },
  { key: 'rejected', label: '已驳回' },
  { key: 'all', label: '全部' },
];

const SPEAK_STATUS_LABEL = {
  none: '未申请',
  pending: '待审核',
  approved: '可发言',
  rejected: '已驳回',
};

const SPEAK_STATUS_TONE = {
  none: '',
  pending: 'chip-warn',
  approved: 'chip-ok',
  rejected: 'chip-danger',
};

function speakStatusChip(status) {
  const s = status || 'none';
  return `<span class="chip ${SPEAK_STATUS_TONE[s] || ''}">${esc(SPEAK_STATUS_LABEL[s] || s)}</span>`;
}

/**
 * 发言权限审核。
 *
 * 注册只给只读：账号能登录、能看内容、能进个人中心，但提不了问、答不了题、
 * 投不了稿。要开口得先交实名（姓名 + 联系方式），这里放行。
 * 入口跟「用户治理」分开：那边管账号能不能用（封禁），这边管能不能说话。
 */
export async function adminSpeakApplicationsPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const tab = SPEAK_TABS.some((t) => t.key === ctx.params.get('tab'))
    ? ctx.params.get('tab') : 'pending';
  const page = Math.max(1, Number(ctx.params.get('page')) || 1);
  const pageSize = 20;

  // 「全部」按后端约定传空串，等价于不过滤状态。
  const data = await api.adminSpeakApplications({
    status: tab === 'all' ? '' : tab,
    page,
    pageSize,
  });
  const items = data.list || [];

  const tabsHTML = `<div class="seg admin-tabs">${SPEAK_TABS.map((t) => `
    <a href="#/admin/speak?tab=${t.key}" class="${t.key === tab ? 'is-active' : ''}">${esc(t.label)}</a>`).join('')}</div>`;

  const rows = items.map((it) => {
    const who = it.nickname || '未设昵称';
    const contact = it.email || it.phone || '';
    const pending = (it.status || 'pending') === 'pending';
    return `<tr>
      <td>
        <div class="admin-cell-title">${esc(who)}</div>
        <div class="admin-cell-sub">#${it.userId}${contact ? ' · ' + esc(contact) : ''}</div>
      </td>
      <td>
        <div class="admin-cell-title">${esc(it.realName || '—')}</div>
        <div class="admin-cell-sub">${esc(it.contact || '未留联系方式')}</div>
      </td>
      <td class="admin-cell-note">${it.note ? esc(it.note) : '<span class="muted-note">未填</span>'}</td>
      <td>${speakStatusChip(it.status)}</td>
      <td>${esc(fmtDate(it.appliedAt || it.createdAt))}</td>
      <td class="admin-ops">${
        pending
          ? `<button class="btn btn-sm btn-primary" type="button" data-speak-approve="${it.userId}">通过</button>
             <button class="btn btn-sm btn-danger" type="button" data-speak-reject="${it.userId}">驳回</button>`
          : `<a class="link-sm" href="#/admin/users?keyword=${encodeURIComponent(it.email || it.phone || '')}">看账号${icon('right', 11)}</a>`
      }</td>
    </tr>`;
  }).join('');

  const body = `
    <div class="admin-note-bar">
      <span>${icon('info', 14)}通过后这个人立刻可以提问、回答与投稿，不需要重新登录。驳回必须写原因——对方要在「我的」里看到它才知道怎么改。</span>
    </div>
    <div class="card admin-table-wrap">
      <table class="admin-table">
        <thead><tr>
          <th>账号</th><th>实名信息</th><th>申请理由</th><th>状态</th><th>申请时间</th><th class="admin-ops">操作</th>
        </tr></thead>
        <tbody>${rows || `<tr><td colspan="6"><p class="muted-note">${
          tab === 'pending' ? '没有待审核的发言申请。' : '没有匹配的申请记录。'
        }</p></td></tr>`}</tbody>
      </table>
    </div>
    <div class="admin-pager">${pagerHTML({ page, pageSize, total: data.total || 0 })}</div>`;

  return {
    title: '发言权限',
    html: adminShell('/admin/speak', {
      title: '发言权限',
      desc: '新注册账号默认只读。实名申请通过后才有提问、回答、投稿的资格。',
      actions: tabsHTML,
      body,
    }),
    mount: (root) => {
      bindPager(root, (p) => setHashQuery({ page: p > 1 ? String(p) : '' }, { keepPage: true }));

      root.addEventListener('click', (e) => {
        const btn = e.target.closest('button');
        if (!btn) return;

        const approveId = btn.getAttribute('data-speak-approve');
        if (approveId) { openSpeakReviewModal(Number(approveId), items, true); return; }

        const rejectId = btn.getAttribute('data-speak-reject');
        if (rejectId) { openSpeakReviewModal(Number(rejectId), items, false); }
      });
    },
  };
}

/** 通过 / 驳回共用一张弹窗：驳回时原因变必填。 */
function openSpeakReviewModal(userId, items, approve) {
  const target = (items || []).find((it) => Number(it.userId) === Number(userId)) || {};
  const title = approve ? '通过发言申请' : '驳回发言申请';

  openModal(`
    <div class="modal-head">
      <div>
        <h2>${esc(title)}</h2>
        <p>${esc(target.nickname || '未设昵称')} · ${esc(target.realName || '未填实名')}${
          target.contact ? ' · ' + esc(target.contact) : ''}</p>
      </div>
    </div>
    <div class="modal-body">
      <p class="admin-hint">${approve
        ? '通过后立刻生效：对方下一次请求就能提问与回答，不用重新登录。'
        : '原因会显示在对方的「我的」页面。写清"缺什么"——比如联系方式打不通、姓名与账号对不上。'}</p>
      <div class="field">
        <label for="speak-note">审核备注${approve ? '（选填）' : '（必填）'}</label>
        <textarea class="input code-input" id="speak-note" rows="3"
          placeholder="${approve ? '例如：已电话确认身份' : '例如：联系方式无法接通，请补充可验证的手机号'}"></textarea>
      </div>
      <div class="modal-actions">
        <button class="btn" type="button" data-modal-close>取消</button>
        <button class="btn ${approve ? 'btn-primary' : 'btn-danger'}" type="button" data-do-speak
          data-approve="${approve ? '1' : ''}">${approve ? '确认通过' : '确认驳回'}</button>
      </div>
    </div>
  `, (modal) => {
    const btn = modal.querySelector('[data-do-speak]');
    btn.addEventListener('click', async () => {
      const note = modal.querySelector('#speak-note').value.trim();
      if (!approve && !note) {
        toastError('请填写驳回原因');
        return;
      }
      btn.disabled = true;
      try {
        await api.adminSpeakReview(userId, { approve, note });
        closeModal();
        toastOk(approve ? '已通过，对方可以发言了' : '已驳回');
        window.__reloadPage();
      } catch (err) {
        btn.disabled = false;
        toastError(err.message || '操作失败');
      }
    });
  });
}

/* -------------------------------------------------------- 财务对账 -- */

const pad2 = (n) => String(n).padStart(2, '0');
const ymd = (d) => `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
const WEEK_CN = ['日', '一', '二', '三', '四', '五', '六'];

/** 金额卡。与概览的 statCard 不同：这里不可点、值是金额，fmtNum 会把分位吃掉。 */
function moneyCard(label, cents, hint, tone = '') {
  return `<div class="admin-stat ${tone}">
    <span class="admin-stat-label">${esc(label)}</span>
    <strong class="admin-stat-value">${esc(fmtCents(cents))}</strong>
    <span class="admin-stat-hint">${esc(hint)}</span>
  </div>`;
}

/** 查询条件栏。快捷区间只改日期，不改查询逻辑。 */
function financeFilterHTML(from, to) {
  return `
    <form class="toolbar admin-filters" data-finance-filter>
      <input class="input" type="date" name="from" aria-label="开始日期" value="${esc(from)}">
      <span class="muted-note">至</span>
      <input class="input" type="date" name="to" aria-label="结束日期" value="${esc(to)}">
      <button class="btn btn-sm" type="submit">${icon('search', 14)}查询</button>
      <button class="btn btn-sm btn-ghost" type="button" data-range="7">近 7 天</button>
      <button class="btn btn-sm btn-ghost" type="button" data-range="30">近 30 天</button>
      <button class="btn btn-sm btn-ghost" type="button" data-range="this-month">本月</button>
      <button class="btn btn-sm btn-ghost" type="button" data-range="last-month">上月</button>
    </form>`;
}

/**
 * 财务对账。
 *
 * 这张表的每一列都有自己的时间口径，页面上刻意**不给"当日利润"**这种合成指标：
 * 收款里含着代管中的专家款，结算只是把它挪进专家余额（不动现金），
 * 打款才是钱真正离开平台。所以只给一个定义明确的 NetCash（收款 − 退款 − 打款）。
 */
export async function adminFinancePage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const fromQ = ctx.params.get('from') || '';
  const toQ = ctx.params.get('to') || '';

  let data = null;
  let loadError = '';
  try {
    data = await api.adminFinanceReport({ from: fromQ, to: toQ });
  } catch (err) {
    loadError = err.message || '读取对账数据失败';
  }

  const effFrom = data ? data.from : fromQ;
  const effTo = data ? data.to : toQ;

  const body = loadError
    ? `${financeFilterHTML(effFrom, effTo)}
       <div class="card card-pad"><p class="admin-hint admin-hint-warn">${esc(loadError)}</p></div>`
    : `${financeFilterHTML(effFrom, effTo)}${financeReportHTML(data)}`;

  return {
    title: '财务对账',
    html: adminShell('/admin/finance', {
      title: '财务对账',
      desc: '收款、退款、结算、打款四个口径各按自己发生的那天归集。账面校验不平，说明某笔钱动错了地方。',
      actions: data
        ? `<button class="btn btn-sm" type="button" data-export>${icon('download', 14)}导出 CSV</button>`
        : '',
      body,
    }),
    mount: (root) => {
      const form = root.querySelector('[data-finance-filter]');
      if (form) {
        form.addEventListener('submit', (e) => {
          e.preventDefault();
          const fd = new FormData(form);
          // 留空即"用默认窗口"，不把空串写进地址栏
          setHashQuery({ from: String(fd.get('from') || ''), to: String(fd.get('to') || '') });
        });
      }

      root.querySelectorAll('[data-range]').forEach((btn) => {
        btn.addEventListener('click', () => {
          const kind = btn.getAttribute('data-range');
          const now = new Date();
          let from;
          let to;
          if (kind === 'this-month') {
            from = new Date(now.getFullYear(), now.getMonth(), 1);
            to = new Date(now.getFullYear(), now.getMonth() + 1, 0);
          } else if (kind === 'last-month') {
            from = new Date(now.getFullYear(), now.getMonth() - 1, 1);
            to = new Date(now.getFullYear(), now.getMonth(), 0);
          } else {
            const days = Number(kind) || 30;
            from = new Date(now.getTime() - (days - 1) * 86400000);
            to = now;
          }
          setHashQuery({ from: ymd(from), to: ymd(to) });
        });
      });

      const exportBtn = root.querySelector('[data-export]');
      if (exportBtn) {
        exportBtn.addEventListener('click', async () => {
          exportBtn.disabled = true;
          try {
            const name = await api.adminFinanceExport({ from: effFrom, to: effTo });
            toastOk(`已导出 ${name}`);
          } catch (err) {
            toastError(err.message || '导出失败');
          } finally {
            exportBtn.disabled = false;
          }
        });
      }

      root.addEventListener('click', (e) => {
        const btn = e.target.closest('[data-day]');
        if (btn) openFinanceDay(btn.getAttribute('data-day'));
      });
    },
  };
}

/** 报表主体：汇总 → 平台欠付 → 账面校验 → 日表。 */
function financeReportHTML(d) {
  const s = d.summary;
  const o = d.owed;
  const t = d.totals;

  const cards = `
    <div class="admin-stats admin-money-cards">
      ${moneyCard('期间收款', s.paidAmountCents, `${fmtNum(s.paidCount)} 笔 · 用户实付`)}
      ${moneyCard('期间退款', s.refundCents, `${fmtNum(s.refundCount)} 笔 · 按退款时间归集`, s.refundCents ? 'is-warn' : '')}
      ${moneyCard('期间提现打款', s.withdrawActualCents, `${fmtNum(s.withdrawCount)} 笔 · 钱已离开平台`)}
      ${moneyCard('期间净现金', s.netCashCents, '收款 − 退款 − 打款',
        s.netCashCents < 0 ? 'is-danger' : (s.netCashCents > 0 ? 'is-ok' : ''))}
    </div>`;

  const owed = `
    <section class="card card-pad">
      <div class="admin-sec-head">
        <h2>平台欠付（按当前时点）</h2>
        <span class="muted-note">不计入上面的区间统计</span>
      </div>
      <div class="admin-stats">
        ${moneyCard('代管中', o.escrowCents, `${fmtNum(o.escrowCount)} 笔已收款未结算`)}
        ${moneyCard('专家可提', o.expertBalanceCents, `${fmtNum(o.expertCount)} 位专家余额合计`)}
        ${moneyCard('提现冻结', o.expertFrozenCents, '申请中，钱已扣但还没打出去')}
        ${moneyCard('处理中提现', o.pendingWithdrawCents, '待审核 + 待打款')}
      </div>
      <p class="admin-hint">
        全时段累计：收款 ${esc(fmtCents(t.paidAmountCents))}、退款 ${esc(fmtCents(t.refundCents))}、
        已打款 ${esc(fmtCents(t.withdrawActualCents))}、已结算佣金 ${esc(fmtCents(t.commissionCents))}，
        累计净现金 <strong>${esc(fmtCents(t.netCashCents))}</strong>。
      </p>
    </section>`;

  const badCount = d.checks.filter((c) => !c.ok && c.level !== 'warn').length;
  const warnCount = d.checks.filter((c) => !c.ok && c.level === 'warn').length;
  const checks = `
    <section class="card card-pad">
      <div class="admin-sec-head">
        <h2>账面校验</h2>
        <span class="muted-note">${badCount
          ? `<span class="is-danger">${badCount} 项不平</span>`
          : (warnCount ? `${warnCount} 项数据不全` : '全部通过')}</span>
      </div>
      <div class="admin-checks">
        ${d.checks.map((c) => {
          const tone = c.ok ? '' : (c.level === 'warn' ? 'is-warn' : 'is-bad');
          const samples = c.samples && c.samples.length
            ? `<details class="admin-check-samples">
                 <summary>查看样本（${c.samples.length} 条）</summary>
                 <ul>${c.samples.map((x) => `<li>${esc(x)}</li>`).join('')}</ul>
               </details>`
            : '';
          return `<div class="admin-check-item ${tone}">
            <span class="admin-check-mark">${c.ok ? '✓' : '!'}</span>
            <div>
              <div class="admin-check-title">${esc(c.name)}</div>
              <div class="admin-check-msg">${esc(c.message)}</div>
              ${samples}
            </div>
          </div>`;
        }).join('')}
      </div>
      <p class="admin-hint">
        这些不是"提醒"，是恒等式：佣金 + 专家实收必须等于订单总额，累计收入必须等于可提余额 + 冻结 + 已打款。
        不平就去订单页与提现页逐笔核对，不要在这里改数——报表是只读的，能改的只有真实记录。
      </p>
    </section>`;

  const rows = d.rows.map((r) => {
    const day = new Date(`${r.day}T00:00:00`);
    const netTone = r.netCashCents < 0 ? 'is-neg' : (r.netCashCents > 0 ? 'is-pos' : '');
    const empty = !r.paidCount && !r.refundCount && !r.settleCount && !r.withdrawCount;
    return `<tr class="${empty ? 'is-muted' : ''}">
      <td>${esc(r.day.slice(5))}<span class="admin-cell-sub">周${WEEK_CN[day.getDay()]}</span></td>
      <td class="num">${r.paidCount ? `${esc(fmtCents(r.paidAmountCents))}<span class="admin-cell-sub">${fmtNum(r.paidCount)} 笔</span>` : '—'}</td>
      <td class="num">${r.refundCents ? esc(fmtCents(r.refundCents)) : '—'}</td>
      <td class="num">${r.settledCommissionCents ? esc(fmtCents(r.settledCommissionCents)) : '—'}</td>
      <td class="num">${r.settledExpertCents ? esc(fmtCents(r.settledExpertCents)) : '—'}</td>
      <td class="num">${r.withdrawActualCents ? `${esc(fmtCents(r.withdrawActualCents))}<span class="admin-cell-sub">${fmtNum(r.withdrawCount)} 笔</span>` : '—'}</td>
      <td class="num ${netTone}">${empty ? '—' : esc(fmtCents(r.netCashCents))}</td>
      <td class="admin-ops">${empty ? '' : `<button class="btn btn-sm" type="button" data-day="${esc(r.day)}">明细</button>`}</td>
    </tr>`;
  }).join('');

  const table = `
    <section class="card admin-table-wrap">
      <table class="admin-table admin-day-table">
        <thead>
          <tr>
            <th>日期</th>
            <th class="num">收款</th>
            <th class="num">退款</th>
            <th class="num">已结算佣金</th>
            <th class="num">专家实收</th>
            <th class="num">提现打款</th>
            <th class="num">净现金</th>
            <th class="admin-ops">明细</th>
          </tr>
        </thead>
        <tbody>${rows}</tbody>
        <tfoot>
          <tr>
            <th>合计</th>
            <th class="num">${esc(fmtCents(s.paidAmountCents))}</th>
            <th class="num">${esc(fmtCents(s.refundCents))}</th>
            <th class="num">${esc(fmtCents(s.settledCommissionCents))}</th>
            <th class="num">${esc(fmtCents(s.settledExpertCents))}</th>
            <th class="num">${esc(fmtCents(s.withdrawActualCents))}</th>
            <th class="num">${esc(fmtCents(s.netCashCents))}</th>
            <th></th>
          </tr>
        </tfoot>
      </table>
    </section>`;

  return `${cards}
    <p class="admin-range-hint">
      ${esc(d.from)} 至 ${esc(d.to)}，共 ${fmtNum(s.days)} 天。
      「收款」按支付时间、「退款」按退款时间、「结算」按分账落账时间、「打款」按打款时间各自归日，
      所以同一天的四列本来就不该相等。
    </p>
    ${owed}
    ${checks}
    ${table}`;
}

/** 单日明细：当天收款的订单、结算的分账、打款的提现。 */
async function openFinanceDay(day) {
  let d;
  try {
    d = await api.adminFinanceDay(day);
  } catch (err) {
    toastError(err.message || '读取当日明细失败');
    return;
  }

  const orders = d.orders.length
    ? `<div class="admin-modal-table">
        <table class="admin-table">
          <thead><tr>
            <th>订单</th><th>状态</th><th class="num">总额</th><th class="num">佣金</th>
            <th class="num">专家</th><th class="num">退款</th><th>时间</th>
          </tr></thead>
          <tbody>${d.orders.map((o) => `<tr>
            <td>${esc(o.orderNo)}<span class="admin-cell-sub">${esc(o.title || '')}</span></td>
            <td>${esc(ORDER_STATUS[o.status]?.label || o.status)}</td>
            <td class="num">${esc(fmtCents(o.amountCents))}</td>
            <td class="num">${esc(fmtCents(o.commissionCents))}</td>
            <td class="num">${esc(fmtCents(o.expertAmountCents))}</td>
            <td class="num">${o.refundCents ? esc(fmtCents(o.refundCents)) : '—'}</td>
            <td>${o.paidAt ? '付 ' + esc(fmtDateTime(o.paidAt)) : ''}${o.refundedAt ? '<br>退 ' + esc(fmtDateTime(o.refundedAt)) : ''}</td>
          </tr>`).join('')}</tbody>
        </table>
      </div>`
    : '<p class="muted-note">当日没有收款或退款。</p>';

  const settlements = d.settlements.length
    ? `<div class="admin-modal-table">
        <table class="admin-table">
          <thead><tr>
            <th>分账单</th><th>订单</th><th class="num">总额</th><th class="num">佣金</th><th class="num">专家实收</th>
          </tr></thead>
          <tbody>${d.settlements.map((x) => `<tr>
            <td>${esc(x.settlementNo)}</td>
            <td>${esc(x.orderNo)}</td>
            <td class="num">${esc(fmtCents(x.amountCents))}</td>
            <td class="num">${esc(fmtCents(x.commissionCents))}</td>
            <td class="num">${esc(fmtCents(x.expertAmountCents))}</td>
          </tr>`).join('')}</tbody>
        </table>
      </div>`
    : '<p class="muted-note">当日没有结算。</p>';

  const withdrawals = d.withdrawals.length
    ? `<div class="admin-modal-table">
        <table class="admin-table">
          <thead><tr>
            <th>提现单</th><th>收款人</th><th>账号</th><th class="num">申请</th>
            <th class="num">手续费</th><th class="num">实付</th>
          </tr></thead>
          <tbody>${d.withdrawals.map((w) => `<tr>
            <td>${esc(w.withdrawalNo)}</td>
            <td>${esc(w.accountName || '—')}</td>
            <td>${esc(w.accountNoMasked || '—')}</td>
            <td class="num">${esc(fmtCents(w.amountCents))}</td>
            <td class="num">${esc(fmtCents(w.feeCents))}</td>
            <td class="num">${esc(fmtCents(w.actualCents))}</td>
          </tr>`).join('')}</tbody>
        </table>
      </div>`
    : '<p class="muted-note">当日没有提现打款。</p>';

  openModal(`
    <div class="modal-head">
      <div>
        <h2>${esc(day)} 明细</h2>
        <p class="modal-sub">收款 ${d.orders.length} 笔 · 结算 ${d.settlements.length} 笔 · 打款 ${d.withdrawals.length} 笔</p>
      </div>
    </div>
    <div class="modal-body">
      <section class="admin-detail-block"><h3>收款与退款</h3>${orders}</section>
      <section class="admin-detail-block"><h3>当日结算</h3>${settlements}</section>
      <section class="admin-detail-block"><h3>当日打款</h3>${withdrawals}</section>
      <div class="modal-actions">
        <button class="btn" type="button" data-modal-close>关闭</button>
      </div>
    </div>
  `);
}

/* --------------------------------------------------------- 系统配置 -- */

/**
 * 系统配置页。
 *
 * 三块内容放在同一页，是因为它们都属于「运营自己就能改、不需要发版」的东西：
 * 抽佣比例决定每一笔新订单怎么分账，分类决定内容归档，技能决定专家能选什么标签。
 *
 * 抽佣的说明文字是刻意写死的：后台改比例只影响**之后创建**的订单，
 * 已下的单按各自快照结算。运营最容易误解的一点就是"改了比例，
 * 之前那些单是不是也按新比例重算"——这里必须一句话说清。
 */
export async function adminSettingsPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  let overview = null;
  let categories = [];
  let skills = [];
  let loadError = '';
  try {
    overview = await api.adminSettings();
  } catch (err) {
    loadError = err.message || '读取系统配置失败';
  }
  if (!loadError) {
    // 分类与技能是附属两块，任一失败不影响抽佣卡片展示：
    // 抽佣才是这一页真正危险的操作，不能因为它俩挂了而看不到。
    [categories, skills] = await Promise.all([
      api.adminCategories().catch(() => []),
      api.adminSkills().catch(() => []),
    ]);
  }

  const body = loadError
    ? `<div class="card card-pad"><p class="admin-hint admin-hint-warn">${esc(loadError)}</p></div>`
    : `${commissionBlock(overview.commission)}${categoriesBlock(categories)}${skillsBlock(skills)}`;

  return {
    title: '系统配置',
    html: adminShell('/admin/settings', {
      title: '系统配置',
      desc: '抽佣比例改完立即对之后创建的订单生效；分类与技能是内容归档与专家标签的字典。',
      body,
    }),
    mount: (root) => {
      root.addEventListener('click', (e) => {
        const btn = e.target.closest('button');
        if (!btn) return;

        if (btn.hasAttribute('data-edit-commission')) {
          openCommissionModal(overview.commission);
          return;
        }
        if (btn.hasAttribute('data-new-cat')) { openCategoryModal(null); return; }
        if (btn.hasAttribute('data-new-skill')) { openSkillModal(null, skills); return; }

        const catId = Number(btn.getAttribute('data-edit-cat') || 0);
        if (catId) {
          openCategoryModal(categories.find((c) => Number(c.id) === catId) || null);
          return;
        }
        const delCatId = Number(btn.getAttribute('data-del-cat') || 0);
        if (delCatId) {
          const cat = categories.find((c) => Number(c.id) === delCatId);
          openDeleteCategoryModal(cat);
          return;
        }

        const skillId = Number(btn.getAttribute('data-edit-skill') || 0);
        if (skillId) {
          openSkillModal(skills.find((s) => Number(s.id) === skillId) || null, skills);
          return;
        }
        const toggleId = Number(btn.getAttribute('data-toggle-skill') || 0);
        if (toggleId) {
          const row = skills.find((s) => Number(s.id) === toggleId);
          if (row) toggleSkill(row);
          return;
        }
        const delSkillId = Number(btn.getAttribute('data-del-skill') || 0);
        if (delSkillId) {
          const row = skills.find((s) => Number(s.id) === delSkillId);
          openDeleteSkillModal(row);
        }
      });
    },
  };
}

/** 基点转百分比展示：1500 → "15"，1550 → "15.5"，1555 → "15.55"。 */
function bpToPct(bp) {
  return String(Math.round(Number(bp) || 0) / 100);
}

function commissionBlock(c) {
  return `
    <section class="card card-pad setting-rate-card">
      <div class="admin-sec-head">
        <h2>平台抽佣</h2>
        <span class="muted-note">${c.isOverride ? '后台已调整' : '等于配置文件默认值'}</span>
      </div>
      <div class="setting-rate">
        <div class="setting-rate-num">${esc(bpToPct(c.rateBp))}<span>%</span></div>
        <div class="setting-rate-meta">
          <p>${c.isOverride
            ? `最后修改：${esc(fmtDateTime(c.updatedAt))}${c.updatedBy ? ` · 操作人 #${esc(String(c.updatedBy))}` : ''}`
            : `出厂默认：${esc(bpToPct(c.defaultRateBp))}%`}</p>
          <p class="muted-note">只作用于之后创建的订单，已下单按各自快照分账，不会被追溯改账。
            每笔订单的佣金 + 专家实收 = 订单总额，这个恒等式由服务端强制。</p>
        </div>
        <button class="btn" type="button" data-edit-commission>调整比例</button>
      </div>
    </section>`;
}

function categoriesBlock(list) {
  const rows = list.map((c) => `<tr>
      <td>
        <div class="admin-cell-title">${esc(c.name)}</div>
        <div class="admin-cell-sub">${esc(c.slug)}</div>
      </td>
      <td class="num">${Number(c.sort) || 0}</td>
      <td class="num">${fmtNum(c.articleCount || 0)}</td>
      <td>${c.enabled ? '<span class="chip chip-ok">启用</span>' : '<span class="chip">停用</span>'}</td>
      <td class="admin-ops">
        <button class="btn btn-sm" type="button" data-edit-cat="${c.id}">编辑</button>
        <button class="btn btn-sm btn-danger" type="button" data-del-cat="${c.id}">删除</button>
      </td>
    </tr>`).join('');

  return `
    <section class="card card-pad">
      <div class="admin-sec-head">
        <h2>内容分类</h2>
        <button class="btn btn-sm" type="button" data-new-cat>+ 新建分类</button>
      </div>
      <p class="admin-hint">分类是全站共用的：文章、工具、题库、资源都挂在同一张分类表上。
        URL 标识（slug）是外部契约——改展示名可以，别改已有分类的 slug，收藏与搜索引擎都在用。</p>
      <div class="admin-table-wrap">
        <table class="admin-table">
          <thead><tr><th>名称 / slug</th><th class="num">排序</th><th class="num">文章数</th><th>状态</th><th class="admin-ops">操作</th></tr></thead>
          <tbody>${rows || '<tr><td colspan="5"><p class="muted-note">还没有分类。</p></td></tr>'}</tbody>
        </table>
      </div>
    </section>`;
}

function skillsBlock(list) {
  if (!list.length) {
    return `
      <section class="card card-pad">
        <div class="admin-sec-head">
          <h2>技能字典</h2>
          <button class="btn btn-sm" type="button" data-new-skill>+ 新增技能</button>
        </div>
        <p class="muted-note">字典还是空的。没有技能，专家入驻时无法选择技术方向。</p>
      </section>`;
  }

  // 按类目分组，组内保持服务端给的 sort 顺序。
  const groups = new Map();
  list.forEach((s) => {
    const key = s.category || '未分类';
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(s);
  });

  const html = Array.from(groups.entries()).map(([cat, rows]) => `
    <div class="setting-group">
      <div class="setting-group-head">
        <h3>${esc(cat)}</h3>
        <span class="muted-note">${rows.length} 个技能</span>
      </div>
      <div class="admin-table-wrap">
        <table class="admin-table">
          <thead><tr><th>技能</th><th class="num">排序</th><th class="num">引用</th><th>状态</th><th class="admin-ops">操作</th></tr></thead>
          <tbody>${rows.map((s) => `<tr>
            <td><div class="admin-cell-title">${esc(s.name)}</div></td>
            <td class="num">${Number(s.sort) || 0}</td>
            <td class="num">${fmtNum(s.expertCount || 0)}</td>
            <td>${s.enabled ? '<span class="chip chip-ok">启用</span>' : '<span class="chip">停用</span>'}</td>
            <td class="admin-ops">
              <button class="btn btn-sm" type="button" data-edit-skill="${s.id}">编辑</button>
              <button class="btn btn-sm" type="button" data-toggle-skill="${s.id}">${s.enabled ? '停用' : '启用'}</button>
              <button class="btn btn-sm btn-danger" type="button" data-del-skill="${s.id}">删除</button>
            </td>
          </tr>`).join('')}</tbody>
        </table>
      </div>
    </div>`).join('');

  return `
    <section class="card card-pad">
      <div class="admin-sec-head">
        <h2>技能字典</h2>
        <button class="btn btn-sm" type="button" data-new-skill>+ 新增技能</button>
      </div>
      <p class="admin-hint">「引用」是被多少位专家的档案使用。被引用的技能不能删——删掉会让专家页与搜索出现悬空 ID；
        下架请用停用，停用只是不再出现在入驻下拉框里，已有档案照常展示。</p>
      ${html}
    </section>`;
}

/** 调整抽佣比例。输入按百分比，提交前换算成基点。 */
function openCommissionModal(cur) {
  openModal(`
    <div class="modal-head">
      <div>
        <h2>调整抽佣比例</h2>
        <p class="modal-sub">当前 ${esc(bpToPct(cur.rateBp))}%${cur.isOverride ? '（后台设置）' : '（配置文件默认）'}</p>
      </div>
    </div>
    <div class="modal-body">
      <form class="admin-form" data-commission-form>
        <div class="field">
          <label for="set-rate">抽佣比例（%）</label>
          <input class="input" id="set-rate" name="percent" type="number"
                 min="0" max="50" step="0.01" value="${esc(bpToPct(cur.rateBp))}" required>
          <p class="muted-note">范围 0-50%，最多两位小数。15 表示平台抽 15%，专家拿 85%。</p>
        </div>
        <p class="admin-hint">保存后立即生效，只影响之后创建的订单。已下单仍按各自快照结算。</p>
        <div class="modal-actions">
          <button class="btn" type="button" data-modal-close>取消</button>
          <button class="btn btn-primary" type="submit">保存</button>
        </div>
      </form>
    </div>
  `, (modal) => {
    const form = modal.querySelector('[data-commission-form]');
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const pct = Number(form.elements.percent.value);
      if (!Number.isFinite(pct) || pct < 0 || pct > 50) {
        toastError('抽佣比例需在 0 到 50 之间');
        return;
      }
      // 百分比转基点必须取整：金额计算不接受浮点，
      // 否则 15.55% 会变成 1555.0000000000002 这种值进库。
      const bp = Math.round(pct * 100);
      const submit = form.querySelector('button[type="submit"]');
      submit.disabled = true;
      try {
        await api.adminCommissionRate(bp);
        closeModal();
        toastOk(`抽佣比例已改为 ${bpToPct(bp)}%`);
        window.__reloadPage();
      } catch (err) {
        submit.disabled = false;
        toastError(err.message || '保存失败');
      }
    });
  });
}

/** 新建 / 编辑分类。slug 在编辑时必须原样回传，留空会被服务端按名称重算。 */
function openCategoryModal(cat) {
  const isNew = !cat;
  openModal(`
    <div class="modal-head">
      <div>
        <h2>${isNew ? '新建分类' : '编辑分类'}</h2>
        <p class="modal-sub">${isNew ? '分类会出现在知识库导航与筛选项里' : `#${esc(String(cat.id))}`}</p>
      </div>
    </div>
    <div class="modal-body">
      <form class="admin-form" data-cat-form>
        <div class="field">
          <label for="cat-name">名称</label>
          <input class="input" id="cat-name" name="name" maxlength="64" required
                 value="${esc(cat ? cat.name : '')}" placeholder="例如：云原生">
        </div>
        <div class="field">
          <label for="cat-slug">URL 标识（slug）</label>
          <input class="input" id="cat-slug" name="slug" maxlength="64"
                 value="${esc(cat ? cat.slug : '')}" placeholder="留空则按名称自动生成">
          <p class="muted-note">${isNew
            ? '只允许字母、数字、汉字与连字符。生成后尽量不要再改。'
            : '这是外部契约：改掉会让旧链接与收藏失效。'}</p>
        </div>
        <div class="field-row">
          <div class="field">
            <label for="cat-sort">排序</label>
            <input class="input" id="cat-sort" name="sort" type="number" value="${Number(cat && cat.sort) || 0}">
          </div>
          <div class="field">
            <label for="cat-enabled">状态</label>
            <select class="select" id="cat-enabled" name="enabled">
              <option value="1" ${!cat || cat.enabled ? 'selected' : ''}>启用</option>
              <option value="0" ${cat && !cat.enabled ? 'selected' : ''}>停用</option>
            </select>
          </div>
        </div>
        <div class="modal-actions">
          <button class="btn" type="button" data-modal-close>取消</button>
          <button class="btn btn-primary" type="submit">${isNew ? '创建' : '保存'}</button>
        </div>
      </form>
    </div>
  `, (modal) => {
    const form = modal.querySelector('[data-cat-form]');
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const fd = new FormData(form);
      const payload = {
        name: String(fd.get('name') || '').trim(),
        slug: String(fd.get('slug') || '').trim(),
        sort: Number(fd.get('sort')) || 0,
        enabled: String(fd.get('enabled')) === '1',
      };
      // 新建时留空是"按名称生成"；编辑时留空会把 slug 重算一遍，
      // 等于悄悄改掉对外地址，所以旧值必须带上。
      if (!payload.slug && cat) payload.slug = cat.slug;
      const submit = form.querySelector('button[type="submit"]');
      submit.disabled = true;
      try {
        if (isNew) await api.createCategory(payload);
        else await api.updateCategory(cat.id, payload);
        closeModal();
        toastOk(isNew ? '分类已创建' : '分类已保存');
        window.__reloadPage();
      } catch (err) {
        submit.disabled = false;
        toastError(err.message || '保存失败');
      }
    });
  });
}

function openDeleteCategoryModal(cat) {
  if (!cat) return;
  const used = Number(cat.articleCount) > 0;
  openModal(`
    <div class="modal-head"><h2>删除分类</h2></div>
    <div class="modal-body">
      <p>确定删除「${esc(cat.name)}」吗？</p>
      ${used
        ? `<p class="admin-hint admin-hint-warn">该分类下还有 ${fmtNum(cat.articleCount)} 篇文章。
            服务端会拒绝删除——请先把文章移出或改到别的分类。</p>`
        : '<p class="admin-hint">删除后不可恢复。</p>'}
      <div class="modal-actions">
        <button class="btn" type="button" data-modal-close>取消</button>
        <button class="btn btn-danger" type="button" data-do-del ${used ? 'disabled' : ''}>确认删除</button>
      </div>
    </div>
  `, (modal) => {
    const btn = modal.querySelector('[data-do-del]');
    if (!btn || used) return;
    btn.addEventListener('click', async () => {
      btn.disabled = true;
      try {
        await api.deleteCategory(cat.id);
        closeModal();
        toastOk('分类已删除');
        window.__reloadPage();
      } catch (err) {
        btn.disabled = false;
        toastError(err.message || '删除失败');
      }
    });
  });
}

/** 新建 / 编辑技能。类目用 datalist 提供已有值，但不限制只能选已有的。 */
function openSkillModal(skill, all) {
  const isNew = !skill;
  const cats = Array.from(new Set(all.map((s) => s.category).filter(Boolean)));
  openModal(`
    <div class="modal-head">
      <div>
        <h2>${isNew ? '新增技能' : '编辑技能'}</h2>
        <p class="modal-sub">技能会出现在专家入驻与筛选里</p>
      </div>
    </div>
    <div class="modal-body">
      <form class="admin-form" data-skill-form>
        <div class="field">
          <label for="sk-name">技能名称</label>
          <input class="input" id="sk-name" name="name" maxlength="32" required
                 value="${esc(skill ? skill.name : '')}" placeholder="例如：etcd">
        </div>
        <div class="field">
          <label for="sk-cat">类目</label>
          <input class="input" id="sk-cat" name="category" maxlength="32" required list="sk-cats"
                 value="${esc(skill ? skill.category : '')}" placeholder="例如：云原生">
          <datalist id="sk-cats">${cats.map((c) => `<option value="${esc(c)}"></option>`).join('')}</datalist>
        </div>
        <div class="field-row">
          <div class="field">
            <label for="sk-sort">排序</label>
            <input class="input" id="sk-sort" name="sort" type="number" value="${Number(skill && skill.sort) || 0}">
          </div>
          <div class="field">
            <label for="sk-enabled">状态</label>
            <select class="select" id="sk-enabled" name="enabled">
              <option value="1" ${!skill || skill.enabled ? 'selected' : ''}>启用</option>
              <option value="0" ${skill && !skill.enabled ? 'selected' : ''}>停用</option>
            </select>
          </div>
        </div>
        <div class="modal-actions">
          <button class="btn" type="button" data-modal-close>取消</button>
          <button class="btn btn-primary" type="submit">${isNew ? '创建' : '保存'}</button>
        </div>
      </form>
    </div>
  `, (modal) => {
    const form = modal.querySelector('[data-skill-form]');
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const fd = new FormData(form);
      const payload = {
        name: String(fd.get('name') || '').trim(),
        category: String(fd.get('category') || '').trim(),
        sort: Number(fd.get('sort')) || 0,
        enabled: String(fd.get('enabled')) === '1',
      };
      const submit = form.querySelector('button[type="submit"]');
      submit.disabled = true;
      try {
        if (isNew) await api.createSkill(payload);
        else await api.updateSkill(skill.id, payload);
        closeModal();
        toastOk(isNew ? '技能已新增' : '技能已保存');
        window.__reloadPage();
      } catch (err) {
        submit.disabled = false;
        toastError(err.message || '保存失败');
      }
    });
  });
}

/** 停用 / 启用技能。停用不碰已有档案，是「下架」的安全做法。 */
async function toggleSkill(row) {
  if (!row) return;
  const next = !row.enabled;
  try {
    await api.updateSkill(row.id, { enabled: next });
    toastOk(next ? '技能已启用' : '技能已停用');
    window.__reloadPage();
  } catch (err) {
    toastError(err.message || '操作失败');
  }
}

function openDeleteSkillModal(row) {
  if (!row) return;
  const used = Number(row.expertCount) > 0;
  openModal(`
    <div class="modal-head"><h2>删除技能</h2></div>
    <div class="modal-body">
      <p>确定删除「${esc(row.name)}」吗？</p>
      ${used
        ? `<p class="admin-hint admin-hint-warn">已有 ${fmtNum(row.expertCount)} 位专家在档案里引用了它，服务端会拒绝删除。
            如果只是不想让它再出现在入驻下拉框，请改用「停用」。</p>`
        : '<p class="admin-hint">删除后不可恢复。若只是暂时不用，停用更稳妥。</p>'}
      <div class="modal-actions">
        <button class="btn" type="button" data-modal-close>取消</button>
        <button class="btn btn-danger" type="button" data-do-del ${used ? 'disabled' : ''}>确认删除</button>
      </div>
    </div>
  `, (modal) => {
    const btn = modal.querySelector('[data-do-del]');
    if (!btn || used) return;
    btn.addEventListener('click', async () => {
      btn.disabled = true;
      try {
        await api.deleteSkill(row.id);
        closeModal();
        toastOk('技能已删除');
        window.__reloadPage();
      } catch (err) {
        btn.disabled = false;
        toastError(err.message || '删除失败');
      }
    });
  });
}

/* --------------------------------------------------------- 问答运营 -- */

/**
 * 问答后台：待审回答 + 处理入口。
 *
 * 只做三件事，别的都不放：待审回答的通过/驳回、删问题、把优质问答转教程草稿。
 * 为什么不在后台再做一个"问答列表管理"：问答的日常运营动作是审核与精选，
 * 而不是逐条编辑别人的提问——那属于社区治理，不该由后台表格来做。
 */
export async function adminQaPage(ctx) {
  const gate = adminGate();
  if (gate) return gate;

  const page = Math.max(1, Number(ctx.params.get('page')) || 1);
  const pageSize = 20;
  const data = await api.adminQaPending({ page, pageSize });
  const list = Array.isArray(data.list) ? data.list : [];

  const rows = list.map((a) => `<tr>
      <td>
        <div class="admin-cell-title qa-admin-text">${esc((a.content || '').slice(0, 160))}${(a.content || '').length > 160 ? '…' : ''}</div>
        <div class="admin-cell-sub">回答 #${a.id}</div>
      </td>
      <td>
        <div class="admin-cell-title">${esc(a.nickname || '未设昵称')}</div>
        <div class="admin-cell-sub">#${a.userId} · ${esc(a.badge || '新手')}${
          a.authorRole === 'expert' ? ' · 专家' : ''}</div>
      </td>
      <td>
        <a href="#/qa/${a.questionId}" class="admin-link">${esc(a.questionTitle || `问题 #${a.questionId}`)}</a>
      </td>
      <td>${esc(fmtDateTime(a.createdAt))}</td>
      <td class="admin-ops">
        <button class="btn btn-sm btn-primary" type="button" data-qa-approve="${a.id}">通过</button>
        <button class="btn btn-sm btn-danger" type="button" data-qa-reject="${a.id}">驳回</button>
      </td>
    </tr>`).join('');

  const body = `
    <p class="admin-hint">
      只有新账号的前 3 条回答与命中敏感词的内容会进这里。专家与管理员的回答免审直发。
      审核通过后才会出现在问题详情页，并把问题顶到列表前面。
    </p>
    <div class="card admin-table-wrap">
      <table class="admin-table">
        <thead><tr><th>回答</th><th>作者</th><th>所属问题</th><th>提交时间</th><th class="admin-ops">操作</th></tr></thead>
        <tbody>${rows || '<tr><td colspan="5"><p class="muted-note">没有待审回答。审核队列是空的，说明社区目前没有可疑内容。</p></td></tr>'}</tbody>
      </table>
    </div>
    ${pagerHTML({ page, pageSize, total: data.total || 0, unit: '条回答' })}
  `;

  return {
    title: '问答运营',
    html: adminShell('/admin/qa', {
      title: '问答运营',
      desc: '待审回答的处理队列。通过即对所有人可见；驳回必须写原因，作者能看到。',
      body,
    }),
    mount: (root) => {
      bindPager(root, (next) => setHashQuery({ page: next }, { keepPage: true }));

      root.addEventListener('click', async (e) => {
        const approve = e.target.closest('[data-qa-approve]');
        if (approve) {
          const id = approve.getAttribute('data-qa-approve');
          approve.disabled = true;
          try {
            await api.adminQaReview(id, { action: 'approve', note: '' });
            toastOk('已通过');
            window.__reloadPage();
          } catch (err) {
            approve.disabled = false;
            toastError(err.message || '操作失败');
          }
          return;
        }

        const reject = e.target.closest('[data-qa-reject]');
        if (reject) openQaRejectModal(reject.getAttribute('data-qa-reject'));
      });
    },
  };
}

/** 驳回必须写原因：不写原因，作者只会看到内容消失然后原样重发。 */
function openQaRejectModal(id) {
  openModal(`
    <div class="modal-head"><h2>驳回回答</h2></div>
    <div class="modal-body">
      <form class="admin-form" data-qa-reject-form>
        <div class="field">
          <label for="qa-note">驳回原因</label>
          <textarea class="input" id="qa-note" name="note" rows="4" maxlength="500" required
            placeholder="例如：只给了结论没有排查步骤，读者无法复现"></textarea>
          <p class="muted-note">原因会显示给回答者，不会公开。</p>
        </div>
        <div class="modal-actions">
          <button class="btn" type="button" data-modal-close>取消</button>
          <button class="btn btn-danger" type="submit">确认驳回</button>
        </div>
      </form>
    </div>
  `, (modal) => {
    const form = modal.querySelector('[data-qa-reject-form]');
    form.addEventListener('submit', async (ev) => {
      ev.preventDefault();
      const note = form.elements.note.value.trim();
      if (!note) { toastError('请填写驳回原因'); return; }
      const submit = form.querySelector('button[type="submit"]');
      submit.disabled = true;
      try {
        await api.adminQaReview(id, { action: 'reject', note });
        closeModal();
        toastOk('已驳回');
        window.__reloadPage();
      } catch (err) {
        submit.disabled = false;
        toastError(err.message || '操作失败');
      }
    });
  });
}
