/* ============================================================================
   pages-experts.js — 专家广场、专家详情、入驻申请、专家工作台。

   这四个页面补的是整条链路上最后一个断点：专家相关的接口从第一个版本就在，
   但页面上只有后台的审核列表。专家本人既申请不了，也看不到自己的收入，
   全靠 curl 维护——接口再全，没有入口就等于没做。

   三处刻意的取舍：

   1. 入驻表单是「一页四段」，不是四步向导。被驳回的人回来通常只改一个字段，
      向导会逼他把前面几步再点一遍（还不能跳步）。分段同样有"走到哪了"的
      节奏，却不用维护一套步骤状态机，也不用处理"刷新后回到第几步"。

   2. 提交成功后先 refreshSession() 再跳工作台。身份其实已经以库为准了
      （middleware.Auth 每个请求都复核账号与专家状态），接口层面不刷新也能过；
      但前端判断"要不要显示专家入口"读的是 localStorage 里登录时的用户快照，
      换一次令牌才能让这份快照跟上，否则刚提交完看不到工作台入口。

   3. 工作台的技能与报价来自 dashboard 接口，服务端一并下发，前端不拼装。
      给一张空表单让人保存，等于用一次点击把已有资料清空。
   ========================================================================= */

import { api, isLoggedIn, ApiError, uploadFile, refreshSession } from './api.js';
import { FLAGS } from './flags.js';
import { esc, fmtDate, fmtDateTime, fmtNum, icon, setHashQuery } from './util.js';
import { avatarHTML, bindPager, emptyHTML, pagerHTML, toastError, toastOk } from './ui.js';
import { fmtCents } from './orders.js';

const PAGE_SIZE = 9;

/** 排序口径必须与服务端 dto.ExpertListQuery.Sort 的可选值逐字一致。 */
const SORTS = [
  { key: 'rating', label: '评分优先' },
  { key: 'orders', label: '接单最多' },
  { key: 'price_asc', label: '价格从低到高' },
  { key: 'price_desc', label: '价格从高到低' },
  { key: 'latest', label: '最新入驻' },
];

/** 服务方式的中文名。key 与 model.ServiceMode* 一一对应。 */
const SERVICE_MODES = [
  { key: 'im', label: '图文' },
  { key: 'voice', label: '语音' },
  { key: 'video', label: '视频' },
  { key: 'screen', label: '屏幕共享' },
];

const LEVELS = [
  { key: 'junior', label: '初级' },
  { key: 'senior', label: '高级' },
  { key: 'expert', label: '资深' },
];

/** 入驻/认证状态。空串表示"从没申请过"，服务端就是这么返回的，不需要 404。 */
const STATUS_META = {
  '': { label: '未申请', tone: 'muted' },
  pending: { label: '审核中', tone: 'warn' },
  approved: { label: '已认证', tone: 'ok' },
  rejected: { label: '未通过', tone: 'danger' },
  suspended: { label: '已停牌', tone: 'muted' },
};

const WITHDRAW_STATUS = {
  pending: { label: '待审核', tone: 'warn' },
  approved: { label: '待打款', tone: 'info' },
  paid: { label: '已打款', tone: 'ok' },
  rejected: { label: '已驳回', tone: 'danger' },
  failed: { label: '打款失败', tone: 'danger' },
};

const INTRO_MIN = 20;
const INTRO_MAX = 2000;
const MAX_SKILLS = 5;
const MAX_CERTS = 10;
const MIN_PRICE_CENTS = 100;
const MAX_PRICE_CENTS = 10000000;
const MIN_WITHDRAW_CENTS = 1000;

/* ==================================================== 专家广场 ====== */

export async function expertListPage(ctx) {
  const p = ctx.params;
  const page = Math.max(1, Number(p.get('page') || 1));
  const sort = SORTS.some((s) => s.key === p.get('sort')) ? p.get('sort') : 'rating';
  const skillId = p.get('skillId') || '';
  const keyword = (p.get('keyword') || '').trim();
  const onlineOnly = p.get('online') === '1';

  const [skills, result] = await Promise.all([
    api.skills().catch(() => []),
    api.experts({
      page,
      pageSize: PAGE_SIZE,
      sort,
      skillId: skillId || undefined,
      keyword: keyword || undefined,
      onlineOnly: onlineOnly || undefined,
    }),
  ]);

  const list = Array.isArray(result.list) ? result.list : [];
  const total = Number(result.total || 0);
  const filtered = Boolean(skillId || keyword || onlineOnly);

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a><span class="crumb-sep">/</span><span>专家广场</span>
    </nav>

    ${FLAGS.expertPlaza ? '' : `<div class="card card-pad notice-bar">
      <b>专家广场正在整理中</b>
      <p class="muted-note">入口已暂时收起，专家主页仍可通过订单与问答里的链接访问。
        找专家解决具体问题，请走悬赏通道。</p>
    </div>`}

    ${pageHead('专家广场', '按技术方向找人。有认证、有接单记录、价格公开——不用先加微信问价。')}

    <div class="toolbar">
      <form class="expert-search" id="ex-search" role="search">
        <input class="input" id="ex-keyword" type="search" value="${esc(keyword)}"
               placeholder="搜昵称或技能，例如 nginx" aria-label="搜索专家" />
        <button class="btn" type="submit">搜索</button>
      </form>

      <select class="select" data-filter="skillId" aria-label="按技术方向筛选">
        <option value="">全部技术方向</option>
        ${skills.map((s) => `<option value="${esc(String(s.id))}"${
          String(s.id) === skillId ? ' selected' : ''}>${esc(s.name)}</option>`).join('')}
      </select>

      <button class="btn btn-sm ${onlineOnly ? 'btn-primary' : ''}" type="button" data-online
              aria-pressed="${onlineOnly ? 'true' : 'false'}">
        ${icon('activity', 14)}只看接单中
      </button>

      <div class="seg" role="group" aria-label="排序">
        ${SORTS.map((s) => `<button type="button" data-sort="${s.key}"
          class="${s.key === sort ? 'is-active' : ''}">${s.label}</button>`).join('')}
      </div>

      <span class="toolbar-spacer"></span>
      <span class="toolbar-count">${fmtNum(total)} 位专家</span>
    </div>

    ${list.length
      ? `<div class="expert-grid">${list.map(expertCardHTML).join('')}</div>
         ${pagerHTML({ page, pageSize: PAGE_SIZE, total, unit: '位' })}`
      : emptyHTML({
          iconName: 'shield',
          title: filtered ? '这些条件下暂时没有专家' : '首批专家正在认证中',
          desc: filtered
            ? '换个技术方向，或者取消「只看接单中」再看看。'
            : '平台正在邀请一线运维工程师入驻。如果你自己就是，现在申请可以排进首批名单。',
          actionHTML: filtered
            ? '<button class="btn" type="button" data-clear>清空筛选</button>'
            : `<a class="btn btn-primary" href="#/expert/apply">申请成为专家</a>`,
        })}

    ${total > 0 ? recruitBannerHTML() : ''}
  `;

  const mount = (root) => {
    const form = root.querySelector('#ex-search');
    const keywordEl = root.querySelector('#ex-keyword');
    if (form) {
      form.addEventListener('submit', (e) => {
        e.preventDefault();
        setHashQuery({ keyword: keywordEl.value.trim() });
      });
    }

    root.querySelectorAll('[data-filter]').forEach((sel) => {
      sel.addEventListener('change', () => {
        setHashQuery({ [sel.getAttribute('data-filter')]: sel.value });
      });
    });

    root.querySelectorAll('[data-sort]').forEach((btn) => {
      btn.addEventListener('click', () => setHashQuery({ sort: btn.getAttribute('data-sort') }));
    });

    const onlineBtn = root.querySelector('[data-online]');
    if (onlineBtn) {
      onlineBtn.addEventListener('click', () => setHashQuery({ online: onlineOnly ? '' : '1' }));
    }

    const clear = root.querySelector('[data-clear]');
    if (clear) clear.addEventListener('click', () => setHashQuery({ skillId: '', keyword: '', online: '' }));

    bindPager(root, (target) => setHashQuery({ page: target }, { keepPage: true }));
  };

  return { html, mount, title: '专家广场' };
}

function expertCardHTML(e) {
  const skills = (e.skills || []).map((s) => s.name).filter(Boolean);
  const shown = skills.slice(0, 3);
  const rest = skills.length - shown.length;

  return `<article class="card expert-card">
    <div class="expert-card-head">
      ${avatarHTML(e.nickname, e.avatar, 46)}
      <div class="expert-card-title">
        <div class="expert-name-row">
          <a class="expert-name" href="#/experts/${esc(String(e.id))}">${esc(e.nickname || '运维专家')}</a>
          ${e.realNameVerified
            ? `<span class="verified" title="实名与资质已核验">${icon('check', 12)}已认证</span>` : ''}
        </div>
        <div class="expert-card-skills">
          ${shown.map((n) => `<span class="chip">${esc(n)}</span>`).join('')}
          ${rest > 0 ? `<span class="chip chip-more">+${rest}</span>` : ''}
        </div>
      </div>
    </div>

    <div class="expert-card-stats">
      <span class="expert-stat"><b>${Number(e.rating || 0).toFixed(1)}</b>评分</span>
      <span class="expert-stat"><b>${fmtNum(e.orderCount || 0)}</b>完成单</span>
      <span class="expert-stat"><b>${Math.round(Number(e.goodRate || 0))}%</b>好评</span>
    </div>

    <div class="expert-card-foot">
      <div class="expert-price">
        ${e.pricePerHourCents > 0
          ? `<b>${esc(fmtCents(e.pricePerHourCents))}</b><span>/小时</span>`
          : e.pricePerTaskCents > 0
            ? `<b>${esc(fmtCents(e.pricePerTaskCents))}</b><span>/次</span>`
            : '<span class="muted">价格面议</span>'}
      </div>
      <div class="expert-card-actions">
        ${e.acceptingOrder
          ? '<span class="status-chip is-ok">接单中</span>'
          : '<span class="status-chip is-muted">暂不接单</span>'}
        <a class="btn btn-sm" href="#/experts/${esc(String(e.id))}">查看主页</a>
      </div>
    </div>
  </article>`;
}

function recruitBannerHTML() {
  return `<section class="recruit">
    <div>
      <h3>你也在替人排障？</h3>
      <p>认证通过后可以自己定价、自己决定接不接单，收入按单结算，随时可提。</p>
    </div>
    <a class="btn btn-primary" href="#/expert/apply">申请成为专家</a>
  </section>`;
}

/* ==================================================== 专家详情 ====== */

export async function expertDetailPage(ctx, id) {
  let e;
  let rv = null;
  try {
    // 评价与详情并行拉。评价失败不能让主页打不开——简介、报价、下单入口
    // 才是这一页的主要用途，评价区缺一块比整页 500 好得多。
    [e, rv] = await Promise.all([
      api.expert(id),
      api.expertReviews(id, { page: 1, pageSize: REVIEW_PAGE_SIZE }).catch(() => null),
    ]);
  } catch (err) {
    // 未通过认证的专家在服务端就是 404。这里给一页说明，比一张报错卡友好：
    // 多半是收藏了旧链接，或者对方主页已经下线。
    if (err instanceof ApiError && err.code === 30012) {
      return {
        html: `${pageHead('专家主页', '')}${emptyHTML({
          iconName: 'user',
          title: '这位专家的主页不在了',
          desc: '可能还没通过认证，或者已经停止接单。广场上还有其他人。',
          actionHTML: '<a class="btn btn-primary" href="#/experts">去专家广场</a>',
        })}`,
        title: '专家不存在',
      };
    }
    throw err;
  }

  const skills = e.skills || [];
  const modes = (e.serviceModes || []).map((k) => SERVICE_MODES.find((m) => m.key === k)).filter(Boolean);

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a><span class="crumb-sep">/</span>
      <a href="#/experts">专家广场</a><span class="crumb-sep">/</span>
      <span>${esc(e.nickname || '专家主页')}</span>
    </nav>

    <section class="card card-pad expert-hero">
      ${avatarHTML(e.nickname, e.avatar, 72)}
      <div class="expert-hero-main">
        <div class="expert-name-row">
          <h1>${esc(e.nickname || '运维专家')}</h1>
          ${e.realNameVerified
            ? `<span class="verified">${icon('check', 12)}实名与资质已核验</span>` : ''}
          ${e.acceptingOrder
            ? '<span class="status-chip is-ok">接单中</span>'
            : '<span class="status-chip is-muted">暂不接单</span>'}
        </div>
        <div class="expert-hero-stats">
          <span><b>${Number(e.rating || 0).toFixed(1)}</b>综合评分</span>
          <span><b>${fmtNum(e.orderCount || 0)}</b>完成单</span>
          <span><b>${Math.round(Number(e.goodRate || 0))}%</b>好评率</span>
          <span><b>${Math.round(Number(e.resolveRate || 0))}%</b>解决率</span>
          ${e.avgResponseSec > 0
            ? `<span><b>${fmtResponse(e.avgResponseSec)}</b>平均响应</span>` : ''}
        </div>
        ${modes.length
          ? `<div class="expert-modes">${modes.map((m) => `<span class="chip chip-brand">${esc(m.label)}</span>`).join('')}</div>`
          : ''}
      </div>
    </section>

    <div class="detail-layout" style="margin-top:20px">
      <div>
        <section class="card card-pad" style="margin-bottom:16px">
          <h2 class="block-title">技能与报价</h2>
          ${skills.length
            ? `<div class="skill-table">${skills.map((s) => `<div class="skill-line">
                <span class="skill-line-name">${esc(s.name)}</span>
                <span class="skill-line-meta">${esc(LEVEL_LABEL(s.level))}${
                  s.years > 0 ? ` · ${s.years} 年` : ''}</span>
              </div>`).join('')}</div>`
            : '<p class="muted">这位专家还没有填技能。</p>'}
          <div class="quote-row">
            <div class="quote-item">
              <span class="quote-label">按小时</span>
              <b>${e.pricePerHourCents > 0 ? esc(fmtCents(e.pricePerHourCents)) : '面议'}</b>
            </div>
            <div class="quote-item">
              <span class="quote-label">按次</span>
              <b>${e.pricePerTaskCents > 0 ? esc(fmtCents(e.pricePerTaskCents)) : '面议'}</b>
            </div>
          </div>
        </section>

        <section class="card card-pad" style="margin-bottom:16px">
          <h2 class="block-title">个人简介</h2>
          <p class="prose">${esc(e.intro || '这位专家还没有写简介。')}</p>
        </section>

        ${e.experience
          ? `<section class="card card-pad" style="margin-bottom:16px">
              <h2 class="block-title">工作经历</h2>
              <p class="prose">${esc(e.experience)}</p>
            </section>`
          : ''}

        ${reviewsSectionHTML(rv)}
      </div>

      <aside>
        <div class="card side-card expert-side">
          <div class="expert-side-price">
            ${e.pricePerTaskCents > 0
              ? `${esc(fmtCents(e.pricePerTaskCents))}<span>/次</span>`
              : e.pricePerHourCents > 0
                ? `${esc(fmtCents(e.pricePerHourCents))}<span>/小时</span>`
                : '价格面议'}
          </div>
          <p class="expert-side-hint">
            ${e.acceptingOrder
              ? '把问题描述和报错日志写清，通过得越快，对方越好判断。'
              : '这位专家目前不接新单，可以先发布悬赏，等人来抢。'}
          </p>
          <a class="btn btn-primary btn-block" href="#/ask">发布悬赏</a>
          <a class="btn btn-block" href="#/experts">看看其他专家</a>
          <div class="expert-side-note">
            ${icon('shield', 13)}
            <span>资金先托管，确认完成才结算给专家。</span>
          </div>
        </div>
      </aside>
    </div>
  `;

  return {
    html,
    title: e.nickname || '专家主页',
    mount: (root) => bindMoreReviews(root, id, rv),
  };
}

// REVIEW_PAGE_SIZE 评价区每页条数。
//
// 5 条而不是 20：评价是带正文的卡片，一屏能扫完的条数比"少点几次加载"
// 更重要；而且首屏与"加载更多"共用同一个 pageSize，不会出现第一页 20 条、
// 之后每点一次只多 5 条这种不一致。
const REVIEW_PAGE_SIZE = 5;

/**
 * 评价区的星级。
 *
 * 只画整颗星，四舍五入，不做半星：4.3 与 4.4 在视觉上本就该是同一种印象，
 * 半星的实现要引一套图标资源，收益远小于让评分数字本身精确到一位小数。
 */
function starsHTML(score, size = 13) {
  const n = Math.max(0, Math.min(5, Math.round(Number(score) || 0)));
  const stars = [1, 2, 3, 4, 5]
    .map((i) => icon('star', size, i <= n ? 'rv-star is-on' : 'rv-star'))
    .join('');
  return `<span class="rv-stars" role="img" aria-label="${n} 星">${stars}</span>`;
}

/**
 * 评价区整块。纯函数：喂同一次接口返回就出同一段 HTML，便于逐条渲染校验。
 *
 * 汇总与第一页由服务端一起下发，所以这一块不再请求接口；
 * 只有"加载更多"会再打一次分页接口。
 */
export function reviewsSectionHTML(page) {
  const s = (page && page.summary) || { total: 0, avgScore: 0, goodRate: 0, distribution: [] };
  if (!s.total) {
    return `
      <section class="card card-pad">
        <div class="block-head">
          <h2 class="block-title">服务评价</h2>
        </div>
        <p class="muted">还没有评价。订单完成并由下单人确认后，评分与描述会出现在这里。</p>
      </section>`;
  }

  const total = Number(s.total) || 0;
  const list = Array.isArray(page.list) ? page.list : [];
  const rows = (Array.isArray(s.distribution) ? s.distribution : []).map((b) => {
    const pct = total > 0 ? Math.round((Number(b.count) / total) * 100) : 0;
    return `<div class="rv-dist-row">
        <span class="rv-dist-label">${Number(b.score) || 0} 星</span>
        <span class="rv-dist-bar"><i style="width:${pct}%"></i></span>
        <span class="rv-dist-num">${fmtNum(b.count)}</span>
      </div>`;
  }).join('');

  const rest = Math.max(0, Number(page.total || total) - list.length);

  return `
    <section class="card card-pad">
      <div class="block-head">
        <h2 class="block-title">服务评价</h2>
        <span class="muted-note">共 ${fmtNum(total)} 条 · 好评率 ${Math.round(Number(s.goodRate) || 0)}%</span>
      </div>

      <div class="rv-summary">
        <div class="rv-score">
          <b>${Number(s.avgScore || 0).toFixed(1)}</b>
          ${starsHTML(s.avgScore, 15)}
          <span class="muted-note">${fmtNum(total)} 条评价</span>
        </div>
        <div class="rv-dist">${rows}</div>
      </div>

      <div class="rv-list" data-review-list>${reviewCardsHTML(list)}</div>
      ${rest > 0
        ? `<button class="btn btn-block" type="button" data-more-reviews>查看更多评价（还有 ${fmtNum(rest)} 条）</button>`
        : ''}
    </section>`;
}

/** 一批评价卡片的 HTML。翻页追加时前端直接 insertAdjacentHTML，不复用整块。 */
export function reviewCardsHTML(items) {
  const list = Array.isArray(items) ? items : [];
  return list.map(reviewCardHTML).join('');
}

function reviewCardHTML(r) {
  // 服务端已把匿名项的昵称/头像换成「匿名用户」，这里再兜一次底：
  // 展示层不信任上游——哪天接口忘了脱敏，前端也不能把实名数据吐出去。
  const item = r.anonymous ? { ...r, nickname: '匿名用户', avatar: '' } : r;

  const subs = [
    { label: '专业度', v: r.professionalScore },
    { label: '响应', v: r.speedScore },
    { label: '沟通', v: r.communicationScore },
  ].filter((x) => Number(x.v) > 0);

  return `
    <article class="rv-item">
      <div class="rv-item-head">
        ${avatarHTML(item.nickname, item.avatar, 32)}
        <div class="rv-item-who">
          <b>${esc(item.nickname || '运维用户')}</b>
          <span class="rv-item-time">${esc(fmtDate(r.createdAt))}</span>
        </div>
        ${starsHTML(r.score)}
      </div>
      ${r.content ? `<p class="rv-item-text">${esc(r.content)}</p>` : ''}
      ${subs.length
        ? `<div class="rv-item-tags">${subs.map((x) => `<span class="chip">${x.label} ${Number(x.v)}</span>`).join('')}</div>`
        : ''}
      ${r.reply
        ? `<div class="rv-reply"><b>专家回复</b><p class="prose">${esc(r.reply)}</p></div>`
        : ''}
    </article>`;
}

/** 「加载更多」：追加下一页，取完就摘掉按钮。 */
function bindMoreReviews(root, expertId, first) {
  const btn = root.querySelector('[data-more-reviews]');
  const listEl = root.querySelector('[data-review-list]');
  if (!btn || !listEl) return;

  let page = Number((first && first.page) || 1);
  let loaded = (first && Array.isArray(first.list)) ? first.list.length : 0;

  btn.addEventListener('click', async () => {
    btn.disabled = true;
    const label = btn.textContent;
    btn.textContent = '加载中…';
    try {
      const next = await api.expertReviews(expertId, { page: page + 1, pageSize: REVIEW_PAGE_SIZE });
      listEl.insertAdjacentHTML('beforeend', reviewCardsHTML(next.list));
      page += 1;
      loaded += Array.isArray(next.list) ? next.list.length : 0;
      // 用服务端返回的 total 判断是否取完，而不是"这页不足 pageSize 就停"：
      // 后者在有评价被隐藏/删除的边界上会提前收工。
      if (loaded >= Number(next.total || 0)) {
        btn.remove();
        return;
      }
      btn.disabled = false;
      btn.textContent = `查看更多评价（还有 ${fmtNum(Number(next.total || 0) - loaded)} 条）`;
    } catch (err) {
      btn.disabled = false;
      btn.textContent = label;
      toastError(err.message || '加载评价失败');
    }
  });
}

const LEVEL_LABEL = (level) => (LEVELS.find((l) => l.key === level) || {}).label || '未定级';

function fmtResponse(sec) {
  const n = Number(sec) || 0;
  if (n >= 3600) return `${Math.max(1, Math.round(n / 3600))} 小时`;
  if (n >= 60) return `${Math.max(1, Math.round(n / 60))} 分钟`;
  return `${n} 秒`;
}

/* ==================================================== 入驻申请 ====== */

export async function expertApplyPage(ctx) {
  if (!isLoggedIn()) {
    return loginGuide(
      '申请成为专家',
      '认证通过后可以自己定价、自己决定接不接单。',
      '入驻申请与账号绑定，审核结果与驳回原因都会发到这个账号下。'
    );
  }

  // 技能与上传限制并行取：都失败也要能把表单打开，
  // 只是少几个可选项、少一行"支持哪些格式"的提示。
  const [status, skills, scenes] = await Promise.all([
    api.expertApplyStatus(),
    api.skills().catch(() => []),
    api.uploadScenes().catch(() => []),
  ]);

  const state = status.status || '';

  // 审核中 / 已通过：不再给表单。重新提交会被服务端拒绝（30011），
  // 与其让人填完一屏再吃一个错误，不如一进来就说清楚。
  if (state === 'pending' || state === 'approved') {
    return { html: applyStatusHTML(status), title: '入驻申请' };
  }

  const certScene = scenes.find((s) => s.scene === 'expert_cert') || {};
  const draft = status.draft || null;

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a><span class="crumb-sep">/</span>
      <a href="#/experts">专家广场</a><span class="crumb-sep">/</span>
      <span>入驻申请</span>
    </nav>

    ${pageHead('申请成为运维专家',
      '填完提交后进入审核队列，通常 1–2 个工作日出结果。资质只给审核方看。')}

    ${state === 'rejected' || state === 'suspended' ? `
      <div class="notice is-danger">
        <strong>${state === 'rejected' ? '上次申请未通过' : '账号已被停牌'}</strong>
        <p>${esc(status.auditRemark || '没有给出具体原因，可以直接修改后重新提交。')}</p>
        <p class="muted">下面的表单已经填好上次的内容，改完再提交即可。</p>
      </div>` : ''}

    <div class="detail-layout">
      <form class="card card-pad" id="ap-form" novalidate>
        <section class="apply-section">
          <h3><span class="apply-step">1</span>实名信息</h3>
          <p class="apply-hint">只用于资质核验。前台永远只显示打码后的号码，接口也不会返回完整证件号。</p>
          <div class="form-row">
            <div class="field">
              <label for="ap-name">真实姓名</label>
              <input class="input" id="ap-name" maxlength="32" autocomplete="name"
                     value="${esc(draft ? draft.realName : '')}" placeholder="与证件一致" />
            </div>
            <div class="field">
              <label for="ap-idcard">身份证号</label>
              ${draft
                ? `<input class="input" id="ap-idcard" maxlength="18" inputmode="text"
                          placeholder="留空即沿用已登记的 ${esc(status.maskedIdCard || '号码')}" />
                   <span class="hint">已经登记过，改证件才需要重填</span>`
                : `<input class="input" id="ap-idcard" maxlength="18" inputmode="text"
                          placeholder="18 位，末位可能是 X" />
                   <span class="hint">会用校验位验证，编一个号码过不去</span>`}
            </div>
          </div>

          <div class="field">
            <label>头像 <span class="hint">选填，公开主页会显示</span></label>
            <div class="upload-inline" data-avatar-box></div>
          </div>
        </section>

        <section class="apply-section">
          <h3><span class="apply-step">2</span>专业能力</h3>
          <p class="apply-hint">最多 ${MAX_SKILLS} 项。等级与年限会出现在主页上，也决定用户筛选时能不能搜到你。</p>
          <div class="chips-select" data-skill-chips></div>
          <div class="picked-skills" data-skill-rows></div>
        </section>

        <section class="apply-section">
          <h3><span class="apply-step">3</span>资质材料</h3>
          <p class="apply-hint">
            执业证书、厂商认证、能证明你做过这件事的工单截图都行，最多 ${MAX_CERTS} 份，
            单份不超过 ${esc(String(certScene.maxSizeMb || 10))}MB。
            <b>私有存储，只有审核方能看到。</b>
          </p>
          <div class="upload-list" data-cert-list></div>
          <label class="upload-add">
            <input type="file" multiple hidden data-cert-file
                   accept="${esc(certScene.accept || 'image/jpeg,image/png,application/pdf')}" />
            ${icon('plus', 15)}<span>选择文件</span>
          </label>
        </section>

        <section class="apply-section">
          <h3><span class="apply-step">4</span>简介、服务方式与定价</h3>
          <p class="apply-hint">写清「你解决过什么类型的问题」，比堆一串认证名头有用。</p>

          <div class="field">
            <label for="ap-intro">个人简介 <span class="hint" data-intro-count></span></label>
            <textarea class="input" id="ap-intro" rows="6" maxlength="${INTRO_MAX}"
              placeholder="例如：做过 6 年 K8s 平台运维，擅长容器网络、存储与大规模集群升级；常处理 Calico 不通、etcd 慢查询、节点 NotReady 这类问题。">${esc(draft ? draft.intro : '')}</textarea>
          </div>

          <div class="field">
            <label for="ap-exp">工作经历 <span class="hint">选填</span></label>
            <textarea class="input" id="ap-exp" rows="4" maxlength="2000"
              placeholder="公司 / 岗位 / 年限，一段一行">${esc(draft ? draft.experience : '')}</textarea>
          </div>

          <div class="field">
            <label>服务方式 <span class="hint">至少选一种</span></label>
            <div class="chips-select" data-mode-chips></div>
          </div>

          <div class="form-row">
            <div class="field">
              <label for="ap-hour">按小时报价（元）</label>
              <input class="input" id="ap-hour" type="number" min="0" step="1"
                     value="${esc(draft && draft.pricePerHourCents ? String(Math.round(draft.pricePerHourCents / 100)) : '')}"
                     placeholder="例如 200" />
              <span class="hint">填 0 或留空表示暂不接按小时的单</span>
            </div>
            <div class="field">
              <label for="ap-task">按次报价（元）</label>
              <input class="input" id="ap-task" type="number" min="0" step="1"
                     value="${esc(draft && draft.pricePerTaskCents ? String(Math.round(draft.pricePerTaskCents / 100)) : '')}"
                     placeholder="例如 600" />
              <span class="hint">一次问题从排查到给结论，按这个价结算</span>
            </div>
          </div>
        </section>

        <p class="form-summary" id="ap-tip"></p>

        <div class="form-actions">
          <button class="btn btn-primary btn-lg" id="ap-submit" type="submit">
            ${state === 'rejected' || state === 'suspended' ? '重新提交审核' : '提交审核'}
          </button>
          <a class="btn btn-lg" href="#/experts">先看看专家广场</a>
        </div>
      </form>

      <aside>
        <div class="card side-card" style="margin-bottom:16px">
          <h3>审核看什么</h3>
          <ol class="steps">
            <li>证件与姓名是否一致（只核验，不留存明文）</li>
            <li>资质材料能不能对上你选的技能方向</li>
            <li>简介里有没有具体故障场景，而不是空泛的形容词</li>
            <li>报价是否明显偏离同类专家</li>
          </ol>
        </div>
        <div class="card side-card">
          <h3>通过之后</h3>
          <ul class="tips-list">
            <li>可以自己开关「接单」状态，随时暂停</li>
            <li>技能、简介、报价都能在工作台随时改</li>
            <li>收入按单结算，可提现到银行卡 / 支付宝 / 微信</li>
            <li>会被停牌的情况：爽约、私下交易、伪造资质</li>
          </ul>
        </div>
      </aside>
    </div>
  `;

  const mount = (root) => mountApplyForm(root, { draft, skills, state });
  return { html, mount, title: '入驻申请' };
}

function applyStatusHTML(status) {
  const meta = STATUS_META[status.status] || STATUS_META[''];
  const approved = status.status === 'approved';

  return `
    <nav class="crumb">
      <a href="#/">首页</a><span class="crumb-sep">/</span>
      <a href="#/experts">专家广场</a><span class="crumb-sep">/</span>
      <span>入驻申请</span>
    </nav>

    ${pageHead('入驻申请', approved ? '认证已通过。' : '材料已经收到，正在排队审核。')}

    <section class="card card-pad apply-status">
      <div class="apply-status-head">
        <span class="status-chip is-${meta.tone}">${meta.label}</span>
        <h2>${approved ? '你已经是认证专家' : '审核中，通常 1–2 个工作日'}</h2>
      </div>
      <dl class="kv">
        <div class="kv-row"><dt>姓名</dt><dd>${esc(status.realName || '—')}</dd></div>
        <div class="kv-row"><dt>证件号</dt><dd class="mono">${esc(status.maskedIdCard || '—')}</dd></div>
        <div class="kv-row"><dt>提交时间</dt><dd>${esc(status.appliedAt ? fmtDateTime(status.appliedAt) : '—')}</dd></div>
        ${status.auditedAt ? `<div class="kv-row"><dt>审核时间</dt><dd>${esc(fmtDateTime(status.auditedAt))}</dd></div>` : ''}
        ${status.auditRemark ? `<div class="kv-row"><dt>审核意见</dt><dd>${esc(status.auditRemark)}</dd></div>` : ''}
      </dl>

      <div class="form-actions">
        ${approved
          ? '<a class="btn btn-primary btn-lg" href="#/expert">进入工作台</a>'
          : '<a class="btn btn-lg" href="#/expert">看看工作台</a>'}
        <a class="btn btn-lg" href="#/experts">去专家广场</a>
      </div>

      <p class="apply-status-note">
        ${approved
          ? '别忘了去工作台把「接单中」打开，并把技能与报价补齐——这两项决定你会不会出现在筛选结果里。'
          : '审核期间不用重复提交，重复提交会被拒。结果出来后这个页面会变，账号区的「工作台」也会出现。'}
      </p>
    </section>
  `;
}

/* --------------------------------------------------- 申请表单逻辑 -- */

function mountApplyForm(root, { draft, skills, state }) {
  const form = root.querySelector('#ap-form');
  const nameEl = root.querySelector('#ap-name');
  const idCardEl = root.querySelector('#ap-idcard');
  const introEl = root.querySelector('#ap-intro');
  const expEl = root.querySelector('#ap-exp');
  const hourEl = root.querySelector('#ap-hour');
  const taskEl = root.querySelector('#ap-task');
  const tipEl = root.querySelector('#ap-tip');
  const submitBtn = root.querySelector('#ap-submit');
  const countEl = root.querySelector('[data-intro-count]');

  const picked = new Map(); // skillId(string) -> { skillId, level, years }
  const certs = [];
  const modes = new Set();
  let avatar = '';

  (draft && draft.skills ? draft.skills : []).forEach((s) => {
    picked.set(String(s.skillId), {
      skillId: s.skillId,
      level: s.level || 'senior',
      years: Number(s.years) || 0,
    });
  });
  (draft && draft.serviceModes ? draft.serviceModes : ['im']).forEach((m) => modes.add(m));
  (draft && draft.avatar ? [draft.avatar] : []).forEach((a) => { avatar = a; });
  (draft && draft.certFiles ? draft.certFiles : []).forEach((f) => {
    certs.push({ key: f.key, url: f.url, name: (f.key || '').split('/').pop() });
  });

  const tip = (text, tone = 'muted') => {
    tipEl.textContent = text;
    tipEl.classList.toggle('is-error', tone === 'error');
    tipEl.classList.toggle('is-ok', tone === 'ok');
  };

  const chipRow = (items, isOn, onPick, labelOf = (x) => x.label) =>
    items.map((it) => `<button type="button" class="chip chip-pick ${isOn(it) ? 'is-on' : ''}"
      data-key="${esc(it.key)}" aria-pressed="${isOn(it) ? 'true' : 'false'}">${esc(labelOf(it))}</button>`).join('');

  /* --- 技能 --- */
  const chipsBox = root.querySelector('[data-skill-chips]');
  const rowsBox = root.querySelector('[data-skill-rows]');

  const renderSkills = () => {
    chipsBox.innerHTML = chipRow(
      skills.map((s) => ({ key: String(s.id), label: s.name })),
      (it) => picked.has(it.key),
      null
    );
    chipsBox.querySelectorAll('[data-key]').forEach((btn) => {
      btn.addEventListener('click', () => {
        const key = btn.getAttribute('data-key');
        if (picked.has(key)) {
          picked.delete(key);
        } else {
          if (picked.size >= MAX_SKILLS) {
            toastError(`最多选 ${MAX_SKILLS} 项技能`);
            return;
          }
          picked.set(key, { skillId: Number(key), level: 'senior', years: 0 });
        }
        renderSkills();
        renderRows();
      });
    });

    if (!skills.length) {
      chipsBox.innerHTML = '<span class="muted">技能清单暂时取不到，请刷新页面重试。</span>';
    }
  };

  const renderRows = () => {
    if (!picked.size) {
      rowsBox.innerHTML = '';
      return;
    }
    const nameOf = new Map(skills.map((s) => [String(s.id), s.name]));
    rowsBox.innerHTML = [...picked.entries()].map(([key, v]) => `
      <div class="picked-row" data-row="${esc(key)}">
        <span class="picked-name">${esc(nameOf.get(key) || `技能 ${key}`)}</span>
        <select class="select" data-level aria-label="等级">
          ${LEVELS.map((l) => `<option value="${l.key}"${l.key === v.level ? ' selected' : ''}>${l.label}</option>`).join('')}
        </select>
        <input class="input" data-years type="number" min="0" max="60" step="1"
               value="${esc(String(v.years || 0))}" aria-label="从业年限" /><span class="picked-unit">年</span>
        <button type="button" class="icon-btn" data-remove aria-label="移除这项技能">${icon('close', 14)}</button>
      </div>`).join('');

    rowsBox.querySelectorAll('[data-row]').forEach((row) => {
      const key = row.getAttribute('data-row');
      const rec = picked.get(key);
      if (!rec) return;
      row.querySelector('[data-level]').addEventListener('change', (e) => { rec.level = e.target.value; });
      row.querySelector('[data-years]').addEventListener('input', (e) => {
        rec.years = Math.max(0, Math.min(60, Number(e.target.value) || 0));
      });
      row.querySelector('[data-remove]').addEventListener('click', () => {
        picked.delete(key);
        renderSkills();
        renderRows();
      });
    });
  };

  /* --- 服务方式 --- */
  const modeBox = root.querySelector('[data-mode-chips]');
  const renderModes = () => {
    modeBox.innerHTML = chipRow(
      SERVICE_MODES.map((m) => ({ key: m.key, label: m.label })),
      (it) => modes.has(it.key)
    );
    modeBox.querySelectorAll('[data-key]').forEach((btn) => {
      btn.addEventListener('click', () => {
        const key = btn.getAttribute('data-key');
        if (modes.has(key)) modes.delete(key);
        else modes.add(key);
        renderModes();
      });
    });
  };

  /* --- 头像 --- */
  const avatarBox = root.querySelector('[data-avatar-box]');
  const renderAvatar = () => {
    avatarBox.innerHTML = `
      ${avatarHTML('我', avatar, 44)}
      <label class="btn btn-sm upload-inline-btn">
        上传头像
        <input type="file" accept="image/jpeg,image/png,image/webp,image/gif" hidden data-avatar-file />
      </label>
      ${avatar ? `<button type="button" class="btn btn-sm" data-avatar-clear>移除</button>` : ''}
      <span class="hint">≤ 2MB，JPG / PNG / WebP</span>`;

    const input = avatarBox.querySelector('[data-avatar-file]');
    input.addEventListener('change', async () => {
      const file = input.files && input.files[0];
      if (!file) return;
      input.disabled = true;
      try {
        const res = await uploadFile('avatar', file);
        avatar = res.url || '';
        renderAvatar();
        toastOk('头像已上传');
      } catch (err) {
        toastError(err instanceof ApiError ? err.message : '头像上传失败');
        input.disabled = false;
      }
    });

    const clear = avatarBox.querySelector('[data-avatar-clear]');
    if (clear) {
      clear.addEventListener('click', () => {
        avatar = '';
        renderAvatar();
      });
    }
  };

  /* --- 资质材料 --- */
  const certBox = root.querySelector('[data-cert-list]');
  root.querySelector('[data-cert-file]').addEventListener('change', async (e) => {
    const input = e.target;
    const files = [...(input.files || [])];
    if (!files.length) return;

    for (const file of files) {
      if (certs.length >= MAX_CERTS) {
        toastError(`最多上传 ${MAX_CERTS} 份材料`);
        break;
      }
      try {
        const res = await uploadFile('expert_cert', file);
        certs.push({ key: res.key, url: res.url, name: res.name || file.name });
        renderCerts();
      } catch (err) {
        toastError(`${file.name}：${err instanceof ApiError ? err.message : '上传失败'}`);
      }
    }
    input.value = '';
  });

  const renderCerts = () => {
    if (!certs.length) {
      certBox.innerHTML = '<p class="muted">还没上传材料。至少要有一份——这是审核的主要依据。</p>';
      return;
    }
    certBox.innerHTML = certs.map((c, i) => `
      <div class="cert-item">
        <span class="cert-icon">${icon('book', 15)}</span>
        <a class="cert-name" href="${esc(c.url || '#')}" target="_blank" rel="noopener">${esc(c.name || '资质材料')}</a>
        <button type="button" class="icon-btn" data-cert-remove="${i}" aria-label="移除">${icon('trash', 14)}</button>
      </div>`).join('');

    certBox.querySelectorAll('[data-cert-remove]').forEach((btn) => {
      btn.addEventListener('click', () => {
        certs.splice(Number(btn.getAttribute('data-cert-remove')), 1);
        renderCerts();
      });
    });
  };

  /* --- 简介字数 --- */
  const renderCount = () => {
    const n = [...introEl.value.trim()].length;
    countEl.textContent = `${n}/${INTRO_MAX}，至少 ${INTRO_MIN} 字`;
    countEl.classList.toggle('is-error', n > 0 && n < INTRO_MIN);
  };
  introEl.addEventListener('input', renderCount);

  /* --- 提交 --- */
  form.addEventListener('submit', async (e) => {
    e.preventDefault();

    const realName = nameEl.value.trim();
    const idCardRaw = idCardEl.value.trim().toUpperCase();
    const intro = introEl.value.trim();

    if ([...realName].length < 2 || [...realName].length > 32) {
      tip('姓名需要 2–32 个字', 'error');
      nameEl.focus();
      return;
    }
    if (idCardRaw && !idCardValid(idCardRaw)) {
      tip('身份证号不合规：需要 18 位，且校验位要对得上', 'error');
      idCardEl.focus();
      return;
    }
    if (!idCardRaw && !draft) {
      tip('请填写身份证号', 'error');
      idCardEl.focus();
      return;
    }
    if (picked.size < 1) {
      tip('至少选一项技能', 'error');
      return;
    }
    if (!certs.length) {
      tip('至少上传一份资质材料', 'error');
      return;
    }
    if (certs.length > MAX_CERTS) {
      tip(`最多 ${MAX_CERTS} 份资质材料`, 'error');
      return;
    }
    const introLen = [...intro].length;
    if (introLen < INTRO_MIN || introLen > INTRO_MAX) {
      tip(`个人简介需要 ${INTRO_MIN}–${INTRO_MAX} 字，现在 ${introLen} 字`, 'error');
      introEl.focus();
      return;
    }
    if (!modes.size) {
      tip('至少选一种服务方式', 'error');
      return;
    }
    const hourCents = centsOf(hourEl.value);
    const taskCents = centsOf(taskEl.value);
    const priceErr = priceProblem(hourCents) || priceProblem(taskCents);
    if (priceErr) {
      tip(priceErr, 'error');
      return;
    }

    const payload = {
      realName,
      certFiles: certs.map((c) => c.key),
      intro,
      experience: expEl.value.trim(),
      avatar,
      skills: [...picked.values()].map((s) => ({
        skillId: s.skillId,
        level: s.level,
        years: s.years,
      })),
      serviceModes: [...modes],
      pricePerHourCents: hourCents,
      pricePerTaskCents: taskCents,
    };
    // 留空即沿用已登记证件号：服务端按库里有没有记录判定，前端不猜。
    if (idCardRaw) payload.idCardNo = idCardRaw;

    submitBtn.disabled = true;
    const original = submitBtn.textContent;
    submitBtn.textContent = '正在提交…';
    try {
      await api.applyExpert(payload);
      // role 与 expertStatus 签在令牌里，换一次令牌再去工作台，
      // 否则刚提交完就是「请先提交入驻申请」。
      await refreshSession();
      toastOk('申请已提交，等待审核');
      location.hash = '#/expert';
    } catch (err) {
      submitBtn.disabled = false;
      submitBtn.textContent = original;
      tip(err instanceof ApiError ? err.message : '提交失败，请稍后再试', 'error');
    }
  });

  renderSkills();
  renderRows();
  renderModes();
  renderAvatar();
  renderCerts();
  renderCount();
  if (state === 'rejected' || state === 'suspended') {
    tip('上次未通过的原因见上方提示，改完直接重新提交。');
  }
}

/* ==================================================== 专家工作台 ==== */

export async function expertWorkbenchPage(ctx) {
  if (!isLoggedIn()) {
    return loginGuide('专家工作台', '接单、报价、收入都在这里。', '工作台与账号绑定，请先登录。');
  }

  let d;
  try {
    d = await api.expertDashboard();
  } catch (err) {
    // 还没申请过的人打开这个地址是正常的：告诉他去哪儿申请，别丢一张 403。
    if (err instanceof ApiError && (err.code === 30002 || err.code === 30003 || err.code === 30004 || err.code === 30005)) {
      return {
        html: `${pageHead('专家工作台', '')}${emptyHTML({
          iconName: 'shield',
          title: '还没有通过认证',
          desc: err.message,
          actionHTML: '<a class="btn btn-primary" href="#/expert/apply">去提交入驻申请</a>',
        })}`,
        title: '专家工作台',
      };
    }
    throw err;
  }

  const tab = ['services', 'profile', 'wallet'].includes(ctx.params.get('tab'))
    ? ctx.params.get('tab') : 'services';

  const statusMeta = STATUS_META[d.status] || STATUS_META[''];
  const approved = d.status === 'approved';

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a><span class="crumb-sep">/</span>
      <span>专家工作台</span>
    </nav>

    <div class="page-head workbench-head">
      <div>
        <h1>专家工作台</h1>
        <p class="lede">
          <span class="status-chip is-${statusMeta.tone}">${statusMeta.label}</span>
          <span class="workbench-name">${esc(d.realName || '')}</span>
          <span class="muted">${esc(d.maskedIdCard || '')}</span>
        </p>
      </div>
      <div class="workbench-switch">
        <span class="hint">接单状态</span>
        <button class="switch ${d.acceptingOrder ? 'is-on' : ''}" type="button" id="accept-switch"
                role="switch" aria-checked="${d.acceptingOrder ? 'true' : 'false'}">
          <span class="switch-knob"></span>
        </button>
        <span class="switch-label" id="accept-label">${d.acceptingOrder ? '接单中' : '已暂停'}</span>
      </div>
    </div>

    ${d.status !== 'approved' ? `
      <div class="notice is-warn">
        <strong>${statusMeta.label}</strong>
        <p>${d.status === 'pending'
          ? '资质正在审核，审核通过后才能接单、收款。现在可以先看看工作台长什么样。'
          : '去「入驻申请」修改资料后重新提交，通过前接单与收款都会失败。'}</p>
        <a class="btn btn-sm" href="#/expert/apply">查看申请状态</a>
      </div>` : ''}

    <div class="metric-grid">
      ${metricHTML('本月收入', fmtCents(d.monthIncomeCents), 'income')}
      ${metricHTML('可提现余额', fmtCents(d.balanceCents), 'balance')}
      ${metricHTML('累计收入', fmtCents(d.totalIncomeCents), 'total')}
      ${metricHTML('完成单数', fmtNum(d.orderCount), 'orders', `待接 ${fmtNum(d.pendingAccept)}`)}
      ${metricHTML('综合评分', Number(d.rating || 0).toFixed(1), 'rating', `好评 ${Math.round(Number(d.goodRate || 0))}%`)}
      ${metricHTML('冻结中', fmtCents(d.frozenCents), 'frozen', '提现处理中')}
    </div>

    ${approved ? `
      <div class="seg workbench-tabs" role="tablist">
        <a href="#/expert?tab=services" class="${tab === 'services' ? 'is-active' : ''}">服务设置</a>
        <a href="#/expert?tab=profile" class="${tab === 'profile' ? 'is-active' : ''}">主页内容</a>
        <a href="#/expert?tab=wallet" class="${tab === 'wallet' ? 'is-active' : ''}">收入钱包</a>
      </div>

      <div id="wb-panel">${loadingPanelHTML()}</div>
    ` : `
      <div class="card card-pad">
        <h2 class="block-title">审核通过后才能编辑</h2>
        <p class="muted" style="font-size:13px;line-height:1.8;margin:0">
          服务设置、主页内容与钱包都要求资质已通过审核——这是服务端的硬约束，
          不是这里少画了几个按钮。现在能做的只有两件事：查看申请状态，或者等审核结果。
        </p>
        <div class="form-actions" style="margin-top:14px">
          <a class="btn btn-primary" href="#/expert/apply">查看申请状态</a>
          <a class="btn" href="#/experts">去专家广场</a>
        </div>
      </div>
    `}
  `;

  const mount = async (root) => {
    bindAcceptSwitch(root, d);
    if (!approved) return;

    const panel = root.querySelector('#wb-panel');
    try {
      if (tab === 'services') {
        const skills = await api.skills().catch(() => []);
        panel.innerHTML = servicesPanelHTML(d, skills);
        bindServicesPanel(panel, d);
      } else if (tab === 'profile') {
        panel.innerHTML = profilePanelHTML(d);
        bindProfilePanel(panel, d);
      } else {
        const w = await api.expertWallet();
        panel.innerHTML = walletPanelHTML(w);
        bindWalletPanel(panel, w);
      }
    } catch (err) {
      panel.innerHTML = `<div class="card"><div class="empty" style="padding:40px 24px">
        <strong>这一栏没能加载出来</strong>
        <p>${esc(err instanceof ApiError ? err.message : '请稍后重试')}</p>
        <button class="btn" type="button" data-panel-retry>重新加载</button>
      </div></div>`;
      const retry = panel.querySelector('[data-panel-retry]');
      if (retry) retry.addEventListener('click', () => window.__reloadPage && window.__reloadPage());
    }
  };

  return { html, mount, title: '专家工作台' };
}

function metricHTML(label, value, key, sub) {
  return `<div class="metric-card metric-${esc(key)}">
    <span class="metric-label">${esc(label)}</span>
    <b class="metric-value">${esc(value)}</b>
    ${sub ? `<span class="metric-sub">${esc(sub)}</span>` : ''}
  </div>`;
}

function loadingPanelHTML() {
  return `<div class="card" style="padding:26px 22px">
    <div class="skeleton-line" style="width:38%;height:14px"></div>
    <div class="skeleton-line" style="width:70%;margin-top:12px"></div>
  </div>`;
}

function bindAcceptSwitch(root, d) {
  const btn = root.querySelector('#accept-switch');
  const label = root.querySelector('#accept-label');
  if (!btn) return;

  btn.addEventListener('click', async () => {
    if (d.status !== 'approved') {
      toastError('审核通过后才能开启接单');
      return;
    }
    const next = btn.getAttribute('aria-checked') !== 'true';
    btn.disabled = true;
    try {
      await api.updateExpertServices({ acceptingOrder: next });
      d.acceptingOrder = next;
      btn.classList.toggle('is-on', next);
      btn.setAttribute('aria-checked', String(next));
      label.textContent = next ? '接单中' : '已暂停';
      toastOk(next ? '已开启接单' : '已暂停接单');
    } catch (err) {
      toastError(err instanceof ApiError ? err.message : '操作失败');
    } finally {
      btn.disabled = false;
    }
  });
}

/* ------------------------------------------------------ 服务设置 -- */

function servicesPanelHTML(d, skills) {
  const picked = new Map((d.skills || []).map((s) => [String(s.skillId), s]));
  const modes = new Set(d.serviceModes || []);

  return `<form class="card card-pad" id="sv-form" novalidate>
    <h2 class="block-title">技能</h2>
    <p class="apply-hint">最多 ${MAX_SKILLS} 项，等级与年限会显示在公开主页上。</p>
    <div class="chips-select" data-skill-chips>
      ${skills.map((s) => `<button type="button" class="chip chip-pick ${picked.has(String(s.id)) ? 'is-on' : ''}"
        data-key="${esc(String(s.id))}">${esc(s.name)}</button>`).join('')
        || '<span class="muted">技能清单暂时取不到</span>'}
    </div>
    <div class="picked-skills" data-skill-rows></div>

    <h2 class="block-title" style="margin-top:24px">服务方式</h2>
    <div class="chips-select" data-mode-chips>
      ${SERVICE_MODES.map((m) => `<button type="button" class="chip chip-pick ${modes.has(m.key) ? 'is-on' : ''}"
        data-key="${m.key}">${esc(m.label)}</button>`).join('')}
    </div>

    <h2 class="block-title" style="margin-top:24px">报价</h2>
    <div class="form-row">
      <div class="field">
        <label for="sv-hour">按小时（元）</label>
        <input class="input" id="sv-hour" type="number" min="0" step="1"
               value="${d.pricePerHourCents ? Math.round(d.pricePerHourCents / 100) : ''}" placeholder="0 表示不接" />
      </div>
      <div class="field">
        <label for="sv-task">按次（元）</label>
        <input class="input" id="sv-task" type="number" min="0" step="1"
               value="${d.pricePerTaskCents ? Math.round(d.pricePerTaskCents / 100) : ''}" placeholder="0 表示不接" />
      </div>
    </div>

    <p class="form-summary" id="sv-tip"></p>
    <div class="form-actions">
      <button class="btn btn-primary" type="submit" id="sv-submit">保存服务设置</button>
      ${d.expertId
        ? `<a class="btn" href="#/experts/${esc(String(d.expertId))}" target="_blank" rel="noopener">查看公开主页</a>`
        : ''}
    </div>
  </form>`;
}

function bindServicesPanel(panel, d) {
  const form = panel.querySelector('#sv-form');
  const tipEl = panel.querySelector('#sv-tip');
  const picked = new Map((d.skills || []).map((s) => [String(s.skillId), {
    skillId: s.skillId,
    level: s.level || 'senior',
    years: Number(s.years) || 0,
  }]));
  const modes = new Set(d.serviceModes || []);

  const nameOf = new Map();
  panel.querySelectorAll('[data-skill-chips] [data-key]').forEach((b) => {
    const label = b.textContent.trim();
    nameOf.set(b.getAttribute('data-key'), label);
  });

  const rowsBox = panel.querySelector('[data-skill-rows]');
  const chipsBox = panel.querySelector('[data-skill-chips]');
  const modeBox = panel.querySelector('[data-mode-chips]');

  const renderChips = () => {
    chipsBox.querySelectorAll('[data-key]').forEach((b) => {
      const on = picked.has(b.getAttribute('data-key'));
      b.classList.toggle('is-on', on);
      b.setAttribute('aria-pressed', String(on));
    });
  };

  const renderRows = () => {
    rowsBox.innerHTML = [...picked.entries()].map(([key, v]) => `
      <div class="picked-row" data-row="${esc(key)}">
        <span class="picked-name">${esc(nameOf.get(key) || `技能 ${key}`)}</span>
        <select class="select" data-level aria-label="等级">
          ${LEVELS.map((l) => `<option value="${l.key}"${l.key === v.level ? ' selected' : ''}>${l.label}</option>`).join('')}
        </select>
        <input class="input" data-years type="number" min="0" max="60" step="1"
               value="${esc(String(v.years || 0))}" aria-label="从业年限" /><span class="picked-unit">年</span>
        <button type="button" class="icon-btn" data-remove aria-label="移除">${icon('close', 14)}</button>
      </div>`).join('');

    rowsBox.querySelectorAll('[data-row]').forEach((row) => {
      const key = row.getAttribute('data-row');
      const rec = picked.get(key);
      if (!rec) return;
      row.querySelector('[data-level]').addEventListener('change', (e) => { rec.level = e.target.value; });
      row.querySelector('[data-years]').addEventListener('input', (e) => {
        rec.years = Math.max(0, Math.min(60, Number(e.target.value) || 0));
      });
      row.querySelector('[data-remove]').addEventListener('click', () => {
        picked.delete(key);
        renderChips();
        renderRows();
      });
    });
  };

  chipsBox.querySelectorAll('[data-key]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const key = btn.getAttribute('data-key');
      if (picked.has(key)) {
        picked.delete(key);
      } else {
        if (picked.size >= MAX_SKILLS) {
          toastError(`最多选 ${MAX_SKILLS} 项技能`);
          return;
        }
        picked.set(key, { skillId: Number(key), level: 'senior', years: 0 });
      }
      renderChips();
      renderRows();
    });
  });

  modeBox.querySelectorAll('[data-key]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const key = btn.getAttribute('data-key');
      if (modes.has(key)) modes.delete(key);
      else modes.add(key);
      btn.classList.toggle('is-on', modes.has(key));
    });
  });

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const tip = (text, tone = 'muted') => {
      tipEl.textContent = text;
      tipEl.classList.toggle('is-error', tone === 'error');
    };

    if (picked.size < 1) { tip('至少保留一项技能', 'error'); return; }
    if (!modes.size) { tip('至少选一种服务方式', 'error'); return; }

    const hourCents = centsOf(panel.querySelector('#sv-hour').value);
    const taskCents = centsOf(panel.querySelector('#sv-task').value);
    const err = priceProblem(hourCents) || priceProblem(taskCents);
    if (err) { tip(err, 'error'); return; }

    const btn = panel.querySelector('#sv-submit');
    btn.disabled = true;
    btn.textContent = '保存中…';
    try {
      await api.updateExpertServices({
        skills: [...picked.values()],
        serviceModes: [...modes],
        pricePerHourCents: hourCents,
        pricePerTaskCents: taskCents,
      });
      toastOk('服务设置已保存');
      tip('已保存。公开主页上立刻生效。', 'ok');
    } catch (e2) {
      tip(e2 instanceof ApiError ? e2.message : '保存失败，请稍后重试', 'error');
    } finally {
      btn.disabled = false;
      btn.textContent = '保存服务设置';
    }
  });

  renderRows();
}

/* ------------------------------------------------------ 主页内容 -- */

function profilePanelHTML(d) {
  return `<form class="card card-pad" id="pf-form" novalidate>
    <h2 class="block-title">头像</h2>
    <div class="upload-inline" data-avatar-box>
      ${avatarHTML(d.realName || '我', d.avatar, 44)}
      <label class="btn btn-sm">
        上传头像
        <input type="file" accept="image/jpeg,image/png,image/webp,image/gif" hidden data-avatar-file />
      </label>
      <span class="hint">≤ 2MB，JPG / PNG / WebP</span>
    </div>

    <div class="field" style="margin-top:20px">
      <label for="pf-intro">个人简介 <span class="hint" data-intro-count></span></label>
      <textarea class="input" id="pf-intro" rows="6" maxlength="${INTRO_MAX}">${esc(d.intro || '')}</textarea>
    </div>

    <div class="field">
      <label for="pf-exp">工作经历 <span class="hint">选填，一段一行</span></label>
      <textarea class="input" id="pf-exp" rows="4" maxlength="2000">${esc(d.experience || '')}</textarea>
    </div>

    <p class="form-summary" id="pf-tip"></p>
    <div class="form-actions">
      <button class="btn btn-primary" type="submit" id="pf-submit">保存主页内容</button>
      ${d.expertId
        ? `<a class="btn" href="#/experts/${esc(String(d.expertId))}" target="_blank" rel="noopener">查看公开主页</a>`
        : ''}
    </div>
  </form>`;
}

function bindProfilePanel(panel, d) {
  const form = panel.querySelector('#pf-form');
  const introEl = panel.querySelector('#pf-intro');
  const expEl = panel.querySelector('#pf-exp');
  const tipEl = panel.querySelector('#pf-tip');
  const countEl = panel.querySelector('[data-intro-count]');
  const avatarBox = panel.querySelector('[data-avatar-box]');
  let avatar = d.avatar || '';

  const renderCount = () => {
    const n = [...introEl.value.trim()].length;
    countEl.textContent = `${n}/${INTRO_MAX}，至少 ${INTRO_MIN} 字`;
    countEl.classList.toggle('is-error', n > 0 && n < INTRO_MIN);
  };
  introEl.addEventListener('input', renderCount);

  // 上传成功后那块 DOM 要重画（换预览图），所以渲染与接线放在同一个函数里：
  // 分成两个函数时，"重画后忘了重新绑定"就是个必然会被漏掉的 bug。
  const renderAvatarBox = () => {
    avatarBox.innerHTML = `
      ${avatarHTML(d.realName || '我', avatar, 44)}
      <label class="btn btn-sm">上传头像
        <input type="file" accept="image/jpeg,image/png,image/webp,image/gif" hidden data-avatar-file />
      </label>
      <span class="hint">≤ 2MB，JPG / PNG / WebP；换完记得点保存</span>`;

    const input = avatarBox.querySelector('[data-avatar-file]');
    input.addEventListener('change', async () => {
      const file = input.files && input.files[0];
      if (!file) return;
      input.disabled = true;
      try {
        const res = await uploadFile('avatar', file);
        avatar = res.url || '';
        renderAvatarBox();
        toastOk('头像已上传');
      } catch (err) {
        toastError(err instanceof ApiError ? err.message : '头像上传失败');
        input.disabled = false;
      }
    });
  };
  renderAvatarBox();

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const intro = introEl.value.trim();
    const n = [...intro].length;
    if (n < INTRO_MIN || n > INTRO_MAX) {
      tipEl.textContent = `个人简介需要 ${INTRO_MIN}–${INTRO_MAX} 字`;
      tipEl.classList.add('is-error');
      introEl.focus();
      return;
    }

    const btn = panel.querySelector('#pf-submit');
    btn.disabled = true;
    btn.textContent = '保存中…';
    try {
      await api.updateExpertProfile({ intro, experience: expEl.value.trim(), avatar });
      toastOk('主页内容已保存');
      tipEl.classList.remove('is-error');
      tipEl.textContent = '已保存。公开主页上立刻生效。';
    } catch (err) {
      tipEl.classList.add('is-error');
      tipEl.textContent = err instanceof ApiError ? err.message : '保存失败，请稍后重试';
    } finally {
      btn.disabled = false;
      btn.textContent = '保存主页内容';
    }
  });

  renderCount();
}

/* ------------------------------------------------------ 收入钱包 -- */

function walletPanelHTML(w) {
  const items = Array.isArray(w.items) ? w.items : [];
  const withdrawals = Array.isArray(w.withdrawals) ? w.withdrawals : [];

  return `
    <div class="card card-pad" style="margin-bottom:16px">
      <h2 class="block-title">申请提现</h2>
      <p class="apply-hint">
        单次不低于 ${MIN_WITHDRAW_CENTS / 100} 元。同一时间只能有一笔处理中的申请。
        <b>收款账户必须与实名信息一致</b>，否则打款会被退回。
      </p>
      <form id="wd-form" novalidate>
        <div class="form-row">
          <div class="field">
            <label for="wd-amount">提现金额（元）</label>
            <input class="input" id="wd-amount" type="number" min="0" step="1"
                   placeholder="${Math.max(MIN_WITHDRAW_CENTS / 100, Math.floor(Number(w.balanceCents || 0) / 100))}" />
            <span class="hint">可提 ${esc(fmtCents(w.balanceCents))}，处理中 ${esc(fmtCents(w.pendingWithdrawCents))}</span>
          </div>
          <div class="field">
            <label for="wd-type">收款方式</label>
            <select class="select" id="wd-type">
              <option value="alipay">支付宝</option>
              <option value="bank">银行卡</option>
              <option value="wechat">微信</option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label for="wd-name">收款人姓名</label>
            <input class="input" id="wd-name" maxlength="64" autocomplete="name" placeholder="与证件一致" />
          </div>
          <div class="field">
            <label for="wd-no">收款账号</label>
            <input class="input" id="wd-no" maxlength="64" autocomplete="off" placeholder="卡号 / 支付宝账号" />
          </div>
        </div>
        <p class="form-summary" id="wd-tip"></p>
        <div class="form-actions">
          <button class="btn btn-primary" type="submit" id="wd-submit">提交提现申请</button>
        </div>
      </form>
    </div>

    <div class="card card-pad" style="margin-bottom:16px">
      <h2 class="block-title">收入明细</h2>
      ${items.length ? `<div class="table-wrap"><table class="data-table">
        <thead><tr><th>结算单号</th><th>订单</th><th class="num">订单金额</th><th class="num">平台抽佣</th><th class="num">我的收入</th><th>状态</th><th>结算时间</th></tr></thead>
        <tbody>${items.map((it) => `<tr>
          <td class="mono">${esc(it.settlementNo)}</td>
          <td class="mono">${esc(it.orderNo)}</td>
          <td class="num">${esc(fmtCents(it.amountCents))}</td>
          <td class="num muted">${esc(fmtCents(it.commissionCents))}</td>
          <td class="num"><b>${esc(fmtCents(it.expertAmountCents))}</b></td>
          <td>${it.status === 'settled'
            ? '<span class="status-chip is-ok">已结算</span>'
            : it.status === 'failed'
              ? '<span class="status-chip is-danger">结算失败</span>'
              : '<span class="status-chip is-warn">待结算</span>'}</td>
          <td class="muted">${esc(it.settledAt ? fmtDateTime(it.settledAt) : '—')}</td>
        </tr>`).join('')}</tbody>
      </table></div>` : '<p class="muted">还没有收入记录。完成订单并确认后，这里会出现明细。</p>'}
    </div>

    <div class="card card-pad">
      <h2 class="block-title">提现记录</h2>
      ${withdrawals.length ? `<div class="table-wrap"><table class="data-table">
        <thead><tr><th>提现单号</th><th class="num">金额</th><th class="num">实际到账</th><th>收款账号</th><th>状态</th><th>申请时间</th></tr></thead>
        <tbody>${withdrawals.map((it) => {
          const meta = WITHDRAW_STATUS[it.status] || { label: it.status, tone: 'muted' };
          return `<tr>
            <td class="mono">${esc(it.withdrawalNo)}</td>
            <td class="num">${esc(fmtCents(it.amountCents))}</td>
            <td class="num">${esc(fmtCents(it.actualCents))}</td>
            <td class="mono muted">${esc(it.accountName)} · ${esc(it.accountNoMasked)}</td>
            <td><span class="status-chip is-${meta.tone}">${esc(meta.label)}</span></td>
            <td class="muted">${esc(fmtDate(it.createdAt))}</td>
          </tr>`;
        }).join('')}</tbody>
      </table></div>` : '<p class="muted">还没有提现申请。</p>'}
    </div>
  `;
}

function bindWalletPanel(panel, w) {
  const form = panel.querySelector('#wd-form');
  const amountEl = panel.querySelector('#wd-amount');
  const typeEl = panel.querySelector('#wd-type');
  const nameEl = panel.querySelector('#wd-name');
  const noEl = panel.querySelector('#wd-no');
  const tipEl = panel.querySelector('#wd-tip');

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const tip = (text, tone = 'muted') => {
      tipEl.textContent = text;
      tipEl.classList.toggle('is-error', tone === 'error');
    };

    const cents = centsOf(amountEl.value);
    if (cents < MIN_WITHDRAW_CENTS) {
      tip(`单次提现不得低于 ${MIN_WITHDRAW_CENTS / 100} 元`, 'error');
      amountEl.focus();
      return;
    }
    if (cents > Number(w.balanceCents || 0)) {
      tip(`可提现余额只有 ${fmtCents(w.balanceCents)}`, 'error');
      return;
    }
    if ([...nameEl.value.trim()].length < 2) {
      tip('请填写收款人姓名', 'error');
      nameEl.focus();
      return;
    }
    if (noEl.value.trim().length < 6) {
      tip('收款账号至少 6 位', 'error');
      noEl.focus();
      return;
    }

    const btn = panel.querySelector('#wd-submit');
    btn.disabled = true;
    btn.textContent = '提交中…';
    try {
      await api.createWithdrawal({
        amountCents: cents,
        accountType: typeEl.value,
        accountName: nameEl.value.trim(),
        accountNo: noEl.value.trim(),
      });
      toastOk('提现申请已提交');
      window.__reloadPage && window.__reloadPage();
    } catch (err) {
      tip(err instanceof ApiError ? err.message : '提交失败，请稍后重试', 'error');
      btn.disabled = false;
      btn.textContent = '提交提现申请';
    }
  });
}

/* ====================================================== 工具函数 ==== */

function pageHead(title, lede) {
  return `<div class="page-head">
    <h1>${esc(title)}</h1>
    ${lede ? `<p class="lede">${esc(lede)}</p>` : ''}
  </div>`;
}

function loginGuide(title, lede, desc) {
  return {
    html: pageHead(title, lede) + emptyHTML({
      iconName: 'user',
      title: '登录后才能继续',
      desc,
      actionHTML: '<button class="btn btn-primary" type="button" data-act="login">去登录</button>',
    }),
    title,
  };
}

/** 元 → 分。用四舍五入而不是取整：输入 199.5 元不该被悄悄抹成 199。 */
function centsOf(value) {
  const n = Number(String(value == null ? '' : value).trim());
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.round(n * 100);
}

/** 报价合法性。0 表示"不接这种计价方式"，是允许的。 */
function priceProblem(cents) {
  if (cents === 0) return '';
  if (cents < MIN_PRICE_CENTS) return `报价不能低于 ${MIN_PRICE_CENTS / 100} 元，不接的话填 0`;
  if (cents > MAX_PRICE_CENTS) return `报价不能超过 ${MAX_PRICE_CENTS / 100} 元`;
  return '';
}

/**
 * 身份证号校验，与服务端 validateIDCard 同一套规则：
 * 18 位、前 17 位数字、末位数字或 X、ISO 7064:1983 MOD 11-2 校验位。
 * 前端这一份只是即时反馈，权威判定仍在服务端。
 */
function idCardValid(id) {
  const s = String(id || '').trim().toUpperCase();
  if (s.length !== 18) return false;
  if (!/^\d{17}[\dX]$/.test(s)) return false;
  const weights = [7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2];
  const checks = '10X98765432';
  let sum = 0;
  for (let i = 0; i < 17; i++) sum += Number(s[i]) * weights[i];
  return checks[sum % 11] === s[17];
}
