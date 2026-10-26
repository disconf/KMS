// 静态检查：每个 import 的符号在目标模块里是否真的导出。
// 浏览器里这类错误只会表现为白屏 + 控制台一行红字，用 curl 完全看不出来。
//
//   node scripts/depcheck.js                  # 默认检查 web/static/assets/js
//   node scripts/depcheck.js <目录>
const fs = require('fs');
const path = require('path');

const dir = process.argv[2] || path.join(__dirname, '..', 'web', 'static', 'assets', 'js');
const files = fs.readdirSync(dir).filter((f) => f.endsWith('.js'));

const exportsOf = {};
for (const f of files) {
  const src = fs.readFileSync(path.join(dir, f), 'utf8');
  const names = new Set();
  for (const m of src.matchAll(/^export\s+(?:async\s+)?(?:function|const|let|var|class)\s+([A-Za-z_$][\w$]*)/gm)) names.add(m[1]);
  for (const m of src.matchAll(/^export\s*\{([^}]+)\}/gm)) {
    for (const part of m[1].split(',')) {
      const t = part.trim(); if (!t) continue;
      const as = t.split(/\s+as\s+/);
      names.add((as[1] || as[0]).trim());
    }
  }
  exportsOf[f] = names;
}

let problems = 0;
for (const f of files) {
  const src = fs.readFileSync(path.join(dir, f), 'utf8');
  for (const m of src.matchAll(/import\s*\{([^}]+)\}\s*from\s*['"]\.\/([\w.-]+)['"]/g)) {
    let target = m[2];
    if (!target.endsWith('.js')) target += '.js';
    const names = exportsOf[target];
    if (!names) { console.log(`模块不存在  ${f} -> ${target}`); problems++; continue; }
    for (const part of m[1].split(',')) {
      const t = part.trim(); if (!t) continue;
      const imported = t.split(/\s+as\s+/)[0].trim();
      if (!names.has(imported)) {
        console.log(`缺少导出    ${f} 引入 ${imported}，但 ${target} 没有导出它`);
        problems++;
      }
    }
  }
}
console.log(problems === 0 ? '所有 import 均能解析' : `发现 ${problems} 个问题`);
process.exit(problems === 0 ? 0 : 1);
