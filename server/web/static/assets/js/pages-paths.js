/* ============================================================================
   pages-paths.js — 运维成长路径：总览 + 单条路线。

   进度只存在浏览器本地（localStorage），不上报、不需要登录。
   理由：这是一份"自学清单"，不是考核成绩。一旦需要登录才能记进度，
   新访客第一步就会被拦住；而如果存到服务端，就得回答"进度被谁看见"这个问题，
   现阶段没有这个必要。换设备会丢进度，页面里明确写出来，不装作是云同步。

   数据形状：{ "<pathId>": { done: ["s1","s2"], at: "<ISO>" } }
   用数组而不是对象，因为顺序没有意义、去重靠 includes，读起来更直接。
   ========================================================================= */

import { esc, icon, fmtNum } from './util.js';
import { PATHS, PATH_LEVEL, RESOURCE_KIND, findPath, stageCountOf } from './paths-data.js';

const STORE_KEY = 'opskb.paths.v1';

/* --------------------------------------------------------- 进度存取 -- */

function readStore() {
  try {
    const raw = localStorage.getItem(STORE_KEY);
    const obj = raw ? JSON.parse(raw) : null;
    return obj && typeof obj === 'object' ? obj : {};
  } catch {
    return {};
  }
}

function writeStore(store) {
  try {
    localStorage.setItem(STORE_KEY, JSON.stringify(store));
  } catch {
    // 隐私模式下写入会抛错。进度存不上不影响使用，页面自己在原地更新即可。
  }
}

/** 某条路线已完成的阶段 id 列表。 */
export function doneStagesOf(pathId) {
  const entry = readStore()[pathId];
  return entry && Array.isArray(entry.done) ? entry.done : [];
}

/** 某条路线的进度统计：完成数、总数、百分比、当前该做的阶段（第一个未完成的）。 */
export function pathStats(path) {
  const total = stageCountOf(path);
  const doneIds = doneStagesOf(path.id).filter((id) => path.stages.some((s) => s.id === id));
  const current = path.stages.find((s) => !doneIds.includes(s.id)) || null;
  return {
    done: doneIds.length,
    total,
    percent: total ? Math.round((doneIds.length / total) * 100) : 0,
    current,
    doneIds,
  };
}

/** 全部路线的汇总，首页与总览页共用，避免两处各算一遍口径不一致。 */
export function overallStats() {
  let done = 0;
  let total = 0;
  let started = 0;
  PATHS.forEach((p) => {
    const s = pathStats(p);
    done += s.done;
    total += s.total;
    if (s.done > 0) started += 1;
  });
  return { done, total, started, percent: total ? Math.round((done / total) * 100) : 0 };
}

/* ------------------------------------------------------------- 零件 -- */

/** 环形进度：conic-gradient 画弧，中间盖一层白圆。比 SVG 少一堆节点。 */
function ringHTML(percent, size = 96, label = '') {
  const p = Math.max(0, Math.min(100, Number(percent) || 0));
  return `<div class="pt-ring" style="--ring-size:${size}px;--ring-pct:${p}%">
    <div class="pt-ring-hole">
      <strong>${p}<span>%</span></strong>
      ${label ? `<small>${esc(label)}</small>` : ''}
    </div>
  </div>`;
}

function levelTagHTML(level) {
  const meta = PATH_LEVEL[level] || PATH_LEVEL.beginner;
  return `<span class="pt-lvl is-${meta.tone}">${esc(meta.label)}</span>`;
}

function pathCardHTML(path) {
  const st = pathStats(path);
  const started = st.done > 0;
  return `<a class="pt-card${started ? ' is-started' : ''}" href="#/paths/${encodeURIComponent(path.id)}">
    <div class="pt-card-top">
      <span class="pt-ic">${icon(path.icon, 20)}</span>
      ${levelTagHTML(path.level)}
      <span class="pt-hours">${icon('clock', 12)}约 ${fmtNum(path.hours)} 小时</span>
    </div>
    <h2 class="pt-card-name">${esc(path.name)}</h2>
    <p class="pt-card-tag">${esc(path.tagline)}</p>
    <div class="pt-card-bar"><i style="width:${st.percent}%"></i></div>
    <div class="pt-card-foot">
      <span class="pt-card-count">${st.done}/${st.total} 阶段${started ? '' : ' · 未开始'}</span>
      <span class="pt-card-go">${started ? (st.current ? '继续' : '已完成') : '开始'}${icon('right', 13)}</span>
    </div>
  </a>`;
}

/* ----------------------------------------------------------- 总览页 -- */

export async function pathsPage() {
  const ov = overallStats();
  const next = PATHS.find((p) => pathStats(p).done > 0) || PATHS[0];

  const html = `
    <div class="page-head">
      <h1>运维成长路径</h1>
      <p class="lede">六条路线，从「能独立处理一台机器」到「能对稳定性负责」。
        每条路线都指向站内真实的教程、题库与排障剧本，学完一节就能当天用上。</p>
    </div>

    <div class="pt-overview">
      <div class="pt-ov-left">
        ${ringHTML(ov.percent, 104, '总进度')}
        <div class="pt-ov-main">
          <p class="pt-ov-title">已完成 ${fmtNum(ov.done)} / ${fmtNum(ov.total)} 个阶段</p>
          <p class="pt-ov-sub">${ov.started
            ? `已开始 ${ov.started} 条路线，选一条继续往下走。`
            : '还没开始。建议从「Linux 运维基本功」起步，其它路线随时可以插进来。'}</p>
          <div class="pt-ov-tips">
            ${icon('info', 13)} 进度只记在这台浏览器上，换设备不会同步
          </div>
        </div>
      </div>
      <a class="btn btn-primary pt-ov-btn" href="#/paths/${encodeURIComponent(next.id)}">
        ${ov.started ? '继续上次的路线' : '从第一条开始'}${icon('right', 14)}
      </a>
    </div>

    <div class="pt-grid">
      ${PATHS.map(pathCardHTML).join('')}
    </div>

    <section class="pt-note">
      <h2>${icon('target', 16)} 这份路线怎么用</h2>
      <ol>
        <li><strong>别按顺序硬啃。</strong>哪些已经会了，直接点「标记完成」跳过，把时间留给不会的。</li>
        <li><strong>每阶段都有可验证的动作。</strong>看完不算完，能在机器上跑通、能在模拟考里答对才算。</li>
        <li><strong>出事时先来这条路线上找。</strong>排障剧本与速查手册就挂在对应阶段里，不用重新搜。</li>
      </ol>
    </section>
  `;

  return { html, title: '运维成长路径' };
}

/* ----------------------------------------------------------- 单条路线 -- */

export async function pathDetailPage(ctx, id) {
  const path = findPath(id);
  if (!path) {
    return {
      title: '路线不存在',
      html: `<div class="page-head"><h1>路线不存在</h1>
        <p class="lede">这条路线可能已经调整过。</p></div>
        <a class="btn" href="#/paths">${icon('left', 14)} 回到全部路线</a>`,
    };
  }

  const st = pathStats(path);
  const html = `
    <nav class="crumb">
      <a href="#/">首页</a><span class="crumb-sep">/</span>
      <a href="#/paths">成长路径</a><span class="crumb-sep">/</span>
      <span>${esc(path.name)}</span>
    </nav>

    <section class="pt-hero">
      <div class="pt-hero-main">
        <div class="pt-hero-top">
          <span class="pt-ic is-lg">${icon(path.icon, 22)}</span>
          ${levelTagHTML(path.level)}
          <span class="pt-hours">${icon('clock', 12)}约 ${fmtNum(path.hours)} 小时 · ${st.total} 个阶段</span>
        </div>
        <h1>${esc(path.name)}</h1>
        <p class="pt-hero-tag">${esc(path.tagline)}</p>
        <p class="pt-hero-summary">${esc(path.summary)}</p>
        <div class="pt-outcome">
          <span class="pt-outcome-label">${icon('check', 13)} 学完之后</span>
          <p>${esc(path.outcome)}</p>
        </div>
      </div>
      <div class="pt-hero-side" data-pt-side>${sideHTML(path, st)}</div>
    </section>

    <ol class="pt-timeline" data-pt-timeline>${stagesHTML(path, st)}</ol>

    ${nextPathHTML(path)}
  `;

  const mount = (root) => {
    // 「继续第 n 阶段」用委托接：勾选进度后右侧会整体重绘，
    // 新生成的跳转按钮也要能点，不能只在首次挂载时绑一次。
    root.addEventListener('click', (e) => {
      const jump = e.target.closest('[data-pt-jump]');
      if (!jump) return;
      e.preventDefault(); // 默认锚点会改 hash 触发重路由，把用户甩回页面顶部
      const target = root.querySelector(jump.getAttribute('href'));
      if (target) target.scrollIntoView({ behavior: 'smooth', block: 'center' });
    });

    const timeline = root.querySelector('[data-pt-timeline]');
    if (!timeline) return;

    // 用一个委托接住所有阶段的「标记完成 / 撤销」。
    // 每次改动只重绘时间轴与右侧进度，不整页重路由——否则滚动位置会跳回顶部，
    // 而用户正在第 4 阶段往下看。
    timeline.addEventListener('click', (e) => {
      const btn = e.target.closest('[data-pt-done]');
      if (!btn) return;
      const stageId = btn.getAttribute('data-pt-done');
      const store = readStore();
      const entry = store[path.id] && Array.isArray(store[path.id].done) ? store[path.id].done : [];
      const nextDone = entry.includes(stageId)
        ? entry.filter((x) => x !== stageId)
        : entry.concat(stageId);
      store[path.id] = { done: nextDone, at: new Date().toISOString() };
      writeStore(store);

      const fresh = pathStats(path);
      timeline.innerHTML = stagesHTML(path, fresh);
      const side = root.querySelector('[data-pt-side]');
      if (side) side.innerHTML = sideHTML(path, fresh);
    });
  };

  return { html, mount, title: path.name };
}

function sideHTML(path, st) {
  return `
    ${ringHTML(st.percent, 112)}
    <p class="pt-hero-count" data-pt-count>${st.done} / ${st.total} 阶段已完成</p>
    ${st.current
      ? `<a class="btn btn-primary pt-hero-btn" href="#pt-stage-${esc(st.current.id)}" data-pt-jump>
          ${st.done ? '继续' : '开始'}第 ${st.done + 1} 阶段${icon('right', 13)}</a>
         <p class="pt-hero-next">下一阶段：${esc(st.current.name)}</p>`
      : `<span class="pt-hero-done">${icon('check', 15)} 这条路线已经走完</span>`}`;
}

function stagesHTML(path, st) {
  const currentId = st.current ? st.current.id : '';
  return path.stages.map((stage, i) => {
    const done = st.doneIds.includes(stage.id);
    const current = stage.id === currentId;
    return `<li class="pt-stage${done ? ' is-done' : ''}${current ? ' is-current' : ''}" id="pt-stage-${esc(stage.id)}">
      <div class="pt-stage-rail">
        <span class="pt-dot">${done ? icon('check', 14) : fmtNum(i + 1)}</span>
      </div>
      <div class="pt-stage-body">
        <div class="pt-stage-head">
          <h2>${esc(stage.name)}</h2>
          ${current ? '<span class="pt-badge-cur">当前阶段</span>' : ''}
          ${done ? '<span class="pt-badge-done">已完成</span>' : ''}
          <span class="pt-stage-n">第 ${fmtNum(i + 1)} / ${fmtNum(st.total)} 阶段</span>
        </div>
        <p class="pt-goal"><span>过关标准</span>${esc(stage.goal)}</p>
        <div class="pt-res-list">
          ${stage.resources.map(resHTML).join('')}
        </div>
        <div class="pt-stage-actions">
          <button class="btn btn-sm${done ? '' : ' btn-primary'}" type="button" data-pt-done="${esc(stage.id)}">
            ${done ? `${icon('refresh', 13)} 撤销完成` : `${icon('check', 13)} 标记完成`}
          </button>
          <span class="pt-stage-hint">${done ? '已记录在本机浏览器' : '做完了就点一下，进度会存在本机'}</span>
        </div>
      </div>
    </li>`;
  }).join('');
}

function resHTML(res) {
  const kind = RESOURCE_KIND[res.kind] || RESOURCE_KIND.read;
  return `<a class="pt-res" href="${esc(res.href)}">
    <span class="pt-res-kind is-${kind.tone}">${icon(kind.icon, 13)}${esc(kind.label)}</span>
    <span class="pt-res-main">
      <span class="pt-res-label">${esc(res.label)}</span>
      <span class="pt-res-note">${esc(res.note)}</span>
    </span>
    <span class="pt-res-arrow">${icon('right', 13)}</span>
  </a>`;
}

/** 路线的下一步推荐：走完这条，下一条建议去哪。最后一条就不给了，别硬凑。 */
function nextPathHTML(path) {
  const i = PATHS.findIndex((p) => p.id === path.id);
  const next = PATHS[i + 1] || null;
  if (!next) return '';
  return `<section class="pt-next">
    <div class="pt-next-main">
      <p class="pt-next-eyebrow">走完这条之后</p>
      <h2>${esc(next.name)}</h2>
      <p>${esc(next.tagline)}</p>
    </div>
    <a class="btn" href="#/paths/${encodeURIComponent(next.id)}">去看看${icon('right', 14)}</a>
  </section>`;
}
