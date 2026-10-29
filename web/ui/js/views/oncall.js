// On call: who is on call now in each rotation, the next shifts, the
// stand-ins arranged, and how fast incidents were acknowledged and resolved.
// A rotation is a contact group channel with rotate= set: it notifies the
// member on call, and the others only if that member cannot be reached.
import { html, useState } from '../lib.js';
import { t, dur, fmtNum } from '../i18n.js';
import { get, post, del } from '../api.js';
import { useFetch, useLive, Loading, ErrorBox, Empty, Field, Modal, Card, toast, toastError, errText, confirmDialog, useDocTitle } from '../ui.js';

const pad = (n) => String(n).padStart(2, '0');
function toLocal(d) {
  d = new Date(d);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
function when(d) {
  d = new Date(d);
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', weekday: 'short' }) + ' ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}
const DAYS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];
// handoffText shows "mon 09:00" with the day in the UI language.
function handoffText(h) {
  const [d, time] = h.split(' ');
  return time && DAYS.includes(d.slice(0, 3).toLowerCase()) ? t('ui.oc.day.' + d.slice(0, 3).toLowerCase()) + ' ' + time : h;
}

// who names a member: the people its channel reaches, else the channel.
function who(rot, member) {
  const m = rot.members.find(x => x.channel === member);
  return m && m.people.length ? m.people.join(', ') : member;
}

function NewRotation({ channels, onClose, onSaved }) {
  const today = new Date();
  const [f, setF] = useState({ name: 'oncall', members: [], every: 'weekly', day: 'mon', time: '09:00',
    start: `${today.getFullYear()}-${pad(today.getMonth() + 1)}-${pad(today.getDate())}` });
  const [err, setErr] = useState(null);
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
  const toggle = (name) => setF({ ...f, members: f.members.includes(name) ? f.members.filter(x => x !== name) : [...f.members, name] });
  const save = async (e) => {
    e.preventDefault();
    setErr(null);
    const opts = { members: f.members.join(','), rotate: f.every, handoff: (f.every === 'weekly' ? f.day + ' ' : '') + f.time, start: f.start };
    try {
      await post('/channels', { name: f.name.trim(), kind: 'group', opts });
      toast(t('ui.saved'));
      onSaved();
    } catch (e2) { setErr(e2); }
  };
  const usable = channels.filter(c => c.kind !== 'group');
  return html`<${Modal} title=${t('ui.oc.new')} onClose=${onClose}>
    <form class="form" onSubmit=${save}>
      <${Field} label=${t('ui.oc.name')} hint=${t('ui.oc.name_hint')}><input type="text" class="ltr" value=${f.name} onInput=${set('name')} required pattern="[A-Za-z0-9][A-Za-z0-9._\\-]*" /><//>
      <${Field} label=${t('ui.oc.members')} hint=${t('ui.oc.members_hint')}>
        ${usable.length ? html`<div class="col" style=${{ gap: '4px' }}>${usable.map(c => html`<label class="switch">
          <input type="checkbox" checked=${f.members.includes(c.name)} onChange=${() => toggle(c.name)} />
          <span class="ltr">${c.name}</span> <span class="small muted">${c.kind}${f.members.includes(c.name) ? ' · #' + (f.members.indexOf(c.name) + 1) : ''}</span></label>`)}</div>`
        : html`<p class="small warn-text">${t('ui.oc.no_channels')}</p>`}<//>
      <div class="two">
        <${Field} label=${t('ui.oc.every')}><select value=${f.every} onChange=${set('every')}>
          <option value="daily">${t('ui.oc.daily')}</option><option value="weekly">${t('ui.oc.weekly')}</option></select><//>
        <${Field} label=${t('ui.oc.handoff')}><div class="row nowrap">
          ${f.every === 'weekly' && html`<select value=${f.day} onChange=${set('day')} style=${{ width: 'auto' }}>${DAYS.map(d => html`<option value=${d}>${t('ui.oc.day.' + d)}</option>`)}</select>`}
          <input type="time" value=${f.time} onInput=${set('time')} required /></div><//>
      </div>
      <${Field} label=${t('ui.oc.start')} hint=${t('ui.oc.start_hint')}><input type="date" value=${f.start} onInput=${set('start')} required /><//>
      ${err && html`<div class="banner error">${errText(err)}</div>`}
      <div class="row"><div class="spacer"></div>
        <button type="button" class="btn" onClick=${onClose}>${t('ui.cancel')}</button>
        <button class="btn primary" disabled=${f.members.length < 2}>${t('ui.save')}</button></div>
    </form>
  <//>`;
}

function StandIn({ rot, onClose, onSaved }) {
  const now = new Date();
  const end = rot.scheduled && new Date(rot.scheduled.until) > now ? new Date(rot.scheduled.until) : new Date(now.getTime() + 86400e3);
  const others = rot.members.filter(m => m.channel !== rot.now);
  const [f, setF] = useState({ member: (others[0] || rot.members[0]).channel, from: toLocal(now), until: toLocal(end), note: '' });
  const [err, setErr] = useState(null);
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
  const save = async (e) => {
    e.preventDefault();
    setErr(null);
    try {
      await post('/oncall/overrides', { rotation: rot.name, member: f.member, from: new Date(f.from).toISOString(), until: new Date(f.until).toISOString(), note: f.note.trim() });
      toast(t('ui.saved'));
      onSaved();
    } catch (e2) { setErr(e2); }
  };
  return html`<${Modal} title=${t('ui.oc.standin_title', { name: rot.name })} onClose=${onClose}>
    <form class="form" onSubmit=${save}>
      <p class="small muted" style=${{ marginTop: 0 }}>${t('ui.oc.standin_intro')}</p>
      <${Field} label=${t('ui.oc.standin_who')}><select value=${f.member} onChange=${set('member')}>
        ${rot.members.map(m => html`<option value=${m.channel}>${who(rot, m.channel)}${m.people.length ? ' (' + m.channel + ')' : ''}</option>`)}</select><//>
      <div class="two">
        <${Field} label=${t('ui.oc.from')}><input type="datetime-local" value=${f.from} onInput=${set('from')} required /><//>
        <${Field} label=${t('ui.oc.until')}><input type="datetime-local" value=${f.until} onInput=${set('until')} required /><//>
      </div>
      <${Field} label=${t('ui.oc.note')}><input type="text" value=${f.note} onInput=${set('note')} placeholder=${t('ui.oc.note_ph')} /><//>
      ${err && html`<div class="banner error">${errText(err)}</div>`}
      <div class="row"><div class="spacer"></div>
        <button type="button" class="btn" onClick=${onClose}>${t('ui.cancel')}</button>
        <button class="btn primary">${t('ui.save')}</button></div>
    </form>
  <//>`;
}

function Rotation({ rot, reload }) {
  const [standIn, setStandIn] = useState(false);
  const cancel = async (o) => {
    if (!await confirmDialog(t('ui.oc.cancel_confirm', { name: who(rot, o.member) }), { danger: true, ok: t('ui.oc.cancel') })) return;
    try { await del('/oncall/overrides/' + o.id); reload(); } catch (e) { toastError(e); }
  };
  const until = rot.stand_in ? rot.stand_in.until : rot.scheduled.until;
  return html`<div class="card oc-rot">
    <div class="hd"><h3 class="ltr">${rot.name}</h3>
      <span class="small muted">${t('ui.oc.plan', { every: ['daily', 'weekly'].includes(rot.rotate) ? t('ui.oc.' + rot.rotate) : rot.rotate, handoff: handoffText(rot.handoff) })}</span>
      <div class="spacer"></div>
      <button class="btn sm" onClick=${() => setStandIn(true)}>${t('ui.oc.standin')}</button>
      <a class="btn sm" href="#/alerts/channels">${t('ui.edit')}</a></div>
    <div class="bd">
      <div class="oc-now">
        <span class="small muted">${t('ui.oc.now')}</span>
        <b>${who(rot, rot.now)}</b>
        <span class="small muted">${t('ui.oc.until_when', { when: when(until) })}</span>
        ${rot.stand_in && html`<span class="tag warn">${t('ui.oc.standing_in', { name: who(rot, rot.scheduled.member) })}</span>`}
      </div>
      ${!rot.policies.length && html`<div class="banner warning small">${t('ui.oc.unused')}</div>`}
      ${rot.policies.length > 0 && html`<p class="small muted" style=${{ margin: '8px 0 0' }}>${t('ui.oc.used_by', { policies: rot.policies.join(', ') })}</p>`}
    </div>
    <div class="bd flush tbl-wrap"><table class="tbl">
      <thead><tr><th>${t('ui.oc.shift')}</th><th>${t('ui.oc.member')}</th><th>${t('ui.oc.standins')}</th></tr></thead>
      <tbody>${rot.shifts.map((s, i) => html`<tr class=${i === 0 ? 'picked' : ''}>
        <td class="nowrap small">${when(s.from)} – ${when(s.until)}</td>
        <td>${who(rot, s.member)} <span class="small muted ltr">${s.member}</span></td>
        <td class="small">${s.stand_ins.map(o => html`<div>${who(rot, o.member)} · ${when(o.from)} – ${when(o.until)}</div>`)}</td>
      </tr>`)}</tbody>
    </table></div>
    ${rot.overrides.length > 0 && html`<div class="bd"><h4 class="small" style=${{ margin: '0 0 6px' }}>${t('ui.oc.standins')}</h4>
      ${rot.overrides.map(o => html`<div class="row small" style=${{ padding: '4px 0' }}>
        <span class="spacer"><b>${who(rot, o.member)}</b> · ${when(o.from)} – ${when(o.until)}${o.note ? ' · ' + o.note : ''} <span class="muted">· ${o.by}</span></span>
        <button class="btn sm" onClick=${() => cancel(o)}>${t('ui.oc.cancel')}</button></div>`)}</div>`}
    ${standIn && html`<${StandIn} rot=${rot} onClose=${() => setStandIn(false)} onSaved=${() => { setStandIn(false); reload(); }} />`}
  </div>`;
}

function secs(v, n) { return n ? dur(v) : '–'; }

function Stats({ stats, rots }) {
  const days = Math.round((new Date(stats.to) - new Date(stats.from)) / 86400e3);
  const a = stats.all;
  const rotOf = (name) => rots.find(r => r.name === name);
  return html`<${Card} title=${t('ui.oc.stats', { n: fmtNum(days) })} flush>
    <div class="oc-kpis">
      <div><b>${fmtNum(a.incidents)}</b><span>${t('ui.oc.incidents')}</span></div>
      <div><b>${secs(a.mtta_s, a.acked)}</b><span>MTTA · ${t('ui.oc.mtta')}</span></div>
      <div><b>${secs(a.mttr_s, a.resolved)}</b><span>MTTR · ${t('ui.oc.mttr')}</span></div>
    </div>
    ${stats.members.length > 0 && html`<div class="tbl-wrap"><table class="tbl">
      <thead><tr><th>${t('ui.oc.member')}</th><th class="right">${t('ui.oc.incidents')}</th><th class="right">${t('ui.oc.acked')}</th><th class="right">MTTA</th><th class="right">MTTR</th></tr></thead>
      <tbody>${stats.members.map(m => html`<tr>
        <td>${rotOf(m.rotation) ? who(rotOf(m.rotation), m.member) : m.member} <span class="small muted ltr">${m.rotation}</span></td>
        <td class="right">${fmtNum(m.incidents)}</td><td class="right">${fmtNum(m.acked)}</td>
        <td class="right">${secs(m.mtta_s, m.acked)}</td><td class="right">${secs(m.mttr_s, m.resolved)}</td>
      </tr>`)}</tbody></table></div>`}
    <p class="small muted" style=${{ margin: '8px 16px 12px' }}>${t('ui.oc.stats_hint')}</p>
  <//>`;
}

export function OnCall() {
  useDocTitle(t('ui.nav.oncall'));
  const d = useFetch(() => get('/oncall'), []);
  const chans = useFetch(() => get('/channels'), []);
  const [creating, setCreating] = useState(false);
  useLive(['oncall', 'serial', 'directory', 'incident'], d.reload, 800);
  if (!d.data) return html`<div class="page">${d.error ? html`<${ErrorBox} error=${d.error} retry=${d.reload} />` : html`<${Loading} />`}</div>`;
  const rots = d.data.rotations;
  return html`<div class="page">
    <div class="row"><p class="muted small spacer" style=${{ margin: 0 }}>${t('ui.oc.intro')}</p>
      <button class="btn primary" onClick=${() => setCreating(true)} disabled=${!chans.data}>${t('ui.oc.new')}</button></div>
    ${!rots.length ? html`<div class="card"><${Empty} title=${t('ui.oc.none')}><p>${t('ui.oc.none_hint')}</p>
        <button class="btn primary" onClick=${() => setCreating(true)} disabled=${!chans.data}>${t('ui.oc.new')}</button><//></div>`
      : rots.map(r => html`<${Rotation} key=${r.name} rot=${r} reload=${d.reload} />`)}
    <${Stats} stats=${d.data.stats} rots=${rots} />
    ${creating && chans.data && html`<${NewRotation} channels=${chans.data} onClose=${() => setCreating(false)} onSaved=${() => { setCreating(false); d.reload(); chans.reload(); }} />`}
  </div>`;
}
