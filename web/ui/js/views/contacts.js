// Contacts: the directory of people who may receive alerts. Channels are
// checked against it: addresses of people who left or that keep bouncing
// are not sent to, unknown ones get a red dot.
import { html, useState } from '../lib.js';
import { t, tn } from '../i18n.js';
import { get, post, put, del } from '../api.js';
import { useFetch, useLive, Loading, ErrorBox, Empty, Ago, Field, Modal, toast, toastError, errText, confirmDialog } from '../ui.js';

const ADDRESS_KEYS = { email: 'to' };
// addressKey is the channel option holding people's addresses, if any.
export function addressKey(kind) {
  if (ADDRESS_KEYS[kind]) return ADDRESS_KEYS[kind];
  return /-(sms|voice)$/.test(kind) ? 'phones' : '';
}

export function issueText(i) {
  return t('ui.dir.issue.' + i.code, { address: i.address || '', person: i.person ? '@' + i.person : '', n: i.count || 0 });
}

// IssueDot is the red dot of a channel or policy with recipient problems;
// its title says what is wrong.
export function IssueDot({ issues, text }) {
  if (!issues || !issues.length) return null;
  const title = text || issues.map(issueText).join('\n');
  return html`<span class="rdot" title=${title} aria-label=${title}></span>`;
}

function handleFrom(s) {
  const v = String(s).toLowerCase().split('@')[0].replace(/[^a-z0-9._-]+/g, '-').replace(/^[-._]+|[-._]+$/g, '');
  return v || 'contact';
}

function list(v) { return String(v || '').split(',').map(x => x.trim()).filter(Boolean); }

function PersonForm({ initial, onClose, onSaved }) {
  const p = initial || {};
  const [f, setF] = useState({ handle: p.handle || '', name: p.name || '', emails: (p.emails || []).join(', '), phones: (p.phones || []).join(', '),
    username: p.username || '', note: p.note || '' });
  const [err, setErr] = useState(null);
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
  const save = async (e) => {
    e.preventDefault();
    setErr(null);
    const body = { handle: f.handle.trim() || handleFrom(f.name || f.emails), name: f.name.trim(), emails: list(f.emails), phones: list(f.phones),
      username: f.username.trim(), note: f.note.trim() };
    try {
      if (p.id) await put('/people/' + p.id, body);
      else await post('/people', body);
      toast(t('ui.saved'));
      onSaved();
    } catch (e2) { setErr(e2); }
  };
  return html`<${Modal} title=${p.id ? t('ui.dir.edit', { name: p.name }) : t('ui.dir.add')} onClose=${onClose}>
    <form class="form" onSubmit=${save}>
      <div class="two">
        <${Field} label=${t('ui.dir.name')}><input type="text" value=${f.name} onInput=${set('name')} required /><//>
        <${Field} label=${t('ui.dir.handle')} hint=${t('ui.dir.handle_hint')}><input type="text" class="ltr" value=${f.handle} onInput=${set('handle')} placeholder=${handleFrom(f.name || f.emails)} pattern="[a-z0-9][a-z0-9._\\-]{0,62}" /><//>
      </div>
      <${Field} label=${t('ui.dir.emails')} hint=${t('ui.dir.list_hint')}><input type="text" class="ltr" value=${f.emails} onInput=${set('emails')} placeholder="li@example.com" /><//>
      <${Field} label=${t('ui.dir.phones')} hint=${t('ui.dir.list_hint')}><input type="text" class="ltr" value=${f.phones} onInput=${set('phones')} placeholder="+8613800000000" /><//>
      <div class="two">
        <${Field} label=${t('ui.dir.username')} hint=${t('ui.dir.username_hint')}><input type="text" class="ltr" value=${f.username} onInput=${set('username')} /><//>
        <${Field} label=${t('ui.dir.note')}><input type="text" value=${f.note} onInput=${set('note')} /><//>
      </div>
      ${err && html`<div class="banner error">${errText(err)}</div>`}
      <div class="row"><div class="spacer"></div>
        <button type="button" class="btn" onClick=${onClose}>${t('ui.cancel')}</button>
        <button class="btn primary">${t('ui.save')}</button></div>
    </form>
  <//>`;
}

export function ContactsTab() {
  const d = useFetch(() => get('/people'), []);
  const [edit, setEdit] = useState(null);
  useLive(['directory'], d.reload, 500);
  if (!d.data) return d.error ? html`<${ErrorBox} error=${d.error} retry=${d.reload} />` : html`<${Loading} />`;
  const { people, bounces, unknown, threshold } = d.data;
  const nUnknown = (unknown.email || []).length + (unknown.phone || []).length;
  const importAll = async () => {
    try {
      const r = await post('/people/import', {});
      toast(tn('ui.dir.imported', r.created));
      if (r.errors.length) toast(r.errors.join('; '), 'error');
      d.reload();
    } catch (e) { toastError(e); }
  };
  const setLeft = async (p, left) => {
    if (left && !await confirmDialog(t('ui.dir.left_confirm', { name: p.name }), { danger: true, ok: t('ui.dir.mark_left') })) return;
    try { await post('/people/' + p.id + '/left', { left }); d.reload(); } catch (e) { toastError(e); }
  };
  const remove = async (p) => {
    if (!await confirmDialog(t('ui.dir.delete_confirm', { name: p.name }), { danger: true, ok: t('ui.delete') })) return;
    try { await del('/people/' + p.id); d.reload(); } catch (e) { toastError(e); }
  };
  const clearBounce = async (b) => {
    try { await del('/bounces?kind=' + encodeURIComponent(b.kind) + '&address=' + encodeURIComponent(b.address)); d.reload(); } catch (e) { toastError(e); }
  };
  return html`<div class="col" style=${{ gap: '12px' }}>
    <div class="row"><p class="muted small spacer" style=${{ margin: 0 }}>${t('ui.dir.intro')}</p>
      <button class="btn primary" onClick=${() => setEdit({})}>${t('ui.dir.add')}</button></div>
    ${nUnknown > 0 && html`<div class="banner warning row">
      <span class="rdot"></span><span class="spacer">${tn('ui.dir.unknown_banner', nUnknown)}
        <span class="small muted ltr"> ${[...(unknown.email || []), ...(unknown.phone || [])].slice(0, 6).join(', ')}${nUnknown > 6 ? ' …' : ''}</span></span>
      <button class="btn sm primary" onClick=${importAll}>${t('ui.dir.import')}</button></div>`}
    <div class="card"><div class="bd flush tbl-wrap">
      ${!people.length ? html`<${Empty} title=${t('ui.dir.none')}>${t('ui.dir.none_hint')}<//>` : html`<table class="tbl">
        <thead><tr><th>${t('ui.dir.name')}</th><th>${t('ui.dir.emails')}</th><th>${t('ui.dir.phones')}</th><th>${t('ui.dir.channels')}</th><th></th></tr></thead>
        <tbody>${people.map(p => html`<tr class=${p.left_at ? 'muted' : ''}>
          <td><span class="name">${p.name}</span> <span class="small muted ltr">@${p.handle}</span>
            ${p.left_at && html` <span class="tag">${t('ui.dir.left')}</span>`}
            ${p.username && html`<div class="sub small ltr">${t('ui.dir.login')}: ${p.username}</div>`}</td>
          <td class="small ltr">${p.emails.join(', ')}</td>
          <td class="small ltr">${p.phones.join(', ')}</td>
          <td class="small">${p.channels.join(', ')}</td>
          <td class="right nowrap">
            <button class="btn sm" onClick=${() => setEdit(p)}>${t('ui.edit')}</button>
            ${p.left_at ? html`<button class="btn sm" onClick=${() => setLeft(p, false)}>${t('ui.dir.back')}</button>`
              : html`<button class="btn sm" onClick=${() => setLeft(p, true)}>${t('ui.dir.mark_left')}</button>`}
            <button class="btn sm danger" onClick=${() => remove(p)}>${t('ui.delete')}</button>
          </td>
        </tr>`)}</tbody>
      </table>`}
    </div></div>
    ${bounces.length > 0 && html`<div class="card"><div class="hd"><h3>${t('ui.dir.bounces')}</h3></div>
      <div class="bd"><p class="small muted" style=${{ marginTop: 0 }}>${threshold > 0 ? tn('ui.dir.bounce_rule', threshold) : t('ui.dir.bounce_off')}</p></div>
      <div class="bd flush tbl-wrap"><table class="tbl">
        <thead><tr><th>${t('ui.dir.address')}</th><th class="right">${t('ui.dir.count')}</th><th>${t('ui.dir.reason')}</th><th>${t('ui.dir.last')}</th><th></th></tr></thead>
        <tbody>${bounces.map(b => html`<tr>
          <td class="ltr">${threshold > 0 && b.count >= threshold && html`<span class="rdot" title=${t('ui.dir.skipped')}></span> `}${b.address}</td>
          <td class="right">${b.count}</td>
          <td class="small muted ltr">${b.reason}</td>
          <td class="small"><${Ago} at=${b.last_at} /></td>
          <td class="right"><button class="btn sm" onClick=${() => clearBounce(b)} title=${t('ui.dir.clear_hint')}>${t('ui.dir.clear')}</button></td>
        </tr>`)}</tbody>
      </table></div></div>`}
    ${edit && html`<${PersonForm} initial=${edit.id ? edit : null} onClose=${() => setEdit(null)} onSaved=${() => { setEdit(null); d.reload(); }} />`}
  </div>`;
}

// NewAddresses asks, before a channel is saved, whether the addresses the
// directory does not know should be added to it (each with a name).
export function NewAddresses({ kind, addresses, onAdd, onSkip, onClose }) {
  const [names, setNames] = useState(addresses.map(a => a.includes('@') ? a.split('@')[0] : ''));
  const [err, setErr] = useState(null);
  const [busy, setBusy] = useState(false);
  const add = async () => {
    setBusy(true);
    setErr(null);
    try {
      for (let i = 0; i < addresses.length; i++) {
        const a = addresses[i].replace(/^.*<([^>]+)>\s*$/, '$1').trim();
        const body = { name: names[i].trim() || a, handle: handleFrom(names[i].trim() || a), emails: kind === 'email' ? [a] : [], phones: kind === 'phone' ? [a] : [] };
        await post('/people', body);
      }
      onAdd();
    } catch (e) { setErr(e); setBusy(false); }
  };
  return html`<${Modal} title=${t('ui.dir.new_title')} onClose=${onClose}>
    <div class="col" style=${{ gap: '10px' }}>
      <p class="small" style=${{ margin: 0 }}><span class="rdot"></span> ${t('ui.dir.new_intro')}</p>
      ${addresses.map((a, i) => html`<div class="two">
        <div class="ltr" style=${{ alignSelf: 'center' }}>${a}</div>
        <input type="text" value=${names[i]} placeholder=${t('ui.dir.name')} onInput=${(e) => { const n = names.slice(); n[i] = e.target.value; setNames(n); }} />
      </div>`)}
      ${err && html`<div class="banner error">${errText(err)}</div>`}
      <div class="row"><div class="spacer"></div>
        <button type="button" class="btn" onClick=${onSkip}>${t('ui.dir.save_anyway')}</button>
        <button type="button" class="btn primary" onClick=${add} disabled=${busy}>${t('ui.dir.add_and_save')}</button></div>
    </div>
  <//>`;
}
