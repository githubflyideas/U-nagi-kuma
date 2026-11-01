// Chromium contracts over the production component; isolated HTTP fixtures.
import { chromium } from 'playwright';
import { createServer } from 'node:http';
import { readFile, readdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';
const root=fileURLToPath(new URL('../',import.meta.url));
let zones=[],writes=[],stale=false,hold=null,lang='en',role='admin';
const zone={id:'11111111-1111-4111-8111-111111111111',name:'demo',revision:'"'+'a'.repeat(64)+'"',role:'local',applied_revision:'',sync_status:'not_connected',source_node:'',fingerprint:''};
const server=createServer(async(req,res)=>{
 const path=new URL(req.url,'http://localhost').pathname;
 const json=data=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify(data));};
 if(path==='/api/v1/session')return json({role,csrf:'zone-csrf'});
 if(path==='/api/v1/zones'&&req.method==='GET')return json({items:zones});
 if(path.startsWith('/api/v1/zones/')&&req.method==='POST') {
  let raw='';for await(const c of req)raw+=c;const body=JSON.parse(raw);writes.push({path,body,headers:req.headers});
  if(path.endsWith('/preview'))return json({monitors:['web'],missing:['secret:missing']});
  if(path.endsWith('/import')) {if(hold)await hold;zones=[{...zone}];return json(zones[0]);}
  if(path.endsWith('/apply')) {if(stale){res.statusCode=412;return json({code:'record_changed',message:'Refresh required'});}zones[0].applied_revision=zone.revision;return json(zones[0]);}
  if(path.endsWith('/rebind')){zones[0]={...zones[0],role:'secondary',source_node:body.source_node,fingerprint:body.fingerprint,revision:'"'+'b'.repeat(64)+'"'};return json(zones[0]);}
 }
 if(path.endsWith('/export'))return json({text:'portable redacted configuration',missing:['secret:portable']});
 if(path.startsWith('/api/v1/i18n/'))return json(JSON.parse(await readFile(root+'internal/i18n/locales/'+path.split('/').at(-1)+'.json','utf8')));
 if(path==='/'){res.setHeader('Content-Type','text/html');return res.end('<meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/app.css"><div id="app"></div><script type="module">import {html,render} from "/js/lib.js";import {setLang} from "/js/i18n.js";import {setCSRF} from "/js/api.js";import {Zones} from "/js/views/zones.js";await setLang('+JSON.stringify(lang)+');setCSRF("zone-csrf");render(html`<${Zones} />`,document.getElementById("app"));</script>');}
 try{res.setHeader('Content-Type',path.endsWith('.js')?'text/javascript':'text/css');res.end(await readFile(root+'web/ui'+path));}catch{res.statusCode=404;res.end();}
});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
const browser=await chromium.launch({headless:true});const page=await browser.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
await page.route('**/*',r=>new URL(r.request().url()).hostname==='127.0.0.1'?r.continue():r.abort());
try{
 await page.goto(`http://127.0.0.1:${server.address().port}`);
 await page.getByTestId('zone-name').fill('demo');await page.getByTestId('zone-text').fill('original draft');
 await page.getByTestId('zone-preview').click();await page.getByText('secret:missing',{exact:true}).waitFor();
 let release;hold=new Promise(r=>release=r);
 await page.getByTestId('zone-import').evaluate(b=>{b.click();b.click();});
 await page.waitForTimeout(100);assert.equal(writes.filter(w=>w.path.endsWith('/import')).length,1);
 assert.equal(await page.getByTestId('zone-text').isDisabled(),true);release();hold=null;
 await page.locator('[data-zone-id]').waitFor();assert.equal(writes.some(w=>w.path.endsWith('/apply')),false,'import is draft only');
 const card=page.locator('[data-zone-id]');assert.match(await card.innerText(),/Draft/);
 stale=true;await card.getByTestId('zone-apply').click();await page.getByTestId('zone-error').waitFor();
 assert.match(await page.getByTestId('zone-error').innerText(),/refresh|review/i);assert.equal(await page.getByTestId('zone-text').inputValue(),'original draft');
 stale=false;await card.getByTestId('zone-apply').click();await page.getByText('Applied',{exact:true}).waitFor();
 await card.getByTestId('zone-export').click();await page.getByTestId('zone-export-text').waitFor();assert.equal(await page.getByTestId('zone-export-text').inputValue(),'portable redacted configuration');
 await card.getByTestId('zone-rebind').click();await page.getByTestId('zone-source').fill('upstream');await page.getByTestId('zone-fingerprint').fill('f'.repeat(64));await page.getByTestId('zone-confirm').check();await page.getByTestId('zone-binding-save').click();
 await page.getByText('upstream',{exact:true}).waitFor();assert.match(await card.innerText(),/not.*sync|no.*sync/i);
 for(const w of writes){assert.equal(w.headers['x-unagikuma-csrf'],'zone-csrf');if(/apply|rebind/.test(w.path))assert.equal(w.headers['if-match'],zone.revision);}
 assert.deepEqual(writes.find(w=>w.path.endsWith('/import')).body,{name:'demo',text:'original draft'});
 assert.equal(writes.find(w=>w.path.endsWith('/rebind')).body.confirm,true);
 const en=JSON.parse(await readFile(root+'internal/i18n/locales/en.json','utf8'));const keys=Object.keys(en).filter(k=>k.startsWith('ui.zone.'));assert.ok(keys.length>10);
 for(const file of (await readdir(root+'internal/i18n/locales')).filter(f=>f.endsWith('.json'))){const cat=JSON.parse(await readFile(root+'internal/i18n/locales/'+file,'utf8'));for(const k of keys)assert.equal(typeof cat[k],'string',file+' '+k);}
 await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);
 role='viewer';await page.reload();await page.getByTestId('zone-forbidden').waitFor();assert.equal(await page.getByTestId('zone-import').count(),0);
 assert.deepEqual(errors,[]);console.log('PASS zone draft/preview/import locking, explicit apply, CAS conflict, export, confirmed metadata rebind, CSRF, role gate, catalogs and mobile');
}finally{await browser.close();await new Promise(r=>server.close(r));}
