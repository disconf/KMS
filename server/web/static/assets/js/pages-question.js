/* ============================================================================
   pages-question.js — 发布悬赏。

   这一页的每个约束都来自服务端，前端只是把"错在哪"提前告诉用户：
   标题 5–200 字、描述 ≥10 字、技能至少 1 个最多 5 个、预算 10–5000 元、
   有效期 1–168 小时。数字写在这里是为了即时反馈，**不是校验权威**——
   服务端仍会再判一次，改这里不会放宽任何规则。

   预算用「元」输入、按「分」提交：用户填 200，接口收到 20000。
   换算只在这一处做，且用 Math.round 收尾——`200.1 * 100` 在浮点下是
   20009.999…，直接取整会少一分钱。
   ========================================================================= */

import { api, isLoggedIn, ApiError } from './api.js';
import { esc, icon } from './util.js';
import { emptyHTML, toastError, toastOk } from './ui.js';
import { fmtCents } from './orders.js';
import { speakGateOrNull } from './auth-modal.js';
import { enableMarkdownImages } from './md-image.js';

const TITLE_MIN = 5;
const TITLE_MAX = 200;
const DESC_MIN = 10;
const DESC_MAX = 5000;
const MAX_SKILLS = 5;
/** 服务端限制（分），这里换成元给用户看。 */
const BUDGET_MIN_CENTS = 1000;
const BUDGET_MAX_CENTS = 500000;
/** 常见档位，点一下比手打快。 */
const BUDGET_PRESETS = [50, 100, 200, 500];

const EXPIRE_OPTIONS = [
  { h: 24, label: '1 天' },
  { h: 72, label: '3 天' },
  { h: 168, label: '7 天' },
];

const EXPECT_OPTIONS = [
  { m: 30, label: '30 分钟内' },
  { m: 60, label: '1 小时内' },
  { m: 240, label: '4 小时内' },
  { m: 1440, label: '24 小时内' },
];

export async function questionNewPage() {
  if (!isLoggedIn()) {
    return {
      html: `<div class="page-head">
        <h1>发布悬赏</h1>
        <p class="lede">把故障现象写清楚，值班专家会接单。</p>
      </div>` + emptyHTML({
        iconName: 'target',
        title: '登录后才能发布悬赏',
        desc: '悬赏与账号绑定，支付、评价、争议都跟着这个账号走。',
        actionHTML: '<button class="btn btn-primary" type="button" data-act="login">去登录</button>',
      }),
      title: '发布悬赏',
    };
  }

  // 只读账号：悬赏是要花真金白银发给服务方的，先要有发言资格。
  // 这个判断必须在渲染表单之前——让人填完一屏再被服务端拒掉是最差的体验。
  const speakGate = speakGateOrNull('发布悬赏', {
    head: '把故障现象写清楚，值班专家会接单。',
    desc: '发布悬赏需要先开通发言权限：悬赏内容会公开展示，也会由专家接单跟进。',
  });
  if (speakGate) return speakGate;

  // 技能拿不到不算致命：没有它只是少一列可选标签，页面照样要能打开
  const skills = await api.skills().catch(() => []);

  const html = `
    <nav class="crumb">
      <a href="#/">首页</a>
      <span class="crumb-sep">/</span>
      <a href="#/orders">我的订单</a>
      <span class="crumb-sep">/</span>
      <span>发布悬赏</span>
    </nav>

    <div class="page-head" style="padding-top:8px">
      <h1>发布悬赏</h1>
      <p class="lede">描述越具体，越容易被对口的专家接到。
        建议写清「现象 + 已排查过什么 + 期望结果」。</p>
    </div>

    <div class="detail-layout">
      <form class="card card-pad" id="q-form" novalidate>
        <div class="field" style="margin-bottom:18px">
          <label for="q-title">标题 <span class="hint">${TITLE_MIN}–${TITLE_MAX} 字</span></label>
          <input class="input" id="q-title" maxlength="${TITLE_MAX}"
                 placeholder="一句话说清现象，例如：K8s 节点 NotReady，kubelet 反复重启" />
        </div>

        <div class="field" style="margin-bottom:18px">
          <label for="q-desc">详细描述 <span class="hint">至少 ${DESC_MIN} 字</span></label>
          <textarea class="input" id="q-desc" rows="10" maxlength="${DESC_MAX}"
            placeholder="现象：&#10;已尝试：&#10;期望：&#10;相关日志可以直接贴在下边"></textarea>
        </div>

        <div class="field" style="margin-bottom:18px">
          <label>技术方向 <span class="hint">最多选 ${MAX_SKILLS} 个</span></label>
          <div class="chips-select" id="q-skills">
            ${(skills || []).length
              ? skills.map((s) => `<button class="chip chip-pick" type="button"
                  data-skill="${esc(String(s.id))}">${esc(s.name)}</button>`).join('')
              : '<p class="muted-note">技能清单暂时取不到，可以先发布，稍后补充。</p>'}
          </div>
          <p class="muted-note" id="q-skills-tip">选中的方向决定了这条悬赏会出现在哪些专家的大厅里。</p>
        </div>

        <div class="form-row">
          <div class="field">
            <label for="q-budget">悬赏金额（元）</label>
            <input class="input" id="q-budget" type="number" min="10" max="5000" step="1" value="100" />
            <div class="preset-row">
              ${BUDGET_PRESETS.map((v) => `<button class="chip chip-pick" type="button"
                data-budget="${v}">${v} 元</button>`).join('')}
            </div>
          </div>
          <div class="field">
            <label for="q-expect">期望响应</label>
            <select class="select" id="q-expect">
              ${EXPECT_OPTIONS.map((o) => `<option value="${o.m}">${esc(o.label)}</option>`).join('')}
            </select>
          </div>
          <div class="field">
            <label for="q-expire">悬赏有效期</label>
            <select class="select" id="q-expire">
              ${EXPIRE_OPTIONS.map((o) => `<option value="${o.h}" ${o.h === 72 ? 'selected' : ''}>${esc(o.label)}</option>`).join('')}
            </select>
          </div>
        </div>

        <p class="form-summary" id="q-summary"></p>

        <div class="form-actions">
          <button class="btn btn-primary btn-lg" id="q-submit" type="submit">发布悬赏</button>
          <a class="btn btn-lg" href="#/orders">取消</a>
        </div>
      </form>

      <aside>
        <div class="card side-card" style="margin-bottom:16px">
          <h3>发布之后</h3>
          <ol class="steps">
            <li>专家在接单大厅看到并接单</li>
            <li>你在 15 分钟内完成支付</li>
            <li>服务中随时沟通，完成后确认</li>
            <li>确认即结算，之后可以评价</li>
          </ol>
        </div>
        <div class="card side-card">
          <h3>写得好，接得快</h3>
          <ul class="tips-list">
            <li>带上版本号与报错原文，专家不必来回追问</li>
            <li>写清已经排除过什么，避免重复排查</li>
            <li>预算与难度匹配，过低会长时间没人接</li>
          </ul>
        </div>
      </aside>
    </div>
  `;

  const mount = (root) => {
    const form = root.querySelector('#q-form');
    const titleEl = root.querySelector('#q-title');
    const descEl = root.querySelector('#q-desc');
    const budgetEl = root.querySelector('#q-budget');
    const expectEl = root.querySelector('#q-expect');
    const expireEl = root.querySelector('#q-expire');
    const summary = root.querySelector('#q-summary');
    const submit = root.querySelector('#q-submit');
    const picked = new Set();

    // 描述支持 Markdown，也就该支持把报错截图直接粘进来。
    enableMarkdownImages(root, descEl);

    const refreshSummary = () => {
      const cents = yuanToCents(budgetEl.value);
      const hours = Number(expireEl.value) || 72;
      const remain = Number(expectEl.value) || 60;
      summary.innerHTML = cents >= BUDGET_MIN_CENTS
        ? `悬赏 <strong>${esc(fmtCents(cents))}</strong>，
           有效期 ${Math.round(hours / 24)} 天，期望 ${formatExpect(remain)} 响应。
           专家单笔实收约为 ${esc(fmtCents(Math.round(cents * 0.85)))}（平台抽佣 15%）。`
        : `悬赏金额至少 ${fmtCents(BUDGET_MIN_CENTS)}。`;
    };

    root.querySelectorAll('[data-skill]').forEach((btn) => {
      btn.addEventListener('click', () => {
        const id = btn.getAttribute('data-skill');
        if (picked.has(id)) {
          picked.delete(id);
          btn.classList.remove('is-on');
        } else {
          if (picked.size >= MAX_SKILLS) {
            toastError(`最多选 ${MAX_SKILLS} 个技术方向`);
            return;
          }
          picked.add(id);
          btn.classList.add('is-on');
        }
      });
    });

    root.querySelectorAll('[data-budget]').forEach((btn) => {
      btn.addEventListener('click', () => {
        budgetEl.value = btn.getAttribute('data-budget');
        refreshSummary();
      });
    });

    [budgetEl, expectEl, expireEl].forEach((el) => {
      el.addEventListener('input', refreshSummary);
      el.addEventListener('change', refreshSummary);
    });
    refreshSummary();

    form.addEventListener('submit', async (e) => {
      e.preventDefault();

      const title = titleEl.value.trim();
      const description = descEl.value.trim();
      const cents = yuanToCents(budgetEl.value);

      // 逐条给出具体问题，而不是一句"请检查输入"——用户不知道该改哪一项
      if (title.length < TITLE_MIN) { toastError(`标题至少 ${TITLE_MIN} 个字`); titleEl.focus(); return; }
      if (description.length < DESC_MIN) { toastError(`详细描述至少 ${DESC_MIN} 个字`); descEl.focus(); return; }
      if (!picked.size) { toastError('至少选一个技术方向'); return; }
      if (cents < BUDGET_MIN_CENTS) { toastError(`悬赏金额不能低于 ${fmtCents(BUDGET_MIN_CENTS)}`); budgetEl.focus(); return; }
      if (cents > BUDGET_MAX_CENTS) { toastError(`悬赏金额不能超过 ${fmtCents(BUDGET_MAX_CENTS)}`); budgetEl.focus(); return; }

      submit.disabled = true;
      submit.textContent = '正在发布…';
      try {
        const created = await api.createQuestion({
          title,
          description,
          skillIds: [...picked],
          budgetCents: cents,
          expectResponseMin: Number(expectEl.value) || undefined,
          expireHours: Number(expireEl.value) || undefined,
        });
        toastOk('悬赏已发布，等待专家接单');
        // 直接落到订单详情：发布后用户最关心的就是"有没有人接"。
        // 创建接口在同一个事务里就把订单建好了，所以这里一定有订单号。
        const orderNo = created && created.orderNo;
        location.hash = orderNo ? `#/orders/${encodeURIComponent(orderNo)}` : '#/orders';
      } catch (err) {
        submit.disabled = false;
        submit.textContent = '发布悬赏';
        toastError(err instanceof ApiError ? err.message : '发布失败，请稍后再试');
      }
    });
  };

  return { html, mount, title: '发布悬赏' };
}

/* ------------------------------------------------------------ 内部 -- */

/** 元 → 分。用 Math.round 收尾，避免 200.1 * 100 = 20009.99… 被截成 20009。 */
function yuanToCents(yuan) {
  const n = Number(yuan);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.round(n * 100);
}

function formatExpect(min) {
  if (min >= 1440) return `${Math.round(min / 1440)} 天`;
  if (min >= 60) return `${Math.round(min / 60)} 小时`;
  return `${min} 分钟`;
}
