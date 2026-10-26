/* ============================================================================
   pages-article.js — 教程与资讯两个栏目的列表页，以及文章详情页。
   两个栏目共用同一套模板：它们在服务端就是同一张表，差别只在栏目 slug 与文案。
   写成两份迟早会分叉，最后变成"教程页有分页、资讯页没有"这种琐碎差异。
   ========================================================================= */

import { api, isLoggedIn, isAdmin, ApiError } from './api.js';
import { esc, fmtNum, fmtDate, icon, renderMarkdown, safeUrl, setHashQuery, adminEditHTML } from './util.js';
import { postCardHTML } from './cards.js';
import { bindPager, chipsHTML, emptyHTML, pagerHTML, toastError } from './ui.js';

const PAGE_SIZE = 10;

const SORTS = [
  { key: 'latest', label: '最新' },
  { key: 'hot', label: '最热' },
  { key: 'like', label: '最多赞' },
];

/* ======================================================== 列表页 ====== */

export async function articleListPage(ctx, channelSlug) {
  const params = ctx.params;
  const page = Math.max(1, Number(params.get('page') || 1));
  const sort = SORTS.some((s) => s.key === params.get('sort')) ? params.get('sort') : 'latest';
  const categoryId = params.get('categoryId') || '';
  const tag = params.get('tag') || '';
  const keyword = params.get('keyword') || '';

  const channel = findChannel(ctx, channelSlug, channelSlug);

  // 分类用于筛选条；它失败不该让正文列表也打不开，所以单独 catch。
  const [categories, result] = await Promise.all([
    api.categories().catch(() => []),
    api.articles({
      channel: channelSlug,
      page,
      pageSize: PAGE_SIZE,
      sort,
      categoryId: categoryId || undefined,
      tag: tag || undefined,
      keyword: keyword || undefined,
    }),
  ]);

  const list = result.list || [];
  const total = result.total || 0;
  const isNews = channelSlug === 'ops-news';

  const html = `
    <div class="page-head">
      <h1>${esc(channel.name)}</h1>
      <p class="lede">${esc(channel.desc || '')}</p>
    </div>

    <div class="toolbar">
      <select class="select" data-filter="categoryId" aria-label="按分类筛选">
        <option value="">全部分类</option>
        ${(categories || []).map((c) => `
          <option value="${esc(c.id)}"${String(c.id) === String(categoryId) ? ' selected' : ''}>
            ${esc(c.name)}（${c.articleCount}）
          </option>`).join('')}
      </select>

      <div class="seg" role="group" aria-label="排序方式">
        ${SORTS.map((s) => `<button type="button" data-sort="${s.key}"
          class="${s.key === sort ? 'is-active' : ''}">${s.label}</button>`).join('')}
      </div>

      <span class="toolbar-spacer"></span>
      <span class="meta-item" style="font-size:12.5px;color:var(--text-3)">共 ${fmtNum(total)} 篇</span>
    </div>

    ${(tag || keyword) ? `<div class="toolbar" style="margin-top:-8px">
      ${tag ? `<button class="chip chip-brand" type="button" data-clear="tag">
        标签：${esc(tag)} ${icon('close', 12)}
      </button>` : ''}
      ${keyword ? `<button class="chip chip-brand" type="button" data-clear="keyword">
        关键词：${esc(keyword)} ${icon('close', 12)}
      </button>` : ''}
    </div>` : ''}

    <div class="post-list">
      ${list.length
        ? list.map(postCardHTML).join('')
        : emptyHTML({
            iconName: isNews ? 'news' : 'book',
            title: tag || keyword ? '没有匹配的内容' : `${channel.name}还没有内容`,
            desc: tag || keyword
              ? '换个关键词或清掉筛选条件再试试。'
              : isNews
                ? '版本发布与安全公告会陆续出现在这里。'
                : '教程正在整理中，稍后再来看看。',
            actionHTML: (tag || keyword)
              ? `<button class="btn btn-sm" type="button" data-act="clear-all">清空筛选</button>`
              : '',
          })}
    </div>

    ${pagerHTML({ page, pageSize: PAGE_SIZE, total, unit: '篇' })}
  `;

  const mount = (root) => {
    const go = (patch, keepPage = false) => {
      setHashQuery(patch, { keepPage });
    };

    root.querySelectorAll('[data-sort]').forEach((btn) => {
      btn.addEventListener('click', () => go({ sort: btn.getAttribute('data-sort') }));
    });

    const select = root.querySelector('[data-filter="categoryId"]');
    if (select) {
      select.addEventListener('change', () => go({ categoryId: select.value }));
    }

    root.querySelectorAll('[data-clear]').forEach((btn) => {
      btn.addEventListener('click', () => go({ [btn.getAttribute('data-clear')]: '' }));
    });

    const clearAll = root.querySelector('[data-act="clear-all"]');
    if (clearAll) {
      clearAll.addEventListener('click', () => {
        setHashQuery({ tag: '', keyword: '', categoryId: '', sort: '' });
      });
    }

    bindPager(root, (target) => setHashQuery({ page: target }, { keepPage: true }));
  };

  return { html, mount };
}

/* ======================================================== 详情页 ====== */

export async function articleDetailPage(ctx, key) {
  const item = await api.article(key);
  const channel = findChannel(ctx, item.channel, 'linux-tutorial');
  const related = item.related || [];
  const tags = item.tags || [];

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a>
      <span class="crumb-sep">/</span>
      <a href="${esc('#/channel/' + encodeURIComponent(item.channel || 'linux-tutorial'))}">${esc(channel.name)}</a>
      <span class="crumb-sep">/</span>
      <span>${esc(item.title)}</span>
    </nav>

    <header class="detail-head">
      <h1>${esc(item.title)}</h1>
      ${item.summary ? `<p class="lede">${esc(item.summary)}</p>` : ''}
      ${sourceNoticeHTML(item)}
      <div class="detail-meta">
        ${item.authorName ? `<span class="meta-item">${icon('user', 14)}${esc(item.authorName)}</span>` : ''}
        <span class="meta-item">${icon('calendar', 14)}${esc(fmtDate(item.publishedAt))}</span>
        <span class="meta-item">${icon('eye', 14)}${fmtNum(item.viewCount)} 次阅读</span>
        ${item.categoryName ? `<span class="chip">${esc(item.categoryName)}</span>` : ''}
        ${item.sourceName ? `<span class="chip chip-source">${icon('link', 11)}${esc(item.sourceName)}</span>` : ''}
        ${isAdmin() ? adminEditHTML(`#/admin/articles/${item.id}`) : ''}
        <button class="btn btn-sm ${item.liked ? 'chip-danger' : ''}" type="button" data-like
          style="margin-left:auto">
          ${icon('heart', 15)}<span data-like-count>${fmtNum(item.likeCount)}</span>
        </button>
      </div>
    </header>

    <div class="detail-layout">
      <article class="prose" data-content>
        ${renderMarkdown(item.content)}
      </article>

      <aside>
        ${tags.length ? `<div class="card side-card" style="margin-bottom:16px">
          <h3>标签</h3>
          ${chipsHTML(tags)}
        </div>` : ''}

        ${item.seo && (item.seo.description || item.seo.keywords) ? `<div class="card side-card" style="margin-bottom:16px">
          <h3>内容信息</h3>
          <dl class="kv">
            ${item.seo.keywords ? kvRow('关键词', item.seo.keywords) : ''}
            ${item.source === 'order' ? kvRow('来源', '订单案例沉淀') : kvRow('来源', '人工撰写')}
          </dl>
        </div>` : ''}

        ${related.length ? `<div class="card side-card">
          <h3>相关推荐</h3>
          <div style="display:flex;flex-direction:column;gap:12px">
            ${related.map(relatedItemHTML).join('')}
          </div>
        </div>` : ''}
      </aside>
    </div>
  `;

  const mount = (root) => {
    const btn = root.querySelector('[data-like]');
    if (!btn) return;

    btn.addEventListener('click', async () => {
      if (!isLoggedIn()) {
        toastError('点赞需要先登录');
        // 交给外层路由处理登录弹窗
        window.dispatchEvent(new CustomEvent('opskb:need-login'));
        return;
      }
      btn.disabled = true;
      try {
        const res = await api.toggleLike(item.slug || item.id);
        const countEl = btn.querySelector('[data-like-count]');
        if (countEl) countEl.textContent = fmtNum(res.likeCount);
        btn.classList.toggle('chip-danger', Boolean(res.liked));
        item.liked = res.liked;
      } catch (err) {
        toastError(err instanceof ApiError ? err.message : '点赞失败，请稍后再试');
      } finally {
        btn.disabled = false;
      }
    });
  };

  return { html, mount };
}

/* ------------------------------------------------------------ 内部 -- */

function relatedItemHTML(item) {
  return `<a href="${esc('#/article/' + encodeURIComponent(item.slug || item.id))}"
    style="display:block;color:inherit">
    <div style="font-size:13.2px;line-height:1.6;font-weight:500">${esc(item.title)}</div>
    <div style="font-size:11.5px;color:var(--text-4);margin-top:4px">
      ${esc(fmtRelativeShort(item.publishedAt))}
    </div>
  </a>`;
}

function kvRow(label, value) {
  return `<div class="kv-row"><dt>${esc(label)}</dt><dd>${esc(value)}</dd></div>`;
}

function fmtRelativeShort(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

function findChannel(ctx, slug, fallbackName) {
  const hit = (ctx.state.channels || []).find((c) => c.slug === slug);
  return hit || { slug, name: fallbackName, desc: '' };
}

/* ------------------------------------------------------- 转载声明 -- */

// sourceNoticeHTML 渲染「本文转述自外部源，点此读原文」的说明条。
//
// 抓取来的资讯必须在正文之前把出处交代清楚，两个原因：
// 一是版权与引用礼貌，二是用户看到"某组件发布 3.0"时要知道这是官方公告
// 还是平台的解读——两者的可信度与时效责任完全不同。
function sourceNoticeHTML(item) {
  const name = String(item.sourceName || '').trim();
  if (!name) return '';

  const url = safeUrl(item.sourceUrl);
  const lead = `本文转述自 <strong>${esc(name)}</strong>，版权归原作者所有。`;
  const link = url
    ? ` <a href="${esc(url)}" target="_blank" rel="noopener noreferrer">查看原文${icon('right', 11)}</a>`
    : '';

  return `<div class="source-notice">
    ${icon('info', 14)}
    <span>${lead}${link}</span>
  </div>`;
}
