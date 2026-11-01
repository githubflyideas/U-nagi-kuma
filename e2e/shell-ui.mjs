// Real Chromium shell contracts. No server or external API is contacted.
// Run: node e2e/shell-ui.mjs
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const browser = await chromium.launch();
const errors = [];
async function fixture(width = 390, phase = 'in') {
  const ctx = await browser.newContext({ viewport: { width, height: 844 }, reducedMotion: 'reduce' });
  const posts = [];
  await ctx.route('**/*', async route => {
    const url = new URL(route.request().url());
    if (url.hostname !== 'unagi.test') return route.abort();
    const p = url.pathname;
    if (route.request().method() === 'POST') { posts.push(route); return; }
    if (p === '/api/v1/session') return route.fulfill(phase === 'in' ? { json: { user: 'test', csrf: 'fixture' } } : { status: 401, json: { code: phase === 'setup' ? 'setup_required' : 'unauthorized' } });
    if (p === '/api/v1/setup/status') return route.fulfill({ json: { local: true } });
    if (p === '/api/v1/languages') return route.fulfill({ json: [{ code: 'en', name: 'English' }] });
    if (p.startsWith('/api/v1/i18n/')) return route.fulfill({ json: JSON.parse(await readFile(path.join(root, 'internal/i18n/locales/en.json'))) });
    if (p === '/api/v1/monitors') return route.fulfill({ json: { items: [] } });
    if (p === '/api/v1/overview') return route.fulfill({ json: {
      counts: { total: 0 }, paths: [], layout: 'single', channels: [], expiring: [],
      health: { uplink: { state: 'cannot_judge' }, gateway: {}, heartbeat: { configured: false } }
    } });
    if (p.startsWith('/api/')) return route.fulfill({ status: 503, json: { code: 'unavailable' } });
    const name = p === '/' ? 'index.html' : decodeURIComponent(p.slice(1));
    if (name.includes('..')) return route.fulfill({ status: 403 });
    try {
      let body = await readFile(path.join(root, 'web/ui', name));
      // Parent integrates this stylesheet; keep the contract runnable beforehand.
      if (name === 'index.html' && !body.toString().includes('shell-nav.css')) body = Buffer.from(body.toString().replace('</head>', '<link rel="stylesheet" href="/shell-nav.css"></head>'));
      return route.fulfill({ contentType: { '.js': 'text/javascript', '.css': 'text/css', '.html': 'text/html', '.png': 'image/png' }[path.extname(name)] || 'text/plain', body });
    } catch { return route.fulfill({ status: 404 }); }
  });
  const page = await ctx.newPage();
  page.on('pageerror', e => errors.push(e.stack || e.message));
  await page.goto('http://unagi.test/#/notfound');
  await page.locator(phase === 'in' ? '.top' : 'form').waitFor();
  return { ctx, page, posts };
}
try {
  for (const width of [390, 320]) {
    const { ctx, page } = await fixture(width);
    const toggle = page.locator('.top > button').first();
    await toggle.click();
    const close = page.locator('.side').getByRole('button', { name: 'Close', exact: true });
    assert.equal(await close.count(), 1, 'mobile navigation has an accessible close button');
    await page.waitForFunction(() => document.querySelector('.side').contains(document.activeElement));
    assert.equal(await page.locator('.main').evaluate(e => e.inert), true);
    assert.equal(await toggle.getAttribute('aria-expanded'), 'true');
    const side = page.locator('.side');
    const first = side.locator('a,button,select').first();
    const last = side.locator('a,button,select').last();
    await first.focus();
    await page.keyboard.press('Shift+Tab');
    assert.equal(await last.evaluate(e => e === document.activeElement), true, 'backward focus wraps');
    await page.keyboard.press('Tab');
    assert.equal(await first.evaluate(e => e === document.activeElement), true, 'forward focus wraps');
    await page.keyboard.press('Escape');
    await page.waitForFunction(() => !document.querySelector('.side').classList.contains('open'));
    assert.equal(await toggle.evaluate(e => e === document.activeElement), true, 'Escape restores opener');
    assert.equal(await side.evaluate(e => e.inert), true, 'closed sidebar is inert');
    await first.evaluate(e => e.focus());
    assert.equal(await first.evaluate(e => e === document.activeElement), false, 'hidden link cannot receive focus');
    await toggle.click();
    await page.locator('.side-backdrop').click({ position: { x: width - 10, y: 400 } });
    await page.waitForFunction(() => !document.querySelector('.side').classList.contains('open'));
    await toggle.click();
    await close.click();
    assert.equal(await toggle.evaluate(e => e === document.activeElement), true, 'close restores opener');
    await toggle.click();
    await side.locator('a[href="#/events"]').click();
    await page.waitForFunction(() => !document.querySelector('.side').classList.contains('open'));
    assert.equal(await page.locator('.main').evaluate(e => e.inert), false);
    await toggle.click();
    await side.locator('a[href="#/events"]').click();
    await page.waitForFunction(() => !document.querySelector('.side').classList.contains('open'), null, { timeout: 2000 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
    const search = page.locator('button.search');
    assert.equal(await search.isVisible(), true, 'mobile command palette trigger is visible');
    const box = await search.boundingBox();
    assert.ok(box.width >= 44 && box.height >= 44 && box.x + box.width <= width, 'search is a reachable touch target');
    await search.click();
    await page.locator('.pal input').waitFor();
    assert.equal(await page.locator('.pal input').evaluate(e => e === document.activeElement), true);
    await page.keyboard.press('Escape');
    await page.locator('.pal').waitFor({ state: 'detached' });
    await toggle.click();
    await page.keyboard.press('Control+k');
    await page.locator('.pal input').waitFor();
    await page.waitForFunction(() => !document.querySelector('.side').classList.contains('open'));
    assert.equal(await page.locator('.pal input').evaluate(e => e === document.activeElement), true, 'palette shortcut hands off focus from navigation');
    await page.keyboard.press('Escape');
    await page.locator('.pal').waitFor({ state: 'detached' });
    assert.equal(await side.evaluate(e => getComputedStyle(e).transitionDuration), '0s', 'reduced motion has no drawer transition');
    console.log('ok mobile navigation and search', width, 'close, backdrop, Escape, focus, inert and route dismissal');
    await ctx.close();
  }
  {
    const { ctx, page } = await fixture(1280);
    const toggle = page.locator('.top > button').first();
    await toggle.click();
    await page.waitForFunction(() => document.querySelector('.shell').classList.contains('mini'));
    assert.equal(await page.evaluate(() => localStorage.getItem('unagikuma.side')), 'mini');
    await page.reload();
    await page.locator('.shell.mini').waitFor();
    assert.equal(await page.locator('.side').evaluate(e => e.inert), false);
    await page.setViewportSize({ width: 860, height: 844 });
    await page.waitForFunction(() => document.querySelector('.side').inert);
    await toggle.click();
    await page.locator('.side.open').waitFor();
    await page.setViewportSize({ width: 1280, height: 844 });
    await page.waitForFunction(() => !document.querySelector('.side').classList.contains('open'));
    assert.equal(await page.locator('.main').evaluate(e => e.inert), false);
    assert.equal(await page.locator('.side').evaluate(e => e.inert), false);
    assert.equal(await page.evaluate(() => document.body.style.overflow), '');
    assert.equal(await page.evaluate(() => localStorage.getItem('unagikuma.side')), 'mini');
    await toggle.click();
    await page.waitForFunction(() => !document.querySelector('.shell').classList.contains('mini'));
    assert.equal(await page.evaluate(() => localStorage.getItem('unagikuma.side')), 'full');
    console.log('ok desktop collapse persistence and breakpoint cleanup');
    await ctx.close();
  }
  for (const phase of ['setup', 'login']) {
    const { ctx, page, posts } = await fixture(390, phase);
    await page.locator('input[autocomplete="username"]').fill('admin');
    for (const input of await page.locator('input[type="password"]').all()) await input.fill('password-fixture');
    await page.locator('form').evaluate(form => {
      form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
      form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    });
    await page.waitForTimeout(150);
    assert.equal(posts.length, 1, phase + ' synchronously suppresses duplicate submits');
    await posts[0].fulfill({ status: 503, json: { message: 'Try again fixture' } });
    await page.locator('.banner.error').waitFor();
    await page.locator('form button').click();
    await page.waitForTimeout(150);
    assert.equal(posts.length, 2, phase + ' unlocks after an error');
    await posts[1].fulfill({ json: { user: 'test', csrf: 'fixture' } });
    await page.locator('.shell').waitFor();
    // Authentication redirects to overview; await its render, not just the shell.
    // Otherwise context teardown can hide errors from incomplete API fixtures.
    await page.locator('.story').waitFor({ timeout: 5000 }).catch(error => {
      assert.deepEqual(errors, []);
      throw error;
    });
    console.log('ok', phase, 'single-flight submission and error retry');
    await ctx.close();
  }
  assert.deepEqual(errors, []);
} finally { await browser.close(); }
