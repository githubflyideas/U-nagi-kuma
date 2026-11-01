// Real Chromium, production Nodes component, isolated API fixtures (not a live cluster).
// Run: node e2e/node-discovery-ui.mjs
import { chromium } from 'playwright';
import { createServer } from 'node:http';
import { readFile, readdir } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';
const root = fileURLToPath(new URL('../', import.meta.url));
const snapshot = {source:'configured_registry',members:['east','offline-with-a-long-name-that-must-wrap-on-mobile'],interval_s:3600,last_refresh:'2026-10-30T03:00:00Z',next_refresh:'2026-10-30T04:00:00Z'};
let discovery = snapshot, fail = false, agent = null, lang = 'en';
const writes=[];
const nodes=[{name:'local',self:true,online:true,probe:true},{name:'east',online:true,probe:true,standby:'https://192.0.2.2:8077'}, {name:snapshot.members[1],online:false,probe:true,last_seen:'2026-10-28T00:00:00Z'}];
const server=createServer(async(req,res)=>{
 const path=new URL(req.url,'http://localhost').pathname;
 const json=data=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify(data));};
 if(req.method!=='GET') {writes.push(path);return json({});}
 if(path==='/api/v1/nodes') {if(fail){res.statusCode=503;return json({message:'Registry read unavailable'});}return json({layout:'cluster',nodes,discovery,agent,instance:'local',monitors:0,server:'https://localhost:8077',fingerprint:'a'.repeat(64)});}
 if(path==='/api/v1/cluster/agent') return json({standby:{size:100,fetched:snapshot.last_refresh,term:2}});
 if(path.startsWith('/api/v1/i18n/')) return json(JSON.parse(await readFile(root+'internal/i18n/locales/'+path.split('/').at(-1)+'.json','utf8')));
 if(path==='/') {res.setHeader('Content-Type','text/html');return res.end('<meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/app.css"><div id="app" style="padding:12px;min-width:0"></div><script type="module">import {html,render} from "/js/lib.js";import {setLang} from "/js/i18n.js";import {Nodes} from "/js/views/nodes.js";await setLang('+JSON.stringify(lang)+');render(html`<${Nodes} />`,document.getElementById("app"));</script>');}
 if(path.startsWith('/api/')) return json({});
 try{res.setHeader('Content-Type',path.endsWith('.js')?'text/javascript':'text/css');res.end(await readFile(root+'web/ui'+path));}catch{res.statusCode=404;res.end();}
});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
const browser=await chromium.launch({headless:true});
const page=await browser.newPage({viewport:{width:1100,height:900},timezoneId:'UTC'});
const errors=[];page.on('pageerror',e=>errors.push(e.message));
await page.route('**/*',route=>new URL(route.request().url()).hostname==='127.0.0.1'?route.continue():route.abort());
await page.clock.install({time:new Date('2026-10-31T03:00:00Z')});
try {
 await page.goto(`http://127.0.0.1:${server.address().port}`);
 const panel=page.locator('[data-node-discovery]');
 await panel.waitFor({timeout:5000});
 assert.match(await panel.innerText(),/Configured node registry/i);
 assert.match(await panel.innerText(),/not network discovery/);
 assert.equal(await panel.locator('[data-known-members]').innerText(),'2');
 assert.match(await panel.innerText(),/3,600 seconds/);
 assert.deepEqual(await panel.locator('time').evaluateAll(es=>es.map(e=>e.dateTime)),[snapshot.last_refresh,snapshot.next_refresh]);
 assert.match(await panel.innerText(),new RegExp(snapshot.members[1]));
 assert.equal(await page.locator('tbody tr').count(),3,'legacy local and probe rows retained');
 assert.equal(await page.getByRole('button',{name:'Make master',exact:true}).count(),0,'global handover is not offered as a node/zone switch');
 assert.equal(await panel.getByRole('button').count(),0,'registry metadata is strictly read-only');
 assert.equal(await page.getByRole('button',{name:'Add node',exact:true}).count(),1);
 console.log('PASS configured schedule and known members independent of connection health; legacy controls');
 nodes[1].online=false;
 await page.clock.runFor(10001); await page.waitForTimeout(100);
 assert.equal(await panel.locator('[data-known-members]').innerText(),'2','disconnect never changes membership');
 assert.equal(await panel.locator('time').first().getAttribute('datetime'),snapshot.last_refresh,'old snapshot stays visibly old');
 fail=true;await page.clock.runFor(10001);
 await page.locator('.banner.error').waitFor();
 assert.match(await page.locator('.banner.error').innerText(),/Registry read unavailable/);
 assert.equal(await panel.locator('[data-known-members]').innerText(),'2','last good metadata retained with visible error');
 fail=false;await page.locator('.banner.error button').click();await page.locator('.banner.error').waitFor({state:'detached'});
 console.log('PASS offline/stale retention and background failure with retry');
 for(const unknown of [undefined,null,{}, {source:'other',members:['not-authorized'],interval_s:2}, {source:'configured_registry',members:null,interval_s:0,last_refresh:'0001-01-01T00:00:00Z',next_refresh:'bad'}]) {
  discovery=unknown;await page.reload();await panel.waitFor();
  assert.equal(await panel.locator('[data-known-members]').innerText(),'Unknown');
  assert.equal(await panel.locator('time').count(),0);
  assert.doesNotMatch(await panel.innerText(),/Invalid Date|not-authorized|NaN/);
 }
 discovery={...snapshot,members:[]};await page.reload();await panel.waitFor();
 assert.equal(await panel.locator('[data-known-members]').innerText(),'0');
 assert.match(await panel.innerText(),/No configured remote members/);
 console.log('PASS missing, partial, unknown-source and empty discovery are distinct');
 discovery=snapshot;
 const catalogs=(await readdir(root+'internal/i18n/locales')).filter(f=>f.endsWith('.json'));
 assert.equal(catalogs.length,15);
 const en=JSON.parse(await readFile(root+'internal/i18n/locales/en.json','utf8'));
 const keys=Object.keys(en).filter(k=>k.startsWith('ui.node.discovery.'));
 assert.ok(keys.length>=9,'dedicated translated discovery labels');
 for(const file of catalogs){
  lang=file.slice(0,-5);const cat=JSON.parse(await readFile(root+'internal/i18n/locales/'+file,'utf8'));
  for(const key of keys){assert.equal(typeof cat[key],'string',lang+' '+key);assert.deepEqual(cat[key].match(/\{\w+\}/g)||[],en[key].match(/\{\w+\}/g)||[]);}
  await page.setViewportSize({width:390,height:844});await page.reload();await panel.waitFor();
  assert.equal(await panel.locator('[data-known-members]').innerText(),'2');
  assert.equal(await panel.evaluate(e=>e.scrollWidth<=e.clientWidth),true,lang+' panel must wrap');
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true,lang+' page must not overflow');
  assert.doesNotMatch(await panel.innerText(),/ui\.node\.discovery\./);
 }
 console.log('PASS all 15 translated catalogs and mobile wrapping');
 lang='en';agent={member:{server:'https://controller:8077',node:'east',fingerprint:'f'.repeat(64),joined:snapshot.last_refresh},status:{connected:false,last_error:'Connection refused',monitors:0}};
 await page.reload();await panel.waitFor();
 assert.equal(await page.getByRole('button',{name:'Leave cluster',exact:true}).count(),1,'legacy agent membership still shown');
 assert.match(await page.locator('#app').innerText(),/Connection refused/);
 await page.getByText("Copy of the master's state",{exact:true}).waitFor();
 assert.equal(await panel.getByRole('button').count(),0,'registry does not expose zone-switch or takeover actions');
 assert.deepEqual(writes,[]);assert.deepEqual(errors,[]);
 console.log('PASS agent membership compatibility, no writes or browser errors');
} finally {await browser.close();await new Promise(r=>server.close(r));}
