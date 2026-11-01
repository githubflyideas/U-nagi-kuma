// Real application process + Chromium, loopback probe and disposable store.
import { chromium } from 'playwright';
import { createServer } from 'node:http';
import assert from 'node:assert/strict';
const [base]=process.argv.slice(2);
assert.match(base,/^http:\/\/127\.0\.0\.1:\d+$/);
const target=createServer((req,res)=>{res.end('healthy');});await new Promise(r=>target.listen(0,'127.0.0.1',r));
const browser=await chromium.launch({headless:true});const page=await browser.newPage({viewport:{width:1366,height:1000}});const errors=[];page.on('pageerror',e=>errors.push(e.message));
await page.route('**/*',r=>new URL(r.request().url()).origin===base?r.continue():r.abort());
const api=(path)=>page.evaluate(async p=>(await fetch('/api/v1'+p)).json(),path);
const until=async fn=>{for(let i=0;i<80;i++){if(await fn())return;await page.waitForTimeout(150);}throw Error('runtime evidence timeout');};
try{
 await page.goto(base);
 await page.fill('input[autocomplete=username]','admin');await page.locator('input[autocomplete=new-password]').nth(0).fill('zone-browser-disposable');await page.locator('input[autocomplete=new-password]').nth(1).fill('zone-browser-disposable');await page.click('.auth .btn.primary');await page.waitForSelector('nav.nav');
 await page.locator('nav a[href="#/zones"]').click();await page.getByTestId('zone-name').fill('browser-zone');
 const text=`@ IN SOA local. admin.localhost. ( 1 1s 500ms 300ms 1 )\nweb IN HTTP http://127.0.0.1:${target.address().port}/ header.Authorization=browser-private-token\n`;
 await page.getByTestId('zone-text').fill(text);await page.getByTestId('zone-preview').click();await page.locator('fieldset li').getByText('web',{exact:true}).waitFor();
 const before=await api('/config');await page.getByTestId('zone-import').click();await page.locator('[data-zone-id]').waitFor();
 let zones=(await api('/zones')).items;assert.equal(zones.length,1);const z=zones[0],name='zone-'+z.id+'.web';assert.equal(z.applied_revision,'');assert.equal(JSON.stringify(z).includes('browser-private-token'),false);
 assert.equal((await api('/monitors')).items.some(m=>m.name===name),false,'draft must not run');
 const card=page.locator('[data-zone-id]');await card.getByTestId('zone-apply').click();await page.getByText('Applied',{exact:true}).waitFor();
 await until(async()=> (await api('/monitors')).items.some(m=>m.name===name&&m.status==='ok'&&m.last_check));
 assert.equal((await api('/config')).text,before.text,'base editing remains isolated');
 const detail=await api('/monitors/'+name);assert.equal(detail.readonly,true);assert.equal(detail.record,undefined);assert.equal(JSON.stringify(detail).includes('browser-private-token'),false);
 await card.getByTestId('zone-export').click();await page.getByTestId('zone-export-text').waitFor();assert.equal((await page.getByTestId('zone-export-text').inputValue()).includes('browser-private-token'),false);
 await card.getByTestId('zone-rebind').click();await page.getByTestId('zone-source').fill('offline-source');await page.getByTestId('zone-fingerprint').fill('a'.repeat(64));await page.getByTestId('zone-confirm').check();await page.getByTestId('zone-binding-save').click();await page.getByText('offline-source',{exact:true}).waitFor();
 assert.match(await card.innerText(),/no authenticated network synchronization/);
 await page.goto(base+'/#/monitors/'+name);await page.locator('.mon-head').waitFor();
 assert.equal(await page.locator('.mon-head a[href$="/edit"]').count(),0,'zone must not open base-record editor');
 const actionLabels=await page.evaluate(async()=>{const {t}=await import('/js/i18n.js');return {remove:t('ui.delete'),pause:t('ui.mon.pause')};});
 assert.equal(await page.locator('.mon-head').getByRole('button',{name:actionLabels.remove,exact:true}).count(),0);
 assert.equal(await page.locator('.mon-head').getByRole('button',{name:actionLabels.pause,exact:true}).count(),0);
 // Missing dependencies surface before applying and leave the good zone running.
 await page.goto(base+'/#/zones');await page.getByTestId('zone-name').fill('missing-zone');await page.getByTestId('zone-text').fill(text.replace('browser-private-token','${secret:browser-missing}'));
 await page.getByTestId('zone-preview').click();await page.getByText('secret:browser-missing',{exact:true}).waitFor();await page.getByTestId('zone-import').click();await page.locator('[data-zone-id]').nth(1).waitFor();
 const missingCard=page.locator('[data-zone-id]').filter({has:page.getByRole('heading',{name:'missing-zone',exact:true})});await missingCard.getByTestId('zone-apply').click();await page.getByTestId('zone-error').waitFor();assert.match(await page.getByTestId('zone-error').innerText(),/secret:browser-missing/);
 assert.equal((await api('/zones')).items.find(v=>v.name==='missing-zone').applied_revision,'');assert.equal((await api('/monitors')).items.some(m=>m.name===name&&m.status==='ok'),true);
 assert.deepEqual(errors,[]);console.log('PASS real-backend browser setup/navigation, draft isolation, loopback scheduled check, secret-safe reads/export, offline-source metadata, readonly monitor UI and dependency failure isolation');
}finally{await browser.close();await new Promise(r=>target.close(r));}
