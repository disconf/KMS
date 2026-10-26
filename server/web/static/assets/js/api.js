/* ============================================================================
   api.js — 后端接口封装、登录态与会话续期。
   业务代码只跟 api.xxx() 打交道，不直接碰 fetch。
   ========================================================================= */

import { qs } from './util.js';

const BASE = '/api/v1';
const STORAGE_KEY = 'opskb.session.v1';

/** 业务错误。带上业务码，调用方可以据此分支（例如 40031 引导登录）。 */
export class ApiError extends Error {
  constructor(code, message, status) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
  }
}

/* ------------------------------------------------------------- 会话 -- */

let session = readSession();
const listeners = new Set();

function readSession() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    return parsed && parsed.accessToken ? parsed : null;
  } catch {
    return null;
  }
}

function writeSession(next) {
  session = next;
  try {
    if (next) localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
    else localStorage.removeItem(STORAGE_KEY);
  } catch {
    // 隐私模式下 localStorage 可能不可写，退化为内存态即可，不必打扰用户
  }
  listeners.forEach((fn) => fn(session));
}

/** 订阅登录态变化，返回取消订阅函数。 */
export function onAuthChange(fn) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

export function currentUser() { return session ? session.user : null; }
export function isLoggedIn() { return Boolean(session && session.accessToken); }

/**
 * 当前登录者是不是管理员。
 *
 * 只用来决定「要不要显示后台入口」，不是权限判断——真正的拦截在后端中间件里。
 * 前端在这里多显示一个按钮，最坏结果是一次 403 提示，不会造成越权。
 */
export function isAdmin() {
  const user = session ? session.user : null;
  return Boolean(user && user.role === 'admin');
}

/* ------------------------------------------------------------- 请求 -- */

/**
 * 统一请求。
 *
 * 访问令牌只有 2 小时，用户开着页面很可能正好在读取过程中过期。
 * 这里遇到 401 会先静默续期再重试一次，避免把"请重新登录"这种
 * 本可自愈的提示推给用户；续期也失败才真的清空登录态。
 */
async function request(method, path, { body, auth = true, retry = true } = {}) {
  const headers = {};
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (auth && session && session.accessToken) {
    headers.Authorization = `Bearer ${session.accessToken}`;
  }

  let res;
  try {
    res = await fetch(BASE + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (err) {
    throw new ApiError(-1, '网络异常，请检查服务是否已启动', 0);
  }

  const text = await res.text();
  let payload = null;
  if (text) {
    try { payload = JSON.parse(text); } catch { payload = null; }
  }

  if (res.ok && payload && payload.code === 0) return payload.data;

  if (res.status === 401 && auth && retry && session && session.refreshToken) {
    const renewed = await renewSession();
    if (renewed) return request(method, path, { body, auth, retry: false });
  }

  const code = payload && typeof payload.code === 'number' ? payload.code : res.status;
  const message = (payload && payload.message) || `请求失败（HTTP ${res.status}）`;

  if (res.status === 401) writeSession(null);

  // 账号被封禁（403 + 30024）。这不是"没权限"，重新登录也解决不了，
  // 所以直接清掉本地会话——否则被封的人会顶着一枚永远 403 的令牌反复刷新，
  // 每个页面都只弹一句红字，看不出到底发生了什么。
  if (code === 30024) writeSession(null);

  throw new ApiError(code, message, res.status);
}

const get  = (path) => request('GET', path);
const post = (path, body) => request('POST', path, { body });
const put  = (path, body) => request('PUT', path, { body });
// 删除接口一律不带请求体：内容 ID 在路径里，请求体带 ID 只会多一种对不上的可能。
const del  = (path) => request('DELETE', path);

/**
 * 带登录态下载文件。
 *
 * 浏览器不会给 <a href> 带 Authorization 头，所以不能把接口地址直接塞进 href；
 * 把令牌拼到 query 里更糟——它会进浏览器历史、进网关访问日志，等于把令牌公开。
 * 只能 fetch 成 blob 再触发下载。
 *
 * token 过期时先续期再重试一次：导出通常发生在用户已经看了几眼报表之后，
 * 这时令牌刚好到期很常见，直接弹"请重新登录"会让人丢掉刚选的日期区间。
 */
export async function downloadFile(path, fallbackName = 'download.bin') {
  const headers = {};
  if (session && session.accessToken) headers.Authorization = `Bearer ${session.accessToken}`;

  const res = await fetch(BASE + path, { headers });

  if (res.status === 401 && session && session.refreshToken) {
    const renewed = await renewSession();
    if (renewed) return downloadFile(path, fallbackName);
  }
  if (!res.ok) {
    // 失败时后端返回的是 JSON，把业务提示读出来给用户看——
    // 否则他只会拿到一段空白或一个坏文件，完全不知道是哪一步错了。
    let msg = `下载失败（HTTP ${res.status}）`;
    try {
      const payload = await res.json();
      if (payload && payload.message) msg = payload.message;
    } catch {
      // 不是 JSON，保留兜底文案
    }
    throw new ApiError(res.status, msg, res.status);
  }

  const disposition = res.headers.get('Content-Disposition') || '';
  const matched = /filename="?([^";]+)"?/.exec(disposition);
  const name = matched ? matched[1] : fallbackName;

  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  // 立刻 revoke 会让部分浏览器的下载中断，延后一拍释放就够了
  setTimeout(() => URL.revokeObjectURL(url), 2000);
  return name;
}

/** 续期访问令牌。并发请求共用同一次续期，避免同时打出多个刷新请求。 */
let renewing = null;
function renewSession() {
  if (renewing) return renewing;
  renewing = (async () => {
    try {
      const res = await fetch(`${BASE}/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refreshToken: session.refreshToken }),
      });
      const payload = await res.json().catch(() => null);
      if (!res.ok || !payload || payload.code !== 0) return false;
      const data = payload.data;
      writeSession({
        accessToken: data.token.accessToken,
        refreshToken: data.token.refreshToken,
        user: data.user,
      });
      return true;
    } catch {
      return false;
    } finally {
      renewing = null;
    }
  })();
  return renewing;
}

/* ------------------------------------------------------------- 认证 -- */

/** 发送验证码。开发环境服务端会把验证码放在 devCode 里回传。 */
export function sendSMSCode(phone, scene = 'login') {
  return request('POST', '/auth/sms/send', { body: { phone, scene }, auth: false });
}

export async function loginBySMS(phone, code, nickname) {
  const data = await request('POST', '/auth/login/sms', {
    body: { phone, code, nickname: nickname || undefined },
    auth: false,
  });
  writeSession({
    accessToken: data.token.accessToken,
    refreshToken: data.token.refreshToken,
    user: data.user,
  });
  return data;
}

/**
 * 手机号 + 密码登录。只对已设置密码的账号有效（管理员/运营）。
 * 普通用户没有密码，仍然走验证码——失败文案统一是"手机号或密码不正确"，
 * 前端不要替服务端解释得更详细。
 */
export async function loginByPassword(phone, password) {
  const data = await request('POST', '/auth/login/password', {
    body: { phone, password },
    auth: false,
  });
  writeSession({
    accessToken: data.token.accessToken,
    refreshToken: data.token.refreshToken,
    user: data.user,
  });
  return data;
}

/** 修改自己的登录密码。 */
export function changePassword(oldPassword, newPassword) {
  return request('POST', '/me/password', { body: { oldPassword, newPassword } });
}

export async function logout() {
  const refreshToken = session && session.refreshToken;
  writeSession(null);
  try {
    await request('POST', '/auth/logout', { body: { refreshToken }, retry: false });
  } catch {
    // 登出以本地清干净为准：服务端吊销失败不该把用户困在"已登出但提示失败"的状态里
  }
}

/** 用本地会话换取最新的用户资料，同时验证令牌是否仍然有效。 */
export async function refreshProfile() {
  if (!isLoggedIn()) return null;
  try {
    const user = await request('GET', '/me');
    writeSession({ ...session, user });
    return user;
  } catch {
    return null;
  }
}

/**
 * 主动续期，把令牌里的 role / expertStatus 换成最新的。
 *
 * 这两个字段是**签在令牌里**的（见 middleware.Auth），服务端改库不会影响
 * 已经签发的那一枚。成为专家、审核通过之后如果不多换一次令牌，
 * 专家接口会一直按旧身份判定，表现为「明明通过了却还是 403」。
 * 返回是否换成功——调用方不必因为它失败而中断后面的流程。
 */
export async function refreshSession() {
  if (!session || !session.refreshToken) return false;
  return renewSession();
}

/* ------------------------------------------------------------- 上传 -- */

/**
 * 上传文件，返回 { key, url, private, ... }。引用文件时传 key，不要存 url。
 *
 * 单独一条 fetch 通道：FormData 的 Content-Type 必须由浏览器自己带 boundary，
 * 走 request() 那套 JSON 头会把请求写坏。同样不重试——上传失败重来一次
 * 通常只是把同一份大文件再传一遍，让用户自己决定更合适。
 */
export async function uploadFile(scene, file) {
  const form = new FormData();
  form.append('scene', scene);
  form.append('file', file);

  let res;
  try {
    res = await fetch(`${BASE}/upload`, {
      method: 'POST',
      headers: session && session.accessToken
        ? { Authorization: `Bearer ${session.accessToken}` } : {},
      body: form,
    });
  } catch {
    throw new ApiError(-1, '网络异常，文件没有传上去', 0);
  }

  const payload = await res.json().catch(() => null);
  if (res.ok && payload && payload.code === 0) return payload.data;
  if (res.status === 401) writeSession(null);
  const code = payload && typeof payload.code === 'number' ? payload.code : res.status;
  const message = (payload && payload.message) || `上传失败（HTTP ${res.status}）`;
  throw new ApiError(code, message, res.status);
}

/* ------------------------------------------------------ 知识库接口 -- */

/**
 * 有些接口把数组包在 `{list: [...]}` 里（栏目、文件类型），有些直接返回数组。
 * 这里统一拆一层，让调用方拿到的永远是数组——否则每个调用点都得自己判断
 * "这次是数组还是对象"，迟早有人漏判，然后表现为一句
 * `xxx.filter is not a function`。
 */
const unwrapList = (data) => {
  if (Array.isArray(data)) return data;
  if (data && Array.isArray(data.list)) return data.list;
  return [];
};

export const api = {
  /* 上传场景（公开）：前端据此给 accept 与大小提示，避免和策略表写岔 */
  uploadScenes: () => get('/upload/scenes').then(unwrapList),

  /* 栏目与文章 */
  channels:   () => get('/channels').then(unwrapList),
  categories: () => get('/kb/categories').then(unwrapList),
  tags:       () => get('/kb/tags').then(unwrapList),
  articles:   (query) => get('/kb/articles' + qs(query)),
  article:    (key) => get(`/kb/articles/${encodeURIComponent(key)}`),
  toggleLike: (key) => post(`/kb/articles/${encodeURIComponent(key)}/like`),
  // 把用户的问句翻译成检索词，并附热门词。失败时调用方要能降级——
  // 建议是锦上添花，不该因为它挂了就让搜索框不可用。
  searchSuggest: (q) => get('/kb/search/suggest' + qs({ q })),

  /* 投稿 */
  // 投稿落库固定为「待审核」，请求里没有 status 字段——是否上线由运营决定，
  // 让客户端传状态等于把发布权限交给了浏览器。
  contributeArticle: (payload) => post('/kb/articles/contribute', payload),
  myContributions:   (query) => get('/me/contributions' + qs(query)),

  // 题目投稿。与文章投稿是两个接口而不是一个带 kind 的接口：
  // 两者的字段完全不同（一个是正文，一个是选项与答案），
  // 合成一个联合体后服务端和前端都要写一堆分支去猜类型。
  contributeQuiz:   (payload) => post('/quiz/contribute', payload),
  myQuizContributions: (query) => get('/quiz/my/contributions' + qs(query)),

  /* 问答广场 */
  // 游客可读：问答是搜索引擎最常落地的页面，拦在登录后面等于放弃这块流量。
  qaQuestions: (query) => get('/qa/questions' + qs(query)),
  qaQuestion:  (id) => get(`/qa/questions/${encodeURIComponent(id)}`),
  askQuestion: (payload) => post('/qa/questions', payload),
  deleteQuestion: (id) => del(`/qa/questions/${encodeURIComponent(id)}`),
  answerQuestion: (id, payload) => post(`/qa/questions/${encodeURIComponent(id)}/answers`, payload),
  acceptAnswer:   (qid, aid) => post(`/qa/questions/${encodeURIComponent(qid)}/accept/${encodeURIComponent(aid)}`),
  closeQuestion:  (id) => post(`/qa/questions/${encodeURIComponent(id)}/close`),
  updateAnswer:   (id, payload) => put(`/qa/answers/${encodeURIComponent(id)}`, payload),
  deleteAnswer:   (id) => del(`/qa/answers/${encodeURIComponent(id)}`),
  likeAnswer:     (id) => post(`/qa/answers/${encodeURIComponent(id)}/like`),
  unlikeAnswer:   (id) => del(`/qa/answers/${encodeURIComponent(id)}/like`),
  myPoints:       () => get('/qa/my/points'),

  /* 开源工具 */
  tools:       (query) => get('/tools' + qs(query)),
  toolFilters: () => get('/tools/filters'),
  tool:        (key) => get(`/tools/${encodeURIComponent(key)}`),
  // 教程集（可折叠目录）。正文不在这里取——每一节都是普通文章，
  // 走 /kb/articles/:slug，复用同一套阅读页。
  toolBook:    (id) => get(`/tool-books/${encodeURIComponent(id)}`),

  /* 资源下载 */
  resources:    (query) => get('/resources' + qs(query)),
  resourceExts: () => get('/resources/file-exts').then(unwrapList),
  resource:     (key) => get(`/resources/${encodeURIComponent(key)}`),
  resourceDownload: (key) => get(`/resources/${encodeURIComponent(key)}/download`),

  /* 题库 */
  quizCategories: () => get('/quiz/categories').then(unwrapList),
  quizQuestions: (query) => get('/quiz/questions' + qs(query)),
  practice:      (query) => get('/quiz/practice' + qs(query)).then((data) => ({
    list: Array.isArray(data && data.list) ? data.list : [],
  })),
  answer:        (id, chosen) => post(`/quiz/questions/${id}/answer`, { chosen }),
  submitAttempt: (payload) => post('/quiz/attempts', payload),
  attempts:      (query) => get('/quiz/attempts' + qs(query)),
  quizStats:     () => get('/quiz/stats'),
  wrongBook:     (query) => get('/quiz/wrong-book' + qs(query)),

  /* 悬赏与订单 */
  // 技能是发布悬赏时必填的维度（服务端 min=1），所以表单要先把清单拿到
  skills: () => get('/skills').then(unwrapList),

  createQuestion: (payload) => post('/questions', payload),
  myQuestions:    (query) => get('/questions' + qs(query)),
  question:       (id) => get(`/questions/${encodeURIComponent(id)}`),
  cancelQuestion: (id, payload) => post(`/questions/${encodeURIComponent(id)}/cancel`, payload || {}),

  orders:      (query) => get('/orders' + qs(query)),
  orderCounts: (side) => get('/orders/counts' + qs({ side })),
  order:       (orderNo) => get(`/orders/${encodeURIComponent(orderNo)}`),
  // 通道清单里带 available，前端据此置灰而不是隐藏——"以后会有"和"本来就不支持"
  // 对用户是两种不同的信息。
  payChannels: () => get('/orders/pay-channels').then(unwrapList),
  pay:         (orderNo, payload) => post(`/orders/${encodeURIComponent(orderNo)}/pay`, payload),
  cancelOrder: (orderNo, payload) => post(`/orders/${encodeURIComponent(orderNo)}/cancel`, payload || {}),
  confirmOrder: (orderNo) => post(`/orders/${encodeURIComponent(orderNo)}/confirm`),
  finishOrder: (orderNo) => post(`/orders/${encodeURIComponent(orderNo)}/finish`),

  // 会话通道：im / voice / video / screen，由服务端白名单校验
  startSession: (orderNo, channel) => post(`/orders/${encodeURIComponent(orderNo)}/sessions`, { channel }),
  endSession:   (orderNo) => post(`/orders/${encodeURIComponent(orderNo)}/sessions/end`),

  reviewOrder: (orderNo, payload) => post(`/orders/${encodeURIComponent(orderNo)}/review`, payload),
  replyReview: (orderNo, payload) => post(`/orders/${encodeURIComponent(orderNo)}/review/reply`, payload),
  appealReview: (orderNo, payload) => post(`/orders/${encodeURIComponent(orderNo)}/review/appeal`, payload),
  raiseDispute: (orderNo, payload) => post(`/orders/${encodeURIComponent(orderNo)}/dispute`, payload),

  hall: (query) => get('/orders/hall' + qs(query)),
  grab: (orderNo) => post(`/orders/${encodeURIComponent(orderNo)}/grab`),

  /* ------------------------------------------------------------ 专家 -- */
  // 三层分别是：游客可读（列表/详情）、登录可用（申请与状态）、
  // 审核通过才可用（服务设置/主页/钱包）。前端只按状态摆界面，
  // 真正的拦截在服务端的 RequireApprovedExpert。
  experts:  (query) => get('/experts' + qs(query)),
  expert:   (id) => get(`/experts/${encodeURIComponent(id)}`),
  // 评价与详情分开拉：首屏拿详情不被评价条数拖慢，翻页只请求这一页。
  expertReviews: (id, query) => get(`/experts/${encodeURIComponent(id)}/reviews` + qs(query)),
  expertApplyStatus: () => get('/expert/apply'),
  applyExpert:       (payload) => post('/expert/apply', payload),
  // 工作台只要求角色，不要求审核通过——审核中/被驳回的人也要能进来看横幅
  expertDashboard:   () => get('/expert/dashboard'),
  updateExpertServices: (payload) => put('/expert/services', payload),
  updateExpertProfile:  (payload) => put('/expert/profile', payload),
  expertWallet:         () => get('/expert/wallet'),
  expertWithdrawals:    () => get('/expert/withdrawals').then(unwrapList),
  createWithdrawal:     (payload) => post('/expert/withdrawals', payload),

  /* ---------------------------------------------------------- 后台 -- */
  // 后台接口全部走 /admin 前缀，服务端用 RequireRole(admin) 把住。
  // 前端这里的 role 判断只决定「入口摆不摆」，不是权限——改一行 JS 绕不过服务端。
  adminArticles:  (query) => get('/admin/kb/articles' + qs(query)),
  adminArticle:   (id) => get(`/admin/kb/articles/${encodeURIComponent(id)}`),
  createArticle:  (payload) => post('/admin/kb/articles', payload),
  updateArticle:  (id, payload) => put(`/admin/kb/articles/${encodeURIComponent(id)}`, payload),
  deleteArticle:  (id) => del(`/admin/kb/articles/${encodeURIComponent(id)}`),
  publishArticle: (id) => post(`/admin/kb/articles/${encodeURIComponent(id)}/publish`),
  offlineArticle: (id) => post(`/admin/kb/articles/${encodeURIComponent(id)}/offline`),
  reviewArticle:  (id, payload) => post(`/admin/kb/articles/${encodeURIComponent(id)}/review`, payload),

  adminCategories: (query) => get('/admin/kb/categories' + qs(query)).then(unwrapList),
  createCategory:  (payload) => post('/admin/kb/categories', payload),
  updateCategory:  (id, payload) => put(`/admin/kb/categories/${encodeURIComponent(id)}`, payload),
  deleteCategory:  (id) => del(`/admin/kb/categories/${encodeURIComponent(id)}`),

  adminTools:   (query) => get('/admin/kb/tools' + qs(query)),
  adminTool:    (id) => get(`/admin/kb/tools/${encodeURIComponent(id)}`),
  publishTool:  (id) => post(`/admin/kb/tools/${encodeURIComponent(id)}/publish`),
  offlineTool:  (id) => post(`/admin/kb/tools/${encodeURIComponent(id)}/offline`),
  deleteTool:   (id) => del(`/admin/kb/tools/${encodeURIComponent(id)}`),
  toolGuides:   (id) => get(`/admin/kb/tools/${encodeURIComponent(id)}/guides`).then(unwrapList),
  setToolGuides: (id, guides) => put(`/admin/kb/tools/${encodeURIComponent(id)}/guides`, { guides }),

  adminResources:  (query) => get('/admin/kb/resources' + qs(query)),
  adminResource:   (id) => get(`/admin/kb/resources/${encodeURIComponent(id)}`),
  publishResource: (id) => post(`/admin/kb/resources/${encodeURIComponent(id)}/publish`),
  offlineResource: (id) => post(`/admin/kb/resources/${encodeURIComponent(id)}/offline`),
  deleteResource:  (id) => del(`/admin/kb/resources/${encodeURIComponent(id)}`),
  resourceLinks:   (id) => get(`/admin/kb/resources/${encodeURIComponent(id)}/links`).then(unwrapList),
  setResourceLinks: (id, links) => put(`/admin/kb/resources/${encodeURIComponent(id)}/links`, { links }),

  adminQuestions:  (query) => get('/admin/kb/quiz/questions' + qs(query)),
  publishQuestion: (id) => post(`/admin/kb/quiz/questions/${encodeURIComponent(id)}/publish`),
  offlineQuestion: (id) => post(`/admin/kb/quiz/questions/${encodeURIComponent(id)}/offline`),
  deleteQuestion:  (id) => del(`/admin/kb/quiz/questions/${encodeURIComponent(id)}`),
  // 批量导入返回 200 + {imported, errors}：错误清单是要给用户逐行看的数据，
  // 塞进错误消息里只能带出第一条。
  importQuestions:  (payload) => post('/admin/kb/quiz/questions/import', payload),
  reviewQuestion:   (id, payload) => post(`/admin/kb/quiz/questions/${encodeURIComponent(id)}/review`, payload),

  // 教程集：书挂在工具下，章节用大纲文本整体替换。
  adminToolBooks:  (toolId) => get(`/admin/kb/tools/${encodeURIComponent(toolId)}/books`).then(unwrapList),
  createToolBook:  (toolId, payload) => post(`/admin/kb/tools/${encodeURIComponent(toolId)}/books`, payload),
  adminToolBook:   (id) => get(`/admin/kb/tool-books/${encodeURIComponent(id)}`),
  updateToolBook:  (id, payload) => put(`/admin/kb/tool-books/${encodeURIComponent(id)}`, payload),
  deleteToolBook:  (id) => del(`/admin/kb/tool-books/${encodeURIComponent(id)}`),
  publishToolBook: (id) => post(`/admin/kb/tool-books/${encodeURIComponent(id)}/publish`),
  offlineToolBook: (id) => post(`/admin/kb/tool-books/${encodeURIComponent(id)}/offline`),
  toolBookOutline: (id) => get(`/admin/kb/tool-books/${encodeURIComponent(id)}/outline`),
  saveToolBookOutline: (id, outline) => put(`/admin/kb/tool-books/${encodeURIComponent(id)}/outline`, { outline }),

  adminExperts: (query) => get('/admin/experts' + qs(query)),
  auditExpert:  (id, payload) => post(`/admin/experts/${encodeURIComponent(id)}/audit`, payload),

  // 提现审批。审核与打款是两个动作：通过只是同意提，钱要等线下转账后才扣冻结。
  adminWithdrawals:     (query) => get('/admin/withdrawals' + qs(query)),
  adminWithdrawalReview: (id, payload) => post(`/admin/withdrawals/${encodeURIComponent(id)}/review`, payload),
  adminWithdrawalPaid:   (id, payload) => post(`/admin/withdrawals/${encodeURIComponent(id)}/mark-paid`, payload || {}),
  adminWithdrawalFail:   (id, payload) => post(`/admin/withdrawals/${encodeURIComponent(id)}/fail`, payload),

  // 用户治理。封禁/解封用显式 status 而不是布尔：请求体缺字段绝不能变成"反向操作"。
  adminUsers:       (query) => get('/admin/users' + qs(query)),
  adminUser:        (id) => get(`/admin/users/${encodeURIComponent(id)}`),
  adminUserStatus:  (id, payload) => post(`/admin/users/${encodeURIComponent(id)}/status`, payload),
  adminUserRole:    (id, payload) => post(`/admin/users/${encodeURIComponent(id)}/role`, payload),

  // 财务对账。只读：报表不能改数，发现不平要去改真实记录。
  adminFinanceReport: (query) => get('/admin/finance/reconciliation' + qs(query)),
  adminFinanceDay:    (day) => get(`/admin/finance/reconciliation/day/${encodeURIComponent(day)}`),
  adminFinanceExport: (query) => downloadFile(
    '/admin/finance/reconciliation/export' + qs(query), 'finance-reconciliation.csv'),

  // 系统配置。抽佣比例改完立即对之后创建的所有订单生效（订单在下单时拍快照）。
  adminSettings:        () => get('/admin/settings'),
  adminCommissionRate:  (rateBp) => put('/admin/settings/commission', { rateBp }),

  // 技能字典。停用与删除是两件事：被专家引用时只能停用。
  adminSkills:      () => get('/admin/skills').then(unwrapList),
  createSkill:      (payload) => post('/admin/skills', payload),
  updateSkill:      (id, payload) => put(`/admin/skills/${encodeURIComponent(id)}`, payload),
  deleteSkill:      (id) => del(`/admin/skills/${encodeURIComponent(id)}`),

  // 问答运营：待审回答 + 精选转教程草稿。
  adminQaPending:        (query) => get('/admin/qa/answers/pending' + qs(query)),
  adminQaReview:         (id, payload) => post(`/admin/qa/answers/${encodeURIComponent(id)}/review`, payload),
  adminQaDeleteQuestion: (id) => del(`/admin/qa/questions/${encodeURIComponent(id)}`),
  adminQaToArticle:      (id, payload) => post(`/admin/qa/questions/${encodeURIComponent(id)}/to-article`, payload || {}),

  adminOrders:      (query) => get('/admin/orders' + qs(query)),
  adminOrder:       (orderNo) => get(`/admin/orders/${encodeURIComponent(orderNo)}`),
  adminMarkPaid:    (orderNo, payload) => post(`/admin/orders/${encodeURIComponent(orderNo)}/mark-paid`, payload || {}),
  adminTransition:  (orderNo, payload) => post(`/admin/orders/${encodeURIComponent(orderNo)}/transition`, payload),
  adminSettle:      (orderNo) => post(`/admin/orders/${encodeURIComponent(orderNo)}/settle`),

  newsSources:       () => get('/admin/news/sources').then(unwrapList),
  createNewsSource:  (payload) => post('/admin/news/sources', payload),
  updateNewsSource:  (id, payload) => put(`/admin/news/sources/${encodeURIComponent(id)}`, payload),
  deleteNewsSource:  (id) => del(`/admin/news/sources/${encodeURIComponent(id)}`),
  fetchNewsSource:   (id) => post(`/admin/news/sources/${encodeURIComponent(id)}/fetch`),
};
