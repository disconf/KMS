/* ============================================================================
   searchbox.js — 顶栏搜索框的输入建议。
   用户打的是「磁盘满了怎么办」，后端会把它翻译成「磁盘 / 硬盘 / disk」，
   这里把这些词摆出来让人直接点。空输入时给热门关键词，避免下拉一片空白。
   ========================================================================= */

import { api } from './api.js';
import { esc, icon } from './util.js';

/** 输入停顿多久才发请求。太短会在打字过程中连发，太长又显得迟钝。 */
const DEBOUNCE_MS = 200;

export function initSearchBox() {
  const form = document.getElementById('search-form');
  const input = document.getElementById('search-input');
  const panel = document.getElementById('search-suggest');
  if (!form || !input || !panel) return;

  /** 当前面板里的候选项，键盘上下键在这个数组里移动。 */
  let items = [];
  let active = -1;
  let timer = null;
  /** 请求序号。晚发出的响应可能先回来，靠它丢弃过期结果。 */
  let seq = 0;

  const close = () => {
    panel.hidden = true;
    panel.innerHTML = '';
    items = [];
    active = -1;
    input.setAttribute('aria-expanded', 'false');
  };

  const go = (value) => {
    const q = String(value || '').trim();
    if (!q) return;
    close();
    input.blur();
    location.hash = `#/search?q=${encodeURIComponent(q)}`;
  };

  const render = () => {
    if (!items.length) {
      close();
      return;
    }
    panel.innerHTML = items.map((it, i) => {
      if (it.group) {
        return `<div class="suggest-group-title">${esc(it.group)}</div>`;
      }
      const idx = i;
      return `<button type="button" class="suggest-item${idx === active ? ' is-active' : ''}"
        role="option" aria-selected="${idx === active}" data-idx="${idx}">
        <span class="suggest-ico">${icon(it.icon || 'search', 14)}</span>
        <span>${it.html}</span>
        ${it.hint ? `<span class="suggest-hint">${esc(it.hint)}</span>` : ''}
      </button>`;
    }).join('');
    panel.querySelectorAll('[data-idx]').forEach((node) => {
      node.addEventListener('mousedown', (e) => {
        // 用 mousedown 而不是 click：click 会在 input 失焦后触发，
        // 那时面板已经被关掉了，点了个寂寞。
        e.preventDefault();
        go(items[Number(node.getAttribute('data-idx'))].value);
      });
    });
    panel.hidden = false;
    input.setAttribute('aria-expanded', 'true');
  };

  /** 把匹配到的部分高亮出来，让人一眼看到为什么推荐这个词。 */
  const highlight = (text, keyword) => {
    const safe = esc(text);
    if (!keyword) return safe;
    const at = safe.toLowerCase().indexOf(esc(keyword).toLowerCase());
    if (at < 0) return safe;
    const len = esc(keyword).length;
    return `${safe.slice(0, at)}<mark>${safe.slice(at, at + len)}</mark>${safe.slice(at + len)}`;
  };

  const buildItems = (query, plan) => {
    const out = [];
    const hints = new Set((plan.terms || []).map((t) => t.toLowerCase()));

    if (query) {
      out.push({
        value: query,
        icon: 'search',
        html: `搜索「${esc(query)}」`,
      });
      out.push({ group: '相关检索词' });
    } else {
      out.push({ group: '热门关键词' });
    }

    const related = (plan.expanded || [])
      .filter((t) => t.toLowerCase() !== query.toLowerCase())
      .slice(0, 8);
    for (const term of related) {
      out.push({
        value: term,
        icon: 'tag',
        html: highlight(term, query),
        // 标出这个词是从哪一个原始概念展开出来的，用户才知道为什么给它。
        hint: hints.has(term.toLowerCase()) ? '原文' : '同义',
      });
    }

    if (!query) {
      for (const hot of (plan.hot || []).slice(0, 8)) {
        out.push({ value: hot, icon: 'flame', html: esc(hot) });
      }
    }
    return out;
  };

  const load = async (query) => {
    const token = ++seq;
    let plan;
    try {
      plan = await api.searchSuggest(query);
    } catch {
      // 建议是附加能力：接口不通就安静地收起面板，
      // 绝不能因此让用户连搜索都搜不了。
      if (token === seq) close();
      return;
    }
    if (token !== seq) return;
    items = buildItems(query, plan || {});
    active = -1;
    render();
  };

  form.addEventListener('submit', (e) => {
    e.preventDefault();
    if (active >= 0 && items[active] && !items[active].group) {
      go(items[active].value);
      return;
    }
    go(input.value);
  });

  input.addEventListener('input', () => {
    clearTimeout(timer);
    const q = input.value.trim();
    timer = setTimeout(() => load(q), DEBOUNCE_MS);
  });

  input.addEventListener('focus', () => {
    if (panel.hidden) load(input.value.trim());
  });

  input.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      close();
      return;
    }
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
    e.preventDefault();

    const selectable = items.map((it, i) => (it.group ? -1 : i)).filter((i) => i >= 0);
    if (!selectable.length) return;
    const pos = selectable.indexOf(active);
    const next = e.key === 'ArrowDown'
      ? selectable[(pos + 1) % selectable.length]
      : selectable[(pos - 1 + selectable.length) % selectable.length];
    active = next;
    render();
  });

  // 点面板外部、或输入框失焦后收起。
  document.addEventListener('click', (e) => {
    if (!form.contains(e.target)) close();
  });
  input.addEventListener('blur', () => {
    // 延迟一拍，让 mousedown 里的选择先执行完。
    setTimeout(() => {
      if (!form.contains(document.activeElement)) close();
    }, 120);
  });
}
