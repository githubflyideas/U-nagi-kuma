// Real browser theme regression, with an explicitly mocked authenticated API.
// Run: node e2e/themes.mjs (requires playwright + Chromium).
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const browser = await chromium.launch();
const failures = [];
try {
  const ctx = await browser.newContext({ colorScheme: 'dark' });
  await ctx.route('http://unagi.test/**', async route => {
    const url = new URL(route.request().url());
    if (url.pathname === '/api/v1/session') return route.fulfill({ json: { user: 'test', csrf: 'fixture' } });
    if (url.pathname === '/api/v1/languages') return route.fulfill({ json: [{code:'en',name:'English'}, {code:'zh',name:'中文'}] });
    if (url.pathname.startsWith('/api/v1/i18n/')) {
      const en = JSON.parse(await readFile(path.join(root, 'internal/i18n/locales/en.json')));
      const cat = JSON.parse(await readFile(path.join(root, 'internal/i18n/locales', url.pathname.endsWith('/zh') ? 'zh.json' : 'en.json')));
      return route.fulfill({ json: { ...en, ...cat } });
    }
    if (url.pathname.startsWith('/api/')) return route.fulfill({ status: 503, json: { code: 'unavailable' } });
    const name = url.pathname === '/' ? 'index.html' : decodeURIComponent(url.pathname.slice(1));
    if (name.includes('..')) return route.fulfill({ status: 403 });
    const types = { '.js': 'text/javascript', '.css': 'text/css', '.html': 'text/html', '.png': 'image/png' };
    try { return route.fulfill({ contentType: types[path.extname(name)] || 'text/plain', body: await readFile(path.join(root, 'web/ui', name)) }); }
    catch { return route.fulfill({ status: 404 }); }
  });
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.goto('http://unagi.test/#/notfound');
  const select = page.locator('select[aria-label="Theme"]');
  await select.waitFor();
  for (const [theme, scheme, bg] of [['midnight','dark','rgb(12, 20, 38)'], ['paper','light','rgb(246, 243, 235)']]) {
    try {
      assert.equal(await select.locator(`option[value="${theme}"]`).count(), 1, `${theme} is available in the theme selector`);
      await select.selectOption(theme);
      assert.equal(await page.evaluate(() => document.documentElement.dataset.theme), theme);
      assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).colorScheme), scheme);
      assert.equal(await page.evaluate(() => getComputedStyle(document.body).backgroundColor), bg);
      const contrast = await page.evaluate(() => {
        const s = getComputedStyle(document.documentElement);
        const luminance = key => {
          const hex = s.getPropertyValue('--' + key).trim();
          const rgb = hex.slice(1).match(/../g).map(x => parseInt(x, 16) / 255).map(v => v <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4);
          return rgb[0] * .2126 + rgb[1] * .7152 + rgb[2] * .0722;
        };
        return [['text','panel'], ['muted','panel'], ['faint','panel'], ['link','panel'], ['accent-text','accent'],
          ...['ok','critical','degraded','unknown','pending','maint'].map(x => [x, x + '-soft'])].map(([a,b]) => {
            const x = luminance(a), y = luminance(b);
            return { pair: a + '/' + b, ratio: (Math.max(x,y) + .05) / (Math.min(x,y) + .05) };
          });
      });
      for (const {pair, ratio} of contrast) assert.ok(ratio >= 4.5, `${theme} ${pair} contrast ${ratio.toFixed(2)} >= 4.5`);
      await page.reload();
      await select.waitFor();
      assert.equal(await select.inputValue(), theme, 'choice persists after reload');
      assert.equal(await page.evaluate(() => getComputedStyle(document.body).backgroundColor), bg);
      console.log('ok', theme, 'selection, palette, native scheme and persistence');
    } catch (e) { failures.push(e.message); console.error('FAIL', e.message); }
  }
  await select.selectOption('auto');
  assert.equal(await page.evaluate(() => document.documentElement.dataset.theme), undefined);
  await page.emulateMedia({ colorScheme: 'light' });
  assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).colorScheme), 'light');
  await page.emulateMedia({ colorScheme: 'dark' });
  assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).colorScheme), 'dark');
  console.log('ok auto follows operating-system changes');
  for (const theme of ['light','dark']) {
    await select.selectOption(theme);
    assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).colorScheme), theme);
  }
  assert.equal(await page.locator('.side').evaluate(el => getComputedStyle(el).backdropFilter), 'none', 'navigation avoids continuous backdrop compositing');
  assert.equal(await page.locator('.top').evaluate(el => getComputedStyle(el).backdropFilter), 'none', 'toolbar avoids continuous backdrop compositing');
  await page.emulateMedia({ reducedMotion: 'reduce' });
  assert.equal(await page.locator('.shell').evaluate(el => getComputedStyle(el).transitionDuration), '0s', 'reduced motion disables layout transition');
  assert.equal(await page.evaluate(() => getComputedStyle(document.body).backgroundImage), 'none', 'console uses lightweight solid surfaces');
  await page.setViewportSize({ width: 390, height: 844 });
  console.log('mobile-target', await page.locator('.top > button').first().evaluate(el => ({width: el.getBoundingClientRect().width, height: el.getBoundingClientRect().height, minWidth: getComputedStyle(el).minWidth, minHeight:getComputedStyle(el).minHeight, viewport:innerWidth, sheet:[...document.styleSheets].map(s=>s.href)})));
  await page.waitForFunction(() => { const r = document.querySelector('.top > button').getBoundingClientRect(); return r.width >= 44 && r.height >= 44; });
  const target = await page.locator('.top > button').first().boundingBox();
  assert.ok(target.width >= 44 && target.height >= 44, 'mobile menu touch target is at least 44px');
  assert.deepEqual(errors, [], 'no script errors');
  console.log('ok original themes and no script errors');
} finally { await browser.close(); }
assert.deepEqual(failures, []);
