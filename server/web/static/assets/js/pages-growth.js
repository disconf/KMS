/* ============================================================================
   pages-growth.js — 成长中心：等级徽章、贡献排行榜、积分流水。

   这里回答的是"我为什么要在意积分"：积分不兑换任何权益（后端也明确
   只做展示），它唯一的作用是让贡献被看见——回答旁边那枚徽章、榜单上的
   那个名次。所以页面刻意不写"兑换""权益"这类承诺，写了就是骗人。
   ========================================================================= */

import { api, isLoggedIn } from './api.js';
import { BADGE_LEVELS } from './flags.js';
import { avatarHTML, bindError, emptyHTML, errorHTML } from './ui.js';
import { esc, fmtDate, fmtNum, icon } from './util.js';

/**
 * 等级的展示元数据。**阈值不在这里定义**——它来自 flags.js 的
 * BADGE_LEVELS（那份又必须与后端 service.badgeLevels 逐字一致）。
 * 三处各写一份数字，早晚出现"页面说还差 10 分，徽章已经升了"。
 */
const LEVEL_META = {
  新手: { icon: 'user', note: '注册即达，回答旁边的徽章从这里开始' },
  活跃: { icon: 'star', note: '一个回答被采纳就有 10 分' },
  贡献者: { icon: 'thumb', note: '持续答题、写教程的人' },
  资深贡献者: { icon: 'shield', note: '社区里叫得出名字的那批人' },
};

const LEVELS = BADGE_LEVELS.map((l) => ({
  name: l.label,
  at: l.at,
  icon: (LEVEL_META[l.label] || {}).icon || 'star',
  note: (LEVEL_META[l.label] || {}).note || '',
}));

/** 积分来源。分值取自后端常量，改后端必须同步这里。 */
const RULES = [
  { action: '回答被采纳', points: 10, icon: 'check', note: '提问者点下采纳的那一刻，分记在回答者头上' },
  { action: '回答被点赞', points: 1, icon: 'thumb', note: '自己点自己不加分，同一人的赞只记一次' },
  { action: '实战教程过审', points: 20, icon: 'book', note: '投稿通过审核并发布' },
  { action: '题目过审入库', points: 5, icon: 'quiz', note: '贡献的题目进入题库' },
];

const ROLE_LABEL = { admin: '管理员', expert: '专家' };

export async function growthPage() {
  const logged = isLoggedIn();

  // 两件事都要，但失败面互不牵连：积分挂了不该让榜单也不显示，
  // 榜单挂了也别把整页判死——所以分别 catch。
  const [mine, board] = await Promise.all([
    logged ? api.myPoints().catch(() => null) : Promise.resolve(null),
    api.leaderboard(20).catch((err) => ({ error: err })),
  ]);

  const html = `
    <div class="page-head">
      <h1>成长中心</h1>
      <p class="lede">
        积分来自真实的社区行为：回答被采纳、被点赞、教程与题目过审。
        它不兑换权益，只做一件事——让贡献被看见。
      </p>
    </div>

    ${heroHTML(mine, board, logged)}
    ${levelsHTML(mine ? mine.points : null)}
    ${rulesHTML()}
    ${boardHTML(board)}
    ${mine && mine.recent && mine.recent.length ? flowHTML(mine.recent) : ''}
  `;

  return {
    title: '成长中心',
    html,
    // 榜单偶尔会因为网络失败留下一块错误态；给它接上重试，
    // 而不是让用户只能靠刷新整页。
    mount: (root) => bindError(root, { onRetry: () => window.__reloadPage() }),
  };
}

/* ------------------------------------------------------------ 顶部卡 -- */

function heroHTML(mine, board, logged) {
  if (!logged) {
    return emptyHTML({
      iconName: 'flame',
      title: '登录后查看你的等级',
      desc: '回答被采纳 +10、被点赞 +1、教程过审 +20、题目过审 +5。积分只增不减，决定你在回答旁的徽章。',
      actionHTML: `
        <button class="btn btn-primary" type="button" data-act="login">登录</button>
        <button class="btn" type="button" data-act="register">注册账号</button>`,
    });
  }
  if (!mine) {
    return `<div class="gs-hero is-plain">
      <p class="gs-hero-fail">${icon('alert', 14)}积分信息加载失败，刷新页面重试。</p>
    </div>`;
  }

  const level = levelOf(mine.points);
  const next = LEVELS.find((l) => l.at > mine.points);
  const pct = next ? progressOf(mine.points, level.at, next.at) : 100;

  const rankText = board && !board.error && board.myRank > 0
    ? `第 ${fmtNum(board.myRank)} 名`
    : '未上榜';
  const rankSub = board && !board.error && board.total > 0
    ? (board.myRank > 0 ? `共 ${fmtNum(board.total)} 人上榜` : `共 ${fmtNum(board.total)} 人上榜，还差 1 分就能进`)
    : '';

  return `<div class="gs-hero">
    <div class="gs-hero-main">
      <div class="gs-badge-ic">${icon(level.icon, 26)}</div>
      <div class="gs-hero-txt">
        <div class="gs-badge-name">${esc(mine.badge || level.name)}</div>
        <div class="gs-badge-sub">
          <strong>${fmtNum(mine.points)}</strong> 分
          ${next ? `· 距离「${esc(next.name)}」还差 ${fmtNum(next.at - mine.points)} 分` : '· 已是最高等级'}
        </div>
      </div>
    </div>

    <div class="gs-progress">
      <div class="gs-progress-track" role="progressbar" aria-valuemin="0" aria-valuemax="100"
           aria-valuenow="${pct}" aria-label="等级进度">
        <i style="width:${pct}%"></i>
      </div>
      <div class="gs-progress-scale">
        ${LEVELS.map((l) => `<span class="${mine.points >= l.at ? 'is-reached' : ''}">${esc(l.name)} ${l.at}</span>`).join('')}
      </div>
    </div>

    <div class="gs-rank">
      <div class="gs-rank-num">${esc(rankText)}</div>
      <div class="gs-rank-sub">${esc(rankSub)}</div>
    </div>
  </div>`;
}

/* -------------------------------------------------------------- 阶梯 -- */

function levelsHTML(points) {
  const reached = points === null ? -1 : LEVELS.filter((l) => points >= l.at).length;
  return `<section class="gs-section">
    <div class="gs-sec-head">
      <h2>四个等级</h2>
      ${points !== null ? `<span class="gs-sec-note">已解锁 ${reached} / ${LEVELS.length} 级</span>` : ''}
    </div>
    <div class="gs-levels">
      ${LEVELS.map((l, i) => {
        const on = points !== null && points >= l.at;
        return `<div class="gs-level ${on ? 'is-reached' : ''}">
          <div class="gs-level-top">
            <span class="gs-level-ic">${icon(l.icon, 18)}</span>
            <span class="gs-level-name">${esc(l.name)}</span>
            <span class="gs-level-at">${l.at} 分</span>
          </div>
          <p class="gs-level-note">${esc(l.note)}</p>
          ${i < LEVELS.length - 1 ? '<span class="gs-level-link" aria-hidden="true"></span>' : ''}
        </div>`;
      }).join('')}
    </div>
  </section>`;
}

/* -------------------------------------------------------------- 规则 -- */

function rulesHTML() {
  return `<section class="gs-section">
    <div class="gs-sec-head"><h2>积分怎么来</h2></div>
    <div class="gs-rules">
      ${RULES.map((r) => `<div class="gs-rule">
        <span class="gs-rule-ic">${icon(r.icon, 16)}</span>
        <div class="gs-rule-body">
          <div class="gs-rule-head">
            <strong>${esc(r.action)}</strong>
            <span class="gs-rule-pt">+${r.points}</span>
          </div>
          <p>${esc(r.note)}</p>
        </div>
      </div>`).join('')}
    </div>
    <p class="gs-foot-note">${icon('info', 12)}
      积分只增不减：取消点赞、撤回内容都不会扣分。封禁账号保留积分但不上榜，避免违规账号占着前排。
    </p>
  </section>`;
}

/* ------------------------------------------------------------ 排行榜 -- */

function boardHTML(board) {
  if (!board || board.error) {
    return `<section class="gs-section">
      <div class="gs-sec-head"><h2>贡献排行榜</h2></div>
      ${errorHTML(board ? board.error : new Error('加载失败'))}
    </section>`;
  }
  const items = board.items || [];
  if (!items.length) {
    return `<section class="gs-section">
      <div class="gs-sec-head"><h2>贡献排行榜</h2></div>
      ${emptyHTML({
        iconName: 'flame',
        title: '榜单还是空的',
        desc: '第一个被采纳的回答就能把名字写上去。',
        actionHTML: '<a class="btn btn-primary" href="#/qa">去问答广场看看</a>',
      })}
    </section>`;
  }

  return `<section class="gs-section">
    <div class="gs-sec-head">
      <h2>贡献排行榜</h2>
      <span class="gs-sec-note">共 ${fmtNum(board.total)} 人上榜 · 同分按注册先后</span>
    </div>
    <div class="gs-board">
      ${items.map((it) => boardRowHTML(it, board.myRank)).join('')}
    </div>
    ${board.myRank === 0 && board.myPoints === 0
      ? `<p class="gs-foot-note">${icon('info', 12)}你还没有积分。完成一次被采纳的回答，就有 10 分。</p>`
      : ''}
  </section>`;
}

function boardRowHTML(it, myRank) {
  // 只有登录用户且名次匹配才高亮——游客的 myRank 恒为 0，
  // 不能因此把所有 rank=0 的行都点亮。
  const me = myRank > 0 && it.rank === myRank;
  const top3 = it.rank <= 3;
  return `<div class="gs-row ${me ? 'is-me' : ''}">
    <span class="gs-rank-no ${top3 ? 'is-top' : ''}">${it.rank}</span>
    ${avatarHTML(it.nickname || '匿名', it.avatar, 32)}
    <div class="gs-row-main">
      <div class="gs-row-name">
        ${esc(it.nickname || '匿名用户')}
        ${ROLE_LABEL[it.role] ? `<span class="gs-role">${esc(ROLE_LABEL[it.role])}</span>` : ''}
        ${me ? '<span class="gs-role is-me">我</span>' : ''}
      </div>
      <div class="gs-row-badge">${esc(it.badge)}</div>
    </div>
    <span class="gs-row-pt">${fmtNum(it.points)}</span>
  </div>`;
}

/* -------------------------------------------------------------- 流水 -- */

function flowHTML(recent) {
  return `<section class="gs-section">
    <div class="gs-sec-head">
      <h2>我的积分流水</h2>
      <span class="gs-sec-note">最近 ${recent.length} 条</span>
    </div>
    <ol class="gs-flow">
      ${recent.map((l) => `<li class="gs-flow-item">
        <span class="gs-flow-dot" aria-hidden="true"></span>
        <div class="gs-flow-main">
          <span class="gs-flow-action">${esc(l.actionText)}</span>
          <span class="gs-flow-time">${esc(fmtDate(l.createdAt))}</span>
        </div>
        <span class="gs-flow-pt">+${l.points}</span>
      </li>`).join('')}
    </ol>
  </section>`;
}

/* ------------------------------------------------------------ 工具函数 -- */

function levelOf(points) {
  let cur = LEVELS[0];
  for (const l of LEVELS) {
    if (points >= l.at) cur = l;
  }
  return cur;
}

/** 当前积分在 [from, to) 区间内的百分比，用于进度条。 */
function progressOf(points, from, to) {
  if (to <= from) return 100;
  const pct = Math.round(((points - from) / (to - from)) * 100);
  return Math.max(0, Math.min(100, pct));
}
