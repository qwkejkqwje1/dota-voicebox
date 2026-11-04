// i18n.js — язык интерфейса. Строки в коде — русские; для английского интерфейса сервер отдаёт словарь
// (ключ — русская строка). Загружается до остальных модулей (top-level await), поэтому t() работает везде.
const TOK = new URLSearchParams(location.search).get('t') || '';
let D = null;
let lang = 'ru';
try {
  const r = await fetch('/api/i18n', {headers: {'X-Token': TOK}});
  if (r.ok) {
    const j = await r.json();
    if (j.lang === 'en') { lang = 'en'; D = j.dict || {}; }
  }
} catch (e) { /* без связи — по-русски */ }
document.documentElement.lang = lang;

export const LANG = lang;
export const EN = lang === 'en';

// _t — перевод строки (неизвестная — как есть; пробелы по краям сохраняются)
export function _t(s) {
  if (!D || typeof s !== 'string') return s;
  const v = D[s];
  if (v !== undefined) return v;
  const tr = s.trim();
  if (tr && tr !== s) {
    const w = D[tr];
    if (w !== undefined) { const i = s.indexOf(tr); return s.slice(0, i) + w + s.slice(i + tr.length); }
  }
  return s;
}

// _tf — перевод шаблона с подстановками {0}, {1}…
export function _tf(s, ...args) {
  return _t(s).replace(/\{(\d+)\}/g, (m, i) => args[+i] === undefined ? m : String(args[+i]));
}

// dec — десятичная запятая по-русски, точка по-английски
export const dec = s => EN ? String(s) : String(s).replace('.', ',');
