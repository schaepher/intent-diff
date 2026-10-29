#!/usr/bin/env node
// 浏览器自检：验证渲染页的切换/深链/sticky/移动端/高亮/离线性。
// 依赖 playwright（npm i playwright；浏览器需已安装）。
// 用法: node scripts/ui-check.mjs [review.html]

import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');
const target = path.resolve(root, process.argv[2] || '.tmp/demo/out/review.html');

if (!fs.existsSync(target)) {
  console.error(`找不到 ${target}；先运行 examples/demo/run.sh`);
  process.exit(2);
}

let chromium;
try {
  ({ chromium } = await import('playwright'));
} catch {
  console.error('未找到 playwright：npm i playwright（或用 npx 安装后重试）');
  process.exit(2);
}

const failures = [];
function check(name, cond, detail) {
  if (!cond) failures.push(`${name}${detail ? ': ' + detail : ''}`);
}

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
const net = [];
const pageErrors = [];
page.on('pageerror', (e) => pageErrors.push(e.message));
page.on('request', (r) => {
  if (!r.url().startsWith('file://') && !r.url().startsWith('data:')) net.push(r.url());
});

const url = pathToFileURL(target).href;
await page.goto(url);
await page.waitForTimeout(300);

const first = await page.evaluate(() => ({
  chains: Array.from(document.querySelectorAll('[data-chain-section]')).map((s) => s.getAttribute('data-chain-section')),
  activeChain: document.querySelector('[data-chain-section]:not([hidden])')?.getAttribute('data-chain-section'),
  activeIntent: document.querySelector('.timeline-node.is-active')?.getAttribute('data-intent'),
  sticky: getComputedStyle(document.querySelector('.intent-timeline')).position,
  panels: document.querySelectorAll('[data-panel]').length,
  rows: document.querySelectorAll('table.diff tbody tr').length,
  tokens: document.querySelectorAll('td.code span[class^="tok-"]').length,
}));
check('至少一条链', first.chains.length >= 1, JSON.stringify(first));
check('默认激活第一条链', !!first.activeChain);
check('默认激活第一个意图', !!first.activeIntent);
check('状态机 sticky', first.sticky === 'sticky', first.sticky);
check('两列 diff 已渲染', first.rows > 0, String(first.rows));
check('语法高亮已生效', first.tokens > 0, String(first.tokens));
check('无外部网络请求', net.length === 0, net.join(', '));

const nodes = await page.$$eval('.timeline-node', (ns) => ns.map((n) => n.getAttribute('data-intent')));
if (nodes.length > 1) {
  await page.click(`.timeline-node[data-intent="${nodes[1]}"] .node-btn`);
  await page.waitForTimeout(200);
  const after = await page.evaluate(() => ({
    active: document.querySelector('.timeline-node.is-active')?.getAttribute('data-intent'),
    hash: location.hash,
    passed: document.querySelectorAll('.timeline-node.is-passed').length,
    current: document.querySelector('.timeline-node.is-active .node-btn')?.getAttribute('aria-current'),
  }));
  check('点击节点切换意图', after.active === nodes[1], after.active);
  check('hash 写入', after.hash === `#${encodeURIComponent(first.activeChain)}/${encodeURIComponent(nodes[1])}`, after.hash);
  check('已过节点高亮', after.passed >= 1, String(after.passed));
  check('aria-current=step', after.current === 'step', String(after.current));

  await page.keyboard.press('ArrowRight');
  await page.waitForTimeout(200);
  if (nodes.length > 2) {
    const key = await page.evaluate(() => document.querySelector('.timeline-node.is-active')?.getAttribute('data-intent'));
    check('方向键切换意图', key === nodes[2], String(key));
  }
}

// 深链
await page.goto(`${url}#${encodeURIComponent(first.activeChain)}/${encodeURIComponent(nodes[0])}/1`);
await page.waitForTimeout(300);
const deep = await page.evaluate(() => ({
  active: document.querySelector('.timeline-node.is-active')?.getAttribute('data-intent'),
  visibleChanges: Array.from(document.querySelectorAll('[data-panel]:not([hidden]) [data-change-index]')).length,
}));
check('深链定位意图', deep.active === nodes[0], String(deep.active));
check('深链下 change 可见', deep.visibleChanges >= 1, String(deep.visibleChanges));

// sticky 实际滚动
await page.evaluate(() => window.scrollTo(0, 300));
await page.waitForTimeout(150);
const stickyTop = await page.evaluate(() => Math.round(document.querySelector('.intent-timeline').getBoundingClientRect().top));
check('滚动后状态机仍贴顶', stickyTop === 0, String(stickyTop));
await page.evaluate(() => window.scrollTo(0, 0));

// 移动端
await page.setViewportSize({ width: 420, height: 800 });
await page.waitForTimeout(200);
const mobile = await page.evaluate(() => ({
  toggle: getComputedStyle(document.querySelector('.nav-toggle')).display !== 'none',
  sidebarLeft: Math.round(document.querySelector('.sidebar').getBoundingClientRect().left),
}));
check('移动端汉堡按钮出现', mobile.toggle);
check('移动端抽屉默认收起', mobile.sidebarLeft < 0, String(mobile.sidebarLeft));
await page.click('.nav-toggle');
await page.waitForTimeout(300);
const opened = await page.evaluate(() => ({
  open: document.body.classList.contains('sidebar-open'),
  left: Math.round(document.querySelector('.sidebar').getBoundingClientRect().left),
}));
check('汉堡展开抽屉', opened.open && opened.left === 0, JSON.stringify(opened));

// 折叠 / 大评审：用内置二进制生成合成页面（无 repo 依赖）。
const bin = path.join(root, 'bin/intent-review');
if (fs.existsSync(bin)) {
  const dir = path.join(root, '.tmp', 'ui-check');
  fs.mkdirSync(dir, { recursive: true });
  const longLines = [];
  for (let i = 1; i <= 20; i++) longLines.push({ type: 'context', content: `line ${i}` });
  longLines[10] = { type: 'del', content: 'old line' };
  longLines[11] = { type: 'add', content: 'new line' };
  const synth = {
    schema_version: '1.0',
    meta: { title: '折叠自检', base_commit: 'deadbeef', head_ref: 'main' },
    chains: [{
      id: 'c1', title: '链一',
      intents: [{
        id: 'i1', title: '长上下文', summary: '折叠',
        changes: [{
          file: 'a.go', location: '长块', business_explanation: '折叠自检',
          old_start: 1, new_start: 1, lines: longLines,
        }],
      }],
    }],
  };
  const synthJson = path.join(dir, 'fold.json');
  const foldHtml = path.join(dir, 'fold.html');
  const largeHtml = path.join(dir, 'large.html');
  fs.writeFileSync(synthJson, JSON.stringify(synth));
  execFileSync(bin, ['render', synthJson, '-o', foldHtml]);
  execFileSync(bin, ['render', synthJson, '-o', largeHtml, '--max-lines', '5']);

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(pathToFileURL(foldHtml).href);
  await page.waitForTimeout(200);
  const fold = await page.evaluate(() => ({
    buttons: document.querySelectorAll('.fold-toggle').length,
    hidden: Array.from(document.querySelectorAll('table.diff tbody tr')).filter((r) => r.hidden).length,
  }));
  check('长 context 生成折叠按钮', fold.buttons >= 2, JSON.stringify(fold));
  check('折叠隐藏中间 context', fold.hidden > 0, String(fold.hidden));
  await page.click('.fold-toggle');
  await page.waitForTimeout(100);
  const unfolded = await page.evaluate(() => ({
    hidden: Array.from(document.querySelectorAll('table.diff tbody tr')).filter((r) => r.hidden).length,
    label: document.querySelector('.fold-toggle')?.textContent || '',
  }));
  check('点击展开折叠行', unfolded.hidden < fold.hidden, JSON.stringify(unfolded));
  check('展开后按钮变为收起', unfolded.label.startsWith('收起'), unfolded.label);

  await page.goto(pathToFileURL(largeHtml).href);
  await page.waitForTimeout(200);
  const large = await page.evaluate(() => ({
    badge: !!document.querySelector('.large-badge'),
    details: document.querySelectorAll('details.panel-details').length,
    open: document.querySelector('details.panel-details')?.open,
  }));
  check('大评审显示徽标', large.badge, JSON.stringify(large));
  check('大评审内容折叠 details', large.details >= 1, String(large.details));
  check('当前意图 details 展开', large.open === true, String(large.open));
}

check('无页面 JS 错误', pageErrors.length === 0, pageErrors.join(' | '));

await browser.close();

if (failures.length) {
  console.error('ui-check 失败:');
  for (const f of failures) console.error(' - ' + f);
  process.exit(1);
}
console.log(`ui-check: ok (${target})`);
