/* ============================================================================
   pages-toolbox.js — 运维工具箱。
   八个纯前端小工具：时间戳 / cron / 编解码 / JWT / 正则 / JSON / 子网 / 进制。
   全部本地计算——运维把 IP、令牌、日志片段粘进来算，数据不能出浏览器，
   这是工具箱的隐私底线，也顺便做到零请求零延迟。
   ========================================================================= */

import { esc, icon } from './util.js';

/* ------------------------------------------------------------- 工具清单 -- */

const TOOLS = [
  { key: 'timestamp', name: '时间戳转换', desc: '秒/毫秒与日期双向换算' },
  { key: 'cron',      name: 'Cron 解析',   desc: '表达式校验与最近执行时间' },
  { key: 'encode',    name: '编码转换',    desc: 'URL 与 Base64 互转' },
  { key: 'jwt',       name: 'JWT 解码',    desc: '拆 Header/Payload 与过期时间' },
  { key: 'regex',     name: '正则测试',    desc: '实时匹配与命中列表' },
  { key: 'json',      name: 'JSON 格式化', desc: '美化/压缩与错误定位' },
  { key: 'subnet',    name: '子网计算',    desc: 'CIDR 网段/掩码/可用数' },
  { key: 'radix',     name: '进制转换',    desc: '2/8/10/16 进制互转' },
];

/* ------------------------------------------------------------- Cron 解析 -- */

/**
 * 标准 5 段 cron（分 时 日 月 周）解析器。
 * 支持 * , - / 四种语法。逐分钟探测太慢，按"先定候选日、再定时刻"两级走：
 * 候选日扫描上限 3 年，时刻在同一天内由三段交集直接算出，纯数字循环不过几十万次。
 */
function parseCronField(expr, min, max) {
  if (expr === '*') return { any: true, set: null };
  const set = new Set();
  for (const part of expr.split(',')) {
    let body = part;
    let step = 1;
    const slash = part.indexOf('/');
    if (slash >= 0) {
      body = part.slice(0, slash);
      step = parseInt(part.slice(slash + 1), 10);
      if (!Number.isInteger(step) || step < 1) throw new Error(`步长非法：${part}`);
    }
    let lo = min;
    let hi = max;
    if (body !== '*') {
      const dash = body.indexOf('-');
      if (dash >= 0) {
        lo = parseInt(body.slice(0, dash), 10);
        hi = parseInt(body.slice(dash + 1), 10);
      } else {
        lo = hi = parseInt(body, 10);
      }
      if (!Number.isInteger(lo) || !Number.isInteger(hi) || lo < min || hi > max || lo > hi) {
        throw new Error(`取值超出范围 [${min}-${max}]：${part}`);
      }
    }
    for (let v = lo; v <= hi; v += step) set.add(v);
  }
  if (!set.size) throw new Error(`字段为空：${expr}`);
  return { any: false, set };
}

const CRON_DOW_ALIAS = { sun: 0, mon: 1, tue: 2, wed: 3, thu: 4, fri: 5, sat: 6 };

function parseCronDow(expr) {
  // 周字段放行 sun/mon/... 别名，先归一成数字再走通用解析。
  const normalized = expr.toLowerCase().replace(/[a-z]{3}/g, (m) =>
    CRON_DOW_ALIAS[m] !== undefined ? String(CRON_DOW_ALIAS[m]) : m);
  return parseCronField(normalized, 0, 6);
}

function cronNextRuns(expr, count = 5) {
  const parts = expr.trim().split(/\s+/);
  if (parts.length !== 5) throw new Error('标准 cron 为 5 段：分 时 日 月 周');
  const f = {
    min:  parseCronField(parts[0], 0, 59),
    hour: parseCronField(parts[1], 0, 23),
    dom:  parseCronField(parts[2], 1, 31),
    mon:  parseCronField(parts[3], 1, 12),
    dow:  parseCronDow(parts[4]),
  };

  const runs = [];
  const probe = new Date();
  probe.setSeconds(0, 0);
  probe.setMinutes(probe.getMinutes() + 1); // 从下一分钟整起算，不算"正在过的"这一分钟

  for (let day = 0; day < 366 * 3 && runs.length < count; day++) {
    const base = new Date(probe);
    base.setDate(base.getDate() + day);
    base.setHours(0, 0, 0, 0);
    if (!f.mon.any && !f.mon.set.has(base.getMonth() + 1)) continue;
    const domOk = f.dom.any || f.dom.set.has(base.getDate());
    const dowOk = f.dow.any || f.dow.set.has(base.getDay());
    // 标准语义：日与周都受限时取并集（满足其一即命中）。
    const dayMatch = (!f.dom.any && !f.dow.any) ? (domOk || dowOk) : (domOk && dowOk);
    if (!dayMatch) continue;

    // 当天内逐分钟找命中。首日只扫当前时刻之后的分钟，之后的日子从头扫。
    const from = day === 0 ? probe.getHours() * 60 + probe.getMinutes() : 0;
    for (let mod = from; mod < 1440 && runs.length < count; mod++) {
      if (!f.min.any && !f.min.set.has(mod % 60)) continue;
      if (!f.hour.any && !f.hour.set.has(Math.floor(mod / 60))) continue;
      const t = new Date(base);
      t.setMinutes(mod);
      runs.push(t);
    }
  }
  if (!runs.length) throw new Error('未来 3 年内没有命中时间，请检查日/月/周字段');
  return runs;
}

/* ------------------------------------------------------------ 各工具实现 -- */

function twoPad(n) { return String(n).padStart(2, '0'); }
function fmtLocal(d) {
  return `${d.getFullYear()}-${twoPad(d.getMonth() + 1)}-${twoPad(d.getDate())} ` +
         `${twoPad(d.getHours())}:${twoPad(d.getMinutes())}:${twoPad(d.getSeconds())}`;
}

const toolPanels = {
  timestamp: `
    <div class="tb-grid">
      <div class="field">
        <label for="ts-in">时间戳（自动识别 10 位秒 / 13 位毫秒）</label>
        <input class="input" id="ts-in" inputmode="numeric" placeholder="如 1796428800" />
        <p class="tb-out" id="ts-out"></p>
      </div>
      <div class="field">
        <label for="ts-date">日期时间</label>
        <input class="input" id="ts-date" type="datetime-local" step="1" />
        <p class="tb-out" id="ts-out2"></p>
      </div>
    </div>
    <button class="btn btn-sm" type="button" data-tb="now">填入当前时间</button>`,
  cron: `
    <div class="field">
      <label for="cr-in">Cron 表达式（分 时 日 月 周）</label>
      <input class="input" id="cr-in" placeholder="如 */5 * * * *" spellcheck="false" />
    </div>
    <div id="cr-out" class="tb-out"></div>`,
  encode: `
    <div class="field">
      <label for="enc-in">原文</label>
      <textarea class="input code-input" id="enc-in" rows="4" spellcheck="false"></textarea>
    </div>
    <div class="tb-btns">
      <button class="btn btn-sm" type="button" data-tb="url-enc">URL 编码</button>
      <button class="btn btn-sm" type="button" data-tb="url-dec">URL 解码</button>
      <button class="btn btn-sm" type="button" data-tb="b64-enc">Base64 编码</button>
      <button class="btn btn-sm" type="button" data-tb="b64-dec">Base64 解码</button>
    </div>
    <div class="field">
      <label for="enc-out">结果</label>
      <textarea class="input code-input" id="enc-out" rows="4" spellcheck="false" readonly></textarea>
    </div>`,
  jwt: `
    <div class="field">
      <label for="jwt-in">JWT 令牌</label>
      <textarea class="input code-input" id="jwt-in" rows="4" spellcheck="false" placeholder="eyJhbGciOi..."></textarea>
    </div>
    <div id="jwt-out" class="tb-out"></div>`,
  regex: `
    <div class="tb-grid">
      <div class="field"><label for="re-pat">正则</label>
        <input class="input code-input" id="re-pat" spellcheck="false" placeholder="如 (\\d{1,3}\\.){3}\\d{1,3}" /></div>
      <div class="field"><label for="re-flags">标志</label>
        <input class="input code-input" id="re-flags" value="g" spellcheck="false" /></div>
    </div>
    <div class="field"><label for="re-text">测试文本</label>
      <textarea class="input code-input" id="re-text" rows="4" spellcheck="false"></textarea></div>
    <div id="re-out" class="tb-out"></div>`,
  json: `
    <div class="field">
      <label for="js-in">JSON</label>
      <textarea class="input code-input" id="js-in" rows="6" spellcheck="false"></textarea>
    </div>
    <div class="tb-btns">
      <button class="btn btn-sm" type="button" data-tb="json-pretty">格式化</button>
      <button class="btn btn-sm" type="button" data-tb="json-min">压缩</button>
    </div>
    <div class="field">
      <label for="js-out">结果</label>
      <textarea class="input code-input" id="js-out" rows="6" spellcheck="false" readonly></textarea>
    </div>`,
  subnet: `
    <div class="field">
      <label for="sb-in">IPv4 地址 / CIDR</label>
      <input class="input code-input" id="sb-in" placeholder="如 192.168.10.37/26" spellcheck="false" />
    </div>
    <div id="sb-out" class="tb-out"></div>`,
  radix: `
    <div class="tb-grid">
      ${['2', '8', '10', '16'].map((b) => `
        <div class="field">
          <label for="rd-${b}">${{ '2': '二进制', '8': '八进制', '10': '十进制', '16': '十六进制' }[b]}</label>
          <input class="input code-input" data-radix="${b}" id="rd-${b}" spellcheck="false" />
        </div>`).join('')}
    </div>
    <p class="tb-hint">在任意一栏输入，其余三栏实时换算。负数与小数不支持。</p>`,
};

/* -------------------------------------------------------------- 页面 ---- */

export async function toolboxPage(ctx) {
  const active = TOOLS.some((t) => t.key === ctx.params.get('t')) ? ctx.params.get('t') : 'timestamp';
  const tool = TOOLS.find((t) => t.key === active);

  const html = `
    <div class="page-head">
      <h1>运维工具箱</h1>
      <p class="lede">八个日常小工具，全部在浏览器本地计算——令牌、IP、日志片段不会发到任何服务器。</p>
    </div>

    <div class="toolbar tb-nav" role="tablist" aria-label="选择工具">
      ${TOOLS.map((t) => `
        <a class="btn btn-sm ${t.key === active ? 'btn-primary' : ''}" role="tab"
           aria-selected="${t.key === active}" href="#/toolbox?t=${t.key}">${esc(t.name)}</a>`).join('')}
    </div>

    <div class="card tb-panel">
      <h2 class="tb-title">${esc(tool.name)}<span class="tb-desc">${esc(tool.desc)}</span></h2>
      ${toolPanels[active]}
    </div>
  `;

  const mount = (root) => mountTool(root, active);

  return { html, mount, title: '运维工具箱' };
}

/* ------------------------------------------------------------ 挂载逻辑 -- */

function mountTool(root, key) {
  const $ = (id) => root.querySelector(`#${CSS.escape(id)}`);

  const wire = (id, ev, fn) => {
    const n = $(id);
    if (n) n.addEventListener(ev, fn);
  };

  /* -- 时间戳 -- */
  if (key === 'timestamp') {
    const parse = (v) => {
      const n = Number(v.trim());
      if (!Number.isFinite(n) || v.trim() === '') return null;
      return n > 1e12 ? new Date(n) : n > 1e9 ? new Date(n * 1000) : null;
    };
    wire('ts-in', 'input', () => {
      const d = parse($('ts-in').value);
      $('ts-out').textContent = d ? fmtLocal(d) : ($('ts-in').value.trim() ? '无法识别，请输入秒（10 位）或毫秒（13 位）' : '');
      if (d) {
        $('ts-date').value = `${d.getFullYear()}-${twoPad(d.getMonth() + 1)}-${twoPad(d.getDate())}T${twoPad(d.getHours())}:${twoPad(d.getMinutes())}:${twoPad(d.getSeconds())}`;
      }
    });
    wire('ts-date', 'input', () => {
      const d = new Date($('ts-date').value);
      if (Number.isNaN(d.getTime())) { $('ts-out2').textContent = ''; return; }
      $('ts-out2').textContent = `秒 ${Math.floor(d.getTime() / 1000)}　毫秒 ${d.getTime()}`;
    });
    root.querySelector('[data-tb="now"]')?.addEventListener('click', () => {
      const now = new Date();
      $('ts-in').value = Math.floor(now.getTime() / 1000);
      $('ts-in').dispatchEvent(new Event('input'));
    });
  }

  /* -- cron -- */
  if (key === 'cron') {
    wire('cr-in', 'input', () => {
      const out = $('cr-out');
      const v = $('cr-in').value.trim();
      if (!v) { out.innerHTML = ''; return; }
      try {
        const runs = cronNextRuns(v);
        out.innerHTML = `
          <p class="tb-ok">${icon('check', 14)} 表达式有效，最近 5 次执行：</p>
          <ol class="tb-list">${runs.map((d) => `<li>${fmtLocal(d)}</li>`).join('')}</ol>`;
      } catch (e) {
        out.innerHTML = `<p class="tb-err">${icon('alert', 14)} ${esc(e.message)}</p>`;
      }
    });
  }

  /* -- 编码 -- */
  if (key === 'encode') {
    const b64 = {
      enc: (s) => btoa(String.fromCharCode(...new TextEncoder().encode(s))),
      dec: (s) => new TextDecoder().decode(Uint8Array.from(atob(s.trim()), (c) => c.charCodeAt(0))),
    };
    root.querySelectorAll('[data-tb]').forEach((btn) => {
      btn.addEventListener('click', () => {
        const src = $('enc-in').value;
        const out = $('enc-out');
        const act = btn.getAttribute('data-tb');
        try {
          out.value = act === 'url-enc' ? encodeURIComponent(src)
            : act === 'url-dec' ? decodeURIComponent(src)
            : act === 'b64-enc' ? b64.enc(src)
            : b64.dec(src);
        } catch (e) {
          out.value = `转换失败：${e.message}`;
        }
      });
    });
  }

  /* -- JWT -- */
  if (key === 'jwt') {
    wire('jwt-in', 'input', () => {
      const out = $('jwt-out');
      const token = $('jwt-in').value.trim();
      if (!token) { out.innerHTML = ''; return; }
      const parts = token.split('.');
      if (parts.length < 2) { out.innerHTML = '<p class="tb-err">格式不对：至少要有两段（header.payload）</p>'; return; }
      const b64u = (s) => new TextDecoder().decode(Uint8Array.from(atob(s.replace(/-/g, '+').replace(/_/g, '/')), (c) => c.charCodeAt(0)));
      try {
        const head = JSON.parse(b64u(parts[0]));
        const body = JSON.parse(b64u(parts[1]));
        let expLine = '';
        if (body.exp) {
          const d = new Date(body.exp * 1000);
          const left = Math.floor((d.getTime() - Date.now()) / 60000);
          expLine = `<p class="${left > 0 ? 'tb-ok' : 'tb-err'}">过期时间：${fmtLocal(d)}（${left > 0 ? `还剩 ${Math.floor(left / 60)} 小时 ${left % 60} 分` : '已过期'}）</p>`;
        }
        out.innerHTML = `
          ${expLine}
          <p class="tb-label">Header</p><pre class="tb-pre">${esc(JSON.stringify(head, null, 2))}</pre>
          <p class="tb-label">Payload</p><pre class="tb-pre">${esc(JSON.stringify(body, null, 2))}</pre>`;
      } catch (e) {
        out.innerHTML = `<p class="tb-err">解码失败：${esc(e.message)}（注意 JWT 的 payload 才是明文，签名段无法反解）</p>`;
      }
    });
  }

  /* -- 正则 -- */
  if (key === 'regex') {
    const run = () => {
      const out = $('re-out');
      const pat = $('re-pat').value;
      const flags = $('re-flags').value;
      const text = $('re-text').value;
      if (!pat || !text) { out.innerHTML = ''; return; }
      try {
        const re = new RegExp(pat, flags.includes('g') ? flags : flags + 'g');
        const matches = [...text.matchAll(re)];
        if (!matches.length) { out.innerHTML = '<p class="tb-hint">没有命中。</p>'; return; }
        // 高亮：按命中区间从左到右拼回去，全程 esc 后再包 <mark>，杜绝注入。
        let html = '';
        let pos = 0;
        for (const m of matches) {
          if (m[0] === '') continue;
          html += esc(text.slice(pos, m.index)) + `<mark>${esc(m[0])}</mark>`;
          pos = m.index + m[0].length;
        }
        html += esc(text.slice(pos));
        out.innerHTML = `
          <p class="tb-ok">${icon('check', 14)} 命中 ${matches.length} 处</p>
          <pre class="tb-pre">${html}</pre>
          <p class="tb-hint">捕获组：${matches.slice(0, 5).map((m) => esc(JSON.stringify(m.slice(1)))).join('、')}</p>`;
      } catch (e) {
        out.innerHTML = `<p class="tb-err">${esc(e.message)}</p>`;
      }
    };
    ['re-pat', 're-flags', 're-text'].forEach((id) => wire(id, 'input', run));
  }

  /* -- JSON -- */
  if (key === 'json') {
    const run = (minify) => {
      const out = $('js-out');
      const src = $('js-in').value.trim();
      if (!src) { out.value = ''; return; }
      try {
        const obj = JSON.parse(src);
        out.value = minify ? JSON.stringify(obj) : JSON.stringify(obj, null, 2);
      } catch (e) {
        // JSON.parse 的报错带位置信息，直接透传比自定义解析更准。
        out.value = `JSON 非法：${e.message}`;
      }
    };
    root.querySelector('[data-tb="json-pretty"]')?.addEventListener('click', () => run(false));
    root.querySelector('[data-tb="json-min"]')?.addEventListener('click', () => run(true));
  }

  /* -- 子网 -- */
  if (key === 'subnet') {
    wire('sb-in', 'input', () => {
      const out = $('sb-out');
      const v = $('sb-in').value.trim();
      if (!v) { out.innerHTML = ''; return; }
      const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})(?:\/(\d{1,2}))?$/.exec(v);
      if (!m) { out.innerHTML = '<p class="tb-err">格式：a.b.c.d/掩码长度，如 192.168.10.37/26</p>'; return; }
      const octs = m.slice(1, 5).map(Number);
      const prefix = m[5] === undefined ? 24 : Number(m[5]);
      if (octs.some((o) => o > 255) || prefix > 32) {
        out.innerHTML = '<p class="tb-err">数值超出范围（八位组 ≤255，掩码 ≤32）</p>';
        return;
      }
      const ip = ((octs[0] << 24) | (octs[1] << 16) | (octs[2] << 8) | octs[3]) >>> 0;
      const mask = prefix === 0 ? 0 : (0xFFFFFFFF << (32 - prefix)) >>> 0;
      const net = (ip & mask) >>> 0;
      const bc = (net | (~mask >>> 0)) >>> 0;
      const toStr = (n) => [n >>> 24, (n >>> 16) & 255, (n >>> 8) & 255, n & 255].join('.');
      const total = 2 ** (32 - prefix);
      const usable = prefix >= 31 ? total : total - 2;
      const maskStr = toStr(mask);
      const rows = [
        ['网络地址', toStr(net)],
        ['子网掩码', `${maskStr}（/${prefix}）`],
        ['广播地址', toStr(bc)],
        ['可用主机', prefix >= 31 ? `${usable} 个（/31、/32 特殊）` : `${usable.toLocaleString('zh-CN')} 个（${toStr(net + 1)} ~ ${toStr(bc - 1)}）`],
        ['地址总数', `${total.toLocaleString('zh-CN')} 个（2^${32 - prefix}）`],
        ['反掩码', toStr(~mask >>> 0)],
      ];
      out.innerHTML = `
        <table class="tb-table"><tbody>
          ${rows.map(([k, val]) => `<tr><th>${k}</th><td><code>${esc(val)}</code></td></tr>`).join('')}
        </tbody></table>`;
    });
  }

  /* -- 进制 -- */
  if (key === 'radix') {
    const inputs = [...root.querySelectorAll('[data-radix]')];
    const busy = { v: false };
    inputs.forEach((input) => {
      input.addEventListener('input', () => {
        if (busy.v) return;
        busy.v = true;
        const base = Number(input.getAttribute('data-radix'));
        const raw = input.value.trim().replace(/[\s_]/g, '');
        if (!raw || !/^[0-9a-fA-F]+$/.test(raw)) {
          inputs.forEach((i) => { if (i !== input) i.value = ''; });
        } else {
          const n = BigInt(parseInt(raw, base));
          inputs.forEach((i) => {
            const b = Number(i.getAttribute('data-radix'));
            i.value = i === input ? i.value : n.toString(b);
          });
        }
        busy.v = false;
      });
    });
  }
}
