import { html } from '../lib.js';
import { t } from '../i18n.js';
import { get } from '../api.js';
import { useFetch } from '../ui.js';

export function HistorySite({ value, onChange }) {
 const sites = useFetch(() => get('/history/sites'), []);
 return html`<label class="row small">${t('ui.audit.site')} <select value=${value} onChange=${e => onChange(e.target.value)}>
 <option value="">${t('ui.obs.local')}</option>
 <option value="*">${t('ui.all')}</option>
 ${(sites.data || []).map(site => html`<option value=${site}>${site}</option>`)}
 </select></label>`;
}
