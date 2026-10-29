// Notices and status: announced changes, the status pages and the daily
// status report: what is told to people, apart from the alerts themselves.
import { html, useState } from '../lib.js';
import { t } from '../i18n.js';
import { get, post } from '../api.js';
import { NoticesTab } from './notices.js';
import { ChangesTab } from './changes.js';
import { useFetch, Loading, ErrorBox, toast, toastError, useDocTitle } from '../ui.js';

function ReportTab() {
  const rep = useFetch(() => get('/reports/daily'), []);
  const [busy, setBusy] = useState(false);
  const send = async () => {
    setBusy(true);
    try { await post('/reports/daily/send', {}); toast(t('ui.report.sent')); } catch (e) { toastError(e); }
    setBusy(false);
  };
  if (!rep.data) return rep.error ? html`<${ErrorBox} error=${rep.error} />` : html`<${Loading} />`;
  const r = rep.data;
  return html`<div class="col" style=${{ gap: '12px' }}>
    <div class="row"><p class="muted small spacer" style=${{ margin: 0 }}>${r.enabled
      ? t('ui.report.intro', { at: r.at, policy: r.policy })
      : html`<span class="warn-text">${t('ui.report.off', { where: t('ui.nav.settings') + ' › ' + t('ui.set.tab.tunables') })}</span>`}</p>
      <button class="btn" onClick=${rep.reload}>${t('ui.report.refresh')}</button>
      <button class="btn primary" onClick=${send} disabled=${busy}>${t('ui.report.send')}</button></div>
    <div class="card"><div class="hd"><h3 class=${r.ok ? '' : 'crit-text'}>${r.title}</h3></div>
      <div class="bd"><pre class="report">${r.body}</pre></div></div>
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
