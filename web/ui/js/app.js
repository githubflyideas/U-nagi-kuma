// Application shell: session, navigation and routing.
import { html, render, useState, useEffect } from './lib.js';
import { t, tn, setLang, detectLang, loadLanguages, langList, currentLang, fmtTime, detailText } from './i18n.js';
import { get, post, del, setCSRF, onUnauthorized } from './api.js';
import { startLive, stopLive, onConnection } from './live.js';
import { useRoute, href, Toasts, ConfirmHost, useFetch, useLive, errText, Field } from './ui.js';
import { Overview } from './views/overview.js';
import { Monitors } from './views/monitors.js';
import { MonitorDetail } from './views/monitor.js';
import { MonitorEditor } from './views/editor.js';
import { Problems, setupIssues } from './views/problems.js';
import { Events } from './views/events.js';
import { ConfigView } from './views/config.js';
import { Alerts } from './views/alerts.js';
import { Notices } from './views/announce.js';
import { OnCall } from './views/oncall.js';
import { Settings } from './views/settings.js';
import { Nodes } from './views/nodes.js';
import { Expiry } from './views/expiry.js';
import { IncidentReport } from './views/incident.js';
import { DrawerHost } from './views/drawer.js';
import { PaletteHost, openPalette } from './views/palette.js';

// The console is dark unless the user picks light or the system setting.
function applyTheme(v) {
  v = v || 'dark';
  if (v === 'light' || v === 'dark') document.documentElement.dataset.theme = v;
  else delete document.documentElement.dataset.theme;
}

function storage(k, v) {
  try {
    if (v === undefined) return localStorage.getItem(k);
    localStorage.setItem(k, v);
  } catch { /* unavailable */ }
  return null;
}

function localZone() {
  try { return Intl.DateTimeFormat('en').resolvedOptions().timeZone; } catch { return ''; }
}

function Logo() { return html`<img src="/logo.png" alt="" />`; }

function LangSelect({ onChange }) {
  return html`<select aria-label=${t('ui.language')} value=${currentLang()} onChange=${(e) => onChange(e.target.value)}>
    ${langList().map(l => html`<option value=${l.code}>${l.name}</option>`)}
  </select>`;
}

// ---- setup and login ----

function Setup({ local, onDone, onLang }) {
  const [f, setF] = useState({ username: 'admin', password: '', confirm: '', token: '' });
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e) => {
    e.preventDefault();
    if (f.password.length < 8) return setErr(t('ui.setup.short'));
    if (f.password !== f.confirm) return setErr(t('ui.setup.mismatch'));
    setBusy(true);
    try {
      const r = await post('/setup', { username: f.username, password: f.password, token: f.token, lang: currentLang(), timezone: localZone() });
      onDone(r);
    } catch (e2) {
      setErr(errText(e2));
    }
    setBusy(false);
  };
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
  return html`<div class="auth"><div>
    <div class="logo"><${Logo} /> Unagikuma</div>
    <form class="card" onSubmit=${submit}>
      <div class="hd"><h2>${t('ui.setup.title')}</h2><div class="spacer"></div><${LangSelect} onChange=${onLang} /></div>
      <div class="bd form">
        <p class="muted small">${t('ui.setup.intro')}</p>
        ${!local && html`<${Field} label=${t('ui.setup.token')} hint=${t('ui.setup.token_hint')}><input type="text" class="ltr" value=${f.token} onInput=${set('token')} required autocomplete="off" /><//>`}
        <${Field} label=${t('ui.username')}><input type="text" value=${f.username} onInput=${set('username')} required autocomplete="username" /><//>
        <${Field} label=${t('ui.password')} hint=${t('ui.setup.short')}><input type="password" value=${f.password} onInput=${set('password')} required autocomplete="new-password" /><//>
        <${Field} label=${t('ui.setup.confirm')}><input type="password" value=${f.confirm} onInput=${set('confirm')} required autocomplete="new-password" /><//>
        ${err && html`<div class="banner error">${err}</div>`}
        <button class="btn primary" disabled=${busy}>${t('ui.setup.create')}</button>
      </div>
    </form>
  </div></div>`;
}

function Login({ onDone, onLang }) {
  const [f, setF] = useState({ username: '', password: '' });
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      onDone(await post('/session', f));
    } catch (e2) {
      setErr(e2.status === 401 ? t('ui.login.wrong') : errText(e2));
    }
    setBusy(false);
  };
  return html`<div class="auth"><div>
    <div class="logo"><${Logo} /> Unagikuma</div>
    <form class="card" onSubmit=${submit}>
      <div class="hd"><h2>${t('ui.login.title')}</h2><div class="spacer"></div><${LangSelect} onChange=${onLang} /></div>
      <div class="bd form">
        <${Field} label=${t('ui.username')}><input type="text" value=${f.username} onInput=${(e) => setF({ ...f, username: e.target.value })} required autofocus autocomplete="username" /><//>
        <${Field} label=${t('ui.password')}><input type="password" value=${f.password} onInput=${(e) => setF({ ...f, password: e.target.value })} required autocomplete="current-password" /><//>
        ${err && html`<div class="banner error">${err}</div>`}
        <button class="btn primary" disabled=${busy}>${t('ui.login.submit')}</button>
        <p class="muted small">${t('ui.login.forgot')} <code class="ltr">unagikuma reset-password</code></p>
      </div>
    </form>
  </div></div>`;
}

// ---- shell ----

const NAV = [
  ['/', 'ui.nav.overview', '◎'],
  ['/monitors', 'ui.nav.monitors', '☰'],
  ['/problems', 'ui.nav.problems', '⚠'],
  ['/events', 'ui.nav.events', '≡'],
  ['/alerts', 'ui.nav.alerts', '✉'],
  ['/oncall', 'ui.nav.oncall', '☎'],
  ['/notices', 'ui.nav.notices', '⚑'],
  ['/config', 'ui.nav.config', '{}'],
  ['/nodes', 'ui.nav.nodes', '⌖'],
  ['/settings', 'ui.nav.settings', '⚙'],
];

const OLD_ALERT_TABS = { changes: 'changes', notices: 'pages', report: 'report' };

function Shell({ user, onLogout, onLang }) {
  const route = useRoute();
  const [open, setOpen] = useState(false);
  const [live, setLive] = useState(false);
  const [theme, setTheme] = useState(storage('unagikuma.theme') || 'dark');
  const [mini, setMini] = useState(storage('unagikuma.side') === 'mini');
  const ov = useFetch(() => get('/overview'), []);
  useLive(['incident', 'state', 'serial', 'banner', 'directory'], ov.reload, 1500);
  useEffect(() => onConnection(setLive), []);
  useEffect(() => setOpen(false), [route.path]);
  const unhandled = ov.data ? ov.data.unhandled : 0;
  const scriptProblems = ov.data ? ov.data.script_problems || 0 : 0;
  const contactIssues = ov.data ? ov.data.contact_issues || 0 : 0;
  const setup = setupIssues(ov.data);
  const first = '/' + (route.parts[0] || '');
  const changeTheme = (v) => { setTheme(v); storage('unagikuma.theme', v); applyTheme(v); };
  // On a wide screen the button folds the sidebar to icons; on a phone it
  // slides the sidebar in.
  const toggleSide = () => {
    if (matchMedia('(max-width: 860px)').matches) return setOpen(!open);
    storage('unagikuma.side', mini ? 'full' : 'mini');
    setMini(!mini);
  };

  let view;
  const p = route.parts;
  switch (p[0]) {
    case undefined: view = html`<${Overview} />`; break;
    case 'problems': view = html`<${Problems} tab=${p[1] || 'unhandled'} />`; break;
    case 'monitors':
      if (p[1] === 'new') view = html`<${MonitorEditor} />`;
      else if (p[1] && p[2] === 'edit') view = html`<${MonitorEditor} name=${p[1]} />`;
      else if (p[1]) view = html`<${MonitorDetail} name=${p[1]} />`;
      else view = html`<${Monitors} query=${route.query} />`;
      break;
    case 'events': view = html`<${Events} tab=${p[1] || 'events'} />`; break;
    case 'expiry': view = html`<${Expiry} />`; break;
    case 'incidents': view = p[1] ? html`<${IncidentReport} id=${p[1]} />` : html`<${Problems} tab="all" />`; break;
    // Changes, status pages and the report used to live under Alerting;
    // their old addresses still open them.
    case 'alerts': view = OLD_ALERT_TABS[p[1]] ? html`<${Notices} tab=${OLD_ALERT_TABS[p[1]]} />` : html`<${Alerts} tab=${p[1] || 'channels'} />`; break;
    case 'oncall': view = html`<${OnCall} />`; break;
    case 'notices': view = html`<${Notices} tab=${p[1] || 'changes'} />`; break;
    case 'config': view = html`<${ConfigView} tab=${p[1] || 'edit'} />`; break;
    case 'settings': view = html`<${Settings} tab=${p[1] || 'general'} user=${user} />`; break;
    case 'nodes': view = html`<div class="page"><${Nodes} /></div>`; break;
    default: view = html`<div class="page"><div class="empty"><b>404</b></div></div>`;
  }
  const banner = ov.data && ov.data.banner;
  // A single machine keeps nodes under Settings; with remote nodes they
  // get their own place in the sidebar.
  const nav = NAV.filter(([path]) => path !== '/nodes' || (ov.data && ov.data.layout === 'cluster'));
  return html`<div class=${'shell' + (mini ? ' mini' : '')}>
    <aside class=${'side' + (open ? ' open' : '')}>
      <a class="brand" href="#/"><${Logo} /><div>Unagikuma</div></a>
      <nav class="nav">
        ${nav.map(([path, key, ico]) => html`<a href=${href(path)} class=${first === path ? 'on' : ''}>
          <span class="ico" title=${mini ? t(key) : ''}>${ico}</span><span class="lbl">${t(key)}</span>
          ${path === '/problems' && setup.length > 0 && html`<span class="rdot setup" style=${{ marginTop: 0, marginInlineStart: 'auto' }} title=${setup.map(x => x.n ? tn(x.key, x.n) : t(x.key)).join('\n')}></span>`}
          ${path === '/problems' && unhandled > 0 && html`<span class="count">${unhandled}</span>`}
          ${path === '/settings' && scriptProblems > 0 && html`<span class="count" title=${t('ui.scripts.badge', { n: scriptProblems })}>${scriptProblems}</span>`}
          ${path === '/alerts' && contactIssues > 0 && html`<span class="rdot" style=${{ marginTop: 0, marginInlineStart: 'auto' }} title=${t('ui.dir.nav_dot', { n: contactIssues })}></span>`}
        </a>`)}
      </nav>
      <div class="foot">
        <${LangSelect} onChange=${onLang} />
        <select aria-label=${t('ui.theme')} value=${theme} onChange=${(e) => changeTheme(e.target.value)}>
          <option value="auto">${t('ui.theme.auto')}</option>
          <option value="light">${t('ui.theme.light')}</option>
          <option value="dark">${t('ui.theme.dark')}</option>
        </select>
        <div class="row"><span class="ellipsis">${user}</span><div class="spacer"></div><button class="btn sm ghost" onClick=${onLogout}>${t('ui.logout')}</button></div>
        ${ov.data && ov.data.instance && html`<div class="host ellipsis" title=${ov.data.instance}>${ov.data.instance}</div>`}
      </div>
    </aside>
    <div class="main">
      <header class="top">
        <button class="btn sm" onClick=${toggleSide} aria-label=${mini ? t('ui.side.expand') : t('ui.side.collapse')} title=${mini ? t('ui.side.expand') : t('ui.side.collapse')}>☰</button>
        <div class="crumbs">${t((NAV.find(n => n[0] === (first === '/expiry' ? '/monitors' : first)) || NAV[0])[1])}</div>
        ${ov.data && html`<${Verdict} ov=${ov.data} />`}
        <div class="spacer"></div>
        <${Search} />
        <span class=${'live' + (live ? ' on' : '')} title=${live ? t('ui.live.on') : t('ui.live.off')}><i></i><span class="hide-sm">${live ? t('ui.live.on') : t('ui.live.off')}</span></span>
      </header>
      ${banner && html`<div class="page" style=${{ paddingBottom: 0 }}><${ConfigBanner} banner=${banner} /></div>`}
      ${ov.data && ov.data.storage && ov.data.storage.failing && html`<div class="page" style=${{ paddingBottom: 0 }}><div class="banner error">
        <div class="spacer">${t('ui.storage_failing', { since: fmtTime(ov.data.storage.since), lost: ov.data.storage.lost })}
          ${ov.data.storage.error && html`<div class="small ltr">${ov.data.storage.error}</div>`}</div></div></div>`}
      ${ov.data && ov.data.superseded && html`<div class="page" style=${{ paddingBottom: 0 }}><div class="banner error"><div class="spacer">
        ${t('ui.ha.superseded', { term: ov.data.superseded.term, by: ov.data.superseded.by || '?', at: fmtTime(ov.data.superseded.at) })}</div></div></div>`}
      ${view}
    </div>
  </div>`;
}

// Verdict sums the whole installation up in one chip, with the counts
// next to it.
function Verdict({ ov }) {
  const paths = ov.paths || [];
  const anomalies = (ov.health && ov.health.anomalies) || [];
  const nodeDown = paths.some(p => !p.self && p.online && ['wan_down', 'lan_down', 'no_route'].includes(p.uplink));
  let cls = 'ok', text = t('ui.top.all_ok');
  if (anomalies.length || nodeDown) { cls = 'held'; text = t('ui.top.held'); }
  else if (ov.unhandled > 0) { cls = 'critical'; text = t('ui.top.unhandled', { n: ov.unhandled }); }
  else if (ov.degraded > 0) { cls = 'warn'; text = t('ui.top.warn'); }
  else if ((ov.counts.unknown || 0) > 0) { cls = 'unknown'; text = t('ui.top.unknown', { n: ov.counts.unknown }); }
  const week = ov.value_week || {};
  const saved = Object.values(week).reduce((a, n) => a + (n || 0), 0);
  const online = paths.filter(p => p.online).length;
  const sub = [t('ui.top.monitors', { n: ov.counts.total || 0 }),
    ov.layout === 'cluster' ? t('ui.top.nodes', { online, n: paths.length }) : t('ui.top.single'),
    saved ? t('ui.top.saved', { n: saved }) : ''].filter(Boolean).join(' · ');
  return html`<a class=${'verdict ' + cls} href=${cls === 'critical' ? '#/problems' : '#/'}><i></i>${text}</a><span class="top-sub">${sub}</span>`;
}

// Search opens the command palette (Ctrl+K).
function Search() {
  return html`<button type="button" class="search" onClick=${openPalette} aria-label=${t('ui.top.search')}><span>⌕</span>
    <span class="spacer">${t('ui.top.search')}</span><kbd>Ctrl K</kbd></button>`;
}

export function ConfigBanner({ banner }) {
  const k = 'ui.banner.' + banner.code;
  const msg = t(k, { serial: banner.serial });
  return html`<div class=${'banner ' + banner.kind}>
    <div class="spacer">${msg === k ? banner.message : msg}
      ${banner.diagnostics && banner.diagnostics.length > 0 && html`<ul class="small">${banner.diagnostics.slice(0, 5).map(d => html`<li class="ltr">${d.pos.line}:${d.pos.col} ${detailText(d.message)}</li>`)}</ul>`}
    </div>
    <a class="btn sm" href="#/config">${t('ui.banner.open')}</a>
  </div>`;
}

// ---- root ----

function Root() {
  const [st, setSt] = useState({ phase: 'loading' });
  const [, setLangTick] = useState(0);
  const changeLang = async (code) => {
    await setLang(code, true);
    setLangTick(x => x + 1);
  };
  const check = async () => {
    try {
      const s = await get('/session');
      setCSRF(s.csrf);
      startLive();
      setSt({ phase: 'in', user: s.user });
    } catch (e) {
      stopLive();
      if (e.code === 'setup_required') {
        let local = true;
        try { local = (await get('/setup/status')).local; } catch { /* keep default */ }
        setSt({ phase: 'setup', local });
      } else if (e.status === 401) {
        setSt({ phase: 'login' });
      } else {
        setSt({ phase: 'error', error: e });
      }
    }
  };
  useEffect(() => {
    onUnauthorized(() => { stopLive(); setSt({ phase: 'login' }); });
    check();
  }, []);
  const signedIn = (r) => { setCSRF(r.csrf); startLive(); setSt({ phase: 'in', user: r.user }); location.hash = '#/'; };
  const logout = async () => {
    try { await del('/session'); } catch { /* ignore */ }
    stopLive();
    setCSRF('');
    setSt({ phase: 'login' });
  };
  let body;
  switch (st.phase) {
    case 'loading': body = html`<div class="boot">Unagikuma</div>`; break;
    case 'setup': body = html`<${Setup} local=${st.local} onDone=${signedIn} onLang=${changeLang} />`; break;
    case 'login': body = html`<${Login} onDone=${signedIn} onLang=${changeLang} />`; break;
    case 'error': body = html`<div class="auth"><div class="banner error">${errText(st.error)} <button class="btn sm" onClick=${check}>${t('ui.retry')}</button></div></div>`; break;
    default: body = html`<${Shell} user=${st.user} onLogout=${logout} onLang=${changeLang} />`;
  }
  return html`${body}${st.phase === 'in' && html`<${DrawerHost} /><${PaletteHost} />`}<${Toasts} /><${ConfirmHost} />`;
}

(async () => {
  applyTheme(storage('unagikuma.theme'));
  await loadLanguages();
  await setLang(detectLang(), false);
  render(html`<${Root} />`, document.getElementById('app'));
})();
