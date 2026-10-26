/* ============================================================================
   cards.js — 各页面共用的展示单元。
   列表卡片在首页、栏目页、相关推荐里反复出现，抽到这里保证同一种内容
   在任何位置长得一样，也避免几处模板各改各的慢慢跑偏。
   ========================================================================= */

import { esc, icon, fmtNum, fmtCompact, fmtRelative, fmtSize, safeUrl, channelIcon, DEPLOY_LABEL } from './util.js';

/** 资源扩展名到中文类型的映射，给下载列表一个可读的副标题。 */
export const FILE_KIND_LABEL = {
  sh: 'Shell 脚本', conf: '配置文件', cfg: '配置文件', ini: '配置文件',
  yaml: 'YAML 配置', yml: 'YAML 配置', json: 'JSON 数据',
  sql: 'SQL 脚本', md: 'Markdown 文档', txt: '纯文本',
  zip: '压缩包', 'tar.gz': '压缩包', tgz: '压缩包', gz: '压缩包', '7z': '压缩包',
  pdf: 'PDF 文档', py: 'Python 脚本', go: 'Go 源码',
};

/** 页面内跳转链接。所有 slug 统一走一次编码，避免中文与斜杠把路由切坏。 */
export function hrefTo(...parts) {
  return '#/' + parts.map((p) => encodeURIComponent(String(p))).join('/');
}

/* ---------------------------------------------------------- 栏目卡片 -- */

export function channelCardHTML(ch) {
  return `<a class="channel-card" href="${esc('#/channel/' + encodeURIComponent(ch.slug))}">
    <div class="channel-card-top">
      <span class="channel-glyph">${channelIcon(ch.icon)}</span>
      <h3>${esc(ch.name)}</h3>
    </div>
    <p>${esc(ch.desc)}</p>
    <div class="channel-card-foot">
      <span>${fmtNum(ch.count)} 条内容</span>
      <span style="margin-left:auto;color:var(--brand)">进入 ${icon('right', 13)}</span>
    </div>
  </a>`;
}

/* ---------------------------------------------------------- 文章卡片 -- */

export function postCardHTML(item) {
  const slug = item.slug || item.id;
  const cover = safeUrl(item.cover);
  return `<a class="post" href="${esc('#/article/' + encodeURIComponent(slug))}">
    <div class="post-body">
      <h3>${esc(item.title)}</h3>
      ${item.summary ? `<p class="post-summary">${esc(item.summary)}</p>` : ''}
      <div class="post-foot">
        ${item.categoryName ? `<span class="chip">${esc(item.categoryName)}</span>` : ''}
        ${sourceBadgeHTML(item)}
        <span class="meta-item">${icon('clock', 13)}${esc(fmtRelative(item.publishedAt))}</span>
        <span class="meta-item">${icon('eye', 13)}${fmtNum(item.viewCount)}</span>
        <span class="meta-item">${icon('heart', 13)}${fmtNum(item.likeCount)}</span>
        ${item.authorName ? `<span class="meta-item">${icon('user', 13)}${esc(item.authorName)}</span>` : ''}
      </div>
    </div>
    ${cover ? `<img class="post-cover" src="${esc(cover)}" alt="" loading="lazy" />` : ''}
  </a>`;
}

/* ---------------------------------------------------------- 来源徽标 -- */

// sourceBadgeHTML 渲染「这条内容来自哪个签约源」。
//
// 资讯栏目里混着两种东西：平台自己写的、从外部订阅源抓来的。
// 不标出来的话，用户会把第三方观点当成平台的推荐，出问题时也找不到出处。
// 只有抓取来源（sourceName 有值）才显示；人工撰写的不加任何徽标，
// 让「有徽标 = 转述，没徽标 = 原创」成为一个可依赖的信号。
//
// 这里刻意用 span 而不是 a：卡片整体已经是一个 <a>，
// 嵌套锚点在 HTML 里是非法的，浏览器会把外层 <a> 提前闭合，整块布局散掉。
// 跳原文的入口放在详情页（那里不在链接里）。
export function sourceBadgeHTML(item) {
  const name = String(item.sourceName || '').trim();
  if (!name) return '';
  return `<span class="chip chip-source" title="内容转述自 ${esc(name)}">${icon('link', 11)}${esc(name)}</span>`;
}

/* ---------------------------------------------------------- 工具卡片 -- */

export function toolCardHTML(item) {
  const slug = item.slug || item.id;
  const logo = safeUrl(item.logo);
  const repoLabel = shortRepo(item.repoUrl);
  const modes = (item.deployModes || []).slice(0, 3);

  return `<a class="tool-card" href="${esc('#/tools/' + encodeURIComponent(slug))}">
    <div class="tool-head">
      <span class="tool-logo">${logo
        ? `<img src="${esc(logo)}" alt="" loading="lazy" />`
        : esc(initialOf(item.name))}</span>
      <div class="tool-title">
        <h3>${esc(item.name)}${item.featured ? `<span class="chip chip-brand" style="padding:0 6px;font-size:10.5px">精选</span>` : ''}</h3>
        ${repoLabel ? `<span class="repo">${esc(repoLabel)}</span>` : ''}
      </div>
    </div>
    <p class="tool-summary">${esc(item.summary || '暂无简介')}</p>
    <div class="chips">
      ${item.language ? `<span class="chip">${esc(item.language)}</span>` : ''}
      ${item.license ? `<span class="chip chip-mono">${esc(item.license)}</span>` : ''}
      ${modes.map((m) => `<span class="chip chip-brand">${esc(DEPLOY_LABEL[m] || m)}</span>`).join('')}
    </div>
    <div class="tool-foot">
      <span class="star-count">${icon('star', 13, 'fill-star')}${fmtCompact(item.stars)}</span>
      <span class="meta-item">${icon('repo', 13)}${fmtCompact(item.forkCount)}</span>
      <span class="meta-item" style="margin-left:auto">${icon('eye', 13)}${fmtNum(item.viewCount)}</span>
    </div>
  </a>`;
}

/* ---------------------------------------------------------- 资源条目 -- */

export function resourceItemHTML(item) {
  const slug = item.slug || item.id;
  const ext = String(item.fileExt || '').toLowerCase();
  const kind = FILE_KIND_LABEL[ext] || '文件';

  return `<a class="res-item" href="${esc('#/resources/' + encodeURIComponent(slug))}">
    <span class="res-glyph ext-${esc(ext)}">${esc(ext ? ext.toUpperCase().slice(0, 5) : 'FILE')}</span>
    <div class="res-body">
      <h3>${esc(item.title)}</h3>
      <div class="res-meta">
        <span>${esc(kind)}</span>
        <span class="meta-num">${esc(fmtSize(item.fileSize))}</span>
        ${item.version ? `<span class="chip chip-mono">${esc(item.version)}</span>` : ''}
        ${item.compatibleWith ? `<span>${esc(item.compatibleWith)}</span>` : ''}
        <span class="meta-item">${icon('download', 12)}${fmtNum(item.downloadCount)}</span>
        ${item.needLogin ? `<span class="chip chip-warn">需登录</span>` : ''}
      </div>
    </div>
    <span class="res-action">${icon('right', 16)}</span>
  </a>`;
}

/* ------------------------------------------------------------ 小件 -- */

export function sectionHeadHTML(title, moreHref, moreText = '查看全部') {
  return `<div class="section-head">
    <h2>${esc(title)}</h2>
    ${moreHref ? `<a class="more" href="${esc(moreHref)}">${esc(moreText)} ${icon('right', 13)}</a>` : ''}
  </div>`;
}

function shortRepo(url) {
  const s = String(url || '').trim();
  if (!s) return '';
  const m = /^https?:\/\/(?:www\.)?github\.com\/([^/?#]+)\/([^/?#]+)/i.exec(s);
  if (m) return `${m[1]}/${m[2].replace(/\.git$/, '')}`;
  return s.replace(/^https?:\/\//i, '').replace(/\/$/, '');
}

function initialOf(name) {
  const s = String(name || '').trim();
  return s ? s[0].toUpperCase() : '?';
}
