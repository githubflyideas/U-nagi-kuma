// Monitor list: dense table with heartbeat bars, or the honeycomb, with
// search and status chips. A row opens the monitor's page; ticked rows can
// be checked now, paused, resumed or deleted together.
import { html, useState, useEffect, useMemo } from '../lib.js';
import { t, tn, fmtNum } from '../i18n.js';
import { get, post } from '../api.js';
import { onLive } from '../live.js';
import { useFetch, useLive, Loading, ErrorBox, Empty, Pill, Beats, Ago, Seg, pct, msText, reasonText, enc, go, toast, toastError, confirmDialog, useDocTitle } from '../ui.js';
import { QuickAdd } from './editor.js';
import { Hive, Legend } from './overview.js';

function savedView() {
  try { return localStorage.getItem('unagikuma.monview') || 'list'; } catch { return 'list'; }
}

const CHIPS = ['critical', 'degraded', 'unknown', 'ok', 'pending', 'paused'];

function splitStatus(q) {
  const m = q.match(/(^|\s)status:(\w+)/);
  return { status: m ? m[2] : '', rest: q.replace(/(^|\s)status:\w+/, '').trim() };
}

export function Monitors({ query }) {
  useDocTitle(t('ui.nav.monitors'));
  const [q, setQ] = useState(query.get('q') || '');
  const [sort, setSort] = useState(query.get('sort') || 'status');
  const [quick, setQuick] = useState(false);
  const [sel, setSel] = useState(new Set());
  const [view, setView] = useState(savedView());
  const changeView = (v) => { setView(v); try { localStorage.setItem('unagikuma.monview', v); } catch { /* unavailable */ } };
  const list = useFetch(() => get('/monitors?sort=' + sort), [sort]);
  const beats = useFetch(() => get('/monitors/heartbeats?range=24h&cells=36'), []);
  useLive(['heartbeat'], beats.reload, 5000);
  useLive(['reset'], list.reload, 500);
  // State events carry the full row: patch it in place.
  useEffect(() => onLive('state', (row) => {
    if (!row || !row.name) return;
    list.setData(d => {
      if (!d) return d;
      const i = d.items.findIndex(x => x.name === row.name);
      if (i < 0) { list.reload(); return d; }
      const items = d.items.slice();
      items[i] = row;
      return { ...d, items };
    });
  }), []);
  useEffect(() => { setQ(query.get('q') || ''); }, [query.get('q')]);

  const { status, rest } = splitStatus(q);
  const counts = useMemo(() => {
    const c = {};
    for (const r of (list.data ? list.data.items : [])) c[r.status] = (c[r.status] || 0) + 1;
    return c;
  }, [list.data]);
  const rows = useMemo(() => {
    if (!list.data) return [];
    const words = rest.toLowerCase().split(/\s+/).filter(Boolean);
    return list.data.items.filter(r => {
      if (status && r.status !== status) return false;
      return words.every(w => {
        const neg = w.startsWith('-') && w.length > 1;
        const term = neg ? w.slice(1) : w;
        const [k, v] = term.includes(':') ? term.split(/:(.*)/) : ['', term];
        let hit;
        switch (k) {
          case 'type': hit = r.type.toLowerCase() === v || r.written_type.toLowerCase() === v; break;
          case 'tag': hit = r.tags.some(tg => tg.k.toLowerCase() === v.split(':')[0] && (!v.includes(':') || (tg.v || '').toLowerCase() === v.split(':')[1])); break;
          case 'group': hit = (r.group || '').toLowerCase().includes(v); break;
          case 'depth': hit = r.depth.toLowerCase() === v; break;
          case 'policy': hit = (r.policy || '').toLowerCase() === v; break;
          case 'flag': hit = r.flags.includes(v); break;
          default: hit = [r.name, r.title, r.target, r.group, r.keyword].join(' ').toLowerCase().includes(term);
        }
        return hit !== neg;
      });
    });
  }, [list.data, status, rest]);

  const setStatus = (s) => {
    const next = (s === status ? rest : (rest + ' status:' + s)).trim();
    setQ(next);
    history.replaceState(null, '', '#/monitors' + (next ? '?q=' + enc(next) : ''));
  };
  // Only ticked rows that are still listed count: a filter or a deletion
  // elsewhere must not leave hidden monitors selected.
  const picked = rows.filter(r => sel.has(r.name));
  const allPicked = rows.length > 0 && picked.length === rows.length;
  const toggle = (name) => {
    const n = new Set(sel);
    n.has(name) ? n.delete(name) : n.add(name);
    setSel(n);
  };
  const toggleAll = () => setSel(allPicked ? new Set() : new Set(rows.map(r => r.name)));
  const checkNow = async () => {
    let n = 0;
    for (const r of picked.slice(0, 50)) {
      try { await post('/monitors/' + enc(r.name) + '/check'); n++; } catch { /* rate limited */ }
    }
    toast(t('ui.mon.checks_queued', { n }));
  };
  // Pause or resume every ticked monitor in one change to the file.
  const pauseAll = async (paused) => {
    const names = picked.map(r => r.name);
    const etags = {};
    for (const r of picked) if (r.etag) etags[r.name] = r.etag;
    try {
      const res = await post('/monitors/bulk-pause', { names, paused, etags });
      toast(t(paused ? 'ui.mon.paused_n' : 'ui.mon.resumed_n', { n: fmtNum(res.changed) }));
      if (res.skipped && res.skipped.length) toast(t('ui.mon.resume_skipped', { names: res.skipped.join(', ') }), 'error');
      list.reload();
    } catch (e) { toastError(e); }
  };
  const nPaused = picked.filter(r => r.status === 'paused').length;
  const remove = async () => {
    const names = picked.map(r => r.name);
    if (!await confirmDialog(tn('ui.mon.delete_selected_confirm', names.length, { names: names.slice(0, 8).join(', ') + (names.length > 8 ? ' …' : '') }), { danger: true, ok: t('ui.delete') })) return;
    const etags = {};
    for (const r of picked) if (r.etag) etags[r.name] = r.etag;
    try {
      await post('/monitors/bulk-delete', { names, etags });
      toast(tn('ui.mon.deleted_n', names.length));
      setSel(new Set());
      list.reload();
    } catch (e) { toastError(e); }
  };

  const total = list.data ? list.data.items.length : 0;
  return html`<div class="page">
    <div class="row">
      <input type="search" style=${{ flex: '1 1 180px', maxWidth: '360px', minWidth: '160px' }} placeholder=${t('ui.mon.search')} value=${q}
        onInput=${(e) => setQ(e.target.value)} aria-label=${t('ui.mon.search')} />
      <select style=${{ width: 'auto' }} value=${sort} onChange=${(e) => setSort(e.target.value)} aria-label=${t('ui.mon.sort')}>
        ${['status', 'name', 'last_change', 'latency', 'uptime'].map(s => html`<option value=${s}>${t('ui.mon.sort.' + s)}</option>`)}
      </select>
      <${Seg} value=${view} onChange=${changeView} options=${[['list', t('ui.mon.view_list')], ['hive', t('ui.mon.view_hive')]]} />
      <div class="spacer"></div>
      <button class="btn" onClick=${() => pauseAll(true)} disabled=${picked.length === nPaused} title=${picked.length ? '' : t('ui.mon.select_first')}>${t('ui.mon.pause')}${picked.length > nPaused ? ' (' + fmtNum(picked.length - nPaused) + ')' : ''}</button>
      <button class="btn" onClick=${() => pauseAll(false)} disabled=${!nPaused} title=${picked.length ? '' : t('ui.mon.select_first')}>${t('ui.mon.resume')}${nPaused ? ' (' + fmtNum(nPaused) + ')' : ''}</button>
      <a class="btn" href="#/expiry">${t('ui.exp.title')}</a>
      <a class="btn primary" href="#/monitors/new">${t('ui.mon.add')}</a>
      <button class="btn" onClick=${() => setQuick(true)}>${t('ui.mon.quick_add')}</button>
      <button class="btn" onClick=${checkNow} disabled=${!picked.length} title=${picked.length ? '' : t('ui.mon.select_first')}>${t('ui.mon.check_all')}${picked.length ? ' (' + fmtNum(picked.length) + ')' : ''}</button>
      <button class="btn danger" onClick=${remove} disabled=${!picked.length} title=${picked.length ? '' : t('ui.mon.select_first')}>${t('ui.delete')}${picked.length ? ' (' + fmtNum(picked.length) + ')' : ''}</button>
    </div>
    <div class="chips">
      <button class=${'chip' + (!status ? ' on' : '')} onClick=${() => setStatus(status)}>${t('ui.all')} <b>${fmtNum(total)}</b></button>
      ${CHIPS.filter(s => counts[s]).map(s => html`<button class=${'chip' + (status === s ? ' on' : '')} onClick=${() => setStatus(s)}>${t('status.' + s)} <b>${fmtNum(counts[s])}</b></button>`)}
    </div>
    <${ErrorBox} error=${list.error} retry=${list.reload} />
    <div class="card"><div class="bd flush tbl-wrap">
      ${!list.data ? html`<${Loading} />` : !total ? html`<${Empty} title=${t('ui.mon.none')}><p>${t('ui.mon.none_hint')}</p><a class="btn primary" href="#/monitors/new">${t('ui.mon.add')}</a><//>` :
        !rows.length ? html`<${Empty} title=${t('ui.mon.no_match')} /> ` :
        view === 'hive' ? html`<div class="row" style=${{ padding: '12px 16px 0' }}><${Legend} /></div><${Hive} rows=${rows} />` :
        html`<table class="tbl">
          <thead><tr>
            <th class="sel"><input type="checkbox" checked=${allPicked} onChange=${toggleAll} aria-label=${t('ui.mon.select_all')} title=${t('ui.mon.select_all')} /></th>
            <th>${t('ui.status')}</th><th>${t('ui.monitor')}</th><th class="hide-sm">${t('ui.mon.last24h')}</th>
            <th class="right hide-sm">${t('ui.latency')}</th><th class="right">${t('ui.uptime24h')}</th><th class="hide-sm">${t('ui.mon.changed')}</th><th></th>
          </tr></thead>
          <tbody>${rows.map(r => html`<tr key=${r.name} class=${'click ' + r.status + (sel.has(r.name) ? ' picked' : '')} onClick=${() => go('/monitors/' + enc(r.name))}>
            <td class="sel" onClick=${(e) => e.stopPropagation()}><input type="checkbox" checked=${sel.has(r.name)} onChange=${() => toggle(r.name)} aria-label=${r.title} /></td>
            <td class="nowrap"><${Pill} status=${r.status} soft=${r.state_type === 'soft' && r.status !== 'ok' && r.status !== 'pending'} />
              ${r.state_type === 'soft' && r.attempt > 0 && r.status !== 'ok' && html` <span class="small muted">${r.attempt}/${r.max_attempts}</span>`}</td>
            <td style=${{ maxWidth: '420px' }}>
              <div class="row nowrap"><a class="name ellipsis" href=${'#/monitors/' + enc(r.name)} onClick=${(e) => e.stopPropagation()}>${r.title}</a>
                ${r.flags.includes('weak') && html`<span class="tag weak" title=${t('ui.flag.weak_hint')}>${t('ui.flag.weak')}</span>`}
                ${r.flags.includes('held') && html`<span class="tag" title=${t('ui.flag.held_hint')}>${t('ui.flag.held')}</span>`}
                ${r.incident && r.incident.acked && html`<span class="tag">${t('ui.inc.acked_short')}</span>`}
              </div>
              <div class="sub ellipsis"><span class="tag">${r.written_type}</span> <span class="ltr">${r.target}</span>
                ${r.status !== 'ok' && r.reason ? html` · <span class=${r.status === 'critical' ? 'err-text' : ''}>${reasonText(r.reason, r.detail)}</span>` : ''}</div>
            </td>
            <td class="hide-sm" style=${{ minWidth: '180px' }}><${Beats} cells=${beats.data && beats.data.items[r.name]} /></td>
            <td class="right nowrap hide-sm">${msText(r.latency_ms)}</td>
            <td class="right nowrap" title=${r.uptime_24h == null && r.coverage_24h > 0 ? t('ui.uptime.coverage', { pct: pct(r.coverage_24h, 0) }) : ''}>${pct(r.uptime_24h)}</td>
            <td class="nowrap small muted hide-sm"><${Ago} at=${r.last_change} /></td>
            <td class="right nowrap"><a class="btn sm" href=${'#/monitors/' + enc(r.name) + '/edit'} onClick=${(e) => e.stopPropagation()}>${t('ui.edit')}</a></td>
          </tr>`)}</tbody>
        </table>`}
    </div></div>
    <p class="small muted">${t('ui.mon.query_help')}</p>
    ${quick && html`<${QuickAdd} onClose=${() => setQuick(false)} onDone=${() => { setQuick(false); list.reload(); beats.reload(); }} />`}
  </div>`;
}
