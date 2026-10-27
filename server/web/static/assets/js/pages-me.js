/**
 * pages-me.js — 我的账号页：资料概览 + 发言权限申请。
 *
 * 为什么把"发言权限"做成独立页面而不是塞进设置弹窗：
 * 只读用户点任何发布按钮都会撞到这道墙，那时需要一个可以直达、
 * 能说明"为什么"和"多久能通过"的落点。弹窗只够提示，不够解释。
 */
import { api, currentUser, isLoggedIn, refreshProfile } from './api.js';
import { emptyHTML, toastError, toastOk } from './ui.js';
import { esc, fmtDate, icon } from './util.js';

const SPEAK_META = {
  none: { label: '只读', tone: 'warn', desc: '可以浏览全部内容，不能提问、回答、评论或投稿。' },
  pending: { label: '审核中', tone: 'brand', desc: '申请已提交，审核通过后即可发言。' },
  approved: { label: '可发言', tone: 'ok', desc: '已开通发言权限，发布的内容仍需经过审核。' },
  rejected: { label: '已驳回', tone: 'danger', desc: '上次申请未通过，可以按审核意见修改后重新提交。' },
};

function pageHead(title, lede) {
  return `<div class="page-head" style="padding-top:8px">
    <h1>${esc(title)}</h1>
    ${lede ? `<p class="lede">${esc(lede)}</p>` : ''}
  </div>`;
}

export async function mePage() {
  if (!isLoggedIn()) {
    return {
      title: '我的',
      html: pageHead('我的账号', '登录后可以查看账号状态与发言权限。') + emptyHTML({
        iconName: 'user',
        title: '请先登录',
        desc: '登录后这里会显示你的账号信息与发言权限状态。',
        actionHTML: '<button class="btn btn-primary" type="button" data-act="login">去登录</button>',
      }),
    };
  }

  // 先刷新一次资料：发言状态可能刚被管理员改过，
  // 用登录时缓存的旧值会让用户看到"已通过"却发不了言。
  let user = currentUser();
  try {
    const fresh = await refreshProfile();
    if (fresh) user = fresh;
  } catch {
    // 刷新失败就用本地会话里的资料，至少页面还能渲染出来。
  }

  const state = await safeSpeakState();
  return {
    title: '我的账号',
    html: pageHead('我的账号', '账号资料与发言权限。')
      + `<div class="me-page">${profileCard(user)}${speakCard(state, user)}</div>`,
    mount: (root) => bindApply(root, state),
  };
}

/** 拉取发言状态，失败时降级成"未知但按只读处理"。 */
async function safeSpeakState() {
  try {
    const data = await api.speakState();
    return {
      status: (data && data.status) || 'none',
      canSpeak: Boolean(data && data.canSpeak),
      appliedAt: data && data.appliedAt,
      reviewedAt: data && data.reviewedAt,
      reviewNote: (data && data.reviewNote) || '',
    };
  } catch {
    return { status: 'none', canSpeak: false, reviewNote: '' };
  }
}

function profileCard(user) {
  if (!user) return '';
  const rows = [
    ['昵称', user.nickname || '—'],
    ['邮箱', user.email || '—'],
    ['手机号', user.phone || '未绑定'],
    ['角色', user.role === 'admin' ? '管理员' : user.role === 'expert' ? '专家' : '普通用户'],
    ['注册时间', user.createdAt ? fmtDate(user.createdAt) : '—'],
  ];
  return `
    <section class="card me-card">
      <div class="me-card-head">
        <div class="me-avatar">${esc((user.nickname || 'U').slice(0, 1))}</div>
        <div>
          <div class="me-name">${esc(user.nickname || '未设置昵称')}</div>
          <div class="me-sub">${esc(user.email || user.phone || '')}</div>
        </div>
      </div>
      <dl class="me-rows">
        ${rows.map(([k, v]) => `<div><dt>${esc(k)}</dt><dd>${esc(v)}</dd></div>`).join('')}
      </dl>
      <div class="me-actions">
        <a class="btn btn-sm" href="#/me/contributions">我的投稿</a>
        <a class="btn btn-sm" href="#/orders">我的订单</a>
      </div>
    </section>`;
}

function speakCard(state, user) {
  const meta = SPEAK_META[state.status] || SPEAK_META.none;
  const badge = `<span class="chip chip-${meta.tone}">${esc(meta.label)}</span>`;

  // 管理员与专家不需要申请，直接把原因说清楚，别让他们去找一个不存在的按钮。
  const exempt = user && (user.role === 'admin' || user.role === 'expert');

  let body = '';
  if (exempt) {
    body = `<p class="me-hint">${user.role === 'admin' ? '管理员' : '专家'}账号默认具备发布权限，无需申请。</p>`;
  } else if (state.canSpeak) {
    body = `<p class="me-hint">已开通发言权限。发布的内容仍会进入审核队列，通过后才会公开。</p>`;
  } else if (state.status === 'pending') {
    body = `<p class="me-hint">
      申请已提交${state.appliedAt ? `（${esc(fmtDate(state.appliedAt))}）` : ''}，审核通过后自动开通。
    </p>`;
  } else {
    const rejected = state.status === 'rejected';
    body = `
      ${rejected && state.reviewNote ? `<div class="me-reject">
        <strong>上次驳回原因</strong><p>${esc(state.reviewNote)}</p>
      </div>` : ''}
      <p class="me-hint">
        提交真实姓名与联系方式，管理员审核通过后即可提问与回答。
        不收集身份证号，信息仅用于确认申请人可联系。
      </p>
      <form class="me-form" id="speak-form">
        <div class="field">
          <label for="speak-name">真实姓名</label>
          <input class="input" id="speak-name" maxlength="32" placeholder="例如：张伟" required />
        </div>
        <div class="field">
          <label for="speak-contact">联系方式</label>
          <input class="input" id="speak-contact" maxlength="64"
                 placeholder="手机号或常用邮箱" required />
        </div>
        <div class="field">
          <label for="speak-note">申请理由<span class="auth-optional">（可选）</span></label>
          <textarea class="input" id="speak-note" rows="3" maxlength="255"
                    placeholder="简单说明你想参与的方向，例如：K8s 运维，想回答集群相关的问题"></textarea>
        </div>
        <p class="auth-tip" id="speak-tip"></p>
        <button class="btn btn-primary" type="submit" id="speak-submit">提交申请</button>
      </form>`;
  }

  return `
    <section class="card me-card">
      <div class="me-card-head">
        <div>
          <div class="me-name">发言权限 ${badge}</div>
          <div class="me-sub">${esc(meta.desc)}</div>
        </div>
      </div>
      ${body}
    </section>`;
}

function bindApply(root, state) {
  const form = root.querySelector('#speak-form');
  if (!form) return;

  const tip = root.querySelector('#speak-tip');
  const submit = root.querySelector('#speak-submit');
  const setTip = (text, tone = 'muted') => {
    tip.textContent = text;
    tip.style.color = tone === 'error' ? 'var(--danger)' : 'var(--text-3)';
  };

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const realName = root.querySelector('#speak-name').value.trim();
    const contact = root.querySelector('#speak-contact').value.trim();
    const note = root.querySelector('#speak-note').value.trim();

    if (realName.length < 2) { setTip('请填写真实姓名', 'error'); return; }
    if (contact.length < 5) { setTip('请填写有效的联系方式', 'error'); return; }

    submit.disabled = true;
    submit.textContent = '提交中…';
    try {
      await api.applySpeak({ realName, contact, note: note || undefined });
      toastOk('申请已提交，请等待审核');
      window.__reloadPage();
    } catch (err) {
      submit.disabled = false;
      submit.textContent = '提交申请';
      const msg = err && err.message ? err.message : '提交失败，请稍后再试';
      setTip(msg, 'error');
      // 已经是 pending 的情况多半是别处提交过，刷新一下让页面显示真实状态。
      if (String(msg).includes('已提交')) {
        toastError(msg);
        window.__reloadPage();
      }
    }
  });
}
