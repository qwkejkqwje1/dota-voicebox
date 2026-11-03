// Голос — микрофон, улучшение голоса (шумоподавление, тембр, фон), режим «когда меня слышно»,
// эффекты голоса с ручками, осциллограмма.
import {app, h, api, act, set, toast, onLive, kbd, keyLabel, meter, slider, seg, toggle, setrow, sectionH, captureKey, bindKey,
  TOKEN, presetById, debounced, requestRender, select} from '../core.js';

const GROUP_ORDER = ['Основные', 'Рация и связь', 'Персонажи', 'Музыка', 'Пространство', 'Мемы', 'custom'];
const GROUP_TITLE = {custom: 'Свои пресеты'};
let showViz = false;

export default function voicePage() {
  const st = app.status, c = app.state.config;
  return h('div', {class: 'stack'}, micCard(c, st), enhanceCard(c, st), effectPanel(st.preset), effectsGrid(st.preset), vizCard());
}

// ---------- улучшить голос ----------
let showEQ = false;
const DN_LEVELS = [[0.75, 'Мягко', 'Шум тише, голос максимально естественный'], [0.92, 'Обычно', 'Лучший баланс (рекомендуем)'], [1, 'Максимум', 'Полная тишина вокруг голоса']];
const CUSTOM_DESC = 'Свой тембр: подвиньте ползунки — слышно сразу.';
const pick = (on, emoji, title, attrs = {}) => h('button', {type: 'button', class: 'opt' + (on ? ' on' : ''), ...attrs}, h('span', {class: 'pe'}, emoji), h('span', {}, title));

function enhanceCard(c, st) {
  const v = app.state.voice || {};
  const m = c.mic, bg = c.background || {};
  const timbres = v.timbres || [], bands = v.eq_bands || [];
  const cur = m.timbre || '';
  const tb = timbres.find(t => t.id === cur);
  const zero = bands.map(() => 0);
  const base = cur === 'custom' ? {eq: m.eq && m.eq.length ? m.eq : zero, smooth: m.smooth ?? 0.4} : tb ? {eq: tb.eq, smooth: tb.smooth} : {eq: zero, smooth: 0};
  const vals = {eq: [...base.eq], smooth: base.smooth};

  // шумоподавление
  const lvl = DN_LEVELS.reduce((a, b) => Math.abs(b[0] - m.denoise_level) < Math.abs(a[0] - m.denoise_level) ? b : a)[0];
  const dnFail = m.denoise && st.audio && st.audio.running && !st.denoise;
  const denoise = h('div', {class: 'enh-sec'},
    h('div', {class: 'enh-h'}, h('b', {}, '🔇 Шумоподавление'),
      m.denoise ? h('span', {class: 'badge ' + (dnFail ? 'warn' : 'ok')}, dnFail ? 'не запустилось' : 'работает') : null,
      h('div', {class: 'grow'}), toggle(m.denoise, on => set({denoise: on}))),
    h('div', {class: 'row', style: 'gap:14px'},
      h('small', {class: 'dim grow', style: 'min-width:240px'}, 'Нейросеть убирает шум вентилятора, клавиатуры, улицы и эхо комнаты — команда слышит только голос.'),
      m.denoise ? seg(DN_LEVELS, lvl, x => set({denoise_level: x}, {render: false})) : null));

  // тембр
  const desc = h('small', {class: 'dim'}, cur === 'custom' ? CUSTOM_DESC : tb ? tb.desc : 'Голос как есть. Выберите тембр — голос станет чище и выразительнее.');
  const markCustom = () => {
    if (c.mic.timbre === 'custom') return;
    c.mic.timbre = 'custom';
    for (const el of document.querySelectorAll('.opt[data-timbre]')) el.classList.toggle('on', el.dataset.timbre === 'custom');
    desc.textContent = CUSTOM_DESC;
  };
  const sendEQ = debounced(() => set({timbre: 'custom', eq: vals.eq, smooth: vals.smooth}, {render: false}), 120);
  const chooseTimbre = id => {
    if (id === 'custom') { showEQ = true; set({timbre: 'custom', eq: vals.eq, smooth: vals.smooth}); return; }
    set({timbre: id});
  };
  const tChips = [['', '🎙️', 'Без обработки'], ...timbres.map(t => [t.id, t.emoji, t.title, t.desc]), ['custom', '🎚️', 'Свой']].map(([id, e, title, d]) =>
    pick(cur === id, e, title, {'data-timbre': id, title: d || null, onclick: () => chooseTimbre(id)}));
  const db = x => (x > 0 ? '+' : x < 0 ? '−' : '') + Math.abs(x).toFixed(1).replace('.', ',').replace(',0', '') + ' дБ';
  const eq = showEQ ? h('div', {class: 'eq'},
    bands.map((b, i) => slider({label: b.label, title: b.hint + ' (' + (b.freq >= 1000 ? (b.freq / 1000).toString().replace('.', ',') + ' кГц' : b.freq + ' Гц') + ')',
      value: vals.eq[i] || 0, min: -(v.eq_max || 12), max: v.eq_max || 12, step: 0.5, fmt: db,
      onInput: x => { vals.eq[i] = x; markCustom(); sendEQ(); }})),
    slider({label: 'Ровная громкость', title: 'Тихие слова громче, крики тише (компрессор)', value: vals.smooth, min: 0, max: 1, step: 0.05,
      fmt: x => x ? Math.round(x * 100) + '%' : 'выкл', onInput: x => { vals.smooth = x; markCustom(); sendEQ(); }}),
    h('div', {class: 'row full', style: 'justify-content:flex-end'},
      h('small', {class: 'faint grow'}, 'Подсказка: «Чёткость» +3…5 дБ — голос слышно даже в замесе; «Бубнение» −3 дБ — уходит «коробка».'),
      h('button', {class: 'btn sm ghost', onclick: () => { vals.eq = [...zero]; vals.smooth = 0; set({timbre: 'custom', eq: vals.eq, smooth: 0}); }}, '↺ Всё в ноль'))) : null;
  const timbre = h('div', {class: 'enh-sec'},
    h('div', {class: 'enh-h'}, h('b', {}, '🎨 Тембр'), h('div', {class: 'grow'}),
      h('button', {class: 'btn sm' + (showEQ ? ' on' : ''), onclick: () => { showEQ = !showEQ; requestRender(); }}, showEQ ? 'Скрыть эквалайзер' : '🎚️ Эквалайзер')),
    h('div', {class: 'opts'}, tChips), h('div', {style: 'margin-top:8px'}, desc), eq);

  // фон под голос
  const ambs = v.ambiences || [];
  const custom = (bg.id || '').startsWith('sound:') ? bg.id.slice(6) : '';
  const long = (app.state.sounds || []).filter(s => s.on_pad && (s.duration || 0) >= 1 && !s.error);
  const bChips = [['', '🔈', 'Нет'], ...ambs.map(a => [a.id, a.emoji, a.title, a.desc])].map(([id, e, title, d]) =>
    pick((bg.id || '') === id, e, title, {title: d || null, onclick: () => set({bg_id: id})}));
  const ownSel = long.length ? select([['', '🎵 Свой звук…'], ...long.map(s => ['sound:' + s.id, (s.emoji || '🔊') + ' ' + s.title])], custom ? 'sound:' + custom : '',
    x => x && set({bg_id: x}), {cls: 'input opt-sel' + (custom ? ' on' : ''), title: 'Любой звук с панели — по кругу'}) : null;
  const back = h('div', {class: 'enh-sec'},
    h('div', {class: 'enh-h'}, h('b', {}, '🌧️ Фон под голос'), h('div', {class: 'grow'}),
      bg.id ? h('button', {class: 'btn sm', title: 'Послушать фон самому (6 секунд, в наушники)', onclick: () => act('/api/bg/preview', {id: bg.id, volume: bg.volume})}, '🎧 Послушать') : null),
    h('div', {class: 'opts'}, bChips, ownSel),
    bg.id ? h('div', {class: 'grid2', style: 'gap:10px 28px;margin-top:12px;align-items:center'},
      slider({label: 'Громкость фона', value: bg.volume ?? 0.3, min: 0.02, max: 1, step: 0.01, fmt: x => Math.round(x * 100) + '%',
        onChange: x => set({bg_volume: x}, {render: false})}),
      toggle(bg.only_talking !== false, on => set({bg_only_talking: on}), {label: 'Только пока говорю (в паузах — тишина)'}))
      : h('small', {class: 'dim', style: 'display:block;margin-top:8px'}, 'Дождь, костёр, космос — тихо под голосом, для атмосферы. Команда слышит фон вместе с вами.'));

  return h('div', {class: 'card enh'},
    h('div', {class: 'card-h'}, h('h3', {}, '✨ Улучшить голос'), h('span', {class: 'hint'}, 'работает вместе с любым эффектом')),
    denoise, timbre, back,
    h('p', {class: 'hint', style: 'margin:12px 0 0'}, '🎧 Включите «Слышать себя» — и подбирайте на слух. Всё применяется сразу.'));
}

// ---------- микрофон ----------
function micCard(c, st) {
  const muted = !!st.mic_mute;
  const gateOn = (c.mic.gate_db || 0) < 0;
  const t = c.talk || {mode: 'always'};
  return h('div', {class: 'card'},
    h('div', {class: 'micbar'},
      h('div', {class: 'micicon' + (muted ? ' muted' : '')}, muted ? '🔇' : '🎙️'),
      h('div', {style: 'min-width:0'},
        h('div', {class: 'row nowrap', style: 'margin-bottom:8px'}, h('b', {}, 'Микрофон'),
          h('span', {class: 'faint ell', style: 'font-size:12.5px'}, st.audio.mic || 'не найден'),
          muted ? h('span', {class: 'badge warn'}, 'выключен') : null),
        meter('mic_in', {big: true, title: 'Громкость микрофона'})),
      h('div', {class: 'row nowrap'},
        h('button', {class: 'btn' + (muted ? ' on' : ''), onclick: () => set({mic_mute: !muted})}, muted ? '🎙️ Включить' : '🔇 Выключить'),
        h('button', {class: 'btn' + (st.mic_monitor ? ' on' : ''), title: 'Слышать свой голос с эффектом в наушниках',
          onclick: () => act('/api/action/sync', {action: 'mic_monitor'})}, '🎧 Слышать себя'))),
    h('div', {class: 'grid2 mt', style: 'gap:10px 28px'},
      slider({label: 'Громкость микрофона', value: c.mic_gain, min: 0, max: 3, step: 0.05, fmt: v => Math.round(v * 100) + '%',
        onChange: v => set({mic_gain: v}, {render: false})}),
      h('div', {class: 'col', style: 'gap:2px'},
        h('div', {class: 'row'}, toggle(gateOn, on => set({gate_db: on ? -50 : 0}), {label: 'Отсекать тихий фон (шумовой порог)'})),
        gateOn ? slider({label: 'Порог: тише этого — тишина', value: c.mic.gate_db, min: -80, max: -20, step: 1, fmt: v => v + ' дБ',
          onInput: v => { viz.gate = v; }, onChange: v => set({gate_db: v}, {render: false})}) :
          h('small', {class: 'faint'}, 'Включите, если в эфир попадает шум клавиатуры или вентилятора.'))),
    setrow('Когда вас слышно', t.mode === 'always' ? 'Микрофон всегда в эфире (как обычно)' : t.mode === 'ptt' ? 'Только пока держите кнопку' : 'Нажали кнопку — голос включён, ещё раз — выключен',
      seg([['always', '🎙️ Всегда'], ['ptt', '✊ Пока держу'], ['toggle', '🔘 Вкл/выкл кнопкой']], t.mode || 'always', v => set({talk_mode: v}))),
    t.mode && t.mode !== 'always' ? talkKey(c, t) : null);
}
function talkKey(c, t) {
  const game = c.ptt.key;
  const own = !!t.key;
  return h('div', {class: 'col', style: 'gap:10px;padding:2px 0 4px'},
    h('div', {class: 'row'}, h('span', {class: 'dim'}, 'Кнопка:'), kbd(t.key || game, {empty: 'не задана'}),
      h('button', {class: 'btn sm', onclick: async () => {
        const k = await captureKey({title: 'Кнопка, пока держите которую вас слышно', single: true, current: t.key || game, hint: 'Подойдут клавиши, Shift/Ctrl/Alt и боковые кнопки мыши.'});
        if (k !== null) set({talk_key: k === game ? '' : k});
      }}, 'Изменить'),
      own && game ? h('button', {class: 'btn sm ghost', onclick: () => set({talk_key: ''})}, 'Как в игре (' + keyLabel(game) + ')') : null,
      h('div', {class: 'grow'}),
      app.status.talk && app.status.talk.open ? h('span', {class: 'chip live'}, h('span', {class: 'dot'}), 'вас слышно') : null),
    h('small', {class: 'dim'}, !own ? (game ? 'Та же кнопка, что и голосовой чат в игре: держите её как обычно — игра включит войс, а VoiceBox откроет микрофон.' : 'Задайте кнопку голосового чата в «Настройках» или выберите свою.')
      : 'Своя кнопка: VoiceBox откроет микрофон' + (t.press_game_key !== false && game ? ' и сам зажмёт кнопку голосового чата игры (' + keyLabel(game) + ').' : '.')),
    own && game ? toggle(t.press_game_key !== false, v => set({talk_press_game: v}), {label: 'Зажимать кнопку голосового чата игры (' + keyLabel(game) + ')'}) : null,
    h('div', {style: 'max-width:420px'}, slider({label: 'Не обрывать конец фразы', value: t.release_ms ?? 150, min: 0, max: 600, step: 10, fmt: v => v + ' мс',
      onChange: v => set({talk_release_ms: v}, {render: false})})));
}

// ---------- эффект: ручки ----------
function effectPanel(id) {
  const p = presetById(id);
  if (!p) return null;
  const vals = {...(p.values || {})};
  const save = debounced(() => api('/api/preset/params', {id: p.id, values: vals}).then(r => { p.values = r.values; }).catch(e => toast(e.message, 'err')), 200);
  const changed = (p.knobs || []).some(k => vals[k.id] !== undefined && Math.abs(vals[k.id] - k.def) > 1e-9);
  const knobs = (p.knobs || []).map(k => {
    const v = vals[k.id] ?? k.def;
    if (k.kind === 'toggle') return h('div', {class: 'knob-choice'}, h('span', {class: 'flabel'}, k.label), toggle(v >= 0.5, on => { vals[k.id] = on ? 1 : 0; save(); }));
    if (k.kind === 'choice') return h('div', {class: 'knob-choice' + ((k.options || []).length > 3 ? ' wide' : '')}, h('span', {class: 'flabel'}, k.label),
      seg((k.options || []).map(o => [o.value, o.label]), v, x => { vals[k.id] = +x; save(); }));
    const dec = k.step && k.step < 1 ? (String(k.step).split('.')[1] || '').length : 0;
    return slider({label: k.label, value: v, min: k.min, max: k.max, step: k.step || 0.01,
      fmt: x => (k.unit === '%' ? Math.round(x) : (+x).toFixed(dec).replace('.', ',')) + (k.unit ? (k.unit === '%' ? '%' : ' ' + k.unit) : ''),
      onInput: x => { vals[k.id] = x; save(); }});
  });
  return h('div', {class: 'card fx-panel'},
    h('div', {class: 'card-h', style: 'align-items:flex-start'},
      h('div', {class: 'micicon', style: 'background:rgba(139,92,246,.25)'}, p.emoji || '🎛️'),
      h('div', {class: 'grow', style: 'min-width:0'}, h('div', {class: 'faint', style: 'font-size:12px'}, 'Сейчас в эфире'), h('h2', {}, p.title), h('div', {class: 'dim', style: 'font-size:13px'}, p.desc || '')),
      h('div', {class: 'row'},
        h('span', {class: 'dim', style: 'font-size:12.5px'}, 'Клавиша'), kbd(p.hotkey, {onclick: () => bindKey('preset:' + p.id), empty: 'назначить'}),
        changed ? h('button', {class: 'btn sm ghost', onclick: () => act('/api/preset/reset', {id: p.id}, {ok: 'Настройки эффекта сброшены'})}, '↺ Сбросить') : null)),
    knobs.length ? h('div', {class: 'knobs'}, knobs) : h('p', {class: 'hint', style: 'margin:0'}, p.custom ? 'Свой пресет: цепочка эффектов настраивается в config.json (voice_presets).' : 'У этого эффекта нет настроек.'),
    h('p', {class: 'hint', style: 'margin:12px 0 0'}, '🎧 Включите «Слышать себя», чтобы подобрать звучание. Изменения применяются сразу.'));
}

// ---------- сетка эффектов ----------
function effectsGrid(active) {
  const presets = app.state.presets || [];
  const groups = [...new Set(presets.map(p => p.group || 'custom'))].sort((a, b) => (GROUP_ORDER.indexOf(a) + 99) % 99 - (GROUP_ORDER.indexOf(b) + 99) % 99);
  return h('div', {},
    sectionH('Эффекты голоса', 'Нажмите, чтобы включить. В игре переключайте через быстрое меню (вкладка «Голос»).'),
    groups.map(g => [h('div', {class: 'flabel', style: 'margin:14px 0 8px'}, GROUP_TITLE[g] || g),
      h('div', {class: 'fx-grid'}, presets.filter(p => (p.group || 'custom') === g).map(p =>
        h('button', {class: 'fx' + (p.id === active ? ' on' : ''), title: p.desc || p.title, onclick: () => pickFx(p.id)},
          h('span', {class: 'fx-e'}, p.emoji || '🎛️'), h('span', {class: 'fx-t'}, h('span', {class: 'ell'}, p.title)), h('span', {class: 'fx-d'}, p.desc || ''),
          p.hotkey ? kbd(p.hotkey) : null)))]));
}
async function pickFx(id) {
  if (app.status.preset === id) return;
  app.status.preset = id;
  try { await api('/api/preset', {id}); } catch (e) { toast(e.message, 'err'); }
  requestRender();
}

// ---------- осциллограмма ----------
const viz = {ws: null, post: false, gate: 0, spectro: null, canvas: null};
function vizCard() {
  viz.gate = app.state.config.mic.gate_db || 0;
  if (!showViz) {
    vizStop();
    return h('div', {class: 'card'}, h('div', {class: 'card-h', style: 'margin:0'}, h('h3', {}, '📈 Осциллограмма и спектр'), h('span', {class: 'hint'}, 'как звучит голос до и после эффекта'),
      h('div', {class: 'grow'}), h('button', {class: 'btn sm', onclick: () => { showViz = true; requestRender(); }}, 'Показать')));
  }
  const cv = h('canvas', {class: 'viz', width: 1200, height: 260, title: 'Тяните мышью по верхней части — порог шумоподавления'});
  viz.canvas = cv;
  let drag = false;
  const setGate = ev => {
    const r = cv.getBoundingClientRect();
    const y = Math.min(1, Math.max(0, (ev.clientY - r.top) / (r.height * 0.45)));
    let db = Math.round(20 * Math.log10(Math.max(Math.abs(y - 0.5) * 2, 0.0001)));
    db = Math.max(-80, Math.min(-20, db));
    viz.gate = db;
    saveGate(db);
  };
  const saveGate = debounced(db => set({gate_db: db}, {render: false}), 300);
  cv.addEventListener('mousedown', e => { if (e.offsetY / cv.clientHeight < 0.45) { drag = true; setGate(e); } });
  window.addEventListener('mousemove', e => drag && setGate(e));
  window.addEventListener('mouseup', () => { drag = false; });
  setTimeout(vizStart);
  return h('div', {class: 'card'},
    h('div', {class: 'card-h'}, h('h3', {}, '📈 Осциллограмма и спектр'),
      seg([['pre', 'Микрофон'], ['post', 'В эфире (с эффектом)']], viz.post ? 'post' : 'pre', v => { viz.post = v === 'post'; if (viz.ws && viz.ws.readyState === 1) viz.ws.send(v); }),
      h('span', {class: 'chip', id: 'gateInd', style: 'height:26px'}, 'порог'), h('div', {class: 'grow'}),
      h('button', {class: 'btn sm ghost', onclick: () => { showViz = false; vizStop(); requestRender(); }}, 'Скрыть')),
    cv, h('p', {class: 'hint', style: 'margin:8px 0 0'}, 'Сверху — осциллограмма (тяните мышью — порог шумоподавления), ниже — спектр и «водопад».'));
}
function vizStart() {
  if (viz.ws && viz.ws.readyState <= 1) return;
  const ws = new WebSocket(`ws://${location.host}/api/viz?t=${encodeURIComponent(TOKEN)}`);
  ws.binaryType = 'arraybuffer';
  viz.ws = ws;
  ws.onopen = () => { ws.send(viz.post ? 'post' : 'pre'); ws.send('fps:30'); };
  ws.onmessage = e => { if (!viz.canvas || !viz.canvas.isConnected) { vizStop(); return; } draw(new Uint8Array(e.data)); };
  ws.onclose = () => { if (viz.ws === ws) { viz.ws = null; if (showViz && viz.canvas && viz.canvas.isConnected) setTimeout(vizStart, 1500); } };
}
function vizStop() { if (viz.ws) { const w = viz.ws; viz.ws = null; w.onclose = null; w.close(); } }
function draw(d) {
  const cv = viz.canvas;
  const W = cv.clientWidth | 0, H = cv.clientHeight | 0;
  if (!W || !H) return;
  if (cv.width !== W || cv.height !== H) { cv.width = W; cv.height = H; viz.spectro = null; }
  const g = cv.getContext('2d');
  const cols = 256, bands = 96, open = d[1] === 1;
  const wave = d.subarray(2, 2 + cols * 2), spec = d.subarray(2 + cols * 2, 2 + cols * 2 + bands), pk = d[2 + cols * 2 + bands];
  const oh = H * 0.45, sh = H * 0.27, wy = oh + sh;
  g.fillStyle = '#0b0c10'; g.fillRect(0, 0, W, wy);
  g.strokeStyle = '#1c1f29'; g.lineWidth = 1; g.beginPath(); g.moveTo(0, oh / 2); g.lineTo(W, oh / 2); g.stroke();
  if (viz.gate < 0) {
    const a = Math.pow(10, viz.gate / 20) * oh / 2;
    g.fillStyle = 'rgba(245,158,11,.07)'; g.fillRect(0, oh / 2 - a, W, a * 2);
    g.strokeStyle = 'rgba(245,158,11,.7)'; g.setLineDash([4, 4]); g.beginPath();
    g.moveTo(0, oh / 2 - a); g.lineTo(W, oh / 2 - a); g.moveTo(0, oh / 2 + a); g.lineTo(W, oh / 2 + a); g.stroke(); g.setLineDash([]);
    g.fillStyle = '#f59e0b'; g.font = '11px Consolas, monospace'; g.fillText('порог ' + viz.gate + ' дБ', 6, Math.max(12, oh / 2 - a - 4));
  }
  const sx = W / cols;
  const grad = g.createLinearGradient(0, 0, 0, oh);
  grad.addColorStop(0, '#f472b6'); grad.addColorStop(0.5, '#8b5cf6'); grad.addColorStop(1, '#f472b6');
  g.fillStyle = open ? grad : '#3a3f4e';
  for (let i = 0; i < cols; i++) {
    const mn = (wave[i * 2] << 24 >> 24) / 127, mx = (wave[i * 2 + 1] << 24 >> 24) / 127;
    const y1 = oh / 2 - mx * oh / 2, y2 = oh / 2 - mn * oh / 2;
    g.fillRect(i * sx, y1, Math.max(1, sx - 0.5), Math.max(1, y2 - y1));
  }
  const bw = W / bands;
  for (let b = 0; b < bands; b++) {
    const v = spec[b] / 255, bh = v * (sh - 14);
    g.fillStyle = `hsl(${265 - v * 180},75%,${38 + v * 25}%)`;
    g.fillRect(b * bw + 1, oh + sh - bh - 12, bw - 2, bh);
  }
  g.fillStyle = '#5b6070'; g.font = '10px Consolas, monospace';
  [['100', 0.15], ['500', 0.42], ['1k', 0.54], ['4k', 0.77], ['10k', 0.92]].forEach(([t, x]) => g.fillText(t, x * W, oh + sh - 1));
  const wh = Math.max(1, (H - wy) | 0);
  if (!viz.spectro || viz.spectro.height !== wh) viz.spectro = g.createImageData(W, wh);
  const img = viz.spectro, row = W * 4;
  img.data.copyWithin(row, 0, img.data.length - row);
  for (let x = 0; x < W; x++) {
    const v = spec[Math.min(bands - 1, (x / W * bands) | 0)] / 255, i = x * 4;
    img.data[i] = Math.min(255, v * 330); img.data[i + 1] = v > 0.45 ? Math.min(255, (v - 0.45) * 460) : v * 40; img.data[i + 2] = Math.min(255, 60 + v * 300); img.data[i + 3] = 255;
  }
  g.putImageData(img, 0, wy);
  g.fillStyle = pk > 240 ? '#ef4444' : '#8b8fa3'; g.font = '12px Consolas, monospace';
  g.fillText('пик ' + (pk ? (20 * Math.log10(pk / 255)).toFixed(0) : '−∞') + ' дБ', W - 92, 14);
  const gi = document.getElementById('gateInd');
  if (gi) { gi.textContent = viz.gate < 0 ? (open ? '● голос проходит' : '○ тишина (порог)') : 'порог выключен'; gi.className = 'chip' + (viz.gate < 0 && open ? ' ok' : ''); }
}
