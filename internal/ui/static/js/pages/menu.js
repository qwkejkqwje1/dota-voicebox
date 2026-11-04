// Быстрое меню поверх игры: включение, клавиша, вид (список, полоска, сетка), место на экране, состав.
// Превью рисуется теми же размерами, что и настоящее меню (internal/overlay).
import {_t, _tf} from '../i18n.js';
import {app, h, act, kbd, keyLabel, bindKey, slider, seg, toggle, setrow, hint, playSound, api, toast, clone, select, icon} from '../core.js';
import {padSounds, setNote} from './sounds.js';

let previewTab = 0, previewPage = 0;
const KEYS = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '0'];
const PAGE = 10;
const MARGIN = 18; // прозрачные поля под тень вокруг панели (как в overlay)

const qm = () => clone(app.state.config.quick_menu);
const save = (m, opt = {}) => act('/api/menu', {menu: m}, opt);
const clamp = (v, a, b) => Math.min(Math.max(v, a), b);

const LAYOUTS = [
  ['list', _t('Список'), _t('Строки с подписями и клавишами')],
  ['strip', _t('Полоска'), _t('Плитки в один ряд — сверху или снизу экрана')],
  ['grid', _t('Сетка'), _t('Крупные плитки 5×2, как у стримеров')],
];
const SPOTS = [['top-left', _t('Слева сверху')], ['top', _t('Сверху')], ['top-right', _t('Справа сверху')], ['left', _t('Слева')], ['center', _t('По центру')],
  ['right', _t('Справа')], ['bottom-left', _t('Слева снизу')], ['bottom', _t('Снизу')], ['bottom-right', _t('Справа снизу')]];
const KEYS_HINT = {
  list: _t('В игре: 1–0 или ↑ ↓ и Enter, ← → — страницы, Tab — голос, Backspace — стоп. Enter сразу после открытия — поиск по названию.'),
  strip: _t('В игре: 1–0 или ← → и Enter, ↑ ↓ — страницы, Tab — голос, Backspace — стоп. Enter сразу после открытия — поиск по названию.'),
  grid: _t('В игре: 1–0 или стрелки и Enter, PgUp PgDn — страницы, Tab — голос, Backspace — стоп. Enter сразу после открытия — поиск по названию.'),
};

export default function menuPage() {
  const m = qm(), menu = app.state.menu, key = menu.menu_key;
  const lay = m.layout || 'list';
  const top = h('div', {class: 'card'},
    setrow(_t('Меню поверх игры'), _t('Открывается клавишей прямо в игре: звуки и эффекты голоса — цифрами 1–0, мышью или колёсиком.'),
      toggle(m.enabled, v => { m.enabled = v; save(m, {ok: v ? _t('⚡ Быстрое меню включено') : _t('Быстрое меню выключено')}); }, {big: true})),
    m.enabled ? [
      setrow(_t('Клавиша вызова'), _t('Нажмите в игре — меню появится, ещё раз — скроется. Esc — закрыть.'),
        kbd(key, {onclick: () => bindKey('menu', {title: _t('Клавиша быстрого меню'), hint: _t('Удобно: Alt+~, F1–F12 или клавиша на цифровом блоке. Клавиша не мешает игре — VoiceBox её только слушает.')}), empty: _t('назначить')})),
      setrow(_t('🔎 Поиск и фраза команде'), _t('Меню откроется сразу с поиском: напечатайте название звука или эффекта и нажмите Enter. Ctrl+Enter — напечатанную фразу озвучит голос Windows, её услышит команда. То же — Enter в открытом меню.'),
        kbd(app.state.hotkeys && app.state.hotkeys.say, {onclick: () => bindKey('say', {title: _t('Клавиша поиска и фраз'), hint: _t('Например, Alt+Enter или F9. Пока поиск открыт, всё, что вы печатаете, попадает в него, а не в игру.')}), empty: _t('назначить')})),
      h('div', {class: 'row', style: 'padding-top:12px;border-top:1px solid var(--line)'},
        h('button', {class: 'btn primary', onclick: () => act('/api/menu/preview', {}, {ok: _t('Меню открыто — посмотрите на экран')})}, _t('▶ Показать сейчас')),
        h('span', {class: 'hint'}, _t('Откроется по-настоящему, поверх всех окон.')))] : null);
  if (!m.enabled) {
    return h('div', {class: 'grid2', style: 'grid-template-columns:minmax(380px,1.1fr) minmax(360px,.9fr)'}, h('div', {class: 'stack'}, top),
      h('div', {style: 'position:sticky;top:0'}, previewCard(m, menu)));
  }
  const tip = hint([_t('🖥️ Меню видно поверх игр в режиме экрана '), h('b', {}, _t('«Оконный без рамки»')), _t(' (в Dota 2: Настройки → Видео → «Окно без рамки»). В эксклюзивном полноэкранном режиме Windows не даёт рисовать поверх игры — звуки по клавишам при этом работают.')], 'accent');
  const look = [layoutCard(m), placeCard(m, menu), behaviorCard(m)];
  const content = [soundsCard(m), effectsCard(m)];
  if (lay !== 'list') { // широкое превью — над настройками
    return h('div', {class: 'stack'},
      h('div', {class: 'grid2'}, top, tip),
      previewCard(m, menu),
      h('div', {class: 'grid2'}, h('div', {class: 'stack'}, look), h('div', {class: 'stack'}, content)));
  }
  return h('div', {class: 'grid2', style: 'grid-template-columns:minmax(380px,1.1fr) minmax(360px,.9fr)'},
    h('div', {class: 'stack'}, top, tip, look, content),
    h('div', {style: 'position:sticky;top:0'}, previewCard(m, menu)));
}

// ---------- вид ----------
function layIcon(id) {
  const n = id === 'list' ? 4 : 10;
  return h('div', {class: 'lay-ic ' + id}, h('div', {}, Array.from({length: n}, () => h('i'))));
}
function layoutCard(m) {
  const cur = m.layout || 'list';
  return h('div', {class: 'card'}, h('h3', {}, _t('🎨 Вид')),
    h('div', {class: 'lays'}, LAYOUTS.map(([id, title, desc]) => h('button', {type: 'button', class: 'lay' + (cur === id ? ' on' : ''), onclick: () => {
      if (cur === id) return;
      m.layout = id;
      let ok = title;
      if (id === 'strip' && ['center', 'left', 'right'].includes(m.position)) { // полоска посреди экрана неудобна
        m.position = 'top';
        ok = _t('Полоска — сверху экрана. Место можно поменять ниже');
      }
      save(m, {ok});
    }}, layIcon(id), h('b', {}, title), h('small', {}, desc)))));
}

// ---------- где появляется ----------
// panelSize — размер панели меню в точках экрана при масштабе 1 (как в overlay).
function panelSize(layout, menu) {
  const ns = (menu.sounds || []).length, ne = (menu.effects || []).length;
  const tabs = ne > 0;
  const slots = Math.min(PAGE, Math.max(ns, ne));
  if (layout === 'strip') {
    const tilesW = slots ? slots * 92 + (slots - 1) * 6 : 330;
    return [8 + 16 + 8 + (tabs ? 74 + 8 : 0) + tilesW + (ns > PAGE ? 8 + 30 : 0) + 8, 80];
  }
  if (layout === 'grid') {
    const rows = Math.max(1, Math.ceil(slots / 5));
    return [592, 10 + (tabs ? 42 : 26) + rows * 88 + (rows - 1) * 8 + 8 + 28 + 10];
  }
  const rows = Math.max(1, Math.min(PAGE, ns));
  return [344, 10 + (tabs ? 42 : 26) + rows * 42 - 2 + 6 + 28 + 10];
}
// placeMock — левый верхний угол панели на экране SW×SH (повторяет overlay.Place).
function placeMock(pos, x, y, pw, ph, SW, SH, sc) {
  const M = MARGIN * sc, bw = pw + 2 * M, bh = ph + 2 * M;
  const cx = (SW - bw) / 2, cy = (SH - bh) / 2, top = SH * .08, bottom = SH - bh - SH * .2, left = SW * .02, right = SW - bw - SW * .02;
  const P = {top: [cx, top], bottom: [cx, bottom], left: [left, cy], right: [right, cy], 'top-left': [left, top], 'top-right': [right, top],
    'bottom-left': [left, bottom], 'bottom-right': [right, bottom], center: [cx, cy], cursor: [cx, SH * .42]};
  let px, py;
  if (pos === 'custom') { px = clamp(x ?? .5, 0, 1) * (SW - pw); py = clamp(y ?? 0, 0, 1) * (SH - ph); }
  else { const o = P[pos] || P.center; px = o[0] + M; py = o[1] + M; }
  return [clamp(px, 0, Math.max(0, SW - pw)), clamp(py, 0, Math.max(0, SH - ph))];
}
function placeCard(m, menu) {
  const atCursor = m.position === 'cursor';
  return h('div', {class: 'card', id: 'qmPlace'},
    h('div', {class: 'card-h'}, h('h3', {}, _t('📍 Где появляется')), h('div', {class: 'grow'}),
      seg([['screen', _t('📌 Своё место')], ['cursor', _t('🖱 У курсора')]], atCursor ? 'cursor' : 'screen', v => {
        m.position = v === 'cursor' ? 'cursor' : (m.layout === 'strip' ? 'top' : 'center');
        save(m);
      })),
    screenMap(m, menu),
    h('p', {class: 'hint', style: 'margin:8px 0 12px'}, atCursor ? _t('Меню появится там, где сейчас курсор мыши.')
      : [_t('Перетащите меню на схеме — или прямо в игре: за '), h('span', {class: 'q-grip inl'}), _t(' или за фон меню. Точки — готовые места. Серым — где обычно интерфейс игры.')]),
    setrow(_t('Монитор'), _t('Если мониторов несколько'), select([['game', _t('🎮 Где игра')], ['cursor', _t('🖱 Где курсор мыши')], ['primary', _t('🖥️ Основной')]],
      m.monitor || 'game', v => { m.monitor = v; save(m, {render: false}); })));
}
function screenMap(m, menu) {
  const SW = screen.width || 1920, SH = screen.height || 1080;
  const sc = m.scale || 1, lay = m.layout || 'list';
  let [pw, ph] = panelSize(lay, menu).map(v => v * sc);
  if (pw + 2 * MARGIN * sc > SW) { const k = SW / (pw + 2 * MARGIN * sc); pw *= k; ph *= k; } // полоска ужимается под экран
  const [x, y] = placeMock(m.position, m.x, m.y, pw, ph, SW, SH, sc);
  const pct = (v, of) => (v / of * 100).toFixed(3) + '%';
  const atCursor = m.position === 'cursor';
  const n = lay === 'list' ? Math.min(6, Math.max(2, (menu.sounds || []).length)) : Math.min(PAGE, Math.max(1, Math.min(PAGE, Math.max((menu.sounds || []).length, (menu.effects || []).length))));
  const mock = h('div', {class: 'scr-menu ' + lay, title: atCursor ? '' : _t('Перетащите'), style: {left: pct(x, SW), top: pct(y, SH), width: pct(pw, SW), height: pct(ph, SH)}},
    Array.from({length: lay === 'grid' ? 10 : n}, () => h('i')));
  const box = h('div', {class: 'scr' + (atCursor ? ' cur' : ''), style: {'aspect-ratio': SW + ' / ' + SH}},
    h('i', {class: 'hud top'}), h('i', {class: 'hud map'}), h('i', {class: 'hud bar'}), h('i', {class: 'scr-guide'}), mock,
    atCursor ? h('span', {class: 'scr-cursor', style: {left: '50%', top: pct(SH * .42 + MARGIN * sc, SH)}}, '🖱') : null,
    atCursor ? null : SPOTS.map(([pos, title]) => {
      // точка — край меню, которым оно «прилипает» к стороне экрана
      const [dx, dy] = placeMock(pos, 0, 0, pw, ph, SW, SH, sc);
      const ax = pos.endsWith('left') ? dx : pos.endsWith('right') ? dx + pw : dx + pw / 2;
      const ay = pos.startsWith('top') ? dy : pos.startsWith('bottom') ? dy + ph : dy + ph / 2;
      return h('button', {type: 'button', class: 'scr-dot' + (m.position === pos ? ' on' : ''), title,
        style: {left: pct(clamp(ax, SW * .015, SW * .985), SW), top: pct(clamp(ay, SH * .025, SH * .975), SH)},
        onclick: () => { if (m.position !== pos) { m.position = pos; save(m, {ok: '📍 ' + title}); } }});
    }));
  if (!atCursor) {
    mock.addEventListener('pointerdown', e => {
      if (e.button !== 0) return;
      e.preventDefault();
      mock.setPointerCapture(e.pointerId);
      const bw = box.clientWidth, bh = box.clientHeight, mw = mock.offsetWidth, mh = mock.offsetHeight;
      const maxL = Math.max(0, bw - mw), maxT = Math.max(0, bh - mh);
      const start = {x: e.clientX, y: e.clientY, l: mock.offsetLeft, t: mock.offsetTop};
      let moved = false, fx = 0, fy = 0;
      const move = ev => {
        const dx = ev.clientX - start.x, dy = ev.clientY - start.y;
        if (!moved && Math.abs(dx) + Math.abs(dy) < 3) return;
        if (!moved) box.classList.add('drag');
        moved = true;
        let l = clamp(start.l + dx, 0, maxL);
        const t = clamp(start.t + dy, 0, maxT);
        const snap = Math.abs(l - maxL / 2) < bw * .015; // магнит к середине
        if (snap) l = maxL / 2;
        box.classList.toggle('snap', snap);
        mock.style.left = l + 'px';
        mock.style.top = t + 'px';
        fx = maxL ? l / maxL : .5;
        fy = maxT ? t / maxT : .5;
      };
      const up = () => {
        mock.removeEventListener('pointermove', move);
        box.classList.remove('snap', 'drag');
        if (!moved) return;
        m.position = 'custom';
        m.x = Math.round(fx * 1000) / 1000;
        m.y = Math.round(fy * 1000) / 1000;
        save(m, {ok: _t('📍 Запомнено — здесь меню и появится')});
      };
      mock.addEventListener('pointermove', move);
      mock.addEventListener('pointerup', up, {once: true});
      mock.addEventListener('pointercancel', up, {once: true});
    });
  }
  return box;
}

// ---------- поведение ----------
function behaviorCard(m) {
  return h('div', {class: 'card'}, h('h3', {}, _t('⚙️ Поведение')),
    h('div', {class: 'grid2', style: 'grid-template-columns:1fr 1fr;gap:12px 24px'},
      slider({label: _t('Размер'), value: m.scale || 1, min: 0.8, max: 1.6, step: 0.05, fmt: v => Math.round(v * 100) + '%',
        onInput: v => { m.scale = v; live(m); }, onChange: v => { m.scale = v; save(m, {render: false}); }}),
      slider({label: _t('Прозрачность фона'), value: Math.round((1 - (m.opacity || 0.95)) * 100) / 100, min: 0, max: 0.7, step: 0.05, fmt: v => Math.round(v * 100) + '%',
        title: _t('Насколько сквозь меню видно игру. Надписи остаются чёткими.'),
        onInput: v => { m.opacity = 1 - v; live(m); }, onChange: v => { m.opacity = Math.round((1 - v) * 100) / 100; save(m, {render: false}); }}),
      slider({label: _t('Само закроется через'), value: m.timeout_sec || 0, min: 0, max: 30, step: 1, fmt: v => v ? v + _t(' с') : _t('не закрывать'),
        onChange: v => { m.timeout_sec = v; save(m, {render: false}); }})),
    h('div', {class: 'mt'}, toggle(m.close_after_pick, v => { m.close_after_pick = v; save(m, {render: false}); }, {label: _t('Закрывать после выбора звука')})));
}
// live — перерисовать превью и схему, пока тянут ползунок (сохранится, когда отпустят).
function live(m) {
  for (const [id, fn] of [['qmPrev', () => previewCard(m, app.state.menu)], ['qmPlace', () => placeCard(m, app.state.menu)]]) {
    const el = document.getElementById(id);
    if (el) el.replaceWith(fn());
  }
}

// ---------- состав ----------
function row(s, i, actions) {
  return h('div', {class: 'pick on', style: 'cursor:default'},
    i != null ? h('span', {class: 'num', title: _t('Страница ') + (Math.floor(i / PAGE) + 1)}, KEYS[i % PAGE]) : null,
    h('span', {class: 'pe'}, s.emoji || '🔊'), h('span', {class: 'ell grow'}, s.title || s.label), s.hotkey ? kbd(s.hotkey) : null, actions);
}
function soundsCard(m) {
  const pad = padSounds();
  const manual = (m.items || []).length > 0;
  const byId = new Map(pad.map(s => [s.id, s]));
  const items = (m.items || []).filter(id => byId.has(id));
  const setItems = list => { m.items = list; save(m); };
  return h('div', {class: 'card'},
    setNote(_t('в меню его звуки по порядку панели. Выбор ниже вернётся, когда включите «Все звуки».')),
    h('div', {class: 'card-h'}, h('h3', {}, _t('🔊 Звуки в меню')), h('div', {class: 'grow'}),
      seg([['auto', _t('Первые с панели')], ['manual', _t('Выбрать самому')]], manual ? 'manual' : 'auto', v => {
        if (v === 'auto') setItems([]);
        else setItems(pad.slice(0, PAGE).map(s => s.id));
      })),
    !manual ? h('p', {class: 'hint', style: 'margin:0'}, _t('В меню попадают первые ') + 2 * PAGE + _t(' плиток со страницы «Звуки» в том же порядке (две страницы по 10). Порядок меняется перетаскиванием плиток.'))
      : [h('div', {class: 'flabel', style: 'margin-bottom:6px'}, _t('В меню · ') + items.length),
        items.length ? h('div', {class: 'pick-list', style: 'max-height:none'}, items.map((id, i) => row(byId.get(id), i, h('div', {class: 'row nowrap', style: 'gap:2px'},
          h('button', {class: 'btn sm icon ghost', title: _t('Выше'), disabled: i === 0, onclick: () => { const l = items.slice(); [l[i - 1], l[i]] = [l[i], l[i - 1]]; setItems(l); }}, '↑'),
          h('button', {class: 'btn sm icon ghost', title: _t('Ниже'), disabled: i === items.length - 1, onclick: () => { const l = items.slice(); [l[i + 1], l[i]] = [l[i], l[i + 1]]; setItems(l); }}, '↓'),
          h('button', {class: 'btn sm icon ghost', title: _t('Убрать из меню'), onclick: () => { const l = items.filter(x => x !== id); setItems(l.length ? l : []); }}, icon('x', 14)))))) : null,
        pad.length > items.length ? [h('div', {class: 'flabel', style: 'margin:14px 0 6px'}, _t('Добавить')),
          h('div', {class: 'pick-list', style: 'max-height:260px'}, pad.filter(s => !items.includes(s.id)).map(s =>
            h('div', {class: 'pick', onclick: () => setItems(items.concat(s.id))}, h('span', {class: 'pe'}, s.emoji || '🔊'), h('span', {class: 'ell grow'}, s.title), h('span', {class: 'faint'}, '＋'))))] : null]);
}
function effectsCard(m) {
  const presets = app.state.presets || [];
  const cur = (m.effects && m.effects.length ? m.effects : app.state.menu.effects.map(e => e.id));
  const sel = new Set(cur);
  return h('div', {class: 'card'},
    setrow(_t('🎭 Вкладка «Голос»'), _t('Переключать эффекты голоса прямо в игре: Tab в меню.'), toggle(m.show_effects, v => { m.show_effects = v; save(m); })),
    m.show_effects ? h('div', {class: 'pick-list', style: 'max-height:300px;margin-top:6px'}, presets.map(p => {
      const on = sel.has(p.id);
      const idx = cur.indexOf(p.id);
      return h('div', {class: 'pick' + (on ? ' on' : ''), onclick: () => {
        const next = on ? cur.filter(x => x !== p.id) : presets.map(x => x.id).filter(x => sel.has(x) || x === p.id);
        if (!next.length) { toast(_t('Нужен хотя бы один эффект'), 'err'); return; }
        m.effects = next; save(m);
      }}, h('span', {class: 'num'}, on ? KEYS[idx % PAGE] : ''), h('span', {class: 'pe'}, p.emoji || '🎛️'), h('span', {class: 'ell grow'}, p.title),
      p.hotkey ? kbd(p.hotkey) : null);
    })) : null);
}

// ---------- превью ----------
function footerText(layout, pages, effects) { // как overlay.Footer
  const parts = [_t('1–0 выбор')];
  if (pages > 1) parts.push((layout === 'grid' ? 'PgUp PgDn' : '← →') + _t(' стр.'));
  if (effects) parts.push(_t('Tab голос'));
  parts.push(_t('Enter поиск'));
  if (pages <= 1 && !effects) parts.push(_t('Backspace стоп'));
  parts.push('Esc');
  return parts.join('  ·  ');
}
function previewData(menu) {
  const tabs = [_t('Звуки')].concat(menu.effects && menu.effects.length ? [_t('Голос')] : []);
  if (previewTab >= tabs.length) previewTab = 0;
  const all = previewTab === 0 ? (menu.sounds || []) : (menu.effects || []);
  const pages = Math.max(1, Math.ceil(all.length / PAGE));
  if (previewPage >= pages) previewPage = 0;
  const items = all.slice(previewPage * PAGE, previewPage * PAGE + PAGE);
  const slots = Math.min(PAGE, Math.max((menu.sounds || []).length, tabs.length > 1 ? menu.effects.length : 0));
  return {tabs, pages, items, slots};
}
const bars = () => h('span', {class: 'qb'}, h('i'), h('i'), h('i'));
function tileEl(it, i, big, pick) {
  return h('div', {class: 'qt' + (big ? ' big' : '') + (it.active ? ' on' : '') + (it.pending ? ' pend' : ''), title: it.label, onclick: () => pick(it)},
    h('span', {class: 'k'}, KEYS[i]), it.active ? bars() : null,
    h('span', {class: 'e', style: {background: (it.color || '#8b5cf6') + '46'}}, it.emoji || '🔊'),
    h('span', {class: 'l'}, it.label),
    big ? h('span', {class: 'hk'}, it.pending ? _t('готовится…') : it.key ? keyLabel(it.key) : '') : null);
}
function head(d, setTab, flip) {
  const pageLabel = d.pages > 1 ? '  ' + (previewPage + 1) + '/' + d.pages : '';
  return h('div', {class: 'qm-head'},
    d.tabs.length > 1 ? h('div', {class: 'qm-tabs'}, d.tabs.map((t, i) => h('span', {class: i === previewTab ? 'on' : '',
      title: i === previewTab && d.pages > 1 ? _t('Следующая страница') : null,
      onclick: () => i === previewTab ? (d.pages > 1 && flip(1)) : setTab(i)}, t + (i === previewTab ? pageLabel : ''))))
      : h('div', {class: 'qm-title', onclick: () => d.pages > 1 && flip(1)}, (d.tabs[0] || _t('Быстрое меню')).toUpperCase() + pageLabel),
    h('span', {class: 'q-grip', title: _t('В игре за эту ручку меню можно перетащить')}));
}
const emptyText = () => { // как overlay: Data.Empty
  const c = app.state.config, s = (c.sets || []).find(x => x.id === c.active_set);
  return s ? _tf('В наборе «{0}» нет звуков · Enter — поиск', s.name) : _t('Звуков пока нет — добавьте их в VoiceBox');
};
function listPrev(d, m, pick, setTab, flip) {
  return h('div', {class: 'qm', style: {'--op': m.opacity || 0.95}},
    head(d, setTab, flip),
    d.items.length ? d.items.map((it, i) => h('div', {class: 'qm-row' + (it.active ? ' on' : ''), onclick: () => pick(it)},
      h('span', {class: 'k'}, KEYS[i]), h('span', {class: 'e', style: {background: (it.color || '#8b5cf6') + '46'}}, it.emoji || '🔊'),
      h('span', {class: 'l'}, it.label), it.key ? h('span', {class: 'hk'}, keyLabel(it.key)) : null, it.active ? bars() : null)) :
      h('div', {class: 'qm-row', style: 'color:#71717a;justify-content:center'}, emptyText()),
    h('div', {class: 'qm-foot'}, footerText('list', d.pages, d.tabs.length > 1)));
}
function stripPrev(d, m, pick, setTab, flip) {
  const n = Math.max(d.slots, d.items.length);
  return h('div', {class: 'qms', style: {'--op': m.opacity || 0.95}},
    h('span', {class: 'q-grip', title: _t('В игре за эту ручку меню можно перетащить')}),
    d.tabs.length > 1 ? h('div', {class: 'qs-tabs'}, d.tabs.map((t, i) => h('span', {class: i === previewTab ? 'on' : '', onclick: () => setTab(i)}, t))) : null,
    n ? h('div', {class: 'qs-tiles'}, Array.from({length: n}, (_, i) => d.items[i] ? tileEl(d.items[i], i, false, pick) : h('div', {class: 'qt slot'})))
      : h('div', {class: 'qs-empty'}, emptyText()),
    d.pages > 1 ? h('div', {class: 'qs-pages'}, h('button', {type: 'button', title: _t('Прошлая страница'), onclick: () => flip(-1)}, '▲'),
      h('span', {}, (previewPage + 1) + '/' + d.pages), h('button', {type: 'button', title: _t('Следующая страница'), onclick: () => flip(1)}, '▼')) : null);
}
function gridPrev(d, m, pick, setTab, flip) {
  const n = Math.max(d.slots, d.items.length), rows = Math.max(1, Math.ceil(n / 5));
  return h('div', {class: 'qmg', style: {'--op': m.opacity || 0.95}},
    head(d, setTab, flip),
    n ? h('div', {class: 'qg-tiles'}, Array.from({length: rows * 5}, (_, i) => d.items[i] ? tileEl(d.items[i], i, true, pick) : h('div', {class: 'qt big slot'})))
      : h('div', {class: 'qs-empty', style: 'height:88px'}, emptyText()),
    h('div', {class: 'qm-foot'}, footerText('grid', d.pages, d.tabs.length > 1)));
}
// fitZoom — превью в масштабе меню, но не шире карточки.
function fitZoom(el, naturalW, scale, maxZoom) {
  const apply = () => {
    const wrap = el.parentElement;
    if (!wrap || !el.isConnected) return;
    el.style.zoom = Math.max(0.3, Math.min(scale, maxZoom, (wrap.clientWidth - 28) / naturalW)).toFixed(3);
  };
  requestAnimationFrame(() => {
    apply();
    if (window.ResizeObserver && el.parentElement) new ResizeObserver(apply).observe(el.parentElement);
  });
}
function previewCard(m, menu) {
  const lay = m.layout || 'list';
  const d = previewData(menu);
  const redraw = () => { const el = document.getElementById('qmPrev'); if (el) el.replaceWith(previewCard(m, app.state.menu)); };
  const pick = it => previewTab === 0 ? playSound(it.id, 'monitor') : api('/api/preset', {id: it.id}).catch(e => toast(e.message, 'err'));
  const setTab = i => { previewTab = i; previewPage = 0; redraw(); };
  const flip = dir => { previewPage = (previewPage + dir + d.pages) % d.pages; redraw(); };
  const el = (lay === 'strip' ? stripPrev : lay === 'grid' ? gridPrev : listPrev)(d, m, pick, setTab, flip);
  fitZoom(el, panelSize(lay, menu)[0] + 2, m.scale || 1, lay === 'list' ? 1.25 : 1.4);
  return h('div', {class: 'card', id: 'qmPrev'},
    h('div', {class: 'card-h'}, h('h3', {}, _t('Так выглядит в игре')), h('div', {class: 'grow'}), menu.menu_key ? kbd(menu.menu_key) : null),
    h('div', {class: 'qm-wrap', style: m.enabled ? '' : 'opacity:.45;filter:grayscale(.6)'}, el),
    h('p', {class: 'hint', style: 'margin:10px 0 0'},
      previewTab === 0 ? _t('🎧 Нажмите на ') + (lay === 'list' ? _t('строку') : _t('плитку')) + _t(' — звук прозвучит только у вас. ') : _t('Нажмите — эффект включится, как в игре. '),
      KEYS_HINT[lay] || KEYS_HINT.list));
}
