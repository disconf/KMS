/* ============================================================================
   pages-mall.js — 积分商城。

   页面回答两个问题：
     1. 「攒分有什么用」——橱窗里摆着能换的东西，游客也能逛；
     2. 「我的分够换什么」——登录后余额常驻头部，不够的按钮直接给出缺口。

   兑换是**扣分操作**，扣错不可自行撤销（要找管理员退），
   所以确认弹窗必须把「扣多少、换什么、剩多少」一次说清。
   ========================================================================= */

import { api, isLoggedIn } from './api.js';
import { emptyHTML, loadingHTML, errorHTML, bindError, openModal, closeModal, toastOk, toastError } from './ui.js';
import { esc, fmtDate, fmtNum, icon } from './util.js';

/* 商品类型 → 图标与色调。色调走 app.css 的 is-* 修饰类。 */
const KIND_META = {
  download: { ic: 'download', tone: 'is-brand',  note: '兑换后发放下载权益' },
  template: { ic: 'book',     tone: 'is-info',   note: '兑换后发放模板文档' },
  code:     { ic: 'key',      tone: 'is-warn',   note: '兑换后发放权益兑换码' },
  benefit:  { ic: 'star',     tone: 'is-ok',     note: '兑换后开通对应站内权益' },
};

const STATUS_TONE = {
  pending:   'is-warn',
  fulfilled: 'is-ok',
  cancelled: '',
};

export async function mallPage() {
  const data = await api.mallItems().catch((err) => ({ error: err }));
  if (data && data.error) {
    return {
      title: '积分商城',
      html: `<div class="page-head"><h1>积分商城</h1></div>${errorHTML(data.error)}`,
      mount: (root) => bindError(root, { onRetry: () => window.__reloadPage() }),
    };
  }

  const logged = isLoggedIn();
  const items = Array.isArray(data.items) ? data.items : [];
  const myPoints = logged ? Number(data.myPoints) || 0 : -1;

  const html = `
    <div class="page-head">
      <h1>积分商城</h1>
      <p class="lede">
        分来自真实贡献：回答被采纳、教程与题目过审。在这里换成实际的东西——
        兑换后由运营发放，取消发放会全额退分。
      </p>
    </div>

    ${balanceHTML(myPoints, logged)}
    ${items.length ? itemsHTML(items, myPoints, logged) : emptyHTML({
      iconName: 'star',
      title: '货架整理中',
      desc: '商品正在上架，先去问答广场或题库攒点分。',
      actionHTML: '<a class="btn btn-primary" href="#/qa">去问答广场</a>',
    })}
  `;

  return {
    title: '积分商城',
    html,
    mount: (root) => {
      bindError(root, { onRetry: () => window.__reloadPage() });
      bindRedeem(root, myPoints, items);
    },
  };
}

/* ----------------------------------------------------------- 余额条 -- */

function balanceHTML(points, logged) {
  if (!logged) {
    return `<div class="mall-balance is-guest">
      <div class="mall-balance-main">
        <div class="mall-balance-ic">${icon('coin', 20)}</div>
        <div>
          <strong>登录后可兑换</strong>
          <span>你的积分余额会显示在这里</span>
        </div>
      </div>
      <button class="btn btn-primary" type="button" data-act="login">登录查看余额</button>
    </div>`;
  }
  return `<div class="mall-balance">
    <div class="mall-balance-main">
      <div class="mall-balance-ic">${icon('coin', 20)}</div>
      <div>
        <strong>${fmtNum(points)} <span>分</span></strong>
        <span>当前可用积分 · <a href="#/growth">查看积分来源</a></span>
      </div>
    </div>
    <a class="btn" href="#/mall/history">我的兑换记录</a>
  </div>`;
}

/* ------------------------------------------------------------- 货架 -- */

function itemsHTML(items, myPoints, logged) {
  return `<section class="mall-grid">
    ${items.map((it) => itemHTML(it, myPoints, logged)).join('')}
  </section>`;
}

function itemHTML(it, myPoints, logged) {
  const meta = KIND_META[it.kind] || KIND_META.benefit;
  const canBuy = logged && !it.soldOut && myPoints >= it.pointsCost;
  let btn;
  if (!logged) {
    btn = `<button class="btn btn-primary" type="button" data-act="login">登录兑换</button>`;
  } else if (it.soldOut) {
    btn = `<button class="btn" type="button" disabled>已抢完</button>`;
  } else if (myPoints < it.pointsCost) {
    btn = `<button class="btn" type="button" disabled>还差 ${fmtNum(it.pointsCost - myPoints)} 分</button>`;
  } else {
    btn = `<button class="btn btn-primary" type="button" data-redeem="${it.id}">兑换</button>`;
  }

  return `<article class="mall-card ${it.soldOut ? 'is-soldout' : ''}">
    <div class="mall-card-top">
      <span class="mall-kind ${meta.tone}">${icon(meta.ic, 13)}${esc(it.kindLabel)}</span>
      ${it.unlimited ? '' : `<span class="mall-stock">剩 ${fmtNum(it.stock)} 件</span>`}
    </div>
    <h2 class="mall-title">${esc(it.title)}</h2>
    ${it.description ? `<p class="mall-desc">${esc(it.description)}</p>` : ''}
    ${it.redeemNote ? `<p class="mall-note">${icon('info', 12)}${esc(it.redeemNote)}</p>` : ''}
    <div class="mall-card-foot">
      <strong class="mall-cost">${icon('coin', 14)}${fmtNum(it.pointsCost)} 分</strong>
      ${btn}
    </div>
  </article>`;
}

/* ------------------------------------------------------------- 兑换 -- */

function bindRedeem(root, myPoints, items) {
  // 登录引导按钮走全局的 auth-modal（data-act="login" 是约定事件）。
  const byId = new Map(items.map((it) => [it.id, it]));
  root.addEventListener('click', async (e) => {
    const btn = e.target.closest('[data-redeem]');
    if (!btn) return;
    const item = byId.get(Number(btn.getAttribute('data-redeem')));
    if (item) openConfirm(item, myPoints);
  });
}

function openConfirm(item, myPoints) {
  const after = myPoints - item.pointsCost;
  openModal(`
    <div class="modal-head"><h2>确认兑换</h2></div>
    <div class="modal-body">
      <div class="mall-confirm">
        <div class="mall-confirm-row"><span>兑换商品</span><strong>${esc(item.title)}</strong></div>
        <div class="mall-confirm-row"><span>消耗积分</span><strong>${fmtNum(item.pointsCost)} 分</strong></div>
        <div class="mall-confirm-row"><span>兑换后余额</span><strong>${fmtNum(Math.max(after, 0))} 分</strong></div>
      </div>
      <p class="mall-confirm-tip">${icon('info', 13)}兑换提交后立即扣分；若运营未发放，可在「我的兑换记录」里看到进度，取消发放会全额退分。</p>
    </div>
    <div class="modal-actions">
      <button class="btn" type="button" data-close>再想想</button>
      <button class="btn btn-primary" type="button" data-confirm>确认兑换</button>
    </div>
  `, (modal) => {
    modal.querySelector('[data-close]').addEventListener('click', closeModal);
    modal.querySelector('[data-confirm]').addEventListener('click', async () => {
      const btn = modal.querySelector('[data-confirm]');
      btn.disabled = true;
      btn.textContent = '兑换中…';
      try {
        const resp = await api.mallRedeem(item.id);
        closeModal();
        toastOk(`兑换成功，剩余 ${fmtNum(resp.pointsAfter)} 分`);
        // 原地重绘：余额与按钮状态都变了，整页刷新最省心。
        window.__reloadPage();
      } catch (err) {
        toastError(err && err.message ? err.message : '兑换失败，请稍后再试');
        btn.disabled = false;
        btn.textContent = '确认兑换';
      }
    });
  });
}

/* ============================================================================
   我的兑换记录（#/mall/history）。
   ========================================================================= */

export async function mallHistoryPage() {
  if (!isLoggedIn()) {
    return {
      title: '我的兑换记录',
      html: `<div class="page-head"><h1>我的兑换记录</h1></div>${emptyHTML({
        iconName: 'user',
        title: '登录后查看兑换记录',
        desc: '兑换进度与发放凭证都在这里。',
        actionHTML: '<button class="btn btn-primary" type="button" data-act="login">登录</button>',
      })}`,
      mount: (root) => bindError(root, {}),
    };
  }

  const data = await api.mallRedemptions({ limit: 100 }).catch((err) => ({ error: err }));
  if (data && data.error) {
    return {
      title: '我的兑换记录',
      html: `<div class="page-head"><h1>我的兑换记录</h1></div>${errorHTML(data.error)}`,
      mount: (root) => bindError(root, { onRetry: () => window.__reloadPage() }),
    };
  }

  const items = Array.isArray(data.items) ? data.items : [];
  const html = `
    <div class="page-head">
      <h1>我的兑换记录</h1>
      <p class="lede">「待发放」的单子由运营处理，发放后凭证会显示在这里。</p>
    </div>
    ${items.length ? historyHTML(items) : emptyHTML({
      iconName: 'star',
      title: '还没有兑换过',
      desc: '去商城看看能换什么。',
      actionHTML: '<a class="btn btn-primary" href="#/mall">逛逛商城</a>',
    })}
  `;
  return { title: '我的兑换记录', html, mount: (root) => bindError(root, {}) };
}

function historyHTML(items) {
  return `<section class="mall-history">
    ${items.map((r) => `
      <div class="mall-hist-row">
        <div class="mall-hist-main">
          <strong>${esc(r.itemTitle)}</strong>
          <span>${fmtDate(r.createdAt)} · ${fmtNum(r.pointsCost)} 分</span>
        </div>
        <div class="mall-hist-side">
          <span class="mall-status ${STATUS_TONE[r.status] || ''}">${esc(r.statusLabel)}</span>
          ${r.remark ? `<div class="mall-remark" title="${esc(r.remark)}">${icon('key', 12)}${esc(r.remark)}</div>` : ''}
        </div>
      </div>`).join('')}
  </section>`;
}
