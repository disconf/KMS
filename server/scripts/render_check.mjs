/* ============================================================================
   渲染逻辑自检（需要服务已在跑，且库里要有种子数据）。
   用真实后端数据驱动各页面的渲染函数，验证：
     ① 不抛异常    ② 输出含预期结构    ③ 用户内容被正确转义
   这比截图更能发现问题——字段名对不上、转义漏了，肉眼看不出来，这里会直接红。

   用法：
     go run ./cmd/api &                      # 或 WEB_DIR=./web/static go run ./cmd/api
     node scripts/render_check.mjs
     node scripts/depcheck.js web/static/assets/js
   ========================================================================= */

import { pathToFileURL } from 'node:url';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// 从脚本自身位置推导，不写死绝对路径——否则换台机器就得先改脚本。
const HERE = path.dirname(fileURLToPath(import.meta.url));
const JS_DIR = path.join(HERE, '..', 'web', 'static', 'assets', 'js');
const API = process.env.API_BASE || 'http://localhost:8080';

/* ---------------------------------------------------------- 环境桩 -- */

const realFetch = globalThis.fetch;
globalThis.fetch = (url, opts) =>
  realFetch(typeof url === 'string' && url.startsWith('/') ? API + url : url, opts);

globalThis.localStorage = {
  _v: {},
  getItem(k) { return Object.prototype.hasOwnProperty.call(this._v, k) ? this._v[k] : null; },
  setItem(k, v) { this._v[k] = String(v); },
  removeItem(k) { delete this._v[k]; },
};

globalThis.document = {
  getElementById: () => null,
  addEventListener() {},
  createElement: () => ({ style: {}, click() {}, remove() {}, appendChild() {} }),
  body: { appendChild() {} },
};

globalThis.window = { addEventListener() {}, dispatchEvent() {}, scrollTo() {} };
globalThis.location = { hash: '#/', origin: API };

const load = (f) => import(pathToFileURL(path.join(JS_DIR, f)).href);

/* ------------------------------------------------------------ 断言 -- */

const rows = [];
const check = (name, ok, detail = '') => rows.push({ name, ok: Boolean(ok), detail });

const apiMod = await load('api.js');
const { api } = apiMod;
const util = await load('util.js');
const cards = await load('cards.js');
const home = await load('pages-home.js');
const article = await load('pages-article.js');
const tools = await load('pages-tools.js');
const quiz = await load('pages-quiz.js');
const resources = await load('pages-resources.js');
const search = await load('pages-search.js');
const ordersMod = await load('orders.js');
const orderPages = await load('pages-orders.js');
const questionPages = await load('pages-question.js');
const hallPages = await load('pages-hall.js');

const channels = await api.channels();
const state = { channels };
const ctx = (query = '') => ({ path: '/', params: new URLSearchParams(query), state });

/* ---------------------------------------------------- 入口页自检 -- */

// 这一组守的是"页面引用的文件真的存在"。曾经 index.html 指向 /assets/app.js，
// 而实际文件在 /assets/js/app.js，浏览器里整站只显示一个转圈图标——
// 而所有模块级自检都直接 import 源文件，完全发现不了这个问题。
const indexRes = await fetch(`${API}/`);
const indexHtml = indexRes.ok ? await indexRes.text() : '';
const scriptSrc = (/<script[^>]+src="([^"]+)"/.exec(indexHtml) || [])[1] || '';
const cssHref = (/<link[^>]+href="(\/assets\/[^"]+)"/.exec(indexHtml) || [])[1] || '';
const scriptOk = scriptSrc ? (await fetch(API + scriptSrc)).ok : false;
const cssOk = cssHref ? (await fetch(API + cssHref)).ok : false;

check('入口页：可访问且含挂载点', indexRes.ok && indexHtml.includes('id="view"'));
check('入口页：入口脚本真实存在', scriptOk, scriptSrc || '未找到 script 标签');
check('入口页：样式文件真实存在', cssOk, cssHref || '未找到 stylesheet');

// [hidden] 必须有兜底规则：UA 的 [hidden]{display:none} 打不过任何类选择器的 display，
// 教程行就因此两边同时显示过。这条只能查样式表，渲染函数里看不出来。
const cssText = cssHref ? await (await fetch(API + cssHref)).text() : '';
check('样式表：[hidden] 有强制隐藏规则', /\[hidden\]\s*\{[^}]*display:\s*none\s*!important/.test(cssText));

/* ------------------------------------------------ 未登录态（最前） -- */

// 必须放在本脚本任何登录动作之前。
// api.js 把令牌缓存在模块级变量里，登录之后清 localStorage 也不会变回未登录，
// 所以这一组一旦挪到后面，就只能测到登录态的 HTML，断言会稳定地红。
const contributeMod = await load('pages-contribute.js');
const contributeAnon = await renderPage('投稿页（未登录）', () => contributeMod.contributePage(ctx()));
check('投稿页：未登录给登录引导', contributeAnon.includes('登录后才能投稿'));
check('投稿页：未登录不渲染表单', !contributeAnon.includes('id="c-form"'));

const mineAnon = await renderPage('我的投稿（未登录）', () => contributeMod.myContributionsPage(ctx()));
check('我的投稿：未登录给登录引导', mineAnon.includes('登录后才能查看投稿'));

const meMod = await load('pages-me.js');
const meAnon = await renderPage('我的（未登录）', () => meMod.mePage(ctx()));
check('我的：未登录给登录引导', meAnon.includes('请先登录'));

const adminAnonMod = await load('pages-admin.js');
const adminAnonHtml = await renderPage('后台（未登录）', () => adminAnonMod.adminDashboardPage(ctx()));
check('后台：未登录给登录引导', adminAnonHtml.includes('请先登录运营账号'));

/* ------------------------------------------------------ 专家（公开） -- */

const expertMod = await load('pages-experts.js');

const expertListHtml = await renderPage('专家广场', () => expertMod.expertListPage(ctx()));
check('专家广场：筛选条完整（搜索/方向/接单/排序）',
  expertListHtml.includes('id="ex-keyword"') &&
  expertListHtml.includes('data-filter="skillId"') &&
  expertListHtml.includes('data-online') &&
  expertListHtml.includes('data-sort="rating"'));

// 卡片与空态必须二选一：两者同时出现说明模板里两条分支都拼进了输出，
// 两者都没有说明渲染中途断了（页面看上去只是"没内容"）。
const hasExpertCards = expertListHtml.includes('class="card expert-card"');
const hasExpertEmpty = expertListHtml.includes('首批专家正在认证中') || expertListHtml.includes('这些条件下暂时没有专家');
check('专家广场：卡片与空态互斥', hasExpertCards !== hasExpertEmpty,
  `cards=${hasExpertCards} empty=${hasExpertEmpty}`);
check('专家广场：空态是招募位而不是一片空白',
  hasExpertCards || expertListHtml.includes('申请成为专家'));

const expertGhostHtml = await renderPage('专家详情（不存在）', () => expertMod.expertDetailPage(ctx(), '99999999'));
check('专家详情：不存在的专家给出返回入口', expertGhostHtml.includes('去专家广场'));

// 专家广场收起入口后，列表页直链仍要能打开，并明确告知"整理中"——
// 收起不等于下线，直链进来的用户（收藏、订单页跳转）不该看到一张白页。
check('专家广场：入口收起后直链有提示条', expertListHtml.includes('专家广场正在整理中'));

/* ------------------------------------------------------ 问答（公开） -- */

const qaMod = await load('pages-qa.js');

// 匿名态必须放在登录之前：问答列表与详情都是游客可读的搜索落地页，
// 这里守的是"游客看到的是完整内容而不是登录墙"。
const qaAnonAsk = await renderPage('问答提问页（未登录）', () => qaMod.qaAskPage(ctx()));
check('问答提问页：未登录给登录引导', qaAnonAsk.includes('登录后才能提问'));
check('问答提问页：未登录不渲染表单', !qaAnonAsk.includes('id="qa-form"'));

const qaPageData = await api.qaQuestions({ page: 1, pageSize: 20 });
const qaListData = qaPageData.list || [];
const qaListHtml = await renderPage('问答列表', () => qaMod.qaListPage(ctx('page=1&pageSize=5')));
check('问答列表：hero 与统计齐备',
  qaListHtml.includes('问答广场') && qaListHtml.includes('待解答') && qaListHtml.includes('已解决'));
check('问答列表：三个筛选 tab', (qaListHtml.match(/class="qa-tab /g) || qaListHtml.match(/class="qa-tab"/g) || []).length >= 3,
  String((qaListHtml.match(/qa-tab/g) || []).length));
check('问答列表：主操作是「我要提问」', qaListHtml.includes('我要提问'));
check('问答列表：有搜索框', qaListHtml.includes('data-qa-search'));

// 依赖数据的断言不能靠抽签（硬约束：断言总数每次必须一样）。
// 详情页直接取第一条：列表按 last_answer_at 排序，排第一的可能是刚发布、
// 零回答的新帖（退回 created_at），所以这里显式挑一条 answerCount > 0 的。
// 挑不到不算"少跑几条"，而是让下面几条照跑并红着——总数稳定，
// 失败信息直接指向"库里没有带回答的问题"，比总数悄悄变成 239 好查得多。
const qaTarget = qaListData.find((q) => q.answerCount > 0) || qaListData[0];
const qaDetailHtml = qaTarget
  ? await renderPage('问答详情', () => qaMod.qaDetailPage(ctx(), String(qaTarget.id)))
  : '';
const qaMiss = qaTarget ? '' : '库里没有任何问题';
check('问答详情：问题标题与正文都渲染',
  Boolean(qaTarget) && qaDetailHtml.includes(qaTarget.title) && qaDetailHtml.includes('qa-q-body'), qaMiss);
check('问答详情：游客视角给登录引导而非隐藏输入框',
  Boolean(qaTarget) && qaDetailHtml.includes('登录后可以回答'), qaMiss);
check('问答详情：游客没有采纳按钮', Boolean(qaTarget) && !qaDetailHtml.includes('data-accept'), qaMiss);
check('问答详情：侧栏说明积分规则',
  Boolean(qaTarget) && qaDetailHtml.includes('贡献分') && qaDetailHtml.includes('点赞'), qaMiss);
check('问答详情：已发布回答带徽章', Boolean(qaTarget) && qaDetailHtml.includes('qa-badge'),
  qaTarget ? `answerCount=${qaTarget.answerCount}` : qaMiss);

// 评价区。渲染真实专家主页 + 评价区两个纯函数：
// 汇总、分布、匿名脱敏、专家回复是这一页新引入的四类结构，逐个验。
const expertDetailHtml = await renderPage('专家详情（含评价）', () => expertMod.expertDetailPage(ctx(), '1'));
check('专家详情：含「服务评价」区块', expertDetailHtml.includes('服务评价'));
// 人才名片（岗位对接一期）：详情接口带 contribution 块时渲染贡献条。
// 有没有数据都该有这个条（种子专家通常有积分与回答），字段名照实断。
check('专家详情：含社区贡献名片条与六项指标', expertDetailHtml.includes('talent-strip')
  && expertDetailHtml.includes('贡献积分') && expertDetailHtml.includes('回答采纳率')
  && expertDetailHtml.includes('文章阅读'));
// 好评率的口径断言放在下面 reviewsSectionHTML 的桩数据上：真实库里没有评价时
// 整个评价区走空态、连「好评率」三个字都不出现，这项就会假红（曾经把人的注意力
// 从真问题上带走）。桩里 goodRate=85.7，展示必须是 86% 而不是 8570%。

const rvPage = {
  summary: {
    total: 7, avgScore: 4.3, goodRate: 85.7,
    distribution: [
      { score: 5, count: 3 }, { score: 4, count: 2 }, { score: 3, count: 1 },
      { score: 2, count: 0 }, { score: 1, count: 1 },
    ],
  },
  list: [
    { nickname: '张三', avatar: '', score: 5, professionalScore: 5, speedScore: 4,
      communicationScore: 5, content: '定位很快', recommend: true, anonymous: false,
      reply: '感谢认可', createdAt: '2026-10-20T10:00:00+08:00' },
    { nickname: '李四', avatar: 'a.png', score: 2, professionalScore: 0, speedScore: 0,
      communicationScore: 0, content: '<script>alert(1)</script>', recommend: false,
      anonymous: true, createdAt: '2026-10-19T10:00:00+08:00' },
  ],
  total: 7, page: 1, pageSize: 5,
};
const rvHtml = expertMod.reviewsSectionHTML(rvPage);
check('评价区：好评率展示为百分比而非小数放大',
  /好评率 86%/.test(rvHtml) && !/好评率 \d{4,}%/.test(rvHtml),
  (rvHtml.match(/好评率 [\d.]+%/) || [''])[0]);
check('评价区：分布固定五档', (rvHtml.match(/rv-dist-row/g) || []).length === 5);
check('评价区：有「查看更多」且剩余数正确', rvHtml.includes('还有 5 条'),
  (rvHtml.match(/查看更多评价[^<]*/) || [''])[0]);
check('评价区：匿名昵称被替换', rvHtml.includes('匿名用户'));
check('评价区：匿名不下发原昵称', !rvHtml.includes('李四'));
check('评价区：评价正文被转义', !rvHtml.includes('<script>alert(1)</script>'));
check('评价区：专家回复块渲染', rvHtml.includes('专家回复') && rvHtml.includes('感谢认可'));
check('评价区：空评价给空态文案',
  expertMod.reviewsSectionHTML({ summary: { total: 0, avgScore: 0, goodRate: 0, distribution: [] }, list: [], total: 0 })
    .includes('还没有评价'));
check('评价区：卡片不包含订单号与用户 ID',
  !expertMod.reviewCardsHTML(rvPage.list).match(/orderId|userId/));

const expertApplyAnon = await renderPage('入驻申请（未登录）', () => expertMod.expertApplyPage(ctx()));
check('入驻申请：未登录给登录引导且不渲染表单',
  expertApplyAnon.includes('登录后才能继续') && !expertApplyAnon.includes('id="ap-form"'));

const expertWbAnon = await renderPage('专家工作台（未登录）', () => expertMod.expertWorkbenchPage(ctx()));
check('专家工作台：未登录给登录引导', expertWbAnon.includes('登录后才能继续'));

/* -------------------------------------------------------- 纯函数 -- */

check('转义：尖括号与引号',
  util.esc('<img src=x onerror="alert(1)">') === '&lt;img src=x onerror=&quot;alert(1)&quot;&gt;');

check('URL 白名单：javascript: 被拒', util.safeUrl('javascript:alert(1)') === '');
check('URL 白名单：data: 被拒', util.safeUrl('data:text/html,<script>') === '');
check('URL 白名单：https 放行', util.safeUrl('https://example.com') === 'https://example.com');
check('URL 白名单：站内路径放行', util.safeUrl('/assets/a.png') === '/assets/a.png');

const md = util.renderMarkdown(
  '# 一级标题\n\n正文 **粗体** 与 `行内代码`\n\n```bash\nrm -rf /\n```\n\n- 甲\n- 乙\n\n> 引用\n\n| A | B |\n|---|---|\n| 1 | 2 |'
);
check('Markdown：标题下沉一级', md.includes('<h2'), md.slice(0, 40));
check('Markdown：代码块保留', md.includes('<pre data-lang="bash"><code>rm -rf /</code></pre>'));
check('Markdown：无序列表', md.includes('<ul><li>甲</li><li>乙</li></ul>'));
check('Markdown：粗体与行内代码', md.includes('<strong>粗体</strong>') && md.includes('<code>行内代码</code>'));
check('Markdown：引用块', md.includes('<blockquote>'));
check('Markdown：表格', md.includes('<thead>') && md.includes('<td>1</td>'));

const evil = util.renderMarkdown('普通文字 <script>alert(1)</script> 结束');
check('Markdown：正文内脚本被转义',
  evil.includes('&lt;script&gt;') && !evil.includes('<script>'), evil.slice(0, 70));

const evilLink = util.renderMarkdown('[点我](javascript:alert(1))');
check('Markdown：伪协议链接被摘掉', !evilLink.includes('javascript:'), evilLink);

const card = cards.postCardHTML({ id: 1, title: '<b>注入</b>', slug: 'x', summary: '<i>y</i>' });
check('卡片：标题与摘要被转义',
  card.includes('&lt;b&gt;') && card.includes('&lt;i&gt;') && !card.includes('<b>注入</b>'));

const toolCard = cards.toolCardHTML({
  id: 2, name: 'T', slug: 't', stars: 12800, deployModes: ['docker', 'helm'],
  repoUrl: 'https://github.com/foo/bar.git',
});
check('工具卡片：仓库名精简', toolCard.includes('foo/bar'), '');
check('工具卡片：星标按万折算', toolCard.includes('1.3万'));

/* ------------------------------------------------------ 页面渲染 -- */

async function renderPage(label, fn) {
  try {
    const out = await fn();
    if (!out || typeof out.html !== 'string') {
      check(label, false, '没有返回 html');
      return '';
    }
    check(label, true, `${out.html.length} 字符`);
    return out.html;
  } catch (e) {
    check(label, false, `${e.name}: ${e.message}`);
    return '';
  }
}

const homeHtml = await renderPage('首页', () => home.homePage(ctx()));
if (homeHtml) {
  // 栏目名取自服务端 /channels 的实际展示名，不在脚本里抄一份。
  // 抄一份的代价是：栏目改名后这里会红，而真正的问题是"脚本没跟上"而不是"页面坏了"，
  // 排查方向被带偏一次，就没人再相信这条断言了。
  const names = channels.map((c) => c.name);
  check('首页：所有栏目都有入口', names.every((n) => homeHtml.includes(n)),
    names.filter((n) => !homeHtml.includes(n)).join('、') || '');
  check('首页：旧栏目名已改为运维实战教程',
    homeHtml.includes('运维实战教程') && !homeHtml.includes('Linux 实战教程'));
}

const articleList = await api.articles({ page: 1, pageSize: 5 });
const firstArticle = (articleList.list || [])[0];

const listHtml = await renderPage('教程栏目页', () => article.articleListPage(ctx(), 'linux-tutorial'));
check('教程页：有内容或空态', listHtml.includes('class="post"') || listHtml.includes('class="empty"'));

const detailHtml = await renderPage('文章详情',
  () => article.articleDetailPage(ctx(), firstArticle ? firstArticle.slug : 'not-exist'));
if (detailHtml && firstArticle) {
  check('文章详情：含正文容器', detailHtml.includes('data-content'));
  check('文章详情：含点赞按钮', detailHtml.includes('data-like'));
  check('文章详情：标题原样出现', detailHtml.includes(util.esc(firstArticle.title)));
}

const toolListHtml = await renderPage('工具列表', () => tools.toolListPage(ctx()));
check('工具列表：含筛选与排序', toolListHtml.includes('data-filter="language"') && toolListHtml.includes('data-sort'));

const toolDetailHtml = await renderPage('工具详情', async () => {
  const t = await api.tools({ pageSize: 1 });
  const item = (t.list || [])[0];
  return tools.toolDetailPage(ctx(), item ? item.slug : 'not-exist');
});
if (toolDetailHtml) {
  check('工具详情：含部署方式或工具信息', toolDetailHtml.includes('工具信息'));
}

/* ------------------------------ 系统教程集（ToolBook） -- */

// 种子把示例教程集挂在 openresty 上（见 bootstrap/seed_toolbook.go）。
// 不写死工具 slug 的代价是要遍历找，但写死又会在种子调整后静默失验——
// 这里取「第一本有教程集的工具」，找不到就报出原因。
let bookTool = null;
let bookTarget = null;
for (const slug of ['openresty', 'prometheus', 'grafana', 'ansible']) {
  const detail = await api.tool(slug).catch(() => null);
  const books = (detail && detail.books) || [];
  if (books.length) {
    bookTool = { slug, detail };
    bookTarget = books[0];
    break;
  }
}
check('教程集：种子里至少有一本已发布教程集', Boolean(bookTarget), bookTool ? bookTool.slug : '没有工具挂教程集');

if (bookTool) {
  const withBookHtml = await renderPage('工具详情（含系统教程集）',
    () => tools.toolDetailPage(ctx(), bookTool.slug));
  // 两块必须并存：教程集回答"怎么系统学会"，四类教程回答"还有哪些值得看"。
  // 只剩一个标题说明其中一块被覆盖了。
  check('工具详情：系统教程集与四类教程并存',
    withBookHtml.includes('系统教程集') && withBookHtml.includes('实战教程'));
  check('工具详情：教程集卡片指向独立目录页',
    new RegExp(`#/tools/${bookTool.slug}/book/\\d+`).test(withBookHtml));
  check('工具详情：教程集卡片带级别与章节数',
    /book-level is-\w+/.test(withBookHtml) && withBookHtml.includes('个章节'));
}

if (bookTarget) {
  const bookHtml = await renderPage('教程集详情',
    () => tools.toolBookPage(ctx(), bookTool.slug, String(bookTarget.id)));
  check('教程集：章与节两级都渲染',
    bookHtml.includes('book-chapter') && bookHtml.includes('book-sec'), '');
  check('教程集：第一章默认展开（进页面就能看到第一节）',
    /<details class="book-chapter"\s+open>/.test(bookHtml));
  // 系统教程集的节现在全部指向站内文章（不再挂官网外链），所以页面里
  // 不该再有 target="_blank"；万一以后重新引入外链节，这一项会红，属预期。
  check('教程集：章节不再跳官网外链', !bookHtml.includes('target="_blank"'));
  check('教程集：站内章节走 hash 路由', bookHtml.includes('#/article/'));
  check('教程集：面包屑能回到所属工具页',
    bookHtml.includes(`#/tools/${bookTool.slug}`));
  check('教程集：有全部展开/收起开关', bookHtml.includes('data-book-expand'));
}

/* -------------------------------------------- 效率三件套（纯前端） -- */

// 三个页面零接口依赖，渲染即可全量校验；数据断言挂模块导出的常量上，
// 不依赖种子库，跑在哪套环境都一样。
const csMod = await load('pages-cheatsheet.js');
const tbMod = await load('pages-toolbox.js');
const wkMod = await load('pages-weekly.js');

const csHtml = await renderPage('速查手册', () => csMod.cheatsheetPage(ctx('')));
check('速查手册：分类按钮与搜索框都在',
  csHtml.includes('data-cs-cat="all"') && csHtml.includes('data-cs-search'));
check('速查手册：条目带一键复制', csHtml.includes('data-copy="'));
check('速查手册：命令量达标', csMod.CHEATSHEET_COUNT >= 100, `${csMod.CHEATSHEET_COUNT} 条`);

const csFiltered = await renderPage('速查手册（按分类）', () => csMod.cheatsheetPage(ctx('c=kubectl')));
check('速查手册：分类过滤后只剩该分类', csFiltered.includes('kubectl get pods') && !csFiltered.includes('mysqldump'));

const tbHtml = await renderPage('工具箱（默认时间戳）', () => tbMod.toolboxPage(ctx('')));
check('工具箱：八个工具导航齐全',
  ['时间戳转换', 'Cron 解析', '编码转换', 'JWT 解码', '正则测试', 'JSON 格式化', '子网计算', '进制转换']
    .every((s) => tbHtml.includes(s)));
check('工具箱：默认面板是时间戳', tbHtml.includes('id="ts-in"') && !tbHtml.includes('id="cr-in"'));
check('工具箱：明确承诺本地计算', tbHtml.includes('本地计算'));

const tbCron = await renderPage('工具箱（cron 面板）', () => tbMod.toolboxPage(ctx('t=cron')));
check('工具箱：切面板后旧面板不残留', tbCron.includes('id="cr-in"') && !tbCron.includes('id="ts-in"'));

const wkHtml = await renderPage('周报生成器', () => wkMod.weeklyPage());
check('周报生成器：五个分区齐全',
  ['变更发布', '故障与处理', '巡检与告警', '优化与改进', '下周计划'].every((s) => wkHtml.includes(s)));
check('周报生成器：复制与下载初始禁用（没有生成前不该能点）',
  /data-wk="copy" disabled/.test(wkHtml) && /data-wk="download" disabled/.test(wkHtml));
check('周报生成器：日期范围默认本周',
  /id="wk-start" type="date" value="\d{4}-\d{2}-\d{2}"/.test(wkHtml));

const quizHomeHtml = await renderPage('题库首页', () => quiz.quizHomePage(ctx()));
check('题库首页：含练习入口', quizHomeHtml.includes('data-start'));
check('题库首页：含模拟考入口', quizHomeHtml.includes('#/exam') && quizHomeHtml.includes('exam-entry'));

const quizPracticeHtml = await renderPage('练习页', () => quiz.quizPracticePage(ctx('count=3')));
check('练习页：含作答容器与进度条',
  quizPracticeHtml.includes('data-stage') && quizPracticeHtml.includes('data-progress'));
// 选项与判分按钮由客户端状态机在挂载后渲染，纯 HTML 只能验到容器这一层；
// 抽题数据本身在这里单独确认一次。
const practiceData = await api.practice({ count: 3 });
check('练习页：抽到题目', practiceData.list.length > 0, `${practiceData.list.length} 道`);
check('练习页：题目含选项', (practiceData.list[0]?.options || []).length >= 2);

/* --------------------------------------------- 模拟考 / 排障剧本 -- */

const examMod = await load('pages-exam.js');
const rbMod = await load('pages-runbook.js');
const rbData = await load('runbook-data.js');

// 模拟考首页不依赖登录态：卷型与规则对游客同样要说得清楚。
const examHtml = await renderPage('模拟考首页', () => examMod.examHomePage(ctx('')));
check('模拟考：三种卷型齐全',
  ['快速测', '标准卷', '挑战卷'].every((s) => examHtml.includes(s)));
check('模拟考：写明限时且交卷才给答案',
  examHtml.includes('限时作答') && examHtml.includes('中途不给答案'));
check('模拟考：卷型配额在服务端上限内',
  examMod.EXAM_PAPERS.length === 3
    && examMod.EXAM_PAPERS.every((p) => p.count >= 5 && p.count <= 50 && p.minutes >= 1),
  examMod.EXAM_PAPERS.map((p) => `${p.count}题/${p.minutes}分`).join(' · '));

// 未登录时不能抽题：既不泄露题目进度，也避免白打一次必然 401 的接口。
const examRunHtml = await renderPage('模拟考（未登录）', () => examMod.examRunPage(ctx('paper=standard')));
check('模拟考：未登录先给登录引导', examRunHtml.includes('登录后开始'));

const rbHtml = await renderPage('排障剧本列表', () => rbMod.runbookListPage(ctx('')));
check('排障剧本：场景数达标', rbData.RUNBOOK_COUNT >= 10, `${rbData.RUNBOOK_COUNT} 个`);
check('排障剧本：卡片可点进详情', rbHtml.includes('#/runbook/crashloop'));

// 分支跳转写错会让值班的人点进死胡同，这条断言守的就是数据完整性。
let rbValid = 'ok';
try { rbData.validateRunbooks(); } catch (e) { rbValid = e.message; }
check('排障剧本：分支跳转全部有效', rbValid === 'ok', rbValid === 'ok' ? '' : rbValid);

const rbSearch = await renderPage('排障剧本（搜索 OOM）', () => rbMod.runbookListPage(ctx('q=OOM')));
check('排障剧本：搜索只留命中项',
  rbSearch.includes('OOM Killer') && !rbSearch.includes('#/runbook/crashloop'));

const rbStep = await renderPage('排障剧本（第一步）', () => rbMod.runbookDetailPage(ctx(''), 'crashloop'));
check('排障剧本：第一步给出命令与多个分支',
  rbStep.includes('data-copy=') && (rbStep.match(/data-branch=/g) || []).length >= 2);

const rbTerm = await renderPage('排障剧本（结论节点）',
  () => rbMod.runbookDetailPage(ctx('p=s1,end-oom'), 'crashloop'));
check('排障剧本：结论页给根因、修复与验证',
  rbTerm.includes('rb-terminal') && rbTerm.includes('根因')
  && rbTerm.includes('怎么修') && rbTerm.includes('验证'));
check('排障剧本：走过的路径可回退', rbTerm.includes('rb-trail') && rbTerm.includes('?p=s1'));

const rbMissing = await renderPage('排障剧本（不存在的 slug）',
  () => rbMod.runbookDetailPage(ctx(''), 'not-a-runbook'));
check('排障剧本：未知剧本给引导不抛错', rbMissing.includes('没有这个剧本'));

/* ------------------------------------------------ 成长中心 / 事故复盘 -- */

const growthMod = await load('pages-growth.js');

// 这一节跑在"临时账号登录"之前，正好是游客视角：榜单公开可读，
// 但我的名次与流水不该出现——未登录的人没有名次。
const growthGuest = await renderPage('成长中心（游客）', () => growthMod.growthPage(ctx('')));
check('成长中心：四个等级都摆出来', growthGuest.includes('四个等级')
  && ['新手', '活跃', '贡献者', '资深贡献者'].every((s) => growthGuest.includes(s)));
check('成长中心：积分规则四条齐全',
  ['回答被采纳', '回答被点赞', '实战教程过审', '题目过审入库'].every((s) => growthGuest.includes(s)));
check('成长中心：游客看得到榜单但看不到我的流水',
  growthGuest.includes('贡献排行榜') && !growthGuest.includes('我的积分流水'));
check('成长中心：未登录给登录引导', growthGuest.includes('登录后查看你的等级'));
// 积分商城上线后口径变了：成长中心要说清"分能换什么"，并给商城入口。
check('成长中心：说明积分有去处并链到商城',
  growthGuest.includes('换成实际的东西') && growthGuest.includes('#/mall'));

// 复盘栏目：引导条只在复盘栏目出现，教程栏目不该被塞进来。
const pmListHtml = await renderPage('事故复盘栏目', () => article.articleListPage(ctx(), 'ops-postmortem'));
check('事故复盘：写作引导条只在复盘栏目出现',
  pmListHtml.includes('pm-banner') && pmListHtml.includes('好复盘回答'));
check('事故复盘：引导条直达带栏目预选的投稿页',
  pmListHtml.includes('#/contribute?channel=ops-postmortem'));
check('事故复盘：教程栏目没有复盘引导条',
  !(await renderPage('教程栏目页（复查）', () => article.articleListPage(ctx(), 'linux-tutorial')))
    .includes('pm-banner'));

const pmPosts = await api.articles({ channel: 'ops-postmortem', pageSize: 10 }).catch(() => ({ total: 0, list: [] }));
check('事故复盘：示例复盘已入库', (pmPosts.total || 0) >= 3, `${pmPosts.total || 0} 篇`);
const pmFirst = (pmPosts.list || [])[0];

const pmDetail = await renderPage('事故复盘详情',
  () => article.articleDetailPage(ctx(), pmFirst ? pmFirst.slug : 'not-exist'));
check('事故复盘：详情页渲染出时间线与改进项',
  pmDetail.includes('时间线') && pmDetail.includes('改进项'));

const resListHtml = await renderPage('资源列表', () => resources.resourceListPage(ctx()));
check('资源列表：含文件类型筛选', resListHtml.includes('data-filter="fileExt"'));

// 必须挑一份**真的有文件**的资源来验下载按钮。
// 资源现在有两种形态：平台托管文件的、只给官网/镜像外链的。
// 拿列表第一条去验，赶上一条外链资源就会红——而页面其实完全正常。
const hosted = (await api.resources({ pageSize: 50 }).catch(() => ({ list: [] })))
  .list.find((r) => r.hasFile) || null;
if (hosted) {
  const resDetailHtml = await renderPage('资源详情（含文件）',
    () => resources.resourceDetailPage(ctx(), hosted.slug));
  check('资源详情：含下载按钮', resDetailHtml.includes('data-download'));
  check('资源详情：展示校验和', resDetailHtml.includes('SHA256') || resDetailHtml.includes('文件信息'));
} else {
  check('资源详情：含下载按钮', false, '库里没有托管文件的资源');
}

const searchHtml = await renderPage('搜索页', () => search.searchPage(ctx('q=重启')));
if (searchHtml) {
  check('搜索页：出现关键词', searchHtml.includes('重启'));
}

/* ------------------------------------------------ 检索理解（问句） -- */

// 这一组守的是最初的痛点：用户打的是完整问句，而不是关键词。
const suggest = await api.searchSuggest('磁盘满了怎么办');
check('检索词解析：识别出概念', Array.isArray(suggest.terms) && suggest.terms.includes('磁盘'),
  JSON.stringify(suggest.terms));
check('检索词解析：同义词展开', (suggest.expanded || []).includes('硬盘') || (suggest.expanded || []).includes('disk'),
  JSON.stringify(suggest.expanded));
check('检索词解析：带热门词', Array.isArray(suggest.hot));

const naturalHtml = await renderPage('问句搜索页', () => search.searchPage(ctx('q=磁盘满了怎么办')));
check('问句搜索页：回显检索依据', naturalHtml.includes('按这些词检索'));

// 关键词换成大白话之后，结果不该反而变少——这正是这次改动的目的。
const plain = await api.articles({ keyword: '磁盘满了怎么办', pageSize: 5 });
const tokens = await api.articles({ keyword: '磁盘', pageSize: 5 });
const plainTotal = plain.total || 0;
const tokenTotal = tokens.total || 0;
check('问句能搜到内容（不再空页）', plainTotal > 0, `问句 ${plainTotal} 条 / 关键词 ${tokenTotal} 条`);

const emptyHtml = await renderPage('零结果搜索页', () => search.searchPage(ctx('q=zzz不存在的词zzz')));
check('零结果页：给出可点选的建议词', emptyHtml.includes('没有找到匹配的内容'));

const blankHtml = await renderPage('空搜索页', () => search.searchPage(ctx('')));
check('空搜索页：给热门词或提示', blankHtml.includes('热门关键词') || blankHtml.includes('输入关键词'));

/* ------------------------------------------------------ 订单域纯函数 -- */

// 金额与倒计时是订单页最容易错的两处：错一位就是"多收一块钱"级别的 bug。
check('金额：分转元带千分位', ordersMod.fmtCents(128000) === '¥1,280.00', ordersMod.fmtCents(128000));
check('金额：零值不变成空', ordersMod.fmtCents(0) === '¥0.00', ordersMod.fmtCents(0));
check('金额：不足一元保留两位', ordersMod.fmtCents(5) === '¥0.05', ordersMod.fmtCents(5));

check('倒计时：过期显示已超时',
  ordersMod.remainText(new Date(Date.now() - 1000).toISOString()) === '已超时');
check('倒计时：空值返回空串而不是 NaN', ordersMod.remainText(null) === '', ordersMod.remainText(null));

// 状态映射必须覆盖后端的 9 个状态。少一个，那一单在页面上就是个空徽标。
const statusKeys = Object.keys(ordersMod.ORDER_STATUS);
check('订单状态：9 个状态齐全', statusKeys.length === 9, `${statusKeys.length} 个`);

// 流转表是 internal/model/order.go 的镜像，这里只能验它自己内部自洽：
// 表里出现的状态都得有文案，终态不能有出边。真正的权威仍在服务端。
const unknownStatus = new Set();
Object.entries(ordersMod.ORDER_TRANSITIONS).forEach(([from, tos]) => {
  if (!ordersMod.ORDER_STATUS[from]) unknownStatus.add(from);
  tos.forEach((t) => { if (!ordersMod.ORDER_STATUS[t]) unknownStatus.add(t); });
});
check('订单状态机：流转表与文案表一致', unknownStatus.size === 0, [...unknownStatus].join('、'));
check('订单状态机：终态没有出边',
  ['settled', 'refunded', 'cancelled'].every((s) => !ordersMod.ORDER_TRANSITIONS[s]));
check('订单状态机：争议不能直达已结算',
  !(ordersMod.ORDER_TRANSITIONS.disputed || []).includes('settled'));

const acts = ordersMod.orderActions;
check('动作：待支付可支付可取消',
  acts({ status: 'pending_pay' }, 'user').includes('pay') &&
  acts({ status: 'pending_pay' }, 'user').includes('cancel'));
check('动作：已支付后不给取消按钮',
  !acts({ status: 'in_service' }, 'user').includes('cancel'));
check('动作：服务中可申请争议', acts({ status: 'in_service' }, 'user').includes('dispute'));
check('动作：待确认可确认可争议',
  acts({ status: 'pending_confirm' }, 'user').includes('confirm') &&
  acts({ status: 'pending_confirm' }, 'user').includes('dispute'));
check('动作：完成后可评价', acts({ status: 'completed' }, 'user').includes('review'));
check('动作：已评价不再提示评价',
  !acts({ status: 'completed', reviewed: true }, 'user').includes('review'));
check('动作：专家在服务中可开会话与标记完成',
  acts({ status: 'in_service' }, 'expert').includes('session') &&
  acts({ status: 'in_service' }, 'expert').includes('finish'));
check('动作：专家只对差评申诉',
  !acts({ status: 'completed', review: { score: 5, reply: 'x' } }, 'expert').includes('appeal') &&
  acts({ status: 'completed', review: { score: 2 } }, 'expert').includes('appeal'));

/* ------------------------------------------------ 9 种状态的详情渲染 -- */

// 详情模板是纯函数（orderDetailHTML），所以可以脱离数据库把每种状态都渲染一遍。
// 这 9 条断言守的是"模板里没有写死某一状态的假设"——比如某个字段只在
// settled 时才有，模板却在所有状态下都读它。
const sampleOrder = (status, extra = {}) => ({
  orderNo: 'OE20261020120000000001', type: 'question', status,
  title: '节点 NotReady 排查', serviceMode: 'im',
  userId: 11, expertId: 22, expertName: '张工', userName: '李工',
  amountCents: 20000, commissionCents: 3000, expertAmountCents: 17000,
  reviewed: false,
  createdAt: '2026-10-20T04:00:00Z',
  timeline: [{ status: 'pending_accept', at: '2026-10-20T04:00:00Z' }],
  sessions: [],
  ...extra,
});

const STATUS_CASES = [
  ['pending_accept', 'user',   ['cancel'],                  ['pay', 'confirm', 'review']],
  ['pending_pay',    'user',   ['pay', 'cancel'],           ['confirm']],
  ['in_service',     'user',   ['dispute'],                 ['pay', 'cancel']],
  ['pending_confirm','user',   ['confirm', 'dispute'],      ['pay']],
  ['completed',      'user',   ['review'],                  ['pay', 'confirm']],
  ['settled',        'user',   [],                          ['pay', 'cancel']],
  ['disputed',       'user',   [],                          ['pay', 'review']],
  ['refunded',       'user',   [],                          ['pay', 'cancel']],
  ['cancelled',      'user',   [],                          ['pay', 'confirm']],
  // 专家侧：服务中才能开会话与标记完成，其它状态下这两个按钮不该出现
  ['in_service',     'expert', ['session', 'finish'],       ['pay', 'confirm']],
  ['pending_pay',    'expert', [],                          ['session', 'finish', 'pay']],
];

let statusRenderBad = [];
for (const [status, role, mustHave, mustNot] of STATUS_CASES) {
  const html = orderPages.orderDetailHTML(sampleOrder(status), role);
  const label = (ordersMod.ORDER_STATUS[status] || {}).label || status;

  if (!html.includes(label)) statusRenderBad.push(`${status}/${role}: 缺状态文案`);
  mustHave.forEach((a) => {
    if (!html.includes(`data-order-act="${a}"`)) statusRenderBad.push(`${status}/${role}: 缺 ${a}`);
  });
  mustNot.forEach((a) => {
    if (html.includes(`data-order-act="${a}"`)) statusRenderBad.push(`${status}/${role}: 多了 ${a}`);
  });
  if (!html.includes('class="status-banner')) statusRenderBad.push(`${status}/${role}: 缺状态横幅`);
}
check('订单详情：11 种"状态 × 视角"组合渲染正确', statusRenderBad.length === 0,
  statusRenderBad.slice(0, 4).join('；'));

const settledHtml = orderPages.orderDetailHTML(
  sampleOrder('settled', {
    settlement: {
      settlementNo: 'ST20261020001', amountCents: 20000, commissionCents: 3000,
      expertAmountCents: 17000, commissionRateBp: 1500, status: 'settled',
      settledAt: '2026-10-21T04:00:00Z',
    },
  }), 'user');
check('订单详情：已结算显示分账凭证', settledHtml.includes('ST20261020001') && settledHtml.includes('15.00%'));

const cancelledHtml = orderPages.orderDetailHTML(
  sampleOrder('cancelled', { cancelReason: '支付超时，系统自动关闭' }), 'user');
check('订单详情：取消订单显示原因', cancelledHtml.includes('支付超时'));

const reviewHtml = orderPages.orderDetailHTML(sampleOrder('settled', {
  reviewed: true,
  review: {
    score: 5, professionalScore: 5, speedScore: 4, communicationScore: 5,
    content: '排障思路清晰，半小时定位到问题。', recommend: true, anonymous: false,
    reply: '感谢反馈', appealed: false, createdAt: '2026-10-21T04:00:00Z',
  },
}), 'user');
check('订单详情：已评价不再给评价按钮',
  reviewHtml.includes('排障思路清晰') && !reviewHtml.includes('data-order-act="review"'));

const disputeHtml = orderPages.orderDetailHTML(sampleOrder('disputed', {
  dispute: {
    id: 9, reason: '专家未按约定完成排查就标记完成', status: 'open',
    evidence: [], createdAt: '2026-10-21T04:00:00Z',
  },
}), 'user');
check('订单详情：争议显示原因与处理中', disputeHtml.includes('专家未按约定完成排查') && disputeHtml.includes('处理中'));

// 详情页拼的 HTML 最多，转义漏一处就是存储型 XSS。这里连 remark 一起验。
const xssHtml = orderPages.orderDetailHTML(sampleOrder('in_service', {
  title: '<script>alert(1)</script>',
  remark: '<img src=x onerror=alert(2)>',
}), 'user');
check('订单详情：标题与备注都被转义',
  !xssHtml.includes('<script>') && !xssHtml.includes('<img src=x'),
  '');

/* -------------------------------------------- 订单页（未登录态） -- */

const askAnon = await renderPage('发布悬赏（未登录）', () => questionPages.questionNewPage(ctx()));
check('发布悬赏：未登录时引导登录', askAnon.includes('data-act="login"'));

const hallAnon = await renderPage('接单大厅（未登录）', () => hallPages.hallPage(ctx()));
check('接单大厅：未登录时引导登录', hallAnon.includes('data-act="login"'));

const ordersAnon = await renderPage('我的订单（未登录）', () => orderPages.orderListPage(ctx()));
check('我的订单：未登录时给登录入口', ordersAnon.includes('data-act="login"'));

/* -------------------------------------------- 订单页（登录态） -- */

// 用一次性手机号注册登录：dev 环境验证码由接口直接回传，不需要读服务日志。
// 号码每次不同，避免与冒烟脚本用的固定号码撞限流。
const phone = '138' + String(Date.now()).slice(-8);
let loggedIn = false;
try {
  const sent = await apiMod.sendSMSCode(phone);
  await apiMod.loginBySMS(phone, sent.devCode);
  loggedIn = true;
} catch (e) {
  check('订单页：临时账号登录', false, `${e.name}: ${e.message}`);
}
check('订单页：临时账号登录', loggedIn);

// ------------------------------------------------------ 只读与发言权限 --
//
// 短信首登自动建的账号默认只读：这正是这条规则最真实的样本，不用再造一个。
// 三个"发布"入口都必须先看到权限说明，而不是一张填完提交必然失败的表格。
const roContribute = await renderPage('投稿页（只读账号）', () => contributeMod.contributePage(ctx()));
check('只读账号：投稿页给权限说明而不是表单',
  roContribute.includes('需要先开通发言权限') && !roContribute.includes('id="c-form"'));
check('只读账号：权限说明有直达申请页的入口', roContribute.includes('data-act="go"'));

const roAsk = await renderPage('提问页（只读账号）', () => qaMod.qaAskPage(ctx()));
check('只读账号：问答提问页同样被拦', roAsk.includes('需要先开通发言权限'));

const roBounty = await renderPage('悬赏页（只读账号）', () => questionPages.questionNewPage(ctx()));
check('只读账号：悬赏发布页同样被拦', roBounty.includes('需要先开通发言权限'));

let speakGranted = false;
if (loggedIn) {
  // 后面几节（悬赏、投稿表单、后台）都需要一个"能发言"的账号。
  // 这里走一次真实链路：提交申请 → 管理员通过 → 还原临时账号的会话。
  // 还原而不是重新登录，是因为验证码有 60 秒间隔，重登会白等一分钟。
  const tempSnap = apiMod.snapshotSession();
  try {
    const uid = (apiMod.currentUser() || {}).id;
    await api.applySpeak({ realName: '自检账号', contact: phone, note: '渲染自检' });
    await loginAsAdmin();
    await api.adminSpeakReview(uid, { approve: true, note: '渲染自检放行' });
    speakGranted = true;
  } catch (e) {
    check('只读账号：提交实名申请并由管理员通过', false, `${e.name}: ${e.message}`);
  } finally {
    apiMod.restoreSession(tempSnap);
    // 快照里的 user.canSpeak 是审批之前的旧值，而前端的权限判断读的就是它。
    // 不刷新一次，后面几节会继续按"只读"渲染，看起来像审核没生效。
    if (speakGranted) await apiMod.refreshProfile();
  }
  check('只读账号：提交实名申请并由管理员通过', speakGranted);
}

if (loggedIn && speakGranted) {
  // 通过之后「我的」页要立刻反映新状态：这张卡是用户唯一能看到"我现在能不能说话"的地方。
  const meHtml = await renderPage('我的（登录态）', () => meMod.mePage(ctx()));
  check('我的：显示发言权限状态', meHtml.includes('发言权限') && meHtml.includes('可发言'));
  check('我的：资料区渲染出来', meHtml.includes('me-rows') && meHtml.includes('me-card'));
  check('我的：成长卡给出等级、积分与上榜情况', meHtml.includes('me-growth') && meHtml.includes('成长中心'));

  // 登录后的成长中心：进度条与名次区要出现——哪怕积分还是 0，
  // 也要显示"未上榜"，比空着一块让人困惑的空白好。
  const growthMe = await renderPage('成长中心（登录）', () => growthMod.growthPage(ctx()));
  check('成长中心：登录后显示等级进度与名次区',
    growthMe.includes('aria-label="等级进度"') && growthMe.includes('未上榜'));
}

if (loggedIn && speakGranted) {
  const skills = await api.skills().catch(() => []);
  const skillId = skills.length ? String(skills[0].id) : '';

  // 标题里塞一段 HTML：订单列表与详情都必须把它转义。
  // 悬赏标题是用户输入里最"长"的一个字段，漏转义就是存储型 XSS。
  const dirtyTitle = '<img src=x onerror=alert(1)> 节点 NotReady 排查';
  let created = null;
  try {
    created = await api.createQuestion({
      title: dirtyTitle,
      description: '节点反复 NotReady，kubelet 日志显示无法连接 apiserver，已重启无果。',
      skillIds: skillId ? [skillId] : ['1'],
      budgetCents: 20000,
      expireHours: 24,
    });
  } catch (e) {
    check('订单页：发布一条悬赏', false, `${e.name}: ${e.message}`);
  }
  check('订单页：发布一条悬赏', Boolean(created && created.orderNo), created && created.orderNo);

  if (created && created.orderNo) {
    const listHtml = await renderPage('我的订单（有数据）', () => orderPages.orderListPage(ctx('side=user')));
    check('我的订单：列出刚发布的订单', listHtml.includes(created.orderNo));
    check('我的订单：显示状态徽标', listHtml.includes('status-chip'));
    check('我的订单：标题被转义',
      listHtml.includes('&lt;img') && !listHtml.includes('<img src=x'), '');

    const detailHtml = await renderPage('订单详情', () => orderPages.orderDetailPage(ctx(), created.orderNo));
    check('订单详情：待接单状态文案', detailHtml.includes('待接单'));
    check('订单详情：含履约时间轴', detailHtml.includes('class="tl"'));
    check('订单详情：金额按元展示', detailHtml.includes('¥200.00'));
    check('订单详情：待接单给取消按钮', detailHtml.includes('data-order-act="cancel"'));
    check('订单详情：待接单不给支付按钮', !detailHtml.includes('data-order-act="pay"'));
    check('订单详情：标题被转义',
      detailHtml.includes('&lt;img') && !detailHtml.includes('<img src=x'), '');
  }

  /* ------------------------ 内容投稿：题目表单（登录态） -- */

  // 一个入口两个表单：用户不知道平台内部把内容分成 articles 与 quiz_questions，
  // 所以两个 tab 必须同时在，且当前项高亮——只有一个 tab 等于没做分流。
  const quizFormHtml = await renderPage('投稿页（题目）', () => contributeMod.contributePage(ctx('type=quiz')));
  check('投稿页：类型 tab 两项都在且当前项高亮',
    quizFormHtml.includes('contrib-tab is-active') &&
    quizFormHtml.includes('href="#/contribute?type=quiz"'));
  check('投稿页：题目表单含题型下拉与选项容器',
    quizFormHtml.includes('id="q-type"') && quizFormHtml.includes('id="q-options"'));
  check('投稿页：答案区与解析字段都在',
    quizFormHtml.includes('id="q-answer"') && quizFormHtml.includes('id="q-analysis"'));
  // 判断题的两个选项由系统填，所以「加了固定行」这件事只在渲染时才看得出来。
  check('投稿页：题目表单给出「解析必填」的说明',
    quizFormHtml.includes('必填'));

  const mineQuizHtml = await renderPage('我的投稿（题目）',
    () => contributeMod.myContributionsPage(ctx('type=quiz')));
  check('我的投稿：题目 tab 与列表/空态互斥',
    mineQuizHtml.includes('href="#/me/contributions?type=quiz"') &&
    (mineQuizHtml.includes('contrib-row') || mineQuizHtml.includes('class="empty"')));

  const askHtml = await renderPage('发布悬赏（登录态）', () => questionPages.questionNewPage(ctx()));  check('发布悬赏：含表单与技能选择',
    askHtml.includes('id="q-form"') && askHtml.includes('data-skill'));
  check('发布悬赏：预算按元输入', askHtml.includes('id="q-budget"'));

  const hallHtml = await renderPage('接单大厅（登录态）', () => hallPages.hallPage(ctx()));
  check('接单大厅：非认证专家给出说明或列表',
    hallHtml.includes('还不是认证专家') || hallHtml.includes('hall-card') || hallHtml.includes('大厅暂时是空的'));

  // 下面几条走的是 api.js 的真实封装，验证的是"前端调的服务端认"，
  // 而不只是"模板渲染得出来"。路径或参数写错会在这里红。
  const channels = await api.payChannels().catch(() => []);
  check('支付通道：清单非空且有可用项',
    channels.length > 0 && channels.some((c) => c.available), `${channels.length} 个`);

  const counts = await api.orderCounts('user').catch(() => null);
  check('订单角标：返回 total 与 byStatus',
    Boolean(counts) && typeof counts.total === 'number' && typeof counts.byStatus === 'object');

  let hallErr = null;
  try {
    await api.hall({ page: 1, pageSize: 5 });
  } catch (e) {
    hallErr = e;
  }
  check('接单大厅：未认证时服务端拒绝且能映射成业务错误',
    hallErr === null || hallErr instanceof apiMod.ApiError, hallErr ? String(hallErr.code) : '未拒绝');

  if (created && created.orderNo) {
    let cancelErr = null;
    try {
      await api.cancelOrder(created.orderNo, { reason: '渲染自检后清理' });
    } catch (e) {
      cancelErr = e;
    }
    const after = cancelErr ? null : await api.order(created.orderNo).catch(() => null);
    check('取消订单：状态落为已取消并保留原因',
      Boolean(after) && after.status === 'cancelled' && after.cancelReason === '渲染自检后清理',
      cancelErr ? `${cancelErr.name}: ${cancelErr.message}` : (after && after.status));
  } else {
    check('取消订单：状态落为已取消并保留原因', false, '没有可取消的订单');
  }
}

/* -------------------------------------- 工具四类教程 / 资源外链 -- */

// 这四类教程是「开源工具推荐」的核心承诺：点开一个工具，能直接拿到
// 部署、实施、优化、集群四条路径。少一类，这个工具页就只是「简介 + 仓库链接」。
const toolWithGuides = await api.tool('prometheus').catch(() => null);
if (toolWithGuides && (toolWithGuides.guides || []).length) {
  const kinds = new Set((toolWithGuides.guides || []).map((g) => g.kind));
  check('工具四类教程：deploy/implement/optimize/cluster 齐全',
    ['deploy', 'implement', 'optimize', 'cluster'].every((k) => kinds.has(k)),
    [...kinds].join(','));

  const guidesHtml = await renderPage('工具详情（含教程）',
    () => tools.toolDetailPage(ctx(), 'prometheus'));
  check('工具详情：渲染教程区块', guidesHtml.includes('class="guide-panel"'));
  check('工具详情：四节标题都在',
    ['部署教程', '实施教程', '优化教程', '集群构建'].every((s) => guidesHtml.includes(s)));
  // 站内教程跳 hash 路由，站外链接必须新窗口打开——
  // 两者混成一个 target，用户点一下就把自己正在读的页面换掉了。
  //
  // 断言不能挂在某一个工具上：库里到底哪条教程是站内文章取决于种子数据，
  // 拿一个恰好全是外链的工具来验，会变成"这次红下次绿"的假失败。
  // 所以先找出一条真正关联了站内文章的工具，再验它的页面。
  const allTools = (await api.tools({ pageSize: 50 }).catch(() => ({ list: [] }))).list || [];
  let linkedTool = null;
  for (const t of allTools) {
    const full = await api.tool(t.slug).catch(() => null);
    if (full && (full.guides || []).some((g) => g.articleSlug)) { linkedTool = full; break; }
  }
  if (linkedTool) {
    const linkedHtml = await renderPage('工具详情（含站内教程）',
      () => tools.toolDetailPage(ctx(), linkedTool.slug));
    check('工具详情：站内教程走 hash 路由', /href="#\/article\//.test(linkedHtml),
      linkedTool.slug);
  } else {
    check('工具详情：站内教程走 hash 路由', false, '没有任何工具关联了站内文章');
  }
  check('工具详情：站外教程带 noopener',
    !guidesHtml.includes('guide-ext') || guidesHtml.includes('rel="noopener noreferrer"'));
} else {
  check('工具四类教程：deploy/implement/optimize/cluster 齐全', false, 'prometheus 未取到教程');
}

// 只给外链的资源不能出现签名下载按钮——那个按钮点下去只会拿不到地址。
const linkOnly = (await api.resources({ pageSize: 50 }).catch(() => ({ list: [] })))
  .list.find((r) => r.hasFile === false) || null;
if (linkOnly) {
  const linkOnlyHtml = await renderPage('资源详情（仅外链）',
    () => resources.resourceDetailPage(ctx(), linkOnly.slug));
  check('资源详情：无托管文件时不显示下载按钮', !linkOnlyHtml.includes('data-download'));
  check('资源详情：渲染外部下载地址', linkOnlyHtml.includes('res-link'));
  check('资源详情：隐藏文件大小等无效信息', !linkOnlyHtml.includes('文件信息'));
} else {
  check('资源详情：无托管文件时不显示下载按钮', false, '库里没有仅外链的资源');
}

/* -------------------------------------------- 题库按方向刷题 -- */

// 分类卡是「按方向刷题」的入口，只有按 categoryId 抽题才叫"刷某个方向"。
const quizCats = await api.quizCategories().catch(() => []);
check('题库分类：返回多个方向', quizCats.length >= 6, `${quizCats.length} 个`);
check('题库分类：覆盖云原生与 Windows Server',
  quizCats.some((c) => c.slug === 'cloud-native') && quizCats.some((c) => c.slug === 'windows-server'));
check('题库首页：渲染方向分类卡', quizHomeHtml.includes('data-cat-id'));

// 0 题的分类要保留但不可点，否则用户点进去是一片空白
const emptyCat = quizCats.find((c) => !c.count);
if (emptyCat) {
  check('题库分类：0 题分类标为筹备中', quizHomeHtml.includes('is-empty'));
}

/* ------------------------------------------------ 投稿与审核 -- */

const contribute = await load('pages-contribute.js');

// 未登录态的渲染在脚本最前面验（见「未登录态」一节）。
// api.js 把令牌缓存在模块级变量里，清 localStorage 不会让它变回未登录——
// 在这里假装「登出」只会得到一份登录态的 HTML，然后三条断言永远为红。
const contributeAuthed = await renderPage('投稿页（登录态）', () => contribute.contributePage(ctx()));
check('投稿页：登录后渲染提交表单', contributeAuthed.includes('id="c-form"'));
check('投稿页：表单含栏目与分类选择',
  contributeAuthed.includes('id="c-channel"') && contributeAuthed.includes('id="c-category"'));
// 资讯栏目不开放投稿，下拉里不能出现它——服务端会拒，让用户填完再报错很蠢。
check('投稿页：栏目下拉不含运维资讯', !contributeAuthed.includes('value="ops-news"'));

const mineAuthed = await renderPage('我的投稿（登录态）', () => contribute.myContributionsPage(ctx()));
check('我的投稿：登录后渲染列表或空态',
  mineAuthed.includes('contrib-list') || mineAuthed.includes('class="empty"'));

// 资讯转述必须标出处：不标的话用户会把第三方观点当成平台推荐。
const feedCard = cards.postCardHTML({
  title: 'Kubernetes v1.34 发布', slug: 'k8s-134',
  sourceName: 'Kubernetes Blog', sourceUrl: 'https://kubernetes.io/blog/x',
});
check('资讯卡片：显示来源徽标', feedCard.includes('chip-source') && feedCard.includes('Kubernetes Blog'));
// 嵌套 <a> 会把外层的卡片链接提前闭合，整块布局散掉——徽标必须是 span。
check('资讯卡片：来源徽标不嵌套锚点', !/chip-source" href/.test(feedCard));

const ownCard = cards.postCardHTML({ title: '自己写的教程', slug: 'own' });
check('资讯卡片：原创内容不显示来源徽标', !ownCard.includes('chip-source'));

/* ------------------------------------------------------ 运营后台 -- */

const admin = await load('pages-admin.js');

// 未登录态在脚本最前面验（api.js 的令牌缓存在模块变量里，这里已经登不回去了）。
// 当前登录的是临时普通账号，必须看到"没有权限"而不是一片空白或 403 堆栈。
const adminDenied = await renderPage('后台（非管理员）', () => admin.adminDashboardPage(ctx()));
check('后台：非管理员给无权限说明', adminDenied.includes('没有后台权限'));

// 管理员账号（dev/test 环境由种子创建）。
//
// 优先用口令登录：自检里要登两次管理员（一次给只读账号放权限，一次跑后台断言），
// 而同一号码 60 秒只能发一条验证码——走短信就必然白等一轮。
// 口令被改过时退回短信，并把"撞限流"当成正常保护等一轮，不让脚本跑得快
// 表现成"后台坏了"：这条断言红过一次，以后就没人信它了。
async function loginAsAdmin() {
  try {
    await apiMod.loginByPassword('13800000000', 'Admin@12345');
    if ((apiMod.currentUser() || {}).role === 'admin') return true;
  } catch {
    // 口令不对就往下走短信，不在这里下结论。
  }
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      const sent = await apiMod.sendSMSCode('13800000000');
      await apiMod.loginBySMS('13800000000', sent.devCode);
      return (apiMod.currentUser() || {}).role === 'admin';
    } catch (e) {
      const rateLimited = /60 秒|频繁/.test(String(e.message || ''));
      if (!rateLimited || attempt === 2) {
        check('后台：管理员登录', false, `${e.name}: ${e.message}`);
        return false;
      }
      await new Promise((r) => setTimeout(r, 62000));
    }
  }
  return false;
}

const adminOk = await loginAsAdmin();
if (adminOk) check('后台：管理员登录', true);

if (adminOk) {
  const dashHtml = await renderPage('后台概览', () => admin.adminDashboardPage(ctx()));
  check('后台概览：统计卡都在',
    ['待审内容', '待审专家', '待审提现', '待打款', '待审发言', '待支付订单', '订阅源异常'].every((s) => dashHtml.includes(s)));
  check('后台概览：子导航可跳到待审队列', dashHtml.includes('#/admin/review'));
  check('后台概览：子导航含提现审批', dashHtml.includes('#/admin/withdrawals'));
  check('后台概览：子导航含发言权限', dashHtml.includes('#/admin/speak'));

  const speakAdminHtml = await renderPage('后台发言申请', () => admin.adminSpeakApplicationsPage(ctx('tab=pending')));
  check('后台发言申请：四个状态 tab 都在',
    ['待审核', '已通过', '已驳回', '全部'].every((s) => speakAdminHtml.includes(s)));
  check('后台发言申请：列表区渲染（有内容或空态）',
    speakAdminHtml.includes('admin-table') && (speakAdminHtml.includes('<tr>') || speakAdminHtml.includes('没有待审核')));

  const reviewHtml = await renderPage('后台待审队列', () => admin.adminReviewPage(ctx()));
  check('待审队列：四个内容 tab 都在',
    ['文章投稿', '工具', '资源', '题目'].every((s) => reviewHtml.includes(s)));
  check('待审队列：列表区渲染（有内容或空态）',
    reviewHtml.includes('admin-list') || reviewHtml.includes('class="empty"'));

  const newsHtml = await renderPage('后台资讯源', () => admin.adminNewsPage(ctx()));
  check('资讯源：表格与新增按钮都在',
    newsHtml.includes('admin-table') && newsHtml.includes('新增订阅源'));
  // 抓取失败必须能一眼看出是哪个源坏了，否则运营只能挨个点开。
  check('资讯源：显示上次抓取状态列', newsHtml.includes('上次抓取') && newsHtml.includes('累计'));

  const artsHtml = await renderPage('后台文章列表', () => admin.adminArticlesPage(ctx()));
  check('文章列表：表格与新建入口都在',
    artsHtml.includes('admin-table') && artsHtml.includes('新建文章'));
  check('文章列表：能按待审核筛选', artsHtml.includes('value="pending"'));

  const artNewHtml = await renderPage('后台新建文章', () => admin.adminArticleEditPage(ctx(), 'new'));
  check('文章编辑：正文用等宽输入框', artNewHtml.includes('code-input'));
  check('文章编辑：栏目下拉不含题库/工具类栏目', !artNewHtml.includes('value="ops-quiz"'));

  const toolsHtml = await renderPage('后台工具列表', () => admin.adminToolsPage(ctx()));
  check('工具列表：每行都有教程维护入口', toolsHtml.includes('维护教程'));
  check('工具列表：每行都有系统教程集入口', toolsHtml.includes('系统教程'));

  const resHtml = await renderPage('后台资源列表', () => admin.adminResourcesPage(ctx()));
  check('资源列表：区分托管文件与外链', resHtml.includes('平台托管') || resHtml.includes('只给外链'));

  const quizHtml = await renderPage('后台题库', () => admin.adminQuizPage(ctx()));
  check('题库：显示方向与答对率', quizHtml.includes('答对率'));
  check('题库：批量导入入口在工具栏里', quizHtml.includes('data-quiz-import'));

  // 导入走一遍真实接口：路径或字段名写错会在这里红，而不只是"模板渲染得出来"。
  // 导入的题是草稿，验完立刻删掉——否则每跑一次题库就多一道自检题。
  let importedRes = null;
  let importErr = null;
  try {
    importedRes = await api.importQuestions({
      items: [{
        stem: 'render_check 自检题：容器内 df 显示已满但 du 求和远小于容量，最可能的原因是什么？',
        type: 'single',
        options: [
          { key: 'A', text: '有进程持有已被删除的文件' },
          { key: 'B', text: '磁盘没有真正挂载' },
        ],
        correct: ['A'],
        analysis: '被删除但仍被进程打开的文件会继续占用空间，lsof +L1 可以查到。',
        difficulty: 'medium',
      }],
    });
  } catch (e) {
    importErr = e;
  }
  check('题库导入：接口可用且只落草稿',
    Boolean(importedRes) && importedRes.imported === 1 &&
    (!importedRes.errors || importedRes.errors.length === 0),
    importErr ? `${importErr.name}: ${importErr.message}`
      : (importedRes ? `imported=${importedRes.imported}` : ''));

  if (importedRes && importedRes.imported === 1) {
    const found = await api.adminQuestions({ keyword: 'render_check 自检题', page: 1, pageSize: 5 })
      .catch(() => ({ list: [] }));
    const row = (found.list || [])[0];
    check('题库导入：落库后状态是草稿而非已发布',
      Boolean(row) && row.status === 'draft', row ? row.status : '没查回导入的题');
    if (row) await api.deleteQuestion(row.id).catch(() => {});
  } else {
    // 前面失败了也要保持断言条数稳定：缺一条断言和"少验了一条"长得一样。
    check('题库导入：落库后状态是草稿而非已发布', false, '导入没成功，跳过');
  }


  const expertsHtml = await renderPage('后台专家审核', () => admin.adminExpertsPage(ctx()));
  check('专家审核：默认只看待审', expertsHtml.includes('value="pending"'));

  const wdHtml = await renderPage('后台提现审批', () => admin.adminWithdrawalsPage(ctx()));
  check('提现审批：默认落在待审核 tab',
    wdHtml.includes('href="#/admin/withdrawals?status=pending" class="is-active"'));
  check('提现审批：五种状态 tab 都在',
    ['待审核', '待打款', '已打款', '已驳回', '打款失败'].every((s) => wdHtml.includes(s)));
  // 「通过」与「打款」是两步：页面必须说清通过之后钱还在冻结里，
  // 否则运营会以为点了通过就算付完了。
  check('提现审批：说明通过不等于打款', wdHtml.includes('钱仍冻') && wdHtml.includes('已打款'));
  check('提现审批：表格列出专家与收款账户',
    wdHtml.includes('提现单') && wdHtml.includes('收款账户') && wdHtml.includes('专家'));

  const ordersHtml = await renderPage('后台订单', () => admin.adminOrdersPage(ctx()));
  check('订单：显示金额与状态', ordersHtml.includes('金额') && ordersHtml.includes('状态'));

  const usersHtml = await renderPage('后台用户治理', () => admin.adminUsersPage(ctx()));
  check('用户治理：列角色与状态两列', usersHtml.includes('>角色<') && usersHtml.includes('>状态<'));
  // 后台列表会被投屏、被截图，手机号必须脱敏；同时要说明怎么才能查到人，
  // 否则运营会拿半个号码去搜，得到空列表以为用户不存在。
  check('用户治理：说明手机号脱敏与精确搜索',
    usersHtml.includes('脱敏') && usersHtml.includes('11 位手机号'));
  check('用户治理：说明封禁即时生效', usersHtml.includes('封禁立即生效'));
  // 封禁要有原因输入，且不能只给一个"删除"了事——删了订单与结算就都断链了。
  check('用户治理：只有封禁/解封，没有删除账号', !usersHtml.includes('删除账号'));

  /* 财务对账：页面只读，但四列口径不同，任何一列被合并成"当日利润"都是错的。 */
  const finHtml = await renderPage('后台财务对账', () => admin.adminFinancePage(ctx()));
  check('财务对账：四个口径的汇总卡都在',
    ['期间收款', '期间退款', '期间提现打款', '期间净现金'].every((s) => finHtml.includes(s)));
  check('财务对账：平台欠付四张卡都在',
    ['代管中', '专家可提', '提现冻结', '处理中提现'].every((s) => finHtml.includes(s)));
  check('财务对账：七项账面校验都在',
    ['订单金额恒等式', '分账金额恒等式', '提现守恒', '专家台账',
      '已结算订单都有分账记录', '进入服务的订单都已收款', '历史退款可归入日报表']
      .every((s) => finHtml.includes(s)));
  check('财务对账：日表有表头与合计行', finHtml.includes('admin-day-table') && finHtml.includes('<tfoot>'));
  check('财务对账：日表列齐（收款/退款/佣金/专家/打款/净现金）',
    ['已结算佣金', '专家实收', '提现打款', '净现金'].every((s) => finHtml.includes(s)));
  check('财务对账：有导出入口与口径说明',
    finHtml.includes('data-export') && finHtml.includes('导出 CSV') && finHtml.includes('各自归日'));
  check('财务对账：日表行数等于区间天数（默认 30）',
    (finHtml.match(/<tr class="(?:is-muted)?">/g) || []).length === 30,
    String((finHtml.match(/<tr class="(?:is-muted)?">/g) || []).length));
  check('财务对账：不提供删除或修改（只读报表）',
    !finHtml.includes('data-delete') && !finHtml.includes('删除记录'));

  /* 系统配置：抽佣是页面上唯一会动钱的操作，说明文字必须把"改了以后算谁的账"说死。 */
  const setHtml = await renderPage('后台系统配置', () => admin.adminSettingsPage(ctx()));
  check('系统配置：抽佣卡片与调整入口都在',
    setHtml.includes('平台抽佣') && setHtml.includes('data-edit-commission'));
  check('系统配置：说明只影响之后创建的订单', setHtml.includes('只作用于之后创建的订单'));
  check('系统配置：分类表含文章数列', setHtml.includes('文章数'));
  check('系统配置：技能按类目分组',
    setHtml.includes('setting-group') && setHtml.includes('技能字典'));
  check('系统配置：技能行显示引用数', setHtml.includes('引用'));
  check('系统配置：删除说明指向停用', setHtml.includes('不能删') && setHtml.includes('停用'));

  // 后台工具列表按 sort DESC 排，题量补全后新加的工具排在最前、而且它们是「一本教程集
  // 带目录、散装教程留空」的，第一条很可能一行教程都没有。所以先按 guides 接口挑一个
  // 真有教程行的工具再验——否则这项会因为「行 0」假红，跟 CSS 那个 bug 完全无关。
  let firstTool = null;
  for (const t of ((await api.adminTools({ pageSize: 12 }).catch(() => ({ list: [] }))).list || [])) {
    const gs = await api.toolGuides(t.id).catch(() => []);
    if (Array.isArray(gs) && gs.length) { firstTool = t; break; }
  }
  if (firstTool) {
    const guidesHtml = await renderPage('后台教程维护',
      () => admin.adminToolGuidesPage(ctx(), String(firstTool.id)));
    check('教程维护：四类教程分节都在',
      ['部署教程', '实施教程', '优化教程', '集群构建'].every((s) => guidesHtml.includes(s)));
    check('教程维护：站内文章与外链二选一', guidesHtml.includes('站内文章') && guidesHtml.includes('外部链接'));
    // 每行「文章 / 链接」只能有一侧可见。曾经两边同时显示：`.field{display:flex}`
    // 压过了 UA 的 [hidden]{display:none}，栅格多出一列、整行挤到第二行。
    const rowCount = (guidesHtml.match(/class="admin-guide-row"/g) || []).length;
    const hiddenCount = (guidesHtml.match(/data-f-(?:article|url) hidden/g) || []).length;
    check('教程维护：每行只有一侧输入可见', rowCount > 0 && hiddenCount === rowCount,
      `行 ${rowCount} / hidden ${hiddenCount}`);
  } else {
    check('教程维护：四类教程分节都在', false, '前 12 个工具都没有教程行');
    check('教程维护：站内文章与外链二选一', false, '前 12 个工具都没有教程行');
    check('教程维护：每行只有一侧输入可见', false, '前 12 个工具都没有教程行');
  }

  /* ------------------------ 后台：系统教程集管理 -- */

  // 列表页与编辑页都验：只测列表等于没测大纲编辑，而后者才是运营真正操作的地方。
  if (bookTool) {
    const toolIdStr = String(bookTool.detail.id);
    const adminBooksHtml = await renderPage('后台教程集列表',
      () => admin.adminToolBooksPage(ctx(), toolIdStr));
    check('教程集管理：列表页有新建入口与状态列',
      adminBooksHtml.includes('data-book-new') && adminBooksHtml.includes('状态'));
    check('教程集管理：列表页提示与四类教程的分工',
      adminBooksHtml.includes('散装好文') || adminBooksHtml.includes('学习路径'));

    const adminBooks = await api.adminToolBooks(toolIdStr).catch(() => []);
    check('教程集管理：后台能读到目录树',
      Array.isArray(adminBooks) && adminBooks.length > 0,
      `${Array.isArray(adminBooks) ? adminBooks.length : 0} 本`);

    const editTarget = Array.isArray(adminBooks) ? adminBooks[0] : null;
    if (editTarget) {
      const editHtml = await renderPage('后台教程集编辑',
        () => admin.adminToolBooksPage(ctx(`book=${editTarget.id}`), toolIdStr));
      check('教程集编辑：给出大纲格式说明与编辑框',
        editHtml.includes('id="bk-outline"') && editHtml.includes('slug'));
      check('教程集编辑：目录预览渲染章与节',
        editHtml.includes('book-chapter') && editHtml.includes('book-sec'));
      check('教程集编辑：有保存目录与发布/下线按钮',
        editHtml.includes('data-save-outline') && editHtml.includes('data-book-state'));
    } else {
      check('教程集编辑：给出大纲格式说明与编辑框', false, '没有可编辑的教程集');
      check('教程集编辑：目录预览渲染章与节', false, '没有可编辑的教程集');
      check('教程集编辑：有保存目录与发布/下线按钮', false, '没有可编辑的教程集');
    }
  } else {
    check('教程集管理：列表页有新建入口与状态列', false, '没有工具挂教程集');
    check('教程集管理：列表页提示与四类教程的分工', false, '没有工具挂教程集');
    check('教程集管理：后台能读到目录树', false, '没有工具挂教程集');
    check('教程集编辑：给出大纲格式说明与编辑框', false, '没有工具挂教程集');
    check('教程集编辑：目录预览渲染章与节', false, '没有工具挂教程集');
    check('教程集编辑：有保存目录与发布/下线按钮', false, '没有工具挂教程集');
  }

  const linkRes = ((await api.adminResources({ pageSize: 50 }).catch(() => ({ list: [] }))).list || [])
    .find((r) => !r.hasFile);
  if (linkRes) {
    const linksHtml = await renderPage('后台外链维护',
      () => admin.adminResourceLinksPage(ctx(), String(linkRes.id)));
    check('外链维护：能看出这份资源有没有托管文件',
      linksHtml.includes('只有外链') || linksHtml.includes('同时有托管文件'));
  } else {
    check('外链维护：能看出这份资源有没有托管文件', false, '没有仅外链的资源');
  }

  // 入驻表单要求「登录但还没申请」。管理员账号正好符合这个条件
  // （admin 没有专家记录），所以不用再单独造一个账号。
  const expertApplyHtml = await renderPage('入驻申请（已登录未申请）', () => expertMod.expertApplyPage(ctx()));
  const applySections = (expertApplyHtml.match(/class="apply-section"/g) || []).length;
  check('入驻申请：四个分段齐全', applySections === 4, `sections=${applySections}`);
  check('入驻申请：资质上传与报价字段都在',
    expertApplyHtml.includes('data-cert-file') &&
    expertApplyHtml.includes('id="ap-hour"') && expertApplyHtml.includes('id="ap-task"'));
  check('入驻申请：证件号字段允许留空（沿用已登记号码）',
    expertApplyHtml.includes('id="ap-idcard"'));

  const expertWbHtml = await renderPage('专家工作台（未申请）', () => expertMod.expertWorkbenchPage(ctx()));
  check('专家工作台：未申请时引导去提交申请',
    expertWbHtml.includes('还没有通过认证') && expertWbHtml.includes('#/expert/apply'));
}

/* ------------------------------------------- 成长路径 / 值班巡检（第四批） -- */

const pathsMod = await load('pages-paths.js');
const pathsData = await load('paths-data.js');

const pathsHtml = await renderPage('成长路径总览', () => pathsMod.pathsPage(ctx('')));
check('成长路径：六条路线全部列出',
  pathsData.PATHS.length === 6 && pathsData.PATHS.every((p) => pathsHtml.includes(p.name)),
  `${pathsData.PATHS.length} 条`);
check('成长路径：总览给总进度与入口',
  pathsHtml.includes('pt-ring') && pathsHtml.includes('pt-ov-btn'));
// 进度只在本机这件事必须写出来。不写就是默认用户以为云同步，
// 换台电脑发现进度没了会怀疑平台丢数据。
check('成长路径：明确说明进度不跨设备同步',
  pathsHtml.includes('换设备不会同步'));
check('成长路径：卡片链接到各自的详情',
  pathsData.PATHS.every((p) => pathsHtml.includes(`#/paths/${p.id}`)));

const firstPath = pathsData.PATHS[0];
const pathDetail = await renderPage('成长路径详情', () => pathsMod.pathDetailPage(ctx(''), firstPath.id));
check('成长路径：详情渲染出全部阶段',
  firstPath.stages.every((s) => pathDetail.includes(s.name)),
  `${firstPath.stages.length} 个阶段`);
check('成长路径：每个阶段都有关过标准',
  pathDetail.includes('过关标准') && firstPath.stages.every((s) => pathDetail.includes(s.goal)));
check('成长路径：首个未完成阶段被标为当前阶段',
  pathDetail.includes('当前阶段') && pathDetail.includes('is-current'));
check('成长路径：资源链接全部指向站内路由',
  pathsData.PATHS.every((p) => p.stages.every((s) => s.resources.every((r) => r.href.startsWith('#/')))));

// 跳过的阶段要能记住：写入本地存储后重渲染，百分比与「已完成」徽章都要跟着变。
localStorage.setItem('opskb.paths.v1', JSON.stringify({ [firstPath.id]: { done: [firstPath.stages[0].id] } }));
const pathAfter = await renderPage('成长路径详情（已完成 1 阶段）',
  () => pathsMod.pathDetailPage(ctx(''), firstPath.id));
check('成长路径：勾选后进度与徽章同步更新',
  pathAfter.includes('已完成') && pathAfter.includes('is-done'));
check('成长路径：进度统计与阶段数一致',
  pathsMod.pathStats(firstPath).percent === Math.round((1 / firstPath.stages.length) * 100),
  `${pathsMod.pathStats(firstPath).percent}%`);
localStorage.removeItem('opskb.paths.v1');

const pathMissing = await renderPage('成长路径（不存在的 id）',
  () => pathsMod.pathDetailPage(ctx(''), 'not-a-path'));
check('成长路径：未知路线给引导不抛错', pathMissing.includes('路线不存在'));

// 首页的「从这里开始」是本批功能唯一的突出位置，位置与内容都要守：
// 内容块挂在栏目卡之后，否则它会和 hero 抢首屏。
const hpStart = homeHtml.indexOf('hp-path');
check('首页：成长路径引导块存在且位于栏目卡之后',
  hpStart > homeHtml.indexOf('channel-grid') && hpStart > 0);
check('首页：四个高频工具入口齐全',
  ['速查手册', '排障剧本', '值班巡检', '模拟考'].every((s) => homeHtml.includes(s))
    && (homeHtml.match(/hp-quick-item/g) || []).length === 4);

const clMod = await load('pages-checklist.js');
const clData = await load('checklist-data.js');

const clHtml = await renderPage('值班巡检（默认日检）', () => clMod.checklistPage(ctx('')));
check('值班巡检：三档清单都可切换',
  ['日检', '周检', '月检'].every((s) => clHtml.includes(s)) && clHtml.includes('data-cl-tab'));
check('值班巡检：日检项数与数据一致',
  clData.itemCountOf(clData.CHECKLISTS[0]) === 12, `${clData.itemCountOf(clData.CHECKLISTS[0])} 项`);
check('值班巡检：每项都带可复制命令', (clHtml.match(/data-copy=/g) || []).length
  === clData.itemCountOf(clData.CHECKLISTS[0]));
// 只有命令没有判定标准，清单就退化成速查表的复制品——这条守的是"有判定"
check('值班巡检：每项都写了判定标准',
  (clHtml.match(/class="cl-expect"/g) || []).length === clData.itemCountOf(clData.CHECKLISTS[0]));
check('值班巡检：必检项有醒目标记', clHtml.includes('cl-crit') && clHtml.includes('必检'));
check('值班巡检：未勾完时提示剩余必检项',
  clHtml.includes('cl-hero-alert') && clHtml.includes('个必检项没勾'));
check('值班巡检：提供导出交接与清空', clHtml.includes('data-cl-export') && clHtml.includes('data-cl-reset'));

const clWeekly = await renderPage('值班巡检（周检）', () => clMod.checklistPage(ctx('t=weekly')));
check('值班巡检：切档渲染的是另一套项',
  clWeekly !== clHtml && clWeekly.includes('周检')
    && clWeekly.includes(clData.CHECKLISTS[1].groups[0].items[0].name));
check('值班巡检：未知档位回落到日检',
  (await renderPage('值班巡检（未知档）', () => clMod.checklistPage(ctx('t=nope')))).includes('日检'));

/* ---------------------------------------------- 积分商城 / 开发者页（第五批） -- */

const mallMod = await load('pages-mall.js');
const devMod = await load('pages-developer.js');

// 本节运行时 api.js 已带登录态（见「投稿与审核」节的说明），
// 所以按登录形态断言；游客形态的分流由 e2e 覆盖。
const mallAuthed = await renderPage('积分商城（登录态）', () => mallMod.mallPage(ctx('')));
check('积分商城：登录后显示余额与记录入口',
  mallAuthed.includes('当前可用积分') && mallAuthed.includes('#/mall/history'));
check('积分商城：货架与空态二选一',
  mallAuthed.includes('mall-grid') || mallAuthed.includes('货架整理中'));

const mallHist = await renderPage('兑换记录（登录态）', () => mallMod.mallHistoryPage(ctx('')));
check('兑换记录：登录后渲染列表或空态',
  mallHist.includes('mall-history') || mallHist.includes('还没有兑换过'));

const devAuthed = await renderPage('开发者页（登录态）', () => devMod.developerPage(ctx('')));
check('开发者页：登录后给新建密钥动作',
  devAuthed.includes('新建密钥') && devAuthed.includes('data-act="create-key"'));
check('开发者页：接口速览与限流说明齐全',
  devAuthed.includes('/open/v1/ping') && devAuthed.includes('dev-table') && devAuthed.includes('X-RateLimit-Remaining'));

// 导航与我的页入口守开关（直接读源文件，别走 fetch——file:// 进不了桩）。
const { readFileSync } = await import('node:fs');
const srcOf = (name) => readFileSync(new URL(`../web/static/assets/js/${name}`, import.meta.url), 'utf8');
const appSrc = srcOf('app.js');
check('导航：积分商城入口受开关守卫',
  appSrc.includes("FEATURES.mall") && appSrc.includes("'积分商城'"));
check('路由：mall/developer/admin-mall 四条路由都已注册',
  ['mallPage', 'mallHistoryPage', 'developerPage', 'adminMallPage'].every((fn) => appSrc.includes(fn)));

const meSrc = srcOf('pages-me.js');
check('我的页：商城与开发者入口受开关与权限分流',
  meSrc.includes('utilityCardsHTML') && meSrc.includes('#/mall') && meSrc.includes('#/developer'));

/* ------------------------------------------------------------ 汇总 -- */

let bad = 0;
for (const r of rows) {
  if (!r.ok) bad++;
  console.log(`${r.ok ? '✓' : '✗'} ${r.name}${r.detail ? `  — ${r.detail}` : ''}`);
}
console.log(`\n${rows.length - bad}/${rows.length} 项通过`);
process.exit(bad ? 1 : 0);
