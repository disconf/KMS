/* ============================================================================
   app.js — 应用入口：路由分发、侧边导航、账号区、登录弹窗与全局搜索。
   ========================================================================= */

import {
  api, onAuthChange, currentUser, isLoggedIn,
  loginBySMS, loginByPassword, logout, sendSMSCode, changePassword, refreshProfile, ApiError,
} from './api.js';
import { esc, icon, delegate } from './util.js';
import { bindError, closeModal, emptyHTML, errorHTML, loadingHTML, openModal, toastError, toastOk } from './ui.js';
import { homePage } from './pages-home.js';
import { articleDetailPage, articleListPage } from './pages-article.js';
import { toolBookPage, toolDetailPage, toolListPage } from './pages-tools.js';
import { quizHomePage, quizPracticePage } from './pages-quiz.js';
import { resourceDetailPage, resourceListPage } from './pages-resources.js';
import { searchPage } from './pages-search.js';
import { orderDetailPage, orderListPage } from './pages-orders.js';
import { questionNewPage } from './pages-question.js';
import { hallPage } from './pages-hall.js';
import { contributePage, myContributionsPage } from './pages-contribute.js';
import { qaListPage, qaAskPage, qaDetailPage } from './pages-qa.js';
import { FLAGS } from './flags.js';
import {
  expertListPage, expertDetailPage, expertApplyPage, expertWorkbenchPage,
} from './pages-experts.js';
import {
  adminDashboardPage, adminReviewPage, adminNewsPage,
  adminArticlesPage, adminArticleEditPage,
  adminToolsPage, adminToolGuidesPage, adminToolBooksPage,
  adminResourcesPage, adminResourceLinksPage,
  adminQuizPage, adminExpertsPage, adminWithdrawalsPage, adminOrdersPage, adminUsersPage,
  adminFinancePage, adminSettingsPage, adminQaPage,
} from './pages-admin.js';
import { initSearchBox } from './searchbox.js';

/** 全局共享的少量状态。栏目是导航与各页页头都要用的公共数据。 */
const state = { channels: [] };

// 站点名：ICP 备案登记的网站名称，页内标题统一跟它走。
const SITE_NAME = '个人工作经验分享';

/**
 * 功能开关统一在 flags.js 里，这里只是个别名。
 *
 * service = false 时，顶部导航不渲染「发布悬赏 / 我的订单 / 接单大厅」。
 * 页面与路由都还在（`#/ask`、`#/orders`、`#/hall` 直连可用），只是不摆出入口：
 * 还没有访问量的时候，把三个空货架放在门口比藏起来更劝退。
 * expertPlaza = false 时同理，但专家主页 `#/experts/:id` 保留——
 * 订单页与问答里的专家角标都还在往那里跳。
 */
const FEATURES = FLAGS;

/**
 * 路由表。
 *
 * 用 hash 路由而不是 History API：静态托管下 History 路由需要服务端
 * 把所有未知路径都回落到 index.html，而本服务还要同时提供 API 与文档，
 * 加这么一条通配路由会把 /healthz、/docs 一起吞掉。hash 没有这个代价。
 */
const ROUTES = [
  { re: /^\/?$/,                  page: (ctx) => homePage(ctx) },
  { re: /^\/search$/,             page: (ctx) => searchPage(ctx) },
  { re: /^\/channel\/([^/]+)$/,   page: (ctx, m) => articleListPage(ctx, decodeSegment(m[1])) },
  { re: /^\/article\/([^/]+)$/,   page: (ctx, m) => articleDetailPage(ctx, decodeSegment(m[1])) },
  { re: /^\/tools$/,              page: (ctx) => toolListPage(ctx) },
  { re: /^\/tools\/([^/]+)$/,     page: (ctx, m) => toolDetailPage(ctx, decodeSegment(m[1])) },
  // 教程集详情：/tools/:slug/book/:bookId。
  // slug 放在路径里不是多余的——面包屑与"回到工具页"都要用它，
  // 而工具名在 URL 里可读比纯 ID 好看：/tools/nginx/book/3 一眼知道是什么。
  { re: /^\/tools\/([^/]+)\/book\/([^/]+)$/,
    page: (ctx, m) => toolBookPage(ctx, decodeSegment(m[1]), decodeSegment(m[2])) },
  { re: /^\/quiz$/,               page: (ctx) => quizHomePage(ctx) },
  { re: /^\/quiz\/practice$/,     page: (ctx) => quizPracticePage(ctx) },
  { re: /^\/resources$/,          page: (ctx) => resourceListPage(ctx) },
  { re: /^\/resources\/([^/]+)$/, page: (ctx, m) => resourceDetailPage(ctx, decodeSegment(m[1])) },
  // 投稿与「我的投稿」。两条都是登录后才有意义的页面，
  // 但路由照常注册——未登录时页面自己会渲染登录引导，比 404 友好。
  // 问答广场：列表与详情游客可读（搜索引擎落地页），提问页自己会给登录引导。
  // /qa/ask 必须排在 /qa/:id 之前——问号形式的正则不匹配纯数字 ID，
  // 但顺序保持一致能省掉以后改正则时的一次排查。
  { re: /^\/qa$/,                page: (ctx) => qaListPage(ctx) },
  { re: /^\/qa\/ask$/,            page: (ctx) => qaAskPage(ctx) },
  { re: /^\/qa\/([^/]+)$/,        page: (ctx, m) => qaDetailPage(ctx, decodeSegment(m[1])) },

  { re: /^\/contribute$/,        page: (ctx) => contributePage(ctx) },
  { re: /^\/me\/contributions$/, page: (ctx) => myContributionsPage(ctx) },

  // 专家：广场与详情游客可读，申请与工作台要登录（页面自己判断，未登录给引导）。
  // /expert/apply 必须排在 /expert 之前读起来才顺，正则本身互不匹配。
  { re: /^\/experts$/,           page: (ctx) => expertListPage(ctx) },
  { re: /^\/experts\/([^/]+)$/,  page: (ctx, m) => expertDetailPage(ctx, decodeSegment(m[1])) },
  { re: /^\/expert\/apply$/,     page: (ctx) => expertApplyPage(ctx) },
  { re: /^\/expert$/,            page: (ctx) => expertWorkbenchPage(ctx) },
  // 订单详情必须排在 /orders 之后，但两者正则互不匹配（带 / 与不带），顺序其实无所谓；
  // 保持"列表在前、详情在后"只是为了让这张表读起来跟导航一致。
  { re: /^\/orders$/,             page: (ctx) => orderListPage(ctx) },
  { re: /^\/orders\/([^/]+)$/,    page: (ctx, m) => orderDetailPage(ctx, decodeSegment(m[1])) },
  { re: /^\/ask$/,                page: (ctx) => questionNewPage(ctx) },
  { re: /^\/hall$/,               page: (ctx) => hallPage(ctx) },

  // 后台。这些路由对所有人可访问，页面自己会拦：未登录给登录引导、非管理员给说明。
  // 把路由藏起来只会让人以为是"页面不存在"，排查起来更费劲。
  { re: /^\/admin$/,                          page: (ctx) => adminDashboardPage(ctx) },
  { re: /^\/admin\/review$/,                   page: (ctx) => adminReviewPage(ctx) },
  { re: /^\/admin\/news$/,                     page: (ctx) => adminNewsPage(ctx) },
  { re: /^\/admin\/articles$/,                 page: (ctx) => adminArticlesPage(ctx) },
  { re: /^\/admin\/articles\/([^/]+)$/,        page: (ctx, m) => adminArticleEditPage(ctx, decodeSegment(m[1])) },
  { re: /^\/admin\/tools$/,                    page: (ctx) => adminToolsPage(ctx) },
  { re: /^\/admin\/tools\/([^/]+)\/guides$/,  page: (ctx, m) => adminToolGuidesPage(ctx, decodeSegment(m[1])) },
  // 教程集管理。一条路由两种视图：不带 ?book= 是列表，带了是那本书的编辑视图。
  { re: /^\/admin\/tools\/([^/]+)\/books$/,   page: (ctx, m) => adminToolBooksPage(ctx, decodeSegment(m[1])) },
  { re: /^\/admin\/resources$/,                page: (ctx) => adminResourcesPage(ctx) },
  { re: /^\/admin\/resources\/([^/]+)\/links$/, page: (ctx, m) => adminResourceLinksPage(ctx, decodeSegment(m[1])) },
  { re: /^\/admin\/quiz$/,                     page: (ctx) => adminQuizPage(ctx) },
  { re: /^\/admin\/qa$/,                       page: (ctx) => adminQaPage(ctx) },
  { re: /^\/admin\/experts$/,                  page: (ctx) => adminExpertsPage(ctx) },
  { re: /^\/admin\/withdrawals$/,              page: (ctx) => adminWithdrawalsPage(ctx) },
  { re: /^\/admin\/orders$/,                   page: (ctx) => adminOrdersPage(ctx) },
  { re: /^\/admin\/users$/,                    page: (ctx) => adminUsersPage(ctx) },
  { re: /^\/admin\/finance$/,                  page: (ctx) => adminFinancePage(ctx) },
  { re: /^\/admin\/settings$/,                 page: (ctx) => adminSettingsPage(ctx) },
];

/* ------------------------------------------------------------- 启动 -- */

boot();

async function boot() {
  bindGlobalEvents();
  renderAccount();

  // 页脚是 index.html 里的静态标记，没法像导航那样用模板条件包住——
  // 启动时按开关摘掉对应链接。开关只管入口显隐，路由仍然可用。
  if (!FEATURES.expertPlaza) {
    document.querySelectorAll('.site-foot a[href="#/experts"], .site-foot a[href="#/expert/apply"]')
      .forEach((a) => { a.hidden = true; a.nextElementSibling?.classList.add('hidden'); });
  }
  hideFooterDocLinksIfDisabled();

  // 导航依赖栏目清单，先把它拿到；失败也要继续——页面主体比导航重要
  try {
    state.channels = await api.channels();
  } catch (err) {
    state.channels = [];
  }
  renderNav(currentPath());

  // 本地有令牌时顺手刷一次用户资料，令牌失效会被清掉
  if (isLoggedIn()) {
    await refreshProfile();
  }

  // 「我的投稿」只在登录后出现，所以登录/登出都要重绘导航——
  // 只刷账号区会让这个入口一直停在登录前的形态，直到用户手动切页才出现。
  onAuthChange(() => {
    renderAccount();
    renderNav(currentPath());
  });
  window.addEventListener('hashchange', () => route());
  window.addEventListener('opskb:need-login', openLoginModal);

  // 页面里做了一次写操作（支付、确认、抢单…）之后要重新取一次数据。
  // 直接 location.reload() 会把用户甩回首页并丢掉展开状态，而页面自己
  // import app.js 会形成循环依赖，所以只暴露这一个刷新入口。
  window.__reloadPage = () => route();

  await route();
}

/**
 * 页脚的「API 文档 / OpenAPI」入口跟着服务端开关走。
 *
 * 生产默认不暴露 /docs 与 /openapi.yaml（见 config.ExposeAPIDocs），
 * 但同一个二进制也会跑在测试环境，前端不能写死显隐。这里探一次：
 * 探不到就把两个链接摘掉，别让人点进一个 404。
 */
async function hideFooterDocLinksIfDisabled() {
  let open = false;
  try {
    const res = await fetch('/openapi.yaml', { method: 'HEAD', cache: 'no-store' });
    open = res.ok;
  } catch (_) {
    open = false;
  }
  if (open) return;
  document.querySelectorAll('.site-foot a[data-doc-link]').forEach((a) => {
    a.hidden = true;
    // 只摘紧邻的分隔符；OpenAPI 后面跟的是页脚说明文字，不能一起摘掉
    const sep = a.nextElementSibling;
    if (sep && sep.classList.contains('foot-sep')) sep.classList.add('hidden');
  });
}

function bindGlobalEvents() {
  // 窄屏搜索：顶栏只有一行，搜索常驻会把导航挤成横向滚动条，
  // 所以折叠成一个图标，点开后从顶栏下方整行展开。
  const topbar = document.getElementById('topbar');
  const searchToggle = document.getElementById('search-toggle');
  const searchForm = document.getElementById('search-form');
  const searchInput = document.getElementById('search-input');

  if (searchToggle && topbar) {
    searchToggle.addEventListener('click', () => {
      const open = topbar.classList.toggle('is-search-open');
      searchToggle.setAttribute('aria-expanded', String(open));
      if (open && searchInput) setTimeout(() => searchInput.focus(), 30);
    });

    // 点面板外面收起来。用 closest 判断而不是给 document 加遮罩：
    // 遮罩会盖住正文，让人以为进了新页面。
    document.addEventListener('click', (e) => {
      if (!topbar.classList.contains('is-search-open')) return;
      if (e.target.closest('.search') || e.target.closest('#search-toggle')) return;
      closeSearchPanel();
    });

    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') closeSearchPanel();
    });

    // 提交后收起：用户已经在搜索页上了，面板继续挡着没有意义
    if (searchForm) searchForm.addEventListener('submit', () => closeSearchPanel());
  }

  // 「更多」下拉：同样靠"点外面收起"，不用遮罩。
  document.addEventListener('click', (e) => {
    if (e.target.closest('.nav-more')) return;
    closeNavMore();
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') closeNavMore();
  });

  // 窗口宽度变了要重新量一次。防抖 160ms：拖拽窗口时会连续触发，
  // 每帧重排导航既浪费又会闪。改宽度后重绘导航而不是只重排——
  // 重排要维护"哪些项被移走"的状态，重绘直接从数据重建，没有半新半旧的状态。
  let navResizeTimer = 0;
  window.addEventListener('resize', () => {
    clearTimeout(navResizeTimer);
    navResizeTimer = setTimeout(() => renderNav(currentPath()), 160);
  });

  // 搜索框：回车跳搜索页，输入时给检索词建议。
  // 细节都在 searchbox.js 里——它要处理防抖、过期响应与键盘导航，
  // 混在这个已经负责路由与账号的文件里会很快失控。
  initSearchBox();

  // 顶部账号区
  delegate(document.getElementById('account') || document.body, '[data-act]', (node) => {
    const act = node.getAttribute('data-act');
    if (act === 'login') openLoginModal();
    if (act === 'logout') doLogout();
    if (act === 'password') openPasswordModal();
    if (act === 'home') location.hash = '#/';
  });

  // 空态里的"去登录"按钮
  document.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-act="login"]');
    if (btn && !btn.closest('#account')) {
      openLoginModal();
    }
  });

  // 顶部导航是横向滚动的，点完一项顺手把搜索面板收掉
  delegate(document.getElementById('nav') || document.body, '.nav-item', () => {
    closeSearchPanel();
  });
}

function closeSearchPanel() {
  const topbar = document.getElementById('topbar');
  const toggle = document.getElementById('search-toggle');
  if (!topbar || !topbar.classList.contains('is-search-open')) return;
  topbar.classList.remove('is-search-open');
  if (toggle) toggle.setAttribute('aria-expanded', 'false');
}

/* ------------------------------------------------------------- 路由 -- */

function parseHash() {
  const raw = location.hash.slice(1) || '/';
  const [path, search] = raw.split('?');
  return {
    path: path || '/',
    params: new URLSearchParams(search || ''),
  };
}

const currentPath = () => parseHash().path;

function decodeSegment(segment) {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}

/** 每次路由递增，用于丢弃"用户已经切走之后才回来的"旧请求结果。 */
let routingToken = 0;

async function route() {
  const token = ++routingToken;
  const view = document.getElementById('view');
  const { path, params } = parseHash();

  closeSearchPanel();

  const matched = ROUTES
    .map((r) => ({ route: r, m: r.re.exec(path) }))
    .find((x) => x.m);

  renderNav(path);

  if (!matched) {
    view.innerHTML = errorHTML(
      Object.assign(new Error(`页面不存在：${path}`), { code: 404 }),
      { retry: false }
    );
    bindError(view, { onLogin: openLoginModal });
    document.title = `${SITE_NAME} · 页面不存在`;
    return;
  }

  view.innerHTML = loadingHTML();

  try {
    const ctx = { path, params, state };
    const page = await matched.route.page(ctx, matched.m);
    if (token !== routingToken) return;

    view.innerHTML = page.html;
    if (page.mount) page.mount(view);

    document.title = page.title ? `${page.title} · ${SITE_NAME}` : SITE_NAME;
    window.scrollTo({ top: 0 });
  } catch (err) {
    if (token !== routingToken) return;
    view.innerHTML = errorHTML(err);
    bindError(view, { onRetry: () => route(), onLogin: openLoginModal });
  }
}

/* ------------------------------------------------------------- 导航 -- */

function renderNav(path) {
  const nav = document.getElementById('nav');
  if (!nav) return;

  const activeSlug = activeChannelOf(path);

  // 横向导航里不放图标：62px 高的条，图标只会让每个标签看起来像按钮，
  // 反而降低"这是一排页面入口"的识别度。栏目多到放不下时才考虑分组。
  const items = [
    navItemHTML({ name: '首页', href: '#/', active: path === '/' || path === '' }),
    ...state.channels.map((ch) => navItemHTML({
      name: ch.name,
      href: navHrefOf(ch),
      count: ch.count,
      active: ch.slug === activeSlug,
    })),
  ];

  // 问答广场放在栏目之后、专家之前：它是内容入口，与五个栏目同类，
  // 排在「专家广场」这种角色入口前面更符合"先看内容再找人"的顺序。
  items.push(navItemHTML({
    name: '问答广场',
    href: '#/qa',
    active: path === '/qa' || path.startsWith('/qa/'),
  }));

  // 专家广场暂时收起入口（FLAGS.expertPlaza）。路由与详情页仍在：
  // 订单页、问答里的专家角标都还在往 #/experts/:id 跳，删路由会直接 404。
  if (FEATURES.expertPlaza) {
    items.push(navItemHTML({
      name: '专家广场',
      href: '#/experts',
      active: path.startsWith('/expert'),
    }));
  }

  // 投稿是内容入口，不做成「只有登录才显示」的按钮：
  // 未登录时点进去会看到登录引导，这比藏起来更能说明平台接受外部投稿。
  items.push(navItemHTML({ name: '投稿', href: '#/contribute', active: path === '/contribute' }));
  if (isLoggedIn()) {
    items.push(navItemHTML({
      name: '我的投稿',
      href: '#/me/contributions',
      active: path === '/me/contributions',
    }));
  }

  // 服务入口：功能代码与路由都在，只是不摆出来（见 FEATURES 的注释）。
  if (FEATURES.service) {
    items.push(
      navItemHTML({ name: '发布悬赏', href: '#/ask', active: path === '/ask' }),
      navItemHTML({ name: '我的订单', href: '#/orders', active: path === '/orders' || path.startsWith('/orders/') }),
      navItemHTML({ name: '接单大厅', href: '#/hall', active: path === '/hall' }),
    );
  }

  nav.innerHTML = items.join('');
  layoutNav(nav);
  keepActiveNavVisible(nav);
}

/* ------------------------------------------------------- 顶栏收纳 -- */

/**
 * 按实测宽度决定「这一屏能放几个栏目」。
 *
 * 为什么不用纯 CSS 断点：栏目数量是运营在后台加的，断点写死 6 项、第 7 项上线就溢出；
 * 而浏览器缩放、侧边栏外的窗口拖拽、不同字体的字宽差异，断点全都覆盖不到。
 * 这里量一次实际像素，宁可多算几毫秒，也不让导航在某种屏宽下挤成一团。
 *
 * 收缩顺序有意为之：先摘计数（辅助信息）→ 再收进「更多」（入口本身）。
 * 反过来会让用户先丢掉入口、却还留着数字。
 */
function layoutNav(nav) {
  resetNavOverflow(nav);

  // 移动端的导航是第二行整排横滚，手指横滑比下拉更顺手，不参与收纳。
  if (window.matchMedia('(max-width: 700px)').matches) return;

  const items = Array.from(nav.querySelectorAll('.nav-item'));
  if (items.length < 3) return;

  const avail = navAvailableWidth(nav);
  if (avail <= 0) return;

  // ① 放得下就到此为止，一个像素都不动
  if (navItemsWidth(items) <= avail) return;

  // ② 先摘计数
  nav.classList.add('is-compact');
  if (navItemsWidth(items) <= avail) return;

  // ③ 还不够就把尾部收进「更多」
  const MORE_W = 86; // 「更多」按钮 + 箭头 + 左右内边距
  let acc = 0;
  let keep = 0;
  for (const el of items) {
    if (acc + el.offsetWidth + MORE_W > avail) break;
    acc += el.offsetWidth;
    keep += 1;
  }
  keep = Math.max(2, keep); // 至少留下首页 + 第一个栏目
  if (keep >= items.length) return;

  const overflow = items.slice(keep);
  overflow.forEach((el) => { el.hidden = true; });

  const activeInside = overflow.some((el) => el.classList.contains('is-active'));
  const more = document.createElement('div');
  more.className = `nav-more is-on${activeInside ? ' is-active' : ''}`;
  more.innerHTML = `
    <button class="nav-more-btn" type="button" aria-expanded="false" aria-haspopup="true"
            aria-label="更多栏目">更多${icon('chevron', 14)}</button>
    <div class="nav-more-panel" hidden>
      ${overflow.map((el) => `
        <a class="nav-more-link${el.classList.contains('is-active') ? ' is-active' : ''}"
           href="${esc(el.getAttribute('href') || '#/')}">${el.innerHTML}</a>`).join('')}
    </div>`;

  const btn = more.querySelector('.nav-more-btn');
  const panel = more.querySelector('.nav-more-panel');
  btn.addEventListener('click', (e) => {
    e.stopPropagation();
    const willOpen = panel.hidden;
    panel.hidden = !willOpen;
    btn.setAttribute('aria-expanded', String(willOpen));
  });
  // 点条目就跳走了，面板要跟着收起，否则回到上一页时它还悬在那儿
  panel.addEventListener('click', () => closeNavMore());

  nav.appendChild(more);
  nav.classList.add('is-overflow');
}

/** 把上一轮的收纳状态清干净。不清的话，窗口拖宽之后被移走的栏目再也回不来。 */
function resetNavOverflow(nav) {
  nav.classList.remove('is-compact', 'is-overflow');
  const more = nav.querySelector('.nav-more');
  if (more) more.remove();
  nav.querySelectorAll('.nav-item[hidden]').forEach((el) => { el.hidden = false; });
}

/** 导航可用宽度 = 顶栏内容宽 − 品牌 − 右侧操作区 − 导航左外边距。 */
function navAvailableWidth(nav) {
  const inner = nav.parentElement;
  if (!inner) return 0;
  const cs = getComputedStyle(inner);
  const pad = (parseFloat(cs.paddingLeft) || 0) + (parseFloat(cs.paddingRight) || 0);
  const brand = inner.querySelector('.brand');
  const right = inner.querySelector('.topbar-right');
  const navMargin = parseFloat(getComputedStyle(nav).marginLeft) || 0;
  return inner.clientWidth - pad
    - (brand ? brand.offsetWidth : 0)
    - (right ? right.offsetWidth : 0)
    - navMargin;
}

function navItemsWidth(items) {
  return items.reduce((sum, el) => sum + el.offsetWidth, 0);
}

function closeNavMore() {
  document.querySelectorAll('.nav-more-panel:not([hidden])').forEach((panel) => {
    panel.hidden = true;
    panel.parentElement?.querySelector('.nav-more-btn')?.setAttribute('aria-expanded', 'false');
  });
}

function navItemHTML({ name, href, count, active }) {
  return `<a class="nav-item ${active ? 'is-active' : ''}" href="${esc(href)}"${
    active ? ' aria-current="page"' : ''
  }>
    <span class="nav-label">${esc(name)}</span>
    ${count !== undefined && count !== null ? `<span class="nav-count">${count}</span>` : ''}
  </a>`;
}

/**
 * 窄屏导航是横向滚动的。切到靠右的栏目时，活动项可能在可视区外，
 * 用户会看不到"我在哪"——所以把它滚到中间。
 * 用 rect 差值而不是 offsetLeft：offsetLeft 依赖 offsetParent，
 * 顶栏一旦加了 position 就会算错。
 */
function keepActiveNavVisible(nav) {
  const el = nav.querySelector('.nav-item.is-active');
  if (!el || nav.scrollWidth <= nav.clientWidth + 1) return;
  const navRect = nav.getBoundingClientRect();
  const elRect = el.getBoundingClientRect();
  const delta = (elRect.left - navRect.left) - (navRect.width - elRect.width) / 2;
  nav.scrollLeft += delta;
}

/** 每种内容模型的入口路径。栏目注册表里给的 path 是语义路径，这里落到实际路由。 */
function navHrefOf(ch) {
  switch (ch.kind) {
    case 'tool':     return '#/tools';
    case 'quiz':     return '#/quiz';
    case 'resource': return '#/resources';
    default:         return `#/channel/${encodeURIComponent(ch.slug)}`;
  }
}

/** 当前路径属于哪个栏目，用于点亮导航。 */
function activeChannelOf(path) {
  if (path.startsWith('/tools')) return 'open-source-tools';
  if (path.startsWith('/quiz')) return 'ops-quiz';
  if (path.startsWith('/resources')) return 'resources';

  const m = /^\/channel\/([^/]+)/.exec(path);
  if (m) return decodeSegment(m[1]);

  return null;
}

/* ------------------------------------------------------------- 账号 -- */

function renderAccount() {
  const box = document.getElementById('account');
  if (!box) return;
  const user = currentUser();

  if (!user) {
    box.innerHTML = `<button class="btn btn-primary btn-sm" type="button" data-act="login">
      ${icon('user', 14)}登录
    </button>`;
    return;
  }

  const name = user.nickname || user.phone || '已登录';
  // 后台入口只摆在账号区，不占读者导航：顶栏那一排是"看什么内容"，
  // 后台是另一套信息架构，混进去会让普通读者也看见"审核队列"。
  const adminEntry = user.role === 'admin'
    ? `<a class="btn btn-sm admin-entry" href="#/admin" title="运营后台">${icon('shield', 14)}后台</a>`
    : '';
  // 改密入口只给管理员：普通用户没有密码，专家也走验证码登录。
  const pwdEntry = user.role === 'admin'
    ? `<button class="icon-btn" type="button" data-act="password"
         title="修改登录密码" aria-label="修改登录密码">${icon('key', 16)}</button>`
    : '';
  // 工作台同理，只对已是专家的人显示。还不是专家的人不需要一个
  // 点进去就是「请先申请」的按钮——入口在专家广场的空态和页脚。
  const expertEntry = user.role === 'expert'
    ? `<a class="btn btn-sm admin-entry" href="#/expert" title="专家工作台">${icon('activity', 14)}工作台</a>`
    : '';
  box.innerHTML = `
    <div class="user-chip" title="${esc(user.phone || '')}">
      <span class="avatar">${esc(String(name)[0].toUpperCase())}</span>
      <span class="name">${esc(name)}</span>
    </div>
    ${expertEntry}
    ${adminEntry}
    ${pwdEntry}
    <button class="icon-btn" type="button" data-act="logout" title="退出登录" aria-label="退出登录">
      ${icon('logout', 16)}
    </button>`;
}

async function doLogout() {
  await logout();
  toastOk('已退出登录');
  // 需要登录的页面（错题本、练习）要立刻回到未登录态
  route();
}

/* --------------------------------------------------------- 登录弹窗 -- */

function openLoginModal() {
  if (isLoggedIn()) return;

  openModal(`
    <div class="modal-head">
      <div>
        <h2>登录</h2>
        <p id="login-sub">手机号验证码登录，首次登录会自动创建账号。</p>
      </div>
    </div>
    <div class="modal-body">
      <div class="field">
        <label for="login-phone">手机号</label>
        <input class="input" id="login-phone" type="tel" inputmode="numeric"
               maxlength="11" placeholder="11 位手机号" autocomplete="tel" />
      </div>
      <div class="field" id="login-code-field">
        <label for="login-code">验证码</label>
        <div style="display:flex;gap:8px">
          <input class="input" id="login-code" type="text" inputmode="numeric"
                 maxlength="6" placeholder="6 位数字" autocomplete="one-time-code" />
          <button class="btn" id="login-send" type="button" style="flex:0 0 auto;height:38px;min-width:104px">
            获取验证码
          </button>
        </div>
      </div>
      <div class="field" id="login-password-field" hidden>
        <label for="login-password">密码</label>
        <input class="input" id="login-password" type="password" maxlength="64"
               placeholder="登录密码" autocomplete="current-password" />
      </div>
      <p id="login-tip" style="font-size:12.5px;color:var(--text-3);min-height:19px;line-height:1.5"></p>
      <button class="btn btn-primary btn-block btn-lg" id="login-submit" type="button">登录</button>
      <p style="text-align:center;margin:14px 0 0">
        <button type="button" id="login-switch"
          style="background:none;border:0;padding:0;cursor:pointer;font-size:13px;color:var(--brand)">
          管理员密码登录
        </button>
      </p>
    </div>
  `, (modal) => {
    const phoneInput = modal.querySelector('#login-phone');
    const codeInput = modal.querySelector('#login-code');
    const sendBtn = modal.querySelector('#login-send');
    const submitBtn = modal.querySelector('#login-submit');
    const tip = modal.querySelector('#login-tip');
    const sub = modal.querySelector('#login-sub');
    const codeField = modal.querySelector('#login-code-field');
    const pwdField = modal.querySelector('#login-password-field');
    const pwdInput = modal.querySelector('#login-password');
    const switchBtn = modal.querySelector('#login-switch');

    // 两种模式共用同一个弹窗：验证码是默认路径（普通用户），
    // 密码是给已设密码的管理员用的，切换时只换字段不重开弹窗，
    // 已输入的手机号不会丢。
    let mode = 'sms';

    const applyMode = (next) => {
      mode = next;
      const byPwd = mode === 'password';
      codeField.hidden = byPwd;
      pwdField.hidden = !byPwd;
      sub.textContent = byPwd
        ? '使用手机号与密码登录，仅限已设置密码的后台账号。'
        : '手机号验证码登录，首次登录会自动创建账号。';
      switchBtn.textContent = byPwd ? '改用验证码登录' : '管理员密码登录';
      setTip('');
      if (byPwd) pwdInput.focus(); else codeInput.focus();
    };

    const setTip = (text, tone = 'muted') => {
      tip.textContent = text;
      tip.style.color = tone === 'error' ? 'var(--danger)'
        : tone === 'ok' ? 'var(--ok)'
        : 'var(--text-3)';
    };

    let countdown = 0;
    let timer = 0;
    const startCountdown = (seconds) => {
      countdown = seconds;
      clearInterval(timer);
      const tick = () => {
        sendBtn.textContent = `${countdown} 秒后重发`;
        sendBtn.disabled = countdown > 0;
        if (countdown <= 0) {
          clearInterval(timer);
          sendBtn.textContent = '获取验证码';
        }
        countdown -= 1;
      };
      tick();
      timer = setInterval(tick, 1000);
    };

    const validPhone = () => /^1[3-9]\d{9}$/.test(phoneInput.value.trim());

    sendBtn.addEventListener('click', async () => {
      const phone = phoneInput.value.trim();
      if (!validPhone()) {
        setTip('请输入正确的 11 位手机号', 'error');
        phoneInput.focus();
        return;
      }
      sendBtn.disabled = true;
      setTip('正在发送…');
      try {
        const res = await sendSMSCode(phone);
        if (res && res.devCode) {
          // 开发环境服务端会把验证码回传，直接填好省一步
          codeInput.value = res.devCode;
          setTip(`开发模式：验证码 ${res.devCode} 已自动填入`, 'ok');
          codeInput.focus();
        } else {
          setTip(`已发送，${res && res.expiresIn ? Math.round(res.expiresIn / 60) + ' 分钟内有效' : '请查看短信'}`);
        }
        startCountdown((res && res.interval) || 60);
      } catch (err) {
        sendBtn.disabled = false;
        setTip(err instanceof ApiError ? err.message : '发送失败，请稍后再试', 'error');
      }
    });

    const submit = async () => {
      const phone = phoneInput.value.trim();
      if (!validPhone()) { setTip('请输入正确的 11 位手机号', 'error'); phoneInput.focus(); return; }

      if (mode === 'password') {
        const password = pwdInput.value;
        if (!password) { setTip('请输入登录密码', 'error'); pwdInput.focus(); return; }
        submitBtn.disabled = true;
        submitBtn.textContent = '登录中…';
        try {
          await loginByPassword(phone, password);
          closeModal();
          toastOk('登录成功');
          await reloadChannels();
          await route();
        } catch (err) {
          submitBtn.disabled = false;
          submitBtn.textContent = '登录';
          setTip(err instanceof ApiError ? err.message : '登录失败，请稍后再试', 'error');
          pwdInput.focus();
        }
        return;
      }

      const code = codeInput.value.trim();
      if (!/^\d{4,6}$/.test(code)) { setTip('请输入收到的验证码', 'error'); codeInput.focus(); return; }

      submitBtn.disabled = true;
      submitBtn.textContent = '登录中…';
      try {
        await loginBySMS(phone, code);
        clearInterval(timer);
        closeModal();
        toastOk('登录成功');
        await reloadChannels();
        await route();
      } catch (err) {
        submitBtn.disabled = false;
        submitBtn.textContent = '登录';
        setTip(err instanceof ApiError ? err.message : '登录失败，请稍后再试', 'error');
      }
    };

    submitBtn.addEventListener('click', submit);
    switchBtn.addEventListener('click', () => applyMode(mode === 'sms' ? 'password' : 'sms'));
    codeInput.addEventListener('keydown', (e) => { if (e.key === 'Enter') submit(); });
    pwdInput.addEventListener('keydown', (e) => { if (e.key === 'Enter') submit(); });
    phoneInput.addEventListener('keydown', (e) => {
      if (e.key !== 'Enter') return;
      if (mode === 'password') submit(); else sendBtn.click();
    });
  });
}

/* --------------------------------------------------------- 修改密码 -- */

function openPasswordModal() {
  openModal(`
    <div class="modal-head">
      <div>
        <h2>修改登录密码</h2>
        <p>至少 8 位，且同时包含字母和数字。</p>
      </div>
    </div>
    <div class="modal-body">
      <div class="field">
        <label for="pw-old">原密码</label>
        <input class="input" id="pw-old" type="password" maxlength="64" autocomplete="current-password" />
      </div>
      <div class="field">
        <label for="pw-new">新密码</label>
        <input class="input" id="pw-new" type="password" maxlength="64" autocomplete="new-password" />
      </div>
      <div class="field">
        <label for="pw-new2">确认新密码</label>
        <input class="input" id="pw-new2" type="password" maxlength="64" autocomplete="new-password" />
      </div>
      <p id="pw-tip" style="font-size:12.5px;color:var(--text-3);min-height:19px;line-height:1.5"></p>
      <button class="btn btn-primary btn-block btn-lg" id="pw-submit" type="button">保存</button>
    </div>
  `, (modal) => {
    const oldInput = modal.querySelector('#pw-old');
    const newInput = modal.querySelector('#pw-new');
    const againInput = modal.querySelector('#pw-new2');
    const submitBtn = modal.querySelector('#pw-submit');
    const tip = modal.querySelector('#pw-tip');

    const setTip = (text, tone = 'muted') => {
      tip.textContent = text;
      tip.style.color = tone === 'error' ? 'var(--danger)'
        : tone === 'ok' ? 'var(--ok)'
        : 'var(--text-3)';
    };

    const submit = async () => {
      if (!oldInput.value) { setTip('请输入原密码', 'error'); oldInput.focus(); return; }
      if (newInput.value !== againInput.value) {
        setTip('两次输入的新密码不一致', 'error'); againInput.focus(); return;
      }
      submitBtn.disabled = true;
      submitBtn.textContent = '保存中…';
      try {
        await changePassword(oldInput.value, newInput.value);
        closeModal();
        toastOk('密码已更新');
      } catch (err) {
        submitBtn.disabled = false;
        submitBtn.textContent = '保存';
        setTip(err instanceof ApiError ? err.message : '保存失败，请稍后再试', 'error');
      }
    };

    submitBtn.addEventListener('click', submit);
    [oldInput, newInput, againInput].forEach((el) => {
      el.addEventListener('keydown', (e) => { if (e.key === 'Enter') submit(); });
    });
    oldInput.focus();
  });
}

/** 登录后角标数量会变，重新拉一次栏目。 */async function reloadChannels() {
  try {
    state.channels = await api.channels();
    renderNav(currentPath());
  } catch {
    // 拿不到就沿用旧数据，不值得为角标打扰用户
  }
}
