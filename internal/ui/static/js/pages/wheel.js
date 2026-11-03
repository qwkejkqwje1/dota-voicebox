// Колесо звуков (как колесо чата в Dota 2): включение, клавиша, 8 секторов, вид.
// Превью повторяет колесо поверх игры (internal/overlay/wheel_draw.go): те же радиусы, зазоры и цвета.
// Сектор — щелчок (выбрать, что в нём) или перетаскивание на другой сектор (поменять местами).
import {app, h, act, kbd, bindKey, slider, seg, toggle, setrow, hint, toast, clone, dialog, icon, keyLabel} from '../core.js';
import {padSounds, setNote} from './sounds.js';

const N = 8, RIN = 66, ROUT = 178, GAP = 6, POP = 7, CR = 56, MARGIN = 22;
const SIZE = 2 * (ROUT + POP + MARGIN), C = SIZE / 2;
const STEP = 2 * Math.PI / N;
const ARROWS = ['↑', '↗', '→', '↘', '↓', '↙', '←', '↖'];
const DIRS = ['вверх', 'вверх-вправо', 'вправо', 'вниз-вправо', 'вниз', 'вниз-влево', 'влево', 'вверх-влево'];
const FONT = '"Segoe UI Variable Text", "Segoe UI", system-ui, sans-serif';

const wcfg = () => clone(app.state.config.wheel);
const save = (w, opt = {}) => act('/api/wheel', {wheel: w}, opt);
const esc = s => String(s).replace(/[&<>"]/g, c => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;'})[c]);
const f2 = v => v.toFixed(2);

export default function wheelPage() {
  const w = wcfg(), wh = app.state.wheel || {items: [], auto: true};
  const top = h('div', {class: 'card'},
    setrow('Колесо звуков', 'Как колесо чата в Dota 2: зажали клавишу — вокруг курсора 8 звуков, повели мышь к нужному — отпустили.',
      toggle(w.enabled, async v => {
        w.enabled = v;
        const r = await save(w, {ok: v ? '🎯 Колесо включено' : 'Колесо выключено'});
        if (r && v && !(app.state.wheel || {}).key) bindWheelKey();
      }, {big: true})),
    w.enabled ? [
      setrow('Клавиша', 'Держите — колесо на экране. Отпустите — прозвучит выбранный звук.', kbd(wh.key, {onclick: bindWheelKey, empty: 'назначить'})),
      h('div', {class: 'row', style: 'padding-top:12px;border-top:1px solid var(--line)'},
        h('button', {class: 'btn primary', onclick: () => act('/api/wheel/preview', {}, {ok: 'Колесо открыто — посмотрите на экран'})}, '▶ Показать сейчас'),
        h('span', {class: 'hint'}, 'Откроется поверх всех окон: щёлкните по звуку или нажмите 1–8.'))] : null);
  const left = [top, howCard(wh)];
  if (w.enabled) {
    left.push(hint(['🖥️ Колесо видно поверх игр в режиме экрана ', h('b', {}, '«Оконный без рамки»'),
      ' (в Dota 2: Настройки → Видео → «Окно без рамки»). В эксклюзивном полноэкранном режиме Windows не даёт рисовать поверх игры.'], 'accent'));
    left.push(slotsCard(w, wh), lookCard(w));
  }
  return h('div', {class: 'grid2', style: 'grid-template-columns:minmax(380px,1fr) minmax(400px,1fr)'},
    h('div', {class: 'stack'}, left),
    h('div', {style: 'position:sticky;top:0'}, previewCard(w, wh)));
}

function bindWheelKey() {
  return bindKey('wheel', {title: 'Клавиша колеса звуков',
    hint: 'Её держат, пока выбирают звук. Возьмите клавишу, которой в игре ничего не назначено (например, бывшую клавишу колеса чата), или боковую кнопку мыши.'});
}

// ---------- как пользоваться ----------
function howCard(wh) {
  const k = wh.key ? kbd(wh.key) : h('b', {}, 'клавишу');
  const step = (n, title, text) => h('div', {class: 'wh-step'}, h('span', {class: 'wh-n'}, n), h('div', {}, h('b', {}, title), h('span', {class: 'dim'}, ' — ' + text)));
  return h('div', {class: 'card'}, h('h3', {}, '🎮 Как пользоваться'),
    h('div', {class: 'wh-steps'},
      step(1, ['Зажмите ', k], 'колесо появится вокруг курсора.'),
      step(2, 'Ведите мышь к звуку', 'сектор подсветится. Далеко вести не нужно.'),
      step(3, 'Отпустите', 'звук прозвучит. Не сдвинули мышь — ничего не случится.')),
    h('p', {class: 'hint', style: 'margin:12px 0 0'}, 'Нажали коротко — колесо останется на экране: щёлкните по сектору или нажмите цифру 1–8. ',
      'Esc или правая кнопка мыши — закрыть.'));
}

// ---------- секторы ----------
// manualItems — свой набор ровно из 8 мест (из автоподбора — то, что сейчас в колесе).
function manualItems(w) {
  if (w.items && w.items.length) {
    const l = w.items.slice(0, N);
    while (l.length < N) l.push('');
    return l;
  }
  return (app.state.wheel.items || []).slice(0, N).map(it => it.missing ? '' : it.act).concat(Array(N).fill('')).slice(0, N);
}
function setSlot(i, a) {
  const w = wcfg(), items = manualItems(w);
  const j = a ? items.indexOf(a) : -1;
  if (j >= 0 && j !== i) items[j] = items[i]; // уже в другом секторе — меняются местами
  items[i] = a;
  w.items = items;
  save(w);
}
function swapSlots(i, j) {
  const w = wcfg(), items = manualItems(w);
  [items[i], items[j]] = [items[j], items[i]];
  w.items = items;
  save(w, {ok: 'Поменялись местами: ' + (i + 1) + ' ⇄ ' + (j + 1)});
}
function slotsCard(w, wh) {
  const items = wh.items || [];
  const rows = wh.auto ? [] : items.map((it, i) => h('div', {class: 'pick wh-row', title: 'Изменить сектор ' + (i + 1), onclick: () => pickSlot(i)},
    h('span', {class: 'num'}, i + 1), h('span', {class: 'wh-arrow', title: DIRS[i]}, ARROWS[i]),
    it.act && !it.missing ? [h('span', {class: 'pe'}, it.emoji || '🔊'), h('span', {class: 'ell grow'}, it.label)]
      : it.missing ? [h('span', {class: 'pe'}, '⚠️'), h('span', {class: 'ell grow warn'}, it.label + ' — удалён или спрятан')]
        : [h('span', {class: 'pe faint'}, '·'), h('span', {class: 'ell grow faint'}, 'пусто')],
    it.act ? h('button', {class: 'btn sm icon ghost', title: 'Очистить сектор', onclick: e => { e.stopPropagation(); setSlot(i, ''); }}, icon('x', 14))
      : h('span', {class: 'wh-add'}, '＋ выбрать')));
  return h('div', {class: 'card'},
    wh.auto ? setNote('в колесе первые 8 его звуков.') : null,
    h('div', {class: 'card-h'}, h('h3', {}, '🎯 Секторы'), h('div', {class: 'grow'}),
      seg([['auto', 'Первые 8 с панели'], ['manual', 'Свой набор']], wh.auto ? 'auto' : 'manual', v => {
        const x = wcfg();
        x.items = v === 'auto' ? [] : manualItems(x);
        save(x, {ok: v === 'auto' ? 'В колесе — первые 8 плиток со страницы «Звуки»' : 'Теперь можно выбрать, что в каждом секторе'});
      })),
    wh.auto ? h('p', {class: 'hint', style: 'margin:0'}, 'По часовой стрелке от верхнего — первые 8 плиток со страницы «Звуки» (их порядок меняется там перетаскиванием). ',
      'Чтобы выбрать звуки самому, щёлкните по сектору на колесе или переключите на «Свой набор».') :
      [h('p', {class: 'hint', style: 'margin:0 0 10px'}, 'В секторе может быть звук, эффект голоса, «Стоп» или кнопка микрофона. Порядок — перетаскиванием секторов на колесе.'),
        h('div', {class: 'pick-list', style: 'max-height:none'}, rows)]);
}

// pickSlot — окно выбора, что будет в секторе i.
function pickSlot(i) {
  const wh = app.state.wheel, cur = (wh.items[i] || {}).act || '';
  const used = new Map();
  wh.items.forEach((it, k) => { if (it.act && !used.has(it.act)) used.set(it.act, k); });
  const pad = padSounds(), presets = app.state.presets || [];
  let q = '', first = null;
  const choose = a => { d.close(); if (a !== cur) setSlot(i, a); };
  const row = (a, emoji, title, sub) => {
    const k = used.get(a);
    if (!first) first = a;
    return h('div', {class: 'pick' + (a === cur ? ' on' : ''), onclick: () => choose(a)},
      h('span', {class: 'pe'}, emoji), h('span', {class: 'ell grow'}, title, sub ? h('span', {class: 'faint'}, ' · ' + sub) : null),
      k != null && k !== i ? h('span', {class: 'badge dim', title: 'Уже в секторе ' + (k + 1) + ' — сектора поменяются местами'}, ARROWS[k] + ' ' + (k + 1)) : null,
      a === cur ? h('span', {class: 'badge'}, 'сейчас') : null);
  };
  const list = h('div', {class: 'pick-list', style: 'max-height:min(52vh,440px)'});
  const fill = () => {
    first = null;
    const m = t => !q || String(t).toLowerCase().includes(q);
    const snd = pad.filter(s => m(s.title)), fx = presets.filter(p => m(p.title));
    const btn = [['stop', '⏹️', 'Стоп', 'остановить все звуки'], ['mic_mute', '🎙️', 'Микрофон', 'выключить / включить']].filter(b => m(b[2]) || m(b[3]));
    list.replaceChildren(...[
      snd.length ? [h('div', {class: 'flabel wh-lab'}, 'Звуки'), snd.map(s => row('sound:' + s.id, s.emoji || '🔊', s.title, s.hotkey ? keyLabel(s.hotkey) : ''))] : null,
      fx.length ? [h('div', {class: 'flabel wh-lab'}, 'Эффекты голоса'), fx.map(p => row('preset:' + p.id, p.emoji || '🎛️', p.title))] : null,
      btn.length ? [h('div', {class: 'flabel wh-lab'}, 'Кнопки'), btn.map(b => row(b[0], b[1], b[2], b[3]))] : null,
      !snd.length && !fx.length && !btn.length ? h('p', {class: 'hint', style: 'text-align:center;margin:18px 0'}, 'Ничего не нашлось') : null,
    ].flat(3).filter(Boolean));
  };
  const inp = h('input', {class: 'input', placeholder: 'Найти звук или эффект…', autofocus: true,
    oninput: () => { q = inp.value.trim().toLowerCase(); fill(); },
    onkeydown: e => { if (e.key === 'Enter' && first) choose(first); }});
  fill();
  const d = dialog({title: 'Сектор ' + (i + 1) + ' · ' + ARROWS[i] + ' ' + DIRS[i],
    body: [inp, h('div', {style: 'height:10px'}), list],
    foot: [cur ? h('button', {class: 'btn ghost', onclick: () => choose('')}, 'Очистить сектор') : null, h('div', {class: 'grow'}),
      h('button', {class: 'btn', onclick: () => d.close()}, 'Отмена')]});
}

// ---------- вид ----------
function lookCard(w) {
  return h('div', {class: 'card'}, h('h3', {}, '🎨 Вид'),
    h('div', {}, setrow('Где появляется', w.center === 'screen' ? 'Всегда посередине экрана — направление считается от курсора' : 'Вокруг курсора мыши, как в Dota 2',
      seg([['cursor', '🖱 У курсора'], ['screen', '🖥️ По центру экрана']], w.center || 'cursor', v => { w.center = v; save(w); }))),
    h('div', {class: 'grid2', style: 'grid-template-columns:1fr 1fr;gap:12px 24px;margin-top:10px'},
      slider({label: 'Размер', value: w.scale || 1, min: 0.7, max: 1.6, step: 0.05, fmt: v => Math.round(v * 100) + '%',
        onInput: v => { w.scale = v; live(w); }, onChange: v => { w.scale = v; save(w, {render: false}); }}),
      slider({label: 'Прозрачность', value: Math.round((1 - (w.opacity || 0.92)) * 100) / 100, min: 0, max: 0.7, step: 0.02, fmt: v => Math.round(v * 100) + '%',
        title: 'Насколько сквозь секторы видно игру. Значки и надписи остаются чёткими.',
        onInput: v => { w.opacity = 1 - v; live(w); }, onChange: v => { w.opacity = Math.round((1 - v) * 100) / 100; save(w, {render: false}); }})));
}
function live(w) {
  const el = document.getElementById('whPrev');
  if (el) el.replaceWith(previewCard(w, app.state.wheel));
}

// ---------- превью ----------
let measureCtx = null;
function textW(s, weight = 600, size = 12) {
  if (!measureCtx) measureCtx = document.createElement('canvas').getContext('2d');
  measureCtx.font = weight + ' ' + size + 'px ' + FONT;
  return measureCtx.measureText(s).width;
}
function ellipsize(s, maxW, weight, size) {
  s = String(s || '');
  if (textW(s, weight, size) <= maxW) return s;
  let lo = 0, hi = s.length;
  while (lo < hi) { const m = (lo + hi + 1) >> 1; if (textW(s.slice(0, m).trimEnd() + '…', weight, size) <= maxW) lo = m; else hi = m - 1; }
  return s.slice(0, lo).trimEnd() + '…';
}
function wrap2(s, maxW) { // до двух строк (как wrapLines в overlay)
  const words = String(s || '').split(/\s+/).filter(Boolean), lines = [];
  let cur = '';
  for (let k = 0; k < words.length; k++) {
    const t = cur ? cur + ' ' + words[k] : words[k];
    if (textW(t, 700, 13.5) <= maxW || !cur) { cur = t; continue; }
    lines.push(cur);
    cur = words.slice(k).join(' ');
    break;
  }
  if (cur) lines.push(cur);
  return lines.slice(0, 2).map(l => ellipsize(l, maxW, 700, 13.5));
}
const dirOf = (dx, dy) => { let a = Math.atan2(dx, -dy); if (a < 0) a += 2 * Math.PI; return a; };
const angleDiff = (a, b) => { let d = (a - b) % (2 * Math.PI); if (d > Math.PI) d -= 2 * Math.PI; else if (d <= -Math.PI) d += 2 * Math.PI; return d; };
function sectorPath(rin, rout, a0, a1, half) {
  const P = (r, a) => f2(C + r * Math.sin(a)) + ' ' + f2(C - r * Math.cos(a));
  const dO = Math.asin(Math.min(half / rout, 1)), dI = Math.asin(Math.min(half / rin, 1));
  return 'M' + P(rout, a0 + dO) + 'A' + rout + ' ' + rout + ' 0 0 1 ' + P(rout, a1 - dO) + 'L' + P(rin, a1 - dI) +
    'A' + rin + ' ' + rin + ' 0 0 0 ' + P(rin, a0 + dI) + 'Z';
}
// slotLayout — где значок и подпись (подпись — с той стороны, где ей просторнее), как в overlay.
function slotLayout(i) {
  const mid = i * STEP, rc = (RIN + ROUT) / 2 + 2;
  const bx = C + rc * Math.sin(mid), by = C - rc * Math.cos(mid);
  const bs = 38, ch = 8.6, gap = 5, pad = 3, half = GAP / 2, blockH = bs + gap + ch;
  const inside = (x, y) => {
    const dx = x - C, dy = y - C, rr = Math.hypot(dx, dy);
    if (rr < RIN + pad || rr > ROUT - pad) return false;
    return Math.abs(angleDiff(dirOf(dx, dy), mid)) <= STEP / 2 - Math.asin(Math.min((half + pad) / rr, 1));
  };
  const room = base => {
    let w = 0;
    while (w < 70) {
      const x0 = bx - w - 1, x1 = bx + w + 1;
      if (!inside(x0, base) || !inside(x1, base) || !inside(x0, base - ch) || !inside(x1, base - ch)) break;
      w++;
    }
    return w;
  };
  const topY = by - blockH / 2, below = topY + blockH, above = topY + ch;
  const wb = room(below), wa = room(above);
  if (wa > wb + 4) return {bx, badgeY: topY + ch + gap, base: above, maxW: 2 * wa, bs};
  return {bx, badgeY: topY, base: below, maxW: 2 * wb, bs};
}
function wheelSVG(items, op) {
  const half = GAP / 2, a = v => (v / 255).toFixed(3);
  let s = '<svg class="wh-svg" viewBox="0 0 ' + SIZE + ' ' + SIZE + '" xmlns="http://www.w3.org/2000/svg">' +
    '<circle class="wh-back" cx="' + C + '" cy="' + C + '" r="' + (ROUT + 4) + '" fill="#08080c" fill-opacity="' + a(120 * op) + '"/>';
  items.forEach((it, i) => {
    const mid = i * STEP, a0 = mid - STEP / 2, a1 = mid + STEP / 2;
    const empty = !it.act || it.missing, col = it.color || '#8b5cf6';
    const path = sectorPath(RIN, ROUT, a0, a1, half), rim = sectorPath(ROUT - 4, ROUT, a0, a1, half);
    const nx = C + (ROUT - 14) * Math.sin(a0 + Math.PI / 18), ny = C - (ROUT - 14) * Math.cos(a0 + Math.PI / 18);
    s += '<g class="wh-s' + (empty ? ' empty' : '') + (it.active ? ' on' : '') + '" data-v="' + i + '" style="--dx:' + f2(POP * Math.sin(mid)) + 'px;--dy:' + f2(-POP * Math.cos(mid)) + 'px">' +
      '<path class="bg" d="' + path + '" fill="#181820" fill-opacity="' + a((empty ? 140 : 235) * op) + '"/>' +
      (empty ? '' : '<path class="tint" d="' + path + '" fill="' + esc(col) + '" fill-opacity="' + a(18 + 22 * op) + '"/>') +
      '<path class="rim" d="' + rim + '"/>' +
      '<text class="num" x="' + f2(nx) + '" y="' + f2(ny) + '">' + (i + 1) + '</text>';
    const L = slotLayout(i);
    if (!empty) {
      s += '<rect class="bd" x="' + f2(L.bx - L.bs / 2) + '" y="' + f2(L.badgeY) + '" width="' + L.bs + '" height="' + L.bs + '" rx="8" fill="' + esc(col) + '" fill-opacity=".27"/>' +
        '<text class="em" x="' + f2(L.bx) + '" y="' + f2(L.badgeY + L.bs / 2 + 1) + '">' + esc(it.emoji || '🔊') + '</text>' +
        '<text class="lb" x="' + f2(L.bx) + '" y="' + f2(L.base) + '">' + esc(ellipsize(it.label, L.maxW, 600, 12)) + '</text>';
    } else {
      const rc = (RIN + ROUT) / 2 + 2;
      s += '<text class="plus" x="' + f2(C + rc * Math.sin(mid)) + '" y="' + f2(C - rc * Math.cos(mid)) + '">＋</text>';
    }
    s += '</g>';
  });
  s += '<circle cx="' + C + '" cy="' + C + '" r="' + CR + '" fill="#111117" fill-opacity="' + Math.max(op, 0.85).toFixed(3) + '" stroke="#fff" stroke-opacity=".094"/>' +
    '<path class="wh-ptr" d="M' + C + ' ' + (C - CR - 8.5) + 'L' + (C + 7.5) + ' ' + (C - CR + 1) + 'L' + (C - 7.5) + ' ' + (C - CR + 1) + 'Z"/>' +
    '<g class="wh-c"></g>';
  // невидимые «ловушки» мыши: не двигаются, когда сектор выдвигается, — без дрожания на краю
  items.forEach((_, i) => {
    const mid = i * STEP;
    s += '<path class="wh-hit" data-slot="' + i + '" d="' + sectorPath(CR + 2, ROUT + POP, mid - STEP / 2, mid + STEP / 2, 0) + '"/>';
  });
  return s + '</svg>';
}
function centerText(svg, it, sticky) {
  const g = svg.querySelector('.wh-c'), ptr = svg.querySelector('.wh-ptr');
  let lines, hintText = '', cls = 'ct';
  if (it) {
    lines = it.act && !it.missing ? wrap2(it.label, 92) : ['Пусто'];
    hintText = it.act && !it.missing ? 'изменить' : 'выбрать звук';
  } else {
    lines = ['Ведите мышь', 'к звуку'];
    cls = 'ct idle';
  }
  const lh = 17, total = lines.length * lh + (hintText ? 15 : 0);
  let y = C - total / 2 + 13;
  let out = '';
  for (const l of lines) { out += '<text class="' + cls + '" x="' + C + '" y="' + f2(y) + '">' + esc(l) + '</text>'; y += lh; }
  if (hintText) out += '<text class="ch" x="' + C + '" y="' + f2(y + 3.8) + '">' + esc(hintText) + '</text>';
  g.innerHTML = out;
  if (ptr) ptr.style.display = it ? '' : 'none';
}
function previewCard(w, wh) {
  const items = (wh.items || []).slice(0, N);
  while (items.length < N) items.push({act: ''});
  const op = Math.min(Math.max(w.opacity || 0.92, 0.3), 1);
  const box = h('div', {class: 'wh', innerHTML: wheelSVG(items, op)});
  const svg = box.firstChild;
  centerText(svg, null);
  let hover = -1;
  const setHover = i => {
    if (i === hover) return;
    hover = i;
    svg.querySelectorAll('.wh-s').forEach(g => g.classList.toggle('hov', +g.dataset.v === i));
    centerText(svg, i >= 0 ? items[i] : null);
    const ptr = svg.querySelector('.wh-ptr');
    if (ptr && i >= 0) ptr.setAttribute('transform', 'rotate(' + (i * 45) + ' ' + C + ' ' + C + ')');
  };
  const slotAt = (x, y) => { const el = document.elementFromPoint(x, y); const t = el && el.closest && el.closest('.wh-hit'); return t ? +t.dataset.slot : -1; };
  svg.addEventListener('pointerover', e => { const t = e.target.closest('.wh-hit'); if (t && !drag) setHover(+t.dataset.slot); });
  svg.addEventListener('pointerleave', () => { if (!drag) setHover(-1); });
  let drag = null;
  svg.addEventListener('pointerdown', e => {
    const t = e.target.closest('.wh-hit');
    if (!t || e.button !== 0) return;
    e.preventDefault();
    drag = {from: +t.dataset.slot, x: e.clientX, y: e.clientY, moved: false, over: -1};
    const g = n => svg.querySelector('.wh-s[data-v="' + n + '"]');
    const move = ev => {
      if (!drag.moved && Math.hypot(ev.clientX - drag.x, ev.clientY - drag.y) < 5) return;
      if (!drag.moved) { drag.moved = true; box.classList.add('dragging'); g(drag.from).classList.add('src'); }
      const over = slotAt(ev.clientX, ev.clientY);
      if (over !== drag.over) {
        if (drag.over >= 0) g(drag.over).classList.remove('drop');
        drag.over = over;
        if (over >= 0 && over !== drag.from) g(over).classList.add('drop');
        setHover(over);
      }
    };
    const up = ev => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
      const d = drag;
      drag = null;
      box.classList.remove('dragging');
      svg.querySelectorAll('.src,.drop').forEach(x => x.classList.remove('src', 'drop'));
      if (ev.type === 'pointercancel') return;
      if (!d.moved) { pickSlot(d.from); return; }
      const to = slotAt(ev.clientX, ev.clientY);
      if (to >= 0 && to !== d.from) swapSlots(d.from, to);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
  });
  const sc = w.scale || 1;
  const wrapEl = h('div', {class: 'wh-wrap' + (w.enabled ? '' : ' off')}, box);
  const fit = () => {
    if (!wrapEl.isConnected) return;
    const px = Math.max(240, Math.min(SIZE * sc, wrapEl.clientWidth - 16));
    box.style.width = box.style.height = px + 'px';
  };
  requestAnimationFrame(() => { fit(); if (window.ResizeObserver) new ResizeObserver(fit).observe(wrapEl); });
  return h('div', {class: 'card', id: 'whPrev'},
    h('div', {class: 'card-h'}, h('h3', {}, 'Так выглядит в игре'), h('div', {class: 'grow'}), wh.key ? kbd(wh.key) : null),
    wrapEl,
    h('p', {class: 'hint', style: 'margin:10px 0 0'}, '🖱 Щёлкните по сектору — выберите, что в нём. Перетащите сектор на другой — поменяются местами.',
      sc > 1.25 ? ' В игре колесо крупнее, чем здесь.' : ''));
}
