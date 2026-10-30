// Settings: general, tunables, API tokens, secrets, keywords, system.
import { html, useState, useEffect, useMemo } from '../lib.js';
import { t, dur, fmtTime, currentLang, langList } from '../i18n.js';
import { get, post, put, del, setCSRF, getCSRF } from '../api.js';
import { Nodes } from './nodes.js';
import { useFetch, Loading, ErrorBox, Empty, Field, Card, Modal, Copy, Ago, bytes, toast, toastError, errText, confirmDialog, useDocTitle } from '../ui.js';

const EXIT_KEY = 'localnet.exit-target';

function General() {
  const s = useFetch(() => get('/settings'), []);
  const tun = useFetch(() => get('/tunables'), []);
  const [f, setF] = useState(null);
  const [hb, setHb] = useState(null);
  const [exit, setExit] = useState(null);
  useEffect(() => { if (s.data) setF({ ...s.data }); }, [s.data]);
  const exitTun = tun.data && tun.data.find(x => x.key === EXIT_KEY);
  useEffect(() => { if (exitTun) setExit(exitTun.value); }, [tun.data]);
  if (!f) return s.error ? html`<${ErrorBox} error=${s.error} />` : html`<${Loading} />`;
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
  const save = async (e) => {
    e.preventDefault();
    try {
      // The internet gateway is a tunable; it is written only when changed,
      // and going back to the default removes the override.
      if (exitTun && exit !== null && exit.trim() !== exitTun.value) {
        const v = exit.trim();
        await put('/tunables', v === exitTun.default ? { set: {}, unset: [EXIT_KEY] } : { set: { [EXIT_KEY]: v }, unset: [] });
        tun.reload();
      }
      s.setData(await put('/settings', f));
      toast(t('ui.saved'));
    } catch (e2) { toastError(e2); }
  };
  const testHb = async () => {
    setHb({ busy: true });
    try { setHb(await post('/settings/heartbeat/test')); } catch (e) { setHb({ err: e }); }
  };
  return html`<form class="card" onSubmit=${save}><div class="bd form">
    <div class="two">
      <${Field} label=${t('ui.set.instance')} hint=${t('ui.set.instance_hint')}><input type="text" value=${f.instance_name} onInput=${set('instance_name')} /><//>
      <${Field} label=${t('ui.set.base_url')} hint=${t('ui.set.base_url_hint')}><input type="url" class="ltr" value=${f.base_url} onInput=${set('base_url')} placeholder="https://monitor.example.com" /><//>
    </div>
    <${Field} label=${t('ui.set.message_lang')} hint=${t('ui.set.message_lang_hint')}>
      <select value=${f.message_lang || 'en'} onChange=${set('message_lang')}>${langList().map(l => html`<option value=${l.code}>${l.name}</option>`)}</select><//>
    ${exitTun && html`<${Field} label=${t('ui.set.exit')} hint=${t('ui.set.exit_hint', { addr: exitTun.default })}>
      <input type="text" class="ltr" value=${exit ?? ''} onInput=${(e) => setExit(e.target.value)} placeholder=${t('ui.set.exit_off')} style=${{ maxWidth: '320px' }} /><//>`}
    <${Field} label=${t('ui.set.heartbeat')} hint=${t('ui.set.heartbeat_hint')}>
      <div class="row nowrap"><input type="url" class="ltr" value=${f.heartbeat_url} onInput=${set('heartbeat_url')} placeholder="https://hc-ping.com/…" />
        <button type="button" class="btn" onClick=${testHb} disabled=${!s.data.heartbeat_url}>${t('ui.set.heartbeat_test')}</button></div><//>
    ${hb && !hb.busy && html`<div class=${'banner ' + (hb.err || hb.LastError ? 'error' : 'ok')}>${hb.err ? errText(hb.err) : hb.LastError || t('ui.set.heartbeat_ok')}</div>`}
    <div class="row"><div class="spacer"></div><button class="btn primary">${t('ui.save')}</button></div>
  </div></form>`;
}

function Tunables() {
  const list = useFetch(() => get('/tunables'), []);
  const [filter, setFilter] = useState('');
  const [onlyChanged, setOnlyChanged] = useState(false);
  const [edits, setEdits] = useState({});
  const [unset, setUnset] = useState(new Set());
  const [busy, setBusy] = useState(false);
  const zh = currentLang() === 'zh';
  const rows = useMemo(() => {
    if (!list.data) return [];
    const q = filter.toLowerCase();
    return list.data.filter(x => (!onlyChanged || x.overridden) && (!q || x.key.includes(q) || x.desc.toLowerCase().includes(q) || x.desc_zh.includes(q)));
  }, [list.data, filter, onlyChanged]);
  if (!list.data) return list.error ? html`<${ErrorBox} error=${list.error} />` : html`<${Loading} />`;
  const pending = Object.keys(edits).length + unset.size;
  const save = async () => {
    setBusy(true);
    try {
      await put('/tunables', { set: edits, unset: [...unset] });
      toast(t('ui.saved'));
      setEdits({});
      setUnset(new Set());
      list.reload();
    } catch (e) { toastError(e); }
    setBusy(false);
  };
  const groups = {};
  for (const r of rows) (groups[r.key.split('.')[0]] = groups[r.key.split('.')[0]] || []).push(r);
  return html`<div class="col" style=${{ gap: '12px' }}>
    <p class="muted small" style=${{ margin: 0 }}>${t('ui.tun.intro')}</p>
    <div class="row">
      <input type="search" style=${{ maxWidth: '320px' }} placeholder=${t('ui.tun.search')} value=${filter} onInput=${(e) => setFilter(e.target.value)} />
      <label class="switch small"><input type="checkbox" checked=${onlyChanged} onChange=${(e) => setOnlyChanged(e.target.checked)} /> ${t('ui.tun.only_changed')}</label>
      <div class="spacer"></div>
      ${pending > 0 && html`<button class="btn" onClick=${() => { setEdits({}); setUnset(new Set()); }}>${t('ui.discard')}</button>`}
      <button class="btn primary" disabled=${!pending || busy} onClick=${save}>${t('ui.tun.save', { n: pending })}</button>
    </div>
    ${Object.entries(groups).map(([g, items]) => html`<${Card} title=${g} flush>
      <div class="tbl-wrap"><table class="tbl"><tbody>${items.map(x => {
        const editing = x.key in edits;
        const cleared = unset.has(x.key);
        const val = editing ? edits[x.key] : cleared ? x.default : x.value;
        return html`<tr>
          <td style=${{ width: '34%' }}><div class="mono ltr">${x.key}</div><div class="sub">${zh && x.desc_zh ? x.desc_zh : x.desc}</div></td>
          <td style=${{ width: '22%' }}>
            ${x.override ? html`<span class="small muted">${t('ui.tun.server_only')}</span>` : html`<input type="text" class="ltr" value=${val}
              onInput=${(e) => { const v = e.target.value; const n = { ...edits }; if (v === x.value && !cleared) delete n[x.key]; else n[x.key] = v; setEdits(n); }} />`}
          </td>
          <td class="small muted nowrap">${t('ui.tun.default')}: <span class="ltr">${x.default}</span>${x.min || x.max ? html`<div class="ltr">${x.min || '…'} – ${x.max || '…'}</div>` : ''}</td>
          <td class="right nowrap">${(x.overridden && !cleared) && html`<button class="btn sm" onClick=${() => { const s = new Set(unset); s.add(x.key); setUnset(s); const n = { ...edits }; delete n[x.key]; setEdits(n); }}>${t('ui.tun.reset')}</button>`}
            ${(editing || cleared) && html`<span class="tag warn">${t('ui.tun.changed')}</span>`}</td>
        </tr>`;
      })}</tbody></table></div>
    <//>`)}
  </div>`;
}

const SCOPES = ['read', 'ack', 'write', 'admin'];

const ROLES = ['admin', 'operator', 'viewer'];

// Users: accounts, their role and, for operators, the sections they may
// change.
function Users() {
  const list = useFetch(() => get('/users'), []);
  const [form, setForm] = useState(null);
  if (!list.data) return list.error ? html`<${ErrorBox} error=${list.error} />` : html`<${Loading} />`;
  const save = async (e) => {
    e.preventDefault();
    const body = { role: form.role, areas: form.areas.split(/[\s,]+/).filter(Boolean) };
    if (form.password) body.password = form.password;
    try {
      if (form.id) await put('/users/' + form.id, body);
      else await post('/users', { ...body, username: form.username.trim() });
      toast(t('ui.saved'));
      setForm(null);
      list.reload();
    } catch (e2) { toastError(e2); }
  };
  const remove = async (u) => {
    if (!await confirmDialog(t('ui.user.delete_confirm', { name: u.username }), { danger: true, ok: t('ui.delete') })) return;
    try { await del('/users/' + u.id); list.reload(); } catch (e) { toastError(e); }
  };
  return html`<div class="col" style=${{ gap: '12px' }}>
    <div class="row"><p class="muted small spacer" style=${{ margin: 0 }}>${t('ui.user.intro')}</p>
      <button class="btn primary" onClick=${() => setForm({ username: '', password: '', role: 'operator', areas: '' })}>${t('ui.user.add')}</button></div>
    <div class="card"><div class="bd flush tbl-wrap"><table class="tbl">
      <thead><tr><th>${t('ui.name')}</th><th>${t('ui.user.role')}</th><th>${t('ui.user.areas')}</th><th>${t('ui.tok.created_at')}</th><th></th></tr></thead>
      <tbody>${list.data.items.map(u => html`<tr>
        <td class="name">${u.username}</td>
        <td><span class="tag">${t('ui.user.role.' + u.role)}</span></td>
        <td class="small mono">${u.areas.length ? u.areas.join(', ') : html`<span class="faint">${t('ui.user.all')}</span>`}</td>
        <td class="small">${fmtTime(u.created_at)}</td>
        <td class="right nowrap"><button class="btn sm" onClick=${() => setForm({ id: u.id, username: u.username, password: '', role: u.role, areas: u.areas.join(', ') })}>${t('ui.edit')}</button>
          ${' '}<button class="btn sm danger" onClick=${() => remove(u)}>${t('ui.delete')}</button></td>
      </tr>`)}</tbody></table></div></div>
    ${form && html`<${Modal} title=${form.id ? form.username : t('ui.user.add')} onClose=${() => setForm(null)}>
      <form class="form" onSubmit=${save}>
        ${!form.id && html`<${Field} label=${t('ui.name')}><input type="text" required value=${form.username} onInput=${(e) => setForm({ ...form, username: e.target.value })} /><//>`}
        <${Field} label=${t('ui.user.password')} hint=${form.id ? t('ui.user.password_keep') : ''}>
          <input type="password" autocomplete="new-password" minlength="8" required=${!form.id} value=${form.password} onInput=${(e) => setForm({ ...form, password: e.target.value })} /><//>
        <${Field} label=${t('ui.user.role')}>
          <div class="col">${ROLES.map(r => html`<label class="switch"><input type="radio" name="role" checked=${form.role === r} onChange=${() => setForm({ ...form, role: r })} />
            <b>${t('ui.user.role.' + r)}</b> <span class="small muted">${t('ui.user.role.' + r + '_hint')}</span></label>`)}</div><//>
        ${form.role === 'operator' && html`<${Field} label=${t('ui.user.areas')} hint=${t('ui.user.areas_hint')}>
          <input type="text" class="ltr mono" value=${form.areas} onInput=${(e) => setForm({ ...form, areas: e.target.value })} placeholder="region:north, bj" /><//>`}
        <div class="row"><div class="spacer"></div><button class="btn primary">${t('ui.save')}</button></div>
      </form>
    <//>`}
  </div>`;
}

function Tokens() {
  const list = useFetch(() => get('/tokens'), []);
  const [form, setForm] = useState(null);
  const [created, setCreated] = useState(null);
  const create = async (e) => {
    e.preventDefault();
    try {
      const r = await post('/tokens', { name: form.name, scopes: [form.scope] });
      setCreated(r.token);
      setForm(null);
      list.reload();
    } catch (e2) { toastError(e2); }
  };
  const remove = async (tk) => {
    if (!await confirmDialog(t('ui.tok.delete_confirm', { name: tk.name }), { danger: true, ok: t('ui.delete') })) return;
    try { await del('/tokens/' + tk.id); list.reload(); } catch (e) { toastError(e); }
  };
  if (!list.data) return list.error ? html`<${ErrorBox} error=${list.error} />` : html`<${Loading} />`;
  return html`<div class="col" style=${{ gap: '12px' }}>
    <div class="row"><p class="muted small spacer" style=${{ margin: 0 }}>${t('ui.tok.intro')}</p><button class="btn primary" onClick=${() => setForm({ name: '', scope: 'read' })}>${t('ui.tok.add')}</button></div>
    ${created && html`<div class="banner ok"><div class="col spacer"><b>${t('ui.tok.created')}</b><div class="row nowrap"><code class="code ltr" style=${{ flex: 1 }}>${created}</code><${Copy} text=${created} /></div></div></div>`}
    <div class="card"><div class="bd flush tbl-wrap">
      ${!list.data.length ? html`<${Empty}>${t('ui.tok.none')}<//>` : html`<table class="tbl">
        <thead><tr><th>${t('ui.name')}</th><th>${t('ui.tok.scopes')}</th><th>${t('ui.tok.created_at')}</th><th>${t('ui.tok.last_used')}</th><th></th></tr></thead>
        <tbody>${list.data.map(tk => html`<tr>
          <td><div class="name">${tk.name}</div><div class="sub mono ltr">${tk.prefix}…</div></td>
          <td>${tk.scopes.map(s => html`<span class="tag">${t('ui.scope.' + s)}</span> `)}</td>
          <td class="small">${fmtTime(tk.created_at)}</td>
          <td class="small"><${Ago} at=${tk.last_used} /></td>
          <td class="right"><button class="btn sm danger" onClick=${() => remove(tk)}>${t('ui.delete')}</button></td>
        </tr>`)}</tbody></table>`}
    </div></div>
    ${form && html`<${Modal} title=${t('ui.tok.add')} onClose=${() => setForm(null)}>
      <form class="form" onSubmit=${create}>
        <${Field} label=${t('ui.name')}><input type="text" value=${form.name} onInput=${(e) => setForm({ ...form, name: e.target.value })} required /><//>
        <${Field} label=${t('ui.tok.scope')}>
          <div class="col">${SCOPES.map(s => html`<label class="switch"><input type="radio" name="scope" checked=${form.scope === s} onChange=${() => setForm({ ...form, scope: s })} />
            <b>${t('ui.scope.' + s)}</b> <span class="small muted">${t('ui.scope.' + s + '_hint')}</span></label>`)}</div><//>
        <div class="row"><div class="spacer"></div><button class="btn primary">${t('ui.tok.create')}</button></div>
      </form>
    <//>`}
  </div>`;
}

function Secrets() {
  const list = useFetch(() => get('/secrets'), []);
  const [form, setForm] = useState(null);
  const save = async (e) => {
    e.preventDefault();
    try { await put('/secrets/' + encodeURIComponent(form.name), { value: form.value }); toast(t('ui.saved')); setForm(null); list.reload(); } catch (e2) { toastError(e2); }
  };
  const remove = async (s) => {
    if (!await confirmDialog(t('ui.sec.delete_confirm', { name: s.name }), { danger: true, ok: t('ui.delete') })) return;
    try { await del('/secrets/' + encodeURIComponent(s.name)); list.reload(); } catch (e) { toastError(e); }
  };
  if (!list.data) return list.error ? html`<${ErrorBox} error=${list.error} />` : html`<${Loading} />`;
  return html`<div class="col" style=${{ gap: '12px' }}>
    <div class="row"><p class="muted small spacer" style=${{ margin: 0 }}>${t('ui.sec.intro')} <code class="ltr">${'${secret:name}'}</code></p>
      <button class="btn primary" onClick=${() => setForm({ name: '', value: '' })}>${t('ui.sec.add')}</button></div>
    <div class="card"><div class="bd flush tbl-wrap">
      ${!list.data.length ? html`<${Empty}>${t('ui.sec.none')}<//>` : html`<table class="tbl"><tbody>${list.data.map(s => html`<tr>
        <td class="mono ltr">${'${secret:' + s.name + '}'}</td><td class="small muted">${fmtTime(s.updated_at)}</td>
        <td class="right nowrap"><button class="btn sm" onClick=${() => setForm({ name: s.name, value: '', existing: true })}>${t('ui.sec.replace')}</button>
          <button class="btn sm danger" onClick=${() => remove(s)}>${t('ui.delete')}</button></td></tr>`)}</tbody></table>`}
    </div></div>
    ${form && html`<${Modal} title=${form.existing ? t('ui.sec.replace') : t('ui.sec.add')} onClose=${() => setForm(null)}>
      <form class="form" onSubmit=${save}>
        <${Field} label=${t('ui.name')}><input type="text" class="ltr" value=${form.name} disabled=${form.existing} onInput=${(e) => setForm({ ...form, name: e.target.value })} required /><//>
        <${Field} label=${t('ui.value')} hint=${t('ui.sec.value_hint')}><input type="password" class="ltr" value=${form.value} onInput=${(e) => setForm({ ...form, value: e.target.value })} required autocomplete="off" /><//>
        <div class="row"><div class="spacer"></div><button class="btn primary">${t('ui.save')}</button></div>
      </form>
    <//>`}
  </div>`;
}

function Keywords() {
  const [dialect, setDialect] = useState('');
  const list = useFetch(() => get('/keywords' + (dialect ? '?dialect=' + dialect : '')), [dialect]);
  const [q, setQ] = useState('');
  const zh = currentLang() === 'zh';
  if (!list.data) return list.error ? html`<${ErrorBox} error=${list.error} />` : html`<${Loading} />`;
  const rows = list.data.filter(k => !q || k.keyword.includes(q.toLowerCase()) || String(k.port) === q || k.service.toLowerCase().includes(q.toLowerCase()));
  return html`<div class="col" style=${{ gap: '12px' }}>
    <p class="muted small" style=${{ margin: 0 }}>${t('ui.kw.intro')}</p>
    <div class="row">
      <input type="search" style=${{ maxWidth: '280px' }} placeholder=${t('ui.kw.search')} value=${q} onInput=${(e) => setQ(e.target.value)} />
      <select style=${{ width: 'auto' }} value=${dialect} onChange=${(e) => setDialect(e.target.value)} aria-label=${t('ui.kw.dialect')}>
        <option value="">${t('ui.kw.dialect')}: auto</option>${['huawei', 'cisco', 'asa', 'xr'].map(d => html`<option value=${d}>${d}</option>`)}
      </select>
    </div>
    <div class="card"><div class="bd flush tbl-wrap"><table class="tbl">
      <thead><tr><th>${t('ui.kw.keyword')}</th><th class="right">${t('ui.kw.port')}</th><th>${t('ui.kw.depth')}</th><th>${t('ui.kw.service')}</th></tr></thead>
      <tbody>${rows.map(k => html`<tr>
        <td class="mono ltr">${k.keyword}${k.aliases && k.aliases.length ? html`<div class="sub">${k.aliases.join(', ')}</div>` : ''}</td>
        <td class="right mono">${k.port}</td>
        <td><span class=${'tag' + (k.effective_depth === 'L1' ? ' weak' : '')}>${k.effective_depth}</span>
          ${k.reflective && html` <span class="tag warn" title=${t('ui.kw.reflective_hint')}>${t('ui.kw.reflective')}</span>`}
          ${k.privileged && html` <span class="tag">${t('ui.kw.privileged')}</span>`}</td>
        <td>${zh && k.service_zh ? k.service_zh : k.service}${(k.suggest || k.note) && html`<div class="sub">${k.suggest || k.note}</div>`}</td>
      </tr>`)}</tbody>
    </table></div></div>
  </div>`;
}

function System() {
  const s = useFetch(() => get('/system'), []);
  if (!s.data) return s.error ? html`<${ErrorBox} error=${s.error} />` : html`<${Loading} />`;
  const d = s.data;
  return html`<div class="grid g2">
    <${Card} title=${t('ui.sys.about')}>
      <dl class="kv">
        <dt>${t('ui.sys.version')}</dt><dd class="mono">${d.version}</dd>
        <dt>${t('ui.sys.instance')}</dt><dd>${d.instance}</dd>
        <dt>${t('ui.sys.platform')}</dt><dd class="mono">${d.platform} · ${d.go}</dd>
        <dt>${t('ui.sys.uptime')}</dt><dd>${dur(d.uptime_s)}</dd>
        <dt>${t('ui.sys.config')}</dt><dd class="mono ltr">${d.config_path}</dd>
        <dt>${t('ui.sys.clients')}</dt><dd>${d.sse_clients}</dd>
      </dl>
    <//>
    <${Card} title=${t('ui.sys.storage')}>
      <dl class="kv">
        <dt>core.db</dt><dd>${bytes(d.storage.core_bytes)}</dd>
        <dt>history.db</dt><dd>${bytes(d.storage.history_bytes)}</dd>
        ${Object.entries(d.storage.rows || {}).map(([k, v]) => html`<dt class="mono">${k}</dt><dd>${v}</dd>`)}
      </dl>
    <//>
    <${BackupCard} />
  </div>`;
}

// BackupCard downloads an archive. With a passphrase it is encrypted and
// also carries the secret key and the cluster CA.
function BackupCard() {
  const [history, setHistory] = useState(false);
  const [pass, setPass] = useState('');
  const [busy, setBusy] = useState(false);
  const download = async (e) => {
    e.preventDefault();
    if (pass && pass.length < 8) return toast(t('ui.sys.backup_pass_short'), 'error');
    setBusy(true);
    try {
      const r = pass
        ? await fetch('/api/v1/backup', { method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-Unagikuma-CSRF': getCSRF() }, body: JSON.stringify({ passphrase: pass, history }) })
        : await fetch('/api/v1/backup' + (history ? '?history=1' : ''), { credentials: 'same-origin' });
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).message || 'HTTP ' + r.status);
      const name = ((r.headers.get('content-disposition') || '').match(/filename="([^"]+)"/) || [])[1] || 'unagikuma-backup';
      const url = URL.createObjectURL(await r.blob());
      const a = document.createElement('a');
      a.href = url; a.download = name;
      document.body.appendChild(a); a.click(); a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 10000);
    } catch (e2) { toastError(e2); }
    setBusy(false);
  };
  return html`<${Card} title=${t('ui.sys.backup')}>
    <form class="form" onSubmit=${download}>
      <p class="small muted" style=${{ margin: 0 }}>${t('ui.sys.backup_hint')}</p>
      <label class="switch small"><input type="checkbox" checked=${history} onChange=${(e) => setHistory(e.target.checked)} /> ${t('ui.sys.backup_history')}</label>
      <${Field} label=${t('ui.sys.backup_pass')} hint=${t('ui.sys.backup_pass_hint')}>
        <input type="password" autocomplete="new-password" value=${pass} onInput=${(e) => setPass(e.target.value)} /><//>
      <div class="row"><div class="spacer"></div><button class="btn primary" disabled=${busy}>${busy ? html`<span class="spin"></span>` : t('ui.sys.backup')}</button></div>
    </form>
  <//>`;
}

function Account({ user }) {
  const [f, setF] = useState({ old: '', new: '', confirm: '' });
  const [err, setErr] = useState('');
  const save = async (e) => {
    e.preventDefault();
    if (f.new.length < 8) return setErr(t('ui.setup.short'));
    if (f.new !== f.confirm) return setErr(t('ui.setup.mismatch'));
    try {
      const r = await put('/session/password', { old: f.old, new: f.new });
      setCSRF(r.csrf);
      toast(t('ui.acc.changed'));
      setF({ old: '', new: '', confirm: '' });
      setErr('');
    } catch (e2) { setErr(errText(e2)); }
  };
  return html`<form class="card" onSubmit=${save} style=${{ maxWidth: '480px' }}><div class="bd form">
    <p class="muted">${t('ui.acc.signed_in', { user })}</p>
    <${Field} label=${t('ui.acc.old')}><input type="password" value=${f.old} onInput=${(e) => setF({ ...f, old: e.target.value })} required autocomplete="current-password" /><//>
    <${Field} label=${t('ui.acc.new')}><input type="password" value=${f.new} onInput=${(e) => setF({ ...f, new: e.target.value })} required autocomplete="new-password" /><//>
    <${Field} label=${t('ui.setup.confirm')}><input type="password" value=${f.confirm} onInput=${(e) => setF({ ...f, confirm: e.target.value })} required autocomplete="new-password" /><//>
    ${err && html`<div class="banner error">${err}</div>`}
    <div class="row"><div class="spacer"></div><button class="btn primary">${t('ui.acc.change')}</button></div>
  </div></form>`;
}

function auditDetail(d) {
  if (!d) return '';
  const who = (x) => x ? t('ui.user.role.' + x.role) + (x.areas && x.areas.length ? ' [' + x.areas.join(', ') + ']' : '') : '';
  const parts = [];
  if (d.before || d.after) parts.push(who(d.before) + ' → ' + who(d.after));
  for (const [k, v] of Object.entries(d)) {
    if (k === 'before' || k === 'after' || v === null) continue;
    const val = Array.isArray(v) ? v.map(x => typeof x === 'object' ? (x.username || '') + (x.result ? ':' + x.result : '') : x).join(', ') : typeof v === 'object' ? JSON.stringify(v) : String(v);
    if (val !== '') parts.push(k + ': ' + val);
  }
  return parts.join(' · ');
}

const AUDIT_ACTIONS = ['user', 'token', 'secret', 'node', 'cluster', 'config', 'monitor', 'maintenance', 'channel', 'policy', 'incident', 'session', 'import', 'backup', 'settings', 'tunables', 'oncall', 'people', 'system', 'audit'];

function CheckScripts() {
  const rep = useFetch(() => get('/scripts'), []);
  if (!rep.data) return rep.error ? html`<${ErrorBox} error=${rep.error} />` : html`<${Loading} />`;
  const r = rep.data;
  const chip = (st) => html`<span class=${'pill ' + (st === 'ok' ? 'ok' : st === 'differs' || st === 'offline' ? 'degraded' : 'critical')}>${t('ui.scripts.state.' + st)}</span>`;
  return html`<div class="col" style=${{ gap: '12px' }}>
    <p class="muted small" style=${{ margin: 0 }}>${t('ui.scripts.intro')} <code class="ltr">${(r.dirs || []).join(', ')}</code></p>
    ${r.problems > 0 && html`<div class="banner error">${t('ui.scripts.problems', { n: r.problems })}</div>`}
    <${Card} title=${t('ui.scripts.checks')} flush>
      ${!r.checks.length ? html`<p class="muted small" style=${{ padding: '12px' }}>${t('ui.scripts.none')}</p>` : html`<div class="tbl-wrap"><table class="tbl dense">
        <thead><tr><th>${t('ui.monitor')}</th><th>${t('ui.scripts.script')}</th><th>${t('ui.audit.area')}</th><th>${t('ui.scripts.nodes')}</th></tr></thead>
        <tbody>${r.checks.map(c => html`<tr>
          <td><a href=${'#/monitors/' + encodeURIComponent(c.monitor)} class="ltr">${c.monitor}</a>${c.paused && html` <span class="tag">${t('ui.scripts.paused')}</span>`}</td>
          <td class="ltr"><code>${c.plugin}</code></td>
          <td class="ltr small">${c.area || ''}</td>
          <td>${c.nodes.map(n => html`<span class="nowrap" style=${{ marginInlineEnd: '10px' }}><span class="ltr">${n.node}</span> ${chip(n.state)}</span>`)}</td>
        </tr>`)}</tbody></table></div>`}
    <//>
    ${r.nodes.map(n => html`<${Card} title=${n.node + (n.local ? ' · ' + t('ui.scripts.this_node') : '') + (n.online ? '' : ' · ' + t('ui.scripts.state.offline'))} flush>
      ${!n.scripts.length ? html`<p class="muted small" style=${{ padding: '12px' }}>${t('ui.scripts.empty_dir')}</p>` : html`<div class="tbl-wrap"><table class="tbl dense">
        <thead><tr><th>${t('ui.scripts.script')}</th><th>${t('ui.scripts.size')}</th><th>${t('ui.scripts.hash')}</th><th></th></tr></thead>
        <tbody>${n.scripts.map(s => html`<tr><td class="ltr"><code>${s.name}</code></td><td class="small">${bytes(s.size)}</td><td class="ltr small muted">${s.hash || ''}</td>
          <td>${!s.exec && chip('not_executable')}</td></tr>`)}</tbody></table></div>`}
    <//>`)}
  </div>`;
}

function Registration() {
  const lic = useFetch(() => get('/license'), []);
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  if (!lic.data) return lic.error ? html`<${ErrorBox} error=${lic.error} />` : html`<${Loading} />`;
  const s = lic.data;
  const install = async (body) => {
    setBusy(true);
    try {
      const r = await fetch('/api/v1/license', { method: 'PUT', credentials: 'same-origin', headers: { 'X-Unagikuma-CSRF': getCSRF() }, body });
      const j = await r.json();
      if (!r.ok) throw new Error(j.message || 'HTTP ' + r.status);
      toast(t('ui.reg.installed')); setText(''); lic.reload();
    } catch (e) { toastError(e); }
    setBusy(false);
  };
  const remove = async () => {
    if (!await confirmDialog(t('ui.reg.remove_confirm'))) return;
    try { await del('/license'); lic.reload(); } catch (e) { toastError(e); }
  };
  const row = (k, v) => v !== undefined && v !== '' && v !== null && html`<tr><th>${k}</th><td class="ltr">${v}</td></tr>`;
  // Over the limit only stops adding more; nothing is switched off.
  const quota = (used, limit) => !limit ? html`${used} <span class="muted small">· ${t('ui.reg.unlimited')}</span>`
    : html`<span class=${used > limit ? 'warn-text' : ''}>${t('ui.reg.usage', { used, limit })}</span>${used > limit ? html` <span class="small muted">${t('ui.reg.over_quota')}</span>` : ''}`;
  return html`<div class="col" style=${{ gap: '12px' }}>
    <${Card} title=${s.registered ? t('ui.reg.registered_to', { name: s.customer }) : s.kind === 'expired' ? t('ui.reg.expired') : t('ui.reg.unregistered')}
      actions=${s.registered && html`<button class="btn sm" onClick=${remove}>${t('ui.reg.remove')}</button>`}>
      ${!s.registered && html`<p class="small muted" style=${{ marginTop: 0 }}>${t('ui.reg.intro')}</p>`}
      ${s.reason && html`<div class="banner warning small">${s.reason}</div>`}
      <table class="kvtable"><tbody>
        <tr><th>${t('ui.reg.installation')}</th><td class="ltr"><b>${s.installation_id}</b> <${Copy} text=${s.installation_id} /></td></tr>
        ${row(t('ui.reg.licensee'), s.customer && s.customer + (s.licensee ? ' (' + t('ui.reg.licensee.' + s.licensee) + ')' : ''))}
        ${row(t('ui.reg.tier'), s.tier)}
        ${row(t('ui.reg.type'), s.type)}
        ${row(t('ui.reg.expires'), s.registered ? (s.days < 0 ? t('ui.reg.never') : s.expires + ' (' + t('ui.reg.days_left', { n: s.days }) + ')') : s.expires)}
        ${row(t('ui.reg.updates'), s.updates_until)}
        ${row(t('ui.reg.features'), (s.features || []).map(f => t('ui.reg.feat.' + f)).join(', '))}
        ${s.usage && html`<tr><th>${t('ui.reg.nodes')}</th><td>${quota(s.usage.nodes, s.registered && s.max_nodes)}</td></tr>
          <tr><th>${t('ui.reg.users')}</th><td>${quota(s.usage.users, s.registered && s.max_users)}</td></tr>`}
        ${row(t('ui.reg.fingerprint'), s.fingerprint)}
      </tbody></table>
    <//>
    <${Card} title=${t('ui.reg.import')}>
      <div class="form">
        <p class="small muted" style=${{ margin: 0 }}>${t('ui.reg.import_hint')}</p>
        <input type="file" accept=".json,application/json" onChange=${(e) => e.target.files[0] && install(e.target.files[0])} disabled=${busy} />
        <textarea class="ltr mono" rows="6" placeholder='{"key_id": "…", "signature": "…"}' value=${text} onInput=${(e) => setText(e.target.value)}></textarea>
        <div class="row"><div class="spacer"></div><button class="btn primary" disabled=${busy || !text.trim()} onClick=${() => install(text)}>${t('ui.reg.install')}</button></div>
      </div>
    <//>
  </div>`;
}

function Audit() {
  const [f, setF] = useState({ actor: '', action: '', area: '', addr: '', range: '', risk: '' });
  const [items, setItems] = useState(null);
  const [more, setMore] = useState(false);
  const [err, setErr] = useState(null);
  const [check, setCheck] = useState(null);
  const query = () => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(f)) if (v.trim()) q.set(k, v.trim());
    return q;
  };
  const verify = async () => {
    try { setCheck(await get('/audit/verify')); } catch (e) { toastError(e); }
  };
  const load = async (before) => {
    const q = query();
    q.set('limit', '100');
    if (before) q.set('before', before);
    try {
      const r = await get('/audit?' + q);
      setItems(before ? [...items, ...r.items] : r.items);
      setMore(r.items.length === 100);
      setErr(null);
    } catch (e) { setErr(e); }
  };
  useEffect(() => { load(); }, [f.action, f.range, f.risk]);
  if (err && err.code === 'registration_required') return html`<div class="card"><div class="bd col" style=${{ gap: '10px' }}>
    <b>${t('ui.audit.locked')}</b><p class="small muted" style=${{ margin: 0 }}>${t('ui.audit.locked_hint')}</p>
    <div><a class="btn primary" href="#/settings/registration">${t('ui.set.tab.registration')}</a></div></div></div>`;
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
  return html`<div class="col" style=${{ gap: '12px' }}>
    <p class="muted small" style=${{ margin: 0 }}>${t('ui.audit.intro')}</p>
    <form class="row" onSubmit=${(e) => { e.preventDefault(); load(); }}>
      <input type="text" class="ltr" placeholder=${t('ui.audit.actor')} value=${f.actor} onInput=${set('actor')} style=${{ maxWidth: '180px' }} />
      <select value=${f.action} onChange=${set('action')} style=${{ maxWidth: '180px' }}>
        <option value="">${t('ui.audit.all')}</option>
        ${AUDIT_ACTIONS.map(a => html`<option value=${a}>${a}</option>`)}
      </select>
      <input type="text" class="ltr" placeholder=${t('ui.audit.area')} value=${f.area} onInput=${set('area')} style=${{ maxWidth: '180px' }} />
      <input type="text" class="ltr" placeholder=${t('ui.audit.addr')} value=${f.addr} onInput=${set('addr')} style=${{ maxWidth: '160px' }} />
      <select value=${f.range} onChange=${set('range')} aria-label=${t('ui.audit.since')}>
        <option value="">${t('ui.audit.any_time')}</option>
        ${['24h', '7d', '30d', '90d'].map(r => html`<option value=${r}>${r}</option>`)}
      </select>
      <label class="row small" style=${{ gap: '4px' }}><input type="checkbox" checked=${f.risk === 'high'} onChange=${(e) => setF({ ...f, risk: e.target.checked ? 'high' : '' })} />${t('ui.audit.high_only')}</label>
      <button class="btn">${t('ui.audit.filter')}</button>
      <div class="spacer"></div>
      <a class="btn" href=${'/api/v1/audit/export?format=csv&' + query()} download>${t('ui.audit.export_csv')}</a>
      <a class="btn" href=${'/api/v1/audit/export?format=json&' + query()} download>${t('ui.audit.export_json')}</a>
      <button type="button" class="btn" onClick=${verify}>${t('ui.audit.verify')}</button>
    </form>
    ${check && html`<div class=${'banner ' + (check.ok ? 'ok' : 'error')}>${check.ok ? html`${t('ui.audit.verify_ok', { n: check.entries, head: '' })}<code class="ltr small">${check.head.slice(0, 16)}…</code>` : t('ui.audit.verify_bad', { problem: check.problem })}</div>`}
    ${err && html`<${ErrorBox} error=${err} />`}
    ${!items ? (!err && html`<${Loading} />`) : !items.length ? html`<${Empty} title=${t('ui.audit.empty')} />` : html`
    <div class="card"><div class="bd flush tbl-wrap"><table class="tbl dense">
      <thead><tr><th>${t('ui.audit.time')}</th><th>${t('ui.audit.actor')}</th><th>${t('ui.audit.action')}</th><th>${t('ui.audit.target')}</th><th>${t('ui.audit.area')}</th><th>${t('ui.audit.detail')}</th><th>${t('ui.audit.addr')}</th></tr></thead>
      <tbody>${items.map(e => html`<tr class=${e.risk === 'high' ? 'risk-high' : ''}>
        <td class="nowrap small">${fmtTime(e.at)}</td>
        <td class="ltr">${e.actor}</td>
        <td><code class=${e.action.endsWith('failed') ? 'crit-text' : ''}>${e.action}</code>${e.risk === 'high' ? html` <span class="tag warn">${t('ui.audit.high')}</span>` : ''}</td>
        <td class="ltr">${e.target || ''}</td>
        <td class="ltr small">${e.area || ''}</td>
        <td class="small ltr" style=${{ maxWidth: '420px', wordBreak: 'break-word' }}>${auditDetail(e.detail)}</td>
        <td class="ltr small muted">${e.addr || ''}</td>
      </tr>`)}</tbody>
    </table></div></div>
    ${more && html`<div><button class="btn" onClick=${() => load(items[items.length - 1].id)}>${t('ui.audit.more')}</button></div>`}`}
  </div>`;
}

const TABS = ['general', 'nodes', 'users', 'audit', 'scripts', 'tunables', 'tokens', 'secrets', 'keywords', 'system', 'registration', 'account'];

export function Settings({ tab, user }) {
  useDocTitle(t('ui.nav.settings'));
  let body;
  switch (tab) {
    case 'nodes': body = html`<${Nodes} embedded />`; break;
    case 'users': body = html`<${Users} />`; break;
    case 'audit': body = html`<${Audit} />`; break;
    case 'registration': body = html`<${Registration} />`; break;
    case 'scripts': body = html`<${CheckScripts} />`; break;
    case 'tunables': body = html`<${Tunables} />`; break;
    case 'tokens': body = html`<${Tokens} />`; break;
    case 'secrets': body = html`<${Secrets} />`; break;
    case 'keywords': body = html`<${Keywords} />`; break;
    case 'system': body = html`<${System} />`; break;
    case 'account': body = html`<${Account} user=${user} />`; break;
    default: body = html`<${General} />`;
  }
  return html`<div class="page">
    <div class="tabs">${TABS.map(k => html`<a href=${'#/settings/' + k} class=${tab === k ? 'on' : ''}>${t('ui.set.tab.' + k)}</a>`)}</div>
    ${body}
  </div>`;
}
