// Isolated real-browser tests: all API traffic is intercepted; no notifications leave this process.
// Run: node --test e2e/channels-ui.mjs
import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { chromium } from 'playwright';
const root = new URL('../web/ui/', import.meta.url);
let server, browser, base;
const kinds = [{ kind: 'feishu', label: 'Feishu', fields: [{ key: 'url', type: 'text' }, { key: 'secret', type: 'secret' }] }];
const channels = [{ name: 'primary', kind: 'feishu', opts: { url: 'https://example.invalid', secret: '***' }, used_by: ['production'], issues: [], etag: 'v1' }, { name: 'backup', kind: 'feishu', opts: {}, used_by: [], issues: [{ code: 'unknown', address: 'ops' }] }];
before(async () => {
  server = createServer(async (req, res) => {
    try {
      if (req.url === '/') { res.setHeader('Content-Type', 'text/html'); res.end(`<div id="app"></div><script type="module">import {html,render} from '/js/lib.js'; import {Alerts} from '/js/views/alerts.js'; render(html\`<\${Alerts} tab="channels" />\`,document.querySelector('#app'));</script>`); return; }
      res.setHeader('Content-Type', req.url.endsWith('.css') ? 'text/css' : 'text/javascript');
      res.end(await readFile(new URL('.' + req.url, root)));
    } catch { res.statusCode = 404; res.end(); }
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  base = `http://127.0.0.1:${server.address().port}`;
  browser = await chromium.launch({ headless: true });
});
after(async () => { await browser?.close(); await new Promise(resolve => server?.close(resolve)); });
async function setup(handler = () => undefined) {
  const page = await browser.newPage(); page.setDefaultTimeout(2000);
  const errors = []; page.on('pageerror', e => errors.push(e.message));
  await page.route('**/api/**', async route => {
    const path = new URL(route.request().url()).pathname.replace('/api/v1', '');
    const custom = await handler(path, route.request());
    const result = custom || { body: path === '/channels' ? channels : path === '/channel-kinds' ? kinds : {} };
    await route.fulfill({ status: result.status || 200, contentType: 'application/json', body: JSON.stringify(result.body) });
  });
  await page.goto(base);
  return { page, errors };
}
for (const name of ['constructor', 'toString']) {
  test(`prototype-like channel name ${name} renders and can be tested`, async () => {
    const { page, errors } = await setup(path => {
      if (path === '/channels') return { body: [{ ...channels[0], name }] };
      if (path === '/channels/test') return { body: { ok: true } };
    });
    try {
      await page.getByRole('button', { name: 'ui.ch.test', exact: true }).click();
      await page.getByRole('status').getByText('ui.ch.test_ok', { exact: true }).waitFor();
      assert.deepEqual(errors, []);
    } finally { await page.close(); }
  });
}
test('saved-channel tests show local pending and outcome and prevent duplicates', async () => {
  let release, calls = 0;
  const gate = new Promise(resolve => { release = resolve; });
  const { page, errors } = await setup(async path => {
    if (path === '/channels/test') { calls++; await gate; return { body: { ok: false, status: 429, error: 'Rate limited' } }; }
  });
  try {
    const row = page.locator('tr').filter({ has: page.getByText('primary', { exact: true }) });
    const button = row.getByRole('button', { name: 'ui.ch.test', exact: true });
    await button.evaluate(el => { el.click(); el.click(); });
    await row.getByRole('status').getByText('ui.ch.testing', { exact: true }).waitFor();
    assert.equal(await button.isDisabled(), true);
    release();
    await row.getByRole('status').getByText(/Rate limited/).waitFor();
    assert.equal(calls, 1); assert.equal(await button.isDisabled(), false); assert.deepEqual(errors, []);
  } finally { release(); await page.close(); }
});
async function savePrimary(page) {
  const row = page.locator('tr').filter({ has: page.getByText('primary', { exact: true }) });
  await row.getByRole('button', { name: 'ui.edit', exact: true }).click();
  await page.getByRole('dialog').locator('input').nth(1).fill('https://new.invalid');
  await page.getByRole('button', { name: 'ui.save', exact: true }).click();
  await page.getByRole('dialog').waitFor({ state: 'detached' });
  // Wait for the saved configuration to be reloaded and rendered.
  await row.getByRole('button', { name: 'ui.edit', exact: true }).click();
  assert.equal(await page.getByRole('dialog').locator('input').nth(1).inputValue(), 'https://new.invalid');
  await page.getByRole('button', { name: 'ui.cancel', exact: true }).click();
  return row;
}
function savedConfigHandler(testHandler) {
  let saved = channels;
  return async (path, request) => {
    if (path === '/channels' && request.method() === 'POST') {
      const body = request.postDataJSON();
      saved = saved.map(c => c.name === body.name ? { ...c, ...body, etag: 'v2' } : c);
      return { body: {} };
    }
    if (path === '/channels') return { body: saved };
    if (path === '/channels/test') return testHandler(request);
  };
}
test('saving a changed channel clears its previous success but retains other channel results', async () => {
  const { page, errors } = await setup(savedConfigHandler(() => ({ body: { ok: true } })));
  try {
    for (const name of ['primary', 'backup']) {
      const row = page.locator('tr').filter({ has: page.getByText(name, { exact: true }) });
      await row.getByRole('button', { name: 'ui.ch.test', exact: true }).click();
      await row.getByRole('status').getByText('ui.ch.test_ok', { exact: true }).waitFor();
    }
    const row = await savePrimary(page);
    assert.equal(await row.getByRole('status').count(), 0);
    await page.locator('tr').filter({ has: page.getByText('backup', { exact: true }) }).getByRole('status').getByText('ui.ch.test_ok', { exact: true }).waitFor();
    assert.deepEqual(errors, []);
  } finally { await page.close(); }
});
for (const outcome of ['success', 'error']) {
  test(`saving discards an old in-flight ${outcome} without clearing a newer pending guard`, async () => {
    const releases = [];
    let calls = 0;
    const { page, errors } = await setup(savedConfigHandler(async () => {
      const index = calls++;
      await new Promise(resolve => { releases[index] = resolve; });
      return index === 0 && outcome === 'error'
        ? { status: 503, body: { message: 'Old configuration failed' } }
        : { body: { ok: true } };
    }));
    try {
      const row = page.locator('tr').filter({ has: page.getByText('primary', { exact: true }) });
      const button = row.getByRole('button', { name: 'ui.ch.test', exact: true });
      await button.click();
      await row.getByRole('status').getByText('ui.ch.testing', { exact: true }).waitFor();
      await savePrimary(page);
      assert.equal(await row.getByRole('status').count(), 0);
      await button.click();
      await row.getByRole('status').getByText('ui.ch.testing', { exact: true }).waitFor();
      assert.equal(calls, 2);
      const oldResponse = page.waitForResponse(r => r.url().endsWith('/channels/test'));
      releases[0]();
      await (await oldResponse).finished();
      await page.waitForTimeout(100);
      await row.getByRole('status').getByText('ui.ch.testing', { exact: true }).waitFor();
      assert.equal(await button.isDisabled(), true);
      // Bypass disabled UI to verify the synchronous duplicate-request guard itself.
      await button.evaluate(el => { el.disabled = false; el.click(); el.click(); });
      await page.waitForTimeout(100);
      assert.equal(calls, 2);
      releases[1]();
      await row.getByRole('status').getByText('ui.ch.test_ok', { exact: true }).waitFor();
      assert.equal(await button.isDisabled(), false);
      assert.deepEqual(errors, []);
    } finally { releases.forEach(release => release()); await page.close(); }
  });
}
test('form labels saved-secret tests honestly and discards results after edits including in-flight responses', async () => {
  let release; const bodies = [];
  const { page, errors } = await setup(async (path, request) => {
    if (path === '/channels/test') { bodies.push(request.postDataJSON()); if (bodies.length === 2) await new Promise(resolve => { release = resolve; }); return { body: { ok: true } }; }
  });
  try {
    await page.locator('tr').filter({ has: page.getByText('primary', { exact: true }) }).getByRole('button', { name: 'ui.edit', exact: true }).click();
    const form = page.getByRole('dialog');
    await form.getByText('ui.ch.test_saved_hint', { exact: true }).waitFor();
    await form.getByRole('button', { name: 'ui.ch.test_saved', exact: true }).click();
    await form.getByRole('status').getByText('ui.ch.test_ok', { exact: true }).waitFor();
    assert.deepEqual(bodies[0], { name: 'primary' });
    const url = form.locator('input').nth(1);
    await url.fill('https://changed.invalid');
    assert.equal(await form.getByRole('status').count(), 0);
    await form.getByText('ui.ch.test_saved_hint', { exact: true }).waitFor();
    await form.locator('input[type=password]').fill('replacement-secret');
    await form.getByText('ui.ch.test_current_hint', { exact: true }).waitFor();
    await form.getByRole('button', { name: 'ui.ch.test_current', exact: true }).evaluate(el => { el.click(); el.click(); });
    await form.getByRole('status').getByText('ui.ch.testing', { exact: true }).waitFor();
    await url.fill('https://newer.invalid');
    release(); await page.waitForTimeout(100);
    assert.equal(await form.getByRole('status').count(), 0);
    assert.equal(bodies.length, 2);
    assert.deepEqual(bodies[1], { name: 'primary', kind: 'feishu', opts: { url: 'https://changed.invalid', secret: 'replacement-secret' } });
    assert.deepEqual(errors, []);
  } finally { release?.(); await page.close(); }
});
test('overview searches names, kinds and policy usage and filters actionable issues', async () => {
  const { page, errors } = await setup();
  try {
    await page.getByRole('searchbox', { name: 'ui.ch.search' }).fill('production');
    assert.equal(await page.locator('tbody tr').count(), 1);
    await page.getByRole('searchbox').fill('Feishu');
    assert.equal(await page.locator('tbody tr').count(), 2);
    await page.getByRole('button', { name: 'ui.ch.needs_attention', exact: false }).click();
    assert.equal(await page.locator('tbody tr').count(), 1);
    await page.locator('tbody').getByText('backup', { exact: true }).waitFor();
    await page.locator('.channel-issues').waitFor();
    await page.getByRole('searchbox').fill('not-found');
    await page.getByText('ui.ch.no_matches', { exact: true }).waitFor();
    assert.deepEqual(errors, []);
  } finally { await page.close(); }
});
test('channel layout fits a narrow viewport with wrapping actions', async () => {
  const { page } = await setup();
  try {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.addStyleTag({ url: base + '/app.css' });
    await page.addStyleTag({ url: base + '/channels.css' }).catch(() => {});
    await page.getByRole('searchbox').waitFor();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    assert.equal(await page.locator('.channel-actions').first().evaluate(el => getComputedStyle(el).flexWrap), 'wrap');
  } finally { await page.close(); }
});
test('testing a new channel sends the default values actually displayed', async () => {
  let body;
  const { page } = await setup((path, request) => {
    if (path === '/channel-kinds') return { body: [{ kind: 'feishu', label: 'Feishu', fields: [{ key: 'url', type: 'text', default: 'https://default.invalid' }] }] };
    if (path === '/channels/test') { body = request.postDataJSON(); return { body: { ok: true } }; }
  });
  try {
    await page.getByRole('button', { name: 'ui.ch.add', exact: false }).click();
    const form = page.getByRole('dialog');
    assert.equal(await form.locator('input').nth(1).inputValue(), 'https://default.invalid');
    await form.getByRole('button', { name: 'ui.ch.test_current', exact: true }).click();
    await form.getByText('ui.ch.test_ok', { exact: true }).waitFor();
    assert.equal(body.opts.url, 'https://default.invalid');
  } finally { await page.close(); }
});
test('contact lookup still offers unknown recipients before saving masked options with etag', async () => {
  const calls = [];
  const { page, errors } = await setup((path, request) => {
    if (path === '/channel-kinds') return { body: [{ kind: 'email', label: 'Email', fields: [{ key: 'to', type: 'email-list' }, { key: 'password', type: 'secret' }] }] };
    if (path === '/channels' && request.method() === 'GET') return { body: [{ name: 'mail', kind: 'email', opts: { to: 'ops@example.invalid', password: '***' }, used_by: [], issues: [], etag: 'v1' }] };
    if (request.method() === 'POST') calls.push({ path, body: request.postDataJSON(), match: request.headers()['if-match'] });
    if (path === '/people/lookup') return { body: { kind: 'email', unknown: ['ops@example.invalid'] } };
  });
  try {
    await page.getByRole('button', { name: 'ui.edit', exact: true }).click();
    assert.equal(await page.locator('input[type=password]').inputValue(), '***');
    await page.getByRole('button', { name: 'ui.save', exact: true }).click();
    await page.getByRole('button', { name: 'ui.dir.save_anyway', exact: true }).waitFor();
    assert.deepEqual(calls.map(c => c.path), ['/people/lookup']);
    await page.getByRole('button', { name: 'ui.dir.save_anyway', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'detached' });
    assert.deepEqual(calls[1], { path: '/channels', body: { name: 'mail', kind: 'email', opts: { to: 'ops@example.invalid', password: '***' } }, match: 'v1' });
    assert.deepEqual(errors, []);
  } finally { await page.close(); }
});
test('channel-kind failures are visible and retry recovers', async () => {
  let calls = 0;
  const { page, errors } = await setup(path => path === '/channel-kinds' ? (++calls === 1 ? { status: 503, body: { message: 'Kinds temporarily unavailable' } } : { body: kinds }) : undefined);
  try {
    await page.getByText('Kinds temporarily unavailable').waitFor();
    await page.getByRole('button', { name: 'ui.retry', exact: true }).click();
    await page.getByText('primary', { exact: true }).waitFor();
    assert.equal(calls, 2); assert.deepEqual(errors, []);
  } finally { await page.close(); }
});
