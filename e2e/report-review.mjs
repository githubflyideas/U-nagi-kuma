// Real executable, real API, local-only monitors and log channel. No external notifications.
// node e2e/report-review.mjs BINARY OUTPUT_DIR [before]
import {chromium} from 'playwright';
import {spawn} from 'node:child_process';
import {mkdir,writeFile,mkdtemp,readFile} from 'node:fs/promises';
import {createServer} from 'node:http';
import {randomBytes} from 'node:crypto';
import assert from 'node:assert/strict';
import path from 'node:path';
const[binary,dest,before]=process.argv.slice(2);assert.ok(binary&&dest);
const out=path.resolve(dest);await mkdir(out,{recursive:true});const data=await mkdtemp(path.join(out,'isolated-data-'));
const target=createServer((q,r)=>{r.end('local fixture healthy');});await new Promise(r=>target.listen(0,'127.0.0.1',r));const targetPort=target.address().port;
const reserve=createServer();await new Promise(r=>reserve.listen(0,'127.0.0.1',r));const port=reserve.address().port;await new Promise(r=>reserve.close(r));
await writeFile(path.join(data,'monitors.bind'),`@ IN SOA report-review. admin.localhost. ( 2026103001 1s 500ms 500ms 1 )
$SET engine.first-round-delay=0s localnet.enabled=off notify.daily-report=off notify.daily-report-max-items=5
ops-log IN CHANNEL log
default IN POLICY chain=ops-log
api IN HTTP http://127.0.0.1:${targetPort}/ title="Public API" device=app-01
payments IN TCP 127.0.0.1:1 title="Payments database" device=db-01
search IN TCP 127.0.0.1:2 title="Search index" device=search-01
cache IN TCP 127.0.0.1:3 title="Cache replica" device=cache-01
jobs IN TCP 127.0.0.1:4 title="Job processor" device=jobs-01
mail IN TCP 127.0.0.1:5 title="Mail relay" device=mail-01
storage IN TCP 127.0.0.1:6 title="Storage replica" device=storage-01
backup IN PUSH period=1h title="Backup heartbeat"
`);
const proc=spawn(path.resolve(binary),['server','--data',data,'--listen',`127.0.0.1:${port}`],{stdio:['ignore','pipe','pipe']});let log='';proc.stdout.on('data',x=>log+=x);proc.stderr.on('data',x=>log+=x);
const browser=await chromium.launch();const errors=[],evidence=[];const base=`http://127.0.0.1:${port}`;
try{
 let ready=false;for(let i=0;i<120;i++){if(proc.exitCode!==null)throw Error('fixture exited: '+log.replace(/token[^\n]*/gi,'token [redacted]'));try{if((await fetch(base+'/healthz')).ok){ready=true;break;}}catch{}await new Promise(r=>setTimeout(r,250));}assert.ok(ready);
 const ctx=await browser.newContext({viewport:{width:1440,height:1000},locale:'en-US',timezoneId:'Asia/Tokyo'});const page=await ctx.newPage();page.on('pageerror',e=>errors.push(e.message));
 await page.goto(base);const password=randomBytes(20).toString('hex');await page.fill('input[autocomplete=username]','report-review');await page.locator('input[autocomplete=new-password]').nth(0).fill(password);await page.locator('input[autocomplete=new-password]').nth(1).fill(password);await page.click('.auth .btn.primary');await page.waitForSelector('nav.nav');
 const api=p=>page.evaluate(async p=>(await fetch('/api/v1'+p)).json(),p);
 let r;for(let i=0;i<80;i++){r=await api('/reports/daily');if(before?!r.ok:r.snapshot?.issues_total===6)break;await page.waitForTimeout(250);}
 if(!before){assert.equal(r.snapshot.issues_total,6);assert.equal(r.snapshot.services.total,8);assert.equal(r.snapshot.issues.length,5);assert.equal(r.snapshot.issues_omitted,1);assert.equal(r.snapshot.services.states.pending,1);assert.equal(r.snapshot.semantics,'current_snapshot');}
 await writeFile(path.join(out,'api-report.json'),JSON.stringify(r,null,2));
 for(const lang of ['en','zh']){
 await page.selectOption('select:has(option[value="en"])',lang,{force:true});
 for(const theme of ['midnight','paper'])for(const width of [1440,390,320]){
 await page.setViewportSize({width,height:width===1440?1000:844});await page.selectOption('select:has(option[value="dark"])',theme,{force:true});await page.goto(base+'/#/notices/report');await page.waitForSelector(before?'pre.report':'.report-totals');
 // Parent owns index.html; inject only if its stylesheet link has not landed yet.
 if(!before&&!await page.locator('link[href="/report.css"]').count())await page.addStyleTag({url:base+'/report.css'});
 const name=`report-${lang}-${theme}-${width}.png`;await page.screenshot({path:path.join(out,name),fullPage:true,animations:'disabled'});evidence.push(name);
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,`${name} overflow`);
 }}
 if(!before){await page.selectOption('select:has(option[value="en"])','en',{force:true});const response=page.waitForResponse(r=>r.url().endsWith('/api/v1/reports/daily/send')&&r.request().method()==='POST');await page.getByRole('button',{name:'Send now',exact:true}).click();const sent=await (await response).json();assert.equal(sent.delivery,'queued');assert.equal(sent.snapshot.semantics,'current_snapshot');await writeFile(path.join(out,'api-send.json'),JSON.stringify(sent,null,2));await page.getByRole('status').getByText(/Queued for sending/).waitFor();}
 assert.deepEqual(errors,[]);await writeFile(path.join(out,'verification.json'),JSON.stringify({backend:'real isolated local executable',channels:'log only',monitors:'loopback HTTP/TCP and passive PUSH only',phase:before?'before':'after',apiVerified:!before,evidence,errors},null,2));console.log(JSON.stringify({ok:true,out,evidence}));
}finally{await browser.close();proc.kill();await new Promise(r=>target.close(r));}
