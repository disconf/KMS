/* ============================================================================
   pages-home.js — 概览页。
   首页是入口，任何一块数据取不到都不该让整页变成错误页：能显示多少显示多少。
   ========================================================================= */

import { api } from './api.js';
import { esc, fmtNum, icon } from './util.js';
import { channelCardHTML, postCardHTML, resourceItemHTML, sectionHeadHTML, toolCardHTML } from './cards.js';
import { emptyHTML } from './ui.js';
import { FLAGS } from './flags.js';
import { overallStats } from './pages-paths.js';

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

    ${startHereHTML()}

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

/**
 * 「从这里开始」区块：成长路径大卡 + 四个高频工具入口。
 *
 * 放在栏目卡之后、内容流之前——首页最容易犯的错是把用户丢进一堆列表里，
 * 让他自己找入口。这里给的是「按顺序学」和「马上要用的四个工具」两条明确路径，
 * 也是本批功能唯一需要的突出位置：不占顶栏也能被看见。
 */
function startHereHTML() {
  // 路径收起时整块不出现；巡检单独收起时只摘掉那张卡，路径引导保留
  if (!FLAGS.paths) return '';
  const ov = overallStats();
  const quick = [
    { href: '#/cheatsheet', ic: 'terminal', name: '速查手册', desc: '128 条命令即点即抄', on: true },
    { href: '#/runbook',    ic: 'activity', name: '排障剧本', desc: '11 个场景的决策树', on: FLAGS.runbook },
    { href: '#/checklist',  ic: 'shield',   name: '值班巡检', desc: '日检 12 项打勾交接', on: FLAGS.checklist },
    { href: '#/exam',       ic: 'target',   name: '模拟考',   desc: '限时整卷 + 弱项分析', on: FLAGS.exam },
  ].filter((q) => q.on);

  return `<section class="section hp-start">
    <a class="hp-path" href="#/paths">
      <div class="hp-path-main">
        <p class="hp-path-eyebrow">${icon('layers', 13)} 不知道从哪开始</p>
        <h2>按路线走一遍：从能上手，到能独立值班</h2>
        <p class="hp-path-desc">六条路线把教程、题库、排障剧本和速查手册串成一条线，
          每个阶段都有可验证的过关标准，学完一节就能当天用上。</p>
        <span class="hp-path-tags">
          <i>Linux 基本功</i><i>Nginx 接入</i><i>数据库</i><i>Kubernetes</i><i>监控体系</i><i>SRE 复盘</i>
        </span>
      </div>
      <div class="hp-path-side">
        <div class="hp-path-ring" style="--ring-pct:${ov.percent}%">
          <div class="hp-path-ring-hole"><strong>${ov.percent}<span>%</span></strong></div>
        </div>
        <p class="hp-path-foot">${ov.done}/${ov.total} 阶段已走完</p>
        <span class="hp-path-go">${ov.started ? '继续' : '开始'}${icon('right', 13)}</span>
      </div>
    </a>

    <div class="hp-quick">
      ${quick.map((q) => `<a class="hp-quick-item" href="${esc(q.href)}">
        <span class="hp-quick-ic">${icon(q.ic, 18)}</span>
        <span class="hp-quick-main">
          <strong>${esc(q.name)}</strong>
          <small>${esc(q.desc)}</small>
        </span>
      </a>`).join('')}
    </div>
  </section>`;
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
