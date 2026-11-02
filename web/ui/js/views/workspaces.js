// Workspace authority is server-derived; route selection never grants access.
import {html,useState,useEffect,useRef} from '../lib.js';
import {get,post,put,del,getCSRF,ApiError} from '../api.js';
import {useFetch,ErrorBox} from '../ui.js';

export function Workspaces(){
 const companies=useFetch(()=>get('/companies'),[]);
 const [company,setCompany]=useState('');
 const [selected,setSelected]=useState(null);
 const [explicit,setExplicit]=useState('');
 const id=company||companies.data?.items?.[0]?.id||'';
 return html`<div class="page col" style=${{gap:'16px',minWidth:0}}>
 <h1>Companies / Workspaces</h1>
 <p class="muted">Company membership and Workspace grants are checked on every operation. Legacy pages remain default-Workspace-only.</p>
 ${companies.error&&html`<${ErrorBox} error=${companies.error} retry=${companies.reload}/>`}
 <label>Company<select value=${id} onChange=${e=>{setCompany(e.target.value);setSelected(null);}}>${(companies.data?.items||[]).map(c=>html`<option value=${c.id}>${c.name}</option>`)}</select></label>
 ${companies.data?.discovery==='default_company_only'&&html`<form onSubmit=${e=>{e.preventDefault();setCompany(explicit);setSelected(null);}}><label>Existing company ID<input value=${explicit} onInput=${e=>setExplicit(e.target.value)} required /></label><button class="btn">Open existing company</button><p class="small muted">Discovery currently lists the default company only; another company requires an existing membership.</p></form>`}
 ${id&&html`<${WorkspaceCompany} key=${id} company=${id} selected=${selected} select=${setSelected}/>`}
 </div>`;
}
function WorkspaceCompany({company,selected,select}){
 const root='/companies/'+encodeURIComponent(company)+'/workspaces';
 const list=useFetch(()=>get(root),[root]);
 const [name,setName]=useState('');const [error,setError]=useState(null);const [busy,setBusy]=useState(false);const lock=useRef(false);
 const create=async e=>{e.preventDefault();if(lock.current)return;lock.current=true;setBusy(true);setError(null);try{await post(root,{name});setName('');list.reload();}catch(e){setError(e);}finally{lock.current=false;setBusy(false);}};
 return html`<section class="col" style=${{gap:'16px',minWidth:0}}><div class="mono small">Company: ${company}</div>
 ${list.error&&html`<${ErrorBox} error=${list.error} retry=${list.reload}/>`}${error&&html`<${ErrorBox} error=${error}/>`}
 <form onSubmit=${create}><fieldset disabled=${busy}><legend>Create Workspace (company admins)</legend><label>Name<input data-testid="workspace-name" value=${name} onInput=${e=>setName(e.target.value)} required maxlength="200" /></label><button class="btn" data-testid="workspace-create">Create Workspace</button></fieldset></form>
 <div class="row">${(list.data?.items||[]).map(w=>html`<button class="btn" onClick=${()=>select(w)}>${w.name}</button>`)}</div>
 ${selected&&html`<${WorkspacePanel} key=${selected.id} root=${root+'/'+encodeURIComponent(selected.id)} workspace=${selected} changed=${()=>{list.reload();select(null);}}/>`}</section>`;
}
function WorkspacePanel({root,workspace,changed}){
 const monitors=useFetch(()=>get(root+'/monitors'),[root]);
 const config=useFetch(()=>get(root+'/config'),[root]);
 const members=useFetch(()=>get(root+'/members'),[root]);
 const events=useFetch(()=>get(root+'/events'),[root]);
 const incidents=useFetch(()=>get(root+'/incidents'),[root]);
 const [text,setText]=useState('');const [source,setSource]=useState('gatus');const [preview,setPreview]=useState(null);const [file,setFile]=useState(null);
 const [raw,setRaw]=useState('@ IN SOA unagikuma. admin.localhost. ( 1 30s 5s 1s 2 )\n');
 const [name,setName]=useState(workspace.name);const [member,setMember]=useState('');const [role,setRole]=useState('viewer');
 const [busy,setBusy]=useState(false);const [error,setError]=useState(null);const [success,setSuccess]=useState('');const lock=useRef(false);
 useEffect(()=>{const timer=setInterval(()=>{monitors.reload();events.reload();incidents.reload();},2000);return()=>clearInterval(timer);},[root]);
 const revision=config.data?.revision;
 const run=async fn=>{if(lock.current)return;lock.current=true;setBusy(true);setError(null);setSuccess('');try{await fn();}catch(e){setError(e);if(e.status===412){setPreview(null);config.reload();}}finally{lock.current=false;setBusy(false);}};
 const upload=async apply=>{
  const r=await fetch('/api/v1'+root+'/import/'+source+(apply?'?apply=1':''),{method:'POST',credentials:'same-origin',headers:{'X-Unagikuma-CSRF':getCSRF(),'If-Match':'"'+revision+'"','Content-Type':'application/octet-stream'},body:file||text});
  const data=await r.json();if(!r.ok)throw new ApiError(r.status,data);return data;
 };
 const edit=fn=>e=>{fn(e.target.value);setPreview(null);setSuccess('');};
 const notification=monitors.data?.runtime?.notification_status||monitors.data?.notification_status;
 return html`<section class="card" style=${{minWidth:0}}><div class="hd"><h2>${workspace.name}</h2></div><div class="bd col" style=${{gap:'16px',minWidth:0}}>
 <div class="mono small">Workspace: ${workspace.id}</div>
 <div class="banner warning" data-testid="workspace-notifications">Notifications: ${notification||'status unavailable — delivery is not confirmed'}. ${notification?.startsWith('blocked')?'Blocked until scoped delivery is implemented and verified.':''}</div>
 ${monitors.data?.runtime&&html`<p>Runtime: ${monitors.data.runtime.state} ${monitors.data.runtime.code||''}</p>`}
 ${error&&html`<${ErrorBox} error=${error}/>`}${success&&html`<div class="banner ok" data-testid="workspace-success" role="status">${success}</div>`}
 <button class="btn" data-testid="workspace-refresh" onClick=${monitors.reload}>Refresh checks</button>
 ${monitors.error&&html`<${ErrorBox} error=${monitors.error} retry=${monitors.reload}/>`}
 <div data-testid="workspace-monitors">${(monitors.data?.items||[]).map(m=>html`<p key=${m.id}><b>${m.name}</b> · ${m.status} · ${m.last_check||'not checked'}</p>`)}</div>
 <h3>Workspace incidents</h3>
 ${incidents.error&&html`<${ErrorBox} error=${incidents.error} retry=${incidents.reload}/>`}
 <div data-testid="workspace-incidents">${(incidents.data?.items||[]).map(i=>html`<p key=${i.id}>Incident ${i.id} · monitor ${i.monitor_id} · ${i.resolved_at?'resolved':'open'} · delivered notifications ${i.delivered}</p>`)}</div>
 <h3>Workspace events</h3>
 ${events.error&&html`<${ErrorBox} error=${events.error} retry=${events.reload}/>`}
 <div data-testid="workspace-events">${(events.data?.items||[]).map(e=>html`<p key=${e.id}>${e.kind} · monitor ${e.monitor_id} · ${e.at}</p>`)}</div>
 ${workspace.is_default?html`<p>The default Workspace is managed by the legacy configuration editor. Scoped publication is not available here.</p>`:html`
 ${config.error&&html`<p>Configuration editing requires Workspace administrator membership.</p>`}
 ${config.data&&html`<fieldset disabled=${busy} style=${{minWidth:0}}><legend>Scoped configuration · revision ${revision}</legend>
 <label>Configuration (complete replacement)<textarea rows="8" class="mono" value=${raw} onInput=${edit(setRaw)}></textarea></label>
 <button class="btn" onClick=${()=>run(async()=>{await post(root+'/config/preview',{text:raw},{'If-Match':'"'+revision+'"'});setSuccess('Configuration validated; not applied.');})}>Validate configuration</button>
 <button class="btn" onClick=${()=>run(async()=>{await put(root+'/config',{text:raw},{'If-Match':'"'+revision+'"'});config.reload();setPreview(null);setSuccess('Configuration published to this Workspace only.');})}>Replace configuration</button>
 <label>Import source<select data-testid="workspace-import-source" value=${source} onChange=${edit(setSource)}><option value="gatus">Gatus</option><option value="checkmate">Checkmate</option><option value="kuma">Uptime Kuma</option><option value="nagios">Nagios</option></select></label>
 <label>Import file<input type="file" data-testid="workspace-import-file" onChange=${e=>{setFile(e.target.files?.[0]||null);setPreview(null);setSuccess('');}} /></label>
 <label>Import document<textarea data-testid="workspace-import-text" rows="8" class="mono" value=${text} onInput=${e=>{setFile(null);edit(setText)(e);}}></textarea></label>
 ${file&&html`<p>Upload: ${file.name}. Typing a document instead clears the selected upload.</p>`}
 <button class="btn" data-testid="workspace-import-preview" onClick=${()=>run(async()=>setPreview(await upload(false)))}>Preview import</button>
 ${preview&&html`<p data-testid="workspace-import-result">${preview.monitors} monitors · ${preview.blocked?'Unsupported semantics: apply blocked':'Ready to apply'}</p>`}
 <button class="btn primary" data-testid="workspace-import-apply" disabled=${!preview||preview.blocked||preview.revision!==revision} onClick=${()=>run(async()=>{await upload(true);setPreview(null);config.reload();monitors.reload();setSuccess('Import published to this Workspace only.');})}>Apply import</button>
 </fieldset>`}`}
 ${members.data&&html`<fieldset disabled=${busy}><legend>Workspace management</legend><label>Name<input value=${name} onInput=${e=>setName(e.target.value)} /></label><button class="btn" onClick=${()=>run(async()=>{await put(root,{name});changed();})}>Rename Workspace</button>
 <label>Existing company member user ID<input type="number" min="1" step="1" value=${member} onInput=${e=>setMember(e.target.value)} /></label><label>Workspace role<select value=${role} onChange=${e=>setRole(e.target.value)}><option>viewer</option><option>operator</option><option>admin</option></select></label>
 <button class="btn" disabled=${!/^\d+$/.test(member)||Number(member)<=0} onClick=${()=>run(async()=>{await put(root+'/members/'+member,{role,disabled:false});members.reload();setSuccess('Workspace grant saved.');})}>Grant Workspace access</button>
 ${(members.data.items||[]).map(m=>html`<p>User ${m.user_id} · ${m.role} · ${m.disabled?'disabled':'active'} <button class="btn" onClick=${()=>run(async()=>{await del(root+'/members/'+m.user_id);members.reload();setSuccess('Workspace grant revoked.');})}>Revoke user ${m.user_id}</button></p>`)}</fieldset>`}
 </div></section>`;
}
