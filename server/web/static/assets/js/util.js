/* ============================================================================
   util.js — 无依赖的基础能力：DOM 构造、文本转义、格式化、Markdown 渲染、图标。
   这一层不碰网络，也不碰业务，纯函数为主，谁都能用。
   ========================================================================= */

/* ----------------------------------------------------------- DOM / 转义 -- */

/**
 * 把一个 HTML 字符串变成元素。
 *
 * 全站只用这一条路径生成节点：模板字符串负责结构，动态数据一律经 esc()
 * 插值。这样"哪里可能被注入"始终只有一处需要审。
 */
export function el(html) {
  const tpl = document.createElement('template');
  tpl.innerHTML = String(html).trim();
  return tpl.content.firstElementChild;
}

/** 创建元素并按需填充，用于需要事件绑定的场景。 */
export function frag(html) {
  const tpl = document.createElement('template');
  tpl.innerHTML = String(html).trim();
  return tpl.content;
}

/**
 * 严格转义。业务数据进模板前必须过这一道。
 *
 * 引号也一并转义：属性值里出现引号会直接截断属性，
 * 那是比标签注入更隐蔽的一类问题。
 */
export function esc(value) {
  if (value === null || value === undefined) return '';
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

/**
 * 清洗链接地址。
 *
 * 只放行 http / https / 站内相对路径 / 锚点。`javascript:` 与 `data:`
 * 这类伪协议在 href 里等同于直接执行脚本，必须挡在渲染之前——
 * 后台录入的仓库地址即使已被服务端校验过，前端也不该把这个假设当依赖。
 */
export function safeUrl(url) {
  const raw = String(url ?? '').trim();
  if (!raw) return '';
  if (/^(https?:)?\/\//i.test(raw) || raw.startsWith('/') || raw.startsWith('#')) return raw;
  return '';
}

/** 用事件委托绑定：容器上挂一次，内部所有 [data-act] 都生效。 */
export function delegate(root, selector, handler) {
  root.addEventListener('click', (e) => {
    const target = e.target.closest(selector);
    if (target && root.contains(target)) handler(target, e);
  });
}

/**
 * 标记一段"由本应用自己构造、可以原样插入"的 HTML。
 *
 * 它什么都不做，存在的意义是把模板里的两类插值区分开：
 * 用户数据必须过 esc()，而我们自己拼的组件片段（分页条、图标、空态）
 * 走这里。审查的时候只要看有没有裸插值，一眼就能定位风险点。
 */
export function clampHtml(html) {
  return html === null || html === undefined ? '' : String(html);
}

/* ------------------------------------------------------------- 格式化 -- */

const pad = (n) => String(n).padStart(2, '0');

/** "2026-10-15 15:30"，用于详情页。 */
export function fmtDateTime(iso) {
  const d = parseDate(iso);
  if (!d) return '—';
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ` +
         `${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** "2026-10-15"，用于列表页。 */
export function fmtDate(iso) {
  const d = parseDate(iso);
  if (!d) return '—';
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** 相对时间：一天内显示"3 小时前"，超过则退回日期。列表页扫读更省力。 */
export function fmtRelative(iso) {
  const d = parseDate(iso);
  if (!d) return '—';
  const diff = Date.now() - d.getTime();
  if (diff < 0) return fmtDate(iso);
  const min = Math.floor(diff / 60000);
  if (min < 1) return '刚刚';
  if (min < 60) return `${min} 分钟前`;
  const hour = Math.floor(min / 60);
  if (hour < 24) return `${hour} 小时前`;
  const day = Math.floor(hour / 24);
  if (day < 30) return `${day} 天前`;
  return fmtDate(iso);
}

function parseDate(iso) {
  if (!iso) return null;
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? null : d;
}

/** 千分位：12345 → "12,345"。 */
export function fmtNum(n) {
  const v = Number(n ?? 0);
  if (!Number.isFinite(v)) return '0';
  return v.toLocaleString('zh-CN');
}

/** 星标数按中文习惯折算：12800 → "1.3万"。 */
export function fmtCompact(n) {
  const v = Number(n ?? 0);
  if (!Number.isFinite(v)) return '0';
  if (Math.abs(v) >= 10000) return (v / 10000).toFixed(1).replace(/\.0$/, '') + '万';
  return v.toLocaleString('zh-CN');
}

/** 字节数：1536 → "1.5 KB"。 */
export function fmtSize(bytes) {
  const v = Number(bytes ?? 0);
  if (!Number.isFinite(v) || v <= 0) return '未知';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let size = v;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  const fixed = i === 0 ? 0 : (size < 10 ? 1 : size < 100 ? 1 : 0);
  return `${size.toFixed(fixed)} ${units[i]}`;
}

/** 秒 → "1 分 24 秒"。 */
export function fmtDuration(sec) {
  const s = Math.max(0, Math.floor(Number(sec) || 0));
  if (s < 60) return `${s} 秒`;
  const m = Math.floor(s / 60);
  const rest = s % 60;
  return rest ? `${m} 分 ${rest} 秒` : `${m} 分`;
}

/** 由标题生成稳定的头像底色索引，避免同一作者每次刷新换颜色。 */
export function hashIndex(str, mod) {
  let h = 0;
  const s = String(str ?? '');
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return h % mod;
}

/** 取首字符作头像文字，兼容中英文。 */
export function initial(name) {
  const s = String(name ?? '').trim();
  return s ? s[0].toUpperCase() : '?';
}

/* -------------------------------------------------------------- 图标 -- */

const ICON_PATHS = {
  home:     '<path d="M3 10.5L12 3l9 7.5"/><path d="M5 9.5V20a1 1 0 001 1h3.5v-6h5v6H18a1 1 0 001-1V9.5"/>',
  terminal: '<rect x="3" y="4" width="18" height="16" rx="2.5"/><path d="M7.5 9.5l2.6 2.5-2.6 2.5"/><path d="M12.8 14.8h4"/>',
  quiz:     '<circle cx="12" cy="12" r="9"/><path d="M9.6 9.3a2.5 2.5 0 013.9-.6c.9.8 1 2.1.2 2.9-.6.6-1.7 1-1.7 2.1"/><path d="M12 17.2h.01"/>',
  news:     '<path d="M4 5.5h11.5v13H5.5A1.5 1.5 0 014 17V5.5z"/><path d="M15.5 8.5H20v8.5a1.5 1.5 0 01-1.5 1.5h-3"/><path d="M6.8 9h6M6.8 12h6M6.8 15h4"/>',
  tools:    '<path d="M14.7 6.3a4 4 0 01-5.4 5.4L5 16v3h3l4.3-4.3a4 4 0 015.4-5.4l-2.4 2.4 1.7 1.7 2.4-2.4"/>',
  download: '<path d="M12 4v10"/><path d="M8.2 10.5L12 14.3l3.8-3.8"/><path d="M4.5 17.5V19a1.5 1.5 0 001.5 1.5h12a1.5 1.5 0 001.5-1.5v-1.5"/>',
  search:   '<circle cx="11" cy="11" r="7"/><path d="M20 20l-3.6-3.6"/>',
  star:     '<path d="M12 3.8l2.6 5.3 5.9.9-4.3 4.1 1 5.8-5.2-2.7-5.2 2.7 1-5.8L3.5 10l5.9-.9z"/>',
  repo:     '<circle cx="6" cy="6" r="2.5"/><circle cx="6" cy="18" r="2.5"/><circle cx="18" cy="9" r="2.5"/><path d="M6 8.5v7"/><path d="M15.7 10.4c-.8 2.6-3 3.1-5.2 3.4"/>',
  link:     '<path d="M10.5 13.5a3.5 3.5 0 005 0l3-3a3.5 3.5 0 00-5-5l-1 1"/><path d="M13.5 10.5a3.5 3.5 0 00-5 0l-3 3a3.5 3.5 0 005 5l1-1"/>',
  check:    '<path d="M4.5 12.5l5 5 10-11"/>',
  close:    '<path d="M6 6l12 12M18 6L6 18"/>',
  plus:     '<path d="M12 5v14M5 12h14"/>',
  left:     '<path d="M14.5 6l-6 6 6 6"/>',
  right:    '<path d="M9.5 6l6 6-6 6"/>',
  eye:      '<path d="M2.5 12S6 5.8 12 5.8 21.5 12 21.5 12 18 18.2 12 18.2 2.5 12 2.5 12z"/><circle cx="12" cy="12" r="3"/>',
  heart:    '<path d="M12 19.5S4 14.8 4 9.7A4.2 4.2 0 0112 7.4a4.2 4.2 0 018 2.3c0 5.1-8 9.8-8 9.8z"/>',
  clock:    '<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 1.8"/>',
  user:     '<circle cx="12" cy="8.2" r="3.6"/><path d="M4.8 20a7.2 7.2 0 0114.4 0"/>',
  logout:   '<path d="M15 5.5H6.5A1.5 1.5 0 005 7v10a1.5 1.5 0 001.5 1.5H15"/><path d="M14 12h7"/><path d="M18.5 9l2.5 3-2.5 3"/>',
  alert:    '<circle cx="12" cy="12" r="9"/><path d="M12 7.6v5.2"/><path d="M12 16.2h.01"/>',
  refresh:  '<path d="M20 11a8 8 0 10-2.2 6.4"/><path d="M20 5.5V11h-5.5"/>',
  play:     '<path d="M8 5.5l11 6.5-11 6.5z"/>',
  list:     '<path d="M8.5 6.5h11M8.5 12h11M8.5 17.5h11"/><path d="M4.5 6.5h.01M4.5 12h.01M4.5 17.5h.01"/>',
  tag:      '<path d="M4 11.2V5a1 1 0 011-1h6.2a1 1 0 01.7.3l7.8 7.8a1 1 0 010 1.4l-6.2 6.2a1 1 0 01-1.4 0L4.3 11.9a1 1 0 01-.3-.7z"/><path d="M8 8h.01"/>',
  book:     '<path d="M5 4.5h9.5A2.5 2.5 0 0117 7v13H7.5A2.5 2.5 0 005 22.5z" /><path d="M5 4.5A1.5 1.5 0 016.5 3H17v4"/>',
  calendar: '<rect x="3.8" y="5.5" width="16.4" height="14.5" rx="2"/><path d="M3.8 10h16.4"/><path d="M8.5 3.5v4M15.5 3.5v4"/>',
  layers:   '<path d="M12 3.5l8.5 4.3-8.5 4.4-8.5-4.4z"/><path d="M4.5 12.6l7.5 3.9 7.5-3.9"/>',
  package:  '<path d="M20.5 8.2v7.6a1.5 1.5 0 01-.8 1.3l-7 3.6a1.5 1.5 0 01-1.4 0l-7-3.6a1.5 1.5 0 01-.8-1.3V8.2"/><path d="M3.9 7.5l8.1 4 8.1-4-8.1-4z"/><path d="M12 15.5v5"/>',
  shield:   '<path d="M12 3.5l7 2.6v5.4c0 4.3-2.9 7.4-7 9-4.1-1.6-7-4.7-7-9V6.1z"/><path d="M9 12l2.2 2.2L15.2 10"/>',
  key:      '<circle cx="8" cy="12" r="3.2"/><path d="M11.2 12H20"/><path d="M17.2 12v2.8"/><path d="M20 12v2"/>',
  activity: '<path d="M3.5 12h4l2.4-6 4.2 12 2.4-6h4"/>',
  chevron:  '<path d="M8 10l4 4 4-4"/>',
  filter:   '<path d="M4 6h16"/><path d="M7 12h10"/><path d="M10 18h4"/>',
  info:     '<circle cx="12" cy="12" r="9"/><path d="M12 11v5.2"/><path d="M12 7.8h.01"/>',
  trash:    '<path d="M5 7.5h14"/><path d="M9.5 7.5V5.8A1.3 1.3 0 0110.8 4.5h2.4a1.3 1.3 0 011.3 1.3v1.7"/><path d="M6.8 7.5l.8 11a1.5 1.5 0 001.5 1.4h5.8a1.5 1.5 0 001.5-1.4l.8-11"/>',
  server:   '<rect x="3.5" y="4.5" width="17" height="6" rx="1.8"/><rect x="3.5" y="13.5" width="17" height="6" rx="1.8"/><path d="M7 7.5h.01M7 16.5h.01"/>',
  target:   '<circle cx="12" cy="12" r="8.5"/><circle cx="12" cy="12" r="4.5"/><circle cx="12" cy="12" r=".6"/>',
  flame:    '<path d="M12 20.5c3.6 0 6-2.3 6-5.4 0-3.9-4-5.2-3.2-9.6-2.1.6-4 2.5-4 4.8 0 1 .3 1.7.3 2.4 0 1-.8 1.7-1.7 1.7-1 0-1.7-.8-1.7-2C6.5 13.6 6 14.9 6 16.1c0 2.6 2.4 4.4 6 4.4z"/>',
  chat:     '<path d="M20.5 12.3c0 4-3.8 7.2-8.5 7.2-1 0-2-.1-2.9-.4L4 21l1.6-3.7a6.6 6.6 0 01-2.1-4.9c0-4 3.8-7.2 8.5-7.2s8.5 3.2 8.5 7.1z"/><path d="M8.7 12h.01M12 12h.01M15.3 12h.01"/>',
  thumb:    '<path d="M7 10.5v9.5H4.5A1.5 1.5 0 013 18.5v-6.5a1.5 1.5 0 011.5-1.5H7z"/><path d="M7 10.5l4.3-6a1.6 1.6 0 012.8 1.2l-.8 3.3h4.9a1.8 1.8 0 011.8 2.2l-1.4 6.6a2 2 0 01-2 1.6H7"/>',
  edit:     '<path d="M4.5 19.5h4l10-10a2.1 2.1 0 00-3-3l-10 10z"/><path d="M14.3 8.7l3 3"/>',
};

/**
 * 生成 SVG 图标。
 * size 之外的一切都走 currentColor，颜色由 CSS 的 color 决定，
 * 这样一个图标在整个页面里不需要任何变体。
 */
export function icon(name, size = 16, extraClass = '') {
  const path = ICON_PATHS[name] || ICON_PATHS.info;
  return `<svg class="ic ${extraClass}" viewBox="0 0 24 24" width="${size}" height="${size}" ` +
         `fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" ` +
         `stroke-linejoin="round" aria-hidden="true">${path}</svg>`;
}

/** 栏目图标名到本地图标集的映射。服务端给的是语义名，前端负责落地成图形。 */
export function channelIcon(name) {
  const map = { terminal: 'terminal', quiz: 'quiz', news: 'news', tools: 'tools', download: 'download' };
  return icon(map[name] || 'layers');
}

/**
 * 管理员在正文页看到的「去后台改这篇」按钮。
 *
 * 为什么要在前台放入口：内容有 96 篇教程 + 10557 道题，管理员发现错别字时，
 * 从"这篇是哪一条"倒推后台列表再搜标题，一次要十几秒；直接带 id 跳过去是一秒。
 * 权限由调用方用 isAdmin() 判断，真正的拦截仍在后端。
 */
export function adminEditHTML(href, label = '编辑') {
  return `<a class="btn btn-sm admin-edit" href="${esc(href)}"
    title="管理员：跳转到后台修改这篇内容">${icon('edit', 14)}${esc(label)}</a>`;
}

/* ----------------------------------------------------------- Markdown -- */

/**
 * 精简 Markdown 渲染器。
 *
 * 刻意不引外部库：一是零构建下不希望多一个 CDN 依赖（离线就打不开页面），
 * 二是知识库正文只需要标题/代码块/列表/表格这几样，一个完整的 CommonMark
 * 实现属于杀鸡用牛刀。
 *
 * 安全前提：**先整体转义，再拼我们自己的标签**。顺序反过来的话，
 * 正文里写的 <script> 就会变成真的脚本。
 */
export function renderMarkdown(src) {
  if (!src) return '';
  let text = String(src).replace(/\r\n?/g, '\n');

  // ① 摘出围栏代码块。必须最先做：代码块里的 # * | 都不该被后续规则解释。
  const blocks = [];
  text = text.replace(/```([\w+#.-]*)[ \t]*\n?([\s\S]*?)```/g, (_m, lang, code) => {
    const body = code.replace(/\n$/, '');
    const idx = blocks.push(
      `<pre data-lang="${esc(lang)}"><code>${esc(body)}</code></pre>`
    ) - 1;
    return `\u0000${idx}\u0000`;
  });

  // ② 整体转义。此后 text 里不存在任何真实标签，可以放心拼接。
  text = esc(text);

  const lines = text.split('\n');
  const out = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];
    const trimmed = line.trim();

    // 代码块占位符
    const ph = /^\u0000(\d+)\u0000$/.exec(trimmed);
    if (ph) { out.push(blocks[Number(ph[1])] ?? ''); i++; continue; }

    if (!trimmed) { i++; continue; }

    // 标题：文章页已有 h1 做主标题，这里整体下沉一级
    let m = /^(#{1,6})\s+(.*)$/.exec(trimmed);
    if (m) {
      const level = Math.min(m[1].length + 1, 6);
      const id = slugify(stripTags(inline(m[2])));
      out.push(`<h${level} id="${esc(id)}">${inline(m[2])}</h${level}>`);
      i++; continue;
    }

    // 分割线
    if (/^([-*_])\1{2,}$/.test(trimmed.replace(/\s/g, ''))) {
      out.push('<hr />'); i++; continue;
    }

    // 引用块：连续的 > 行合成一段
    if (/^&gt;\s?/.test(trimmed)) {
      const buf = [];
      while (i < lines.length && /^\s*&gt;\s?/.test(lines[i])) {
        buf.push(lines[i].replace(/^\s*&gt;\s?/, ''));
        i++;
      }
      out.push(`<blockquote>${buf.map((l) => inline(l)).join('<br />')}</blockquote>`);
      continue;
    }

    // 表格：本行含 |，下一行是分隔行
    if (trimmed.includes('|') && i + 1 < lines.length && isTableDivider(lines[i + 1])) {
      const head = splitRow(trimmed);
      i += 2;
      const rows = [];
      while (i < lines.length && lines[i].includes('|') && lines[i].trim()) {
        rows.push(splitRow(lines[i].trim()));
        i++;
      }
      out.push(
        '<table><thead><tr>' +
        head.map((c) => `<th>${inline(c)}</th>`).join('') +
        '</tr></thead><tbody>' +
        rows.map((r) => '<tr>' + r.map((c) => `<td>${inline(c)}</td>`).join('') + '</tr>').join('') +
        '</tbody></table>'
      );
      continue;
    }

    // 列表：连续的同类项合成一个 ol / ul。
    // 项与项之间允许隔空行（宽松列表）：写「1. …」的人习惯每条之间空一行，
    // 不处理的话每项各成一条 <ol>，页面上全显示成"1."。
    if (/^\s*([-*+]|\d+[.)])\s+/.test(line)) {
      const ordered = /^\s*\d+[.)]\s+/.test(line);
      // 同一列表只吞同类标记的行：无序列表后面跟编号行时不把编号吞成圆点。
      const isItem = (l) => ordered ? /^\s*\d+[.)]\s+/.test(l) : /^\s*[-*+]\s+/.test(l);
      const items = [];
      while (i < lines.length && (isItem(lines[i]) ||
             (lines[i].trim() === '' && i + 1 < lines.length && isItem(lines[i + 1])))) {
        if (isItem(lines[i])) {
          items.push(lines[i].replace(/^\s*([-*+]|\d+[.)])\s+/, ''));
        }
        i++;
      }
      const tag = ordered ? 'ol' : 'ul';
      out.push(`<${tag}>${items.map((t) => `<li>${inline(t)}</li>`).join('')}</${tag}>`);
      continue;
    }

    // 段落：吃到空行或下一个块级标记为止
    const para = [];
    while (i < lines.length && lines[i].trim() &&
           !/^(#{1,6})\s+/.test(lines[i].trim()) &&
           !/^\s*([-*+]|\d+[.)])\s+/.test(lines[i]) &&
           !/^\s*&gt;\s?/.test(lines[i]) &&
           !/^\u0000\d+\u0000$/.test(lines[i].trim()) &&
           !(lines[i].includes('|') && i + 1 < lines.length && isTableDivider(lines[i + 1]))) {
      para.push(lines[i].trim());
      i++;
    }
    if (para.length) out.push(`<p>${inline(para.join('<br />'))}</p>`);
  }

  return out.join('\n');
}

/** 行内标记。入参必须是**已转义**的文本。 */
function inline(s) {
  return String(s)
    // 行内代码优先：反引号里的内容不再参与其它标记解析
    .replace(/`([^`]+)`/g, (_m, code) => `<code>${code}</code>`)
    .replace(/!\[([^\]]*)\]\(([^)\s]+)\)/g, (m, alt, url) => {
      const u = safeUrl(url);
      return u ? `<img src="${esc(u)}" alt="${alt}" loading="lazy" />` : m;
    })
    .replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (m, text, url) => {
      const u = safeUrl(url);
      return u ? `<a href="${esc(u)}" target="_blank" rel="noopener noreferrer">${text}</a>` : text;
    })
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/(^|[^*\w])\*([^*\n]+)\*/g, '$1<em>$2</em>')
    .replace(/~~([^~]+)~~/g, '<del>$1</del>');
}

function isTableDivider(line) {
  return /^\s*\|?[\s:|-]*-[\s:|-]*\|?\s*$/.test(line) && line.includes('-');
}

function splitRow(line) {
  let s = line;
  if (s.startsWith('|')) s = s.slice(1);
  if (s.endsWith('|')) s = s.slice(0, -1);
  return s.split('|').map((c) => c.trim());
}

function stripTags(html) {
  return String(html).replace(/<[^>]*>/g, '');
}

/** 从标题生成锚点 id，供文章内的目录跳转使用。中文原样保留可读性更好。 */
export function slugify(text) {
  const s = String(text ?? '').trim().toLowerCase();
  return s
    .replace(/[\s\u3000]+/g, '-')
    .replace(/[^\p{L}\p{N}-]+/gu, '')
    .replace(/-{2,}/g, '-')
    .replace(/^-|-$/g, '') || 'section';
}

/**
 * splitTags 按中英文逗号、顿号、分号、空白切分，顺带去重、去空、限 5 个。
 *
 * 用户从文档里复制标签时这几种分隔符会混着出现，只认英文逗号会让「卡, 很, 慢」
 * 变成一整条 tag 存进库，之后筛选永远命中不到。
 * 放在 util.js 是因为投稿表单与后台批量导入都要用同一套切分规则——
 * 两处各写一份迟早会不一致（比如一处认顿号一处不认，同一个标签进库成两条）。
 */
export function splitTags(raw) {
  const seen = new Set();
  const out = [];
  String(raw || '')
    .split(/[,，、;；\s]+/)
    .map((s) => s.trim())
    .filter(Boolean)
    .forEach((s) => {
      const key = s.toLowerCase();
      if (seen.has(key) || out.length >= 5) return;
      seen.add(key);
      out.push(s);
    });
  return out;
}

/* -------------------------------------------------------------- 杂项 -- */

/** 防抖：搜索框输入时别每敲一个字就发一次请求。 */
export function debounce(fn, wait = 320) {
  let timer = 0;
  return (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), wait);
  };
}

/** 把对象拼成查询串，空值自动丢掉。 */
export function qs(params) {
  const sp = new URLSearchParams();
  Object.entries(params || {}).forEach(([k, v]) => {
    if (v === null || v === undefined || v === '' || v === false) return;
    sp.set(k, String(v));
  });
  const s = sp.toString();
  return s ? `?${s}` : '';
}

/**
 * 改写当前 hash 上的查询参数。
 *
 * 筛选与翻页一律走这里改地址，而不是就地重渲染：地址栏会跟着变，
 * 用户能直接复制给别人，也能用回退键退回上一个筛选条件。
 * 除非显式指定 page，否则任何条件变化都把页码归零——没人想在
 * 换了分类之后还停在第 7 页。
 */
export function setHashQuery(patch, { keepPage = false } = {}) {
  const raw = location.hash.slice(1) || '/';
  const [path, search] = raw.split('?');
  const sp = new URLSearchParams(search || '');

  Object.entries(patch || {}).forEach(([k, v]) => {
    if (v === null || v === undefined || v === '') sp.delete(k);
    else sp.set(k, String(v));
  });
  if (!keepPage) sp.delete('page');

  const query = sp.toString();
  location.hash = path + (query ? `?${query}` : '');
}

/** 难度 / 题型的中文名。后端存英文枚举，展示层负责翻译。 */
export const DIFFICULTY_LABEL = { easy: '简单', medium: '中等', hard: '困难' };
export const QUIZ_TYPE_LABEL = { single: '单选', multiple: '多选', judge: '判断' };
export const DEPLOY_LABEL = {
  binary: '二进制', docker: 'Docker', helm: 'Helm',
  'k8s-operator': 'Operator', source: '源码编译', cloud: '云托管',
};
/**
 * 教程集的深度级别。与后端 model.ToolBookLevels() 一一对应。
 *
 * 用「入门 / 进阶 / 深入」而不是直译 beginner/advanced/expert：
 * expert 直译成「专家」会让人以为要先通过认证，而这里只表示内容深度。
 * 前后台共用同一份文案——两处各写一遍，改词时必然漏一处。
 */
export const BOOK_LEVEL_LABEL = { beginner: '入门', advanced: '进阶', expert: '深入' };
