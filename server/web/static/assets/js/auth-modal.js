/**
 * auth-modal.js — 登录 / 注册弹窗。
 *
 * 为什么单独一个模块而不是塞在 app.js：
 * 这块要同时处理「登录 + 注册」「邮箱 + 手机号」「密码 + 验证码」四组组合，
 * 混在路由入口里会让 app.js 越来越难读。这里只依赖 api.js 与 ui.js，
 * 不反向依赖 app.js（避免循环导入），成功后的刷新动作由调用方传回调。
 */
import {
  ApiError, canSpeak, isLoggedIn,
  loginByEmail, loginByEmailCode, loginByPassword, loginBySMS,
  registerByEmail, sendEmailCode, sendSMSCode,
} from './api.js';
import { closeModal, emptyHTML, openModal, toastError, toastOk } from './ui.js';
import { esc } from './util.js';

/** 打开认证弹窗。已登录时直接返回。 */
export function openAuthModal({ onSuccess } = {}) {
  if (isLoggedIn()) return;

  openModal(`
    <div class="modal-head">
      <div>
        <h2 id="auth-title">登录</h2>
        <p id="auth-sub">用邮箱登录，也可以注册一个只读账号先看看。</p>
      </div>
    </div>
    <div class="modal-body">
      <div class="auth-tabs" role="tablist">
        <button type="button" class="auth-tab is-active" data-tab="login" role="tab">登录</button>
        <button type="button" class="auth-tab" data-tab="register" role="tab">注册</button>
      </div>

      <!-- 登录 -->
      <div data-pane="login">
        <div class="field" data-only="email">
          <label for="auth-email">邮箱</label>
          <input class="input" id="auth-email" type="email" maxlength="128"
                 placeholder="you@example.com" autocomplete="username" />
        </div>

        <div class="field" data-only="phone" hidden>
          <label for="auth-phone">手机号</label>
          <input class="input" id="auth-phone" type="tel" inputmode="numeric" maxlength="11"
                 placeholder="11 位手机号" autocomplete="tel" />
        </div>

        <div class="field" data-only="password">
          <label for="auth-password">密码</label>
          <input class="input" id="auth-password" type="password" maxlength="64"
                 placeholder="登录密码" autocomplete="current-password" />
        </div>

        <div class="field" data-only="code" hidden>
          <label for="auth-code">验证码</label>
          <div style="display:flex;gap:8px">
            <input class="input" id="auth-code" type="text" inputmode="numeric" maxlength="6"
                   placeholder="6 位数字" autocomplete="one-time-code" />
            <button class="btn" id="auth-send" type="button"
                    style="flex:0 0 auto;height:38px;min-width:104px">获取验证码</button>
          </div>
        </div>

        <p class="auth-tip" id="auth-tip"></p>
        <button class="btn btn-primary btn-block btn-lg" id="auth-submit" type="button">登录</button>

        <div class="auth-links">
          <button type="button" data-act="toggle-code" id="auth-toggle-code">用验证码登录</button>
          <span class="auth-sep">·</span>
          <button type="button" data-act="toggle-phone" id="auth-toggle-phone">改用手机号</button>
        </div>
      </div>

      <!-- 注册 -->
      <div data-pane="register" hidden>
        <div class="field">
          <label for="reg-email">邮箱</label>
          <input class="input" id="reg-email" type="email" maxlength="128"
                 placeholder="you@example.com" autocomplete="email" />
        </div>
        <div class="field">
          <label for="reg-code">邮箱验证码</label>
          <div style="display:flex;gap:8px">
            <input class="input" id="reg-code" type="text" inputmode="numeric" maxlength="6"
                   placeholder="6 位数字" autocomplete="one-time-code" />
            <button class="btn" id="reg-send" type="button"
                    style="flex:0 0 auto;height:38px;min-width:104px">获取验证码</button>
          </div>
        </div>
        <div class="field">
          <label for="reg-password">设置密码</label>
          <input class="input" id="reg-password" type="password" maxlength="64"
                 placeholder="至少 8 位，需含字母和数字" autocomplete="new-password" />
        </div>
        <div class="field">
          <label for="reg-password2">确认密码</label>
          <input class="input" id="reg-password2" type="password" maxlength="64"
                 placeholder="再输入一次" autocomplete="new-password" />
        </div>
        <div class="field">
          <label for="reg-nickname">昵称<span class="auth-optional">（可选）</span></label>
          <input class="input" id="reg-nickname" type="text" maxlength="20"
                 placeholder="留空则用邮箱前缀" />
        </div>

        <p class="auth-tip" id="reg-tip"></p>
        <button class="btn btn-primary btn-block btn-lg" id="reg-submit" type="button">注册</button>
        <p class="auth-note">
          注册后为<strong>只读账号</strong>：可以浏览全部内容，
          需要发言时在「我的 → 发言权限」提交实名申请，审核通过后即可提问与回答。
        </p>
      </div>
    </div>
  `, (modal) => mount(modal, onSuccess));
}

/** 组装交互。mode 决定哪些字段可见。 */
function mount(modal, onSuccess) {
  const q = (sel) => modal.querySelector(sel);

  const title = q('#auth-title');
  const sub = q('#auth-sub');
  const tip = q('#auth-tip');
  const submit = q('#auth-submit');
  const emailInput = q('#auth-email');
  const phoneInput = q('#auth-phone');
  const pwdInput = q('#auth-password');
  const codeInput = q('#auth-code');
  const sendBtn = q('#auth-send');
  const toggleCode = q('#auth-toggle-code');
  const togglePhone = q('#auth-toggle-phone');

  const regEmail = q('#reg-email');
  const regCode = q('#reg-code');
  const regSend = q('#reg-send');
  const regPwd = q('#reg-password');
  const regPwd2 = q('#reg-password2');
  const regNick = q('#reg-nickname');
  const regTip = q('#reg-tip');
  const regSubmit = q('#reg-submit');

  // 登录模式：
  //   email-pwd  邮箱 + 密码（默认，最常用）
  //   email-code 邮箱 + 验证码（忘记密码时的退路）
  //   sms        手机号 + 验证码（老账号 / 开发环境）
  //   sms-pwd    手机号 + 密码（后台账号）
  let mode = 'email-pwd';

  const fieldEls = {
    email: modal.querySelectorAll('[data-only="email"]'),
    phone: modal.querySelectorAll('[data-only="phone"]'),
    password: modal.querySelectorAll('[data-only="password"]'),
    code: modal.querySelectorAll('[data-only="code"]'),
  };

  const show = (name, visible) => {
    fieldEls[name].forEach((el) => { el.hidden = !visible; });
  };

  const setTip = (el, text, tone = 'muted') => {
    el.textContent = text;
    el.style.color = tone === 'error' ? 'var(--danger)'
      : tone === 'ok' ? 'var(--ok)' : 'var(--text-3)';
  };

  const applyMode = () => {
    const byEmail = mode.startsWith('email');
    const byPwd = mode.endsWith('pwd');

    show('email', byEmail);
    show('phone', !byEmail);
    show('password', byPwd);
    show('code', !byPwd);

    toggleCode.textContent = byPwd ? '用验证码登录' : '改用密码登录';
    togglePhone.textContent = byEmail ? '改用手机号' : '改用邮箱';
    sub.textContent = byEmail
      ? '用邮箱登录。忘记密码时可以直接用邮箱验证码进来。'
      : '手机号登录，首次登录会自动创建账号。';

    setTip(tip, '');
    const focusEl = byEmail ? (byPwd ? pwdInput : codeInput) : (byPwd ? pwdInput : codeInput);
    focusEl.focus();
  };

  /* ------------------------------------------------------- 倒计时 -- */
  const makeCountdown = (btn) => {
    let timer = 0;
    return (seconds) => {
      let left = seconds || 60;
      clearInterval(timer);
      const tick = () => {
        btn.textContent = `${left} 秒后重发`;
        btn.disabled = left > 0;
        if (left <= 0) {
          clearInterval(timer);
          btn.textContent = '获取验证码';
        }
        left -= 1;
      };
      tick();
      timer = setInterval(tick, 1000);
      return () => clearInterval(timer);
    };
  };
  const loginCountdown = makeCountdown(sendBtn);
  const regCountdown = makeCountdown(regSend);

  const validEmail = (v) => /^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$/.test(v);
  const validPhone = (v) => /^1[3-9]\d{9}$/.test(v);

  /* --------------------------------------------------- 发送验证码 -- */
  sendBtn.addEventListener('click', async () => {
    const target = mode.startsWith('email') ? emailInput.value.trim() : phoneInput.value.trim();
    if (mode.startsWith('email') && !validEmail(target)) {
      setTip(tip, '请输入正确的邮箱地址', 'error'); emailInput.focus(); return;
    }
    if (!mode.startsWith('email') && !validPhone(target)) {
      setTip(tip, '请输入正确的 11 位手机号', 'error'); phoneInput.focus(); return;
    }
    sendBtn.disabled = true;
    setTip(tip, '正在发送…');
    try {
      const res = mode.startsWith('email')
        ? await sendEmailCode(target, 'login')
        : await sendSMSCode(target);
      if (res && res.devCode) {
        codeInput.value = res.devCode;
        setTip(tip, `开发模式：验证码 ${res.devCode} 已自动填入`, 'ok');
      } else {
        setTip(tip, `已发送，${res && res.expiresIn ? Math.round(res.expiresIn / 60) + ' 分钟内有效' : '请查收'}`);
      }
      codeInput.focus();
      loginCountdown((res && res.interval) || 60);
    } catch (err) {
      sendBtn.disabled = false;
      setTip(tip, err instanceof ApiError ? err.message : '发送失败，请稍后再试', 'error');
    }
  });

  regSend.addEventListener('click', async () => {
    const email = regEmail.value.trim();
    if (!validEmail(email)) {
      setTip(regTip, '请输入正确的邮箱地址', 'error'); regEmail.focus(); return;
    }
    regSend.disabled = true;
    setTip(regTip, '正在发送…');
    try {
      const res = await sendEmailCode(email, 'register');
      if (res && res.devCode) {
        regCode.value = res.devCode;
        setTip(regTip, `开发模式：验证码 ${res.devCode} 已自动填入`, 'ok');
      } else {
        setTip(regTip, '已发送，请到邮箱查收（注意垃圾邮件）');
      }
      regCode.focus();
      regCountdown((res && res.interval) || 60);
    } catch (err) {
      regSend.disabled = false;
      const msg = err instanceof ApiError ? err.message : '发送失败，请稍后再试';
      setTip(regTip, msg, 'error');
    }
  });

  /* ------------------------------------------------------------ 登录 -- */
  const doLogin = async () => {
    const byEmail = mode.startsWith('email');
    const byPwd = mode.endsWith('pwd');
    const account = byEmail ? emailInput.value.trim() : phoneInput.value.trim();

    if (byEmail && !validEmail(account)) { setTip(tip, '请输入正确的邮箱地址', 'error'); emailInput.focus(); return; }
    if (!byEmail && !validPhone(account)) { setTip(tip, '请输入正确的 11 位手机号', 'error'); phoneInput.focus(); return; }

    let call;
    if (byEmail && byPwd) {
      if (!pwdInput.value) { setTip(tip, '请输入密码', 'error'); pwdInput.focus(); return; }
      call = () => loginByEmail(account, pwdInput.value);
    } else if (byEmail && !byPwd) {
      if (!/^\d{6}$/.test(codeInput.value.trim())) { setTip(tip, '请输入 6 位验证码', 'error'); codeInput.focus(); return; }
      call = () => loginByEmailCode(account, codeInput.value.trim());
    } else if (byPwd) {
      if (!pwdInput.value) { setTip(tip, '请输入密码', 'error'); pwdInput.focus(); return; }
      call = () => loginByPassword(account, pwdInput.value);
    } else {
      if (!/^\d{4,6}$/.test(codeInput.value.trim())) { setTip(tip, '请输入收到的验证码', 'error'); codeInput.focus(); return; }
      call = () => loginBySMS(account, codeInput.value.trim());
    }

    submit.disabled = true;
    submit.textContent = '登录中…';
    try {
      await call();
      finish('登录成功');
    } catch (err) {
      submit.disabled = false;
      submit.textContent = '登录';
      setTip(tip, err instanceof ApiError ? err.message : '登录失败，请稍后再试', 'error');
    }
  };

  /* ------------------------------------------------------------ 注册 -- */
  const doRegister = async () => {
    const email = regEmail.value.trim();
    if (!validEmail(email)) { setTip(regTip, '请输入正确的邮箱地址', 'error'); regEmail.focus(); return; }
    if (!/^\d{6}$/.test(regCode.value.trim())) { setTip(regTip, '请输入邮箱收到的 6 位验证码', 'error'); regCode.focus(); return; }
    if (regPwd.value.length < 8) { setTip(regTip, '密码至少 8 位', 'error'); regPwd.focus(); return; }
    if (regPwd.value !== regPwd2.value) { setTip(regTip, '两次输入的密码不一致', 'error'); regPwd2.focus(); return; }

    regSubmit.disabled = true;
    regSubmit.textContent = '注册中…';
    try {
      await registerByEmail(email, regCode.value.trim(), regPwd.value, regNick.value.trim());
      // 提示与"只读"这个事实必须一起说：用户注册完就去点提问，
      // 结果只看到"需要申请"，会以为注册没成功。
      toastOk('注册成功，当前为只读账号');
      finish('');
    } catch (err) {
      regSubmit.disabled = false;
      regSubmit.textContent = '注册';
      setTip(regTip, err instanceof ApiError ? err.message : '注册失败，请稍后再试', 'error');
    }
  };

  const finish = async (message) => {
    closeModal();
    if (message) toastOk(message);
    try {
      await onSuccess?.();
    } catch {
      // 登录已经成功，刷新失败不该反过来把用户当未登录处理。
      toastError('登录成功，但页面刷新失败，请手动刷新');
    }
  };

  /* ------------------------------------------------------------ 绑定 -- */
  submit.addEventListener('click', doLogin);
  regSubmit.addEventListener('click', doRegister);

  toggleCode.addEventListener('click', () => {
    mode = mode.endsWith('pwd')
      ? (mode.startsWith('email') ? 'email-code' : 'sms')
      : (mode.startsWith('email') ? 'email-pwd' : 'sms-pwd');
    applyMode();
  });

  togglePhone.addEventListener('click', () => {
    mode = mode.startsWith('email')
      ? (mode.endsWith('pwd') ? 'sms-pwd' : 'sms')
      : (mode.endsWith('pwd') ? 'email-pwd' : 'email-code');
    applyMode();
  });

  // 回车提交：注册页签里回车不该触发登录。
  modal.addEventListener('keydown', (e) => {
    if (e.key !== 'Enter') return;
    const pane = modal.querySelector('[data-pane="login"]');
    if (pane && !pane.hidden) doLogin(); else doRegister();
  });

  // 页签切换
  modal.querySelectorAll('.auth-tab').forEach((btn) => {
    btn.addEventListener('click', () => {
      const tab = btn.dataset.tab;
      modal.querySelectorAll('.auth-tab').forEach((b) => b.classList.toggle('is-active', b === btn));
      modal.querySelectorAll('[data-pane]').forEach((p) => { p.hidden = p.dataset.pane !== tab; });
      title.textContent = tab === 'register' ? '注册' : '登录';
      sub.textContent = tab === 'register'
        ? '邮箱验证后即可创建账号，注册后为只读状态。'
        : '用邮箱登录。忘记密码时可以直接用邮箱验证码进来。';
      if (tab === 'register') regEmail.focus(); else emailInput.focus();
    });
  });

  applyMode();
  emailInput.focus();
}

/**
 * 发布类页面的权限闸门。
 *
 * 只拦"已登录但还不能发言"这一种情况：未登录由各页自己的登录引导处理，
 * 因为那时候该说的是"先登录"，而不是"先实名"。
 *
 * 为什么做成整页而不是弹窗：`#/contribute`、`#/ask` 都能从外部直连进来，
 * 弹窗只在点击的那一刻才有机会出现，直连进来的用户会直接看到一张填不出去的表格。
 *
 * 返回 null 表示放行；返回页面对象时调用方直接 return 它。
 */
export function speakGateOrNull(pageTitle, { head = '', desc = '' } = {}) {
  if (!isLoggedIn() || canSpeak()) return null;

  return {
    title: pageTitle,
    html: `<div class="page-head">
        <h1>${esc(pageTitle)}</h1>
        ${head ? `<p class="lede">${esc(head)}</p>` : ''}
      </div>` + emptyHTML({
      iconName: 'shield',
      title: '需要先开通发言权限',
      desc: desc || '注册账号默认为只读：可以浏览全部内容，但提问、回答与投稿需要先通过实名申请。',
      actionHTML: '<button class="btn btn-primary" type="button" data-act="go">去提交实名申请</button>',
    }),
    mount: (root) => {
      const btn = root.querySelector('[data-act="go"]');
      if (btn) {
        btn.addEventListener('click', () => {
          location.hash = '#/me';
        });
      }
    },
  };
}