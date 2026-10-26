/* ============================================================================
   ui.js — 跨页面复用的界面零件：提示、弹窗、分页、空态、加载与错误态。
   ========================================================================= */

import { el, esc, icon, clampHtml } from './util.js';

/* -------------------------------------------------------------- 提示 -- */

const toastRoot = () => document.getElementById('toast-root');

/**
 * 右下角轻提示。
 * 用轻提示而非弹窗报告结果：知识库里的绝大多数操作（点赞、下载、判分）
 * 都不该打断阅读，用户扫一眼就知道成没成。
 */
export function toast(message, kind = 'info', ms = 2600) {
  const root = toastRoot();
  if (!root) return;
  const node = el(`<div class="toast is-${esc(kind)}">
    <span class="toast-dot"></span><span>${esc(message)}</span>
  </div>`);
  root.appendChild(node);
  setTimeout(() => {
    node.style.transition = 'opacity .2s ease';
    node.style.opacity = '0';
    setTimeout(() => node.remove(), 220);
  }, ms);
}

export const toastOk = (m) => toast(m, 'ok');
export const toastError = (m) => toast(m, 'error', 3600);

/* -------------------------------------------------------------- 弹窗 -- */

let closeCurrent = null;

/**
 * 打开弹窗。contentHTML 里的动态数据必须已转义。
 * onMount 收到 (modalEl, maskEl)，用来绑定表单事件。
 */
export function openModal(contentHTML, onMount, { className = '' } = {}) {
  closeModal();
  const mask = el(`<div class="modal-mask">
    <div class="modal ${clampHtml(className)}" role="dialog" aria-modal="true" aria-label="对话框">
      ${clampHtml(contentHTML)}
      <button class="modal-close" type="button" data-modal-close aria-label="关闭" style="position:absolute;top:14px;right:14px">
        ${icon('close', 18)}
      </button>
    </div>
  </div>`);
  mask.querySelector('.modal').style.position = 'relative';

  const root = document.getElementById('modal-root');
  root.innerHTML = '';
  root.appendChild(mask);

  const onKey = (e) => { if (e.key === 'Escape') closeModal(); };
  mask.addEventListener('click', (e) => { if (e.target === mask) closeModal(); });
  mask.querySelectorAll('[data-modal-close]').forEach((b) => {
    b.addEventListener('click', () => closeModal());
  });
  document.addEventListener('keydown', onKey);

  closeCurrent = () => {
    document.removeEventListener('keydown', onKey);
    root.innerHTML = '';
    closeCurrent = null;
  };

  if (onMount) onMount(mask.querySelector('.modal'), mask);
  const focusable = mask.querySelector('input, button:not([data-modal-close])');
  if (focusable) setTimeout(() => focusable.focus(), 60);
  return mask;
}

export function closeModal() {
  if (closeCurrent) closeCurrent();
}

/* -------------------------------------------------------------- 分页 -- */

/**
 * 分页条。
 *
 * 页码做两端省略：知识库翻到第 40 页时，把 1..40 全列出来只会挤满一行。
 */
export function pagerHTML({ page, pageSize, total, unit = '条' }) {
  const pages = Math.max(1, Math.ceil(total / Math.max(1, pageSize)));
  if (total <= 0) return '';
  const items = pageWindow(page, pages);
  const btn = (label, target, opts = {}) =>
    `<button class="btn btn-sm" type="button" data-page="${target}"
      ${opts.disabled ? 'disabled' : ''}${opts.active ? ' aria-current="page"' : ''}
     >${label}</button>`;

  const nums = items.map((it) =>
    it === '...'
      ? '<span class="pager-info">…</span>'
      : btn(String(it), it, { active: it === page })
  ).join('');

  return `<nav class="pager" aria-label="分页">
    ${btn(icon('left', 14) + '上一页', page - 1, { disabled: page <= 1 })}
    ${nums}
    ${btn('下一页' + icon('right', 14), page + 1, { disabled: page >= pages })}
    <span class="pager-info">共 ${total} ${esc(unit)}</span>
  </nav>`;
}

function pageWindow(page, pages) {
  if (pages <= 7) return range(1, pages);
  const out = [1];
  const start = Math.max(2, page - 1);
  const end = Math.min(pages - 1, page + 1);
  if (start > 2) out.push('...');
  out.push(...range(start, end));
  if (end < pages - 1) out.push('...');
  out.push(pages);
  return out;
}

function range(a, b) {
  const out = [];
  for (let i = a; i <= b; i++) out.push(i);
  return out;
}

/** 给容器里的分页按钮统一接上翻页回调。 */
export function bindPager(container, onGo) {
  container.querySelectorAll('[data-page]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const target = Number(btn.getAttribute('data-page'));
      const current = Number(container.querySelector('[aria-current="page"]')?.textContent || 0);
      if (!Number.isFinite(target) || target < 1 || target === current) return;
      onGo(target);
    });
  });
}

/* -------------------------------------------------------- 空 / 加载 -- */

export function emptyHTML({ iconName = 'info', title, desc, actionHTML = '' }) {
  return `<div class="empty">
    <div class="empty-mark">${icon(iconName, 26)}</div>
    <strong>${esc(title)}</strong>
    ${desc ? `<p>${esc(desc)}</p>` : ''}
    ${clampHtml(actionHTML)}
  </div>`;
}

export function loadingHTML(text = '正在加载…') {
  return `<div class="page-loading"><span class="spinner"></span><p>${esc(text)}</p></div>`;
}

/** 骨架屏：列表页比转圈更不容易让人以为"加载完了但是空的"。 */
export function skeletonHTML(rows = 4) {
  let out = '<div class="post-list" style="margin-top:4px">';
  for (let i = 0; i < rows; i++) {
    out += `<div class="card" style="padding:18px 20px">
      <div class="skeleton-line" style="width:42%;height:15px"></div>
      <div class="skeleton-line" style="width:86%;margin-top:12px"></div>
      <div class="skeleton-line" style="width:60%;margin-top:8px"></div>
    </div>`;
  }
  return out + '</div>';
}

/**
 * 错误态。
 * 权限类错误（401 / 权限不足）不提供"重试"，重试多少次结果都一样，
 * 该做的是引导用户去登录。
 */
export function errorHTML(err, { retry = true } = {}) {
  const code = err && err.code;
  const isAuth = code === 20004 || code === 20005 || code === 20007 || (err && err.status === 401);
  const message = (err && err.message) || '加载失败';
  const action = isAuth
    ? `<button class="btn btn-primary" type="button" data-act="login">去登录</button>`
    : retry
      ? `<button class="btn" type="button" data-act="retry">${icon('refresh', 15)}重新加载</button>`
      : '';

  return `<div class="card"><div class="empty" style="padding:56px 24px">
    <div class="empty-mark" style="color:var(--danger);background:var(--danger-soft);border-color:#F6C8CE">
      ${icon('alert', 26)}
    </div>
    <strong>${esc(isAuth ? '需要登录后才能查看' : '没能加载出来')}</strong>
    <p>${esc(message)}</p>
    ${action}
  </div></div>`;
}

/** 给错误态里的按钮接线。 */
export function bindError(container, { onRetry, onLogin }) {
  const retry = container.querySelector('[data-act="retry"]');
  if (retry && onRetry) retry.addEventListener('click', onRetry);
  const login = container.querySelector('[data-act="login"]');
  if (login && onLogin) login.addEventListener('click', onLogin);
}

/* ------------------------------------------------------------ 零件 -- */

export function chipsHTML(list, className = '') {
  const arr = Array.isArray(list) ? list.filter(Boolean) : [];
  if (!arr.length) return '';
  return `<div class="chips">${arr
    .map((t) => `<span class="chip ${className}">${esc(t)}</span>`)
    .join('')}</div>`;
}

export function metaHTML(items) {
  const parts = (items || [])
    .filter((it) => it && it.text !== undefined && it.text !== null && it.text !== '')
    .map((it) => `<span class="meta-item">${it.icon ? icon(it.icon, 13) : ''}${esc(it.text)}</span>`);
  if (!parts.length) return '';
  return `<div class="meta">${parts.join('')}</div>`;
}

/** 头像：有图用图，没图用首字。 */
export function avatarHTML(name, url, size = 30) {
  const safe = url ? safeImage(url) : '';
  if (safe) {
    return `<span class="avatar" style="width:${size}px;height:${size}px">
      <img src="${esc(safe)}" alt="${esc(name)}" onerror="this.remove()" />
    </span>`;
  }
  return `<span class="avatar" style="width:${size}px;height:${size}px">${esc(initialOf(name))}</span>`;
}

function safeImage(url) {
  const s = String(url || '').trim();
  if (/^https?:\/\//i.test(s) || s.startsWith('/')) return s;
  return '';
}

function initialOf(name) {
  const s = String(name || '').trim();
  return s ? s[0].toUpperCase() : '?';
}

/** 交替行背景用的序号，避免每次渲染都重新算。 */
export function numberBadge(index) {
  return `<span class="badge num">${index + 1}</span>`;
}
