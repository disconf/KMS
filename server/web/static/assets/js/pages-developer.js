/* ============================================================================
   pages-developer.js — 开放 API / 开发者页。

   给写脚本的人用：创建密钥、看用量、吊销，配一段能直接复制跑通的 curl。
   设计上的两个执念：
     1. 明文密钥只展示一次——弹窗里放大字号、给复制按钮、写明「关掉就没了」；
     2. 示例必须用当前站点域名拼出来，不能写死 localhost 让用户自己改。
   ========================================================================= */

import { api, isLoggedIn } from './api.js';
import { emptyHTML, errorHTML, bindError, openModal, closeModal, toastOk, toastError } from './ui.js';
import { esc, fmtDateTime, fmtNum, icon } from './util.js';

export async function developerPage() {
  if (!isLoggedIn()) {
    return {
      title: '开发者',
      html: `<div class="page-head"><h1>开放 API</h1></div>
        ${introHTML()}
        ${emptyHTML({
          iconName: 'key',
          title: '登录后创建 API 密钥',
          desc: '密钥跟账号走，脚本带着它就能查文档、搜题库。',
          actionHTML: '<button class="btn btn-primary" type="button" data-act="login">登录</button>',
        })}`,
      mount: (root) => bindError(root, {}),
    };
  }

  const data = await api.apiKeys().catch((err) => ({ error: err }));
  if (data && data.error) {
    return {
      title: '开发者',
      html: `<div class="page-head"><h1>开放 API</h1></div>${errorHTML(data.error)}`,
      mount: (root) => bindError(root, { onRetry: () => window.__reloadPage() }),
    };
  }

  const keys = Array.isArray(data.keys) ? data.keys : [];
  const activeCount = keys.filter((k) => k.status === 'active').length;

  const html = `
    <div class="page-head">
      <h1>开放 API</h1>
      <p class="lede">
        把平台接进你的脚本：巡检脚本开机先查文档、定时抽题给团队做晨会一问。
        只读接口，每密钥 60 次/分钟，密钥可随时吊销。
      </p>
    </div>

    ${introHTML()}

    <section class="dev-keys">
      <div class="dev-keys-head">
        <h2>${icon('key', 15)}我的密钥 <span class="dev-key-count">${activeCount}/${data.maxKeys || 10}</span></h2>
        <button class="btn btn-primary" type="button" data-act="create-key">
          ${icon('plus', 13)}新建密钥
        </button>
      </div>
      ${keys.length ? keysHTML(keys) : emptyHTML({
        iconName: 'key',
        title: '还没有密钥',
        desc: '新建一把，明文只显示一次，记得当场保存。',
      })}
    </section>

    ${docsHTML()}
  `;

  return {
    title: '开发者',
    html,
    mount: (root) => {
      bindError(root, {});
      bindDevActions(root);
    },
  };
}

/* --------------------------------------------------------------- 说明 -- */

function introHTML() {
  return `<section class="dev-intro">
    <div class="dev-intro-item">
      <div class="dev-intro-ic">${icon('book', 16)}</div>
      <div><strong>查文档</strong><span>文章列表与详情，正文是 Markdown 原文</span></div>
    </div>
    <div class="dev-intro-item">
      <div class="dev-intro-ic">${icon('search', 16)}</div>
      <div><strong>搜内容</strong><span>按关键词过滤文章，接进你自己的知识面板</span></div>
    </div>
    <div class="dev-intro-item">
      <div class="dev-intro-ic">${icon('quiz', 16)}</div>
      <div><strong>抽题</strong><span>随机抽题带答案解析，晨会一问、新人小测都行</span></div>
    </div>
  </section>`;
}

/* -------------------------------------------------------------- 密钥 -- */

function keysHTML(keys) {
  return `<div class="dev-key-list">
    ${keys.map((k) => `
      <div class="dev-key-row ${k.status === 'revoked' ? 'is-revoked' : ''}">
        <div class="dev-key-main">
          <strong>${esc(k.name)}</strong>
          <code class="dev-key-prefix">${esc(k.prefix)}••••••••</code>
        </div>
        <div class="dev-key-meta">
          <span>已调用 ${fmtNum(k.requestCount)} 次</span>
          <span>${k.lastUsedAt ? `最近 ${fmtDateTime(k.lastUsedAt)}` : '从未使用'}</span>
          <span>建于 ${fmtDateTime(k.createdAt)}</span>
        </div>
        <div class="dev-key-side">
          <span class="dev-key-status ${k.status === 'active' ? 'is-ok' : ''}">${esc(k.statusLabel)}</span>
          ${k.status === 'active'
            ? `<button class="btn btn-sm" type="button" data-revoke="${k.id}" data-name="${esc(k.name)}">吊销</button>`
            : ''}
        </div>
      </div>`).join('')}
  </div>`;
}

function bindDevActions(root) {
  root.addEventListener('click', (e) => {
    const create = e.target.closest('[data-act="create-key"]');
    if (create) { openCreate(); return; }
    const revoke = e.target.closest('[data-revoke]');
    if (revoke) {
      const id = revoke.getAttribute('data-revoke');
      const name = revoke.getAttribute('data-name');
      openModal(`
        <div class="modal-head"><h2>吊销密钥</h2></div>
        <div class="modal-body">
          <p class="dev-revoke-tip">
            ${icon('alert', 14)}吊销「${esc(name)}」后，正在使用它的脚本会立刻收到 401，
            且<strong>无法恢复</strong>——需要重新生成并替换到脚本里。确认吊销？
          </p>
        </div>
        <div class="modal-actions">
          <button class="btn" type="button" data-close>取消</button>
          <button class="btn btn-danger" type="button" data-confirm>确认吊销</button>
        </div>
      `, (modal) => {
        modal.querySelector('[data-close]').addEventListener('click', closeModal);
        modal.querySelector('[data-confirm]').addEventListener('click', async () => {
          try {
            await api.revokeApiKey(id);
            closeModal();
            toastOk('已吊销');
            window.__reloadPage();
          } catch (err) {
            toastError(err && err.message ? err.message : '吊销失败');
          }
        });
      });
    }
  });
}

function openCreate() {
  openModal(`
    <div class="modal-head"><h2>新建 API 密钥</h2></div>
    <div class="modal-body">
      <label class="field">
        <span class="field-label">密钥名称</span>
        <input class="input" type="text" data-key-name maxlength="60"
               placeholder="例如：公司巡检脚本" />
      </label>
      <p class="dev-revoke-tip">${icon('info', 13)}明文密钥只在创建成功后显示一次，请当场复制保存。</p>
    </div>
    <div class="modal-actions">
      <button class="btn" type="button" data-close>取消</button>
      <button class="btn btn-primary" type="button" data-go>创建</button>
    </div>
  `, (modal) => {
    const input = modal.querySelector('[data-key-name]');
    input.focus();
    modal.querySelector('[data-close]').addEventListener('click', closeModal);
    const go = modal.querySelector('[data-go]');
    const submit = async () => {
      go.disabled = true;
      go.textContent = '创建中…';
      try {
        const created = await api.createApiKey(input.value.trim());
        showCreated(created);
      } catch (err) {
        toastError(err && err.message ? err.message : '创建失败');
        go.disabled = false;
        go.textContent = '创建';
      }
    };
    go.addEventListener('click', submit);
    input.addEventListener('keydown', (e) => { if (e.key === 'Enter') submit(); });
  });
}

/** 明文只此一次的展示弹窗。 */
function showCreated(created) {
  openModal(`
    <div class="modal-head"><h2>${icon('check', 15)}密钥已创建</h2></div>
    <div class="modal-body">
      <p class="dev-once-warn">${icon('alert', 14)}<strong>这是明文密钥唯一一次展示</strong>，关闭后无法再查看，只能吊销重建。</p>
      <div class="dev-key-show">
        <code id="dev-plain-key">${esc(created.key)}</code>
        <button class="btn btn-sm" type="button" data-copy>复制</button>
      </div>
      <p class="dev-revoke-tip">名称：${esc(created.name)} · 前缀 <code>${esc(created.prefix)}</code>（用于列表识别）</p>
    </div>
    <div class="modal-actions">
      <button class="btn btn-primary" type="button" data-done>我已保存</button>
    </div>
  `, (modal) => {
    const done = () => { closeModal(); window.__reloadPage(); };
    modal.querySelector('[data-done]').addEventListener('click', done);
    modal.querySelector('[data-copy]').addEventListener('click', async (e) => {
      try {
        await navigator.clipboard.writeText(created.key);
        e.target.textContent = '已复制';
      } catch {
        // 剪贴板被拒（http 环境常见）：选中文本让用户 Ctrl+C，别报错吓人。
        const range = document.createRange();
        range.selectNodeContents(modal.querySelector('#dev-plain-key'));
        const sel = window.getSelection();
        sel.removeAllRanges(); sel.addRange(range);
        e.target.textContent = '按 Ctrl+C 复制';
      }
    });
  });
}

/* --------------------------------------------------------------- 文档 -- */

/** 接口文档。地址用当前域名拼，示例直接可跑。 */
function docsHTML() {
  const base = `${location.protocol}//${location.host}/open/v1`;
  const curl = (path, note) => `
    <div class="dev-snippet">
      <div class="dev-snippet-head"><span>${esc(note)}</span><button class="btn btn-sm" type="button" data-copy-cmd>复制</button></div>
      <pre><code>curl -H "X-Api-Key: opsk_你的密钥" \\
  "${base}${path}"</code></pre>
    </div>`;

  return `<section class="dev-docs">
    <h2>${icon('terminal', 15)}接口速览</h2>
    <table class="dev-table">
      <thead><tr><th>接口</th><th>说明</th><th>参数</th></tr></thead>
      <tbody>
        <tr><td><code>GET /open/v1/ping</code></td><td>探活，验证密钥有效</td><td>—</td></tr>
        <tr><td><code>GET /open/v1/articles</code></td><td>已发布文章列表</td><td><code>channel</code> <code>q</code> <code>page</code> <code>pageSize</code></td></tr>
        <tr><td><code>GET /open/v1/articles/:slug</code></td><td>文章详情（Markdown 原文）</td><td>路径带 slug</td></tr>
        <tr><td><code>GET /open/v1/quizzes/random</code></td><td>随机抽题（带答案与解析）</td><td><code>category</code> <code>count</code>（≤20）</td></tr>
      </tbody>
    </table>
    <p class="dev-docs-note">
      ${icon('info', 13)}鉴权放 <code>X-Api-Key</code> 头或
      <code>Authorization: Bearer opsk_…</code>；限流 60 次/分钟，响应头
      <code>X-RateLimit-Remaining</code> 会带剩余配额，脚本请按它退避。
    </p>
    ${curl('/ping', '先探活')}
    ${curl('/articles?pageSize=5', '最新 5 篇文章')}
    ${curl('/quizzes/random?count=3', '抽 3 道题')}
  </section>`;
}
