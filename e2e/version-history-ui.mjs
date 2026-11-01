// Local Chromium contracts: application version and monitor history ranges.
// Run: node e2e/version-history-ui.mjs [version|history]
import { chromium } from 'playwright';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const browser = await chromium.launch();
const errors = [];
async function fixture(width, version, hash = '/notfound') {
  const ctx = await browser.newContext({ viewport: { width, height: 844 }, reducedMotion: 'reduce' });
  const requests = [];
  await ctx.route('**/*', async route => {
    const url = new URL(route.request().url());
    if (url.hostname !== 'unagi.test') return route.abort();
    const p = url.pathname;
    if (p.startsWith('/api/')) requests.push(p + url.search);
    if (p === '/api/v1/session') return route.fulfill({ json: { user: 'test', csrf: 'fixture' } });
    if (p === '/api/v1/languages') return route.fulfill({ json: [{ code: 'en', name: 'English' }] });
    if (p.startsWith('/api/v1/i18n/')) return route.fulfill({ json: JSON.parse(await readFile(path.join(root, 'internal/i18n/locales/en.json'))) });
    if (p === '/api/v1/monitors') return route.fulfill({ json: { items: [] } });
    if (p === '/api/v1/overview') return route.fulfill({ json: { version, counts: { total: 0 }, paths: [], layout: 'single' } });
    if (p === '/api/v1/monitors/web') return route.fulfill({ json: { row: { name: 'web', title: 'Web', status: 'ok', written_type: 'HTTP', type: 'HTTP', target: 'localhost', tags: [], flags: [], interval_s: 60, max_attempts: 3 }, uptime: {}, evidence: [] } });
    if (p === '/api/v1/monitors/web/latency') return route.fulfill({ json: { points: [] } });
    if (p === '/api/v1/monitors/heartbeats') return route.fulfill({ json: { items: { web: [] } } });
    if (p === '/api/v1/monitors/web/timeline') return route.fulfill({ json: { items: [] } });
    if (p.startsWith('/api/')) return route.fulfill({ status: 503, json: { code: 'unavailable' } });
    const name = p === '/' ? 'index.html' : decodeURIComponent(p.slice(1));
    if (name.includes('..')) return route.fulfill({ status: 403 });
    try {
      const body = await readFile(path.join(root, 'web/ui', name));
      return route.fulfill({ contentType: { '.js': 'text/javascript', '.css': 'text/css', '.html': 'text/html', '.png': 'image/png' }[path.extname(name)] || 'text/plain', body });
    } catch { return route.fulfill({ status: 404 }); }
  });
  const page = await ctx.newPage();
  page.on('pageerror', e => errors.push(e.message));
  await page.goto('http://unagi.test/#' + hash);
  await page.locator('.top').waitFor();
  return { ctx, page, requests };
}
try {
  if (process.argv[2] !== 'history') {
    for (const width of [1280, 390, 320]) {
      const version = 'v9.8.7-local+abc123';
      const { ctx, page, requests } = await fixture(width, version);
      if (width <= 860) await page.locator('.top > button').first().click();
      const label = page.locator('.brand-version');
      await label.waitFor({ timeout: 2500 });
      assert.equal(await label.textContent(), version, 'the API version is rendered without a hardcoded release');
      const name = await page.locator('.brand-name').boundingBox();
      const box = await label.boundingBox();
      assert.ok(box.y >= name.y + name.height - 1, 'version is directly below the logo text');
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'no viewport overflow');
      assert.ok(!requests.some(p => p.includes('/settings/system')), 'reuse overview rather than another request');
      if (width > 860) {
        await page.locator('.top > button').first().click();
        assert.equal(await label.isVisible(), false, 'compact sidebar hides text');
        assert.ok((await page.locator('.brand').getAttribute('aria-label')).includes(version), 'compact brand retains an accessible version');
        await page.setViewportSize({ width: 390, height: 844 });
        await page.locator('.top > button').first().click();
        assert.equal(await label.isVisible(), true, 'mobile drawer restores version after compact desktop');
      }
      console.log('ok dynamic sidebar version', width);
      await ctx.close();
    }
    for (const version of [undefined, null, '']) {
      const { ctx, page } = await fixture(1280, version);
      await page.waitForTimeout(100);
      assert.equal(await page.locator('.brand-version').count(), 0, 'missing version has no empty label');
      assert.doesNotMatch(await page.locator('.brand').innerText(), /undefined|null/);
      console.log('ok absent sidebar version', String(version));
      await ctx.close();
    }
  }
  if (process.argv[2] !== 'version') {
    for (const width of [1280, 390, 320]) {
      const { ctx, page, requests } = await fixture(width, 'dev', '/monitors/web');
      await page.locator('.seg').waitFor();
      for (const range of ['5m', '15m', '30m', '1h', '6h', '12h', '24h', '7d', '30d']) {
        const start = requests.length;
        await Promise.all([
          page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/monitors/web/latency' && new URL(r.url()).searchParams.get('range') === range),
          page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1/monitors/heartbeats' && new URL(r.url()).searchParams.get('range') === range),
          page.locator('.seg').getByRole('button', { name: range, exact: true }).click(),
        ]);
        await page.waitForTimeout(50);
        const batch = requests.slice(start).map(p => new URL(p, 'http://unagi.test'));
        assert.ok(batch.some(u => u.pathname === '/api/v1/monitors/web/latency' && u.searchParams.get('range') === range), 'latency uses ' + range);
        assert.ok(batch.some(u => u.pathname === '/api/v1/monitors/heartbeats' && u.searchParams.get('range') === range), 'heartbeats use ' + range);
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'history controls do not overflow at ' + width);
      }
      console.log('ok heartbeat and latency request ranges and overflow', width);
      await ctx.close();
    }
  }
  assert.deepEqual(errors, [], 'no browser runtime errors');
} finally { await browser.close(); }
