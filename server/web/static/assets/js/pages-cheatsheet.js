/* ============================================================================
   pages-cheatsheet.js — 速查手册。
   工位常开的页面：按分类浏览、全文搜索、一键复制。全部本地计算，零请求。
   初始内容在页面函数里就渲染好（不空壳等挂载），搜索/切分类时由 mount 重绘。
   ========================================================================= */

import { esc, icon } from './util.js';
import { SHEETS } from './cheatsheet-data.js';
import { FLAGS } from './flags.js';

/** 展开后的扁平条目（带所属分类），搜索用。 */
const ALL_ITEMS = SHEETS.flatMap((s) =>
  s.groups.flatMap((g) =>
    g.items.map((it) => ({ ...it, sheet: s.slug, sheetName: s.name, groupName: g.name }))));

export const CHEATSHEET_COUNT = ALL_ITEMS.length;

/** 按分类与关键词产出主体 HTML。页面函数与挂载后的重绘共用这一份逻辑。 */
function bodyHTML(currentSlug, query) {
  const sheet = SHEETS.find((s) => s.slug === currentSlug);
  const pool = sheet ? ALL_ITEMS.filter((it) => it.sheet === sheet.slug) : ALL_ITEMS;
  const q = (query || '').trim().toLowerCase();
  const hits = q
    ? pool.filter((it) =>
        it.cmd.toLowerCase().includes(q) ||
        it.desc.toLowerCase().includes(q) ||
        it.sheetName.toLowerCase().includes(q) ||
        it.groupName.toLowerCase().includes(q))
    : pool;

  if (!hits.length) {
    return `
      <div class="empty">
        <p class="empty-title">没有匹配的命令</p>
        <p class="empty-desc">换个说法试试，比如搜「日志」「内存」「证书」。</p>
      </div>`;
  }

  // 搜索时按分类再聚合一次，让结果仍带上下文；浏览时保持原有分组顺序。
  const bySheet = new Map();
  for (const it of hits) {
    if (!bySheet.has(it.sheet)) bySheet.set(it.sheet, []);
    bySheet.get(it.sheet).push(it);
  }

  return [...bySheet.entries()].map(([slug, items]) => {
    const meta = SHEETS.find((s) => s.slug === slug);
    return `
      <section class="cs-sheet">
        <h2 class="cs-sheet-name">${esc(meta.name)}
          <span class="cs-sheet-count">${items.length} 条</span></h2>
        <div class="cs-list">
          ${items.map((it) => `
            <div class="cs-row">
              <div class="cs-main">
                <code class="cs-cmd">${esc(it.cmd)}</code>
                <span class="cs-desc">${esc(it.desc)}</span>
              </div>
              <button class="btn btn-sm cs-copy" type="button" data-copy="${esc(it.cmd)}"
                      title="复制命令" aria-label="复制命令">${icon('list', 13)}复制</button>
            </div>`).join('')}
        </div>
      </section>`;
  }).join('');
}

export async function cheatsheetPage(ctx) {
  const current = ctx.params.get('c') || 'all';
  const query = (ctx.params.get('q') || '').trim();
  const valid = current === 'all' || SHEETS.some((s) => s.slug === current);
  const cat = valid ? current : 'all';

  const html = `
    <div class="page-head">
      <h1>速查手册</h1>
      <p class="lede">运维高频命令一页纸，点命令即复制。${CHEATSHEET_COUNT} 条实战命令，持续补充。</p>
    </div>

    <div class="toolbar">
      <input class="input cs-search" data-cs-search type="search"
             placeholder="搜命令或场景，如：磁盘、慢查询、证书…"
             value="${esc(query)}" aria-label="搜索速查命令" />
      <button class="btn btn-sm ${cat === 'all' ? 'btn-primary' : ''}" type="button"
              data-cs-cat="all">全部</button>
      ${SHEETS.map((s) => `
        <button class="btn btn-sm ${s.slug === cat ? 'btn-primary' : ''}" type="button"
                data-cs-cat="${s.slug}">${esc(s.name)}</button>`).join('')}
      ${FLAGS.checklist ? `<a class="btn btn-sm cs-check" href="#/checklist"
         title="交班前过一遍清单">${icon('shield', 13)}今日巡检</a>` : ''}
    </div>

    <div class="cs-body" data-cs-body>${bodyHTML(cat, query)}</div>
  `;

  const mount = (root) => {
    const body = root.querySelector('[data-cs-body]');
    const search = root.querySelector('[data-cs-search]');
    const activeSlug = () => {
      const on = root.querySelector('.toolbar [data-cs-cat].btn-primary');
      return on ? on.getAttribute('data-cs-cat') : 'all';
    };

    // 复制：优先剪贴板 API，非安全上下文（http）退回 execCommand 兜底。
    body.addEventListener('click', async (e) => {
      const btn = e.target.closest('[data-copy]');
      if (!btn) return;
      const text = btn.getAttribute('data-copy');
      let ok = false;
      try {
        await navigator.clipboard.writeText(text);
        ok = true;
      } catch {
        const ta = document.createElement('textarea');
        ta.value = text;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        ok = document.execCommand('copy');
        ta.remove();
      }
      const old = btn.innerHTML;
      btn.innerHTML = ok ? `${icon('check', 13)}已复制` : `${icon('close', 13)}失败`;
      btn.classList.add('is-done');
      setTimeout(() => { btn.innerHTML = old; btn.classList.remove('is-done'); }, 1200);
    });

    // 搜索防抖 200ms：速查是边打边看的页面，延迟必须比表单短。
    // 搜索只重绘主体，不刷地址——键盘党不被光标丢失打断。
    let timer = 0;
    search.addEventListener('input', () => {
      clearTimeout(timer);
      timer = setTimeout(() => { body.innerHTML = bodyHTML(activeSlug(), search.value); }, 200);
    });

    // 切分类：改地址让筛选可分享，hashchange 会触发整页重路由。
    root.querySelectorAll('[data-cs-cat]').forEach((btn) => {
      btn.addEventListener('click', () => {
        const slug = btn.getAttribute('data-cs-cat');
        location.hash = slug === 'all' ? '#/cheatsheet' : `#/cheatsheet?c=${slug}`;
      });
    });
  };

  return { html, mount, title: '速查手册' };
}
