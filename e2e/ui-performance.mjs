// Local-only Chromium fixtures. Run --baseline to measure before editing, --test
// for deterministic contracts, or without flags for paired repeated benchmarks.
// Run --snapshot BEFORE editing (refuses to replace an existing snapshot).
import { chromium } from 'playwright';
import { createServer } from 'node:http';
import { readFile, mkdir, writeFile, access, cp } from 'node:fs/promises';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
const root = new URL('../', import.meta.url);
const snapshot = new URL('test-results/ui-performance-baseline/ui/', root);
const live = new URL('web/ui/', root);
const hasSnapshot = await access(new URL('js/views/alerts.js',snapshot)).then(()=>true,()=>false);
if(process.argv.includes('--snapshot')){
  assert.equal(hasSnapshot,false,'refusing to overwrite pre-change source evidence');
  await cp(live,snapshot,{recursive:true,errorOnExist:true,force:false});
  console.log('Saved pre-change UI snapshot');process.exit(0);
}
if(!hasSnapshot && !process.argv.includes('--test')) throw new Error('Missing pre-change snapshot: run --snapshot before changing production source');
const shared = hasSnapshot?snapshot:live;
const fixed = '2026-10-30T03:00:00Z';
const channels = n => Array.from({length:n}, (_,i) => ({name:`channel-${String(i).padStart(4,'0')}`,kind:'log',opts:{},used_by:i%2?['production']:[],issues:[],etag:'v1'}));
const shifts = Array.from({length:44},(_,i)=>({member:i%2?'bob':'alice',from:new Date(Date.UTC(2026,8,27+i,9)).toISOString(),until:new Date(Date.UTC(2026,8,28+i,9)).toISOString()}));
const rotations = n => Array.from({length:n},(_,i)=>({name:`rotation-${i}`,rotate:'daily',handoff:'09:00',tz:'UTC',now:'alice',scheduled:shifts[32],members:[{channel:'alice',people:['Alice']},{channel:'bob',people:['Bob']}],policies:['production'],overrides:[],shifts}));
let active = {variant:'before',view:'channels',size:100};
const server = createServer(async(req,res)=>{
  const path = new URL(req.url,'http://localhost').pathname;
  const json = data => {res.setHeader('Content-Type','application/json');res.end(JSON.stringify(data));};
  if(path==='/api/v1/channels') return json(channels(active.view==='channels'?active.size:100));
  if(path==='/api/v1/channel-kinds') return json([{kind:'log',label:'Log',fields:[]}]);
  if(path==='/api/v1/oncall') return json({rotations:rotations(active.size),stats:{from:fixed,to:fixed,all:{incidents:0,acked:0,resolved:0},members:[]}});
  if(path==='/api/v1/oncall/calendar') return json({rotations:rotations(active.size).map(r=>({name:r.name,segments:shifts}))});
  if(path.startsWith('/api/')) return json({});
  if(path==='/') {res.setHeader('Content-Type','text/html');return res.end(`<link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/channels.css"><link rel="stylesheet" href="/oncall.css"><div id="app"></div><script type="module">import {html,render} from '/js/lib.js';import {${active.view==='channels'?'Alerts':'OnCall'}} from '/js/views/${active.view==='channels'?'alerts':'oncall'}.js';render(html\`<\${${active.view==='channels'?'Alerts':'OnCall'}} tab="channels" />\`,document.querySelector('#app'));</script>`);}
  try {res.setHeader('Content-Type',path.endsWith('.js')?'text/javascript':'text/css');
    // Freeze unrelated shared files/styles across variants: only owned modules differ.
    const dir=active.variant==='after' && ['/js/views/alerts.js','/js/views/oncall.js'].includes(path)?live:shared;
    res.end(await readFile(new URL('.'+path,dir)));
  }catch{res.statusCode=404;res.end();}
});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
const base=`http://127.0.0.1:${server.address().port}`;
const browser=await chromium.launch({headless:true});
async function setup(variant,view,size,profile=false){
  active={variant,view,size};
  const page=await browser.newPage({viewport:{width:1100,height:900},timezoneId:'UTC'});
  page.setDefaultTimeout(15000);
  const errors=[]; page.on('pageerror',e=>errors.push(e.message));
  await page.route('**/*',route=>new URL(route.request().url()).origin===base?route.continue():route.abort());
  await page.addInitScript(({fixed,profile})=>{
    const OriginalDate=Date; window.Date=class extends OriginalDate {constructor(...args){super(...(args.length?args:[fixed]));} static now(){return +new OriginalDate(fixed);}};
    window.formatProfile={calls:0,elapsedMs:0};
    if(profile) for(const key of ['toLocaleDateString','toLocaleTimeString']){
      const original=OriginalDate.prototype[key];OriginalDate.prototype[key]=function(...args){const start=performance.now();try{return original.apply(this,args);}finally{window.formatProfile.calls++;window.formatProfile.elapsedMs+=performance.now()-start;}};
    }
  },{fixed,profile});
  return {page,errors};
}
const paint=page=>page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))));
async function ready(page,view,size){
  await page.waitForFunction(({view,size})=>view==='channels'?document.querySelectorAll('.channels-table tbody tr').length>0:document.querySelectorAll('.oc-rot').length===size && document.querySelectorAll('.oc-seg').length>size,{view,size});
  await paint(page);
}
async function sample(variant,view,size,profile=false){
  const {page,errors}=await setup(variant,view,size,profile);
  try{
    const start=performance.now();await page.goto(base);await ready(page,view,size);
    const loadMs=performance.now()-start;
    const cdp=await page.context().newCDPSession(page);
    await cdp.send('HeapProfiler.collectGarbage');
    const heap=await cdp.send('Runtime.getHeapUsage');
    const dom=await page.evaluate(()=>{const walker=document.createTreeWalker(document,NodeFilter.SHOW_ALL);let nodes=1;while(walker.nextNode()) nodes++;return {nodes,elements:document.querySelectorAll('*').length,rows:document.querySelectorAll('tbody tr').length,profile:window.formatProfile};});
    let taskMs;
    if(view==='channels'){
      taskMs=await page.evaluate(async()=>{const input=document.querySelector('input[type=search]');const start=performance.now();input.value='channel-00';input.dispatchEvent(new Event('input',{bubbles:true}));await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return performance.now()-start;});
      await page.getByRole('searchbox').fill(`channel-${String(size-1).padStart(4,'0')}`);
      await page.locator('tbody tr').filter({hasText:`channel-${String(size-1).padStart(4,'0')}`}).waitFor();
      assert.equal(await page.locator('tbody tr').count(),1,'search includes final item, beyond first page');
    }else{
      taskMs=await page.evaluate(async()=>{const start=performance.now();[...document.querySelectorAll('.oc-bar button')].find(b=>b.textContent==='ui.oc.week').click();await new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)));return performance.now()-start;});
      await page.waitForFunction(size=>document.querySelectorAll('.oc-day').length===size*7,size);
    }
    assert.deepEqual(errors,[]);
    return {loadMs,taskMs,domNodes:dom.nodes,domElements:dom.elements,tableRows:dom.rows,heapUsedBytes:heap.usedSize,formatterProfile:dom.profile};
  }finally{await page.close();}
}
async function contracts(){
  for(const size of [2,100,2000]){
    const {page,errors}=await setup('after','channels',size);
    try{
      await page.goto(base);await ready(page,'channels',size);
      assert.equal(await page.locator('tbody tr').count(),Math.min(size,100),'large channel lists must render at most 100 rows');
      assert.ok(await page.evaluate(()=>document.querySelectorAll('*').length)<2000,'fixture DOM stays bounded independent of channel count');
      const nav=page.getByRole('navigation',{name:'ui.ch.tab'});
      assert.equal(await nav.count(),size>100?1:0,'small lists have no pagination friction');
      if(size>100){
        const next=nav.getByRole('button',{name:'ui.oc.next',exact:true});
        const prev=nav.getByRole('button',{name:'ui.oc.prev',exact:true});
        assert.equal(await prev.isDisabled(),true);
        await next.focus();await page.keyboard.press('Enter');
        await page.getByText('channel-0100',{exact:true}).waitFor();
        assert.equal(await next.evaluate(e=>e===document.activeElement),true,'keyboard focus survives paging');
        await prev.click();await page.getByText('channel-0000',{exact:true}).waitFor();
        for(let i=1;i<20;i++) await next.click();
        assert.equal(await next.isDisabled(),true,'last page bounds');
        await page.getByRole('searchbox').fill('channel-0000');
        await page.getByText('channel-0000',{exact:true}).waitFor();
        assert.equal(await page.locator('tbody tr').count(),1);
        await page.getByRole('searchbox').fill('');
        await page.getByText('channel-0000',{exact:true}).waitFor();
        await page.getByRole('searchbox').fill('channel-1999');
        await page.getByText('channel-1999',{exact:true}).waitFor();
        assert.equal(await page.locator('tbody tr').count(),1);
      }
      assert.deepEqual(errors,[]);
    }finally{await page.close();}
  }
  console.log('PASS bounded channel DOM, complete search, page limits, keyboard navigation, small-list simplicity');
  const displays=[];
  for(const variant of hasSnapshot?['before','after']:['after']){
    const {page,errors}=await setup(variant,'oncall',2,true);
    try{
      await page.goto(base);await ready(page,'oncall',2);
      displays.push(await page.locator('.oc-rot').evaluateAll(els=>els.map(e=>({text:e.innerText,titles:[...e.querySelectorAll('.oc-seg')].map(b=>b.title)}))));
      if(variant==='after') assert.ok((await page.evaluate(()=>window.formatProfile.calls))<50,'calendar must reuse formatters instead of thousands of locale conversions');
      for(const [lang,mode] of [['ja','week'],['en','month']]){
        await page.evaluate(async lang=>{const {setLang}=await import('/js/i18n.js');await setLang(lang);},lang);
        await page.getByRole('button',{name:'ui.oc.'+mode,exact:true}).click();
        await page.waitForFunction(days=>document.querySelectorAll('.oc-day').length===days,mode==='week'?14:84);
        const date=page.locator('.oc-handoff time').first();
        assert.equal(await date.innerText(),await date.evaluate((e,lang)=>{const d=new Date(e.dateTime);return d.toLocaleDateString(lang,{month:'short',day:'numeric',weekday:'short'})+' '+d.toLocaleTimeString(lang,{hour:'2-digit',minute:'2-digit'});},lang),'language changes invalidate the formatter cache');
      }
      assert.deepEqual(errors,[]);
    }finally{await page.close();}
  }
  if(hasSnapshot) assert.deepEqual(displays[1],displays[0],'formatter reuse preserves exact date text and shift titles');
  console.log('PASS oncall formatter reuse' + (hasSnapshot?' and identical visible calendar/tooltip dates':' (baseline text comparison skipped: no snapshot)'));
}
const median=values=>[...values].sort((a,b)=>a-b)[Math.floor(values.length/2)];
try{
  if(process.argv.includes('--test')) await contracts();
  else{
    const cases=[['channels',100],['channels',2000],['oncall',2],['oncall',12]];
    const variants=process.argv.includes('--baseline')?['before']:['before','after'];
    const results=[];
    for(const [view,size] of cases){
      const records=Object.fromEntries(variants.map(v=>[v,[]]));
      for(const variant of variants) await sample(variant,view,size); // discarded warmup
      for(let i=0;i<5;i++) for(const variant of i%2?[...variants].reverse():variants) records[variant].push(await sample(variant,view,size));
      const profiles={};for(const variant of variants) profiles[variant]=(await sample(variant,view,size,true)).formatterProfile;
      const summaries=Object.fromEntries(variants.map(v=>[v,Object.fromEntries(['loadMs','taskMs','domNodes','domElements','tableRows','heapUsedBytes'].map(k=>[k,median(records[v].map(r=>r[k]))]))]));
      results.push({view,size,summaries,samples:records,formatterDiagnostic:profiles});
      console.log(JSON.stringify({view,size,summaries,profiles}));
    }
    const hashes={};for(const variant of variants){hashes[variant]={};for(const file of ['alerts.js','oncall.js'])hashes[variant][file]=createHash('sha256').update(await readFile(new URL('js/views/'+file,variant==='before'?snapshot:live))).digest('hex');}
    const evidence={browser:browser.version(),fixedDate:fixed,samplesPerCase:5,hashes,method:'Fresh browser contexts; discarded warmup; alternating variants; shared assets frozen to pre-change snapshot. Load = host wall-clock navigation through ready + two frames. Task = browser elapsed event through two frames (channels broad search / oncall month-to-week). Heap = CDP used JS heap after forced GC, not process memory. Formatter diagnostic separately instrumented, excluded from timings. No timing thresholds; all requests loopback only.',results};
    await mkdir(new URL('test-results/',root),{recursive:true});
    await writeFile(new URL(process.argv.includes('--baseline')?'test-results/ui-performance-before.json':'test-results/ui-performance.json',root),JSON.stringify(evidence,null,2));
  }
}finally{await browser.close();await new Promise(r=>server.close(r));}
