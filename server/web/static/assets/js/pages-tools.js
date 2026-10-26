/* ============================================================================
   pages-tools.js — 开源工具推荐：列表（按语言 / 协议 / 部署方式筛选）与详情。
   ========================================================================= */

import { api, isAdmin } from './api.js';
import {
  esc, fmtNum, fmtCompact, fmtDate, icon, renderMarkdown, safeUrl, setHashQuery,
  DEPLOY_LABEL, BOOK_LEVEL_LABEL, adminEditHTML,
} from './util.js';
import { toolCardHTML } from './cards.js';
import { bindPager, chipsHTML, emptyHTML, pagerHTML } from './ui.js';

const PAGE_SIZE = 12;

const SORTS = [
  { key: 'latest', label: '最新收录' },
  { key: 'hot', label: '浏览最多' },
  { key: 'stars', label: '星标最多' },
];

/* ======================================================== 列表页 ====== */

export async function toolListPage(ctx) {
  const params = ctx.params;
  const page = Math.max(1, Number(params.get('page') || 1));
  const sort = SORTS.some((s) => s.key === params.get('sort')) ? params.get('sort') : 'latest';
  const language = params.get('language') || '';
  const license = params.get('license') || '';
  const keyword = params.get('keyword') || '';
  const featuredOnly = params.get('featuredOnly') === '1';

  const channel = findChannel(ctx, 'open-source-tools');

  const [filters, result] = await Promise.all([
    api.toolFilters().catch(() => ({ languages: [], licenses: [] })),
    api.tools({
      page,
      pageSize: PAGE_SIZE,
      sort,
      language: language || undefined,
      license: license || undefined,
      keyword: keyword || undefined,
      featuredOnly: featuredOnly || undefined,
    }),
  ]);

  const list = result.list || [];
  const total = result.total || 0;
  const hasFilter = Boolean(language || license || keyword || featuredOnly);

  const html = `
    <div class="page-head">
      <h1>${esc(channel.name)}</h1>
      <p class="lede">${esc(channel.desc || '')}</p>
    </div>

    <div class="toolbar">
      <select class="select" data-filter="language" aria-label="按语言筛选">
        <option value="">全部语言</option>
        ${(filters.languages || []).map((v) => `
          <option value="${esc(v)}"${v === language ? ' selected' : ''}>${esc(v)}</option>`).join('')}
      </select>

      <select class="select" data-filter="license" aria-label="按开源协议筛选">
        <option value="">全部协议</option>
        ${(filters.licenses || []).map((v) => `
          <option value="${esc(v)}"${v === license ? ' selected' : ''}>${esc(v)}</option>`).join('')}
      </select>

      <div class="seg" role="group" aria-label="排序方式">
        ${SORTS.map((s) => `<button type="button" data-sort="${s.key}"
          class="${s.key === sort ? 'is-active' : ''}">${s.label}</button>`).join('')}
      </div>

      <button class="btn btn-sm ${featuredOnly ? 'btn-primary' : ''}" type="button" data-toggle-featured>
        ${icon('star', 14)}只看精选
      </button>

      <span class="toolbar-spacer"></span>
      <span style="font-size:12.5px;color:var(--text-3)">共 ${fmtNum(total)} 个工具</span>
    </div>

    ${(language || license || keyword) ? `<div class="toolbar" style="margin-top:-8px">
      ${language ? `<button class="chip chip-brand" type="button" data-clear="language">语言：${esc(language)} ${icon('close', 12)}</button>` : ''}
      ${license ? `<button class="chip chip-brand" type="button" data-clear="license">协议：${esc(license)} ${icon('close', 12)}</button>` : ''}
      ${keyword ? `<button class="chip chip-brand" type="button" data-clear="keyword">关键词：${esc(keyword)} ${icon('close', 12)}</button>` : ''}
    </div>` : ''}

    ${list.length
      ? `<div class="tool-grid">${list.map(toolCardHTML).join('')}</div>`
      : emptyHTML({
          iconName: 'tools',
          title: hasFilter ? '没有匹配的工具' : '工具库还在补充中',
          desc: hasFilter
            ? '当前筛选条件下没有结果，试着放宽一点条件。'
            : '经过实战验证的工具会陆续收录到这里。',
          actionHTML: hasFilter
            ? `<button class="btn btn-sm" type="button" data-act="reset">清空筛选</button>`
            : '',
        })}

    ${pagerHTML({ page, pageSize: PAGE_SIZE, total, unit: '个' })}
  `;

  const mount = (root) => {
    root.querySelectorAll('[data-sort]').forEach((btn) => {
      btn.addEventListener('click', () => setHashQuery({ sort: btn.getAttribute('data-sort') }));
    });

    root.querySelectorAll('[data-filter]').forEach((sel) => {
      sel.addEventListener('change', () => setHashQuery({ [sel.getAttribute('data-filter')]: sel.value }));
    });

    const featuredBtn = root.querySelector('[data-toggle-featured]');
    if (featuredBtn) {
      featuredBtn.addEventListener('click', () => {
        setHashQuery({ featuredOnly: featuredOnly ? '' : '1' });
      });
    }

    root.querySelectorAll('[data-clear]').forEach((btn) => {
      btn.addEventListener('click', () => setHashQuery({ [btn.getAttribute('data-clear')]: '' }));
    });

    const reset = root.querySelector('[data-act="reset"]');
    if (reset) {
      reset.addEventListener('click', () => {
        setHashQuery({ language: '', license: '', keyword: '', featuredOnly: '', sort: '' });
      });
    }

    bindPager(root, (target) => setHashQuery({ page: target }, { keepPage: true }));
  };

  return { html, mount };
}

/* ======================================================== 详情页 ====== */

export async function toolDetailPage(ctx, key) {
  const item = await api.tool(key);
  const channel = findChannel(ctx, 'open-source-tools');
  const related = item.related || [];
  const tags = item.tags || [];
  const repo = safeUrl(item.repoUrl);
  const homepage = safeUrl(item.homepageUrl);

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a>
      <span class="crumb-sep">/</span>
      <a href="#/tools">${esc(channel.name)}</a>
      <span class="crumb-sep">/</span>
      <span>${esc(item.name)}</span>
    </nav>

    <header class="detail-head">
      <div style="display:flex;align-items:flex-start;gap:16px">
        <span class="tool-logo" style="width:56px;height:56px;border-radius:14px;font-size:20px">
          ${safeUrl(item.logo)
            ? `<img src="${esc(safeUrl(item.logo))}" alt="" />`
            : esc(String(item.name || '?')[0].toUpperCase())}
        </span>
        <div style="flex:1;min-width:0">
          <h1 style="display:flex;align-items:center;gap:9px;flex-wrap:wrap">
            ${esc(item.name)}
            ${item.featured ? '<span class="chip chip-brand">精选</span>' : ''}
          </h1>
          ${item.summary ? `<p class="lede">${esc(item.summary)}</p>` : ''}
        </div>
      </div>

      <div class="detail-meta">
        ${item.language ? `<span class="chip">${esc(item.language)}</span>` : ''}
        ${item.license ? `<span class="chip chip-mono">${esc(item.license)}</span>` : ''}
        <span class="meta-item star-count">${icon('star', 14, 'fill-star')}${fmtCompact(item.stars)}</span>
        <span class="meta-item">${icon('repo', 14)}${fmtCompact(item.forkCount)}</span>
        <span class="meta-item">${icon('eye', 14)}${fmtNum(item.viewCount)}</span>
        <span class="meta-item">${icon('calendar', 14)}${esc(fmtDate(item.publishedAt))}</span>
        ${isAdmin() ? `<span style="margin-left:auto">${adminEditHTML(`#/admin/tools/${item.id}/books`, '管理教程')}</span>` : ''}
      </div>

      ${(repo || homepage) ? `<div style="display:flex;gap:10px;margin-top:16px">
        ${repo ? `<a class="btn btn-primary" href="${esc(repo)}" target="_blank" rel="noopener noreferrer">
          ${icon('repo', 15)}访问仓库</a>` : ''}
        ${homepage ? `<a class="btn" href="${esc(homepage)}" target="_blank" rel="noopener noreferrer">
          ${icon('link', 15)}官方网站</a>` : ''}
      </div>` : ''}
    </header>

    ${guidesHTML(item.guides)}

    ${booksHTML(item.books, item.slug)}

    <div class="detail-layout">
      <article class="prose">
        ${item.content
          ? renderMarkdown(item.content)
          : '<p style="color:var(--text-3)">这个工具还没有补充详细介绍。</p>'}
      </article>

      <aside>
        <div class="card side-card" style="margin-bottom:16px">
          <h3>工具信息</h3>
          <dl class="kv">
            ${kvRow('开发语言', item.language || '—')}
            ${kvRow('开源协议', item.license || '—', true)}
            ${kvRow('Star 数', fmtCompact(item.stars))}
            ${kvRow('Fork 数', fmtCompact(item.forkCount))}
            ${item.categoryName ? kvRow('分类', item.categoryName) : ''}
            ${item.starsUpdated ? kvRow('星标更新', fmtDate(item.starsUpdated)) : ''}
          </dl>
        </div>

        ${(item.deployModes || []).length ? `<div class="card side-card" style="margin-bottom:16px">
          <h3>支持的部署方式</h3>
          <div class="chips">
            ${(item.deployModes || []).map((m) =>
              `<span class="chip chip-brand">${icon('package', 12)}${esc(DEPLOY_LABEL[m] || m)}</span>`).join('')}
          </div>
        </div>` : ''}

        ${tags.length ? `<div class="card side-card" style="margin-bottom:16px">
          <h3>标签</h3>
          ${chipsHTML(tags)}
        </div>` : ''}

        ${related.length ? `<div class="card side-card">
          <h3>相关工具</h3>
          <div style="display:flex;flex-direction:column;gap:12px">
            ${related.map(relatedItemHTML).join('')}
          </div>
        </div>` : ''}
      </aside>
    </div>
  `;

  return { html };
}

/* ------------------------------------------------------------ 内部 -- */

// GUIDE_SECTIONS 是四类教程的展示顺序与文案。
//
// kind 取值必须与后端 model.IsValidToolGuideKind 一致（deploy/implement/optimize/cluster），
// 后端就是按这四个枚举校验的，前端拼错一个字母不会报错、只会让那一整节内容永远不显示。
//
// 顺序刻意按「装上 → 用起来 → 调好 → 撑住规模」的落地先后排，
// 而不是按字母或后端返回顺序：用户看工具页的真实问题就是"我该怎么开始"。
const GUIDE_SECTIONS = [
  { kind: 'deploy', label: '部署教程', hint: '从零装起来', iconName: 'package' },
  { kind: 'implement', label: '实施教程', hint: '接到真实业务上', iconName: 'terminal' },
  { kind: 'optimize', label: '优化教程', hint: '性能与成本', iconName: 'flame' },
  { kind: 'cluster', label: '集群构建', hint: '高可用与扩容', iconName: 'layers' },
];

// guidesHTML 渲染「四类实战教程」区块。没有任何教程时整块不输出，
// 避免每个还没补教程的工具页都挂着一个空壳标题。
function guidesHTML(guides) {
  const list = Array.isArray(guides) ? guides : [];
  if (!list.length) return '';

  const sections = GUIDE_SECTIONS.map((sec) => {
    const items = list.filter((g) => g.kind === sec.kind);
    if (!items.length) return '';
    return `<div class="guide-col">
      <div class="guide-col-head">
        <span class="guide-col-title">${icon(sec.iconName, 14)}${esc(sec.label)}</span>
        <span class="guide-col-hint">${esc(sec.hint)}</span>
      </div>
      <div class="guide-list">${items.map(guideItemHTML).join('')}</div>
    </div>`;
  }).join('');

  if (!sections) return '';

  return `<section class="guide-panel">
    <div class="guide-panel-head">
      <h2>实战教程</h2>
      <p>按落地阶段拆开的四类教程，点进去是可执行的步骤，不是概念介绍。</p>
    </div>
    <div class="guide-grid">${sections}</div>
  </section>`;
}

// guideItemHTML 渲染一条教程。
//
// 站内文章走 hash 路由（同一份阅读体验、能点赞、有相关推荐），
// 站外链接才新窗口打开。两者在数据上是互斥的：后端 SetGuides 要求
// 「要么关联已发布文章，要么给外链」，不会同时存在。
function guideItemHTML(g) {
  const slug = String(g.articleSlug || '');
  const external = safeUrl(g.url);
  const href = slug ? `#/article/${encodeURIComponent(slug)}` : external;
  if (!href) return '';

  const attrs = slug
    ? ''
    : ' target="_blank" rel="noopener noreferrer"';
  const mark = slug ? '' : `<span class="guide-ext">${icon('link', 11)}站外</span>`;

  return `<a class="guide-item" href="${esc(href)}"${attrs}>
    <span class="guide-item-title">${esc(g.title || '未命名教程')}${mark}</span>
    ${g.summary ? `<span class="guide-item-sum">${esc(g.summary)}</span>` : ''}
  </a>`;
}

// booksHTML 渲染「系统教程集」区块。
//
// 与 guidesHTML 并存而不是取代：
//   Guides 回答「还有哪些值得看」——散装好文，彼此无序；
//   Books  回答「我该怎么系统学会它」——有先后顺序的学习路径。
// 合并成一个列表会同时丢掉这两个语义，用户分不出哪篇是起点。
//
// 一本书都不存在时整块不输出，避免每个还没整理教程集的工具页挂个空标题。
function booksHTML(books, toolSlug) {
  const list = Array.isArray(books) ? books : [];
  if (!list.length) return '';

  const cards = list.map((b) => {
    const href = `#/tools/${encodeURIComponent(toolSlug || '')}/book/${encodeURIComponent(String(b.id))}`;
    const level = BOOK_LEVEL_LABEL[b.level] || '入门';
    const cover = safeUrl(b.cover);
    return `<a class="book-card" href="${esc(href)}">
      <span class="book-cover">
        ${cover ? `<img src="${esc(cover)}" alt="" />` : icon('book', 22)}
      </span>
      <span class="book-main">
        <span class="book-title">${esc(b.title || '未命名教程集')}</span>
        <span class="book-sub">
          <span class="book-level is-${esc(b.level || 'beginner')}">${esc(level)}</span>
          <span>${fmtNum(b.chapterCount)} 个章节</span>
        </span>
        ${b.summary ? `<span class="book-summary">${esc(b.summary)}</span>` : ''}
      </span>
    </a>`;
  }).join('');

  return `<section class="book-block">
    <div class="guide-panel-head">
      <h2>系统教程集</h2>
      <p>按顺序读的完整路径，从装起来到撑住规模；每一节都是一篇可直接照着做的正文。</p>
    </div>
    <div class="book-grid">${cards}</div>
  </section>`;
}

/* ================================================== 教程集详情 ====== */

/**
 * 教程集详情页：一本「书」的目录。
 *
 * 这里刻意不展示正文，只做导航。正文是 articles，复用同一套阅读页
 * （评论、点赞、相关推荐都在那边）；把正文搬到这里等于再造一个落后的编辑器。
 *
 * 折叠用原生 <details>：目录是静态结构，用 JS 维护"哪些章展开"的状态
 * 只会在前进后退时和浏览器历史打架。
 */
export async function toolBookPage(ctx, toolSlug, bookId) {
  const book = await api.toolBook(bookId);
  const chapters = Array.isArray(book.chapters) ? book.chapters : [];

  if (!chapters.length) {
    return {
      title: book.title || '教程集',
      html: `<div class="page-head">
          <h1>${esc(book.title || '教程集')}</h1>
          ${book.summary ? `<p class="lede">${esc(book.summary)}</p>` : ''}
        </div>` +
        emptyHTML({
          iconName: 'book',
          title: '这本教程集还没有整理好目录',
          desc: '目录还在编写中，先看看这个工具的其他教程。',
          actionHTML: `<a class="btn btn-primary" href="#/tools/${encodeURIComponent(toolSlug || book.toolSlug || '')}">回到工具页</a>`,
        }),
    };
  }

  const crumb = `
    <nav class="crumb">
      <a href="#/">首页</a>
      <span class="crumb-sep">/</span>
      <a href="#/tools">开源工具</a>
      <span class="crumb-sep">/</span>
      <a href="#/tools/${encodeURIComponent(book.toolSlug || toolSlug || '')}">${esc(book.toolName || '工具')}</a>
      <span class="crumb-sep">/</span>
      <span>${esc(book.title)}</span>
    </nav>`;

  const level = BOOK_LEVEL_LABEL[book.level] || '入门';
  const total = Number(book.chapterCount) || chapters.length;

  const hero = `
    <header class="book-hero" style="margin:6px 0 20px">
      <div class="book-hero-main">
        <span class="book-level is-${esc(book.level || 'beginner')}">${esc(level)}</span>
        <h1>${esc(book.title)}</h1>
        ${book.summary ? `<p>${esc(book.summary)}</p>` : ''}
      </div>
      <div class="book-hero-side">
        <div><b>${fmtNum(total)}</b>章节</div>
        ${book.publishedAt ? `<div><b>${esc(fmtDate(book.publishedAt))}</b>发布</div>` : ''}
      </div>
    </header>`;

  const toc = `
    <section>
      <div class="page-tools">
        <h2 style="margin:0;font-size:16px">目录</h2>
        ${isAdmin() ? adminEditHTML(`#/admin/tools/${book.toolId}/books?book=${book.id}`, '管理目录') : ''}
        <button class="btn btn-sm" type="button" data-book-expand>全部展开</button>
      </div>
      <div class="book-toc" data-book-toc>
        ${chapters.map((ch, i) => chapterHTML(ch, i + 1)).join('')}
      </div>
    </section>`;

  const aside = `
    <aside>
      <div class="card side-card" style="margin-bottom:16px">
        <h3>怎么用这本教程集</h3>
        <ol class="steps">
          <li>按顺序读，别跳章——后面默认你已经会前面的内容</li>
          <li>每一节都动手敲一遍，只看不动手等于没学</li>
          <li>遇到卡住的地方，去问答广场提问并附上报错</li>
        </ol>
      </div>
      <div class="card side-card">
        <h3>所属工具</h3>
        <a href="#/tools/${encodeURIComponent(book.toolSlug || toolSlug || '')}"
           style="font-size:13.5px;font-weight:500">${esc(book.toolName || '查看工具详情')}</a>
        <p class="muted-note" style="margin-top:8px">这个工具的安装、配置与集群资料都在那一页。</p>
      </div>
    </aside>`;

  return {
    title: book.title || '教程集',
    html: `${crumb}${hero}<div class="detail-layout" style="margin-top:0">
      <div>${toc}</div>
      ${aside}
    </div>`,
    mount: (root) => {
      const btn = root.querySelector('[data-book-expand]');
      const details = [...root.querySelectorAll('[data-book-toc] .book-chapter')];
      if (!btn || !details.length) return;
      // 按钮文案跟着实际状态走：只要有任一章是收起的，它就是「全部展开」。
      const sync = () => {
        const allOpen = details.every((d) => d.open);
        btn.textContent = allOpen ? '全部收起' : '全部展开';
        btn.dataset.allOpen = allOpen ? '1' : '0';
      };
      btn.addEventListener('click', () => {
        const toOpen = btn.dataset.allOpen !== '1';
        details.forEach((d) => { d.open = toOpen; });
        sync();
      });
      details.forEach((d) => d.addEventListener('toggle', sync));
      sync();
    },
  };
}

// chapterHTML 渲染一个章：摘要行 + 它下面的节。
//
// 没有子节点且自身也没正文的章不该出现在这里——后端已经把它过滤掉了
// （前台 broken 节点直接丢弃），前端只负责渲染拿到的结构。
function chapterHTML(ch, index) {
  const subs = Array.isArray(ch.children) ? ch.children : [];
  const selfLink = chapterLink(ch);

  const head = selfLink
    ? `<a href="${esc(selfLink.href)}"${selfLink.ext} style="color:inherit;text-decoration:none">
         ${esc(ch.title)}
       </a>`
    : esc(ch.title);

  // 第一章默认展开：进页面就能看到第一节，比一片折叠的标题更有"从这开始"的指示。
  const open = index === 1 ? ' open' : '';
  return `<details class="book-chapter"${open}>
    <summary>
      <span class="book-chapter-no">${index}</span>
      <span class="book-chapter-title">${head}</span>
      ${subs.length ? `<span class="book-chapter-count">${subs.length} 节</span>` : ''}
      ${icon('chevron', 16)}
    </summary>
    ${subs.length ? `<ul class="book-sections">
      ${subs.map((s, i) => sectionHTML(s, `${index}.${i + 1}`)).join('')}
    </ul>` : ''}
  </details>`;
}

function sectionHTML(sec, no) {
  const link = chapterLink(sec);
  if (!link) {
    return `<li class="book-sec">
      <div class="book-sec-no">${esc(no)}</div>
      <div>${esc(sec.title)}</div>
    </li>`;
  }
  return `<li class="book-sec${sec.broken ? ' is-broken' : ''}">
    <a href="${esc(link.href)}"${link.ext}>
      <span class="book-sec-no">${esc(no)}</span>
      <span>${esc(sec.title)}</span>
    </a>
    ${sec.summary ? `<span class="book-sec-summary">${esc(sec.summary)}</span>` : ''}
  </li>`;
}

// chapterLink 把「站内文章」与「站外链接」归一成一个 href。
//
// 站内走 hash 路由（同一套阅读体验）；站外新窗口打开。
// 两者数据上互斥，后端 SetOutline 保证不会同时有值。
// 都没有则返回 null，调用方据此渲染成"不可点"的条目——
// 前端不做"猜它应该去哪里"，那是配置错误，得在后台修。
function chapterLink(node) {
  const slug = String(node.articleSlug || '');
  const external = safeUrl(node.url);
  if (slug) return { href: `#/article/${encodeURIComponent(slug)}`, ext: '' };
  if (external) return { href: external, ext: ' target="_blank" rel="noopener noreferrer"' };
  return null;
}

function relatedItemHTML(item) {
  return `<a href="${esc('#/tools/' + encodeURIComponent(item.slug || item.id))}"
    style="display:block;color:inherit">
    <div style="font-size:13.2px;font-weight:500">${esc(item.name)}</div>
    <div style="font-size:11.8px;color:var(--text-4);margin-top:3px;line-height:1.5">
      ${esc(item.summary || item.language || '')}
    </div>
  </a>`;
}

function kvRow(label, value, mono = false) {
  return `<div class="kv-row"><dt>${esc(label)}</dt><dd class="${mono ? 'mono' : ''}">${esc(value)}</dd></div>`;
}

function findChannel(ctx, slug) {
  const hit = (ctx.state.channels || []).find((c) => c.slug === slug);
  return hit || { slug, name: '开源工具推荐', desc: '' };
}
