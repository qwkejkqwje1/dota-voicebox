'use strict';
// похоже на виртуальный кабель (VB-Cable, Voicemeeter, Virtual Audio Cable «Line 1» и др.) — как audio.IsVirtual
const VIRT = /vb-audio|voicemeeter|vaio|virtual|\bcable\b|\bvac\b|^line ?\d+\s*(\(|$)/i;
const isVirt = name => VIRT.test((name || '').trim()) || (S && [S.config.devices.voice_out, S.config.devices.cable_rec, S.devices.cable, S.devices.cable_rec].some(n => n && (name === n || (name || '').startsWith(n + ' ('))));
const shortDev = n => (n || '').replace(/\s*\(.*\)$/, '');
const cableRecName = () => (S && S.devices.cable_rec) || 'CABLE Output';
const cableName = () => (S && S.devices.cable) || 'CABLE Input';
const TOKEN = new URLSearchParams(location.search).get('t') || '';
let S = null;              // полное состояние (/api/state)
let ST = null;             // живой статус
let LV = {mic_in:0, voice_out:0, monitor:0};
let page = 'home';

window.addEventListener('error', e => toast('Ошибка интерфейса: ' + e.message, true));
window.addEventListener('unhandledrejection', e => toast('Ошибка интерфейса: ' + (e.reason && e.reason.message || e.reason), true));
// ---------- утилиты ----------
function h(tag, attrs, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v == null || v === false) continue;
    if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
    else if (k === 'class') el.className = v;
    else if (k === 'style') el.style.cssText = v;
    else if (k in el && k !== 'list') el[k] = v;
    else el.setAttribute(k, v === true ? '' : v);
  }
  for (const k of kids.flat(9)) if (k != null && k !== false) el.append(k instanceof Node ? k : String(k));
  return el;
}
async function api(method, path, body) {
  const opt = {method, headers: {'X-Token': TOKEN}};
  if (body instanceof FormData) opt.body = body;
  else if (body !== undefined) { opt.body = JSON.stringify(body); opt.headers['Content-Type'] = 'application/json'; }
  const r = await fetch(path, opt);
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(j.error || r.statusText);
  return j;
}
function toast(msg, err) {
  const d = h('div', {class: err ? 'err' : ''}, msg);
  document.getElementById('toast').append(d);
  setTimeout(() => d.remove(), err ? 6000 : 2600);
}
const fmt = s => { const n = s < 0; s = Math.abs(s); return (n ? '-' : '') + Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0'); };
const clone = o => JSON.parse(JSON.stringify(o));
const busName = {both: 'войс + вы', voice: 'только войс', monitor: 'только вы'};

// изменение конфига: mut(c) → сохранить → перерисовать
let saveTimer = null, pendingCfg = null, lastSave = 0;
function edit(mut, {debounce = 0, rerender = true} = {}) {
  pendingCfg = pendingCfg || clone(S.config);
  mut(pendingCfg);
  if (!pendingCfg.sounds) pendingCfg.sounds = {};
  clearTimeout(saveTimer);
  saveTimer = setTimeout(async () => {
    const c = pendingCfg; pendingCfg = null; lastSave = Date.now();
    try { S = await api('PUT', '/api/config', c); if (rerender) render(); }
    catch (e) { toast('Не сохранено: ' + e.message, true); await load(); }
  }, debounce);
}
async function load() { S = await api('GET', '/api/state'); ST = S.status; render(); }

function hotkeysFor(action) {
  return Object.entries(S.config.hotkeys || {}).filter(([, a]) => a === action || a.startsWith(action + '@')).map(([k]) => k);
}
const kbd = keys => keys.map(k => h('span', {class: 'kbd'}, k));

// ---------- захват клавиши ----------
const codeMap = {Backquote: 'Tilde', Space: 'Space', Tab: 'Tab', Enter: 'Enter', Escape: 'Esc', Backspace: 'Backspace',
  Insert: 'Insert', Delete: 'Delete', Home: 'Home', End: 'End', PageUp: 'PageUp', PageDown: 'PageDown', Pause: 'Pause',
  ScrollLock: 'ScrollLock', CapsLock: 'CapsLock', ArrowUp: 'Up', ArrowDown: 'Down', ArrowLeft: 'Left', ArrowRight: 'Right',
  NumpadAdd: 'NumAdd', NumpadSubtract: 'NumSub', NumpadMultiply: 'NumMul', NumpadDivide: 'NumDiv', NumpadDecimal: 'NumDot',
  ShiftLeft: 'LShift', ShiftRight: 'RShift', ControlLeft: 'LCtrl', ControlRight: 'RCtrl', AltLeft: 'LAlt', AltRight: 'RAlt'};
function keyName(e) {
  const c = e.code;
  if (/^Key[A-Z]$/.test(c)) return c.slice(3);
  if (/^Digit\d$/.test(c)) return c.slice(5);
  if (/^Numpad\d$/.test(c)) return 'Num' + c.slice(6);
  if (/^F\d+$/.test(c)) return c;
  return codeMap[c] || null;
}
// single=true — одна клавиша без модификаторов (для PTT, можно Mouse4/5 и Shift/Ctrl)
function captureKey(title, single) {
  return new Promise(resolve => {
    api('POST', '/api/hotkeys/pause', {on: true}).catch(() => {});
    const modal = document.getElementById('modal'), body = document.getElementById('modalBody');
    const show = h('div', {class: 'capture'}, '…');
    body.replaceChildren(h('h3', {}, title), h('p', {class: 'mute'}, single ? 'Нажмите клавишу или боковую кнопку мыши' : 'Нажмите сочетание (например Num1, F9, Ctrl+Alt+S)'), show,
      h('div', {class: 'row', style: 'justify-content:center'}, h('button', {onclick: () => done(null)}, 'Отмена')));
    modal.classList.remove('hidden');
    function done(v) {
      window.removeEventListener('keydown', kd, true); window.removeEventListener('mousedown', md, true);
      modal.classList.add('hidden');
      api('POST', '/api/hotkeys/pause', {on: false}).catch(() => {});
      resolve(v);
    }
    function kd(e) {
      e.preventDefault(); e.stopPropagation();
      if (e.code === 'Escape') return done(null);
      const k = keyName(e);
      if (!k) return;
      if (single) return done(k);
      if (/Shift|Ctrl|Alt/.test(k)) { show.textContent = [e.ctrlKey && 'Ctrl', e.altKey && 'Alt', e.shiftKey && 'Shift'].filter(Boolean).join('+') + '+…'; return; }
      const combo = [e.ctrlKey && 'Ctrl', e.altKey && 'Alt', e.shiftKey && 'Shift', e.metaKey && 'Win', k].filter(Boolean).join('+');
      show.textContent = combo; setTimeout(() => done(combo), 250);
    }
    function md(e) {
      if (!single) return;
      if (e.button === 3) { e.preventDefault(); done('Mouse4'); }
      if (e.button === 4) { e.preventDefault(); done('Mouse5'); }
      if (e.button === 1) { e.preventDefault(); done('Mouse3'); }
    }
    window.addEventListener('keydown', kd, true); window.addEventListener('mousedown', md, true);
  });
}
function bindHotkey(action, combo) {
  const prev = S.config.hotkeys[combo];
  if (prev && prev !== action && !confirm(`${combo} уже назначена на «${prev}». Заменить?`)) return;
  edit(c => {
    for (const [k, a] of Object.entries(c.hotkeys)) if (a === action) delete c.hotkeys[k];
    c.hotkeys[combo] = action;
  });
  toast(`${combo} → ${action}`);
}

// ---------- шапка ----------
function renderPills() {
  if (!ST) return;
  const a = ST.audio, g = ST.game;
  const pills = [
    h('span', {class: 'pill ' + (a.running ? 'ok' : 'bad')}, a.running ? '🔊 Аудио работает' : '🔇 Аудио: ' + (a.error ? 'ошибка' : 'не запущено')),
    h('span', {class: 'pill ' + (g.connected ? 'ok' : '')}, g.connected ? `🎮 Dota ${g.has_clock ? fmt(g.clock) : ''}${g.paused ? ' ⏸' : ''}` : '🎮 Dota не подключена'),
    h('span', {class: 'pill ' + (ST.ptt_key ? '' : 'warn')}, '⌨️ PTT: ' + (ST.ptt_key || 'не задана')),
    h('span', {class: 'pill'}, '🎙 ' + ST.preset),
  ];
  if (ST.ptt_held) pills.push(h('span', {class: 'pill live'}, '● В ЭФИРЕ'));
  if (ST.mic_monitor) pills.push(h('span', {class: 'pill warn'}, '🎧 Слышу себя'));
  document.getElementById('pills').replaceChildren(...pills);
}

// ---------- страницы ----------
const pages = {};

// ---------- пульт ----------
const DOTA_CHECKS = ['dota', 'dota_input', 'gsi', 'launch', 'ptt', 'gsi_live'];
function meterEl(id, vertical) {
  return h('div', {class: 'vu'}, h('div', {class: 'meter' + (vertical ? ' v' : '')}, h('i', {id: 'm_' + id}), h('b', {class: 'hold', id: 'mh_' + id})),
    h('span', {class: 'db', id: 'md_' + id}, '-∞'));
}
// VU: шкала −60…0 dBFS, удержание пика 1.5 с, отметка перегруза 2 с
const VU = {};
const vuPos = l => l > 0 ? Math.max(0, Math.min(100, (20 * Math.log10(l) + 60) / 60 * 100)) : 0;
function vuSet(id, raw) {
  const v = VU[id] || (VU[id] = {lvl: 0, hold: 0, holdT: 0, clipT: 0});
  const now = performance.now();
  v.lvl = Math.max(raw, v.lvl * 0.85);
  if (raw >= v.hold || now - v.holdT > 1500) { v.hold = raw; v.holdT = now; }
  if (raw >= 0.99) v.clipT = now;
  const m = document.getElementById('m_' + id); if (!m) return;
  m.style.width = vuPos(v.lvl) + '%';
  const hd = document.getElementById('mh_' + id); if (hd) hd.style.left = 'calc(' + vuPos(v.hold) + '% - 2px)';
  const clip = now - v.clipT < 2000;
  m.parentNode.classList.toggle('clip', clip);
  const d = document.getElementById('md_' + id);
  if (d) { d.textContent = clip ? 'CLIP' : (v.hold > 0.001 ? (20 * Math.log10(v.hold)).toFixed(0) + ' dB' : '-∞'); d.classList.toggle('bad', clip); }
}
function strip({id, title, sub, icon, children, cls}) {
  return h('div', {class: 'strip ' + (cls || '')}, h('div', {class: 'sthead'}, h('span', {class: 'sticon'}, icon), h('div', {style: 'min-width:0'}, h('b', {}, title), h('small', {}, sub || ''))),
    meterEl(id), children);
}
function knob(label, value, min, max, step, onchange, fmtv) {
  const out = h('span', {class: 'mute'}, fmtv ? fmtv(value) : (+value).toFixed(2));
  return h('div', {class: 'knob'}, h('span', {}, label),
    h('input', {type: 'range', min, max, step, value, oninput: e => { out.textContent = fmtv ? fmtv(+e.target.value) : (+e.target.value).toFixed(2); onchange(+e.target.value); }}), out);
}
const gateFmt = v => v >= 0 ? 'выкл' : v + ' dB';
pages.home = () => {
  const c = S.config;
  const st = ST.strips || [];
  const micStrip = strip({id: 'mic_in', icon: '🎤', title: 'Микрофон', sub: (ST.audio.mic || 'не запущен') + ' → пресет «' + ST.preset + '»', cls: c.mic.mute ? 'muted' : '', children: [
    knob('Усиление', c.mic_gain, 0, 3, 0.05, v => edit(x => x.mic_gain = v, {debounce: 300, rerender: false})),
    knob('Гейт', c.mic.gate_db || 0, -80, 0, 1, v => { edit(x => x.mic.gate_db = v, {debounce: 300, rerender: false}); viz.gate = v; }, gateFmt),
    h('div', {class: 'row'},
      h('button', {class: 'small ' + (c.mic.mute ? 'on' : ''), onclick: () => api('POST', '/api/action', {action: 'mic_mute'}).then(load)}, c.mic.mute ? '🔇 Включить' : '🔇 Mute'),
      h('button', {class: 'small ' + (ST.mic_monitor ? 'on' : ''), onclick: () => api('POST', '/api/action', {action: 'mic_monitor'})}, '🎧 Слышать себя'),
      h('select', {class: 'small', onchange: e => api('POST', '/api/action', {action: 'preset:' + e.target.value})}, S.presets.map(p => h('option', {value: p.name, selected: ST.preset === p.name}, '🎛 ' + p.name))))]});
  const srcStrips = (c.sources || []).map((src, i) => {
    const s = st.find(x => x.id === src.id) || {};
    const set = (k, debounce) => v => edit(x => x.sources[i][k] = v, {debounce: debounce ? 300 : 0, rerender: !debounce});
    return strip({id: 'src_' + src.id, icon: src.kind === 'loopback' ? '🔁' : '🎚', title: src.label || src.id,
      sub: s.running ? s.device + (src.channel ? ' · вход ' + src.channel : '') : '⚠ ' + (s.error || 'не запущен'), cls: src.mute ? 'muted' : '', children: [
        knob('Громкость', src.gain, 0, 3, 0.05, set('gain', 1)),
        knob('В эфир', src.to_air, 0, 1.5, 0.05, set('to_air', 1)),
        knob('В наушники', src.to_mon, 0, 1.5, 0.05, set('to_mon', 1)),
        knob('Тише, когда говорю', src.duck || 0, 0, 1, 0.05, set('duck', 1)),
        h('div', {class: 'row'},
          h('button', {class: 'small ' + (src.mute ? 'on' : ''), onclick: () => edit(x => x.sources[i].mute = !x.sources[i].mute)}, src.mute ? '🔇 Включить' : '🔇 Mute'),
          s.xruns ? h('span', {class: 'mute', title: 'компенсация разницы часов устройств / опустошения буфера'}, `⏱ ${((s.ratio - 1) * 1e6).toFixed(0)} ppm · ${s.xruns} xrun`) : '',
          h('div', {class: 'grow'}),
          h('button', {class: 'small danger', onclick: () => confirm('Удалить источник?') && edit(x => x.sources.splice(i, 1))}, '🗑'))]});
  });
  const kind = h('select', {}, h('option', {value: 'input'}, '🎚 Вход (микрофон / аудиоинтерфейс)'), h('option', {value: 'loopback'}, '🔁 Звук устройства (музыка, браузер, игра)'));
  const dev = h('select', {});
  const fillDev = () => dev.replaceChildren(...(kind.value === 'loopback' ? S.devices.playback : S.devices.capture || []).filter(d => !isVirt(d.name)).map(d => h('option', {value: d.name}, d.name)));
  kind.onchange = fillDev; fillDev();
  const chan = h('input', {type: 'number', min: 0, max: 32, value: 0, title: '0 — все каналы; 1, 2… — конкретный вход аудиоинтерфейса'});
  const addCard = h('div', {class: 'strip add'}, h('b', {}, '＋ Источник'), kind, dev, h('div', {class: 'row'}, 'Канал', chan, h('span', {class: 'mute'}, '0 = все')),
    h('button', {class: 'primary', onclick: () => {
      const label = (dev.value || '').replace(/\s*\(.*\)$/, '').slice(0, 30) || 'Источник';
      edit(x => (x.sources = x.sources || []).push({id: 'src' + Date.now().toString(36), label, kind: kind.value, device: dev.value, channel: +chan.value, gain: 1, to_air: kind.value === 'loopback' ? 0.6 : 1, to_mon: kind.value === 'loopback' ? 0 : 0, duck: kind.value === 'loopback' ? 0.5 : 0}));
      toast('Источник добавлен: ' + label);
    }}, 'Добавить'));
  const bus = (id, icon, title, dev, vol) => h('div', {class: 'strip bus'}, h('div', {class: 'sthead'}, h('span', {class: 'sticon'}, icon), h('div', {}, h('b', {}, title), h('small', {}, dev || '—'))), meterEl(id), vol);
  return h('div', {},
    h('div', {class: 'mixer'}, micStrip, srcStrips, addCard,
      h('div', {class: 'busses'},
        bus('voice_out', '📡', 'Эфир', (ST.audio.voice_out || 'кабель не найден') + ' → Discord / Dota / OBS',
          [knob('Звуки в эфир', c.sfx_volume, 0, 2, 0.05, v => edit(x => x.sfx_volume = v, {debounce: 300, rerender: false})),
            knob('Голос во время звука', c.ducking, 0, 1, 0.05, v => edit(x => x.ducking = v, {debounce: 300, rerender: false}))]),
        bus('monitor', '🎧', 'Мониторинг', ST.audio.monitor,
          knob('Громкость', c.monitor_volume, 0, 1.5, 0.05, v => edit(x => x.monitor_volume = v, {debounce: 300, rerender: false}))))),
    vizCard());
}

// ---------- визуализация ----------
const viz = {ws: null, post: false, gate: 0, frames: [], spectro: null, canvas: null};
function vizCard() {
  viz.gate = S.config.mic.gate_db || 0;
  const cv = h('canvas', {id: 'vizc', width: 1200, height: 300});
  viz.canvas = cv;
  const mode = h('div', {class: 'seg'},
    h('button', {class: viz.post ? '' : 'on', onclick: e => { viz.post = false; viz.ws && viz.ws.readyState === 1 && viz.ws.send('pre'); e.target.parentNode.childNodes.forEach(b => b.classList.toggle('on', b === e.target)); }}, 'До эффектов'),
    h('button', {class: viz.post ? 'on' : '', onclick: e => { viz.post = true; viz.ws && viz.ws.readyState === 1 && viz.ws.send('post'); e.target.parentNode.childNodes.forEach(b => b.classList.toggle('on', b === e.target)); }}, 'В эфире (после)'));
  // перетаскивание порога гейта по осциллограмме
  let drag = false;
  const setGate = ev => {
    const r = cv.getBoundingClientRect(); const y = (ev.clientY - r.top) / r.height; // 0..1 в области осциллограммы (верх 45%)
    const amp = Math.max(0.0001, Math.min(1, Math.abs(0.5 - y / 0.45 * 0.45 / 0.45 * 0.5) * 2));
    const yy = Math.min(0.45, Math.max(0, y)) / 0.45; const a = Math.abs(yy - 0.5) * 2;
    let db = Math.round(20 * Math.log10(Math.max(a, 0.0001)));
    if (db > -3) db = 0; if (db < -80) db = -80;
    viz.gate = db; edit(x => x.mic.gate_db = db, {debounce: 300, rerender: false});
    void amp;
  };
  cv.addEventListener('mousedown', e => { if ((e.offsetY / cv.clientHeight) < 0.45) { drag = true; setGate(e); } });
  window.addEventListener('mousemove', e => drag && setGate(e));
  window.addEventListener('mouseup', () => drag = false);
  setTimeout(vizStart);
  return h('div', {class: 'card', style: 'margin-top:16px'},
    h('div', {class: 'row', style: 'margin-bottom:8px'}, h('h3', {style: 'margin:0'}, 'Голос'), mode, h('span', {id: 'gateInd', class: 'pill'}, 'гейт'), h('div', {class: 'grow'}),
      h('span', {class: 'mute', style: 'font-size:12px'}, 'Осциллограмма · спектр · водопад. Тяните мышью по осциллограмме — порог гейта')), cv);
}
function vizStart() {
  if (viz.ws && viz.ws.readyState <= 1) return;
  const ws = new WebSocket(`ws://${location.host}/api/viz?t=${encodeURIComponent(TOKEN)}`);
  ws.binaryType = 'arraybuffer'; viz.ws = ws;
  ws.onopen = () => { ws.send(viz.post ? 'post' : 'pre'); ws.send('fps:30'); };
  ws.onmessage = e => { if (page === 'home') vizDraw(new Uint8Array(e.data)); };
  ws.onclose = () => { viz.ws = null; if (page === 'home') setTimeout(vizStart, 1500); };
}
function vizStop() { if (viz.ws) { const w = viz.ws; viz.ws = null; w.onclose = null; w.close(); } }
function vizDraw(d) {
  const cv = document.getElementById('vizc'); if (!cv) return;
  const W = cv.clientWidth | 0, H = cv.clientHeight | 0;
  if (cv.width !== W || cv.height !== H) { cv.width = W; cv.height = H; viz.spectro = null; }
  const g = cv.getContext('2d');
  const cols = 256, bands = 96, gateOpen = d[1] === 1;
  const wave = d.subarray(2, 2 + cols * 2), spec = d.subarray(2 + cols * 2, 2 + cols * 2 + bands), pk = d[2 + cols * 2 + bands];
  const oh = H * 0.45, sh = H * 0.27, wy = oh + sh;
  g.fillStyle = '#0b0d11'; g.fillRect(0, 0, W, wy);
  // сетка
  g.strokeStyle = '#1c2029'; g.lineWidth = 1; g.beginPath(); g.moveTo(0, oh / 2); g.lineTo(W, oh / 2); g.stroke();
  // порог гейта
  if (viz.gate < 0) {
    const a = Math.pow(10, viz.gate / 20) * oh / 2;
    g.fillStyle = 'rgba(232,179,58,.08)'; g.fillRect(0, oh / 2 - a, W, a * 2);
    g.strokeStyle = 'rgba(232,179,58,.7)'; g.setLineDash([4, 4]); g.beginPath(); g.moveTo(0, oh / 2 - a); g.lineTo(W, oh / 2 - a); g.moveTo(0, oh / 2 + a); g.lineTo(W, oh / 2 + a); g.stroke(); g.setLineDash([]);
    g.fillStyle = '#e8b33a'; g.font = '11px Consolas'; g.fillText('гейт ' + viz.gate + ' dB', 6, Math.max(12, oh / 2 - a - 4));
  }
  // осциллограмма
  const sx = W / cols;
  const grad = g.createLinearGradient(0, 0, 0, oh); grad.addColorStop(0, '#ff6b4a'); grad.addColorStop(0.5, '#3fbf6f'); grad.addColorStop(1, '#ff6b4a');
  g.fillStyle = gateOpen ? grad : '#3a4150';
  for (let i = 0; i < cols; i++) {
    const mn = (wave[i * 2] << 24 >> 24) / 127, mx = (wave[i * 2 + 1] << 24 >> 24) / 127;
    const y1 = oh / 2 - mx * oh / 2, y2 = oh / 2 - mn * oh / 2;
    g.fillRect(i * sx, y1, Math.max(1, sx - 0.5), Math.max(1, y2 - y1));
  }
  // спектр
  const bw = W / bands;
  for (let b = 0; b < bands; b++) {
    const v = spec[b] / 255, bh = v * (sh - 14);
    g.fillStyle = `hsl(${200 - v * 200},80%,${35 + v * 25}%)`;
    g.fillRect(b * bw + 1, oh + sh - bh - 12, bw - 2, bh);
  }
  g.fillStyle = '#5b6375'; g.font = '10px Consolas';
  [['100', 0.15], ['500', 0.42], ['1k', 0.54], ['4k', 0.77], ['10k', 0.92]].forEach(([t, x]) => g.fillText(t, x * W, oh + sh - 1));
  // водопад (спектрограмма): сдвиг вниз на 1 строку
  const wh = H - wy;
  if (!viz.spectro) viz.spectro = g.createImageData(W, Math.max(1, wh | 0));
  const img = viz.spectro, row = W * 4;
  img.data.copyWithin(row, 0, img.data.length - row);
  for (let x = 0; x < W; x++) {
    const v = spec[Math.min(bands - 1, (x / W * bands) | 0)] / 255;
    const i = x * 4, r = v > 0.6 ? 255 : v * 400, gg = v > 0.3 ? Math.min(255, (v - 0.3) * 500) : 0, bb = v < 0.5 ? v * 400 : Math.max(0, 255 - (v - 0.5) * 600);
    img.data[i] = r; img.data[i + 1] = gg; img.data[i + 2] = bb; img.data[i + 3] = 255;
  }
  g.putImageData(img, 0, wy);
  // пиковый уровень и индикатор гейта
  g.fillStyle = pk > 240 ? '#e0453a' : '#8b93a3'; g.font = '12px Consolas';
  g.fillText('пик ' + (pk ? (20 * Math.log10(pk / 255)).toFixed(0) : '-∞') + ' dB', W - 90, 14);
  const gi = document.getElementById('gateInd');
  if (gi) { gi.textContent = viz.gate < 0 ? (gateOpen ? '● гейт открыт' : '○ гейт закрыт') : 'гейт выключен'; gi.className = 'pill ' + (viz.gate < 0 ? (gateOpen ? 'ok' : '') : ''); }
}

// ---------- саундборд ----------
pages.board = () => {
  const presetBtns = S.presets.map(p => h('button', {class: 'tile' + (ST.preset === p.name ? ' on' : ''), onclick: () => api('POST', '/api/action', {action: 'preset:' + p.name})},
    h('b', {}, p.name), h('small', {}, p.desc || 'свой пресет'), ...kbd(hotkeysFor('preset:' + p.name))));
  const rank = s => (hotkeysFor('sound:' + s.id).length ? 0 : 2) + (s.label ? 0 : 1);
  const all = S.sounds.slice().sort((a, b) => rank(a) - rank(b));
  const soundBtns = all.map(s => h('button', {class: 'tile pad', onclick: () => api('POST', '/api/action', {action: 'sound:' + s.id})},
    h('b', {}, s.label || s.id), h('small', {}, busName[s.bus] + ' · ' + s.duration + ' с'), ...kbd(hotkeysFor('sound:' + s.id))));
  return h('div', {class: 'grid'},
    h('div', {class: 'card', style: 'grid-column:1/-1'}, h('div', {class: 'row', style: 'margin-bottom:10px'}, h('h3', {style: 'margin:0'}, 'Звуки'), h('div', {class: 'grow'}),
      h('button', {onclick: () => api('POST', '/api/action', {action: 'stop'})}, '⏹ Стоп всё'), ...kbd(hotkeysFor('stop')), h('button', {onclick: () => go('sounds')}, '＋ Добавить звуки')),
      h('div', {class: 'btns'}, soundBtns)),
    h('div', {class: 'card', style: 'grid-column:1/-1'}, h('h3', {}, 'Голос'), h('div', {class: 'btns'}, presetBtns)));
};

// ---------- интеграции ----------
let integ = 'dota';
pages.integrations = () => {
  const tabs = h('div', {class: 'seg', style: 'margin-bottom:14px'}, [['dota', '🎮 Dota 2'], ['discord', '💬 Discord']].map(([id, l]) =>
    h('button', {class: integ === id ? 'on' : '', onclick: () => { integ = id; render(); }}, l)));
  return h('div', {}, tabs, integ === 'dota' ? dotaPage() : discordPage());
};
function checkRows(filter) {
  if (!SETUP) { loadSetup(); return [h('p', {class: 'mute'}, 'Проверяю…')]; }
  return SETUP.checks.filter(filter).map(ch => h('div', {class: 'check'}, h('span', {class: 'ic'}, ch.ok ? '✅' : (ch.fix ? '⚠️' : (ch.id === 'gsi_live' ? '⏳' : '❌'))),
    h('div', {}, h('b', {}, ch.title), h('small', {}, ch.detail), ch.manual && h('small', {class: 'warn'}, ch.manual)),
    ch.fix && !ch.ok ? h('button', {class: 'primary small', onclick: () => fix(ch.fix)}, ch.fix_label) : h('span')));
}
function dotaPage() {
  return h('div', {},
    h('div', {class: 'grid'},
      h('div', {class: 'card'}, h('h3', {}, 'Игра'), h('div', {id: 'game'}), h('div', {class: 'row', style: 'margin-top:10px'},
        h('button', {onclick: () => api('POST', '/api/action', {action: 'rosh'})}, '🐉 Рошан убит'), ...kbd(hotkeysFor('rosh')),
        h('button', {title: 'Если Dota не подключена: нажмите в момент горна — таймеры и скрипты пойдут по ручному времени', onclick: () => api('POST', '/api/clock', {mode: 'horn'}).then(() => toast('Отсчёт с 0:00'))}, '📯 Горн 0:00'), ...kbd(hotkeysFor('clock_horn')),
        h('button', {onclick: () => { const t = prompt('Сколько сейчас на игровых часах? (например 12:34)'); if (t) api('POST', '/api/clock', {mode: 'sync', time: t}).then(() => toast('Время: ' + t)).catch(e => toast(e.message, true)); }}, '🕐 Синхр.'))),
      h('div', {class: 'card'}, h('h3', {}, 'Настройка Dota 2'), checkRows(c => DOTA_CHECKS.includes(c.id)), h('div', {class: 'mute', style: 'margin-top:8px'}, 'Микрофон в Dota 2:'), appCheck(/dota2/i, 'Dota 2'),
        h('div', {class: 'row', style: 'margin-top:10px'}, h('button', {onclick: loadSetup}, '🔄 Проверить снова'), h('button', {onclick: () => go('scripts')}, '⚡ Скрипт «Ганк на миде»')))),
    h('div', {style: 'margin-top:16px'}, timersContent()));
}
// какая программа слушает какое устройство записи (Windows audio sessions)
function appCheck(re, app) {
  const out = h('span', {class: 'mute'});
  const btn = h('button', {class: 'small', onclick: async () => {
    out.textContent = '…';
    try {
      const l = ((await api('GET', '/api/apps/capture')).sessions || []).filter(x => re.test(x.process));
      const onCable = l.find(x => isVirt(x.device));
      if (onCable) { out.className = 'ok'; out.textContent = `✅ ${app} слушает ${onCable.device.replace(/\s*\(.*\)$/, '')}${onCable.active ? '' : ' (сейчас не пишет)'}`; }
      else if (l.length) { out.className = 'bad'; out.textContent = `❌ ${app} слушает «${l[0].device.replace(/\s*\(.*\)$/, '')}» — выберите «${shortDev(cableRecName())}»`; }
      else { out.className = 'warn'; out.textContent = `⚠ ${app} сейчас не открыл микрофон — зайдите в голосовой канал или откройте «Голос и видео» и проверьте снова`; }
    } catch (e) { out.className = 'bad'; out.textContent = e.message; }
  }}, '🔍 Проверить');
  return h('div', {class: 'row', style: 'margin-top:6px'}, btn, out);
}
function listenersCard() {
  const box = h('div', {}, h('p', {class: 'mute'}, 'Нажмите «Обновить», чтобы увидеть, какие программы сейчас пишут звук и с какого устройства.'));
  const load = async () => {
    try {
      const l = (await api('GET', '/api/apps/capture')).sessions || [];
      box.replaceChildren(l.length ? h('table', {}, h('tr', {}, h('th', {}, 'Программа'), h('th', {}, 'Устройство'), h('th', {}, '')),
        l.map(x => h('tr', {}, h('td', {}, x.process || ('PID ' + x.pid)), h('td', {class: isVirt(x.device) ? 'ok' : ''}, x.device), h('td', {class: 'mute'}, x.active ? '● пишет' : '')))) :
        h('p', {class: 'mute'}, 'Сейчас ни одна программа не открыла микрофон.'));
    } catch (e) { box.replaceChildren(h('p', {class: 'bad'}, e.message)); }
  };
  return h('div', {class: 'card'}, h('div', {class: 'row'}, h('h3', {style: 'margin:0'}, 'Кто слушает микрофоны'), h('div', {class: 'grow'}), h('button', {class: 'small', onclick: load}, '🔄 Обновить')),
    h('p', {class: 'mute', style: 'font-size:12px'}, 'Зелёным — программы, которые получают звук VoiceBox через кабель. Если нужная программа слушает настоящий микрофон, эффекты и звуки до неё не дойдут.'), box);
}
function discordPage() {
  const cable = {name: cableRecName()};
  const step = (n, title, text, extra) => h('div', {class: 'check'}, h('span', {class: 'ic'}, n), h('div', {}, h('b', {}, title), h('small', {}, text)), extra || h('span'));
  return h('div', {class: 'grid'},
    h('div', {class: 'card'}, h('h3', {}, 'Discord — настройка за минуту'),
      step('1', 'Устройство ввода: ' + (cable ? cable.name : 'CABLE Output'), 'Discord → Настройки → Голос и видео → «Устройство ввода». Так Discord услышит ваш голос с эффектами и звуки.',
        h('button', {class: 'primary small', onclick: () => api('POST', '/api/open', {what: 'discord_voice'}).catch(e => toast(e.message, true))}, 'Открыть настройки')),
      appCheck(/discord/i, 'Discord'),
      step('2', 'Устройство вывода: ваши наушники', 'Не выбирайте ' + shortDev(cableName()) + ' — иначе собеседники услышат сами себя.'),
      step('3', 'Шумоподавление Discord (Krisp) — выключить', 'Если у вас включён гейт или пресет с обработкой: Krisp режет эффекты и звуки саундборда. Эхоподавление тоже лучше выключить.'),
      step('4', 'Чувствительность ввода — вручную', 'Отключите «Автоматически определять» и поставьте порог около −50 dB, иначе тихие звуки обрежутся.'),
      step('5', 'Режим рации (по желанию)', 'Если в Discord включена «Рация», задайте ту же клавишу в «Настройка → Кнопка голосового чата» — VoiceBox будет нажимать её сам, когда играет звук.'),
      h('div', {class: 'row', style: 'margin-top:12px'}, h('button', {onclick: () => { go('test'); }}, '🩺 Проверить: «Как меня слышит команда»'))),
    h('div', {class: 'card'}, h('h3', {}, 'Другие программы'),
      h('p', {class: 'mute'}, 'Принцип тот же для любой программы: микрофон = ', h('b', {}, cable ? cable.name : 'CABLE Output'), '.'),
      h('table', {}, [['TeamSpeak', 'Настройки → Захват → Устройство захвата'], ['Telegram', 'Настройки → Звонки → Микрофон'], ['OBS', 'Источник «Захват входного аудиопотока» → CABLE Output'], ['CS2', 'Windows → Звук → устройство ввода по умолчанию (игра берёт системное)'], ['Dota 2', 'вкладка «Dota 2» — настраивается автоматически']].map(([a, b]) => h('tr', {}, h('td', {}, h('b', {}, a)), h('td', {class: 'mute'}, b))))), listenersCard());
}

function renderGame() {
  const el = document.getElementById('game'); if (!el || !ST) return;
  const g = ST.game;
  const ci = g.clock_info || {};
  const src = {gsi: ['ok', 'GSI · ' + (ci.rate || 0) + ' пак/с'], estimate: ['warn', '≈ оценка: нет данных ' + Math.round(ci.packet_age) + ' с'], manual: ['warn', 'ручное время'], idle: ['', 'матч не идёт'], none: ['', '']}[ci.source] || ['', ''];
  const srcEl = src[1] ? h('span', {class: 'pill ' + src[0], style: 'margin-left:10px;vertical-align:middle'}, src[1]) : '';
  if (!g.connected && ci.source !== 'manual') { el.replaceChildren(h('div', {class: 'clock mute'}, '--:--'), h('p', {class: 'mute'}, 'Запустите Dota 2. Если игра уже запущена — проверьте вкладку «Настройка». Без Dota можно нажать «Горн 0:00».')); return; }
  const up = (ST.upcoming || []).slice(0, 6).map(u => [h('span', {}, (u.label || u.id) + ' ', h('span', {class: 'mute'}, 'в ' + fmt(u.event_at))), h('b', {style: 'text-align:right'}, fmt(u.in)),
    h('div', {class: 'bar'}, h('i', {style: `width:${Math.max(0, 100 - u.in / 1.2)}%`}))]);
  el.replaceChildren(h('div', {class: 'clock'}, g.has_clock ? fmt(g.clock) : '--:--', g.paused ? ' ⏸' : '', srcEl),
    h('div', {class: 'mute'}, [g.hero && ('Герой: ' + g.hero), g.daytime ? '☀️ день' : '🌙 ночь', (g.state || '').toLowerCase().replaceAll('_', ' ')].filter(Boolean).join(' · ')),
    up.length ? h('div', {class: 'up'}, up) : h('p', {class: 'mute'}, 'Таймеры появятся, когда начнётся матч'));
}

function soundSelect(value, onchange, allowEmpty) {
  return h('select', {onchange: e => onchange(e.target.value)},
    allowEmpty && h('option', {value: ''}, '— нет —'),
    S.sounds.map(s => h('option', {value: s.id, selected: (value || '').split('@')[0] === s.id}, s.label ? `${s.label} (${s.id})` : s.id)));
}
function ensureDef(c, s) {
  if (!c.sounds[s.id]) c.sounds[s.id] = {files: s.files, bus: s.bus, volume: s.volume, cooldown: s.cooldown, mode: s.mode};
  return c.sounds[s.id];
}

pages.sounds = () => {
  const drop = h('div', {class: 'drop', onclick: () => fileIn.click()}, '📁 Перетащите сюда .mp3 / .wav или нажмите, чтобы выбрать файлы');
  const fileIn = h('input', {type: 'file', multiple: true, accept: '.mp3,.wav', class: 'hidden', onchange: e => upload(e.target.files)});
  drop.addEventListener('dragover', e => { e.preventDefault(); drop.classList.add('over'); });
  drop.addEventListener('dragleave', () => drop.classList.remove('over'));
  drop.addEventListener('drop', e => { e.preventDefault(); drop.classList.remove('over'); upload(e.dataTransfer.files); });
  async function upload(files) {
    const fd = new FormData(); for (const f of files) fd.append('files', f);
    try { const r = await api('POST', '/api/sounds', fd); S = r.state; render(); toast('Добавлено: ' + r.saved.join(', ')); }
    catch (e) { toast(e.message, true); }
  }
  const ttsId = h('input', {placeholder: 'id, например mid_miss'}), ttsText = h('input', {placeholder: 'Текст: «Мид пропал!»', style: 'flex:1'});
  const rows = S.sounds.map(s => h('tr', {},
    h('td', {}, h('b', {}, s.label || s.id), h('br'), h('small', {class: 'mute'}, s.id + ' · ' + {builtin: 'встроенный', file: 'файл', config: (s.files[0] || '').startsWith('tts:') ? 'озвучка' : 'конфиг'}[s.source] + ' · ' + s.duration + ' с')),
    h('td', {}, h('select', {onchange: e => edit(c => ensureDef(c, s).bus = e.target.value)}, Object.entries(busName).map(([v, l]) => h('option', {value: v, selected: s.bus === v}, l)))),
    h('td', {style: 'width:120px'}, h('input', {type: 'range', min: 0, max: 2, step: 0.05, value: s.volume, title: 'Громкость', onchange: e => edit(c => ensureDef(c, s).volume = +e.target.value)})),
    h('td', {}, h('input', {type: 'number', min: 0, step: 1, value: s.cooldown, title: 'Кулдаун, с', onchange: e => edit(c => ensureDef(c, s).cooldown = +e.target.value)})),
    h('td', {}, h('select', {onchange: e => edit(c => ensureDef(c, s).mode = e.target.value)}, [['', 'наложение'], ['interrupt', 'перезапуск'], ['ignore', 'не повторять']].map(([v, l]) => h('option', {value: v, selected: (s.mode || '') === v}, l)))),
    h('td', {}, kbd(hotkeysFor('sound:' + s.id)), ' ', h('button', {class: 'small', onclick: async () => { const k = await captureKey('Клавиша для «' + (s.label || s.id) + '»'); if (k) bindHotkey('sound:' + s.id, k); }}, '⌨️')),
    h('td', {style: 'white-space:nowrap'},
      h('button', {class: 'small', title: 'Прослушать (только вам)', onclick: () => api('POST', '/api/action', {action: `sound:${s.id}@monitor`})}, '▶'), ' ',
      h('button', {class: 'small', title: 'В войс', onclick: () => api('POST', '/api/action', {action: `sound:${s.id}@both`})}, '📢'), ' ',
      s.source !== 'builtin' && h('button', {class: 'small danger', title: 'Удалить', onclick: async () => { if (!confirm('Удалить ' + s.id + '?')) return; try { S = await api('POST', '/api/sounds/delete', {id: s.id}); render(); } catch (e) { toast(e.message, true); } }}, '🗑'))));
  return h('div', {},
    h('div', {class: 'flex'},
      h('div', {class: 'card'}, h('h3', {}, 'Добавить файлы'), drop, fileIn,
        h('div', {class: 'row', style: 'margin-top:10px'}, h('button', {onclick: () => api('POST', '/api/open', {what: 'sounds'})}, '📂 Открыть папку звуков'),
          h('label', {class: 'sw'}, h('input', {type: 'checkbox', checked: S.config.normalize_sounds, onchange: e => edit(c => c.normalize_sounds = e.target.checked)}), 'Выравнивать громкость'))),
      h('div', {class: 'card'}, h('h3', {}, 'Озвучка текста (Windows TTS)'), h('div', {class: 'row'}, ttsId, ttsText),
        h('div', {class: 'row', style: 'margin-top:10px'}, h('button', {class: 'primary', onclick: () => {
          const id = ttsId.value.trim().replace(/\s+/g, '_'); const t = ttsText.value.trim();
          if (!id || !t) return toast('Укажите id и текст', true);
          edit(c => c.sounds[id] = {label: t, files: ['tts:' + t], bus: 'both', cooldown: 3}); toast('Создано: ' + id);
        }}, '＋ Создать'), h('span', {class: 'mute'}, 'русский голос, если он установлен в Windows')))),
    h('div', {class: 'card', style: 'margin-top:16px'}, h('table', {},
      h('tr', {}, ['Звук', 'Куда', 'Громкость', 'Кулдаун', 'Повтор', 'Клавиша', ''].map(t => h('th', {}, t))), rows)));
};

function timersContent() {
  const c = S.config;
  const rows = (c.timers || []).map((t, i) => h('tr', {},
    h('td', {}, h('label', {class: 'sw'}, h('input', {type: 'checkbox', checked: !t.disabled, onchange: e => edit(x => x.timers[i].disabled = !e.target.checked)}))),
    h('td', {}, h('input', {value: t.label || t.id, onchange: e => edit(x => x.timers[i].label = e.target.value)})),
    h('td', {}, h('input', {value: (t.at || []).map(fmt).join(', '), placeholder: '6:00, 12:00', style: 'width:110px', title: 'Разовые моменты',
      onchange: e => edit(x => x.timers[i].at = e.target.value.split(',').map(v => v.trim()).filter(Boolean).map(parseClock))})),
    h('td', {}, h('input', {value: t.every ? fmt(t.start || 0) : '', placeholder: '7:00', style: 'width:70px', onchange: e => edit(x => x.timers[i].start = parseClock(e.target.value || '0'))}), ' каждые ',
      h('input', {value: t.every ? fmt(t.every) : '', placeholder: '7:00', style: 'width:70px', onchange: e => edit(x => x.timers[i].every = parseClock(e.target.value || '0'))})),
    h('td', {}, 'за ', h('input', {type: 'number', value: t.warn_before || 0, onchange: e => edit(x => x.timers[i].warn_before = +e.target.value)}), ' с'),
    h('td', {}, soundSelect(t.sound, v => edit(x => x.timers[i].sound = v))),
    h('td', {}, h('button', {class: 'small', onclick: () => api('POST', '/api/action', {action: 'sound:' + t.sound})}, '▶'), ' ',
      h('button', {class: 'small danger', onclick: () => edit(x => x.timers.splice(i, 1))}, '🗑'))));
  const evRows = S.events.map(ev => h('tr', {}, h('td', {}, ev.label), h('td', {},
    soundSelect((c.events || {})[ev.id], v => edit(x => { x.events = x.events || {}; if (v) x.events[ev.id] = v; else delete x.events[ev.id]; }), true))));
  return h('div', {},
    h('div', {class: 'hint'}, 'Время — по игровым часам Dota 2 (пауза учитывается). Интервалы рун меняются от патча к патчу — правьте здесь.'),
    h('div', {class: 'card'}, h('h3', {}, 'Таймеры'), h('table', {},
      h('tr', {}, ['', 'Название', 'Разово', 'Повтор', 'Предупредить', 'Звук', ''].map(t => h('th', {}, t))), rows),
      h('button', {style: 'margin-top:10px', onclick: () => edit(x => (x.timers = x.timers || []).push({id: 'timer_' + Date.now(), label: 'Новый таймер', sound: 'beep', at: [600], warn_before: 10}))}, '＋ Добавить таймер')),
    h('div', {class: 'flex', style: 'margin-top:16px'},
      h('div', {class: 'card'}, h('h3', {}, 'События игры → звук'), h('table', {}, evRows)),
      h('div', {class: 'card'}, h('h3', {}, 'Рошан'), h('p', {class: 'mute'}, 'Нажмите «Рошан убит» (или клавишу) — программа отсчитает аегис 5:00 и окно респауна 8:00–11:00.'),
        h('div', {class: 'sl'}, 'Звук', soundSelect(c.rosh.sound, v => edit(x => x.rosh.sound = v)), ''),
        h('div', {class: 'sl'}, 'Аегис: предупредить за', h('input', {type: 'number', value: c.rosh.aegis_warn, onchange: e => edit(x => x.rosh.aegis_warn = +e.target.value)}), 'с'),
        h('div', {class: 'sl'}, 'Респаун: предупредить за', h('input', {type: 'number', value: c.rosh.min_warn, onchange: e => edit(x => x.rosh.min_warn = +e.target.value)}), 'с'))));
};
function parseClock(v) { v = String(v).trim(); if (v.includes(':')) { const n = v.startsWith('-'); const [m, s] = v.replace('-', '').split(':').map(Number); return (n ? -1 : 1) * (m * 60 + (s || 0)); } return +v || 0; }

const effectHelp = 'gain{db} · lowpass/highpass/bandpass{freq,q} · peaking/lowshelf/highshelf{freq,q,db} · drive{amount,mode:soft|hard|fold} · bitcrush{bits,downsample} · compressor{threshold_db,ratio,attack_ms,release_ms,makeup_db} · gate{threshold_db,hang_ms} · squelch{threshold_db,click_db,static_db,tail_ms,tail_db} · noise{db,kind:white|crackle} · dropout{chance,len_ms} · ringmod{freq,mix} · limiter{threshold_db}';
let editing = null;
pages.voice = () => {
  if (!editing) { const cur = S.presets.find(p => p.name === ST.preset); if (cur) editing = {name: cur.name, text: JSON.stringify(cur.specs, null, 2), builtin: cur.builtin}; }
  const list = S.presets.map(p => h('button', {class: 'tile' + (ST.preset === p.name ? ' on' : '') + (editing && editing.name === p.name ? ' on' : ''), onclick: () => { editing = {name: p.name, text: JSON.stringify(p.specs, null, 2), builtin: p.builtin}; api('POST', '/api/action', {action: 'preset:' + p.name}); render(); }},
    h('b', {}, p.name), h('small', {}, (p.builtin ? '' : '✎ ') + (p.desc || 'свой пресет')), ...kbd(hotkeysFor('preset:' + p.name))));
  let editor = h('p', {class: 'mute'}, 'Выберите пресет, чтобы включить его и открыть в редакторе.');
  if (editing) {
    const name = h('input', {value: editing.builtin ? editing.name + '_my' : editing.name});
    const ta = h('textarea', {value: editing.text, oninput: e => editing.text = e.target.value});
    const parse = () => { try { return JSON.parse(ta.value); } catch (e) { toast('Ошибка JSON: ' + e.message, true); return null; } };
    editor = h('div', {},
      h('div', {class: 'row'}, 'Имя:', name, h('button', {class: 'small', onclick: async () => { const k = await captureKey('Клавиша для пресета'); if (k) bindHotkey('preset:' + name.value, k); }}, '⌨️ клавиша')),
      h('p', {class: 'mute', style: 'font-size:12px'}, effectHelp), ta,
      h('div', {class: 'row', style: 'margin-top:10px'},
        h('button', {onclick: async () => { const sp = parse(); if (!sp) return; try { await api('POST', '/api/preset/test', {name: name.value, specs: sp}); toast('Применено на лету (не сохранено). Включите «Слышать себя».'); } catch (e) { toast(e.message, true); } }}, '🎧 Попробовать'),
        h('button', {class: 'primary', onclick: () => { const sp = parse(); if (!sp) return; const n = name.value.trim(); edit(c => { c.voice_presets = c.voice_presets || {}; c.voice_presets[n] = sp; }); editing = {name: n, text: ta.value, builtin: false}; api('POST', '/api/action', {action: 'preset:' + n}); toast('Сохранено: ' + n); }}, '💾 Сохранить'),
        !editing.builtin && (S.config.voice_presets || {})[editing.name] && h('button', {class: 'danger', onclick: () => { const n = editing.name; editing = null; edit(c => delete c.voice_presets[n]); api('POST', '/api/action', {action: 'preset:clean'}); }}, '🗑 Удалить / сбросить'),
        h('button', {class: ST.mic_monitor ? 'on' : '', onclick: () => api('POST', '/api/action', {action: 'mic_monitor'})}, '🎧 Слышать себя')));
  }
  return h('div', {class: 'flex'},
    h('div', {class: 'card', style: 'max-width:420px'}, h('h3', {}, 'Пресеты'), h('div', {class: 'btns'}, list),
      h('label', {class: 'sw', style: 'margin-top:12px'}, h('input', {type: 'checkbox', checked: S.config.fx_on_sounds, onchange: e => edit(c => c.fx_on_sounds = e.target.checked)}), 'Пропускать звуки через пресет (сирена «по рации»)'),
      h('div', {class: 'sl'}, 'Пресет при запуске', h('select', {onchange: e => edit(c => c.start_preset = e.target.value)}, S.presets.map(p => h('option', {value: p.name, selected: S.config.start_preset === p.name}, p.name))), '')),
    h('div', {class: 'card'}, h('h3', {}, 'Редактор пресета'), editor));
};

function actionOptions(sel) {
  const opts = [['Звуки', S.sounds.flatMap(s => [['sound:' + s.id, '🔊 ' + (s.label || s.id)], ['sound:' + s.id + '@monitor', '🎧 ' + (s.label || s.id) + ' (только мне)']])],
    ['Голос', [...S.presets.map(p => ['preset:' + p.name, '🎙 ' + p.name]), ['preset:next', '🎙 следующий пресет'], ['preset:prev', '🎙 предыдущий пресет']]],
    ['Прочее', [['rosh', '🐉 Рошан убит'], ['clock_horn', '📯 горн 0:00 (ручное время)'], ['stop', '⏹ стоп всё'], ['mic_monitor', '🎧 слышать себя'], ['mic_mute', '🔇 микрофон вкл/выкл'], ['reload', '🔄 перечитать конфиг'], ['ui', '🪟 открыть окно']]]];
  return opts.map(([g, items]) => h('optgroup', {label: g}, items.map(([v, l]) => h('option', {value: v, selected: v === sel}, l))));
}
pages.keys = () => {
  const hk = S.config.hotkeys || {};
  const rows = Object.keys(hk).sort().map(k => h('tr', {},
    h('td', {}, h('span', {class: 'kbd'}, k)),
    h('td', {}, h('select', {onchange: e => edit(c => c.hotkeys[k] = e.target.value)}, actionOptions(hk[k]))),
    h('td', {}, h('button', {class: 'small', onclick: async () => { const n = await captureKey('Новая клавиша вместо ' + k); if (n) edit(c => { const a = c.hotkeys[k]; delete c.hotkeys[k]; c.hotkeys[n] = a; }); }}, 'изменить'), ' ',
      h('button', {class: 'small danger', onclick: () => edit(c => delete c.hotkeys[k])}, '🗑'))));
  const newAct = h('select', {}, actionOptions('sound:siren'));
  return h('div', {class: 'card'}, h('h3', {}, 'Горячие клавиши'),
    h('div', {class: 'hint'}, 'Клавиши глобальные — работают поверх Dota 2. Не используйте клавиши, нужные в игре (QWERDF, ZXCVBN и т.п.). Numpad и F-клавиши — хороший выбор.'),
    h('table', {}, h('tr', {}, h('th', {}, 'Клавиша'), h('th', {}, 'Действие'), h('th', {}, '')), rows),
    h('div', {class: 'row', style: 'margin-top:12px'}, newAct, h('button', {class: 'primary', onclick: async () => { const k = await captureKey('Клавиша для действия'); if (k) bindHotkey(newAct.value, k); }}, '＋ Назначить клавишу')));
};

// ---------- скрипты ----------
const API_REF = [
  ['hotkey("Ctrl+F1", fn)', 'клавиша или сочетание', 'hotkey("Ctrl+F1", () => {\n  play("siren", { to: "team" });\n});\n'],
  ['combo("Num1 Num1", fn, {within: 400})', 'последовательность: двойное нажатие, «Num1 Num2»…', 'combo("Num1 Num1", () => {\n  say("Мид, ганк!", { to: "team" });\n}, { within: 400 });\n'],
  ['hold("Num3", 500, fn, onRelease)', 'удержание клавиши N мс; onRelease — при отпускании', 'hold("Num3", 500, () => {\n  play("siren", { to: "team" });\n}, () => stop("siren"));\n'],
  ['onKey("F9", (down) => …)', 'любое нажатие/отпускание клавиши', 'onKey("F9", (down) => {\n  if (down) log("F9 нажата");\n});\n'],
  ['at("6:00", fn, {before: 10})', 'разово по игровому времени (за N с до)', 'at("6:00", () => {\n  say("Руны через 10 секунд", { to: "me" });\n}, { before: 10 });\n'],
  ['every("7:00", fn, {from, until, before})', 'повторять по игровому времени', 'every("7:00", (t) => {\n  notify("Руна мудрости в " + fmt(t));\n}, { from: "7:00", before: 20 });\n'],
  ['on("death", fn)', 'событие: kill, death, respawn, low_hp, night, day, rosh_killed, game_start, new_game, clock, state…', 'on("death", () => {\n  play("beep", { to: "me" });\n});\n'],
  ['after(5, fn) / repeat(2, fn) / cancel(id)', 'реальные секунды (не игровые)', 'const id = repeat(2, () => log("тик"));\nafter(10, () => cancel(id));\n'],
  ['play("siren", {to, volume})', 'звук: to = "team" (войс + вы), "me" (только вы), "both"', 'play("siren", { to: "team", volume: 0.8 });\n'],
  ['say("текст", {to})', 'озвучить текст голосом Windows (по умолчанию в войс)', 'say("Мид пропал!", { to: "team" });\n'],
  ['stop() / stop("siren")', 'остановить звуки', 'stop();\n'],
  ['preset("radio") / preset()', 'включить пресет голоса / узнать текущий', 'preset("radio");\n'],
  ['action("rosh")', 'любое действие клавиш: rosh, stop, mic_monitor, clock_horn, preset:next…', 'action("rosh");\n'],
  ['notify("текст") / log(…)', 'всплывающее уведомление в окне / строка в журнал', 'notify("Готово");\n'],
  ['game.clock · game.seconds · game.hero', 'игровое время (null — нет игры), герой, game.paused, game.daytime, game.state, game.clockSource', 'if (game.clock !== null && game.clock > 20 * 60) {\n  log("поздняя игра");\n}\n'],
  ['game.raw', 'весь JSON от Dota: hero, player, abilities, items, buildings', 'on("state", () => {\n  const r = game.raw;\n  if (r && r.hero) log(r.hero.health);\n});\n'],
  ['time("6:00") / fmt(360)', 'перевод времени: строка ⇄ секунды', 'log(fmt(time("6:00") + 30));\n'],
];
const JS_KW = /\b(const|let|var|function|return|if|else|for|while|of|in|new|true|false|null|undefined|break|continue|switch|case|default|typeof)\b/;
const API_NAMES = /\b(hotkey|combo|hold|onKey|at|every|on|after|repeat|cancel|play|say|stop|preset|action|notify|log|game|time|fmt)\b/;
function highlight(code) {
  const esc = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;');
  const re = /(\/\/[^\n]*|\/\*[\s\S]*?\*\/)|("(?:\\.|[^"\\\n])*"?|'(?:\\.|[^'\\\n])*'?|`(?:\\.|[^`\\])*`?)|(\b\d+(?:\.\d+)?\b)|([A-Za-z_$][\w$]*)/g;
  let out = '', last = 0, m;
  while ((m = re.exec(code))) {
    out += esc(code.slice(last, m.index));
    const t = esc(m[0]);
    if (m[1]) out += '<i class="c">' + t + '</i>';
    else if (m[2]) out += '<i class="s">' + t + '</i>';
    else if (m[3]) out += '<i class="n">' + t + '</i>';
    else if (JS_KW.test(m[0]) && m[0].match(JS_KW)[0] === m[0]) out += '<i class="k">' + t + '</i>';
    else if (API_NAMES.test(m[0]) && m[0].match(API_NAMES)[0] === m[0]) out += '<i class="f">' + t + '</i>';
    else out += t;
    last = re.lastIndex;
  }
  return out + esc(code.slice(last)) + '\n';
}
let SCR = null;          // список скриптов
let SC = null;           // {name, code, saved, err:{error,line}}
async function loadScripts(open) {
  try { SCR = (await api('GET', '/api/scripts')).scripts; } catch (e) { toast(e.message, true); SCR = []; }
  if (open || (!SC && SCR.length)) await openScript(open || (SCR.find(s => s.enabled) || SCR[0]).name, true);
  if (page === 'scripts') render();
}
async function openScript(name, quiet) {
  if (SC && SC.code !== SC.saved && !confirm('Есть несохранённые изменения в «' + SC.name + '». Отбросить?')) return;
  try { const r = await api('GET', '/api/scripts/code?name=' + encodeURIComponent(name)); SC = {name, code: r.code, saved: r.code, err: null}; }
  catch (e) { toast(e.message, true); }
  if (!quiet) render();
}
async function saveScript(enable) {
  if (!SC) return;
  try {
    const r = await api('PUT', '/api/scripts', {name: SC.name, code: SC.code, enable});
    SC.saved = SC.code; SC.err = r.compile.error ? r.compile : null; SCR = r.scripts;
    if (SC.err) toast('Сохранено, но есть ошибка в строке ' + (SC.err.line || '?'), true); else toast('Сохранено: ' + SC.name);
  } catch (e) { toast(e.message, true); }
  render();
}
function scriptEditor() {
  const lines = h('div', {class: 'gut'});
  const pre = h('pre', {class: 'hl'});
  const ta = h('textarea', {class: 'code', spellcheck: false, value: SC.code, wrap: 'off'});
  const paint = () => {
    const n = SC.code.split('\n').length;
    const errLine = SC.err && SC.err.line;
    lines.replaceChildren(...Array.from({length: n}, (_, i) => h('div', {class: i + 1 === errLine ? 'errl' : ''}, i + 1)));
    pre.innerHTML = highlight(SC.code);
    const dirty = document.getElementById('scDirty'); if (dirty) dirty.textContent = SC.code !== SC.saved ? '● не сохранено' : '';
  };
  let chk = null;
  ta.addEventListener('input', () => {
    SC.code = ta.value; paint();
    clearTimeout(chk); chk = setTimeout(async () => {
      try { const r = await api('POST', '/api/scripts/check', {name: SC.name, code: SC.code}); SC.err = r.error ? r : null; paint(); showErr(); } catch (e) {}
    }, 600);
  });
  ta.addEventListener('scroll', () => { pre.scrollTop = ta.scrollTop; pre.scrollLeft = ta.scrollLeft; lines.scrollTop = ta.scrollTop; });
  ta.addEventListener('keydown', e => {
    if (e.key === 'Tab') { e.preventDefault(); document.execCommand('insertText', false, '  '); }
    if ((e.ctrlKey || e.metaKey) && e.code === 'KeyS') { e.preventDefault(); saveScript(); }
    if (e.key === 'Enter') { // автоотступ
      const before = ta.value.slice(0, ta.selectionStart); const ind = (before.split('\n').pop().match(/^\s*/) || [''])[0];
      const extra = /[{(\[]\s*$/.test(before) ? '  ' : '';
      e.preventDefault(); document.execCommand('insertText', false, '\n' + ind + extra);
    }
  });
  const errBox = h('div', {id: 'scErr'});
  function showErr() {
    errBox.replaceChildren(SC.err ? h('div', {class: 'errbox', onclick: () => gotoLine(SC.err.line)}, '⚠ ', SC.err.line ? 'Строка ' + SC.err.line + ': ' : '', SC.err.error) : '');
  }
  function gotoLine(n) {
    if (!n) return; const pos = SC.code.split('\n').slice(0, n - 1).join('\n').length + (n > 1 ? 1 : 0);
    ta.focus(); ta.setSelectionRange(pos, pos); ta.scrollTop = Math.max(0, (n - 5) * 18);
  }
  window._scInsert = snippet => { ta.focus(); document.execCommand('insertText', false, snippet); };
  setTimeout(() => { paint(); showErr(); });
  return h('div', {}, h('div', {class: 'ed'}, lines, h('div', {class: 'edw'}, pre, ta)), errBox);
}
pages.scripts = () => {
  if (!SCR) { loadScripts(); return h('p', {class: 'mute'}, 'Загружаю скрипты…'); }
  const info = SC && SCR.find(s => s.name === SC.name);
  const list = SCR.map(s => h('div', {class: 'scitem' + (SC && SC.name === s.name ? ' on' : ''), onclick: () => openScript(s.name)},
    h('label', {class: 'sw', onclick: e => e.stopPropagation()}, h('input', {type: 'checkbox', checked: s.enabled, onchange: async e => {
      try { SCR = (await api('POST', '/api/scripts/enable', {name: s.name, on: e.target.checked})).scripts; render(); } catch (er) { toast(er.message, true); } }})),
    h('div', {style: 'min-width:0'}, h('b', {}, s.name), h('small', {}, s.title || ''),
      h('small', {class: s.error ? 'bad' : (s.running ? 'ok' : 'mute')}, s.error ? '⚠ ошибка' : (s.running ? '● работает' : (s.enabled ? 'не запущен' : 'выключен'))))));
  const newBtn = h('button', {onclick: async () => {
    const n = (prompt('Имя скрипта (латиница/кириллица, цифры, _ и -):', 'my_script') || '').trim();
    if (!n) return;
    if (SCR.some(s => s.name === n)) return toast('Уже есть', true);
    SC = null;
    try { await api('PUT', '/api/scripts', {name: n, code: '// ' + n + ': описание\n\nhotkey("F9", () => {\n  play("beep", { to: "me" });\n  notify("Работает!");\n});\n', enable: true}); await loadScripts(n); } catch (e) { toast(e.message, true); }
  }}, '＋ Новый скрипт');
  const editorCard = !SC ? h('div', {class: 'card'}, h('p', {class: 'mute'}, 'Создайте скрипт или выберите слева.')) :
    h('div', {class: 'card'},
      h('div', {class: 'row', style: 'margin-bottom:10px'}, h('h3', {style: 'margin:0'}, SC.name + '.js'), h('span', {id: 'scDirty', class: 'warn'}), h('div', {class: 'grow'}),
        h('button', {class: 'primary', onclick: () => saveScript()}, '💾 Сохранить (Ctrl+S)'),
        h('button', {title: 'Сохранить и перезапустить, даже если выключен — для проверки', onclick: async () => {
          await saveScript(); try { const r = await api('POST', '/api/scripts/run', {name: SC.name}); SCR = r.scripts; SC.err = r.compile.error ? r.compile : null; toast(SC.err ? 'Ошибка запуска' : 'Перезапущен', !!SC.err); render(); } catch (e) { toast(e.message, true); } }}, '▶ Перезапустить'),
        h('button', {class: 'danger', onclick: async () => { if (!confirm('Удалить скрипт ' + SC.name + '?')) return; try { SCR = (await api('POST', '/api/scripts/delete', {name: SC.name})).scripts; SC = null; SCR.length && await openScript(SCR[0].name, true); render(); } catch (e) { toast(e.message, true); } }}, '🗑')),
      scriptEditor(),
      info && info.hooks.length ? h('div', {class: 'row', style: 'margin-top:10px'}, h('span', {class: 'mute'}, 'Слушает:'), info.hooks.map(k => h('span', {class: 'kbd', title: k.kind}, (k.kind === 'at' || k.kind === 'every' ? (k.kind === 'every' ? 'каждые ' : 'в ') : k.kind === 'on' ? 'событие ' : '') + k.what))) : '',
      h('h3', {style: 'margin-top:14px'}, 'Вывод скрипта'),
      h('div', {class: 'out'}, info && info.output.length ? info.output.join('\n') : h('span', {class: 'mute'}, 'log(...) и ошибки появятся здесь')),
      h('div', {class: 'row', style: 'margin-top:8px'}, h('button', {class: 'small', onclick: () => loadScripts()}, '🔄 Обновить')));
  const ref = h('div', {class: 'card ref'}, h('h3', {}, 'Справка · нажмите, чтобы вставить'),
    API_REF.map(([sig, desc, snip]) => h('div', {class: 'refi', onclick: () => window._scInsert && window._scInsert(snip)}, h('code', {}, sig), h('small', {}, desc))));
  return h('div', {},
    h('div', {class: 'hint'}, 'Скрипты на JavaScript: свои клавиши, комбинации, тайминги и реакции на игру. Клавиши не отбираются у Dota. Каждый скрипт работает отдельно — ошибка или зависание одного не ломает остальные.'),
    h('div', {class: 'scgrid'},
      h('div', {class: 'card'}, h('h3', {}, 'Скрипты'), list, h('div', {style: 'margin-top:10px'}, newBtn)),
      editorCard, ref));
};

// ---------- тесты ----------
const TESTS = [
  ['monitor', '🎧 Наушники', 'Короткий сигнал только вам. Проверяет вывод в наушники.', 'Проверить'],
  ['mic', '🎤 Микрофон', 'Говорите 3 секунды обычным голосом, как в игре. Покажет громкость, шум, перегруз и подскажет усиление.', 'Записать 3 с'],
  ['cable', '🔌 Кабель → Dota', 'Пропускает тон через виртуальный кабель и слушает его с другой стороны. Покажет уровень и задержку.', 'Проверить'],
  ['voice', '🎛 Пресет голоса', 'Запишите фразу — услышите её сначала без эффекта, затем через выбранный пресет.', 'Записать 3 с'],
  ['team', '👂 Как меня слышит команда', 'Записывает ровно то, что уходит в Dota (голос + эффекты + звуки), и проигрывает вам.', 'Записать 4 с'],
  ['ptt', '⌨️ Кнопка голосового чата', 'Зажмёт вашу кнопку голосового чата на 1,5 с — в Dota загорится иконка микрофона.', 'Нажать'],
];
const TR = {}; let testRunning = null, testPreset = '';
async function runTest(id) {
  if (testRunning) return;
  testRunning = id; TR[id] = null; render();
  try {
    TR[id] = await api('POST', '/api/test', {id, sec: id === 'team' ? 4 : 3, preset: testPreset || ST.preset});
  } catch (e) { TR[id] = {verdict: 'fail', summary: e.message}; }
  testRunning = null; render();
}
// мастер «Проверить всё»: наушники → микрофон → кабель → пресет → команда → кнопка чата
let wizard = null;
async function runAll() {
  const order = TESTS.map(t => t[0]).filter(id => id !== 'ptt' || (S.config.ptt && S.config.ptt.key));
  wizard = {i: 0, n: order.length, cur: ''};
  for (const id of order) {
    wizard.cur = id; render();
    if (['mic', 'voice', 'team'].includes(id)) { toast('Сейчас запись — говорите обычным голосом'); await new Promise(r => setTimeout(r, 800)); }
    await runTest(id);
    wizard.i++;
    if (TR[id] && TR[id].verdict === 'fail' && ['monitor', 'cable'].includes(id)) { toast('Дальше проверять нет смысла — сначала исправьте: ' + TESTS.find(t => t[0] === id)[1], true); break; }
  }
  wizard.done = true; wizard.cur = ''; render();
}
function wizardCard() {
  const done = TESTS.filter(t => TR[t[0]]);
  const cnt = v => done.filter(t => TR[t[0]].verdict === v).length;
  return h('div', {class: 'card', style: 'margin-bottom:16px'},
    h('div', {class: 'row'}, h('h3', {style: 'margin:0'}, 'Проверить всё'), h('div', {class: 'grow'}),
      wizard && !wizard.done ? h('span', {class: 'pill warn'}, `⏳ ${wizard.i + 1}/${wizard.n}: ${(TESTS.find(t => t[0] === wizard.cur) || [, ''])[1]}`) : '',
      h('button', {class: 'primary', disabled: !!testRunning, onclick: runAll}, '▶ Проверить всё'),
      h('button', {title: 'Журнал, настройки, устройства и результаты тестов одним файлом — приложите его к сообщению об ошибке',
        onclick: async () => { try { const r = await api('POST', '/api/diag/bundle', {report: TR}); toast('Сохранено: ' + r.file); } catch (e) { toast(e.message, true); } }}, '📦 Диагностический пакет')),
    done.length ? h('div', {class: 'row', style: 'margin-top:10px'},
      done.map(t => h('span', {class: 'pill ' + ({ok: 'ok', warn: 'warn', fail: 'bad'}[TR[t[0]].verdict] || '')}, t[1] + ' ' + ({ok: '✅', warn: '⚠️', fail: '❌'}[TR[t[0]].verdict] || ''))),
      h('span', {class: 'mute'}, `${cnt('ok')} ок · ${cnt('warn')} предупр. · ${cnt('fail')} ошибок`)) :
      h('p', {class: 'mute', style: 'margin:8px 0 0'}, 'Все тесты по очереди, около 20 секунд. Во время записи говорите обычным голосом.'));
}
function statsLine(s) {
  if (!s) return '';
  return h('div', {class: 'stats'},
    [['Речь', s.speech_db], ['Фон', s.noise_db], ['Пик', s.peak_db]].map(([l, v]) => h('div', {}, h('small', {}, l), h('b', {}, Math.round(v) + ' dB'),
      h('div', {class: 'meter'}, h('i', {style: `width:${Math.max(0, Math.min(100, (v + 60) / 60 * 100))}%`})))),
    s.clip_pct > 0 ? h('div', {}, h('small', {}, 'Перегруз'), h('b', {class: s.clip_pct > 0.1 ? 'bad' : ''}, s.clip_pct + '%')) : '');
}
pages.test = () => {
  const cards = TESTS.map(([id, title, desc, btn]) => {
    const r = TR[id], busy = testRunning === id;
    return h('div', {class: 'card test ' + (r ? r.verdict : '')},
      h('div', {class: 'row'}, h('b', {style: 'font-size:15px'}, title), h('div', {class: 'grow'}),
        r && h('span', {class: 'verdict'}, {ok: '✅', warn: '⚠️', fail: '❌'}[r.verdict] || '')),
      h('p', {class: 'mute', style: 'margin:6px 0 10px'}, desc),
      id === 'voice' && h('div', {class: 'sl'}, 'Пресет', h('select', {onchange: e => testPreset = e.target.value}, S.presets.map(p => h('option', {value: p.name, selected: (testPreset || ST.preset) === p.name}, p.name))), ''),
      h('button', {class: 'primary', disabled: !!testRunning, onclick: () => runTest(id)}, busy ? (id === 'mic' || id === 'voice' || id === 'team' ? '● Запись… говорите' : '⏳ Проверяю…') : btn),
      r && h('div', {class: 'res'}, h('b', {}, r.summary), statsLine(r.stats),
        (r.details || []).map(d => h('div', {class: 'mute'}, '• ' + d)),
        r.ask && h('div', {class: 'hint', style: 'margin:8px 0 0'}, r.ask),
        r.suggest && r.suggest.mic_gain && h('button', {style: 'margin-top:8px', onclick: () => { edit(c => c.mic_gain = r.suggest.mic_gain); toast('Усиление микрофона: ' + r.suggest.mic_gain); }}, '✔ Применить усиление ' + r.suggest.mic_gain)));
  });
  return h('div', {}, wizardCard(),
    h('div', {class: 'hint'}, 'Тесты идут по-настоящему через выбранные устройства. Порядок: наушники → микрофон → кабель → «как меня слышит команда». Если в Dota включена активация голосом, тестовый тон может попасть в чат.'),
    h('div', {class: 'grid'}, cards));
};

let SETUP = null;
async function loadSetup() { try { SETUP = await api('GET', '/api/setup'); } catch (e) { toast(e.message, true); } if (page === 'setup' || page === 'integrations') render(); updateBadge(); }
function updateBadge() { document.getElementById('setupBadge').classList.toggle('hidden', !SETUP || SETUP.all_ok); }
async function fix(id) {
  if (id === 'launch' && SETUP.checks.find(c => c.id === 'launch').detail.includes('перезапущен') && !confirm('Steam будет закрыт и запущен заново. Продолжить?')) return;
  try { toast('Выполняю…'); SETUP = await api('POST', '/api/setup/fix', {id}); toast('Готово'); } catch (e) { toast(e.message, true); }
  await load(); loadSetup();
}
pages.setup = () => {
  const c = S.config;
  if (!SETUP) { loadSetup(); return h('p', {class: 'mute'}, 'Проверяю…'); }
  const checks = SETUP.checks.filter(ch => !DOTA_CHECKS.includes(ch.id)).map(ch => h('div', {class: 'check'}, h('span', {class: 'ic'}, ch.ok ? '✅' : (ch.fix ? '⚠️' : (ch.id === 'gsi_live' ? '⏳' : '❌'))),
    h('div', {}, h('b', {}, ch.title), h('small', {}, ch.detail), ch.manual && h('small', {class: 'warn'}, ch.manual)),
    ch.fix && !ch.ok ? h('button', {class: 'primary small', onclick: () => fix(ch.fix)}, ch.fix_label) : h('span')));
  const devSel = (key, list, autoLabel) => h('select', {onchange: e => edit(x => x.devices[key] = e.target.value)},
    h('option', {value: ''}, autoLabel), (list || []).map(d => h('option', {value: d.name, selected: c.devices[key] === d.name}, d.name + (d.is_default ? ' (по умолчанию)' : ''))));
  const num = (label, get, set, unit) => h('div', {class: 'sl'}, label, h('input', {type: 'number', value: get(), onchange: e => edit(x => set(x, +e.target.value))}), unit);
  return h('div', {class: 'flex'},
    h('div', {class: 'card'}, h('h3', {}, 'Проверка и автонастройка'),
      SETUP.busy && h('div', {class: 'hint'}, '⏳ ' + SETUP.busy),
      checks,
      h('div', {class: 'row', style: 'margin-top:14px'},
        h('button', {class: 'primary', onclick: async () => { try { const r = await api('POST', '/api/setup/auto', {allow_steam_restart: confirm('Разрешить перезапуск Steam, если нужно добавить параметр запуска?')}); SETUP = r.setup; r.report.forEach(x => toast(x)); await load(); } catch (e) { toast(e.message, true); } }}, '✨ Настроить всё автоматически'),
        h('button', {onclick: loadSetup}, '🔄 Проверить снова'))),
    h('div', {class: 'card'}, h('h3', {}, 'Устройства'),
      h('div', {class: 'sl'}, 'Микрофон', devSel('mic', (S.devices.capture || []).filter(d => !isVirt(d.name)), 'Авто (по умолчанию)'), ''),
      h('div', {class: 'sl'}, 'Виртуальный кабель', devSel('voice_out', S.devices.playback, 'Авто (найти кабель)'), ''),
      h('div', {class: 'sl', title: 'Устройство записи, на которое приходит звук из кабеля. Его выбирают микрофоном в Discord и играх.'}, 'Другой конец кабеля',
        devSel('cable_rec', S.devices.capture, 'Авто' + (S.devices.cable_rec ? ' (' + shortDev(S.devices.cable_rec) + ')' : '')), ''),
      S.devices.cable && h('p', {class: 'mute', style: 'font-size:12px;margin:0 0 6px'}, '📡 VoiceBox пишет в «' + S.devices.cable + '». В Discord / Dota / OBS выберите микрофон «' + cableRecName() + '».'),
      !S.devices.cable && h('p', {class: 'warn', style: 'font-size:12px'}, 'Кабель не найден автоматически. Если он у вас есть (например «Line 1» от Virtual Audio Cable) — выберите его в списке выше.'),
      h('div', {class: 'sl'}, 'Наушники', devSel('monitor', (S.devices.playback || []).filter(d => !isVirt(d.name)), 'Авто (по умолчанию)'), ''),
      S.devices.error && h('p', {class: 'bad'}, S.devices.error),
      h('h3', {style: 'margin-top:18px'}, 'Голосовой чат Dota 2'),
      h('div', {class: 'row'}, 'Кнопка:', h('span', {class: 'kbd'}, c.ptt.key || 'не задана'),
        h('button', {class: 'small', onclick: async () => { const k = await captureKey('Нажмите кнопку голосового чата из Dota 2', true); if (k) edit(x => x.ptt.key = k); }}, 'Назначить'),
        h('button', {class: 'small', onclick: () => fix('ptt')}, 'Взять из Dota 2')),
      h('label', {class: 'sw'}, h('input', {type: 'checkbox', checked: c.ptt.auto_detect, onchange: e => edit(x => x.ptt.auto_detect = e.target.checked)}), 'Следить за биндом в Dota 2 и обновлять автоматически'),
      h('label', {class: 'sw'}, h('input', {type: 'checkbox', checked: c.ptt.auto, onchange: e => edit(x => x.ptt.auto = e.target.checked)}), 'Зажимать кнопку, пока играет звук'),
      num('Задержка перед звуком', () => c.ptt.lead_ms, (x, v) => x.ptt.lead_ms = v, 'мс'),
      num('Держать после звука', () => c.ptt.tail_ms, (x, v) => x.ptt.tail_ms = v, 'мс'),
      h('h3', {style: 'margin-top:18px'}, 'Программа'),
      h('label', {class: 'sw'}, h('input', {type: 'checkbox', checked: SETUP.autostart, onchange: async e => { try { await api('POST', '/api/setup/autostart', {on: e.target.checked}); loadSetup(); } catch (er) { toast(er.message, true); } }}), 'Запускать вместе с Windows (в фоне)'),
      h('label', {class: 'sw'}, h('input', {type: 'checkbox', checked: c.ui.keep_running_on_close, onchange: e => edit(x => x.ui.keep_running_on_close = e.target.checked)}), 'Работать в фоне после закрытия окна (открыть снова — запуском exe или клавишей «открыть окно»)'),
      num('Мало HP, порог', () => c.gsi.low_hp_percent, (x, v) => x.gsi.low_hp_percent = v, '%'),
      h('div', {class: 'row', style: 'margin-top:10px'},
        h('button', {onclick: () => api('POST', '/api/open', {what: 'config'})}, '📂 Папка программы'),
        h('button', {onclick: () => api('POST', '/api/open', {what: 'repo'})}, 'GitHub'))),
    updateCard(), backupsCard());
};

// ---------- обновления и бэкапы ----------
let UPD = null, BK = null;
const UPD_STATE = {idle: 'Ещё не проверялось', checking: 'Проверяю…', uptodate: '✅ У вас последняя версия', available: '⬆ Доступна новая версия', downloading: '⬇ Скачиваю…', ready: 'Готово к установке', error: '⚠ Ошибка'};
async function updAct(path, msg) {
  try { toast(msg); UPD = await api('POST', path); } catch (e) { toast(e.message, true); }
  if (page === 'setup') render();
}
function updateCard() {
  const c = S.config, u = UPD || (ST && ST.update) || {};
  if (!UPD) api('GET', '/api/update').then(r => { UPD = r; if (page === 'setup') render(); }).catch(() => {});
  const upd = c.update || {};
  return h('div', {class: 'card'}, h('h3', {}, 'Обновления'),
    h('p', {}, 'Версия: ', h('b', {}, S.version), u.latest ? ['  ·  последняя: ', h('b', {class: u.available ? 'warn' : 'ok'}, u.latest)] : ''),
    u.state && h('p', {class: 'mute'}, UPD_STATE[u.state] || u.state, u.state === 'downloading' && u.progress ? ' ' + Math.round(u.progress * 100) + '%' : ''), u.error && h('p', {class: 'bad'}, u.error),
    u.notes && h('div', {class: 'out', style: 'max-height:120px'}, u.notes),
    h('div', {class: 'row', style: 'margin:10px 0'},
      h('button', {onclick: () => updAct('/api/update/check', 'Проверяю…')}, '🔄 Проверить'),
      u.available && h('button', {class: 'primary', onclick: () => confirm(`Установить ${u.latest}? Программа перезапустится.`) && updAct('/api/update/install', 'Скачиваю…')}, '⬇ Обновить до ' + u.latest),
      u.can_rollback && u.previous && h('button', {onclick: () => confirm(`Вернуть версию ${u.previous}?`) && updAct('/api/update/rollback', 'Откатываю…')}, '↩ Откатить на ' + u.previous)),
    h('label', {class: 'sw'}, h('input', {type: 'checkbox', checked: upd.auto_check !== false, onchange: e => edit(x => (x.update = x.update || {}).auto_check = e.target.checked)}), 'Проверять обновления автоматически'),
    h('label', {class: 'sw'}, h('input', {type: 'checkbox', checked: !!upd.auto_install, onchange: e => edit(x => (x.update = x.update || {}).auto_install = e.target.checked)}), 'Устанавливать сами (не во время матча)'),
    h('p', {class: 'mute', style: 'font-size:12px'}, 'Обновления проверяются по цифровой подписи. Если новая версия не запустится — программа сама вернёт предыдущую.'));
}
function backupsCard() {
  if (!BK) { BK = []; api('GET', '/api/backups').then(r => { BK = r.backups || []; if (page === 'setup') render(); }).catch(() => {}); }
  return h('div', {class: 'card'}, h('h3', {}, 'Резервные копии настроек'),
    BK.length ? h('table', {}, BK.map(b => { const n = b.name || b; return h('tr', {}, h('td', {}, n.replace(/^config-|\.json$/g, '')),
      h('td', {style: 'text-align:right'}, h('button', {class: 'small', onclick: async () => { if (!confirm('Восстановить настройки из ' + n + '?')) return;
        try { await api('POST', '/api/backups/restore', {name: n}); toast('Настройки восстановлены'); BK = null; await load(); } catch (e) { toast(e.message, true); } }}, 'Восстановить'))); }))
      : h('p', {class: 'mute'}, 'Копий пока нет — они создаются при каждом сохранении (хранятся 5 последних).'));
}

pages.log = () => {
  const el = h('div', {id: 'log'}, (S.logs || []).join('\n'));
  setTimeout(() => el.scrollTop = el.scrollHeight);
  return h('div', {}, h('h2', {}, 'Журнал'), el);
};

// ---------- рендер ----------
function render() {
  if (!S) return;
  document.getElementById('ver').textContent = S.version;
  const el = document.getElementById('page');
  const scroll = el.scrollTop;
  el.replaceChildren(pages[page]());
  el.scrollTop = scroll;
  renderPills(); renderGame();
}
document.querySelectorAll('#nav button[data-page]').forEach(b => b.addEventListener('click', () => {
  document.querySelectorAll('#nav button').forEach(x => x.classList.remove('active'));
  b.classList.add('active'); page = b.dataset.page; history.replaceState(null, '', '#' + page);
  if (page === 'setup') loadSetup();
  if (page === 'scripts') loadScripts();
  if (page !== 'home') vizStop();
  render();
}));
document.getElementById('quitBtn').addEventListener('click', () => { if (confirm('Закрыть VoiceBox? Звуки и голосовые пресеты перестанут работать.')) api('POST', '/api/quit'); });

// ---------- живые данные ----------
function connectEvents() {
  const es = new EventSource('/api/events?t=' + encodeURIComponent(TOKEN));
  es.addEventListener('levels', e => {
    LV = JSON.parse(e.data);
    for (const [id, v] of Object.entries(LV.strips || {})) vuSet('src_' + id, v);
    for (const k of ['mic_in', 'voice_out', 'monitor']) vuSet(k, LV[k] || 0);
  });
  es.addEventListener('status', e => {
    const prev = ST; ST = JSON.parse(e.data);
    renderPills(); renderGame();
    if (prev && (prev.preset !== ST.preset || prev.mic_monitor !== ST.mic_monitor) && (page === 'home' || page === 'voice')) render();
  });
  es.addEventListener('log', e => {
    const line = JSON.parse(e.data);
    S && S.logs.push(line);
    const nm = line.match(/🔔 (.*)$/); if (nm) toast('🔔 ' + nm[1]);
    const el = document.getElementById('log');
    if (el) { const atEnd = el.scrollTop + el.clientHeight >= el.scrollHeight - 30; el.append('\n' + line); if (atEnd) el.scrollTop = el.scrollHeight; }
    if (/Обнаружены изменения|Кнопка голосового чата|VB-Cable установлен/.test(line) && Date.now() - lastSave > 3000 && !pendingCfg) clearTimeout(window._rl), window._rl = setTimeout(() => !document.querySelector('#modal:not(.hidden)') && load(), 600);
  });
  es.onerror = () => { document.getElementById('pills').replaceChildren(h('span', {class: 'pill bad'}, 'Нет связи с программой…')); };
}
function go(p) { if (p === 'timers') p = 'integrations'; const b = document.querySelector(`[data-page=${p}]`); if (b) b.click(); }
load().then(() => { connectEvents(); loadSetup(); const hp = location.hash.slice(1); if (hp) go(hp); else if (!S.config.setup_done) go('setup'); })
  .catch(e => document.getElementById('page').replaceChildren(h('p', {class: 'bad'}, 'Ошибка: ' + e.message)));
