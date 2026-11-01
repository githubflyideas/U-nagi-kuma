// Chromium component contract tests. API responses are isolated fixtures, not a real backend.
import {test,before,after} from 'node:test';
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {readFile,mkdir,writeFile} from 'node:fs/promises';
import {chromium} from 'playwright';
const root=new URL('../web/ui/',import.meta.url);
const out=new URL('../test-results/report-ui/',import.meta.url);
let browser,server,base;
export const report={title:'Daily status',body:'Current snapshot · not a 24-hour history\n2026-10-30 09:00:00 +00:00 · UTC\nNeeds attention\n• Database: CRITICAL — Connection refused',policy:'primary',ok:false,enabled:true,at:'10:00',timezone:'UTC',generated_at:'2026-10-30T09:00:00Z',snapshot:{generated_at:'2026-10-30T09:00:00Z',timezone:'UTC',semantics:'current_snapshot',state:'attention',hosts:{total:4,states:{up:2,down:1,pending:1,paused:0,maintenance:0,unreachable:0}},services:{total:6,states:{ok:2,critical:1,degraded:1,pending:1,paused:1,maintenance:0,unknown:0,unreachable:0}},issues:[{name:'database',title:'Database',status:'critical',since:'2026-10-30T08:00:00Z',acked:true,acked_by:'Operations',detail:'Connection refused'},{name:'web/api',title:'Public API',status:'degraded',detail:'Response exceeded threshold',acked:false}],issues_total:3,issues_omitted:1}};
before(async()=>{
 await mkdir(out,{recursive:true});
 server=createServer(async(req,res)=>{try{
 if(req.url==='/'){res.setHeader('Content-Type','text/html');res.end(`<meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/themes.css"><link rel="stylesheet" href="/console-polish.css"><link rel="stylesheet" href="/report.css"><div id="app"></div><script type="module">import{html,render}from'/js/lib.js';import{Notices}from'/js/views/announce.js';import{setLang}from'/js/i18n.js';await setLang('en');render(html\`<\${Notices} tab="report"/>\`,document.querySelector('#app'));</script>`);return;}
 res.setHeader('Content-Type',req.url.endsWith('.css')?'text/css':'text/javascript');res.end(await readFile(new URL('.'+req.url,root)));
 }catch{res.statusCode=404;res.end();}});
 await new Promise(r=>server.listen(0,'127.0.0.1',r));base=`http://127.0.0.1:${server.address().port}`;browser=await chromium.launch();
});
after(async()=>{await browser?.close();await new Promise(r=>server.close(r));});
async function setup(handler=()=>undefined){
 const page=await browser.newPage();page.setDefaultTimeout(2500);const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.route('**/api/**',async route=>{const p=new URL(route.request().url()).pathname.replace('/api/v1','');
 const custom=await handler(p);const result=custom||{body:p==='/i18n/en'?JSON.parse(await readFile(new URL('../internal/i18n/locales/en.json',import.meta.url),'utf8')):report};
 await route.fulfill({status:result.status||200,contentType:'application/json',body:JSON.stringify(result.body)});
 });await page.goto(base);return{page,errors};
}
test('structured snapshot, issue links and collapsed delivery preview',async()=>{
 const{page,errors}=await setup();try{
 await page.locator('.report-view').waitFor();
 assert.match(await page.locator('.report-view').innerText(),/not a 24-hour history/);
 assert.doesNotMatch(await page.locator('.report-totals').innerText(),/status\./);
 await page.locator('a[href="#/monitors/web%2Fapi"]').waitFor();
 assert.equal(await page.locator('.report-preview').getAttribute('open'),null);
 await page.locator('.report-preview summary').click();assert.match(await page.locator('.report-preview pre').innerText(),/Connection refused/);
 assert.deepEqual(errors,[]);
 }finally{await page.close();}
});
test('send guards synchronous double click and shows inline failure, retry and queued outcome',async()=>{
 let calls=0,release;let gate=new Promise(r=>release=r);
 const{page}=await setup(async p=>{if(p==='/reports/daily/send'){calls++;await gate;return calls===1?{status:503,body:{message:'Queue unavailable'}}:{body:{...report,delivery:'queued'}};}});
 try{
 const send=page.getByRole('button',{name:'Send now',exact:true});await send.evaluate(e=>{e.click();e.click();});
 await page.waitForTimeout(120);assert.equal(calls,1);assert.equal(await send.isDisabled(),true);release();
 await page.getByRole('alert').waitFor();await page.getByRole('button',{name:'Retry',exact:true}).click();
 await page.getByRole('status').getByText(/Queued for sending/).waitFor();assert.equal(calls,2);
 }finally{release();await page.close();}
});
test('send rejects unaccepted results, clears old success and permits retry',async()=>{
 for(const delivery of ['rejected',undefined,'delivered']){
 let calls=0;const{page,errors}=await setup(p=>p==='/reports/daily/send'?{body:{...report,ok:true,delivery:++calls===2?delivery:'queued'}}:undefined);
 try{
 const send=page.getByRole('button',{name:'Send now',exact:true});
 await send.click();await page.locator('.report-feedback').waitFor();
 await send.click();await page.getByRole('alert').waitFor();
 assert.equal(await page.locator('.report-feedback').count(),0);
 assert.match(await page.getByRole('alert').innerText(),/not accepted.*queue/i);
 assert.equal(await send.isDisabled(),false);
 await page.getByRole('button',{name:'Retry',exact:true}).click();
 await page.locator('.report-feedback').waitFor();assert.equal(calls,3);
 assert.equal(await page.getByRole('alert').count(),0);assert.deepEqual(errors,[]);
 }finally{await page.close();}
 }
});
test('refresh failure remains visible beside retained snapshot',async()=>{
 let calls=0;const{page}=await setup(p=>p==='/reports/daily'&&++calls>1?{status:503,body:{message:'Offline'}}:undefined);
 try{await page.locator('.report-view').waitFor();await page.getByRole('button',{name:'Refresh',exact:true}).click();await page.getByText(/Showing the previous snapshot/).waitFor();await page.getByRole('link',{name:'Database',exact:true}).waitFor();}finally{await page.close();}
});
test('server-local timestamps retain their explicit offset',async()=>{
 const{page}=await setup(p=>p==='/reports/daily'?{body:{...report,timezone:'Local',generated_at:'2026-10-30T18:00:00+09:00'}}:undefined);
 try{await page.locator('.report-caption time').waitFor();assert.match(await page.locator('.report-caption time').innerText(),/18:00:00.*\+09:00/);}finally{await page.close();}
});
test('empty and incomplete are not presented as healthy',async()=>{
 for(const state of ['empty','incomplete']){const{page}=await setup(p=>p==='/reports/daily'?{body:{...report,ok:true,snapshot:{...report.snapshot,state,issues:[],issues_total:0,issues_omitted:0,services:{total:state==='empty'?0:2,states:{paused:1,pending:1,ok:0}}}}}:undefined);
 try{await page.locator('.report-view').waitFor();assert.doesNotMatch(await page.locator('.report-overview').innerText(),/All services healthy/);if(state==='empty')await page.getByRole('link',{name:'Monitors',exact:true}).waitFor();}finally{await page.close();}}
});
test('initial fetch failure has a visible retry and recovers',async()=>{
 let fail=true;const{page}=await setup(p=>p==='/reports/daily'&&fail?{status:503,body:{message:'Report unavailable'}}:undefined);
 try{await page.getByRole('button',{name:'Retry',exact:true}).waitFor();assert.equal(await page.locator('.report-overview').count(),0);fail=false;await page.getByRole('button',{name:'Retry',exact:true}).click();await page.locator('.report-overview').waitFor();}finally{await page.close();}
});
test('long issue names and details wrap at 320px; preview supports keyboard',async()=>{
 const value={...report,snapshot:{...report.snapshot,issues:[{...report.snapshot.issues[0],title:'監視'.repeat(150),detail:'x'.repeat(1000)}]}};
 const{page,errors}=await setup(p=>p==='/reports/daily'?{body:value}:undefined);
 try{await page.setViewportSize({width:320,height:844});await page.locator('.report-overview').waitFor();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await page.locator('.report-preview summary').focus();await page.keyboard.press('Enter');assert.notEqual(await page.locator('.report-preview').getAttribute('open'),null);assert.deepEqual(errors,[]);}finally{await page.close();}
});
test('theme and responsive screenshots: 320, 390, 1440',async()=>{
 const{page,errors}=await setup();const evidence=[];try{await page.locator('.report-view').waitFor();for(const theme of ['midnight','paper'])for(const width of [320,390,1440]){
 await page.setViewportSize({width,height:900});await page.evaluate(v=>document.documentElement.dataset.theme=v,theme);
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,`${theme} ${width} overflow`);
 if(width===1440)assert.ok((await page.locator('.report-view').boundingBox()).width>=1000,'desktop overview uses available space');
 const name=`${theme}-${width}.png`;await page.screenshot({path:new URL(name,out).pathname.replace(/^\/([A-Z]:)/,'$1'),fullPage:true});evidence.push(name);
 }assert.deepEqual(errors,[]);await writeFile(new URL('verification.json',out),JSON.stringify({backend:'isolated API fixtures',evidence,errors},null,2));}finally{await page.close();}
});
