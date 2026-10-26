/* ============================================================================
   pages-home.js — 概览页。
   首页是入口，任何一块数据取不到都不该让整页变成错误页：能显示多少显示多少。
   ========================================================================= */

import { api } from './api.js';
import { esc, fmtNum } from './util.js';
import { channelCardHTML, postCardHTML, resourceItemHTML, sectionHeadHTML, toolCardHTML } from './cards.js';
import { emptyHTML } from './ui.js';

export async function homePage(ctx) {
  const channels = ctx.state.channels || [];

  // 四个请求互不依赖，并行发出；用 allSettled 而不是 all，
  // 这样"精选工具挂了"不会连带把教程列表也吞掉。
  const [articles, tools, resources] = await Promise.allSettled([
    api.articles({ channel: 'linux-tutorial', pageSize: 5, sort: 'latest' }),
    api.tools({ featuredOnly: true, pageSize: 4, sort: 'stars' }),
    api.resources({ pageSize: 4, sort: 'latest' }),
  ]);

  const articleList = pick(articles, 'list', []);
  const toolList = pick(tools, 'list', []);
  const resourceList = pick(resources, 'list', []);

  const countOf = (slug) => {
    const hit = channels.find((c) => c.slug === slug);
    return hit ? hit.count : 0;
  };

  const html = `
    <section class="hero">
      <div class="hero-inner">
        <p class="hero-eyebrow">linux · kubernetes · database</p>
        <h1>排障时踩过的坑，<br>都该变成下次能查到的答案</h1>
        <p>把现场经验沉淀成可检索的教程、可自测的题库、可复用的工具与资源。
           全部内容开放在线阅读，登录后还能记录你的答题与错题。</p>
        <div class="hero-cta">
          <a class="btn hero-btn" href="#/channel/linux-tutorial">开始读教程</a>
          <a class="btn hero-btn hero-btn-ghost" href="#/quiz">做题自测</a>
        </div>
        <div class="hero-stats">
          ${statHTML(totalArticles(channels), '篇教程与资讯')}
          ${statHTML(countOf('ops-quiz'), '道运维题目')}
          ${statHTML(countOf('open-source-tools'), '个开源工具')}
          ${statHTML(countOf('resources'), '份可下载资源')}
        </div>
      </div>
    </section>

    <section class="section">
      <div class="channel-grid">
        ${channels.map(channelCardHTML).join('')}
      </div>
    </section>

    <section class="section">
      ${sectionHeadHTML('最新教程', '#/channel/linux-tutorial')}
      <div class="post-list">
        ${articleList.length
          ? articleList.map(postCardHTML).join('')
          : emptyHTML({
              iconName: 'book',
              title: '教程还在整理中',
              desc: '第一篇实战记录正在路上，稍后再来看看。',
            })}
      </div>
    </section>

    <section class="section">
      ${sectionHeadHTML('精选工具', '#/tools')}
      <div class="tool-grid">
        ${toolList.length
          ? toolList.map(toolCardHTML).join('')
          : emptyHTML({ iconName: 'tools', title: '暂无精选工具' })}
      </div>
    </section>

    <section class="section">
      ${sectionHeadHTML('最新资源', '#/resources')}
      <div class="res-list">
        ${resourceList.length
          ? resourceList.map(resourceItemHTML).join('')
          : emptyHTML({ iconName: 'download', title: '暂无资源', desc: '脚本与配置模板正在补充中。' })}
      </div>
    </section>
  `;

  return { html };
}

function statHTML(value, label) {
  return `<div class="hero-stat">
    <strong class="num">${fmtNum(value)}</strong>
    <span>${esc(label)}</span>
  </div>`;
}

/** 两个文章类栏目的条数之和，用于首页顶部统计。 */
function totalArticles(channels) {
  const articleSlugs = ['linux-tutorial', 'ops-news'];
  return channels
    .filter((c) => articleSlugs.includes(c.slug))
    .reduce((sum, c) => sum + (c.count || 0), 0);
}

function pick(settled, field, fallback) {
  if (settled.status !== 'fulfilled' || !settled.value) return fallback;
  const value = settled.value[field];
  return Array.isArray(value) ? value : fallback;
}
