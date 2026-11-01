// Rebuild and photograph the real local application, never a mocked API or edited DOM.
// node scripts/capture-readme.mjs PATH_TO_GO SCRATCH_DIRECTORY
import {chromium} from 'playwright';
import {spawn, execFileSync} from 'node:child_process';
import {mkdir,writeFile,mkdtemp,readFile,rm,stat} from 'node:fs/promises';
import {createServer} from 'node:http';
import {randomBytes,createHash} from 'node:crypto';
import assert from 'node:assert/strict';
import path from 'node:path';
const [go,scratch]=process.argv.slice(2);assert.ok(go&&scratch,'Go executable and disposable scratch directory required');
const out=path.resolve('docs/images/current');await mkdir(out,{recursive:true});
const data=await mkdtemp(path.join(path.resolve(scratch),'readme-capture-'));
const binary=path.join(data,process.platform==='win32'?'unagikuma.exe':'unagikuma');
const git=(...args)=>execFileSync('git',args,{encoding:'utf8'}).trim();
const revision=git('rev-parse','HEAD');const statusBefore=git('status','--porcelain');
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
const diffBefore=hash(execFileSync('git',['diff','HEAD','--','internal','web','cmd','go.mod','go.sum']));
const buildArgs=['build','-p','1','-trimpath','-o',binary,'./cmd/unagikuma'];
execFileSync(go,buildArgs,{env:{...process.env,CGO_ENABLED:'0'},stdio:'pipe'});
const diffAfter=hash(execFileSync('git',['diff','HEAD','--','internal','web','cmd','go.mod','go.sum']));
assert.equal(diffBefore,diffAfter,'Tracked product source changed during build; rerun after integration stabilizes');
const binaryHash=hash(await readFile(binary));
const target=createServer((q,r)=>{if(q.url==='/unavailable'){r.statusCode=503;r.end('Illustrative demo outage');}else r.end('Illustrative demo healthy');});
await new Promise(r=>target.listen(0,'127.0.0.1',r));const targetPort=target.address().port;
const reserve=createServer();await new Promise(r=>reserve.listen(0,'127.0.0.1',r));const port=reserve.address().port;await new Promise(r=>reserve.close(r));
const day=new Date(Date.now()-7*86400000).toISOString().slice(0,10);
await writeFile(path.join(data,'monitors.bind'),`@ IN SOA readme-demo. admin.localhost. ( 2026103101 2s 500ms 500ms 1 )
$SET engine.first-round-delay=0s localnet.enabled=off notify.daily-report=off
alice-demo IN CHANNEL log
bob-demo IN CHANNEL log
casey-demo IN CHANNEL log
ops-demo IN CHANNEL log
primary-demo IN CHANNEL group members=alice-demo,bob-demo,casey-demo rotate=daily handoff=09:00 start=${day} tz=UTC
backup-demo IN CHANNEL group members=casey-demo,bob-demo rotate=weekly handoff="mon 09:00" start=${day} tz=UTC
default IN POLICY chain=primary-demo,ops-demo
web-demo IN HTTP http://127.0.0.1:${targetPort}/ title="Demo website" device=demo-web
api-demo IN HTTP http://127.0.0.1:${targetPort}/api title="Demo API" device=demo-app
checkout-demo IN HTTP http://127.0.0.1:${targetPort}/unavailable title="Demo checkout (intentional 503)" device=demo-app
heartbeat-demo IN PUSH period=1h title="Demo backup heartbeat (waiting)"
`);
const proc=spawn(binary,['server','--data',data,'--listen',`127.0.0.1:${port}`],{stdio:['ignore','pipe','pipe']});
let serverLog='';proc.stdout.on('data',x=>serverLog+=x);proc.stderr.on('data',x=>serverLog+=x);
const browser=await chromium.launch();const errors=[],images=[];const base=`http://127.0.0.1:${port}`;
try{
 let ready=false;for(let i=0;i<160;i++){assert.equal(proc.exitCode,null,'demo server exited: '+serverLog.replace(/.*(?:token|secret|password).*$/gim,'[redacted]'));try{if((await fetch(base+'/healthz')).ok){ready=true;break;}}catch{}await new Promise(r=>setTimeout(r,250));}assert.ok(ready,'server readiness');
 const context=await browser.newContext({viewport:{width:1440,height:1000},locale:'en-US',timezoneId:'Asia/Tokyo'});
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
 await page.goto(base);const password=randomBytes(24).toString('hex');await page.fill('input[autocomplete=username]','readme-demo');await page.locator('input[autocomplete=new-password]').nth(0).fill(password);await page.locator('input[autocomplete=new-password]').nth(1).fill(password);await page.click('.auth .btn.primary');await page.waitForSelector('nav.nav');
 const api=p=>page.evaluate(async p=>(await fetch('/api/v1'+p)).json(),p);
 let report;for(let i=0;i<120;i++){report=await api('/reports/daily');if(report.snapshot?.issues_total===1)break;await page.waitForTimeout(250);}
 assert.equal(report.snapshot.issues_total,1);assert.equal(report.snapshot.services.total,4);assert.equal(report.snapshot.semantics,'current_snapshot');
 async function capture(name,route,theme,selector,width=1440,height=1000){
  await page.setViewportSize({width,height});await page.selectOption('select:has(option[value="dark"])',theme,{force:true});await page.goto(base+'/'+route);await page.waitForSelector(selector);
  if(name.startsWith('dashboard') && await page.getByRole('button',{name:'Hide',exact:true}).count()) await page.getByRole('button',{name:'Hide',exact:true}).click();
  if(name.startsWith('oncall')) await page.getByRole('button',{name:'Week',exact:true}).click();
  await page.waitForTimeout(500);await page.evaluate(()=>document.fonts.ready);
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,name+' horizontal overflow');
  const file=name+'.png';await page.screenshot({path:path.join(out,file),fullPage:!name.startsWith('dashboard'),animations:'disabled'});const png=await readFile(path.join(out,file));images.push({file,route,theme,viewport:{width,height},dimensions:{width:png.readUInt32BE(16),height:png.readUInt32BE(20)},bytes:png.length});
 }
 await capture('dashboard-midnight','#/','midnight','.card');
 await capture('dashboard-paper','#/','paper','.card');
 await capture('alerts-midnight','#/alerts/channels','midnight','.tbl');
 await capture('oncall-paper','#/oncall','paper','.oc-cal');
 await capture('daily-report-midnight','#/notices/report','midnight','.report-totals');
 await capture('mobile-paper','#/notices/report','paper','.report-totals',390,844);
 assert.deepEqual(errors,[]);
 await writeFile(path.join(out,'capture-manifest.json'),JSON.stringify({capturedAt:new Date().toISOString(),sourceRevision:revision,sourceDirty:!!statusBefore,sourceNote:'Unreleased current working-tree UI; not a frozen final-release build. Zone integration ongoing.',trackedProductDiffSha256:diffBefore,trackedProductUnchangedDuringBuild:diffBefore===diffAfter,buildInvocation:'CGO_ENABLED=0 go build -p 1 -trimpath -o <disposable-scratch>/unagikuma.exe ./cmd/unagikuma',versionOverride:false,binarySha256:binaryHash,runtimeURLType:'loopback HTTP (127.0.0.1, ephemeral port)',data:'Disposable illustrative demo; real loopback HTTP probes, intentional 503 and passive PUSH; no production data',notifications:'Local log channels only; automatic daily report disabled',reportVerified:{services:report.snapshot.services.total,issues:report.snapshot.issues_total,semantics:report.snapshot.semantics},images,errors},null,2)+'\n');
 console.log(JSON.stringify({ok:true,images:images.length,out,errors}));
}finally{
 await browser.close();proc.kill();await new Promise(r=>proc.exitCode!==null?r():proc.once('exit',r));await new Promise(r=>target.close(r));await rm(data,{recursive:true,force:true});
}
