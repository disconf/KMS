/* ============================================================================
   pages-orders.js — 订单中心：我的订单列表与订单详情（含支付、会话、评价、争议）。

   两个页面的共同前提：**所有动作都以服务端返回的状态为准**。
   前端只是把"这个状态下该出现哪些按钮"画出来（orders.js 的 orderActions），
   点了之后仍然可能被服务端拒绝——比如两个标签页同时操作同一单，
   后点的那个会拿到状态已变化的错误。这是正常的，提示出来即可，
   绝不能在前端先把状态改成"看起来成功了"。
   ========================================================================= */

import { api, isLoggedIn, currentUser, ApiError } from './api.js';
import {
  esc, fmtDateTime, fmtRelative, fmtDuration, fmtNum, icon, setHashQuery,
} from './util.js';
import {
  ORDER_STATUS, ORDER_TYPE_LABEL, SERVICE_MODE_LABEL, PAY_CHANNEL_LABEL,
  statusChipHTML, statusDesc, orderActions, ACTION_LABEL, remainText,
  fmtCents, TERMINAL_STATUS,
} from './orders.js';
import {
  bindPager, emptyHTML, openModal, pagerHTML, toastError, toastOk,
} from './ui.js';

const PAGE_SIZE = 10;

/** 列表页的状态筛选。空字符串表示不限——与后端 status 为空即不过滤的语义一致。 */
const STATUS_FILTERS = [
  { key: '', label: '全部' },
  { key: 'pending_accept', label: '待接单' },
  { key: 'pending_pay', label: '待支付' },
  { key: 'in_service', label: '服务中' },
  { key: 'pending_confirm', label: '待确认' },
  { key: 'completed', label: '已完成' },
  { key: 'disputed', label: '争议中' },
];

const SORTS = [
  { key: 'latest', label: '最新' },
  { key: 'oldest', label: '最早' },
  { key: 'amount', label: '金额' },
];

/* ====================================================== 订单列表 ====== */

export async function orderListPage(ctx) {
  if (!isLoggedIn()) {
    return {
      html: `<div class="page-head">
        <h1>我的订单</h1>
        <p class="lede">登录后可以在这里跟进悬赏与预约的服务进度。</p>
      </div>` + emptyHTML({
        iconName: 'list',
        title: '登录后查看订单',
        desc: '订单与登录账号绑定，登录后即可看到进行中的服务。',
        actionHTML: '<button class="btn btn-primary" type="button" data-act="login">去登录</button>',
      }),
      title: '我的订单',
    };
  }

  const params = ctx.params;
  const side = params.get('side') === 'expert' ? 'expert' : 'user';
  const status = STATUS_FILTERS.some((s) => s.key === params.get('status')) ? params.get('status') : '';
  const sort = SORTS.some((s) => s.key === params.get('sort')) ? params.get('sort') : 'latest';
  const keyword = params.get('keyword') || '';
  const page = Math.max(1, Number(params.get('page') || 1));

  const [counts, result] = await Promise.all([
    api.orderCounts(side).catch(() => ({ total: 0, byStatus: {} })),
    api.orders({
      side, status: status || undefined, sort,
      keyword: keyword || undefined, page, pageSize: PAGE_SIZE,
    }),
  ]);

  const list = result.list || [];
  const total = result.total || 0;
  const byStatus = counts.byStatus || {};

  const html = `
    <div class="page-head">
      <h1>${side === 'expert' ? '我接的单' : '我的订单'}</h1>
      <p class="lede">${side === 'expert'
        ? '这里是你接下的悬赏与预约。服务中的订单可以开始会话、标记完成。'
        : '悬赏与预约共用一套流程：待支付 → 服务中 → 待确认 → 已完成 → 已结算。'}</p>
    </div>

    <div class="toolbar">
      <div class="seg" role="group" aria-label="订单视角">
        <button type="button" data-side="user" class="${side === 'user' ? 'is-active' : ''}">
          我下的单${counts.total ? ` · ${counts.total}` : ''}
        </button>
        <button type="button" data-side="expert" class="${side === 'expert' ? 'is-active' : ''}">
          我接的单
        </button>
      </div>

      <span class="toolbar-spacer"></span>
      <a class="btn btn-primary btn-sm" href="#/ask">${icon('plus', 14)}发布悬赏</a>
      ${side === 'expert' ? `<a class="btn btn-sm" href="#/hall">${icon('target', 14)}接单大厅</a>` : ''}
    </div>

    <div class="tabs" style="border:0;margin-bottom:14px">
      ${STATUS_FILTERS.map((f) => `
        <button class="tab ${f.key === status ? 'is-active' : ''}" type="button" data-status="${f.key}">
          ${f.label}${f.key && byStatus[f.key] ? ` <span class="tab-count">${byStatus[f.key]}</span>` : ''}
        </button>`).join('')}
    </div>

    <div class="toolbar" style="margin-top:-6px">
      <div class="seg" role="group" aria-label="排序">
        ${SORTS.map((s) => `<button type="button" data-sort="${s.key}"
          class="${s.key === sort ? 'is-active' : ''}">${s.label}</button>`).join('')}
      </div>
      ${keyword ? `<button class="chip chip-brand" type="button" data-clear="keyword">
        关键词：${esc(keyword)} ${icon('close', 12)}</button>` : ''}
      <span class="toolbar-spacer"></span>
      <span style="font-size:12.5px;color:var(--text-3)">共 ${fmtNum(total)} 单</span>
    </div>

    <div class="order-list">
      ${list.length
        ? list.map((o) => orderCardHTML(o, side)).join('')
        : emptyHTML({
            iconName: 'list',
            title: '这里还没有订单',
            desc: side === 'expert'
              ? '去接单大厅看看有没有对口的悬赏。'
              : '发布一条悬赏，描述清楚故障现象，值班专家会接单。',
            actionHTML: side === 'expert'
              ? '<a class="btn btn-primary btn-sm" href="#/hall">去接单大厅</a>'
              : '<a class="btn btn-primary btn-sm" href="#/ask">发布悬赏</a>',
          })}
    </div>

    ${pagerHTML({ page, pageSize: PAGE_SIZE, total, unit: '单' })}
  `;

  const mount = (root) => {
    root.querySelectorAll('[data-side]').forEach((btn) => {
      btn.addEventListener('click', () => setHashQuery({ side: btn.getAttribute('data-side'), status: '' }));
    });
    root.querySelectorAll('[data-status]').forEach((btn) => {
      btn.addEventListener('click', () => setHashQuery({ status: btn.getAttribute('data-status') }));
    });
    root.querySelectorAll('[data-sort]').forEach((btn) => {
      btn.addEventListener('click', () => setHashQuery({ sort: btn.getAttribute('data-sort') }));
    });
    const clear = root.querySelector('[data-clear="keyword"]');
    if (clear) clear.addEventListener('click', () => setHashQuery({ keyword: '' }));
    bindPager(root, (target) => setHashQuery({ page: target }, { keepPage: true }));
  };

  return { html, mount, title: side === 'expert' ? '我接的单' : '我的订单' };
}

/* ====================================================== 订单详情 ====== */

export async function orderDetailPage(ctx, orderNo) {
  const order = await api.order(orderNo);
  const role = viewerRole(order);

  const mount = (root) => {
    root.querySelectorAll('[data-order-act]').forEach((btn) => {
      btn.addEventListener('click', () => {
        runAction(btn.getAttribute('data-order-act'), order, role, btn);
      });
    });
  };

  return { html: orderDetailHTML(order, role), mount, title: order.title || order.orderNo };
}

/** 谁在看这一单：专家本人看专家视角，其余（含管理员）按下单人视角。 */
function viewerRole(order) {
  const me = Number((currentUser() || {}).id);
  if (!me) return 'user';
  return Number(order.expertId) === me && Number(order.userId) !== me ? 'expert' : 'user';
}

/**
 * 详情页的纯渲染：给订单对象就出 HTML，不取数、不绑事件。
 *
 * 单独抽出来是为了能在 Node 里把 9 种状态逐一渲染一遍（见 scripts/render_check.mjs）——
 * 否则每个状态的模板都得在真库里造一笔单才能验证，而造一笔"争议已结案"的单
 * 需要一整套后台操作。渲染是纯函数，就不该被取数绑住。
 */
export function orderDetailHTML(order, role) {
  const acts = orderActions(order, role);
  const remark = order.remark || '';

  return `
    <nav class="crumb">
      <a href="#/">首页</a>
      <span class="crumb-sep">/</span>
      <a href="#/orders?side=${role === 'expert' ? 'expert' : 'user'}">${role === 'expert' ? '我接的单' : '我的订单'}</a>
      <span class="crumb-sep">/</span>
      <span>${esc(order.orderNo)}</span>
    </nav>

    <header class="detail-head">
      <div class="order-title-row">
        <h1>${esc(order.title || '未命名订单')}</h1>
        ${statusChipHTML(order.status)}
      </div>
      <div class="detail-meta">
        <span class="chip">${esc(ORDER_TYPE_LABEL[order.type] || order.type)}</span>
        ${order.serviceMode ? `<span class="chip">${esc(SERVICE_MODE_LABEL[order.serviceMode] || order.serviceMode)}</span>` : ''}
        <span class="meta-item">${icon('clock', 14)}${esc(fmtDateTime(order.createdAt))}</span>
        <span class="meta-item mono">${esc(order.orderNo)}</span>
      </div>
    </header>

    ${statusBannerHTML(order)}

    <div class="detail-layout">
      <div>
        ${remark || order.title ? `<section class="card card-pad" style="margin-bottom:18px">
          <h2 class="block-title">需求说明</h2>
          ${order.title ? `<p style="font-size:14.5px;line-height:1.75;margin-bottom:10px">${esc(order.title)}</p>` : ''}
          ${remark ? `<p style="color:var(--text-2);line-height:1.75;white-space:pre-wrap">${esc(remark)}</p>` : ''}
        </section>` : ''}

        ${timelineSectionHTML(order)}
        ${sessionsSectionHTML(order, role)}
        ${reviewSectionHTML(order, role)}
        ${disputeSectionHTML(order)}
      </div>

      <aside>
        ${actionsCardHTML(order, acts)}

        <div class="card side-card" style="margin-bottom:16px">
          <h3>金额</h3>
          <div class="amount-row">
            <span>订单总额</span>
            <strong class="amount-value">${fmtCents(order.amountCents)}</strong>
          </div>
          <dl class="kv" style="margin-top:12px">
            ${kvRow('平台佣金', fmtCents(order.commissionCents))}
            ${kvRow('专家实收', fmtCents(order.expertAmountCents))}
            ${order.payChannel ? kvRow('支付方式', PAY_CHANNEL_LABEL[order.payChannel] || order.payChannel) : ''}
          </dl>
          ${order.settlement ? `<div class="settle-note">
            <div>结算单 <span class="mono">${esc(order.settlement.settlementNo)}</span></div>
            <div>抽佣 ${(Number(order.settlement.commissionRateBp || 0) / 100).toFixed(2)}%
              · ${esc(order.settlement.settledAt ? fmtDateTime(order.settlement.settledAt) : '待入账')}</div>
          </div>` : ''}
        </div>

        <div class="card side-card">
          <h3>订单信息</h3>
          <dl class="kv">
            ${kvRow('订单号', order.orderNo, true)}
            ${kvRow('类型', ORDER_TYPE_LABEL[order.type] || order.type)}
            ${role === 'expert'
              ? kvRow('下单人', order.userName || `用户 ${order.userId}`)
              : kvRow('服务专家', order.expertName || (order.expertId ? `专家 ${order.expertId}` : '待接单'))}
            ${order.payExpireAt && order.status === 'pending_pay'
              ? kvRow('支付截止', fmtDateTime(order.payExpireAt)) : ''}
            ${order.confirmDeadline && order.status === 'pending_confirm'
              ? kvRow('自动确认', fmtDateTime(order.confirmDeadline)) : ''}
            ${order.cancelReason ? kvRow('取消原因', order.cancelReason) : ''}
          </dl>
        </div>
      </aside>
    </div>
  `;
}

/* -------------------------------------------------------- 列表零件 -- */

/**
 * 一张订单卡片。
 *
 * 点整块区域进详情，卡片里不再放"查看"按钮——订单列表是扫读场景，
 * 多一个按钮只多一个误点。
 */
function orderCardHTML(o, side) {
  const counterpart = side === 'expert'
    ? (o.userName || (o.userId ? `用户 ${o.userId}` : ''))
    : (o.expertName || (o.expertId ? `专家 ${o.expertId}` : '等待专家接单'));

  // 倒计时只在不倒计时的状态下展示，否则用户会以为"剩余 3 天"是付款期限
  let countdown = '';
  if (o.status === 'pending_pay' && o.payExpireAt) {
    countdown = `<span class="order-countdown">${esc(remainText(o.payExpireAt))}</span>`;
  } else if (o.status === 'pending_confirm' && o.confirmDeadline) {
    countdown = `<span class="order-countdown">${esc(remainText(o.confirmDeadline))} 自动确认</span>`;
  }

  return `<a class="order-card" href="#/orders/${encodeURIComponent(o.orderNo)}">
    <div class="order-card-main">
      <div class="order-card-top">
        ${statusChipHTML(o.status)}
        <span class="order-type">${esc(ORDER_TYPE_LABEL[o.type] || o.type)}</span>
        <span class="mono">${esc(o.orderNo)}</span>
      </div>
      <h3>${esc(o.title || '未命名订单')}</h3>
      <div class="order-card-meta">
        ${counterpart ? `<span class="meta-item">${icon('user', 13)}${esc(counterpart)}</span>` : ''}
        <span class="meta-item">${icon('clock', 13)}${esc(fmtRelative(o.createdAt))}</span>
        ${o.reviewed ? `<span class="chip chip-ok">已评价</span>` : ''}
      </div>
    </div>
    <div class="order-card-side">
      <span class="amount-tag">${fmtCents(o.amountCents)}</span>
      ${countdown}
    </div>
  </a>`;
}

/* -------------------------------------------------------- 详情零件 -- */

function statusBannerHTML(order) {
  const desc = statusDesc(order.status);
  const tips = [];

  if (order.status === 'pending_pay' && order.payExpireAt) {
    tips.push(`${remainText(order.payExpireAt)}，超时未支付订单会自动关闭`);
  }
  if (order.status === 'pending_accept') {
    tips.push('有专家接单后会立刻通知你，支付完成前都可以取消');
  }
  if (order.status === 'pending_confirm' && order.confirmDeadline) {
    tips.push(`${remainText(order.confirmDeadline)} 将自动确认完成并结算`);
  }
  if (order.status === 'cancelled' && order.cancelReason) {
    tips.push(`原因：${order.cancelReason}`);
  }
  if (order.status === 'disputed' && order.dispute) {
    tips.push('争议处理期间款项冻结，平台会在时限内给出结论');
  }
  if (order.status === 'settled') {
    tips.push('结算已完成，专家收入已入账');
  }

  return `<div class="status-banner is-${esc((ORDER_STATUS[order.status] || {}).tone || 'muted')}">
    <div class="status-banner-main">
      <strong>${esc((ORDER_STATUS[order.status] || {}).label || order.status)}</strong>
      ${desc ? `<span>${esc(desc)}</span>` : ''}
    </div>
    ${tips.length ? `<ul class="status-banner-tips">${tips.map((t) => `<li>${esc(t)}</li>`).join('')}</ul>` : ''}
  </div>`;
}

function timelineSectionHTML(order) {
  const nodes = (order.timeline || []).filter((n) => n && n.status);
  if (!nodes.length) return '';
  return `<section class="card card-pad" style="margin-bottom:18px">
    <h2 class="block-title">履约进度</h2>
    <ol class="tl">
      ${nodes.map((n, i) => {
        const meta = ORDER_STATUS[n.status] || { label: n.status };
        const last = i === nodes.length - 1;
        return `<li class="tl-node ${last ? 'is-current' : ''}">
          <span class="tl-dot"></span>
          <div class="tl-body">
            <div class="tl-label">${esc(meta.label)}</div>
            <div class="tl-time">${esc(fmtDateTime(n.at))} · ${esc(fmtRelative(n.at))}</div>
          </div>
        </li>`;
      }).join('')}
    </ol>
  </section>`;
}

function sessionsSectionHTML(order, role) {
  const sessions = order.sessions || [];
  const canStart = order.status === 'in_service';

  return `<section class="card card-pad" style="margin-bottom:18px">
    <div class="block-head">
      <h2 class="block-title">服务会话</h2>
      ${canStart ? `<button class="btn btn-sm" type="button" data-order-act="session">
        ${icon('play', 14)}开始会话</button>` : ''}
    </div>

    ${sessions.length ? `<div class="session-list">
      ${sessions.map((s) => `<div class="session-item">
        <div class="session-main">
          <span class="chip">${esc(SERVICE_MODE_LABEL[s.channel] || s.channel)}</span>
          <span class="session-time">${esc(fmtDateTime(s.startTime || ''))}
            ${s.endTime ? `→ ${esc(fmtDateTime(s.endTime))}` : ''}</span>
        </div>
        <div class="session-side">
          <span class="mono">${s.durationSec ? esc(fmtDuration(s.durationSec)) : '进行中'}</span>
          ${s.status === 'ongoing'
            ? `<button class="btn btn-sm" type="button" data-order-act="end-session">结束会话</button>`
            : ''}
        </div>
      </div>`).join('')}
    </div>` : `<p class="muted-note">
      ${canStart ? '还没有会话记录。开始会话后由服务端计时，用于核对服务时长。'
        : '订单进入服务中之后，双方可以在这里开始会话。'}
    </p>`}
  </section>`;
}

function reviewSectionHTML(order, role) {
  const r = order.review;
  if (!r) {
    if (order.status === 'completed' || order.status === 'settled') {
      return `<section class="card card-pad" style="margin-bottom:18px">
        <h2 class="block-title">服务评价</h2>
        <p class="muted-note">${role === 'expert' ? '用户还没有评价这一单。' : '还没有评价。评价能帮后来的用户判断专家是否靠谱。'}</p>
      </section>`;
    }
    return '';
  }

  return `<section class="card card-pad" style="margin-bottom:18px">
    <div class="block-head">
      <h2 class="block-title">服务评价</h2>
      <div class="review-score">${starsHTML(r.score)}<strong>${Number(r.score).toFixed(1)}</strong></div>
    </div>
    <div class="review-sub">
      ${r.professionalScore ? `<span>专业 ${r.professionalScore} 分</span>` : ''}
      ${r.speedScore ? `<span>响应 ${r.speedScore} 分</span>` : ''}
      ${r.communicationScore ? `<span>沟通 ${r.communicationScore} 分</span>` : ''}
      ${r.recommend ? '<span class="chip chip-ok">愿意推荐</span>' : ''}
      ${r.anonymous ? '<span class="chip">匿名</span>' : ''}
    </div>
    <p class="review-content">${esc(r.content)}</p>
    <div class="review-time">${esc(fmtDateTime(r.createdAt))}</div>
    ${r.reply ? `<div class="review-reply">
      <strong>专家回复</strong>
      <p>${esc(r.reply)}</p>
      ${r.repliedAt ? `<span class="review-time">${esc(fmtDateTime(r.repliedAt))}</span>` : ''}
    </div>` : ''}
    ${r.appealed ? '<p class="muted-note" style="margin-top:10px">已提交申诉，等待平台复核。</p>' : ''}
  </section>`;
}

function disputeSectionHTML(order) {
  const d = order.dispute;
  if (!d) return '';
  return `<section class="card card-pad" style="margin-bottom:18px">
    <h2 class="block-title">争议记录</h2>
    <div class="review-sub" style="margin-bottom:10px">
      <span class="chip ${d.status === 'open' ? 'chip-warn' : 'chip-ok'}">
        ${d.status === 'open' ? '处理中' : '已结案'}</span>
      ${d.result ? `<span>结论：${esc(d.result)}</span>` : ''}
    </div>
    <p class="review-content">${esc(d.reason)}</p>
    ${(d.evidence || []).length ? `<div class="evidence-list">
      ${d.evidence.map((f) => `<a class="chip" href="${esc(f.url || '#')}" target="_blank" rel="noopener">
        ${esc(f.name || '附件')}</a>`).join('')}
    </div>` : ''}
    <div class="review-time">
      发起于 ${esc(fmtDateTime(d.createdAt))}
      ${d.handledAt ? ` · 处理于 ${esc(fmtDateTime(d.handledAt))}` : ''}
    </div>
  </section>`;
}

function actionsCardHTML(order, acts) {
  if (!acts.length) {
    const terminal = TERMINAL_STATUS.includes(order.status);
    return `<div class="card side-card" style="margin-bottom:16px">
      <h3>操作</h3>
      <p class="muted-note">${terminal
        ? '这一单已经结束，没有待办操作。'
        : '当前没有需要你做的事，等待对方或系统推进。'}</p>
    </div>`;
  }

  // 主操作排最前：用户在详情页最想知道"我现在该点哪个"
  const primary = { pay: 1, confirm: 1, review: 1, finish: 1, session: 1 };
  const ordered = [...acts].sort((a, b) => (primary[b] || 0) - (primary[a] || 0));

  return `<div class="card side-card" style="margin-bottom:16px">
    <h3>操作</h3>
    <div class="action-stack">
      ${ordered.map((a, i) => `<button class="btn ${i === 0 ? 'btn-primary btn-block' : 'btn-block'}"
        type="button" data-order-act="${a}">${esc(ACTION_LABEL[a] || a)}</button>`).join('')}
    </div>
  </div>`;
}

/* ------------------------------------------------------------ 动作 -- */

async function runAction(act, order, role, btn) {
  const orderNo = order.orderNo;
  try {
    switch (act) {
      case 'pay':         return openPayModal(order);
      case 'cancel':      return openCancelModal(order);
      case 'confirm':     return openConfirmModal(order);
      case 'review':      return openReviewModal(order);
      case 'dispute':     return openDisputeModal(order);
      case 'session':     return openSessionModal(order);
      case 'finish':      return finishOrder(order, btn);
      case 'end-session': return endSession(order, btn);
      case 'reply':       return openReplyModal(order);
      case 'appeal':      return openAppealModal(order);
      default:            toastError('未知操作');
    }
  } catch (err) {
    reportError(err);
  }
}

function openPayModal(order) {
  api.payChannels().then((channels) => {
    const list = (channels || []).length
      ? channels
      : [{ name: 'mock', label: '模拟支付', available: true, current: true }];

    openModal(`
      <div class="modal-head">
        <div>
          <h2>支付 ${fmtCents(order.amountCents)}</h2>
          <p>订单 ${esc(order.orderNo)}</p>
        </div>
      </div>
      <div class="modal-body">
        <div class="pay-list">
          ${list.map((c, i) => `<label class="pay-item ${c.available ? '' : 'is-disabled'}">
            <input type="radio" name="pay-channel" value="${esc(c.name)}"
              ${c.available ? '' : 'disabled'} ${i === 0 && c.available ? 'checked' : ''} />
            <span class="pay-name">${esc(c.label || c.name)}</span>
            ${c.current ? '<span class="chip chip-brand">当前通道</span>' : ''}
            ${c.available ? '' : '<span class="chip">暂未开通</span>'}
          </label>`).join('')}
        </div>
        <p class="muted-note" id="pay-hint">
          未开通的通道会置灰而不是隐藏：以后能用和本来就不支持，是两回事。
        </p>
        <button class="btn btn-primary btn-block btn-lg" id="pay-submit" type="button">
          确认支付
        </button>
      </div>
    `, (modal) => {
      const submit = modal.querySelector('#pay-submit');
      const hint = modal.querySelector('#pay-hint');

      submit.addEventListener('click', async () => {
        const picked = modal.querySelector('input[name="pay-channel"]:checked');
        if (!picked) { hint.textContent = '请选择一个支付方式。'; return; }

        submit.disabled = true;
        submit.textContent = '正在发起…';
        try {
          const res = await api.pay(order.orderNo, { channel: picked.value });
          if (res.manual) {
            // 线下转账：接口不会把订单置为已支付，必须把"下一步做什么"讲清楚
            openInstructionModal(res);
            return;
          }
          toastOk(res.paid ? '支付成功' : '支付已提交');
          closeAndReload();
        } catch (err) {
          submit.disabled = false;
          submit.textContent = '确认支付';
          hint.textContent = err instanceof ApiError ? err.message : '支付失败，请稍后再试。';
        }
      });
    });
  }).catch(reportError);
}

function openInstructionModal(res) {
  openModal(`
    <div class="modal-head">
      <div>
        <h2>待人工核账</h2>
        <p>线下转账不会即时到账，后台确认收款后订单才会进入服务中。</p>
      </div>
    </div>
    <div class="modal-body">
      <p class="instruction">${esc(res.instruction || '请按平台提供的账户完成转账，并在备注里填写订单号。')}</p>
      <dl class="kv">
        <div class="kv-row"><dt>订单号</dt><dd class="mono">${esc(res.orderNo || '')}</dd></div>
        <div class="kv-row"><dt>通道</dt><dd>${esc(PAY_CHANNEL_LABEL[res.channel] || res.channel || '')}</dd></div>
      </dl>
      <button class="btn btn-block" type="button" data-modal-close>知道了</button>
    </div>
  `);
}

function openCancelModal(order) {
  openModal(`
    <div class="modal-head">
      <div>
        <h2>取消订单</h2>
        <p>取消后订单关闭且不可恢复，需要重新发布或预约。</p>
      </div>
    </div>
    <div class="modal-body">
      <div class="field">
        <label for="cancel-reason">取消原因（可选）</label>
        <textarea class="input" id="cancel-reason" rows="3" maxlength="200"
          placeholder="例如：问题已经自己解决了"></textarea>
      </div>
      <p class="muted-note" style="margin-top:4px">已支付的订单不能直接取消，请走「申请退款 / 争议」。</p>
      <button class="btn btn-danger btn-block" id="cancel-submit" type="button">确认取消订单</button>
    </div>
  `, (modal) => {
    const btn = modal.querySelector('#cancel-submit');
    btn.addEventListener('click', async () => {
      btn.disabled = true;
      btn.textContent = '正在取消…';
      try {
        await api.cancelOrder(order.orderNo, {
          reason: modal.querySelector('#cancel-reason').value.trim() || undefined,
        });
        toastOk('订单已取消');
        closeAndReload();
      } catch (err) {
        btn.disabled = false;
        btn.textContent = '确认取消订单';
        reportError(err);
      }
    });
  });
}

function openConfirmModal(order) {
  openModal(`
    <div class="modal-head">
      <div>
        <h2>确认服务完成</h2>
        <p>确认后款项将按分账结果结算给专家：${fmtCents(order.expertAmountCents)}。</p>
      </div>
    </div>
    <div class="modal-body">
      <p class="muted-note">如果服务还没有真正完成，先别确认——确认之后订单会直接进入结算，
        后续只能通过争议处理。</p>
      <button class="btn btn-primary btn-block btn-lg" id="confirm-submit" type="button">
        确认完成并结算
      </button>
    </div>
  `, (modal) => {
    const btn = modal.querySelector('#confirm-submit');
    btn.addEventListener('click', async () => {
      btn.disabled = true;
      btn.textContent = '正在确认…';
      try {
        await api.confirmOrder(order.orderNo);
        toastOk('已确认完成');
        closeAndReload();
      } catch (err) {
        btn.disabled = false;
        btn.textContent = '确认完成并结算';
        reportError(err);
      }
    });
  });
}

function openReviewModal(order) {
  openModal(`
    <div class="modal-head">
      <div>
        <h2>评价这次服务</h2>
        <p>一单只能评一次，提交后不可修改。</p>
      </div>
    </div>
    <div class="modal-body">
      <div class="field">
        <label>总体评分</label>
        <div class="star-picker" data-stars="score">${starPickerHTML(5)}</div>
      </div>
      <div class="field-row">
        <div class="field">
          <label>专业程度</label>
          <div class="star-picker" data-stars="professionalScore">${starPickerHTML(5)}</div>
        </div>
        <div class="field">
          <label>响应速度</label>
          <div class="star-picker" data-stars="speedScore">${starPickerHTML(5)}</div>
        </div>
        <div class="field">
          <label>沟通体验</label>
          <div class="star-picker" data-stars="communicationScore">${starPickerHTML(5)}</div>
        </div>
      </div>
      <div class="field">
        <label for="review-content">评价内容（至少 10 个字）</label>
        <textarea class="input" id="review-content" rows="4" maxlength="1000"
          placeholder="说说专家的排障思路、响应速度，给后来的用户一个参考"></textarea>
      </div>
      <label class="check-line"><input type="checkbox" id="review-recommend" checked /> 愿意推荐这位专家</label>
      <label class="check-line"><input type="checkbox" id="review-anonymous" /> 匿名评价</label>
      <button class="btn btn-primary btn-block btn-lg" id="review-submit" type="button">提交评价</button>
    </div>
  `, (modal) => {
    bindStarPickers(modal);
    const btn = modal.querySelector('#review-submit');
    btn.addEventListener('click', async () => {
      const content = modal.querySelector('#review-content').value.trim();
      if (content.length < 10) {
        toastError('评价内容至少 10 个字');
        return;
      }
      const score = readScore(modal, 'score');
      if (!score) { toastError('请先给个总体评分'); return; }

      btn.disabled = true;
      btn.textContent = '正在提交…';
      try {
        await api.reviewOrder(order.orderNo, {
          score,
          // 分项默认也是满分：三个分项各点一遍才能评价太啰嗦，
          // 想打低分的人自然会去点。服务端对 0 值视为"未评分"，所以这里不会传 0。
          professionalScore: readScore(modal, 'professionalScore') || undefined,
          speedScore: readScore(modal, 'speedScore') || undefined,
          communicationScore: readScore(modal, 'communicationScore') || undefined,
          content,
          recommend: modal.querySelector('#review-recommend').checked,
          anonymous: modal.querySelector('#review-anonymous').checked,
        });
        toastOk('评价已提交');
        closeAndReload();
      } catch (err) {
        btn.disabled = false;
        btn.textContent = '提交评价';
        reportError(err);
      }
    });
  });
}

function openDisputeModal(order) {
  openModal(`
    <div class="modal-head">
      <div>
        <h2>申请退款 / 发起争议</h2>
        <p>平台介入后款项冻结，处理完成前订单不再推进。</p>
      </div>
    </div>
    <div class="modal-body">
      <div class="field">
        <label for="dispute-reason">问题描述（至少 10 个字）</label>
        <textarea class="input" id="dispute-reason" rows="4" maxlength="500"
          placeholder="说明服务哪里没有达到约定，越具体处理越快"></textarea>
      </div>
      <p class="muted-note">证据附件可以在后续补充给客服；这里先把问题描述清楚最重要。</p>
      <button class="btn btn-danger btn-block" id="dispute-submit" type="button">提交争议</button>
    </div>
  `, (modal) => {
    const btn = modal.querySelector('#dispute-submit');
    btn.addEventListener('click', async () => {
      const reason = modal.querySelector('#dispute-reason').value.trim();
      if (reason.length < 10) {
        toastError('问题描述至少 10 个字');
        return;
      }
      btn.disabled = true;
      btn.textContent = '正在提交…';
      try {
        await api.raiseDispute(order.orderNo, { reason });
        toastOk('已提交，平台会尽快处理');
        closeAndReload();
      } catch (err) {
        btn.disabled = false;
        btn.textContent = '提交争议';
        reportError(err);
      }
    });
  });
}

function openSessionModal(order) {
  openModal(`
    <div class="modal-head">
      <div>
        <h2>开始服务会话</h2>
        <p>选择沟通方式，服务时长由服务端计时。</p>
      </div>
    </div>
    <div class="modal-body">
      <div class="pay-list">
        ${Object.entries(SERVICE_MODE_LABEL).map(([k, v], i) => `<label class="pay-item">
          <input type="radio" name="session-channel" value="${esc(k)}" ${i === 0 ? 'checked' : ''} />
          <span class="pay-name">${esc(v)}</span>
        </label>`).join('')}
      </div>
      <button class="btn btn-primary btn-block btn-lg" id="session-submit" type="button">开始会话</button>
    </div>
  `, (modal) => {
    const btn = modal.querySelector('#session-submit');
    btn.addEventListener('click', async () => {
      const picked = modal.querySelector('input[name="session-channel"]:checked');
      if (!picked) return;
      btn.disabled = true;
      btn.textContent = '正在开始…';
      try {
        await api.startSession(order.orderNo, picked.value);
        toastOk('会话已开始');
        closeAndReload();
      } catch (err) {
        btn.disabled = false;
        btn.textContent = '开始会话';
        reportError(err);
      }
    });
  });
}

function openReplyModal(order) {
  openModal(`
    <div class="modal-head">
      <div><h2>回复评价</h2><p>回复会展示在评价下方，代表你的服务态度。</p></div>
    </div>
    <div class="modal-body">
      <div class="field">
        <label for="reply-text">回复内容</label>
        <textarea class="input" id="reply-text" rows="4" maxlength="1000"
          placeholder="说明处理思路，或对用户的反馈表示感谢"></textarea>
      </div>
      <button class="btn btn-primary btn-block" id="reply-submit" type="button">提交回复</button>
    </div>
  `, (modal) => {
    const btn = modal.querySelector('#reply-submit');
    btn.addEventListener('click', async () => {
      const text = modal.querySelector('#reply-text').value.trim();
      if (!text) { toastError('回复内容不能为空'); return; }
      btn.disabled = true;
      btn.textContent = '正在提交…';
      try {
        await api.replyReview(order.orderNo, { reply: text });
        toastOk('回复已提交');
        closeAndReload();
      } catch (err) {
        btn.disabled = false;
        btn.textContent = '提交回复';
        reportError(err);
      }
    });
  });
}

function openAppealModal(order) {
  openModal(`
    <div class="modal-head">
      <div><h2>申诉差评</h2><p>仅当评价与事实明显不符时使用，平台会人工复核。</p></div>
    </div>
    <div class="modal-body">
      <div class="field">
        <label for="appeal-text">申诉理由</label>
        <textarea class="input" id="appeal-text" rows="4" maxlength="1000"
          placeholder="说明实际情况，附上可核对的依据"></textarea>
      </div>
      <button class="btn btn-primary btn-block" id="appeal-submit" type="button">提交申诉</button>
    </div>
  `, (modal) => {
    const btn = modal.querySelector('#appeal-submit');
    btn.addEventListener('click', async () => {
      const text = modal.querySelector('#appeal-text').value.trim();
      if (!text) { toastError('申诉理由不能为空'); return; }
      btn.disabled = true;
      btn.textContent = '正在提交…';
      try {
        await api.appealReview(order.orderNo, { appealText: text });
        toastOk('申诉已提交');
        closeAndReload();
      } catch (err) {
        btn.disabled = false;
        btn.textContent = '提交申诉';
        reportError(err);
      }
    });
  });
}

async function finishOrder(order, btn) {
  if (!window.confirm('确认服务已经完成？标记后由用户确认，逾期未确认会自动确认。')) return;
  btn.disabled = true;
  try {
    await api.finishOrder(order.orderNo);
    toastOk('已标记完成，等待用户确认');
    closeAndReload();
  } catch (err) {
    btn.disabled = false;
    reportError(err);
  }
}

async function endSession(order, btn) {
  btn.disabled = true;
  try {
    const res = await api.endSession(order.orderNo);
    toastOk(`会话已结束，时长 ${fmtDuration(res && res.durationSec)}`);
    closeAndReload();
  } catch (err) {
    btn.disabled = false;
    reportError(err);
  }
}

/* ------------------------------------------------------------ 内部 -- */

function closeAndReload() {
  // 关弹窗后让当前页面重新取一次数据：状态是服务端算的，前端不自己改。
  const reload = window.__reloadPage;
  if (typeof reload === 'function') { reload(); return; }
  location.reload();
}

function reportError(err) {
  if (err instanceof ApiError && (err.status === 401 || err.code === 20004 || err.code === 20005)) {
    toastError('登录状态已失效，请重新登录');
    return;
  }
  toastError(err instanceof ApiError ? err.message : '操作失败，请稍后再试');
}

function kvRow(label, value, mono = false) {
  return `<div class="kv-row"><dt>${esc(label)}</dt>
    <dd class="${mono ? 'mono' : ''}">${esc(value)}</dd></div>`;
}

function starsHTML(score) {
  const n = Math.max(0, Math.min(5, Math.round(Number(score) || 0)));
  return `<span class="stars" aria-label="${n} 分">${'★'.repeat(n)}<i>${'★'.repeat(5 - n)}</i></span>`;
}

function starPickerHTML(n) {
  let out = '';
  for (let i = 1; i <= n; i++) {
    out += `<button class="star-btn" type="button" data-star="${i}" aria-label="${i} 分">★</button>`;
  }
  return out;
}

/**
 * 星级打分：点第 i 颗星即 i 分。
 *
 * 分数不存闭包变量，而是从 DOM 的 is-on 状态读——弹窗里的按钮会被替换、
 * 事件会重新绑定，闭包里的值一旦和画面不同步，用户看到 3 颗亮星却提交了 5 分。
 */
function bindStarPickers(root) {
  root.querySelectorAll('[data-stars]').forEach((group) => {
    const btns = [...group.querySelectorAll('[data-star]')];
    const paint = (value) => {
      btns.forEach((b) => {
        b.classList.toggle('is-on', Number(b.getAttribute('data-star')) <= value);
      });
    };
    btns.forEach((b) => {
      b.addEventListener('click', () => paint(Number(b.getAttribute('data-star'))));
    });
    paint(5);   // 默认满分，多数服务是正常的，省一次点击
  });
}

/** 读某个评分组的当前分数；没点过就是 5（默认满分）。 */
function readScore(root, key) {
  const group = root.querySelector(`[data-stars="${key}"]`);
  if (!group) return 0;
  return group.querySelectorAll('[data-star].is-on').length;
}
