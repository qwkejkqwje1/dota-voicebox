// core.js — общие утилиты интерфейса VoiceBox: состояние, запросы, DOM, диалоги, элементы управления.
import {_t, _tf, dec, EN} from './i18n.js';
export const TOKEN = new URLSearchParams(location.search).get('t') || '';

// app — общее состояние интерфейса
export const app = {
  state: null,      // /api/state
  status: null,     // живой статус (SSE, 2 раза в секунду)
  levels: {mic_in: 0, voice_out: 0, monitor: 0, playing: []},
  page: 'sounds',
  setup: null,      // /api/setup
  offline: false,
  dirty: false,     // нужна перерисовка, отложенная из-за ввода
};
export const cfg = () => app.state.config;

// ---------- DOM ----------
export function h(tag, attrs, ...kids) {
  const el = document.createElement(tag);
  if (attrs) for (const [k, v] of Object.entries(attrs)) {
    if (v == null || v === false) continue;
    if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2), v);
    else if (k === 'class') el.className = v;
    else if (k === 'style') { if (typeof v === 'string') el.style.cssText = v; else for (const [p, x] of Object.entries(v)) el.style.setProperty(p, x); }
    else if (k === 'ref') v(el);
    else if (k in el && !k.includes('-') && k !== 'list' && k !== 'form') el[k] = v;
    else el.setAttribute(k, v === true ? '' : v);
  }
  add(el, kids);
  return el;
}
function add(el, kids) {
  for (const k of kids) {
    if (k == null || k === false || k === true) continue;
    if (Array.isArray(k)) add(el, k);
    else el.append(k instanceof Node ? k : String(k));
  }
}
export const $ = (sel, root = document) => root.querySelector(sel);
export const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

// icon — небольшие SVG-значки (не зависят от шрифтов)
const ICONS = {
  x: '<path d="M6 6l12 12M18 6L6 18"/>',
  power: '<path d="M12 3.5v7.5"/><path d="M6.6 6.8a7.6 7.6 0 1 0 10.8 0"/>',
};
export function icon(name, size = 16) {
  return h('span', {class: 'ico', innerHTML: '<svg viewBox="0 0 24 24" width="' + size + '" height="' + size + '" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">' + ICONS[name] + '</svg>'});
}

// ---------- запросы ----------
export async function api(path, body, opt = {}) {
  const method = opt.method || (body === undefined ? 'GET' : 'POST');
  const init = {method, headers: {'X-Token': TOKEN}};
  if (body instanceof FormData || body instanceof Blob || body instanceof ArrayBuffer) init.body = body;
  else if (body !== undefined) { init.body = JSON.stringify(body); init.headers['Content-Type'] = 'application/json'; }
  let r;
  try { r = await fetch(path, init); } catch (e) { throw new Error(_t('Нет связи с VoiceBox — программа закрыта?')); }
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || r.statusText || (_t('Ошибка ') + r.status));
  return j;
}
const isState = j => j && typeof j === 'object' && j.config && j.sounds !== undefined && j.presets;
// act — действие с обработкой ошибок; если сервер вернул состояние — применяем и перерисовываем.
export async function act(path, body, {ok, render = true, method} = {}) {
  try {
    const j = await api(path, body, {method});
    if (isState(j)) setState(j, render);
    else if (j && isState(j.state)) setState(j.state, render);
    if (ok) toast(ok, 'ok');
    return j || {};
  } catch (e) {
    toast(e.message, 'err');
    return null;
  }
}
// set — изменить отдельные настройки (/api/settings)
export const set = (values, opt = {}) => act('/api/settings', values, {render: opt.render !== false, ok: opt.ok});
// debounced — отложенный вызов (ползунки)
export function debounced(fn, ms = 250) {
  let t = null, last = null;
  const f = (...a) => { last = a; clearTimeout(t); t = setTimeout(() => { t = null; fn(...last); }, ms); };
  f.flush = () => { if (t) { clearTimeout(t); t = null; fn(...last); } };
  return f;
}

let renderer = () => {};
export function setRenderer(fn) { renderer = fn; }
export function setState(st, render = true) {
  app.state = st;
  if (st.status && (!app.status || st.status.rev >= app.status.rev)) app.status = st.status;
  if (render) requestRender();
}
let renderQueued = false;
export function requestRender() {
  if (renderQueued) return;
  renderQueued = true;
  queueMicrotask(() => { renderQueued = false; renderer(false); });
}
export async function reload(render = true) {
  try { setState(await api('/api/state'), render); } catch (e) { toast(e.message, 'err'); }
}

// живые обновления страницы (сбрасываются при каждой перерисовке)
const hooks = {levels: new Set(), status: new Set(), log: new Set()};
export function onLive(kind, fn) { hooks[kind].add(fn); }
export function clearLive() { for (const k in hooks) hooks[k].clear(); }
export function emitLive(kind, data) {
  for (const f of hooks[kind]) { try { f(data); } catch (e) { console.error(e); } }
}

// ---------- уведомления ----------
export function toast(msg, kind = '') {
  const box = $('#toasts');
  if (!box) return;
  const el = h('div', {class: 'toast ' + kind}, msg);
  box.append(el);
  while (box.children.length > 4) box.firstChild.remove();
  setTimeout(() => { el.classList.add('out'); setTimeout(() => el.remove(), 220); }, kind === 'err' ? 6500 : 2800);
}

// ---------- диалоги ----------
// dialog({title, body, foot, wide}) — body/foot: узлы или функции (d) => узлы; d.update() перерисовывает.
export function dialog({title, body, foot, wide, cls, onClose, onKey}) {
  const layer = $('#layer');
  const bodyEl = h('div', {class: 'dlg-b'});
  const footEl = h('div', {class: 'dlg-f'});
  const titleEl = h('h2', {}, title);
  const box = h('div', {class: 'dlg' + (wide ? ' wide' : '')},
    h('div', {class: 'dlg-h'}, titleEl, h('button', {class: 'btn ghost icon sm x', title: _t('Закрыть (Esc)'), onclick: () => d.close()}, icon('x'))),
    bodyEl, footEl);
  const ov = h('div', {class: 'overlay ' + (cls || '')}, box);
  let downOnOverlay = false;
  ov.addEventListener('mousedown', e => { downOnOverlay = e.target === ov; });
  ov.addEventListener('click', e => { if (e.target === ov && downOnOverlay) d.close(); });
  const keys = e => {
    if (layer.lastChild !== ov) return;
    if (onKey && onKey(e) === false) return;
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); d.close(); }
  };
  const d = {
    el: box, closed: false,
    title(t) { titleEl.textContent = t; },
    update() {
      const b = typeof body === 'function' ? body(d) : body;
      const f = typeof foot === 'function' ? foot(d) : foot;
      const sc = bodyEl.scrollTop;
      bodyEl.replaceChildren(...[b].flat(9).filter(x => x != null && x !== false));
      bodyEl.scrollTop = sc;
      footEl.replaceChildren(...[f].flat(9).filter(x => x != null && x !== false));
      footEl.classList.toggle('hidden', !footEl.children.length);
    },
    close(v) {
      if (d.closed) return;
      d.closed = true;
      document.removeEventListener('keydown', keys, true);
      ov.remove();
      if (onClose) onClose(v);
      if (!layer.children.length && app.dirty) requestRender();
    },
  };
  document.addEventListener('keydown', keys, true);
  layer.append(ov);
  d.update();
  setTimeout(() => { const f = box.querySelector('[autofocus]'); if (f) f.focus(); });
  return d;
}
export function confirmDlg(text, {title = _t('Подтвердите'), ok = _t('Да'), cancel = _t('Отмена'), danger = false} = {}) {
  return new Promise(res => {
    const d = dialog({title, body: h('p', {style: 'margin:0;color:var(--dim)'}, text), onClose: v => res(!!v),
      foot: [h('div', {class: 'grow'}), h('button', {class: 'btn', onclick: () => d.close(false)}, cancel),
        h('button', {class: 'btn ' + (danger ? 'danger' : 'primary'), autofocus: true, onclick: () => d.close(true)}, ok)]});
  });
}
export function promptDlg(title, {label = '', value = '', placeholder = '', ok = 'OK', hint = ''} = {}) {
  return new Promise(res => {
    const inp = h('input', {class: 'input', value, placeholder, autofocus: true, onkeydown: e => { if (e.key === 'Enter') d.close(inp.value); }});
    const d = dialog({title, onClose: v => res(v == null ? null : String(v)),
      body: [label ? h('label', {class: 'field'}, h('span', {}, label), inp) : inp, hint && h('p', {class: 'hint mt-s'}, hint)],
      foot: [h('div', {class: 'grow'}), h('button', {class: 'btn', onclick: () => d.close()}, _t('Отмена')),
        h('button', {class: 'btn primary', onclick: () => d.close(inp.value)}, ok)]});
  });
}

// ---------- клавиши ----------
const DISP = {TILDE: '~', '`': '~', ESC: 'Esc', SPACE: _t('Пробел'), ENTER: 'Enter', TAB: 'Tab', BACKSPACE: 'Backspace',
  PAUSE: 'Pause', CAPSLOCK: 'CapsLock', SCROLLLOCK: 'ScrollLock', PAGEUP: 'PgUp', PAGEDOWN: 'PgDn', END: 'End', HOME: 'Home',
  INSERT: 'Ins', DELETE: 'Del', LEFT: '←', RIGHT: '→', UP: '↑', DOWN: '↓', NUMMUL: 'Num *', NUMADD: 'Num +', NUMSUB: 'Num −',
  NUMDOT: 'Num .', NUMDIV: 'Num /', MOUSE3: _t('Колёсико'), MOUSE4: _t('Мышь 4'), MOUSE5: _t('Мышь 5'), LSHIFT: _t('Левый Shift'),
  RSHIFT: _t('Правый Shift'), LCTRL: _t('Левый Ctrl'), RCTRL: _t('Правый Ctrl'), LALT: _t('Левый Alt'), RALT: _t('Правый Alt')};
const MODS = {CTRL: 'Ctrl', CONTROL: 'Ctrl', ALT: 'Alt', SHIFT: 'Shift', WIN: 'Win'};
// keyLabel — подпись клавиши для человека (как keys.Display в программе)
export function keyLabel(s) {
  if (!s || !String(s).trim()) return '';
  const parts = String(s).replace(/\s+/g, '').split('+');
  return parts.map((p, i) => {
    const up = p.toUpperCase();
    if (i < parts.length - 1) return MODS[up] || p;
    if (DISP[up]) return DISP[up];
    if (/^NUM\d$/.test(up)) return 'Num ' + up.slice(3);
    return up;
  }).join('+');
}
export const normKey = s => String(s || '').replace(/\s+/g, '').toUpperCase().replace('CONTROL', 'CTRL').replace(/(^|\+)`$/, '$1TILDE');
export function kbd(key, {onclick, empty = _t('нет'), title} = {}) {
  const l = keyLabel(key);
  return h(onclick ? 'button' : 'span', {class: 'kbd' + (l ? '' : ' none') + (onclick ? ' click' : ''), title: title || (onclick ? _t('Изменить клавишу') : null),
    onclick: onclick ? e => { e.stopPropagation(); onclick(e); } : null, style: onclick ? 'font:inherit;font-size:11.5px;font-weight:600' : null}, l || empty);
}
const CODES = {Backquote: 'Tilde', Space: 'Space', Tab: 'Tab', Enter: 'Enter', NumpadEnter: 'Enter', Backspace: 'Backspace',
  Insert: 'Insert', Delete: 'Delete', Home: 'Home', End: 'End', PageUp: 'PageUp', PageDown: 'PageDown', Pause: 'Pause',
  ScrollLock: 'ScrollLock', CapsLock: 'CapsLock', ArrowUp: 'Up', ArrowDown: 'Down', ArrowLeft: 'Left', ArrowRight: 'Right',
  NumpadAdd: 'NumAdd', NumpadSubtract: 'NumSub', NumpadMultiply: 'NumMul', NumpadDivide: 'NumDiv', NumpadDecimal: 'NumDot',
  ShiftLeft: 'LShift', ShiftRight: 'RShift', ControlLeft: 'LCtrl', ControlRight: 'RCtrl', AltLeft: 'LAlt', AltRight: 'RAlt'};
function codeName(e) {
  const c = e.code;
  if (/^Key[A-Z]$/.test(c)) return c.slice(3);
  if (/^Digit\d$/.test(c)) return c.slice(5);
  if (/^Numpad\d$/.test(c)) return 'Num' + c.slice(6);
  if (/^F\d{1,2}$/.test(c)) return c;
  return CODES[c] || null;
}
export function hotkeyOwner(key) {
  const n = normKey(key);
  for (const [k, a] of Object.entries(cfg().hotkeys || {})) if (normKey(k) === n) return a;
  return '';
}
// captureKey — окно «нажмите клавишу». single — одна клавиша (кнопка рации: можно Shift/Ctrl/кнопки мыши).
// Возвращает строку клавиши, '' — «убрать», null — отмена.
export function captureKey({title = _t('Назначить клавишу'), hint, single = false, current = '', action = '', clear = true} = {}) {
  return new Promise(res => {
    api('/api/hotkeys/pause', {on: true}).catch(() => {});
    let value = null;
    const show = h('div', {class: 'capture'});
    const info = h('div', {class: 'hint', style: 'text-align:center;min-height:20px'});
    const okBtn = h('button', {class: 'btn primary', disabled: true, onclick: () => d.close(value)}, _t('Сохранить'));
    const paint = (txt, pending) => {
      show.replaceChildren(txt ? h('span', {style: pending ? 'opacity:.55' : ''}, txt) : h('span', {class: 'faint', style: 'font-size:16px;font-weight:500'}, single ? _t('Нажмите клавишу или боковую кнопку мыши…') : _t('Нажмите сочетание клавиш…')));
    };
    const pick = k => {
      value = k; paint(keyLabel(k)); okBtn.disabled = false;
      const owner = !single && hotkeyOwner(k);
      info.className = 'hint' + (owner && owner !== action ? ' warn' : '');
      info.textContent = owner && owner !== action ? _t('⚠ Сейчас эта клавиша: ') + actionLabel(owner) + _t('. Она перейдёт сюда.') : (owner === action && owner ? _t('Уже назначена сюда') : '');
    };
    paint(current ? keyLabel(current) : '', !!current);
    const kd = e => {
      e.preventDefault(); e.stopPropagation();
      if (e.code === 'Escape') { d.close(null); return; }
      const k = codeName(e);
      if (!k) { info.className = 'hint warn'; info.textContent = _t('Эта клавиша не поддерживается — выберите другую'); return; }
      if (single) { pick(k); return; }
      if (/Shift|Ctrl|Alt/.test(k)) { paint([e.ctrlKey && 'Ctrl', e.altKey && 'Alt', e.shiftKey && 'Shift'].filter(Boolean).join('+') + '+…', true); return; }
      pick([e.ctrlKey && 'Ctrl', e.altKey && 'Alt', e.shiftKey && 'Shift', e.metaKey && 'Win', k].filter(Boolean).join('+'));
    };
    const md = e => {
      const m = {1: 'Mouse3', 3: 'Mouse4', 4: 'Mouse5'}[e.button];
      if (!m) return; // левая и правая — обычные щелчки по окну
      e.preventDefault(); e.stopPropagation();
      pick(single ? m : [e.ctrlKey && 'Ctrl', e.altKey && 'Alt', e.shiftKey && 'Shift', m].filter(Boolean).join('+'));
    };
    const stopCtx = e => e.preventDefault();
    window.addEventListener('keydown', kd, true);
    window.addEventListener('mousedown', md, true);
    window.addEventListener('auxclick', stopCtx, true);
    const d = dialog({title, onClose: v => {
      window.removeEventListener('keydown', kd, true);
      window.removeEventListener('mousedown', md, true);
      window.removeEventListener('auxclick', stopCtx, true);
      api('/api/hotkeys/pause', {on: false}).catch(() => {});
      res(v === undefined ? null : v);
    }, onKey: () => false,
    body: [hint && h('p', {class: 'hint', style: 'margin:0'}, hint), show, info],
    foot: [clear && current ? h('button', {class: 'btn ghost', onclick: () => d.close('')}, _t('Убрать клавишу')) : null, h('div', {class: 'grow'}),
      h('button', {class: 'btn', onclick: () => d.close(null)}, _t('Отмена')), okBtn]});
  });
}
// bindKey — назначить клавишу действию через окно захвата
export async function bindKey(action, {title, hint} = {}) {
  const cur = (app.state.hotkeys || {})[action] || '';
  const k = await captureKey({title: title || _t('Клавиша: ') + actionLabel(action), hint, current: cur, action});
  if (k === null) return;
  await act('/api/hotkey', {action, key: k}, {ok: k ? keyLabel(k) + ' → ' + actionLabel(action) : _t('Клавиша убрана')});
}

// ---------- подписи ----------
export const soundById = id => (app.state.sounds || []).find(s => s.id === id);
export const presetById = id => (app.state.presets || []).find(p => p.id === id);
const BUS = {both: _t('вам и в эфир'), voice: _t('только в эфир'), monitor: _t('только вам')};
export function actionLabel(act) {
  if (!act) return '';
  const fixed = {menu: _t('Быстрое меню'), wheel: _t('Колесо звуков'), stop: _t('Остановить все звуки'), ui: _t('Открыть окно VoiceBox'), mic_mute: _t('Микрофон: выкл/вкл'),
    mic_monitor: _t('Слышать себя'), talk: _t('Голос в эфир: вкл/выкл'), 'preset:next': _t('Следующий эффект'), 'preset:prev': _t('Предыдущий эффект'),
    rosh: _t('Рошан убит'), clock_horn: _t('Горн: игра началась (0:00)'), clock_clear: _t('Сбросить ручное время'), reload: _t('Перечитать настройки'),
    say: _t('Поиск и фраза команде поверх игры'), denoise: _t('Шумоподавление: вкл/выкл'), bg: _t('Фон под голос: вкл/выкл'),
    'set:next': _t('Следующий набор звуков'), 'set:prev': _t('Предыдущий набор звуков'), 'set:all': _t('Набор «Все звуки»'), 'mic:next': _t('Следующий микрофон'),
    reader: _t('Читалка поверх игры'), 'reader:repeat': _t('Читалка: повторить фразу'), 'reader:stop': _t('Читалка: замолчать'),
    'reader:next': _t('Читалка: следующий голос'), chat_read: _t('Чтение чата Dota 2: вкл/выкл')};
  if (fixed[act]) return fixed[act];
  if (act.startsWith('set:')) { const s = app.state && (app.state.config.sets || []).find(x => x.id === act.slice(4)); return _tf('Набор «{0}»', (s ? s.name : act.slice(4))); }
  if (act.startsWith('sound:')) {
    const [id, bus] = act.slice(6).split('@');
    const s = app.state && soundById(id);
    return _tf('Звук «{0}»', (s ? s.title : id)) + (bus && BUS[bus] ? ' — ' + BUS[bus] : '');
  }
  if (act.startsWith('preset:')) { const p = app.state && presetById(act.slice(7)); return _tf('Эффект «{0}»', (p ? p.title : act.slice(7))); }
  if (act.startsWith('clock_sync:')) return _t('Игровое время = ') + act.slice(11);
  if (act.startsWith('source_mute:')) return _t('Источник: выкл/вкл');
  return act;
}

// ---------- форматирование ----------
export function fmtTime(s) {
  if (s == null || isNaN(s)) return '--:--';
  const neg = s < 0; s = Math.abs(Math.round(s));
  return (neg ? '−' : '') + Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
}
export function parseTime(v) {
  v = String(v ?? '').trim().replace('−', '-');
  if (!v) return null;
  const neg = v.startsWith('-'); if (neg) v = v.slice(1);
  let n;
  if (v.includes(':')) { const [m, s] = v.split(':'); n = (+m || 0) * 60 + (+s || 0); } else n = +v;
  if (isNaN(n)) return null;
  return neg ? -n : n;
}
export const fmtDur = s => !s ? '' : s < 60 ? (s < 10 ? dec(s.toFixed(1)) : Math.round(s)) + _t(' с') : fmtTime(s);
export const pct = v => Math.round((v || 0) * 100) + '%';
export function plural(n, one, few, many) {
  if (EN) return Math.abs(n) === 1 ? one : many;
  const a = Math.abs(n) % 100, b = a % 10;
  if (a > 10 && a < 20) return many;
  if (b > 1 && b < 5) return few;
  if (b === 1) return one;
  return many;
}
export const secText = n => n + ' ' + plural(n, _t('секунда'), _t('секунды'), _t('секунд'));

// ---------- устройства ----------
const VIRT = /vb-audio|voicemeeter|vaio|virtual|\bcable\b|\bvac\b|^line ?\d+\s*(\(|$)/i;
export function isVirt(name) {
  if (!name) return false;
  if (VIRT.test(name.trim())) return true;
  const s = app.state; if (!s) return false;
  return [s.config.devices.voice_out, s.config.devices.cable_rec, s.devices.cable, s.devices.cable_rec].some(n => n && (name === n || name.startsWith(n + ' (')));
}
export const shortDev = n => (n || '').replace(/\s*\((?!.*\().*\)$/, '').trim() || n;
export const cableRec = () => (app.state && app.state.devices.cable_rec) || 'CABLE Output';
export const cableIn = () => (app.state && app.state.devices.cable) || 'CABLE Input';

// ---------- элементы управления ----------
export function toggle(checked, onchange, {big, label, title, disabled} = {}) {
  const inp = h('input', {type: 'checkbox', checked: !!checked, disabled, onchange: e => onchange(e.target.checked, e)});
  return h('label', {class: 'sw' + (big ? ' big' : ''), title}, inp, h('i'), label ? h('span', {}, label) : null);
}
export function seg(options, current, onchange, {accent, cls = ''} = {}) {
  const el = h('div', {class: 'seg' + (accent ? ' accent' : '') + (cls ? ' ' + cls : '')});
  for (const [v, l, t] of options) {
    const b = h('button', {type: 'button', class: String(v) === String(current) ? 'on' : '', title: t || null, onclick: () => {
      for (const x of el.children) x.classList.toggle('on', x === b);
      onchange(v);
    }}, l);
    el.append(b);
  }
  return el;
}
// slider({label, value, min, max, step, fmt, onInput, onChange}) — onChange вызывается с задержкой
export function slider({label, value, min = 0, max = 1, step = 0.01, fmt = v => v, onInput, onChange, delay = 250, title}) {
  const out = h('span', {class: 'sl-v'}, fmt(+value));
  const inp = h('input', {type: 'range', min, max, step, value});
  const paint = () => inp.style.setProperty('--p', ((inp.value - min) / (max - min) * 100) + '%');
  const send = onChange ? debounced(onChange, delay) : null;
  inp.addEventListener('input', () => { const v = +inp.value; out.textContent = fmt(v); paint(); onInput && onInput(v); send && send(v); });
  inp.addEventListener('change', () => send && send.flush());
  paint();
  return h('div', {class: 'slider', title}, h('span', {class: 'sl-l'}, label), out, inp);
}
export function select(options, current, onchange, {cls = 'input', title} = {}) {
  const el = h('select', {class: cls, title, onchange: e => onchange(e.target.value)});
  for (const o of options) {
    if (o.group) { const g = h('optgroup', {label: o.group}); for (const [v, l] of o.items) g.append(h('option', {value: v, selected: v === current}, l)); el.append(g); }
    else { const [v, l] = o; el.append(h('option', {value: v, selected: v === current}, l)); }
  }
  return el;
}
export const setrow = (title, sub, ...ctl) => h('div', {class: 'setrow'}, h('div', {class: 'txt'}, h('b', {}, title), sub ? h('small', {}, sub) : null), h('div', {class: 'ctl'}, ctl));
export const card = (title, ...kids) => h('div', {class: 'card'}, title ? (typeof title === 'string' ? h('h3', {}, title) : title) : null, kids);
export const cardH = (title, ...right) => h('div', {class: 'card-h'}, h('h3', {}, title), h('div', {class: 'grow'}), right);
export const sectionH = (title, hint, ...right) => h('div', {class: 'section-h'}, h('h2', {}, title), hint ? h('span', {class: 'hint'}, hint) : null, h('div', {class: 'grow'}), right);
export const hint = (text, kind = '') => h('div', {class: 'hint box' + (kind ? ' ' + kind : '')}, text);

// индикатор уровня: обновляется из main.js по data-meter
export function meter(id, {big, title} = {}) {
  return h('div', {class: 'meter' + (big ? ' big-meter' : ''), 'data-meter': id, title}, h('i'), h('b'));
}
const vu = {};
export const vuPos = l => l > 0 ? Math.max(0, Math.min(100, (20 * Math.log10(l) + 60) / 60 * 100)) : 0;
export function vuValue(id, raw) {
  const v = vu[id] || (vu[id] = {lvl: 0, hold: 0, holdT: 0, clipT: 0});
  const now = performance.now();
  v.lvl = Math.max(raw, v.lvl * 0.8);
  if (raw >= v.hold || now - v.holdT > 1500) { v.hold = raw; v.holdT = now; }
  if (raw >= 0.99) v.clipT = now;
  return {pos: vuPos(v.lvl), hold: vuPos(v.hold), clip: now - v.clipT < 1500};
}
export function paintMeters(levels) {
  for (const el of document.querySelectorAll('[data-meter]')) {
    const id = el.dataset.meter;
    const raw = id.startsWith('src_') ? ((levels.strips || {})[id.slice(4)] || 0) : (levels[id] || 0);
    const v = vuValue(id, raw);
    const i = el.firstChild, b = el.lastChild;
    i.style.width = v.pos + '%';
    i.style.setProperty('--mw', el.clientWidth + 'px');
    if (b && b.tagName === 'B') b.style.left = 'calc(' + v.hold + '% - 1px)';
    el.classList.toggle('clip', v.clip);
  }
}

// ---------- эмодзи и цвета ----------
export const EMOJI = ('🔊 📯 🎺 🥁 🪘 🎷 🎸 🎹 🎻 🪗 🎵 🎶 🎤 🎧 📢 📣 🔔 🛎️ ⏰ ⏱️ 🚨 ⚠️ ❗ ❓ ‼️ ⁉️ ✅ ❌ ⛔ 🚫 🆘 💯 ' +
  '😂 🤣 😅 😆 😎 🤓 🥳 😇 😈 👿 😱 😨 😰 😭 🥺 😤 😡 🤬 🤯 😴 🤫 🤔 🙃 😏 😬 🫠 🥶 🥵 🤢 🤮 🤡 💀 ☠️ 👻 👽 🤖 👹 👺 🗿 💩 ' +
  '👍 👎 👏 🙏 💪 🤝 🫡 👋 ✋ 👀 🙈 🫵 👑 💎 💰 🏆 🥇 🎯 🎲 🎮 🕹️ 🎉 🎊 🎁 🍿 🍺 ☕ ' +
  '🔥 💥 ⚡ 💣 🧨 💫 ✨ 🌟 ⭐ ☄️ 🌪️ 🌊 🌫️ ❄️ ☀️ 🌙 🌈 ❤️ 💔 💜 💙 💚 💛 🖤 ' +
  '⚔️ 🗡️ 🏹 🛡️ 🪓 🔫 🧙 🧛 🧟 🥷 🐲 🐉 🐺 🦁 🐸 🐷 🐔 🦆 🐐 🐄 🐱 🐶 🐒 🐍 🦖 🦗 🐝 🦀 🐿️ ' +
  '🚀 🛸 📻 ☎️ 📞 📡 🎙️ 🗣️ 💬 🧠 🫀 🩸 💊 🧪 🔮 🪄 🧿 🕳️ 🏟️ 🌀 🪖 👮 🚓 🚑 🚒 ✈️ 🚂 🏁').split(' ');
export function emojiPicker(current, onpick) {
  const grid = h('div', {class: 'emoji-pick'});
  const paint = cur => { for (const b of grid.children) b.classList.toggle('on', b.textContent === cur); };
  for (const e of EMOJI) grid.append(h('button', {type: 'button', title: e, onclick: () => { paint(e); onpick(e); }}, e));
  paint(current);
  return grid;
}
export function colorPicker(current, onpick) {
  const el = h('div', {class: 'colors'});
  for (const c of app.state.palette || []) {
    el.append(h('button', {type: 'button', style: {'--c': c}, class: c === current ? 'on' : '', title: c, onclick: () => {
      for (const b of el.children) b.classList.toggle('on', b.title === c); onpick(c);
    }}));
  }
  return el;
}

// ---------- прочее ----------
export const clone = o => JSON.parse(JSON.stringify(o));
export function go(page) { window.dispatchEvent(new CustomEvent('vb-go', {detail: page})); }
export function playSound(id, to) { return api('/api/play', to ? {id, to} : {id}).catch(e => toast(e.message, 'err')); }
export const empty = (big, title, text, ...actions) => h('div', {class: 'empty'}, h('div', {class: 'big'}, big), h('h2', {}, title), text ? h('p', {}, text) : null, actions.length ? h('div', {class: 'row', style: 'justify-content:center'}, actions) : null);
