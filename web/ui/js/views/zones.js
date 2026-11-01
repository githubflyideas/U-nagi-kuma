// Zone drafts and applied runtime are separate; binding is not network sync.
import { html, useState, useRef } from '../lib.js';
import { get, post, ifMatch, newKey } from '../api.js';
import { t } from '../i18n.js';
import { useFetch, ErrorBox } from '../ui.js';

export function Zones() {
  const session = useFetch(() => get('/session'), []);
  if (session.error) return html`<${ErrorBox} error=${session.error} retry=${session.reload} />`;
  if (!session.data) return null;
  if (session.data.role !== 'admin' || session.data.areas?.length) return html`<div class="page" data-testid="zone-forbidden">${t('ui.zone.admin')}</div>`;
  return html`<${ZoneManager} />`;
}
function ZoneManager() {
  const list = useFetch(() => get('/zones'), []);
  const [form, setForm] = useState({name:'',text:''});
  const [preview, setPreview] = useState(null);
  const [error, setError] = useState(null);
  const [result, setResult] = useState('');
  const [exported, setExported] = useState(null);
  const [binding, setBinding] = useState(null);
  const [busy, setBusy] = useState(false);
  const lock = useRef(false);
  // Retain one idempotency key across an uncertain response, not across edits.
  const importKey = useRef(newKey());
  const change = key => e => {setForm({...form,[key]:e.target.value});setPreview(null);setResult('');importKey.current=newKey();};
  const run = async fn => {
    if (lock.current) return;
    lock.current=true;setBusy(true);setError(null);setResult('');
    try {await fn();} catch(e) {
      setError({conflict:e.status===412 || e.status===428,duplicate:e.status===409,missing:Array.isArray(e.body?.missing)?e.body.missing:[]});
      if(e.status===412 || e.status===409) list.reload();
    } finally {lock.current=false;setBusy(false);}
  };
  const importDraft=()=>run(async()=>{await post('/zones/import',form,{'Idempotency-Key':importKey.current});list.reload();setResult(t('ui.zone.imported'));});
  const apply=z=>run(async()=>{await post('/zones/'+encodeURIComponent(z.id)+'/apply',{},ifMatch(z.revision));list.reload();setResult(t('ui.zone.applied_hint'));});
  const saveBinding=e=>{e.preventDefault();run(async()=>{
    await post('/zones/'+encodeURIComponent(binding.zone.id)+'/rebind',{source_node:binding.source_node,fingerprint:binding.fingerprint,confirm:binding.confirm},ifMatch(binding.zone.revision));
    setBinding(null);list.reload();setResult(t('ui.zone.binding_saved'));
  });};
  return html`<div class="page col" style=${{gap:'16px',minWidth:0}}>
    <h1>${t('ui.zone.title')}</h1><p class="muted">${t('ui.zone.intro')}</p>
    ${list.error && html`<${ErrorBox} error=${list.error} retry=${list.reload} />`}
    ${error && html`<div class="banner error" role="alert" data-testid="zone-error"><div>${t(error.conflict?'ui.zone.conflict':error.duplicate?'ui.zone.duplicate':'ui.zone.error')}<${Missing} refs=${error.missing} /></div></div>`}
    ${result && html`<div class="banner ok" role="status">${result}</div>`}
    <fieldset disabled=${busy} class="card" style=${{minWidth:0}}><legend>${t('ui.zone.import')}</legend>
      <div class="bd form">
        <label>${t('ui.name')}<input data-testid="zone-name" value=${form.name} onInput=${change('name')} /></label>
        <label>${t('ui.zone.text')}<textarea data-testid="zone-text" rows="9" class="mono ltr" value=${form.text} onInput=${change('text')} spellcheck="false"></textarea></label>
        <div class="row"><button class="btn" data-testid="zone-preview" onClick=${()=>run(async()=>setPreview(await post('/zones/preview',form)))}>${t('ui.zone.preview')}</button>
          <button class="btn primary" data-testid="zone-import" onClick=${importDraft}>${t('ui.zone.import')}</button></div>
        ${preview && html`<div><b>${t('ui.zone.preview')}</b><ul>${(preview.monitors||[]).map(n=>html`<li>${n}</li>`)}</ul><${Missing} refs=${preview.missing} /></div>`}
      </div>
    </fieldset>
    <div class="row"><h2 class="spacer">${t('ui.zone.title')}</h2><button class="btn" disabled=${busy} onClick=${list.reload}>${t('ui.zone.refresh')}</button></div>
    ${(list.data?.items||[]).map(z=>html`<section class="card" key=${z.id} data-zone-id=${z.id} style=${{overflowWrap:'anywhere',minWidth:0}}>
      <div class="hd"><h2>${z.name}</h2><span class="tag">${t(z.applied_revision===z.revision?'ui.zone.applied':'ui.zone.draft')}</span></div>
      <div class="bd col" style=${{gap:'10px',minWidth:0}}><div class="mono small">${z.id}</div><div>${t('ui.zone.'+z.role)} · <span>${z.source_node}</span></div>
      <p class="muted small">${t('ui.zone.no_sync')}</p>
      <div class="row"><button class="btn primary" disabled=${busy} data-testid="zone-apply" onClick=${()=>apply(z)}>${t('ui.zone.apply')}</button>
      <button class="btn" disabled=${busy} data-testid="zone-export" onClick=${()=>run(async()=>setExported(await get('/zones/'+encodeURIComponent(z.id)+'/export')))}>${t('ui.zone.export')}</button>
      <button class="btn" disabled=${busy} data-testid="zone-rebind" onClick=${()=>setBinding({zone:z,source_node:z.source_node||'',fingerprint:z.fingerprint||'',confirm:false})}>${t('ui.zone.rebind')}</button></div></div>
    </section>`)}
    ${exported && html`<section class="card"><div class="bd form"><label>${t('ui.zone.export')}<textarea data-testid="zone-export-text" class="mono ltr" rows="9" readonly value=${exported.text}></textarea></label><${Missing} refs=${exported.missing} /></div></section>`}
    ${binding && html`<form class="card" onSubmit=${saveBinding}><fieldset disabled=${busy} class="bd form" style=${{minWidth:0}}><legend>${t('ui.zone.rebind')} · ${binding.zone.name}</legend>
      <p>${t('ui.zone.binding_hint')}</p>
      <label>${t('ui.zone.source')}<input data-testid="zone-source" value=${binding.source_node} onInput=${e=>setBinding({...binding,source_node:e.target.value,confirm:false})} /></label>
      <label>${t('ui.zone.fingerprint')}<input class="mono ltr" data-testid="zone-fingerprint" value=${binding.fingerprint} onInput=${e=>setBinding({...binding,fingerprint:e.target.value,confirm:false})} /></label>
      <label class="switch"><input type="checkbox" required data-testid="zone-confirm" checked=${binding.confirm} onChange=${e=>setBinding({...binding,confirm:e.target.checked})} />${t('ui.zone.confirm')}</label>
      <div class="row"><button type="submit" class="btn primary" data-testid="zone-binding-save">${t('ui.save')}</button><button type="button" class="btn" onClick=${()=>setBinding(null)}>${t('ui.cancel')}</button></div>
    </fieldset></form>`}
  </div>`;
}
function Missing({refs}) {
  if (!refs?.length) return null;
  return html`<div><b>${t('ui.zone.missing')}</b><ul>${refs.map(ref=>html`<li class="mono" style=${{overflowWrap:'anywhere'}}>${ref}</li>`)}</ul></div>`;
}
