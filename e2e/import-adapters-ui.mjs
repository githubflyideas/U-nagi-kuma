// Real backend + Chromium, disposable store; imported monitors stay disabled.
import { chromium } from 'playwright';
import { readFile } from 'node:fs/promises';
import assert from 'node:assert/strict';
const [base]=process.argv.slice(2);
assert.match(base,/^http:\/\/127\.0\.0\.1:\d+$/);
const browser=await chromium.launch({headless:true});
const page=await browser.newPage();
const errors=[];page.on('pageerror',e=>errors.push(e.message));
await page.route('**/*',r=>new URL(r.request().url()).origin===base?r.continue():r.abort());
const api=p=>page.evaluate(async p=>(await fetch('/api/v1'+p)).json(),p);
try {
 await page.goto(base);
 await page.fill('input[autocomplete=username]','admin');
 await page.locator('input[autocomplete=new-password]').nth(0).fill('import-browser-disposable');
 await page.locator('input[autocomplete=new-password]').nth(1).fill('import-browser-disposable');
 await page.click('.auth .btn.primary');await page.waitForSelector('nav.nav');
 await page.goto(base+'/#/config/import');
 const labels=await page.evaluate(async()=>{const {t}=await import('/js/i18n.js');return {preview:t('ui.import.preview'),apply:t('ui.import.apply')};});
 for (const [source,title,ext] of [['gatus','Gatus','yaml'],['checkmate','Checkmate','json']]) {
  await page.getByRole('radio',{name:new RegExp(title)}).click({timeout:5000});
  const file=new URL(`../internal/importer/testdata/${source}.${ext}`,import.meta.url);
  const data=await readFile(file);
  await page.locator('input[type=file]').setInputFiles({name:`config.${ext}`,mimeType:'text/plain',buffer:data});
  const serial=(await api('/config')).serial;
  await page.getByRole('button',{name:labels.preview,exact:true}).click();
  await page.locator('pre.code').waitFor();
  assert.equal((await api('/config')).serial,serial,'preview is read-only');
  assert.match(await page.locator('pre.code').innerText(),/disabled=yes/);
  await page.getByRole('button',{name:labels.apply,exact:true}).click();
  await page.locator('.banner.ok').waitFor();
  const mons=await api('/monitors');
  assert.equal(mons.items.some(m=>m.name===source+'-example'),true,'actual stored configuration visible');
  await page.getByRole('button',{name:labels.preview,exact:true}).click();
  await page.getByRole('button',{name:labels.apply,exact:true}).waitFor();
  assert.equal(await page.getByRole('button',{name:labels.apply,exact:true}).isDisabled(),true,'collision blocks UI apply');
 }
 await page.getByRole('radio',{name:/Gatus/}).click();
 const unsafe="endpoints:\n  - name: unsafe\n    url: https://example.invalid\n    enabled: false\n    conditions: ['[BODY].ok == true']\n";
 await page.locator('input[type=file]').setInputFiles({name:'unsafe.yaml',mimeType:'text/plain',buffer:Buffer.from(unsafe)});
 await page.getByRole('button',{name:labels.preview,exact:true}).click();
 await page.getByRole('button',{name:labels.apply,exact:true}).waitFor();
 assert.equal(await page.getByRole('button',{name:labels.apply,exact:true}).isDisabled(),true);
 assert.match(await page.locator('.banner.warning').innerText(),/unsupported condition/);
 assert.deepEqual(errors,[]);
 console.log('PASS real browser/API Checkmate + Gatus preview/apply, persisted monitors, collision and unsupported-condition blocking; zero outbound probes');
} finally {await browser.close();}
