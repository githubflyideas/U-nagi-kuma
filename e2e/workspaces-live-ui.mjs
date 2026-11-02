import { chromium } from 'playwright';
import assert from 'node:assert/strict';
const [base,target,selection]=process.argv.slice(2);
const sources=selection?selection.split(','):['gatus','checkmate'];
assert.match(base,/^http:\/\/127\.0\.0\.1:\d+$/);
assert.match(target,/^http:\/\/127\.0\.0\.1:\d+$/);
const browser=await chromium.launch({headless:true});
const page=await browser.newPage();
const errors=[];page.on('pageerror',e=>errors.push(e.stack));
await page.route('**/*',r=>new URL(r.request().url()).origin===base?r.continue():r.abort());
try {
 await page.goto(base);
 await page.fill('input[autocomplete=username]','admin');
 await page.locator('input[autocomplete=new-password]').nth(0).fill('workspace-browser-disposable');
 await page.locator('input[autocomplete=new-password]').nth(1).fill('workspace-browser-disposable');
 await page.click('.auth .btn.primary');await page.waitForSelector('nav.nav');
 await page.locator('nav a[href="#/workspaces"]').click({timeout:5000});
 await page.getByTestId('workspace-name').fill('Browser department');
 await page.getByTestId('workspace-create').click();
 await page.getByRole('button',{name:'Browser department',exact:true}).click();
 for(const source of sources){
  await page.getByTestId('workspace-import-source').selectOption(source);
  const text=source==='gatus'?`endpoints:\n  - name: browser-gatus\n    url: ${target}\n    interval: 1s\n    client: {timeout: 1s}\n    conditions: ['[STATUS] == 200']\n`:JSON.stringify([{name:'browser-checkmate',type:'http',url:target,method:'GET',interval:1000,isActive:true,proxyMode:'none',statusWindowSize:1,statusWindowThreshold:100}]);
  await page.getByTestId('workspace-import-text').fill(text);
  await page.getByTestId('workspace-import-preview').click();
  await page.getByTestId('workspace-import-result').waitFor();
  assert.match(await page.getByTestId('workspace-import-result').innerText(),/1/);
  await page.getByTestId('workspace-import-apply').click();
  await page.getByTestId('workspace-success').waitFor();
 }
 await page.getByTestId('workspace-refresh').click();
 await page.waitForFunction(sources=>sources.every(s=>document.querySelector('[data-testid="workspace-monitors"]')?.textContent.includes('browser-'+s)),sources,{timeout:15000});
 const global=await page.evaluate(async()=>(await fetch('/api/v1/monitors')).json());
 assert.equal(global.items.some(m=>m.name.startsWith('browser-')),false,'global default API stays default-only');
 await page.getByTestId('workspace-notifications').waitFor();
 await page.reload();await page.getByRole('button',{name:'Browser department',exact:true}).click();
 await page.getByTestId('workspace-monitors').waitFor();
 assert.match(await page.getByTestId('workspace-monitors').innerText(),/browser-gatus/);
 assert.deepEqual(errors,[]);
 console.log('PASS real Chromium: authenticated create, '+sources.join('/')+' preview/apply, scoped live monitors, default isolation, reload and runtime notification status');
} catch(error) {console.error('Workspace UI failure:',await page.locator('.main').innerText());console.error('page errors:',errors);throw error;} finally {await browser.close();}
