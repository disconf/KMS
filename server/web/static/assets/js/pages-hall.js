/* ============================================================================
   pages-hall.js — 接单大厅（需要已通过审核的专家）。

   大厅里每一条都是「一笔待接单的订单」，所以列表项带 orderNo：
   抢单接口按订单号走，而不是按问题 ID（见 dto.HallItem 的注释）。

   抢单是典型的并发场景：同一单可能被多人同时点。服务端用 CAS
   （UPDATE ... WHERE status='pending_accept' AND expert_id=0）判定谁赢，
   输的人拿到「订单已被接走」。前端在这里**不做任何预判**——
   预先置灰只会让人以为按钮坏了，点一下拿到明确结果反而更清楚。
   ========================================================================= */

import { api, isLoggedIn, ApiError } from './api.js';
import { esc, fmtDateTime, fmtNum, icon, setHashQuery } from './util.js';
import { bindPager, emptyHTML, pagerHTML, toastError, toastOk } from './ui.js';
import { fmtCents } from './orders.js';

const PAGE_SIZE = 10;

const SORTS = [
  { key: 'latest', label: '最新发布' },
  { key: 'amount', label: '悬赏最高' },
  { key: 'expiring', label: '即将到期' },
];

export async function hallPage(ctx) {
  const params = ctx.params;
  const page = Math.max(1, Number(params.get('page') || 1));
  const sort = SORTS.some((s) => s.key === params.get('sort')) ? params.get('sort') : 'latest';
  const skillId = params.get('skillId') || '';

  if (!isLoggedIn()) {
    return {
      html: `<div class="page-head">
        <h1>接单大厅</h1>
        <p class="lede">这里是被发布出来的悬赏，接单需要先通过专家认证。</p>
      </div>` + emptyHTML({
        iconName: 'shield',
        title: '登录后查看接单大厅',
        desc: '大厅只对通过审核的专家开放。',
        actionHTML: '<button class="btn btn-primary" type="button" data-act="login">去登录</button>',
      }),
      title: '接单大厅',
    };
  }

  // 技能筛选与列表并行取：筛选条拿不到时也要能把列表显示出来
  const [skills, result] = await Promise.all([
    api.skills().catch(() => []),
    api.hall({ page, pageSize: PAGE_SIZE, sort, skillId: skillId || undefined }).catch((err) => {
      // 未通过审核的专家会拿到 300xx 的错误，这时不该整页报错，
      // 而应说明"还没通过认证"——因为大厅本来就不是给普通用户看的
      if (err instanceof ApiError && /专家/.test(err.message)) {
        return { __denied: err.message, list: [], total: 0 };
      }
      throw err;
    }),
  ]);

  if (result.__denied) {
    return {
      html: `<div class="page-head">
        <h1>接单大厅</h1>
        <p class="lede">接单需要先通过专家认证。</p>
      </div>` + emptyHTML({
        iconName: 'shield',
        title: '还不是认证专家',
        desc: result.__denied,
        actionHTML: '<a class="btn btn-primary" href="#/orders">去看我的订单</a>',
      }),
      title: '接单大厅',
    };
  }

  const list = result.list || [];
  const total = result.total || 0;

  const html = `
    <div class="page-head">
      <h1>接单大厅</h1>
      <p class="lede">按技术方向筛选，接单后请在支付截止前提醒用户完成支付；
        超时未支付订单会自动关闭。</p>
    </div>

    <div class="toolbar">
      <select class="select" data-filter="skillId" aria-label="按技术方向筛选">
        <option value="">全部技术方向</option>
        ${(skills || []).map((s) => `<option value="${esc(String(s.id))}"
          ${String(s.id) === skillId ? 'selected' : ''}>${esc(s.name)}</option>`).join('')}
      </select>

      <div class="seg" role="group" aria-label="排序">
        ${SORTS.map((s) => `<button type="button" data-sort="${s.key}"
          class="${s.key === sort ? 'is-active' : ''}">${s.label}</button>`).join('')}
      </div>

      <span class="toolbar-spacer"></span>
      <span style="font-size:12.5px;color:var(--text-3)">${fmtNum(total)} 条待接单</span>
    </div>

    <div class="hall-list">
      ${list.length
        ? list.map((item) => hallCardHTML(item, skills)).join('')
        : emptyHTML({
            iconName: 'target',
            title: '大厅暂时是空的',
            desc: '有新悬赏发布时会出现在这里。换个技术方向筛选也可能有收获。',
            actionHTML: skillId
              ? '<button class="btn btn-sm" type="button" data-clear>清空筛选</button>' : '',
          })}
    </div>

    ${pagerHTML({ page, pageSize: PAGE_SIZE, total, unit: '条' })}
  `;

  const mount = (root) => {
    root.querySelectorAll('[data-sort]').forEach((btn) => {
      btn.addEventListener('click', () => setHashQuery({ sort: btn.getAttribute('data-sort') }));
    });
    root.querySelectorAll('[data-filter]').forEach((sel) => {
      sel.addEventListener('change', () => setHashQuery({ [sel.getAttribute('data-filter')]: sel.value }));
    });
    const clear = root.querySelector('[data-clear]');
    if (clear) clear.addEventListener('click', () => setHashQuery({ skillId: '' }));

    root.querySelectorAll('[data-grab]').forEach((btn) => {
      btn.addEventListener('click', () => grab(btn.getAttribute('data-grab'), btn));
    });

    bindPager(root, (target) => setHashQuery({ page: target }, { keepPage: true }));
  };

  return { html, mount, title: '接单大厅' };
}

/* ------------------------------------------------------------ 零件 -- */

function hallCardHTML(item, skills) {
  const budget = fmtCents(item.budgetCents);
  const skillNames = skillNamesOf(item.skillIds, skills);
  const expire = item.expiredAt ? fmtDateTime(item.expiredAt) : '';

  return `<article class="card hall-card">
    <div class="hall-main">
      <div class="hall-head">
        <h3>${esc(item.title)}</h3>
        <span class="amount-tag">${esc(budget)}</span>
      </div>
      <p class="hall-desc">${esc(clip(item.description, 160))}</p>
      <div class="hall-meta">
        ${skillNames.length ? skillNames.map((n) => `<span class="chip">${esc(n)}</span>`).join('') : ''}
        <span class="meta-item">${icon('clock', 13)}期望 ${esc(expectLabel(item.expectResponseMin))}响应</span>
        ${expire ? `<span class="meta-item">${icon('calendar', 13)}${esc(expire)} 截止</span>` : ''}
        ${(item.attachments || []).length
          ? `<span class="meta-item">${icon('link', 13)}${item.attachments.length} 个附件</span>` : ''}
      </div>
    </div>
    <div class="hall-side">
      <button class="btn btn-primary" type="button" data-grab="${esc(item.orderNo)}">接这一单</button>
      <span class="hall-hint">接单后请在支付截止前</span>
    </div>
  </article>`;
}

/* ------------------------------------------------------------ 内部 -- */

async function grab(orderNo, btn) {
  if (!orderNo) return;
  const original = btn.textContent;
  btn.disabled = true;
  btn.textContent = '正在接单…';
  try {
    const detail = await api.grab(orderNo);
    toastOk('接单成功，等待用户支付');
    location.hash = `#/orders/${encodeURIComponent(detail.orderNo || orderNo)}`;
  } catch (err) {
    btn.disabled = false;
    btn.textContent = original;
    // 抢单失败基本都是"被别人先接了"或"已不在待接单"，照实说，
    // 然后刷新列表——这条已经不该出现在大厅里了。
    // 注意此时地址往往没变（同一个筛选条件），hashchange 不会触发，要主动刷新一次。
    toastError(err instanceof ApiError ? err.message : '接单失败，请稍后再试');
    const before = location.hash;
    setHashQuery({});
    if (location.hash === before && window.__reloadPage) window.__reloadPage();
  }
}

function skillNamesOf(ids, skills) {
  const map = new Map((skills || []).map((s) => [String(s.id), s.name]));
  return (ids || []).map((id) => map.get(String(id))).filter(Boolean);
}

function expectLabel(min) {
  const n = Number(min) || 60;
  if (n >= 1440) return `${Math.round(n / 1440)} 天`;
  if (n >= 60) return `${Math.round(n / 60)} 小时`;
  return `${n} 分钟`;
}

function clip(text, max) {
  const s = String(text || '').replace(/\s+/g, ' ').trim();
  return s.length > max ? `${s.slice(0, max)}…` : s;
}
