// Browser tests of the web UI against a running server.
// Usage: node e2e/ui.mjs http://127.0.0.1:PORT OUTDIR NAGIOS_CFG
// Run through `go test -tags e2e ./e2e/`, which starts the server.
import { createRequire } from 'module';
const require = createRequire(import.meta.url);
const { chromium } = require('playwright');

const [base, out, nagiosCfg] = process.argv.slice(2);
const browser = await chromium.launch();
const errors = [];
let failed = 0;
let ctx = await browser.newContext({ viewport: { width: 1366, height: 900 }, locale: 'en-US' });
let page = await ctx.newPage();
const watch = (p) => p.on('pageerror', (e) => errors.push(e.message));
watch(page);

async function step(name, fn) {
  try {
    await fn();
    console.log('ok   ' + name);
  } catch (e) {
    failed++;
    console.log('FAIL ' + name + ': ' + e.message.split('\n')[0]);
    await page.screenshot({ path: `${out}/${name.replace(/\W+/g, '-')}.png`, fullPage: true }).catch(() => {});
  }
}
const api = (path) => page.evaluate(async (p) => (await fetch('/api/v1' + p)).json(), path);
async function until(what, cond, ms = 15000) {
  const end = Date.now() + ms;
  while (!(await cond())) {
    if (Date.now() > end) throw new Error('timed out: ' + what);
    await page.waitForTimeout(250);
  }
}

await step('first run creates the admin', async () => {
  await page.goto(base);
  await page.fill('input[autocomplete=username]', 'admin');
  await page.locator('input[autocomplete=new-password]').nth(0).fill('ui-test-pass-1');
  await page.locator('input[autocomplete=new-password]').nth(1).fill('ui-test-pass-1');
  await page.click('.auth .btn.primary');
  await page.waitForSelector('nav.nav');
});

await step('overview shows the first-run guide', async () => {
  await page.waitForSelector('.ob');
});

await step('command palette adds a monitor', async () => {
  await page.keyboard.press('Control+k');
  await page.waitForSelector('.pal input');
  await page.keyboard.type('127.0.0.1:1');
  await page.waitForTimeout(300);
  await page.keyboard.press('Enter');
  await page.waitForSelector('.drawer', { timeout: 10000 });
  await page.keyboard.press('Escape');
  await until('the new monitor to fail', async () => {
    const r = await api('/monitors');
    return r.items.some((m) => m.target === '127.0.0.1:1' && m.status === 'critical');
  });
});

await step('pages render with delivery statistics', async () => {
  await until('a notification to be delivered', async () => (await api('/overview')).channels.some((c) => c.sent_24h > 0));
  for (const [route, sel] of [['#/', '.fp-scroll'], ['#/monitors', '.mon-list, .tbl, .hive'], ['#/problems', '.page']]) {
    await page.goto(base + '/' + route);
    await page.waitForSelector(sel, { timeout: 8000 });
  }
  if (errors.length) throw new Error(errors.join(' | '));
});

await step('monitor list and detail open', async () => {
  await page.goto(base + '/#/monitors');
  const name = (await api('/monitors')).items.find((m) => m.target === '127.0.0.1:1').name;
  // Clicking a row opens the monitor's page.
  await page.click(`tr:has(a.name[href="#/monitors/${encodeURIComponent(name)}"]) td:nth-child(3)`);
  await page.waitForSelector('.mon-head');
});

await step('ticked monitors are deleted together', async () => {
  await page.goto(base + '/#/monitors');
  await page.click('button:has-text("Quick add monitors")');
  await page.fill('.modal textarea', '127.0.0.1:2\n127.0.0.1:3');
  await page.click('.modal button:has-text("Preview")');
  await page.click('.modal .btn.primary:has-text("Add")');
  await until('two new monitors', async () => (await api('/monitors')).items.filter((m) => /^127\.0\.0\.1:[23]$/.test(m.target)).length === 2);
  const names = (await api('/monitors')).items.filter((m) => /^127\.0\.0\.1:[23]$/.test(m.target)).map((m) => m.name);
  await page.goto(base + '/#/monitors?q=' + encodeURIComponent('127.0.0.1:'));
  for (const n of names) await page.check(`tr:has(a.name[href="#/monitors/${encodeURIComponent(n)}"]) td.sel input`);
  await page.click('button.danger:has-text("Delete (2)")');
  await page.click('.modal .btn.danger');
  await until('both monitors deleted', async () => !(await api('/monitors')).items.some((m) => names.includes(m.name)));
  if (!(await api('/monitors')).items.some((m) => m.target === '127.0.0.1:1')) throw new Error('an unticked monitor was deleted');
});

await step('configuration editor shows monitors.bind', async () => {
  await page.goto(base + '/#/config/edit');
  // Polled with evaluate: waitForFunction needs eval, which the UI's CSP forbids.
  await until('monitors.bind in the editor', () => page.evaluate(() =>
    document.body.innerText.includes('IN SOA') || [...document.querySelectorAll('textarea')].some((t) => t.value.includes('IN SOA'))));
});

await step('Nagios import previews', async () => {
  await page.goto(base + '/#/config/import');
  await page.click('.src-card:has-text("Nagios")');
  await page.setInputFiles('input[type=file]', nagiosCfg);
  await page.click('button:has-text("Preview")');
  await page.waitForSelector('pre.code:has-text("IN PING")');
});

await step('a channel is added', async () => {
  await page.goto(base + '/#/alerts/channels');
  await page.click('button:has-text("Add channel")');
  await page.selectOption('.modal select', 'log');
  await page.fill('.modal input[pattern]', 'ops-log');
  await page.click('.modal .btn.primary');
  await page.waitForSelector('td:has-text("ops-log")');
});

await step('a new e-mail address is offered for the contact directory', async () => {
  await page.goto(base + '/#/alerts/channels');
  await page.click('button:has-text("Add channel")');
  await page.selectOption('.modal select', 'email');
  await page.fill('.modal input[pattern]', 'ops-mail');
  const field = (label) => `.modal .field:has(label:text-is("${label}")) input`;
  await page.fill(field('SMTP server *'), '127.0.0.1');
  await page.fill(field('From *'), 'mon@example.com');
  await page.fill(field('To *'), 'li@example.com');
  await page.click('.modal .btn.primary:has-text("Save")');
  await page.waitForSelector('.modal h2:has-text("New addresses")');
  await page.fill('.modal:has(h2:has-text("New addresses")):not(:has(form)) input', 'Li Lei');
  await page.click('button:has-text("Add and save")');
  await page.waitForSelector('td:has-text("ops-mail")');
  if (await page.$('tr:has-text("ops-mail") .rdot')) throw new Error('a registered address has a red dot');
  // Li leaves: the channel gets a red dot with the reason.
  await page.goto(base + '/#/alerts/contacts');
  await page.waitForSelector('td:has-text("Li Lei")');
  await page.click('tr:has-text("Li Lei") button:has-text("Mark as left")');
  await page.click('.modal .btn.danger');
  await page.waitForSelector('tr:has-text("Li Lei") .tag');
  await page.goto(base + '/#/alerts/channels');
  await page.waitForSelector('tr:has-text("ops-mail") .rdot');
  // It is a problem too: listed on the Problems page, whose menu entry gets a red dot.
  await page.goto(base + '/#/problems');
  await page.waitForSelector('.setup-issues a[href="#/alerts/contacts"]');
  await page.waitForSelector('.nav a[href="#/problems"] .rdot');
  if (errors.length) throw new Error(errors.join(' | '));
});

await step('a change is announced and closed with a receipt', async () => {
  await page.goto(base + '/#/notices/changes');
  await page.click('button:has-text("Announce a change")');
  await page.fill('.modal .field:has(label:text-is("Title")) input', 'Core switch upgrade');
  await page.fill('.modal .field:has(label:text-is("Impact")) input', 'VPN down for 10 minutes');
  await page.click('.modal .btn.primary:has-text("Announce")');
  await page.waitForSelector('td:has-text("Core switch upgrade")');
  await page.click('tr:has-text("Core switch upgrade") button:has-text("Done")');
  await page.fill('.modal textarea', 'Upgraded to 17.9, links checked');
  await page.click('.modal .btn.primary');
  await page.waitForSelector('tr:has-text("Core switch upgrade") .tag:has-text("Done")');
  await until('the notices to be delivered', async () => (await api('/notifications')).items.filter((n) => n.kind === 'CHANGE').length >= 2);
  if (errors.length) throw new Error(errors.join(' | '));
});

await step('an announcement reaches the public status page', async () => {
  await page.goto(base + '/#/notices/pages');
  await page.click('button:has-text("New announcement")');
  await page.fill('.modal input[type=text]', 'Planned database upgrade');
  await page.click('.modal .btn.primary');
  await page.waitForSelector('td:has-text("Planned database upgrade")');
  const pub = await browser.newPage();
  watch(pub);
  await pub.goto(base + '/status/public');
  await pub.waitForSelector('.notice:has-text("Planned database upgrade")');
  await pub.close();
});

await step('a rotation shows who is on call and takes a stand-in', async () => {
  await page.goto(base + '/#/oncall');
  await page.click('button:has-text("New rotation")');
  const boxes = page.locator('.modal label.switch input');
  await boxes.nth(0).check();
  await boxes.nth(1).check();
  await page.click('.modal .btn.primary:has-text("Save")');
  await page.waitForSelector('.oc-rot .oc-now b');
  const before = await page.textContent('.oc-rot .oc-now b');
  await page.click('.oc-rot button:has-text("Arrange a stand-in")');
  await page.click('.modal .btn.primary:has-text("Save")');
  await page.waitForSelector('.oc-rot .oc-now .tag');
  if ((await page.textContent('.oc-rot .oc-now b')) === before) throw new Error('the stand-in is not on call');
  await page.click('.oc-rot button:has-text("Cancel")');
  await page.click('.modal .btn.danger');
  await until('the stand-in to be cancelled', async () => !(await page.$('.oc-rot .oc-now .tag')));
  if (errors.length) throw new Error(errors.join(' | '));
});

await step('the language can be switched', async () => {
  await page.goto(base + '/#/');
  await page.selectOption('select:has(option[value="zh"])', 'zh');
  await page.waitForSelector('nav.nav:has-text("监控")');
  await page.selectOption('select:has(option[value="en"])', 'en');
  await page.waitForSelector('nav.nav:has-text("Monitors")');
});

await step('phones get no sideways scrolling', async () => {
  const state = await ctx.storageState();
  const phone = await browser.newContext({ viewport: { width: 390, height: 844 }, locale: 'en-US', storageState: state, isMobile: true });
  const p = await phone.newPage();
  watch(p);
  const mon = (await api('/monitors')).items.find((m) => m.target === '127.0.0.1:1').name;
  for (const route of ['#/', '#/monitors', '#/monitors/' + encodeURIComponent(mon), '#/alerts/channels', '#/alerts/contacts', '#/oncall', '#/notices/changes', '#/notices/pages', '#/settings']) {
    await p.goto(base + '/' + route);
    await p.waitForTimeout(800);
    const w = await p.evaluate(() => document.documentElement.scrollWidth);
    if (w > 392) {
      // Name the widest elements so the fix is easy to find.
      const wide = await p.evaluate(() => [...document.querySelectorAll('body *')]
        .filter((e) => e.getBoundingClientRect().right > 392)
        .slice(0, 6).map((e) => `${e.tagName.toLowerCase()}.${[...e.classList].join('.')} ${Math.round(e.getBoundingClientRect().right)}`).join(', '));
      throw new Error(`${route} is ${w}px wide: ${wide}`);
    }
  }
  await phone.close();
});

await step('no script errors', async () => {
  if (errors.length) throw new Error(errors.join(' | '));
});

await browser.close();
console.log(failed ? `${failed} failed` : 'all passed');
process.exit(failed ? 1 : 0);
