/* ============================================================================
   pages-search.js — 全站搜索。
   后端没有跨表检索接口，这里改为并行查四类内容再分组展示。
   四路请求各自独立，任何一路失败只让那一组显示为空，不影响其它组。

   检索词解析（/kb/search/suggest）单独走一路：它决定「我按什么词在找」，
   是搜索结果不准时用户唯一能看到的线索，所以它挂掉要能降级而不是拖垮整页。
   ========================================================================= */

import { api } from './api.js';
import { esc, fmtNum, icon, DIFFICULTY_LABEL, QUIZ_TYPE_LABEL } from './util.js';
import { postCardHTML, resourceItemHTML, toolCardHTML } from './cards.js';
import { emptyHTML } from './ui.js';

/** 一次最多展示多少个检索词。太多会把页面头部撑成标签墙。 */
const MAX_SHOWN_TERMS = 12;

const searchHref = (word) => `#/search?q=${encodeURIComponent(word)}`;

export async function searchPage(ctx) {
  const keyword = (ctx.params.get('q') || '').trim();

  if (!keyword) {
    return { html: await blankPage(), title: '搜索' };
  }

  const [articles, tools, resources, quizzes, suggest] = await Promise.allSettled([
    api.articles({ keyword, pageSize: 8 }),
    api.tools({ keyword, pageSize: 6 }),
    api.resources({ keyword, pageSize: 6 }),
    api.quizQuestions({ keyword, pageSize: 6 }),
    api.searchSuggest(keyword),
  ]);

  const articleList = take(articles, 8);
  const toolList = take(tools, 6);
  const resourceList = take(resources, 6);
  const quizList = take(quizzes, 6);
  const plan = fulfilled(suggest) || { terms: [], expanded: [], hot: [] };

  const totalHits = articleList.length + toolList.length + resourceList.length + quizList.length;

  const html = `
    <div class="page-head">
      <h1>“${esc(keyword)}”的搜索结果</h1>
      <p class="lede">共找到 ${fmtNum(totalHits)} 条相关内容。</p>
    </div>

    ${termsHTML(plan)}

    ${totalHits === 0
      ? zeroResultHTML(plan)
      : ''}

    ${articleList.length ? `<section class="section">
      <div class="section-head"><h2>教程与资讯（${articleList.length}）</h2></div>
      <div class="post-list">${articleList.map(postCardHTML).join('')}</div>
    </section>` : ''}

    ${toolList.length ? `<section class="section">
      <div class="section-head"><h2>开源工具（${toolList.length}）</h2></div>
      <div class="tool-grid">${toolList.map(toolCardHTML).join('')}</div>
    </section>` : ''}

    ${resourceList.length ? `<section class="section">
      <div class="section-head"><h2>资源下载（${resourceList.length}）</h2></div>
      <div class="res-list">${resourceList.map(resourceItemHTML).join('')}</div>
    </section>` : ''}

    ${quizList.length ? `<section class="section">
      <div class="section-head"><h2>相关题目（${quizList.length}）</h2></div>
      <div class="post-list">${quizList.map(quizItemHTML).join('')}</div>
    </section>` : ''}
  `;

  return { html, title: `搜索：${keyword}` };
}

/* ------------------------------------------------------------ 片段 -- */

/**
 * 展示「我按哪些词在找」。
 *
 * 只在检索词与原话不一致时才展示——如果用户输的就是一个词，且没有展开，
 * 这段话纯属噪音。展开过（或识别出多个概念）才有解释价值。
 */
function termsHTML(plan) {
  const expanded = (plan.expanded || []).slice(0, MAX_SHOWN_TERMS);
  if (!expanded.length) return '';

  const terms = (plan.terms || []).map((t) => t.toLowerCase());
  const hasExpansion = expanded.some((w) => !terms.includes(w.toLowerCase()));
  if (!hasExpansion && terms.length <= 1) return '';

  return `<div class="search-terms">
    <span class="search-terms-label">${icon('search', 13)} 按这些词检索</span>
    <div class="chips">
      ${expanded.map((w) => `<a class="chip chip-brand" href="${esc(searchHref(w))}">${esc(w)}</a>`).join('')}
    </div>
  </div>`;
}

/**
 * 零结果时的页面。
 *
 * 原来的做法是劝用户「换个说法，只保留核心关键词」——那等于把翻译工作
 * 推回给用户。现在把系统想到的相关词直接摆出来，点一下就是一次新搜索。
 */
function zeroResultHTML(plan) {
  const hot = (plan.hot || []).slice(0, 8);
  return `<div class="empty">
    <div class="empty-mark">${icon('search', 26)}</div>
    <strong>没有找到匹配的内容</strong>
    <p>试试下面这些词，或者换一个说法。</p>
    ${hot.length ? `<div class="chips chips-center">
      ${hot.map((w) => `<a class="chip" href="${esc(searchHref(w))}">${esc(w)}</a>`).join('')}
    </div>` : ''}
  </div>`;
}

/** 空输入时的页面：给热门词，别让人对着一个空框发呆。 */
async function blankPage() {
  let plan = { hot: [] };
  try {
    const data = await api.searchSuggest('');
    if (data && Array.isArray(data.hot)) plan = data;
  } catch {
    // 拿不到热门词就退回纯提示，不影响搜索本身。
  }

  return `<div class="page-head"><h1>搜索</h1>
      <p class="lede">在教程、工具、题目与资源里一起找。可以直接用一句话提问，例如「磁盘满了怎么办」。</p></div>
    ${plan.hot.length
      ? `<section class="section">
          <div class="section-head"><h2>热门关键词</h2></div>
          <div class="chips">
            ${plan.hot.map((w) => `<a class="chip" href="${esc(searchHref(w))}">${esc(w)}</a>`).join('')}
          </div>
        </section>`
      : emptyHTML({ iconName: 'search', title: '输入关键词开始搜索', desc: '支持标题、摘要、正文与标签的模糊匹配。' })}`;
}

/** 题目没有独立详情页，点进去落在题库首页——搜索的意义是"知道有这道题"。 */
function quizItemHTML(item) {
  return `<a class="post" href="#/quiz">
    <div class="post-body">
      <h3>${esc(item.stem)}</h3>
      <div class="post-foot">
        <span class="chip chip-brand">${esc(QUIZ_TYPE_LABEL[item.type] || item.type)}</span>
        <span class="chip">${esc(DIFFICULTY_LABEL[item.difficulty] || item.difficulty || '')}</span>
        ${item.categoryName ? `<span class="chip">${esc(item.categoryName)}</span>` : ''}
        <span class="meta-item">${icon('target', 13)}正确率 ${Math.round(item.correctRate || 0)}%</span>
      </div>
    </div>
  </a>`;
}

function fulfilled(settled) {
  if (!settled || settled.status !== 'fulfilled') return null;
  return settled.value;
}

function take(settled, size) {
  const value = fulfilled(settled);
  if (!value) return [];
  const list = value.list;
  return Array.isArray(list) ? list.slice(0, size) : [];
}
