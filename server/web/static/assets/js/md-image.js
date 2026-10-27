/**
 * md-image.js — 给 Markdown 输入框装上图片上传：按钮 / 粘贴 / 拖入。
 *
 * 为什么单独一个模块：
 * 站内至少四处要写 Markdown（文章投稿、悬赏描述、问答提问与回答、后台文章编辑），
 * 每处都自己写一遍"粘贴板取图 → 上传 → 插入 ![]()"必然会走形。
 * 这里只依赖 api.js / ui.js / util.js，不反向依赖任何页面模块。
 *
 * 交互约定：
 *   - 截图后直接 Ctrl+V 粘贴，图会先插一条"上传中"占位，成功后就地替换成图片语法；
 *   - 也可以点「插入图片」选文件，或把图片拖进输入框；
 *   - 失败不留残渣：占位会被删掉，只弹一条错误提示。
 */
import { ApiError, isLoggedIn, uploadFile } from './api.js';
import { toastError } from './ui.js';
import { icon } from './util.js';

/** 正文插图场景：≤5MB，JPG / PNG / WebP / GIF。与后端 storage 策略保持一致。 */
const DEFAULT_SCENE = 'article_image';
const DEFAULT_MAX_MB = 5;
const ALLOWED_TYPES = /^image\/(jpeg|png|webp|gif)$/i;

/**
 * 给输入框装上图片上传能力，并在上方插入一条工具栏。
 *
 * @param {Element} root 容器（页面根节点或弹窗元素），用于限定查询范围
 * @param {string|HTMLTextAreaElement} target 选择器或 textarea 本身
 * @param {{scene?: string, maxSizeMB?: number}} [options]
 * @returns {{destroy: () => void} | null} 找不到输入框时返回 null
 */
export function enableMarkdownImages(root, target, options = {}) {
  const el = typeof target === 'string' ? root.querySelector(target) : target;
  if (!el) return null;

  const scene = options.scene || DEFAULT_SCENE;
  const maxBytes = (options.maxSizeMB || DEFAULT_MAX_MB) * 1024 * 1024;

  const tools = document.createElement('div');
  tools.className = 'md-tools';
  tools.innerHTML = `
    <button class="btn btn-sm" type="button" data-md-pick>${icon('plus', 14)}插入图片</button>
    <span class="md-tools-hint">可直接粘贴截图（Ctrl+V），或把图片拖进来</span>
    <input type="file" accept="image/*" multiple hidden data-md-file />
  `;
  el.parentNode.insertBefore(tools, el);

  const pickBtn = tools.querySelector('[data-md-pick]');
  const fileInput = tools.querySelector('[data-md-file]');

  let seq = 0;
  let busy = false;

  pickBtn.addEventListener('click', () => fileInput.click());

  fileInput.addEventListener('change', () => {
    const files = Array.from(fileInput.files || []);
    fileInput.value = '';           // 允许连续选同一个文件
    if (files.length) uploadAll(files);
  });

  el.addEventListener('paste', (e) => {
    const files = imagesFrom(e.clipboardData);
    if (!files.length) return;
    // 有图就交给上传：截图粘贴时这段文本只是文件名的残留，插进正文反而更乱。
    e.preventDefault();
    uploadAll(files);
  });

  // 拖拽：dragover 必须 preventDefault，否则浏览器会直接打开图片、离开当前页。
  let dragDepth = 0;
  el.addEventListener('dragenter', (e) => {
    if (!hasFiles(e.dataTransfer)) return;
    e.preventDefault();
    dragDepth += 1;
    el.classList.add('is-drop-target');
  });
  el.addEventListener('dragover', (e) => {
    if (!hasFiles(e.dataTransfer)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'copy';
  });
  el.addEventListener('dragleave', () => {
    dragDepth = Math.max(0, dragDepth - 1);
    if (dragDepth === 0) el.classList.remove('is-drop-target');
  });
  el.addEventListener('drop', (e) => {
    if (!hasFiles(e.dataTransfer)) return;
    e.preventDefault();
    dragDepth = 0;
    el.classList.remove('is-drop-target');
    const files = Array.from(e.dataTransfer.files || []);
    if (files.length) uploadAll(files);
  });

  async function uploadAll(files) {
    if (!isLoggedIn()) {
      toastError('请先登录后再上传图片');
      return;
    }
    if (busy) {
      toastError('上一张图片还在上传，稍等一下');
      return;
    }

    const usable = files.filter((f) => {
      if (!ALLOWED_TYPES.test(f.type)) {
        toastError(`${f.name || '该文件'}不是支持的图片格式（JPG / PNG / WebP / GIF）`);
        return false;
      }
      if (f.size > maxBytes) {
        toastError(`${f.name || '该图片'}超过 ${Math.round(maxBytes / 1024 / 1024)}MB 上限`);
        return false;
      }
      return true;
    });
    if (!usable.length) return;

    busy = true;
    el.setAttribute('data-md-uploading', '1');
    try {
      for (const file of usable) {
        const marker = `![图片上传中…](#md-upload-${Date.now()}-${seq++})`;
        insertAt(el, marker);
        try {
          const res = await uploadFile(scene, file);
          replaceOnce(el, marker, `![${altOf(file)}](${inlineURL(res.url)})`);
        } catch (err) {
          // 占位必须删掉：留着它用户会以为图已经插进去了，提交后才发现是句报错文本。
          replaceOnce(el, marker, '');
          toastError(err instanceof ApiError ? err.message : '图片上传失败，请重试');
        }
      }
    } finally {
      busy = false;
      el.removeAttribute('data-md-uploading');
    }
  }

  return {
    destroy() {
      tools.remove();
    },
  };
}

/** 从粘贴板里挑出图片文件；非图片内容原样交给浏览器处理。 */
function imagesFrom(clipboard) {
  if (!clipboard) return [];
  const items = Array.from(clipboard.items || []);
  return items
    .filter((it) => it.kind === 'file')
    .map((it) => it.getAsFile())
    .filter((f) => f && ALLOWED_TYPES.test(f.type));
}

function hasFiles(dt) {
  if (!dt) return false;
  return Array.from(dt.types || []).includes('Files');
}

/**
 * 本站图片写成站内相对路径。
 *
 * 上传接口在有 public_base_url / app.base_url 配置时返回的是绝对地址，
 * 原样写进正文等于把域名钉死在文章里：换域名、http 换 https 之后
 * 历史文章的图全变裂图。同源的地址只取 path 部分。
 */
function inlineURL(url) {
  const raw = String(url || '').trim();
  if (!raw) return '';
  try {
    const u = new URL(raw, window.location.href);
    if (u.origin === window.location.origin) return u.pathname + u.search;
  } catch {
    // 不是合法绝对地址（相对路径也走这里）——原样返回即可。
  }
  return raw;
}

/**
 * alt 文案：截图粘贴出来的文件名通常是 image.png 这种无意义的名字，
 * 统一写成「图片」，读屏与加载失败时至少说的是人话。
 */
function altOf(file) {
  const base = String(file.name || '').replace(/\.[^.]+$/, '').trim();
  if (!base || /^image$/i.test(base) || /^screenshot$/i.test(base) || /^截图$/.test(base)) {
    return '图片';
  }
  return base.replace(/[[\]]/g, '');
}

/** 在光标处插入文本，并把光标移到插入内容之后。 */
function insertAt(el, text) {
  const start = typeof el.selectionStart === 'number' ? el.selectionStart : el.value.length;
  const end = typeof el.selectionEnd === 'number' ? el.selectionEnd : start;
  // 前后各补一个换行：图片单独成段，否则会和上一行的文字挤在同一段里。
  const snippet = `${start > 0 && el.value[start - 1] !== '\n' ? '\n' : ''}${text}\n`;
  el.value = el.value.slice(0, start) + snippet + el.value.slice(end);
  const caret = start + snippet.length;
  el.selectionStart = el.selectionEnd = caret;
  el.dispatchEvent(new Event('input', { bubbles: true }));
}

/** 把占位符替换成最终内容；光标在占位之后时要跟着平移。 */
function replaceOnce(el, needle, text) {
  const idx = el.value.indexOf(needle);
  if (idx < 0) return;
  const delta = text.length - needle.length;
  const caret = el.selectionStart;
  el.value = el.value.slice(0, idx) + text + el.value.slice(idx + needle.length);
  if (typeof caret === 'number' && caret > idx) {
    const next = Math.max(idx, caret + delta);
    el.selectionStart = el.selectionEnd = next;
  }
  el.dispatchEvent(new Event('input', { bubbles: true }));
}
