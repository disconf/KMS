/* ============================================================================
   pages-weekly.js — 周报生成器。
   运维最讨厌的日常是写周报：把"做过什么"翻成"结构化汇报"。
   这里只做一件事——分区块填草稿，一键生成结构化 Markdown。
   草稿存 localStorage（自动保存），成品复制 / 下载 .md，全程不出浏览器。
   ========================================================================= */

import { esc, icon, renderMarkdown } from './util.js';
import { toastOk } from './ui.js';

const STORE_KEY = 'opskb.weekly.draft.v1';

/** 报告分区。顺序就是周报的阅读顺序：先干了什么、再出了什么事、最后要什么。 */
const SECTIONS = [
  { key: 'change',  label: '变更发布',   hint: '例：升级 nginx 至 1.26（周三晚 22:00，灰度 2 台观察 30 分钟后全量）' },
  { key: 'incident',label: '故障与处理', hint: '例：周三 14:20 API 5xx 突增，定位为数据库连接池耗尽，扩容后恢复，全程 25 分钟' },
  { key: 'inspect', label: '巡检与告警', hint: '例：例行巡检无异常；处理磁盘告警 2 起（日志清理）' },
  { key: 'improve', label: '优化与改进', hint: '例：慢查询 Top5 整理完毕，其中 3 条已加索引，P95 从 1.8s 降至 420ms' },
  { key: 'plan',    label: '下周计划',   hint: '例：完成 K8s 集群版本升级演练；输出灾备演练报告' },
];

/** 默认日期范围：本周一 ~ 周日。 */
function thisWeek() {
  const now = new Date();
  const dow = (now.getDay() + 6) % 7; // 周一=0
  const mon = new Date(now);
  mon.setDate(now.getDate() - dow);
  const sun = new Date(mon);
  sun.setDate(mon.getDate() + 6);
  const d = (x) => `${x.getFullYear()}-${String(x.getMonth() + 1).padStart(2, '0')}-${String(x.getDate()).padStart(2, '0')}`;
  return { start: d(mon), end: d(sun) };
}

/** 从分区文本生成 Markdown。空区块跳过，不让"（无）"占版面。 */
function buildMarkdown({ start, end, author, sections }) {
  const lines = [`# 运维周报（${start} ~ ${end}）`];
  if (author.trim()) lines.push(`汇报人：${author.trim()}`);
  for (const sec of SECTIONS) {
    const body = (sections[sec.key] || '').trim();
    if (!body) continue;
    lines.push('', `## ${sec.label}`);
    // 每行一条；行首已带编号/短横的不重复加，尊重写作者的格式。
    for (const raw of body.split('\n')) {
      const line = raw.trim();
      if (!line) continue;
      lines.push(/^([-*\d]|①|②|③|④|⑤)/.test(line) ? line : `- ${line}`);
    }
  }
  return lines.join('\n') + '\n';
}

export async function weeklyPage() {
  const week = thisWeek();

  const html = `
    <div class="page-head">
      <h1>运维周报生成器</h1>
      <p class="lede">按区块随手记，一键生成结构化周报。草稿自动保存在本机浏览器，成品复制走人或下载 .md。</p>
    </div>

    <div class="wk-grid">
      <div class="card wk-form">
        <div class="tb-grid">
          <div class="field">
            <label for="wk-start">开始日期</label>
            <input class="input" id="wk-start" type="date" value="${week.start}" />
          </div>
          <div class="field">
            <label for="wk-end">结束日期</label>
            <input class="input" id="wk-end" type="date" value="${week.end}" />
          </div>
          <div class="field">
            <label for="wk-author">汇报人（可选）</label>
            <input class="input" id="wk-author" maxlength="40" placeholder="姓名 / 团队名" />
          </div>
        </div>

        ${SECTIONS.map((s) => `
          <div class="field">
            <label for="wk-${s.key}">${s.label}</label>
            <textarea class="input code-input" id="wk-${s.key}" rows="3" data-wk-sec="${s.key}"
              placeholder="${esc(s.hint)}" spellcheck="false"></textarea>
          </div>`).join('')}

        <div class="tb-btns">
          <button class="btn btn-primary" type="button" data-wk="gen">${icon('edit', 14)}生成周报</button>
          <button class="btn" type="button" data-wk="clear">${icon('trash', 14)}清空草稿</button>
        </div>
        <p class="tb-hint" data-wk-status></p>
      </div>

      <div class="card wk-preview">
        <div class="wk-preview-head">
          <h2 class="tb-title">周报预览</h2>
          <div class="tb-btns">
            <button class="btn btn-sm" type="button" data-wk="copy" disabled>${icon('list', 13)}复制 Markdown</button>
            <button class="btn btn-sm" type="button" data-wk="download" disabled>${icon('download', 13)}下载 .md</button>
          </div>
        </div>
        <div class="prose wk-prose" data-wk-preview><p class="tb-hint">填好左侧内容后点「生成周报」。</p></div>
      </div>
    </div>
  `;

  const mount = (root) => {
    const $ = (id) => root.querySelector(`#${CSS.escape(id)}`);
    const preview = root.querySelector('[data-wk-preview]');
    const status = root.querySelector('[data-wk-status]');
    const copyBtn = root.querySelector('[data-wk="copy"]');
    const dlBtn = root.querySelector('[data-wk="download"]');
    let markdown = '';

    /* 草稿恢复与自动保存。字段一多手动保存按钮必然被忘掉，改为输入即存。 */
    try {
      const saved = JSON.parse(localStorage.getItem(STORE_KEY) || '{}');
      ['start', 'end', 'author'].forEach((k) => { if (saved[k] && $(`wk-${k}`)) $(`wk-${k}`).value = saved[k]; });
      Object.entries(saved.sections || {}).forEach(([k, v]) => {
        const ta = root.querySelector(`[data-wk-sec="${k}"]`);
        if (ta) ta.value = v;
      });
      if (Object.keys(saved).length) status.textContent = '已恢复上次未提交的草稿。';
    } catch { /* 草稿坏了就当没有 */ }

    const collect = () => {
      const sections = {};
      root.querySelectorAll('[data-wk-sec]').forEach((ta) => { sections[ta.getAttribute('data-wk-sec')] = ta.value; });
      return {
        start: $('wk-start').value,
        end: $('wk-end').value,
        author: $('wk-author').value,
        sections,
      };
    };

    let saveTimer = 0;
    const autoSave = () => {
      clearTimeout(saveTimer);
      saveTimer = setTimeout(() => {
        try {
          localStorage.setItem(STORE_KEY, JSON.stringify(collect()));
          status.textContent = '草稿已自动保存。';
        } catch { /* 隐私模式存不了就算了 */ }
      }, 400);
    };
    root.querySelectorAll('input, textarea').forEach((n) => n.addEventListener('input', autoSave));

    const enableActions = (on) => {
      copyBtn.disabled = !on;
      dlBtn.disabled = !on;
    };

    root.querySelector('[data-wk="gen"]').addEventListener('click', () => {
      const data = collect();
      markdown = buildMarkdown(data);
      if (markdown.split('\n').length <= 3) {
        status.textContent = '至少填一个区块再生成。';
        return;
      }
      preview.innerHTML = renderMarkdown(markdown);
      enableActions(true);
      status.textContent = '生成完毕，检查无误后复制或下载。';
    });

    copyBtn.addEventListener('click', async () => {
      if (!markdown) return;
      let ok = false;
      try {
        await navigator.clipboard.writeText(markdown);
        ok = true;
      } catch {
        const ta = document.createElement('textarea');
        ta.value = markdown;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        ok = document.execCommand('copy');
        ta.remove();
      }
      toastOk(ok ? 'Markdown 已复制' : '复制失败，请手动选择预览区文本');
    });

    dlBtn.addEventListener('click', () => {
      if (!markdown) return;
      const blob = new Blob([markdown], { type: 'text/markdown;charset=utf-8' });
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = `运维周报-${$('wk-start').value}.md`;
      a.click();
      URL.revokeObjectURL(a.href);
    });

    root.querySelector('[data-wk="clear"]').addEventListener('click', () => {
      root.querySelectorAll('[data-wk-sec]').forEach((ta) => { ta.value = ''; });
      $('wk-author').value = '';
      markdown = '';
      preview.innerHTML = '<p class="tb-hint">已清空。填好内容后点「生成周报」。</p>';
      enableActions(false);
      try { localStorage.removeItem(STORE_KEY); } catch { /* 忽略 */ }
      status.textContent = '';
    });
  };

  return { html, mount, title: '周报生成器' };
}
