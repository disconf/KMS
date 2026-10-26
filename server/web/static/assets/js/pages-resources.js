/* ============================================================================
   pages-resources.js — 资源下载：列表（按文件类型筛选）与详情（含签名下载）。
   ========================================================================= */

import { api, isLoggedIn, isAdmin, ApiError } from './api.js';
import { esc, fmtNum, fmtSize, fmtDate, icon, renderMarkdown, safeUrl, setHashQuery, adminEditHTML } from './util.js';
import { resourceItemHTML, FILE_KIND_LABEL } from './cards.js';
import { bindPager, chipsHTML, emptyHTML, pagerHTML } from './ui.js';

const PAGE_SIZE = 12;

const SORTS = [
  { key: 'latest', label: '最新' },
  { key: 'download', label: '下载最多' },
  { key: 'hot', label: '浏览最多' },
];

/* ======================================================== 列表页 ====== */

export async function resourceListPage(ctx) {
  const params = ctx.params;
  const page = Math.max(1, Number(params.get('page') || 1));
  const sort = SORTS.some((s) => s.key === params.get('sort')) ? params.get('sort') : 'latest';
  const fileExt = params.get('fileExt') || '';
  const keyword = params.get('keyword') || '';

  const channel = findChannel(ctx, 'resources');

  const [exts, result] = await Promise.all([
    api.resourceExts().catch(() => []),
    api.resources({
      page,
      pageSize: PAGE_SIZE,
      sort,
      fileExt: fileExt || undefined,
      keyword: keyword || undefined,
    }),
  ]);

  const list = result.list || [];
  const total = result.total || 0;
  const hasFilter = Boolean(fileExt || keyword);

  const html = `
    <div class="page-head">
      <h1>${esc(channel.name)}</h1>
      <p class="lede">${esc(channel.desc || '')}</p>
    </div>

    <div class="toolbar">
      <select class="select" data-filter="fileExt" aria-label="按文件类型筛选">
        <option value="">全部文件类型</option>
        ${(exts || []).map((v) => `
          <option value="${esc(v)}"${v === fileExt ? ' selected' : ''}>
            ${esc(FILE_KIND_LABEL[v] || v.toUpperCase())}
          </option>`).join('')}
      </select>

      <div class="seg" role="group" aria-label="排序方式">
        ${SORTS.map((s) => `<button type="button" data-sort="${s.key}"
          class="${s.key === sort ? 'is-active' : ''}">${s.label}</button>`).join('')}
      </div>

      <span class="toolbar-spacer"></span>
      <span style="font-size:12.5px;color:var(--text-3)">共 ${fmtNum(total)} 份资源</span>
    </div>

    ${hasFilter ? `<div class="toolbar" style="margin-top:-8px">
      ${fileExt ? `<button class="chip chip-brand" type="button" data-clear="fileExt">
        类型：${esc(FILE_KIND_LABEL[fileExt] || fileExt.toUpperCase())} ${icon('close', 12)}
      </button>` : ''}
      ${keyword ? `<button class="chip chip-brand" type="button" data-clear="keyword">
        关键词：${esc(keyword)} ${icon('close', 12)}
      </button>` : ''}
    </div>` : ''}

    <div class="res-list">
      ${list.length
        ? list.map(resourceItemHTML).join('')
        : emptyHTML({
            iconName: 'package',
            title: hasFilter ? '没有匹配的资源' : '还没有可下载的资源',
            desc: hasFilter ? '换个文件类型或关键词再试试。' : '脚本与配置模板正在整理中。',
            actionHTML: hasFilter ? '<button class="btn btn-sm" type="button" data-act="reset">清空筛选</button>' : '',
          })}
    </div>

    ${pagerHTML({ page, pageSize: PAGE_SIZE, total, unit: '份' })}
  `;

  const mount = (root) => {
    root.querySelectorAll('[data-sort]').forEach((btn) => {
      btn.addEventListener('click', () => setHashQuery({ sort: btn.getAttribute('data-sort') }));
    });

    root.querySelectorAll('[data-filter]').forEach((sel) => {
      sel.addEventListener('change', () => setHashQuery({ [sel.getAttribute('data-filter')]: sel.value }));
    });

    root.querySelectorAll('[data-clear]').forEach((btn) => {
      btn.addEventListener('click', () => setHashQuery({ [btn.getAttribute('data-clear')]: '' }));
    });

    const reset = root.querySelector('[data-act="reset"]');
    if (reset) reset.addEventListener('click', () => setHashQuery({ fileExt: '', keyword: '', sort: '' }));

    bindPager(root, (target) => setHashQuery({ page: target }, { keepPage: true }));
  };

  return { html, mount };
}

/* ======================================================== 详情页 ====== */

export async function resourceDetailPage(ctx, key) {
  const item = await api.resource(key);
  const channel = findChannel(ctx, 'resources');
  const related = item.related || [];
  const tags = item.tags || [];
  const ext = String(item.fileExt || '').toLowerCase();
  // 没有托管文件的资源（只给外链）不该被标成"文件"——用户会以为点下载能拿到东西
  const kind = FILE_KIND_LABEL[ext] || (item.hasFile ? '文件' : '外部资源');

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a>
      <span class="crumb-sep">/</span>
      <a href="#/resources">${esc(channel.name)}</a>
      <span class="crumb-sep">/</span>
      <span>${esc(item.title)}</span>
    </nav>

    <header class="detail-head">
      <div style="display:flex;align-items:flex-start;gap:16px">
        <span class="res-glyph ext-${esc(ext)}" style="width:54px;height:54px;border-radius:13px;font-size:13px">
          ${esc(ext ? ext.toUpperCase().slice(0, 5) : 'FILE')}
        </span>
        <div style="flex:1;min-width:0">
          <h1>${esc(item.title)}</h1>
          ${item.summary ? `<p class="lede">${esc(item.summary)}</p>` : ''}
        </div>
      </div>

      <div class="detail-meta">
        <span class="chip">${esc(kind)}</span>
        ${item.version ? `<span class="chip chip-mono">${esc(item.version)}</span>` : ''}
        ${item.hasFile ? `<span class="meta-item">${icon('package', 14)}${esc(fmtSize(item.fileSize))}</span>` : ''}
        ${item.hasFile ? `<span class="meta-item">${icon('download', 14)}${fmtNum(item.downloadCount)} 次下载</span>` : ''}
        <span class="meta-item">${icon('eye', 14)}${fmtNum(item.viewCount)} 次浏览</span>
        ${item.needLogin && item.hasFile ? '<span class="chip chip-warn">需登录下载</span>' : ''}
        ${isAdmin() ? `<span style="margin-left:auto">${adminEditHTML(`#/admin/resources/${item.id}/links`, '管理下载链接')}</span>` : ''}
      </div>
    </header>

    <div class="detail-layout">
      <article class="prose">
        ${item.content
          ? renderMarkdown(item.content)
          : '<p style="color:var(--text-3)">这份资源还没有补充使用说明。</p>'}
      </article>

      <aside>
        <div class="card side-card" style="margin-bottom:16px">
          <h3>${item.hasFile ? '下载' : '获取方式'}</h3>
          ${item.hasFile ? `<button class="btn btn-primary btn-block btn-lg" type="button" data-download>
            ${icon('download', 16)}<span data-download-label>${item.needLogin && !isLoggedIn() ? '登录后下载' : '下载文件'}</span>
          </button>
          <p style="font-size:12px;color:var(--text-4);margin-top:10px;line-height:1.6"
             data-download-hint>
            ${item.needLogin && !isLoggedIn()
              ? '这份资源需要登录后获取下载地址。'
              : '下载地址带时效签名，仅用于本次下载。'}
          </p>` : ''}
          ${linksHTML(item.links, item.hasFile)}
        </div>

        <div class="card side-card" style="margin-bottom:16px">
          <h3>${item.hasFile ? '文件信息' : '资源信息'}</h3>
          <dl class="kv">
            ${item.hasFile ? kvRow('文件名', item.fileName || '—', true) : ''}
            ${item.hasFile ? kvRow('文件大小', fmtSize(item.fileSize)) : ''}
            ${kvRow('类型', kind)}
            ${item.version ? kvRow('版本', item.version, true) : ''}
            ${item.compatibleWith ? kvRow('适用环境', item.compatibleWith) : ''}
            ${item.categoryName ? kvRow('分类', item.categoryName) : ''}
            ${kvRow('发布时间', fmtDate(item.publishedAt))}
          </dl>
        </div>

        ${item.checksum ? `<div class="card side-card" style="margin-bottom:16px">
          <h3>校验和（SHA256）</h3>
          <p style="font-family:var(--font-mono);font-size:11.5px;word-break:break-all;
                    line-height:1.7;color:var(--text-2);margin:0;user-select:all">
            ${esc(item.checksum)}
          </p>
          <p style="font-size:11.5px;color:var(--text-4);margin-top:9px;line-height:1.6">
            下载后可用 <code style="font-family:var(--font-mono)">sha256sum</code> 比对，
            确认文件在传输中没有被改动。
          </p>
        </div>` : ''}

        ${tags.length ? `<div class="card side-card" style="margin-bottom:16px">
          <h3>标签</h3>
          ${chipsHTML(tags)}
        </div>` : ''}

        ${related.length ? `<div class="card side-card">
          <h3>相关资源</h3>
          <div style="display:flex;flex-direction:column;gap:12px">
            ${related.map(relatedItemHTML).join('')}
          </div>
        </div>` : ''}
      </aside>
    </div>
  `;

  const mount = (root) => {
    const btn = root.querySelector('[data-download]');
    if (!btn) return;
    const label = btn.querySelector('[data-download-label]');
    const hint = root.querySelector('[data-download-hint]');

    btn.addEventListener('click', async () => {
      // 明知要登录就先别发请求：让用户点一下再弹登录，比先看一次报错体验好
      if (item.needLogin && !isLoggedIn()) {
        window.dispatchEvent(new CustomEvent('opskb:need-login'));
        if (hint) hint.textContent = '登录后即可获取这份资源的下载地址。';
        return;
      }

      btn.disabled = true;
      if (label) label.textContent = '正在获取下载地址…';
      try {
        const res = await api.resourceDownload(item.slug || item.id);
        if (label) label.textContent = '开始下载';
        if (hint) {
          hint.textContent = `已下发 ${res.fileName || '文件'}，若浏览器没有自动开始，请再点一次。`;
        }
        triggerDownload(res.url);
      } catch (err) {
        if (label) label.textContent = '下载文件';
        if (err instanceof ApiError && (err.code === 40031 || err.status === 401)) {
          window.dispatchEvent(new CustomEvent('opskb:need-login'));
          if (hint) hint.textContent = '这份资源需要登录后获取下载地址。';
        } else {
          if (hint) hint.textContent = err instanceof ApiError ? err.message : '获取下载地址失败，请稍后再试。';
        }
      } finally {
        btn.disabled = false;
      }
    });
  };

  return { html, mount };
}

/** 用隐藏的 a 触发下载，比 location.href 更不容易打断当前页面。 */
function triggerDownload(url) {
  const safe = String(url || '').trim();
  if (!safe) return;
  const a = document.createElement('a');
  a.href = safe;
  a.rel = 'noopener';
  a.style.display = 'none';
  document.body.appendChild(a);
  a.click();
  setTimeout(() => a.remove(), 0);
}

/* ------------------------------------------------------------ 内部 -- */

// LINK_KIND_LABEL 与后端 model.ResourceLink* 常量逐字对齐
// （official / mirror / package / repo / image / other）。
// 不认识的取值直接不显示徽标，而不是把内部枚举名摆在用户面前。
const LINK_KIND_LABEL = {
  official: '官网',
  mirror: '镜像',
  package: '包管理',
  repo: '代码仓库',
  image: '容器镜像',
  other: '',
};

// linksHTML 渲染资源的外部下载地址。
//
// 为什么需要这个：安装包动辄几百 MB、版本更新频繁，平台自己托管既不划算也容易过期。
// 这类资源只给「去哪下」，不提供托管文件——所以详情页必须能区分两种形态：
// 有文件的走签名下载，没文件的把链接摆出来。
function linksHTML(links, hasFile) {
  const list = (Array.isArray(links) ? links : []).filter((l) => safeUrl(l.url));
  if (!list.length) {
    // 既没有文件也没有外链：这是数据问题，但不该整页报错，给一句可读的提示
    return hasFile ? '' : `<p style="font-size:12px;color:var(--text-4);line-height:1.6">
      这份资源暂未提供下载地址，正在补充中。</p>`;
  }

  return `<div class="res-links">
    ${hasFile ? '<div class="res-links-head">也可以从官方渠道获取</div>' : ''}
    ${list.map((l) => {
      const badge = LINK_KIND_LABEL[l.kind] || '';
      return `<a class="res-link" href="${esc(safeUrl(l.url))}" target="_blank" rel="noopener noreferrer">
        <span class="res-link-icon">${icon('link', 14)}</span>
        <span class="res-link-body">
          <span class="res-link-label">${esc(l.label || '下载地址')}${badge ? `<span class="res-link-kind">${esc(badge)}</span>` : ''}</span>
          ${l.note ? `<span class="res-link-note">${esc(l.note)}</span>` : ''}
        </span>
        ${icon('right', 13)}
      </a>`;
    }).join('')}
  </div>`;
}

function relatedItemHTML(item) {
  const ext = String(item.fileExt || '').toUpperCase();
  return `<a href="${esc('#/resources/' + encodeURIComponent(item.slug || item.id))}"
    style="display:block;color:inherit">
    <div style="font-size:13.2px;font-weight:500">${esc(item.title)}</div>
    <div style="font-size:11.5px;color:var(--text-4);margin-top:4px">
      ${esc(ext)} · ${esc(fmtSize(item.fileSize))}
    </div>
  </a>`;
}

function kvRow(label, value, mono = false) {
  return `<div class="kv-row"><dt>${esc(label)}</dt><dd class="${mono ? 'mono' : ''}">${esc(value)}</dd></div>`;
}

function findChannel(ctx, slug) {
  const hit = (ctx.state.channels || []).find((c) => c.slug === slug);
  return hit || { slug, name: '资源下载', desc: '' };
}
