// Translations come from the server's catalogs (English fills gaps).
// Plural keys use CLDR suffixes: key_one, key_few, ... chosen by
// Intl.PluralRules, matching the server's own rules.

const RTL = new Set(['ar', 'ur']);
let cat = {};
let lang = 'en';
let rules = new Intl.PluralRules('en');
let languages = [];

export function currentLang() { return lang; }
export function isRTL() { return RTL.has(lang); }
export function langList() { return languages; }

export function detectLang() {
  try {
    const saved = localStorage.getItem('nk.lang');
    if (saved) return saved;
  } catch { /* storage may be unavailable */ }
  const want = (navigator.languages || [navigator.language || 'en']).map(l => l.toLowerCase().split(/[-_]/)[0]);
  const have = new Set(languages.map(l => l.code));
  return want.find(l => have.has(l)) || 'en';
}

export async function loadLanguages() {
  try {
    const r = await fetch('/api/v1/languages');
    if (r.ok) languages = await r.json();
  } catch { /* offline: keep defaults */ }
  if (!languages.length) languages = [{ code: 'en', name: 'English' }];
  return languages;
}

export async function setLang(code, remember) {
  const r = await fetch('/api/v1/i18n/' + encodeURIComponent(code));
  if (!r.ok) {
    if (code !== 'en') return setLang('en', false);
    cat = {};
  } else {
    cat = await r.json();
  }
  lang = r.ok ? code : 'en';
  rules = new Intl.PluralRules(lang);
  document.documentElement.lang = lang;
  document.documentElement.dir = RTL.has(lang) ? 'rtl' : 'ltr';
  if (remember) {
    try { localStorage.setItem('nk.lang', lang); } catch { /* ignore */ }
  }
}

function fill(s, params) {
  if (!params) return s;
  return s.replace(/\{(\w+)\}/g, (m, k) => (k in params && params[k] != null ? String(params[k]) : m));
}

// t translates key; params fill {name} placeholders.
export function t(key, params) {
  const s = cat[key];
  return fill(s === undefined ? key : s, params);
}

// tn picks the plural form for n; {n} is filled automatically.
export function tn(key, n, params) {
  const form = rules.select(n);
  const k = form !== 'other' && cat[key + '_' + form] !== undefined ? key + '_' + form : key;
  return t(k, { n: fmtNum(n), ...params });
}

// has reports whether a translation exists (for optional hints).
export function has(key) { return cat[key] !== undefined; }

export function fmtNum(n, digits) {
  if (n == null || Number.isNaN(n)) return '–';
  return new Intl.NumberFormat(lang, digits == null ? {} : { maximumFractionDigits: digits, minimumFractionDigits: digits }).format(n);
}

export function fmtTime(d, withDate = true) {
  if (!d) return '–';
  const x = d instanceof Date ? d : new Date(d);
  if (Number.isNaN(x.getTime()) || x.getFullYear() < 1971) return '–';
  const opt = withDate ? { dateStyle: 'medium', timeStyle: 'medium' } : { timeStyle: 'medium' };
  return new Intl.DateTimeFormat(lang, opt).format(x);
}

export function fmtShortTime(d) {
  const x = d instanceof Date ? d : new Date(d);
  const today = new Date();
  const same = x.toDateString() === today.toDateString();
  return new Intl.DateTimeFormat(lang, same ? { hour: '2-digit', minute: '2-digit', second: '2-digit' } : { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(x);
}

export function ago(d) {
  if (!d) return '–';
  const x = d instanceof Date ? d : new Date(d);
  if (Number.isNaN(x.getTime()) || x.getFullYear() < 1971) return '–';
  const s = (x.getTime() - Date.now()) / 1000;
  const rtf = new Intl.RelativeTimeFormat(lang, { numeric: 'auto', style: 'short' });
  const a = Math.abs(s);
  if (a < 45) return rtf.format(Math.round(s), 'second');
  if (a < 2700) return rtf.format(Math.round(s / 60), 'minute');
  if (a < 64800) return rtf.format(Math.round(s / 3600), 'hour');
  if (a < 2592000) return rtf.format(Math.round(s / 86400), 'day');
  return rtf.format(Math.round(s / 2592000), 'month');
}

// dur renders seconds compactly with the catalog's unit words.
export function dur(sec) {
  if (sec == null || Number.isNaN(sec)) return '–';
  sec = Math.abs(sec);
  if (sec < 60) return t('dur.s', { n: Math.round(sec) });
  const m = Math.round(sec / 60);
  const parts = [];
  const d = Math.floor(m / 1440), h = Math.floor((m % 1440) / 60), mm = m % 60;
  if (d) parts.push(t('dur.d', { n: d }));
  if (h) parts.push(t('dur.h', { n: h }));
  if (mm && !d) parts.push(t('dur.m', { n: mm }));
  const sep = ['en', 'zh', 'ja', 'ko'].includes(lang) ? '' : ' ';
  return parts.join(sep);
}
