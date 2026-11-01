// Notices and status: announced changes, the status pages and the daily
// status report: what is told to people, apart from the alerts themselves.
import { html, useState, useRef } from '../lib.js';
import { t, currentLang, detailText } from '../i18n.js';
import { get, post } from '../api.js';
import { NoticesTab } from './notices.js';
import { ChangesTab } from './changes.js';
import { useFetch, Loading, ErrorBox, Pill, useDocTitle } from '../ui.js';

function ReportTab() {
  const rep = useFetch(() => get('/reports/daily'), []);
  const lock = useRef(false);
  const [busy, setBusy] = useState(false);
  const [sendError, setSendError] = useState(null);
  const [queued, setQueued] = useState(false);
  const send = async () => {
    if (lock.current) return;
    lock.current = true; setBusy(true); setSendError(null); setQueued(false);
    try {
      const result = await post('/reports/daily/send', {});
      if (result?.delivery !== 'queued') throw new Error(t('ui.report.delivery_rejected'));
      setQueued(true);
    }
    catch (e) { setSendError(e); }
    finally { lock.current = false; setBusy(false); }
  };
  if (!rep.data) return html`<div class="report-view">${rep.error ? html`<${ErrorBox} error=${rep.error} retry=${rep.reload} />` : html`<${Loading} />`}</div>`;
  const r = rep.data, s = r.snapshot;
  const time = value => {
    if (!value) return '';
    const date = new Date(value);
    if (!Number.isFinite(date.getTime())) return '';
    // 'Local' belongs to the server; never silently substitute the browser zone.
    if (!r.timezone || r.timezone === 'Local') return String(value).replace(/\.\d+(?=[Z+-])/, '').replace('T', ' ').replace('Z', ' +00:00');
    try { return new Intl.DateTimeFormat(currentLang(), { dateStyle: 'medium', timeStyle: 'medium', timeZone: r.timezone === 'Local' ? undefined : r.timezone }).format(date); }
    catch { return date.toISOString(); }
  };
  const counts = (label, data, states) => html`<section class="report-counts">
    <h3>${label}<strong>${data.total}</strong></h3>
    <dl>${states.map(st => html`<div key=${st}><dt><i class=${'dot ' + (st === 'up' ? 'ok' : st === 'down' ? 'critical' : st)}></i>${t((st === 'up' || st === 'down' ? 'ui.report.' : 'status.') + st)}</dt><dd>${data.states[st] || 0}</dd></div>`)}</dl>
  </section>`;
  return html`<div class="report-view">
    <header class="report-heading"><div><p class="report-eyebrow">${t('ui.report.tab')}</p><h2>${t('ui.report.state.' + (s?.state || 'incomplete'))}</h2></div>
      <div class="report-actions"><button class="btn" onClick=${rep.reload} disabled=${rep.loading}>${t('ui.report.refresh')}</button><button class="btn primary" onClick=${send} disabled=${busy}>${t('ui.report.send')}</button></div>
    </header>
    ${rep.error && html`<div role="alert" class="report-error"><p>${t('ui.report.stale')}</p><${ErrorBox} error=${rep.error} retry=${rep.reload} /></div>`}
    ${sendError && html`<div role="alert" class="report-error"><${ErrorBox} error=${sendError} retry=${send} /></div>`}
    ${queued && html`<p role="status" class="report-feedback">${t('ui.report.queued')}</p>`}
    <section class=${'report-overview report-state-' + (s?.state || 'incomplete')}>
      <div class="report-caption"><span>${t('ui.report.snapshot')}</span><time dateTime=${r.generated_at}>${time(r.generated_at)}${r.timezone && ' · ' + r.timezone}</time></div>
      ${s?.state === 'empty' ? html`<div class="report-empty"><h3>${t('ui.report.state.empty')}</h3><p>${t('ui.report.empty_hint')}</p><a class="btn" href="#/monitors">${t('ui.nav.monitors')}</a></div>` : s && html`<div class="report-totals">
        ${counts(t('ui.report.services'), s.services, ['critical','degraded','unknown','unreachable','ok','pending','paused','maintenance'])}
        ${counts(t('ui.report.hosts'), s.hosts, ['down','unreachable','up','pending','paused','maintenance'])}
      </div>`}
    </section>
    ${s?.services.total > 0 && html`<section class="report-issues"><div class="report-section-heading"><h3>${t('ui.report.issues')}</h3><span class="report-total">${s.issues_total}</span></div>
      ${s.issues.length ? html`<ol>${s.issues.map(issue => html`<li key=${issue.name}>
        <div class="report-issue-top"><a href=${'#/monitors/' + encodeURIComponent(issue.name)}>${issue.title || issue.name}</a><${Pill} status=${issue.status}/></div>
        ${issue.detail && html`<p>${detailText(issue.detail)}</p>`}
        ${(issue.since || issue.acked) && html`<div class="report-issue-meta">${issue.since && html`<span>${t('ui.report.since')} <time dateTime=${issue.since}>${time(issue.since)}</time></span>`}${issue.acked && html`<span>${t('ui.report.acked')}${issue.acked_by && ' · ' + issue.acked_by}</span>`}</div>`}
      </li>`)}</ol>` : html`<p class="report-noissues">${t('ui.report.state.' + s.state)}</p>`}
      ${s.issues_omitted > 0 && html`<p class="report-omitted">${t('ui.report.omitted', {n:s.issues_omitted})} · <a href="#/monitors">${t('ui.nav.monitors')}</a></p>`}
    </section>`}
    <footer class="report-delivery"><dl><div><dt>${t('ui.report.schedule')}</dt><dd>${r.enabled ? r.at : t('ui.report.disabled')}</dd></div><div><dt>${t('ui.report.policy')}</dt><dd>${r.policy}</dd></div><div><dt>${t('ui.report.timezone')}</dt><dd>${r.timezone}</dd></div></dl><a href="#/settings">${t('ui.nav.settings')}</a></footer>
    <details class="report-preview"><summary>${t('ui.report.preview')}</summary><h3>${r.title}</h3><pre>${r.body}</pre></details>
  </div>`;
}

export const NOTICE_TABS = [['changes', 'ui.chg.tab'], ['pages', 'ui.notice.tab'], ['report', 'ui.report.tab']];

export function Notices({ tab }) {
  useDocTitle(t('ui.nav.notices'));
  return html`<div class="page">
    <div class="tabs">${NOTICE_TABS.map(([k, label]) => html`<a href=${'#/notices/' + k} class=${tab === k ? 'on' : ''}>${t(label)}</a>`)}</div>
    ${tab === 'pages' ? html`<${NoticesTab} />` : tab === 'report' ? html`<${ReportTab} />` : html`<${ChangesTab} />`}
  </div>`;
}
