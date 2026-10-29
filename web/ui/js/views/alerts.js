// Alerting: notification channels, escalation policies and the contact directory.
import { html, useState } from '../lib.js';
import { t, tn, dur, has } from '../i18n.js';
import { get, post, del, put, ifMatch } from '../api.js';
import { ContactsTab, IssueDot, NewAddresses, addressKey } from './contacts.js';
import { useFetch, Loading, ErrorBox, Empty, Field, Modal, toast, toastError, errText, confirmDialog, useDocTitle } from '../ui.js';

function fieldLabel(k) {
  const key = 'ui.field.' + k;
  return has(key) ? t(key) : k;
}

function ChannelForm({ kinds, initial, onClose, onSaved }) {
  const editing = !!initial;
  const [kind, setKind] = useState(initial ? (initial.opts.preset ? 'webhook:' + initial.opts.preset : initial.kind) : 'feishu');
  const [name, setName] = useState(initial ? initial.name : '');
  const [opts, setOpts] = useState(initial ? { ...initial.opts } : {});
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);
  const [test, setTest] = useState(null);
  const [unknown, setUnknown] = useState(null); // addresses to offer for the directory
  const found = kinds.find(k => k.kind === kind);
  // Any channel may be limited to a time period ("!name" = outside it).
  const info = { ...found, fields: [...((found && found.fields) || []), { key: 'period', type: 'text', placeholder: 'office-hours' }] };
  const set = (k, v) => setOpts({ ...opts, [k]: v });
  const values = () => {
    const o = {};
    for (const f of info.fields) {
      const v = opts[f.key] !== undefined ? opts[f.key] : '';
      if (v !== '') o[f.key] = v;
    }
    return o;
  };
  const store = async () => {
    setBusy(true);
    setErr(null);
    try {
      await post('/channels', { name, kind, opts: values() }, ifMatch(initial && initial.etag));
      toast(t('ui.saved'));
      onSaved();
    } catch (e2) { setErr(e2); }
    setBusy(false);
  };
  // Recipients must be in the contact directory: new addresses are
  // offered for it before saving (saving without is allowed, flagged).
  const save = async (e) => {
    e.preventDefault();
    const key = addressKey(kind);
    const v = key && values()[key];
    if (v && !v.includes('${')) {
      try {
        const r = await post('/people/lookup', { kind, list: v });
        if (r.unknown.length) { setUnknown({ kind: r.kind, addresses: r.unknown }); return; }
      } catch { /* the check is advice: save anyway */ }
    }
    store();
  };
  const runTest = async () => {
    setTest({ busy: true });
    try {
      const k = kind.startsWith('webhook:') ? 'webhook' : kind;
      const o = values();
      if (kind.startsWith('webhook:')) o.preset = kind.slice(8);
      setTest({ res: await post('/channels/test', editing && Object.values(o).includes('***') ? { name } : { name: name || 'test', kind: k, opts: o }) });
    } catch (e2) { setTest({ err: e2 }); }
  };
  return html`<${Modal} title=${editing ? t('ui.ch.edit', { name }) : t('ui.ch.add')} onClose=${onClose}>
    <form class="form" onSubmit=${save}>
      <div class="two">
        <${Field} label=${t('ui.ch.kind')}>
          <select value=${kind} onChange=${(e) => { setKind(e.target.value); setTest(null); }} disabled=${editing}>
            <optgroup label=${t('ui.ch.native')}>${kinds.filter(k => !k.preset).map(k => html`<option value=${k.kind}>${k.label}</option>`)}</optgroup>
            <optgroup label=${t('ui.ch.presets')}>${kinds.filter(k => k.preset).map(k => html`<option value=${k.kind}>${k.label}</option>`)}</optgroup>
          </select><//>
        <${Field} label=${t('ui.name')}><input type="text" class="ltr" value=${name} onInput=${(e) => setName(e.target.value)} required disabled=${editing} pattern="[A-Za-z0-9][A-Za-z0-9_.\\-]{0,62}" /><//>
      </div>
      ${info.fields.map(f => {
        const v = opts[f.key] !== undefined ? opts[f.key] : (editing ? '' : f.default || '');
        const label = fieldLabel(f.key) + (f.required ? ' *' : '');
        if (f.type === 'select') {
          return html`<${Field} label=${label}><select value=${v} onChange=${(e) => set(f.key, e.target.value)}>${['', ...(f.options || [])].map(o => html`<option value=${o}>${o || '–'}</option>`)}</select><//>`;
        }
        if (f.type === 'textarea') {
          return html`<${Field} label=${label}><textarea class="ltr mono" rows="5" value=${v} placeholder=${f.placeholder || ''} onInput=${(e) => set(f.key, e.target.value)}></textarea><//>`;
        }
        const hint = f.type === 'secret' ? t('ui.ch.secret_hint') : f.type === 'email-list' ? t('ui.ch.email_list') : f.key === 'period' ? t('ui.ch.period_hint') : f.key === 'members' ? t('ui.ch.members_hint') : '';
        return html`<${Field} label=${label} hint=${hint}>
          <input type=${f.type === 'secret' ? 'password' : f.type === 'number' ? 'number' : 'text'} class="ltr" value=${v} required=${f.required && !editing}
            placeholder=${f.type === 'secret' && editing ? '***' : f.placeholder || ''} autocomplete="off" onInput=${(e) => set(f.key, e.target.value)} /><//>`;
      })}
      ${err && html`<div class="banner error">${errText(err)}</div>`}
      ${test && !test.busy && html`<div class=${'banner ' + (test.err || !test.res.ok ? 'error' : 'ok')}>
        ${test.err ? errText(test.err) : test.res.ok ? t('ui.ch.test_ok') : html`${t('ui.ch.test_failed')}: <span class="ltr">${test.res.error || (test.res.status ? 'HTTP ' + test.res.status : '')}</span>`}
      </div>`}
      ${initial && initial.issues && initial.issues.length > 0 && html`<div class="banner warning">${initial.issues.map(i => html`<div><span class="rdot"></span> <${IssueLine} i=${i} /></div>`)}</div>`}
      <div class="row">
        <button type="button" class="btn" onClick=${runTest} disabled=${test && test.busy}>${test && test.busy ? html`<span class="spin"></span>` : ''} ${t('ui.ch.test')}</button>
        <div class="spacer"></div>
        <button type="button" class="btn" onClick=${onClose}>${t('ui.cancel')}</button>
        <button class="btn primary" disabled=${busy}>${t('ui.save')}</button>
      </div>
    </form>
  <//>
  ${unknown && html`<${NewAddresses} kind=${unknown.kind} addresses=${unknown.addresses} onClose=${() => setUnknown(null)}
    onSkip=${() => { setUnknown(null); store(); }} onAdd=${() => { setUnknown(null); store(); }} />`}`;
}

function IssueLine({ i }) {
  return html`<span>${t('ui.dir.issue.' + i.code, { address: i.address || '', person: i.person ? '@' + i.person : '', n: i.count || 0 })}</span>`;
}

function ChannelsTab() {
  const list = useFetch(() => get('/channels'), []);
  const kinds = useFetch(() => get('/channel-kinds'), []);
  const [edit, setEdit] = useState(null);
  const remove = async (c) => {
    if (!await confirmDialog(t('ui.ch.delete_confirm', { name: c.name }), { danger: true, ok: t('ui.delete') })) return;
    try { await del('/channels/' + encodeURIComponent(c.name), ifMatch(c.etag)); list.reload(); } catch (e) { toastError(e); }
  };
  const test = async (c) => {
    try {
      const r = await post('/channels/test', { name: c.name });
      r.ok ? toast(t('ui.ch.test_ok')) : toast(t('ui.ch.test_failed') + ': ' + (r.error || ''), 'error');
    } catch (e) { toastError(e); }
  };
  if (!list.data || !kinds.data) return list.error ? html`<${ErrorBox} error=${list.error} />` : html`<${Loading} />`;
  return html`<div class="col" style=${{ gap: '12px' }}>
    <div class="row"><p class="muted small spacer" style=${{ margin: 0 }}>${t('ui.ch.intro')}</p><button class="btn primary" onClick=${() => setEdit({})}>${t('ui.ch.add')}</button></div>
    <div class="card"><div class="bd flush tbl-wrap">
      ${!list.data.length ? html`<${Empty} title=${t('ui.ch.none')}>${t('ui.ch.none_hint')}<//>` : html`<table class="tbl">
        <thead><tr><th>${t('ui.name')}</th><th>${t('ui.ch.kind')}</th><th>${t('ui.ch.used_by')}</th><th></th></tr></thead>
        <tbody>${list.data.map(c => html`<tr>
          <td class="name ltr">${c.name} <${IssueDot} issues=${c.issues} /></td>
          <td>${(kinds.data.find(k => k.kind === (c.opts.preset ? 'webhook:' + c.opts.preset : c.kind)) || { label: c.kind }).label}</td>
          <td class="small">${c.used_by.length ? c.used_by.join(', ') : html`<span class="warn-text">${t('ui.ch.unused')}</span>`}</td>
          <td class="right nowrap">
            <button class="btn sm" onClick=${() => test(c)}>${t('ui.ch.test')}</button>
            <button class="btn sm" onClick=${() => setEdit(c)}>${t('ui.edit')}</button>
            <button class="btn sm danger" onClick=${() => remove(c)}>${t('ui.delete')}</button>
          </td>
        </tr>`)}</tbody>
      </table>`}
    </div></div>
    ${edit && html`<${ChannelForm} kinds=${kinds.data} initial=${edit.name ? edit : null} onClose=${() => setEdit(null)} onSaved=${() => { setEdit(null); list.reload(); }} />`}
  </div>`;
}

function secs(v) { return v ? (v % 3600 === 0 ? v / 3600 + 'h' : v % 60 === 0 ? v / 60 + 'm' : v + 's') : ''; }

function PolicyForm({ initial, channels, onClose, onSaved }) {
  const p = initial || {};
  const [f, setF] = useState({
    name: p.name || '', chain: p.chain || [], mode: p.mode || 'fallback', repeat: secs(p.repeat_s), max: p.max != null ? String(p.max) : '',
    delay: secs(p.delay_s), reopen: secs(p.reopen_window_s), ackexp: secs(p.ack_expire_s), recovery: p.recovery !== false, degraded: !!p.notify_degraded,
    escalate: p.escalate || '',
  });
  const [err, setErr] = useState(null);
  const [busy, setBusy] = useState(false);
  const set = (k) => (e) => setF({ ...f, [k]: e.target.type === 'checkbox' ? e.target.checked : e.target.value });
  const toggle = (c) => setF({ ...f, chain: f.chain.includes(c) ? f.chain.filter(x => x !== c) : [...f.chain, c] });
  const move = (i, d) => { const ch = f.chain.slice(); [ch[i], ch[i + d]] = [ch[i + d], ch[i]]; setF({ ...f, chain: ch }); };
  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    setErr(null);
    const opts = { mode: f.mode, repeat: f.repeat, max: f.max, delay: f.delay, 'reopen-window': f.reopen, 'ack-expire': f.ackexp, escalate: f.escalate,
      recovery: f.recovery ? '' : 'no', 'notify-degraded': f.degraded ? 'yes' : '' };
    try {
      await post('/policies', { name: f.name, chain: f.chain, opts }, ifMatch(p.etag));
      toast(t('ui.saved'));
      onSaved();
    } catch (e2) { setErr(e2); }
    setBusy(false);
  };
  return html`<${Modal} title=${initial && initial.name ? t('ui.pol.edit', { name: p.name }) : t('ui.pol.add')} onClose=${onClose}>
    <form class="form" onSubmit=${save}>
      <${Field} label=${t('ui.name')}><input type="text" class="ltr" value=${f.name} onInput=${set('name')} required disabled=${!!(initial && initial.name && !initial.implicit)} /><//>
      <${Field} label=${t('ui.pol.chain')} hint=${t('ui.pol.chain_hint')}>
        <div class="col">
          ${f.chain.map((c, i) => html`<div class="row nowrap"><b class="mono">${i + 1}.</b><span class="spacer ltr">${c}</span>
            <button type="button" class="btn sm ghost" disabled=${i === 0} onClick=${() => move(i, -1)}>↑</button>
            <button type="button" class="btn sm ghost" disabled=${i === f.chain.length - 1} onClick=${() => move(i, 1)}>↓</button>
            <button type="button" class="btn sm ghost" onClick=${() => toggle(c)}>✕</button></div>`)}
          <div class="chips">${channels.filter(c => !f.chain.includes(c.name)).map(c => html`<button type="button" class="chip" onClick=${() => toggle(c.name)}>+ ${c.name}</button>`)}</div>
        </div><//>
      <div class="two">
        <${Field} label=${t('ui.pol.mode')}><select value=${f.mode} onChange=${set('mode')}>
          <option value="fallback">${t('ui.pol.mode.fallback')}</option><option value="all">${t('ui.pol.mode.all')}</option></select><//>
        <${Field} label=${t('ui.pol.max')} hint=${t('ui.pol.max_hint')}><input type="number" min="0" max="1000" value=${f.max} onInput=${set('max')} placeholder="3" /><//>
        <${Field} label=${t('ui.pol.repeat')} hint=${t('ui.pol.repeat_hint')}><input type="text" class="ltr" value=${f.repeat} onInput=${set('repeat')} placeholder="30m" /><//>
        <${Field} label=${t('ui.pol.delay')} hint=${t('ui.pol.delay_hint')}><input type="text" class="ltr" value=${f.delay} onInput=${set('delay')} placeholder="0s" /><//>
        <${Field} label=${t('ui.pol.reopen')} hint=${t('ui.pol.reopen_hint')}><input type="text" class="ltr" value=${f.reopen} onInput=${set('reopen')} placeholder="10m" /><//>
        <${Field} label=${t('ui.pol.ack_expire')} hint=${t('ui.pol.ack_expire_hint')}><input type="text" class="ltr" value=${f.ackexp} onInput=${set('ackexp')} placeholder="0s" /><//>
      </div>
      <${Field} label=${t('ui.pol.escalate')} hint=${t('ui.pol.escalate_hint')}><input type="text" class="ltr" value=${f.escalate} onInput=${set('escalate')} placeholder="30m:oncall, 2h:manager+sms" /><//>
      <label class="switch"><input type="checkbox" checked=${f.recovery} onChange=${set('recovery')} /> ${t('ui.pol.recovery')}</label>
      <label class="switch"><input type="checkbox" checked=${f.degraded} onChange=${set('degraded')} /> ${t('ui.pol.degraded')}</label>
      ${err && html`<div class="banner error">${errText(err)}</div>`}
      <div class="row"><div class="spacer"></div>
        <button type="button" class="btn" onClick=${onClose}>${t('ui.cancel')}</button>
        <button class="btn primary" disabled=${busy || !f.chain.length}>${t('ui.save')}</button></div>
    </form>
  <//>`;
}

function Predict({ policy, onClose }) {
  const mons = useFetch(() => get('/monitors?q=' + encodeURIComponent('policy:' + policy)), [policy]);
  const [res, setRes] = useState(null);
  const [minutes, setMinutes] = useState(120);
  const run = async (monitor) => {
    try { setRes(await post('/policies/' + encodeURIComponent(policy) + '/predict', { monitor, minutes: +minutes })); } catch (e) { toastError(e); }
  };
  return html`<${Modal} title=${t('ui.pol.predict_title', { name: policy })} onClose=${onClose}>
    <div class="form">
      <p class="small muted">${t('ui.pol.predict_hint')}</p>
      <div class="row nowrap">
        <select id="pm">${(mons.data ? mons.data.items : []).map(m => html`<option value=${m.name}>${m.title}</option>`)}</select>
        <input type="number" value=${minutes} min="1" max="10080" onInput=${(e) => setMinutes(e.target.value)} style=${{ width: '110px' }} aria-label=${t('ui.minutes')} />
        <button class="btn primary" onClick=${() => run(document.getElementById('pm').value)}>${t('ui.pol.predict')}</button>
      </div>
      ${res && (res.timeline.length ? html`<table class="tbl"><tbody>${res.timeline.map(s => html`<tr>
        <td class="mono nowrap">+${dur(s.at_s)}</td><td>${t('msg.kind.' + s.kind)}</td><td class="small">${s.max ? `${s.seq}/${s.max}` : s.seq || ''}</td><td class="ltr">${s.channel}</td></tr>`)}</tbody></table>`
        : html`<p class="muted">${t('ui.pol.predict_none')}</p>`)}
    </div>
  <//>`;
}

function PoliciesTab() {
  const list = useFetch(() => get('/policies'), []);
  const ch = useFetch(() => get('/channels'), []);
  const [edit, setEdit] = useState(null);
  const [predict, setPredict] = useState(null);
  const test = async (p) => {
    try { await post('/policies/' + encodeURIComponent(p.name) + '/test', {}); toast(t('ui.pol.test_sent')); } catch (e) { toastError(e); }
  };
  if (!list.data || !ch.data) return list.error ? html`<${ErrorBox} error=${list.error} />` : html`<${Loading} />`;
  return html`<div class="col" style=${{ gap: '12px' }}>
    <div class="row"><p class="muted small spacer" style=${{ margin: 0 }}>${t('ui.pol.intro')}</p>
      <button class="btn primary" onClick=${() => setEdit({})} disabled=${!ch.data.length}>${t('ui.pol.add')}</button></div>
    <div class="card"><div class="bd flush tbl-wrap"><table class="tbl">
      <thead><tr><th>${t('ui.name')}</th><th>${t('ui.pol.chain')}</th><th>${t('ui.pol.rules')}</th><th class="right">${t('ui.monitors')}</th><th></th></tr></thead>
      <tbody>${list.data.map(p => html`<tr>
        <td><span class="name ltr">${p.name}</span>${p.implicit && html` <span class="tag">${t('ui.pol.implicit')}</span>`}
          ${p.issue_channels && html` <${IssueDot} issues=${p.issue_channels} text=${t('ui.dir.policy_dot', { channels: p.issue_channels.join(', ') })} />`}</td>
        <td class="ltr">${p.chain.length ? p.chain.join(p.mode === 'all' ? ' + ' : ' → ') : html`<span class="warn-text">${t('ui.pol.no_chain')}</span>`}</td>
        <td class="small muted">${[
          p.max ? tn('ui.pol.max_n', p.max) : t('ui.pol.unlimited'),
          p.repeat_s ? t('ui.pol.every', { d: dur(p.repeat_s) }) : '',
          p.delay_s ? t('ui.pol.after', { d: dur(p.delay_s) }) : '',
          p.recovery ? t('ui.pol.recovery_short') : '',
        ].filter(Boolean).join(' · ')}</td>
        <td class="right">${p.used_by_count}</td>
        <td class="right nowrap">
          ${p.chain.length > 0 && html`<button class="btn sm" onClick=${() => test(p)}>${t('ui.pol.test')}</button>`}
          <button class="btn sm" onClick=${() => setPredict(p.name)}>${t('ui.pol.predict')}</button>
          <button class="btn sm" onClick=${() => setEdit(p)}>${t('ui.edit')}</button>
        </td>
      </tr>`)}</tbody>
    </table></div></div>
    ${edit && html`<${PolicyForm} initial=${edit.name ? edit : null} channels=${ch.data} onClose=${() => setEdit(null)} onSaved=${() => { setEdit(null); list.reload(); }} />`}
    ${predict && html`<${Predict} policy=${predict} onClose=${() => setPredict(null)} />`}
  </div>`;
}

export function Alerts({ tab }) {
  useDocTitle(t('ui.nav.alerts'));
  return html`<div class="page">
    <div class="tabs">
      <a href="#/alerts/channels" class=${tab === 'channels' ? 'on' : ''}>${t('ui.ch.tab')}</a>
      <a href="#/alerts/policies" class=${tab === 'policies' ? 'on' : ''}>${t('ui.pol.tab')}</a>
      <a href="#/alerts/contacts" class=${tab === 'contacts' ? 'on' : ''}>${t('ui.dir.tab')}</a>
    </div>
    ${tab === 'policies' ? html`<${PoliciesTab} />` : tab === 'contacts' ? html`<${ContactsTab} />` : html`<${ChannelsTab} />`}
  </div>`;
}

export { put };
