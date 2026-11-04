/* ============================================================================
   pages-checklist.js — 值班巡检（日检 / 周检 / 月检）。

   打卡记录按「日期 + 清单」存 localStorage，不登录也能用：
   巡检是值班动作，不该因为没登录就做不了记录。
   换班时用「导出交接记录」把今天的勾选状态导成 Markdown，
   直接贴进交接群或工单——这是这个页面存在的真正理由。

   存储形状：{ "2026-11-04": { "daily": { "load": 1 }, "weekly": {} } }
   只保留最近 45 天，避免 localStorage 越积越多（每天三份清单的键会一直长）。
   ========================================================================= */

import { esc, icon, fmtNum } from './util.js';
import { CHECKLISTS, findChecklist, itemCountOf, TOTAL_ITEMS } from './checklist-data.js';

const STORE_KEY = 'opskb.checklist.v1';
const KEEP_DAYS = 45;

/* --------------------------------------------------------- 日期与存储 -- */

/** 本地日期串。不能用 toISOString——那是 UTC，晚上 8 点后东八区会变成第二天。 */
function todayKey(d = new Date()) {
  const p = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

function readStore() {
  try {
    const raw = localStorage.getItem(STORE_KEY);
    const obj = raw ? JSON.parse(raw) : null;
    return obj && typeof obj === 'object' ? obj : {};
  } catch {
    return {};
  }
}

function writeStore(store) {
  try {
    // 顺手清理过老的数据：按日期键排序后只留最后 45 天。
    const keys = Object.keys(store).sort();
    if (keys.length > KEEP_DAYS) {
      keys.slice(0, keys.length - KEEP_DAYS).forEach((k) => { delete store[k]; });
    }
    localStorage.setItem(STORE_KEY, JSON.stringify(store));
  } catch {
    // 隐私模式写不进去：页面仍然可以勾，只是刷新后会丢。不弹窗打扰。
  }
}

/** 取某天某份清单的勾选表：{ itemId: true }。 */
function checksOf(listId, day = todayKey()) {
  const store = readStore();
  const dayEntry = store[day];
  const one = dayEntry && dayEntry[listId];
  return one && typeof one === 'object' ? one : {};
}

function setCheck(listId, itemId, checked, day = todayKey()) {
  const store = readStore();
  if (!store[day]) store[day] = {};
  if (!store[day][listId]) store[day][listId] = {};
  if (checked) store[day][listId][itemId] = 1;
  else delete store[day][listId][itemId];
  writeStore(store);
}

/* ------------------------------------------------------------- 统计 -- */

function statsOf(list, checks) {
  const total = itemCountOf(list);
  let done = 0;
  const criticalLeft = [];
  list.groups.forEach((g) => {
    g.items.forEach((it) => {
      if (checks[it.id]) done += 1;
      else if (it.critical) criticalLeft.push(it.name);
    });
  });
  return {
    done,
    total,
    percent: total ? Math.round((done / total) * 100) : 0,
    criticalLeft,
  };
}

function groupStats(group, checks) {
  const total = group.items.length;
  const done = group.items.filter((it) => checks[it.id]).length;
  return { done, total, percent: total ? Math.round((done / total) * 100) : 0 };
}

/* ------------------------------------------------------------- 零件 -- */

function ringHTML(percent, size = 92) {
  const p = Math.max(0, Math.min(100, Number(percent) || 0));
  return `<div class="cl-ring" style="--ring-size:${size}px;--ring-pct:${p}%">
    <div class="cl-ring-hole"><strong>${p}<span>%</span></strong></div>
  </div>`;
}

function itemHTML(item, checks) {
  const on = Boolean(checks[item.id]);
  const id = `cl-${item.id}`;
  return `<div class="cl-item${on ? ' is-done' : ''}${item.critical ? ' is-critical' : ''}"
       data-cl-row="${esc(item.id)}">
    <input class="cl-check" type="checkbox" id="${esc(id)}" data-cl-item="${esc(item.id)}"${on ? ' checked' : ''}>
    <div class="cl-main">
      <label class="cl-name" for="${esc(id)}">
        ${esc(item.name)}
        ${item.critical ? '<span class="cl-crit">必检</span>' : ''}
      </label>
      <div class="cl-cmdrow">
        <code class="cl-cmd">${esc(item.cmd)}</code>
        <button class="btn btn-sm cl-copy" type="button" data-copy="${esc(item.cmd)}"
                title="复制命令" aria-label="复制命令">${icon('list', 12)}复制</button>
      </div>
      <p class="cl-expect"><span>判定</span>${esc(item.expect)}</p>
    </div>
    ${item.runbook
      ? `<a class="cl-rb" href="${esc(item.runbook)}" title="异常时按剧本排查">
           ${icon('activity', 13)}${esc(item.runbookText || '剧本')}</a>`
      : ''}
  </div>`;
}

function groupHTML(group, checks) {
  const gs = groupStats(group, checks);
  return `<section class="cl-group" data-cl-group="${esc(group.id)}">
    <header class="cl-group-head">
      <h2>${esc(group.name)}</h2>
      <span class="cl-group-count" data-cl-gcount="${esc(group.id)}">${gs.done}/${gs.total}</span>
    </header>
    <div class="cl-group-bar"><i data-cl-gbar="${esc(group.id)}" style="width:${gs.percent}%"></i></div>
    <div class="cl-items">${group.items.map((it) => itemHTML(it, checks)).join('')}</div>
  </section>`;
}

/* --------------------------------------------------------------- 页面 -- */

export async function checklistPage(ctx) {
  const want = ctx.params.get('t') || 'daily';
  const list = findChecklist(want) || CHECKLISTS[0];
  const day = todayKey();
  const checks = checksOf(list.id);
  const st = statsOf(list, checks);

  const html = `
    <div class="page-head">
      <h1>值班巡检</h1>
      <p class="lede">把「这一班该看什么」固化成清单：每项都带命令与判定标准，勾选记录只存在本机。
        共 ${fmtNum(TOTAL_ITEMS)} 项，日常真正要跑的是日检的 ${fmtNum(itemCountOf(CHECKLISTS[0]))} 项。</p>
    </div>

    <div class="cl-bar">
      <div class="cl-tabs" role="tablist" aria-label="巡检类型">
        ${CHECKLISTS.map((c) => `<button type="button" role="tab"
          class="cl-tab${c.id === list.id ? ' is-active' : ''}"
          aria-selected="${c.id === list.id}"
          data-cl-tab="${esc(c.id)}">${esc(c.name)}<span>${fmtNum(itemCountOf(c))} 项</span></button>`).join('')}
      </div>
      <div class="cl-bar-actions">
        <button class="btn btn-sm" type="button" data-cl-export>${icon('download', 13)}导出交接记录</button>
        <button class="btn btn-sm" type="button" data-cl-reset>${icon('refresh', 13)}清空本张</button>
      </div>
    </div>

    <section class="cl-hero">
      ${ringHTML(st.percent, 96)}
      <div class="cl-hero-main">
        <div class="cl-hero-top">
          <h2>${esc(list.name)}<span class="cl-hero-cadence">${esc(list.cadence)}</span></h2>
          <p class="cl-hero-date" data-cl-date>${esc(day)}</p>
        </div>
        <p class="cl-hero-desc">${esc(list.desc)}</p>
        <p class="cl-hero-count" data-cl-count>已完成 ${st.done} / ${st.total} 项</p>
        <div class="cl-hero-alert" data-cl-alert${st.criticalLeft.length ? '' : ' hidden'}>
          ${icon('alert', 13)}
          <span data-cl-alert-text>${st.criticalLeft.length
            ? `还有 ${st.criticalLeft.length} 个必检项没勾：${esc(st.criticalLeft.slice(0, 3).join('、'))}${st.criticalLeft.length > 3 ? ' 等' : ''}`
            : ''}</span>
        </div>
      </div>
    </section>

    <div class="cl-groups" data-cl-groups>
      ${list.groups.map((g) => groupHTML(g, checks)).join('')}
    </div>

    <p class="cl-foot">${icon('info', 13)}
      勾选只存在这台浏览器（最近 ${KEEP_DAYS} 天）。换班请先导出交接记录，勾选不会跟着人走。</p>
  `;

  const mount = (root) => {
    const groups = root.querySelector('[data-cl-groups]');
    const alertBox = root.querySelector('[data-cl-alert]');
    const alertText = root.querySelector('[data-cl-alert-text]');
    const countText = root.querySelector('[data-cl-count]');
    const ring = root.querySelector('.cl-ring');

    // 只更新汇总，不重绘整个列表：重绘会把用户刚展开/滚到的位置冲掉。
    const refresh = () => {
      const now = checksOf(list.id);
      const s = statsOf(list, now);
      if (countText) countText.textContent = `已完成 ${s.done} / ${s.total} 项`;
      if (ring) ring.style.setProperty('--ring-pct', `${s.percent}%`);
      const hole = ring && ring.querySelector('strong');
      if (hole) hole.innerHTML = `${s.percent}<span>%</span>`;
      if (alertBox && alertText) {
        if (s.criticalLeft.length) {
          alertBox.hidden = false;
          alertText.textContent = `还有 ${s.criticalLeft.length} 个必检项没勾：`
            + s.criticalLeft.slice(0, 3).join('、') + (s.criticalLeft.length > 3 ? ' 等' : '');
        } else {
          alertBox.hidden = true;
          alertText.textContent = '';
        }
      }
      list.groups.forEach((g) => {
        const gs = groupStats(g, now);
        const cnt = root.querySelector(`[data-cl-gcount="${g.id}"]`);
        if (cnt) cnt.textContent = `${gs.done}/${gs.total}`;
        const bar = root.querySelector(`[data-cl-gbar="${g.id}"]`);
        if (bar) bar.style.width = `${gs.percent}%`;
      });
    };

    groups.addEventListener('change', (e) => {
      const box = e.target.closest('[data-cl-item]');
      if (!box) return;
      const itemId = box.getAttribute('data-cl-item');
      setCheck(list.id, itemId, box.checked);
      const row = box.closest('[data-cl-row]');
      if (row) row.classList.toggle('is-done', box.checked);
      refresh();
    });

    // 复制命令：与速查手册同一套兜底（http 下 clipboard API 不可用）
    groups.addEventListener('click', async (e) => {
      const btn = e.target.closest('[data-copy]');
      if (!btn) return;
      e.preventDefault();
      const text = btn.getAttribute('data-copy');
      let ok = false;
      try {
        await navigator.clipboard.writeText(text);
        ok = true;
      } catch {
        const ta = document.createElement('textarea');
        ta.value = text;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        ok = document.execCommand('copy');
        ta.remove();
      }
      const old = btn.innerHTML;
      btn.innerHTML = ok ? `${icon('check', 12)}已复制` : `${icon('close', 12)}失败`;
      setTimeout(() => { btn.innerHTML = old; }, 1200);
    });

    // 切清单走地址，链接可分享给同一个班次的同事
    root.querySelectorAll('[data-cl-tab]').forEach((btn) => {
      btn.addEventListener('click', () => {
        location.hash = `#/checklist?t=${encodeURIComponent(btn.getAttribute('data-cl-tab'))}`;
      });
    });

    const resetBtn = root.querySelector('[data-cl-reset]');
    if (resetBtn) {
      resetBtn.addEventListener('click', () => {
        if (!window.confirm(`清空「${list.name}」今天的勾选记录？`)) return;
        const store = readStore();
        if (store[day] && store[day][list.id]) delete store[day][list.id];
        writeStore(store);
        root.querySelectorAll('[data-cl-item]').forEach((b) => {
          b.checked = false;
          const row = b.closest('[data-cl-row]');
          if (row) row.classList.remove('is-done');
        });
        refresh();
      });
    }

    const exportBtn = root.querySelector('[data-cl-export]');
    if (exportBtn) exportBtn.addEventListener('click', () => exportHandover(list, day));
  };

  return { html, mount, title: '值班巡检' };
}

/* ----------------------------------------------------------- 导出交接 -- */

/**
 * 交接记录：一页 Markdown，未完成项与必检项排在最前面。
 *
 * 顺序刻意如此——接班的人第一眼要看到「哪些没做」，而不是「做了多少」。
 * 已完成的只列名字，不贴命令，压到两屏内，方便贴进群里。
 */
function exportHandover(list, day) {
  const checks = checksOf(list.id, day);
  const st = statsOf(list, checks);
  const lines = [];
  lines.push(`# ${list.name}交接记录 · ${day}`);
  lines.push('');
  lines.push(`完成度：${st.done}/${st.total}（${st.percent}%）`);
  if (st.criticalLeft.length) {
    lines.push('');
    lines.push(`> 未完成的必检项：${st.criticalLeft.join('、')}`);
  }
  lines.push('');

  list.groups.forEach((g) => {
    const left = g.items.filter((it) => !checks[it.id]);
    const done = g.items.filter((it) => checks[it.id]);
    lines.push(`## ${g.name}`);
    if (left.length) {
      lines.push('未完成：');
      left.forEach((it) => {
        lines.push(`- [ ] ${it.name}${it.critical ? '（必检）' : ''}`);
      });
    }
    if (done.length) {
      lines.push(`已完成：${done.map((it) => it.name).join('、')}`);
    }
    if (!left.length && !done.length) lines.push('（无）');
    lines.push('');
  });

  lines.push('---');
  lines.push(`导出时间：${new Date().toLocaleString('zh-CN')}`);
  const text = lines.join('\n');

  // 用 Blob + a[download]：不需要后端接口，也不把值班内容发到任何地方。
  try {
    const blob = new Blob([text], { type: 'text/markdown;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `巡检交接-${list.id}-${day}.md`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 3000);
  } catch {
    // 下载被拦时退回复制到剪贴板，至少还能贴出去
    if (navigator.clipboard) navigator.clipboard.writeText(text).catch(() => {});
  }
}
