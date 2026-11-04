/* ============================================================================
   pages-runbook.js — 排障剧本：一个可走的决策树。

   交互核心是"路径"：每一步的选择都存在 hash 的 p 参数里（p=s1,s3,end-index），
   所以刷新、分享链接、后退都能回到同一条排查路径上。这点比存在内存里重要——
   值班交接时把链接发给同事，对方打开就是你走到的那一步。
   ========================================================================= */

import { esc, icon } from './util.js';
import { RUNBOOKS, RUNBOOK_COUNT, SEVERITY_LABEL, validateRunbooks } from './runbook-data.js';

/* ====================================================== 列表页 ======== */

export function runbookListPage(ctx) {
  validateRunbooks();

  const q = (ctx.params.get('q') || '').trim();
  const hit = q ? filterRunbooks(q) : RUNBOOKS;

  const html = `
    <div class="page-head">
      <h1>排障剧本</h1>
      <p class="lede">线上炸了的时候没人想看长文。这里把常见故障拆成一条条判断：
        敲哪条命令、看到什么现象、下一步查哪里，最后落到根因和修复清单。</p>
    </div>

    <div class="rb-toolbar">
      <div class="rb-search">
        ${icon('search', 15)}
        <input class="input" type="search" placeholder="搜现象、报错关键字，比如 502、OOM、lag"
          value="${esc(q)}" data-rb-search aria-label="搜索排障剧本" />
      </div>
      <span class="rb-count">${RUNBOOK_COUNT} 个场景${
        q ? ` · 命中 ${hit.length} 个` : ''}</span>
    </div>

    <div class="rb-grid" data-rb-grid>
      ${hit.length ? hit.map(runbookCardHTML).join('') : `
        <div class="card card-pad" style="grid-column:1/-1;text-align:center;padding:40px">
          <p style="font-size:14px">没有匹配「${esc(q)}」的剧本</p>
          <p style="font-size:12.5px;color:var(--text-3);margin-top:6px">
            试试更短的关键字，或者直接看下面的全部场景。</p>
          <a class="btn btn-sm" href="#/runbook" style="margin-top:14px">看全部</a>
        </div>`}
    </div>

    <p class="rb-foot">${icon('info', 12)}
      剧本里的命令用 &lt;占位符&gt; 标出要替换的部分，直接复制前请先替换成实际值。</p>
  `;

  const mount = (root) => {
    const input = root.querySelector('[data-rb-search]');
    if (!input) return;
    let timer = 0;
    input.addEventListener('input', () => {
      // 输入即过滤，但不改 hash：改 hash 会整页重渲染，输入框会失焦。
      clearTimeout(timer);
      timer = setTimeout(() => {
        const kw = input.value.trim();
        const list = kw ? filterRunbooks(kw) : RUNBOOKS;
        const grid = root.querySelector('[data-rb-grid]');
        if (!grid) return;
        grid.innerHTML = list.length
          ? list.map(runbookCardHTML).join('')
          : `<div class="card card-pad" style="grid-column:1/-1;text-align:center;padding:40px">
              <p style="font-size:14px">没有匹配「${esc(kw)}」的剧本</p>
              <p style="font-size:12.5px;color:var(--text-3);margin-top:6px">试试更短的关键字</p>
            </div>`;
        const counter = root.querySelector('.rb-count');
        if (counter) counter.textContent = `${RUNBOOK_COUNT} 个场景 · 命中 ${list.length} 个`;
      }, 180);
    });
  };

  return { html, title: '排障剧本', mount };
}

function filterRunbooks(kw) {
  const low = kw.toLowerCase();
  return RUNBOOKS.filter((rb) => {
    const hay = [rb.title, rb.summary, rb.scene, ...(rb.keywords || [])].join(' ').toLowerCase();
    return hay.includes(low);
  });
}

function runbookCardHTML(rb) {
  return `<a class="card rb-card" href="#/runbook/${esc(rb.slug)}">
    <span class="rb-card-top">
      <span class="rb-card-ic">${icon(rb.icon || 'alert', 18)}</span>
      <span class="chip ${rb.severity === 'high' ? 'chip-danger' : 'chip-warn'}">
        ${esc(SEVERITY_LABEL[rb.severity] || rb.severity)}
      </span>
    </span>
    <span class="rb-card-title">${esc(rb.title)}</span>
    <span class="rb-card-sum">${esc(rb.summary)}</span>
    <span class="rb-card-foot">
      <span class="chip">${esc(rb.scene)}</span>
      <span class="rb-card-steps">${rb.steps.filter((s) => !s.terminal).length} 步</span>
    </span>
  </a>`;
}

/* ====================================================== 详情页 ======== */

export function runbookDetailPage(ctx, slug) {
  validateRunbooks();

  const rb = RUNBOOKS.find((r) => r.slug === slug);
  if (!rb) {
    return {
      html: `<nav class="crumb"><a href="#/runbook">排障剧本</a></nav>
        <div class="card card-pad">
          <p>没有这个剧本，它可能已经改名了。</p>
          <a class="btn" href="#/runbook">返回列表</a>
        </div>`,
      title: '排障剧本',
    };
  }

  // 路径来自 hash；过滤掉不存在的 id，避免手改 URL 后走进空步骤。
  const byId = new Map(rb.steps.map((s) => [s.id, s]));
  const raw = (ctx.params.get('p') || '').split(',').map((x) => x.trim()).filter(Boolean);
  const path = raw.filter((id) => byId.has(id));
  if (!path.length) path.push(rb.entry);

  const cur = byId.get(path[path.length - 1]);
  const trail = path.map((id) => byId.get(id));

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a><span class="crumb-sep">/</span>
      <a href="#/runbook">排障剧本</a><span class="crumb-sep">/</span>
      <span>${esc(rb.title)}</span>
    </nav>

    <div class="rb-head">
      <div>
        <h1 class="rb-title">${esc(rb.title)}</h1>
        <p class="rb-sum">${esc(rb.summary)}</p>
      </div>
      <div class="rb-head-tags">
        <span class="chip ${rb.severity === 'high' ? 'chip-danger' : 'chip-warn'}">
          ${esc(SEVERITY_LABEL[rb.severity] || rb.severity)}</span>
        <span class="chip">${esc(rb.scene)}</span>
        <a class="btn btn-sm" href="#/runbook/${esc(rb.slug)}">${icon('refresh', 13)}重新开始</a>
      </div>
    </div>

    ${trailHTML(rb, trail, path)}

    ${cur.terminal ? terminalHTML(cur) : stepHTML(cur, path, rb.slug)}

    <p class="rb-foot">${icon('info', 12)}
      判断依据来自现象，不是猜测。拿不准就按"不确定"那条分支走，多查一步比猜错方向省时间。</p>
  `;

  return {
    html,
    title: `${rb.title} · 排障剧本`,
    mount: (root) => bindRunbookCopy(root),
  };
}

/** 已走过的路径：点任意一步可以回退到那里，后面的选择会被截断。 */
function trailHTML(rb, trail, path) {
  if (trail.length <= 1) return '';
  return `<ol class="rb-trail">
    ${trail.slice(0, -1).map((s, i) => `<li>
      <a href="#/runbook/${esc(rb.slug)}?p=${esc(path.slice(0, i + 1).join(','))}">
        ${esc(shortTitle(s.title))}</a>
    </li>`).join('')}
    <li class="is-current">${esc(shortTitle(trail[trail.length - 1].title))}</li>
  </ol>`;
}

function stepHTML(step, path, slug) {
  const nextBase = path.join(',');
  return `<div class="card rb-step">
    <div class="rb-step-body">
      <h2 class="rb-step-title">${esc(step.title)}</h2>
      <p class="rb-step-detail">${esc(step.detail)}</p>
      ${step.cmd ? `<div class="rb-cmd"><code>${esc(step.cmd)}</code>
        <button class="btn btn-xs rb-copy" type="button" data-copy="${esc(step.cmd)}"
          data-copy-done="已复制">复制</button></div>` : ''}
      ${step.tip ? `<p class="rb-tip">${icon('info', 12)}${esc(step.tip)}</p>` : ''}
    </div>
    <div class="rb-branches">
      <p class="rb-branches-head">看到什么？</p>
      ${(step.branches || []).map((b) => `<a class="rb-branch"
        href="#/runbook/${esc(slug)}?p=${esc(nextBase)},${esc(b.goto)}" data-branch="${esc(b.goto)}">
        <span class="rb-branch-when">${esc(b.when)}</span>
        ${b.hint ? `<span class="rb-branch-hint">${esc(b.hint)}</span>` : ''}
        <span class="rb-branch-go">${icon('right', 14)}</span>
      </a>`).join('')}
    </div>
  </div>`;
}

function terminalHTML(step) {
  return `<div class="card rb-terminal">
    <div class="rb-terminal-head">
      <span class="rb-terminal-ic">${icon('check', 18)}</span>
      <h2>${esc(step.title)}</h2>
    </div>
    <p class="rb-step-detail">${esc(step.detail)}</p>

    <div class="rb-block">
      <h3>根因</h3>
      <p>${esc(step.rootCause || '')}</p>
    </div>

    ${(step.fix || []).length ? `<div class="rb-block">
      <h3>怎么修</h3>
      <ol class="rb-fix">
        ${step.fix.map((f) => `<li>${esc(f)}</li>`).join('')}
      </ol>
    </div>` : ''}

    ${step.verify ? `<div class="rb-block rb-verify">
      <h3>验证</h3>
      <p><code>${esc(step.verify)}</code></p>
    </div>` : ''}
  </div>`;
}

/** 面包屑里只放短标题：完整标题是"在确认什么"的长句式，横向会溢出。 */
function shortTitle(t) {
  const s = String(t || '');
  const cut = s.split(/[，,：:（(]/)[0];
  return cut.length > 18 ? `${cut.slice(0, 18)}…` : cut;
}

/* ====================================================== 挂载增强 ====== */

/** 复制命令：与速查手册同一套降级逻辑（非 https 下 clipboard API 不可用）。 */
export function bindRunbookCopy(root) {
  root.querySelectorAll('[data-copy]').forEach((btn) => {
    if (btn.dataset.bound) return;
    btn.dataset.bound = '1';
    btn.addEventListener('click', async () => {
      const text = btn.getAttribute('data-copy') || '';
      const done = btn.getAttribute('data-copy-done') || '已复制';
      const ok = await copyText(text);
      if (!ok) return;
      const old = btn.textContent;
      btn.textContent = done;
      btn.classList.add('is-done');
      setTimeout(() => {
        btn.textContent = old;
        btn.classList.remove('is-done');
      }, 1400);
    });
  });
}

async function copyText(text) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch { /* 落到兼容分支 */ }
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
