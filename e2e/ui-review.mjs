// Isolated real-server visual acceptance. All channels write to the local log;
// no notification providers, personal accounts or external targets are used.
// node e2e/ui-review.mjs BINARY OUTPUT_DIR [baseline]
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import { mkdir, writeFile, mkdtemp } from 'node:fs/promises';
import { createServer } from 'node:net';
import { randomBytes } from 'node:crypto';
import assert from 'node:assert/strict';
import path from 'node:path';
const [binary, dest, baseline] = process.argv.slice(2);
assert.ok(binary && dest, 'binary and output directory required');
const out = path.resolve(dest);
await mkdir(out, {recursive: true});
const data = await mkdtemp(path.join(out, 'isolated-data-'));
const listener = createServer();
await new Promise(resolve => listener.listen(0, '127.0.0.1', resolve));
const port = listener.address().port;
await new Promise(resolve => listener.close(resolve));
const now = new Date();
const day = new Date(now.getTime() - 7 * 86400000).toISOString().slice(0,10);
await writeFile(path.join(data, 'monitors.bind'), `@ IN SOA ui-review. admin.localhost. ( 2026102801 30s 5s 1s 1 )
$SET engine.first-round-delay=0s
lin IN CHANNEL log
chen IN CHANNEL log
wang IN CHANNEL log
ops-log IN CHANNEL log
unused-log IN CHANNEL log
primary IN CHANNEL group members=lin,chen,wang rotate=daily handoff=09:00 start=${day} tz=Asia/Shanghai
backup IN CHANNEL group members=wang,chen rotate=weekly handoff="mon 09:00" start=${day} tz=Asia/Tokyo
default IN POLICY chain=primary,ops-log
`);
const proc = spawn(path.resolve(binary), ['server','--data', data,'--listen',`127.0.0.1:${port}`], {stdio:['ignore','pipe','pipe']});
let log = '';
proc.stdout.on('data', x => { log += x; });
proc.stderr.on('data', x => { log += x; });
const browser = await chromium.launch();
const errors = [];
const evidence = [];
try {
  const base = `http://127.0.0.1:${port}`;
  let ready = false;
  for (let i=0;i<120;i++) {
    if (proc.exitCode !== null) throw new Error('server exited: ' + log.replace(/token[^\n]*/gi,'token [redacted]'));
    try { if ((await fetch(base+'/healthz')).ok) {ready=true; break;} } catch { /* startup */ }
    await new Promise(r => setTimeout(r,250));
  }
  assert.ok(ready,'server ready');
  const ctx = await browser.newContext({viewport:{width:1440,height:1000},locale:'zh-CN',timezoneId:'Asia/Shanghai'});
  const page = await ctx.newPage();
  page.on('pageerror', e => errors.push(e.message));
  await page.goto(base);
  const password = randomBytes(20).toString('hex');
  await page.fill('input[autocomplete=username]', 'review');
  await page.locator('input[autocomplete=new-password]').nth(0).fill(password);
  await page.locator('input[autocomplete=new-password]').nth(1).fill(password);
  await page.click('.auth .btn.primary');
  await page.waitForSelector('nav.nav');
  const palettes = baseline ? ['dark'] : ['midnight','paper','dark','light'];
  for (const theme of palettes) {
    await page.selectOption('select:has(option[value=dark])', theme);
    for (const [route, name, readySelector] of [['#/alerts/channels','channels','.tbl'], ['#/oncall','oncall','.oc-rot']]) {
      await page.goto(base+'/'+route);
      await page.waitForSelector(readySelector);
      if (name==='oncall') await page.waitForSelector('.oc-cal');
      await page.screenshot({path:path.join(out, `${name}-${theme}.png`), fullPage:true, animations:'disabled'});
      evidence.push(`${name}-${theme}.png`);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, `${name} ${theme} desktop overflow`);
    }
  }
  if (!baseline) {
    for (const theme of ['midnight','paper']) {
      await page.selectOption('select:has(option[value=dark])',theme);
      await page.setViewportSize({width:390,height:844});
      for (const [route,name] of [['#/alerts/channels','channels'],['#/oncall','oncall']]) {
        await page.goto(base+'/'+route);
        await page.waitForSelector(name==='channels'?'.tbl':'.oc-rot');
        if (name==='oncall') await page.waitForSelector('.oc-cal');
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth),false,`${name} ${theme} mobile overflow`);
        await page.screenshot({path:path.join(out,`${name}-${theme}-mobile.png`),fullPage:true, animations:'disabled'});
        evidence.push(`${name}-${theme}-mobile.png`);
      }
      await page.setViewportSize({width:1440,height:1000});
    }
  }
  assert.deepEqual(errors, [], 'no browser errors');
  await writeFile(path.join(out,'verification.json'),JSON.stringify({backend:'real local executable', channels:'local log only', evidence, errors},null,2));
  console.log(JSON.stringify({ok:true,screenshots:evidence.length,out,errors}));
} finally {
  await browser.close();
  proc.kill();
}
