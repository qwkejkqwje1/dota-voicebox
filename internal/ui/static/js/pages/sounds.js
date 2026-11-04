// Звуки — панель плиток: клик — в эфир, 🎧 — послушать самому, правый клик — настроить.
import {_t, _tf, dec} from '../i18n.js';
import {app, h, api, act, set, toast, setState, requestRender, onLive, kbd, keyLabel, fmtDur, dialog, confirmDlg, captureKey, emojiPicker, colorPicker,
  slider, seg, select, toggle, playSound, empty, hotkeyOwner, actionLabel, bindKey, clone, meter, hint} from '../core.js';
import {trimEditor, trimDialog, canTrim, presetTitle, fmtT} from '../wave.js';

let filter = 'all';
let query = '';

const KIND = {file: _t('Файлы'), phrase: _t('Фразы'), builtin: _t('Встроенные')};
const BGROUP = {fun: _t('🎉 Весёлые'), signal: _t('🔔 Сигналы'), dota: '🐲 Dota 2', voice: _t('🗣️ Голос')};
export const padSounds = () => {
  const by = new Map((app.state.sounds || []).map(s => [s.id, s]));
  return (app.state.pad || []).map(id => by.get(id)).filter(s => s && s.on_pad);
};
// наборы звуков: свои плитки для каждой игры
const allSets = () => app.state.config.sets || [];
const curSet = () => allSets().find(x => x.id === app.state.config.active_set) || null;
const match = (s, f) => f === 'all' || (f === 'menu' ? s.in_menu : f.startsWith('cat:') ? s.category === f.slice(4) : (s.kind === f || (f === 'phrase' && s.kind === 'mixed' && s.text)));

export default function soundsPage() {
  const all = padSounds();
  if (!all.length) {
    return h('div', {},
      empty('🔊', _t('Здесь будут ваши звуки'), _t('Перетащите в окно файлы MP3, WAV или OGG, добавьте фразу для озвучки или выберите встроенный звук. Нажатие на плитку — и звук слышат все в голосовом чате.'),
        h('button', {class: 'btn primary lg', onclick: () => addDialog('file')}, _t('＋ Добавить звук')),
        h('button', {class: 'btn lg', onclick: () => addDialog('rec')}, _t('🎙️ Записать с микрофона')),
        h('button', {class: 'btn lg', onclick: () => addDialog('builtin')}, _t('✨ Встроенные звуки'))));
  }
  const cur = curSet();
  const byId = new Map(all.map(s => [s.id, s]));
  const base = cur ? (cur.sounds || []).map(id => byId.get(id)).filter(Boolean) : all;
  const cats = [...new Set(base.map(s => s.category).filter(Boolean))];
  const filters = [['all', _t('Все')]];
  for (const k of Object.keys(KIND)) if (base.some(s => match(s, k))) filters.push([k, KIND[k]]);
  for (const c of cats) filters.push(['cat:' + c, c]);
  if (base.some(s => s.in_menu)) filters.push(['menu', _t('⚡ В меню')]);
  if (!filters.some(f => f[0] === filter)) filter = 'all';
  const list = base.filter(s => match(s, filter));
  const canDrag = filter === 'all';

  const grid = h('div', {class: 'tiles'});
  const tiles = new Map();
  for (const s of list) { const t = tile(s, canDrag); tiles.set(s.id, t); grid.append(t); }
  const addTile = (title, sub, fn) => h('div', {class: 'tile add', tabindex: 0, onclick: fn, onkeydown: e => e.key === 'Enter' && fn()},
    h('div', {class: 'plus'}, '＋'), h('b', {}, title), h('small', {}, sub));
  grid.append(cur ? addTile(_t('Звуки в набор'), _t('выбрать из всех звуков'), () => setDialog(cur, {sounds: true}))
    : addTile(_t('Добавить звук'), _t('файл, фраза или встроенный'), () => addDialog('file')));
  if (canDrag) {
    dnd(grid, () => base.map(s => s.id), ids => {
      if (cur) {
        cur.sounds = ids.concat((cur.sounds || []).filter(id => !ids.includes(id)));
        api('/api/sets/order', {id: cur.id, order: ids}).catch(er => toast(er.message, 'err'));
      } else {
        app.state.pad = ids;
        api('/api/sounds/order', {order: ids}).catch(er => toast(er.message, 'err'));
      }
    });
  }

  const search = h('input', {class: 'input', type: 'search', placeholder: _t('Найти звук…'), value: query, oninput: e => { query = e.target.value; applySearch(); }});
  const applySearch = () => {
    const q = query.trim().toLowerCase();
    let shown = 0;
    for (const [id, el] of tiles) { const ok = !q || el.dataset.search.includes(q); el.classList.toggle('hidden', !ok); shown += ok; }
    nothing.classList.toggle('hidden', shown > 0 || !q);
  };
  const nothing = h('p', {class: 'hint hidden', style: 'padding:20px 4px'}, _t('Ничего не нашлось.'));
  setTimeout(applySearch);

  onLive('levels', lv => {
    const now = new Map((lv.playing || []).map(p => [p.id, p]));
    for (const [id, el] of tiles) {
      const p = now.get(id);
      el.classList.toggle('playing', !!p);
      el.lastChild.firstChild.style.width = p && p.len ? Math.min(100, p.pos / p.len * 100) + '%' : '0';
    }
  });

  const body = cur && !base.length
    ? empty(cur.emoji || '🗂️', _tf('В наборе «{0}» пока нет звуков', cur.name), _t('Выберите звуки из уже добавленных или добавьте новые — они сразу попадут в этот набор.'),
      h('button', {class: 'btn primary', onclick: () => setDialog(cur, {sounds: true})}, _t('☑️ Выбрать из всех звуков')),
      h('button', {class: 'btn', onclick: () => addDialog('file')}, _t('＋ Новый звук')),
      h('button', {class: 'btn ghost', onclick: () => act('/api/sets/active', {id: ''})}, _t('🎛️ Показать все звуки')))
    : [grid, nothing];
  return h('div', {},
    setsBar(cur, all),
    h('div', {class: 'pad-tools'},
      h('div', {class: 'search'}, search),
      filters.length > 1 ? h('div', {class: 'filters'}, filters.map(([v, l]) => h('button', {class: v === filter ? 'on' : '', onclick: () => { filter = v; requestRender(); }}, l))) : null,
      h('div', {class: 'grow'}),
      allSets().length ? null : h('button', {class: 'btn ghost', title: _t('Свои плитки для каждой игры: включаются сами, когда игра на экране'), onclick: () => setDialog()}, _t('🗂️ Наборы для игр')),
      h('button', {class: 'btn ghost', title: _t('Остановить все звуки'), onclick: () => act('/api/stop', {})}, _t('⏹ Стоп')),
      h('button', {class: 'btn', title: _t('Записать звук с микрофона'), onclick: () => addDialog('rec')}, _t('🎙️ Записать')),
      h('button', {class: 'btn primary', onclick: () => addDialog('file')}, _t('＋ Добавить'))),
    body,
    h('p', {class: 'hint', style: 'margin-top:16px'}, cur
      ? _tf('💡 Включён набор «{0}»: эти звуки — на панели, в быстром меню, на колесе и в пульте. Новые звуки попадают в него сами. Плитки можно перетаскивать.', cur.name)
      : _t('💡 Нажмите на плитку — звук услышат все в голосовом чате. 🎧 — послушать самому. Правый клик — настроить. Плитки можно перетаскивать.')));
}

// setNote — подсказка «включён набор» для страниц меню и колеса (null — набор не включён).
export function setNote(what) {
  const cur = curSet();
  if (!cur) return null;
  return h('div', {class: 'hint box accent row nowrap', style: 'margin-bottom:12px'},
    h('span', {class: 'grow'}, (cur.emoji || '🗂️') + _tf(' Включён набор «{0}» — ', cur.name) + what),
    h('button', {class: 'btn sm', onclick: () => act('/api/sets/active', {id: ''}, {ok: _t('🎛️ Все звуки')})}, _t('🎛️ Все звуки')));
}

// setsBar — переключатель наборов над панелью (если наборы есть).
function setsBar(cur, all) {
  const list = allSets();
  if (!list.length) return null;
  const c = app.state.config, have = new Set(all.map(s => s.id));
  const games = st => (st.games || []).map(x => ((app.state.games || []).find(g => g.exe.includes(x)) || {name: x}).name);
  const chip = (st, n) => {
    const id = st ? st.id : '', on = (cur ? cur.id : '') === id, g = st ? [...new Set(games(st))] : [];
    return h('button', {type: 'button', class: 'opt' + (on ? ' on' : ''),
      title: st ? (g.length ? _t('Игры: ') + g.join(', ') + (c.auto_set ? _t(' — включается сам, когда игра на экране') : '') + '\n' : '') + _t('Правый клик — изменить набор') : _t('Все звуки панели'),
      onclick: () => { if (!on) act('/api/sets/active', {id}); },
      oncontextmenu: st ? e => { e.preventDefault(); setDialog(st); } : null},
    h('span', {class: 'pe'}, st ? st.emoji || '🗂️' : '🎛️'), h('span', {}, st ? st.name : _t('Все звуки')), h('span', {class: 'auto'}, n + (st && g.length && c.auto_set ? _t(' · авто') : '')));
  };
  const hk = (app.state.hotkeys || {})['set:next'];
  return h('div', {class: 'sets-bar'},
    h('div', {class: 'opts grow'},
      chip(null, all.length),
      list.map(st => chip(st, (st.sounds || []).filter(id => have.has(id)).length)),
      h('button', {type: 'button', class: 'opt add', title: _t('Новый набор звуков'), onclick: () => setDialog()}, h('span', {class: 'pe'}, '＋'), h('span', {}, _t('Набор')))),
    h('div', {class: 'row nowrap'},
      toggle(c.auto_set, v => set({auto_set: v}, {ok: v ? _t('🎮 Набор будет включаться сам, когда его игра на экране') : _t('Наборы включаются только вручную')}),
        {label: _t('Сам по игре'), title: _t('Включать набор, когда его игра на экране')}),
      kbd(hk, {empty: _t('⌨ клавиша'), title: _t('Клавиша «следующий набор» — переключать наборы прямо в игре'), onclick: () => bindKey('set:next')}),
      cur ? h('button', {class: 'btn sm', title: _t('Название, игры и звуки набора'), onclick: () => setDialog(cur)}, _t('✏️ Изменить')) : null));
}

// setDialog — создать или изменить набор. opts.sounds — сразу к выбору звуков.
export function setDialog(orig, opts = {}) {
  const all = padSounds();
  const known = app.state.games || [];
  const knownExe = new Set(known.flatMap(g => g.exe));
  const st = orig ? clone(orig) : {id: '', name: '', emoji: '', sounds: [], games: []};
  st.sounds = st.sounds || [];
  st.games = st.games || [];
  let showEmoji = false, custom = '', q = '';
  const has = x => st.games.includes(x);
  const gameOn = g => g.exe.some(has);
  const toggleGame = g => {
    if (gameOn(g)) st.games = st.games.filter(x => !g.exe.includes(x));
    else {
      st.games = st.games.concat(g.exe.filter(x => !has(x)));
      if (!st.name.trim()) st.name = g.name;
      if (!st.emoji) st.emoji = g.emoji;
    }
    d.update();
  };
  const addCustom = () => {
    let x = custom.trim().toLowerCase().replace(/^.*[\\/]/, '').replace(/^"|"$/g, '');
    if (!x) return;
    if (!/\.exe$/.test(x)) x += '.exe';
    if (!has(x)) st.games = st.games.concat(x);
    custom = '';
    d.update();
  };
  const d = dialog({title: orig ? _tf('Набор «{0}»', orig.name) : _t('Новый набор звуков'), wide: true, body: () => {
    const recent = (app.state.recent_apps || []).filter(x => !knownExe.has(x));
    const others = [...new Set(st.games.filter(x => !knownExe.has(x)).concat(recent))];
    const count = h('span', {class: 'flabel'});
    const paintCount = () => { count.textContent = _t('Звуки набора · ') + st.sounds.filter(id => all.some(s => s.id === id)).length + _t(' из ') + all.length; };
    paintCount();
    const rows = all.map(s => {
      const on = st.sounds.includes(s.id);
      const el = h('label', {class: 'spick' + (on ? ' on' : ''), 'data-search': [s.title, s.category, s.text].join(' ').toLowerCase()},
        h('input', {type: 'checkbox', checked: on, onchange: e => {
          st.sounds = e.target.checked ? st.sounds.concat(s.id) : st.sounds.filter(x => x !== s.id);
          el.classList.toggle('on', e.target.checked);
          paintCount();
        }}),
        h('span', {class: 'pe'}, s.emoji || '🔊'), h('span', {class: 'ell'}, s.title));
      return el;
    });
    const filterRows = () => { const ql = q.trim().toLowerCase(); for (const r of rows) r.classList.toggle('hidden', !!ql && !r.dataset.search.includes(ql)); };
    filterRows();
    const auto = app.state.config.auto_set;
    return [
      orig ? null : h('p', {class: 'hint', style: 'margin:0 0 14px'}, _t('Набор — свои плитки для игры. Включите его, и панель, быстрое меню, колесо и пульт покажут только эти звуки. Набор может включаться сам, когда игра на экране.')),
      h('div', {class: 'row nowrap', style: 'gap:10px;align-items:flex-end'},
        h('button', {class: 'btn set-emo', title: _t('Выбрать значок'), onclick: () => { showEmoji = !showEmoji; d.update(); }}, st.emoji || '🗂️'),
        h('label', {class: 'field grow'}, h('span', {}, _t('Название')),
          h('input', {class: 'input', value: st.name, maxlength: 40, autofocus: !orig, placeholder: _t('Например: Dota 2 или «Тихий вечер»'), oninput: e => { st.name = e.target.value; }}))),
      showEmoji ? h('div', {class: 'mt-s'}, emojiPicker(st.emoji, x => { st.emoji = x; showEmoji = false; d.update(); })) : null,
      h('div', {class: 'flabel mt'}, _t('Игры — когда одна из них на экране, набор включается сам')),
      h('div', {class: 'opts mt-s'},
        known.map(g => h('button', {type: 'button', class: 'opt' + (gameOn(g) ? ' on' : ''), title: g.exe.join(', '), onclick: () => toggleGame(g)},
          h('span', {class: 'pe'}, g.emoji), h('span', {}, g.name))),
        others.map(x => h('button', {type: 'button', class: 'opt' + (has(x) ? ' on' : ''), title: has(x) ? _t('Нажмите, чтобы убрать') : _t('Недавно была на экране'),
          onclick: () => { st.games = has(x) ? st.games.filter(y => y !== x) : st.games.concat(x); d.update(); }},
        h('span', {class: 'pe'}, '🖥️'), h('span', {}, x)))),
      h('div', {class: 'row mt-s'},
        h('input', {class: 'input', style: 'max-width:240px', placeholder: _t('Другая игра: game.exe'), value: custom,
          oninput: e => { custom = e.target.value; }, onkeydown: e => { if (e.key === 'Enter') { e.preventDefault(); addCustom(); } }}),
        h('button', {class: 'btn sm', onclick: addCustom}, _t('Добавить')),
        h('span', {class: 'hint'}, _t('Запустите игру — она появится в списке сама.'))),
      st.games.length && !auto ? h('div', {class: 'row mt-s'}, toggle(false, v => set({auto_set: v}, {render: false}), {label: _t('Включать наборы сами, когда игра на экране (сейчас выключено)')})) : null,
      h('div', {class: 'row mt', style: 'gap:6px'}, count, h('div', {class: 'grow'}),
        h('input', {class: 'input sm-input', type: 'search', placeholder: _t('Найти…'), value: q, oninput: e => { q = e.target.value; filterRows(); }}),
        h('button', {class: 'btn sm ghost', onclick: () => { st.sounds = st.sounds.concat(all.map(s => s.id).filter(id => !st.sounds.includes(id))); d.update(); }}, _t('Все')),
        h('button', {class: 'btn sm ghost', onclick: () => { st.sounds = []; d.update(); }}, _t('Ни одного'))),
      all.length ? h('div', {class: 'spicks mt-s'}, rows) : h('p', {class: 'hint mt-s'}, _t('Звуков пока нет — добавьте их на панель, и они появятся здесь.')),
    ];
  }, foot: () => [
    orig ? h('button', {class: 'btn danger', onclick: async () => {
      if (!await confirmDlg(_t('Звуки останутся на панели — удалится только набор.'), {title: _tf('Удалить набор «{0}»?', orig.name), ok: _t('Удалить'), danger: true})) return;
      if (await act('/api/sets/delete', {id: orig.id}, {ok: _t('Набор удалён')})) d.close();
    }}, _t('🗑 Удалить')) : null,
    h('div', {class: 'grow'}),
    h('button', {class: 'btn', onclick: () => d.close()}, _t('Отмена')),
    h('button', {class: 'btn primary', onclick: save}, orig ? _t('Сохранить') : _t('Создать набор'))]});
  if (opts.sounds) setTimeout(() => { const l = d.el.querySelector('.spicks'); if (l) l.scrollIntoView({block: 'nearest'}); });
  async function save() {
    const name = st.name.trim();
    if (!name) { toast(_t('Назовите набор'), 'err'); const i = d.el.querySelector('.dlg-b input.input'); if (i) i.focus(); return; }
    const r = await act('/api/sets/save', {set: {...st, name, emoji: st.emoji || '🗂️'}}, {render: !!orig, ok: orig ? _t('Сохранено') : null});
    if (!r) return;
    if (!orig) await act('/api/sets/active', {id: r.id}, {ok: _tf('✅ Набор «{0}» создан и включён', name)});
    d.close();
  }
}

function meta(s) {
  if (s.error) return [h('span', {}, '⚠ ' + s.error)];
  if (s.pending) return [h('span', {}, _t('Готовлю голос…'))];
  const out = [];
  if (s.duration) out.push(h('span', {}, fmtDur(s.duration)));
  if (s.kind === 'phrase' || s.text) out.push(h('span', {title: _t('Фраза: ') + (s.text || '')}, '🗣️'));
  if (s.bus === 'monitor') out.push(h('span', {title: _t('Слышите только вы')}, _t('🎧 только вам')));
  if (s.bus === 'voice') out.push(h('span', {title: _t('Только в эфир, вы не слышите')}, _t('📡 в эфир')));
  if (s.in_menu) out.push(h('span', {title: _t('Есть в быстром меню')}, '⚡'));
  if (s.start || s.end) out.push(h('span', {title: _t('Обрезан: ') + fmtT(s.start || 0) + ' – ' + (s.end ? fmtT(s.end) : _t('конец'))}, '✂️'));
  return out;
}

function tile(s, draggable) {
  const el = h('div', {class: 'tile' + (s.pending ? ' pending' : '') + (s.error ? ' broken' : ''), style: {'--c': s.color || '#8b5cf6'},
    tabindex: 0, draggable, 'data-id': s.id, 'data-search': [s.title, s.category, s.text, s.id].join(' ').toLowerCase(),
    title: s.title + (s.hotkey ? ' · ' + keyLabel(s.hotkey) : ''),
    onclick: () => playSound(s.id),
    oncontextmenu: e => { e.preventDefault(); editSound(s.id); },
    onkeydown: e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); playSound(s.id); } if (e.key === 'F2') editSound(s.id); }},
  h('div', {class: 't-top'}, h('div', {class: 'emo'}, s.emoji || '🔊'), s.hotkey ? h('span', {class: 'hk'}, kbd(s.hotkey)) : null),
  h('div', {class: 't-title'}, s.title),
  h('div', {class: 't-meta'}, meta(s)),
  h('div', {class: 't-acts'},
    h('button', {class: 't-act', title: _t('Послушать самому (только в наушники)'), onclick: e => { e.stopPropagation(); playSound(s.id, 'monitor'); }}, '🎧'),
    h('button', {class: 't-act', title: _t('Настроить'), onclick: e => { e.stopPropagation(); editSound(s.id); }}, '✏️')),
  h('div', {class: 't-prog'}, h('i', {style: 'transition:width .1s linear'})));
  return el;
}

// перетаскивание плиток — новый порядок (order — плитки по порядку, save — сохранить новый)
function dnd(grid, order, save) {
  let dragId = null, mark = null;
  const clear = () => { if (mark) mark.classList.remove('drop-before'); mark = null; };
  grid.addEventListener('dragstart', e => {
    const t = e.target.closest('.tile[data-id]'); if (!t) return;
    dragId = t.dataset.id; e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/x-vb', dragId);
    setTimeout(() => t.classList.add('dragging'));
  });
  grid.addEventListener('dragover', e => {
    if (!dragId) return;
    const t = e.target.closest('.tile'); if (!t) return;
    e.preventDefault();
    let ref = t;
    if (t.dataset.id) { const r = t.getBoundingClientRect(); if (e.clientX > r.left + r.width / 2) ref = t.nextElementSibling; }
    if (ref && ref.dataset.id === dragId) ref = null;
    if (ref !== mark) { clear(); mark = ref; if (mark) mark.classList.add('drop-before'); }
  });
  grid.addEventListener('drop', e => {
    if (!dragId) return;
    e.preventDefault();
    const ids = order().filter(id => id !== dragId);
    const refId = mark && mark.dataset.id;
    const at = refId ? ids.indexOf(refId) : ids.length;
    ids.splice(at < 0 ? ids.length : at, 0, dragId);
    clear();
    save(ids);
    requestRender();
  });
  grid.addEventListener('dragend', () => { clear(); dragId = null; for (const t of grid.querySelectorAll('.dragging')) t.classList.remove('dragging'); });
}

// ---------- настройка плитки ----------
const MODES = [['toggle', _t('Остановить звук')], ['interrupt', _t('Запустить заново')], ['overlap', _t('Наложить ещё один')], ['ignore', _t('Ничего (дождаться конца)')]];
export function editSound(id) {
  const s = app.state.sounds.find(x => x.id === id);
  if (!s) return;
  const e = {label: s.title, emoji: s.emoji || '🔊', color: s.color || (app.state.palette || [])[0], bus: s.bus || 'both', volume: s.volume || 1,
    mode: s.mode || 'toggle', cooldown: s.cooldown || 0, category: s.category || ''};
  let text = s.text || '', hotkey = s.hotkey || '', inMenu = !!s.in_menu, showEmoji = false;
  const cats = [...new Set(app.state.sounds.map(x => x.category).filter(Boolean))];
  const sets = allSets();
  let inSets = sets.filter(x => (x.sounds || []).includes(s.id)).map(x => x.id);
  const setsKey = l => l.slice().sort().join('\n'), sets0 = setsKey(inSets);
  const d = dialog({title: _t('Настройка звука'), wide: true, body: () => {
    const cur = app.state.sounds.find(x => x.id === s.id) || s; // после обрезки — новая длина
    const prev = h('div', {class: 'tile', style: {'--c': e.color, width: '170px', cursor: 'default', flex: 'none'}},
      h('div', {class: 't-top'}, h('div', {class: 'emo'}, e.emoji), hotkey ? h('span', {class: 'hk'}, kbd(hotkey)) : null),
      h('div', {class: 't-title'}, e.label || _t('Без названия')), h('div', {class: 't-meta'}, fmtDur(cur.duration)));
    return [
      h('div', {class: 'row nowrap', style: 'align-items:flex-start;gap:18px'}, prev,
        h('div', {class: 'col grow', style: 'gap:12px'},
          h('label', {class: 'field'}, h('span', {}, _t('Название')), h('input', {class: 'input', value: e.label, maxlength: 40, oninput: ev => { e.label = ev.target.value; prev.querySelector('.t-title').textContent = e.label || _t('Без названия'); }})),
          h('div', {class: 'row'}, h('button', {class: 'btn sm', onclick: () => { showEmoji = !showEmoji; d.update(); }}, e.emoji + _t(' Значок')), colorPicker(e.color, c => { e.color = c; prev.style.setProperty('--c', c); })))),
      showEmoji ? h('div', {class: 'mt-s'}, emojiPicker(e.emoji, x => { e.emoji = x; showEmoji = false; d.update(); })) : null,
      s.text != null && s.text !== '' || s.kind === 'phrase' ? h('label', {class: 'field mt'}, h('span', {}, _t('Что сказать (голос Windows)')),
        h('textarea', {class: 'input', rows: 2, value: text, oninput: ev => { text = ev.target.value; }})) : null,
      h('div', {class: 'form mt'},
        h('div', {class: 'col'}, h('span', {class: 'flabel'}, _t('Кто слышит')),
          seg([['both', _t('Все')], ['voice', _t('Только эфир')], ['monitor', _t('Только я')]], e.bus, v => { e.bus = v; })),
        h('div', {class: 'col'}, h('span', {class: 'flabel'}, _t('Клавиша')),
          h('div', {class: 'row'}, kbd(hotkey, {empty: _t('не задана')}), h('button', {class: 'btn sm', onclick: async () => {
            const k = await captureKey({title: _tf('Клавиша для «{0}»', (e.label || s.title)), current: hotkey, action: 'sound:' + s.id});
            if (k !== null) { hotkey = k; d.update(); }
          }}, hotkey ? _t('Изменить') : _t('Назначить')))),
        slider({label: _t('Громкость'), value: e.volume, min: 0, max: 2, step: 0.05, fmt: v => Math.round(v * 100) + '%', onInput: v => { e.volume = v; }}),
        slider({label: _t('Не чаще раза в'), value: e.cooldown, min: 0, max: 30, step: 0.5, fmt: v => v ? dec(v) + _t(' с') : _t('без ограничений'), onInput: v => { e.cooldown = v; }}),
        h('label', {class: 'field'}, h('span', {}, _t('Повторное нажатие, пока звук играет')), select(MODES, e.mode, v => { e.mode = v; })),
        h('label', {class: 'field'}, h('span', {}, _t('Группа (фильтр на панели)')),
          h('input', {class: 'input', value: e.category, list: 'vb-cats', placeholder: _t('без группы'), oninput: ev => { e.category = ev.target.value; }}),
          h('datalist', {id: 'vb-cats'}, cats.map(c => h('option', {value: c})))),
        h('div', {class: 'full'}, toggle(inMenu, v => { inMenu = v; }, {label: _t('Показывать в быстром меню поверх игры')})),
        sets.length ? h('div', {class: 'full'}, h('span', {class: 'flabel'}, _t('В каких наборах')),
          h('div', {class: 'opts mt-s'}, sets.map(x => h('button', {type: 'button', class: 'opt' + (inSets.includes(x.id) ? ' on' : ''), onclick: ev => {
            inSets = inSets.includes(x.id) ? inSets.filter(y => y !== x.id) : inSets.concat(x.id);
            ev.currentTarget.classList.toggle('on', inSets.includes(x.id));
          }}, h('span', {class: 'pe'}, x.emoji || '🗂️'), h('span', {}, x.name))))) : null),
      h('p', {class: 'hint mt', style: 'word-break:break-all'}, _t('Источник: '), (s.files || []).map(f => f.replace(/^builtin:/, _t('встроенный: ')).replace(/^tts:/, _t('фраза: '))).join(' + ') || s.id,
        cur.start || cur.end ? _t(' · ✂️ обрезан: ') + fmtT(cur.start || 0) + ' – ' + (cur.end ? fmtT(cur.end) : _t('конец')) : null),
    ];
  }, foot: () => [
    h('button', {class: 'btn danger', onclick: async () => {
      if (!await confirmDlg(_tf('Плитка «{0}» будет удалена', s.title) + (s.kind === 'file' ? _t(', файл переместится в папку trash.') : '.'), {title: _t('Удалить звук?'), ok: _t('Удалить'), danger: true})) return;
      if (await act('/api/sounds/delete', {id: s.id}, {ok: _t('Звук удалён')})) d.close();
    }}, _t('🗑 Удалить')),
    h('div', {class: 'grow'}),
    canTrim(s) ? h('button', {class: 'btn', title: _t('Срезать тишину или лишнее начало и конец'), onclick: () => trimDialog(s, () => { if (!d.closed) d.update(); })}, _t('✂️ Обрезать')) : null,
    h('button', {class: 'btn', title: _t('Только в наушники'), onclick: () => playSound(s.id, 'monitor')}, _t('🎧 Послушать')),
    h('button', {class: 'btn', onclick: () => d.close()}, _t('Отмена')),
    h('button', {class: 'btn primary', onclick: async () => {
      const edit = {...e, label: e.label.trim()};
      if (s.text != null && s.text !== '' || s.kind === 'phrase') { if (!text.trim()) { toast(_t('Фраза не может быть пустой'), 'err'); return; } if (text !== s.text) edit.text = text; }
      if (hotkey !== (s.hotkey || '')) edit.hotkey = hotkey;
      if (inMenu !== !!s.in_menu) edit.in_menu = inMenu;
      if (setsKey(inSets) !== sets0) edit.sets = inSets;
      if (await act('/api/sounds/update', {id: s.id, edit}, {ok: _t('Сохранено')})) d.close();
    }}, _t('Сохранить'))]});
}

// ---------- добавление ----------
export function addDialog(tab = 'file', opts = {}) {
  let files = [];   // {name, state: '…'|'ok'|'err', msg}
  let phrase = '', pEmoji = opts.dota ? '📣' : '🗣️';
  const builtins = app.state.builtins || [];
  const rec = recorder(() => d);
  const d = dialog({title: opts.dota ? _t('Новая кнопка-оповещение') : _t('Добавить звук'), wide: true, onClose: () => rec.reset(), body: () => {
    const tabs = opts.dota ? null : h('div', {class: 'tabs'}, [['file', _t('📁 Файл')], ['rec', _t('🎙️ Запись')], ['phrase', _t('🗣️ Фраза')], ['builtin', _t('✨ Встроенные')]].map(([v, l]) =>
      h('button', {class: tab === v ? 'on' : '', onclick: () => { if (tab === 'rec' && v !== 'rec') rec.reset(); tab = v; d.update(); }}, l)));
    if (tab === 'rec') return [tabs, rec.body()];
    if (tab === 'file') {
      const inp = h('input', {type: 'file', multiple: true, accept: 'audio/*,video/*,.mp3,.wav,.ogg,.flac,.m4a,.opus,.webm', class: 'hidden',
        onchange: ev => upload([...ev.target.files])});
      const zone = h('div', {class: 'drop', onclick: () => inp.click(),
        ondragover: ev => { ev.preventDefault(); zone.classList.add('over'); }, ondragleave: () => zone.classList.remove('over'),
        ondrop: ev => { ev.preventDefault(); ev.stopPropagation(); zone.classList.remove('over'); document.body.classList.remove('dropzone-over'); upload([...ev.dataTransfer.files]); }},
      h('div', {class: 'big'}, '🎵'), h('b', {}, _t('Перетащите файлы сюда')), h('div', {class: 'hint mt-s'}, _t('или нажмите, чтобы выбрать · MP3, WAV, OGG, FLAC, M4A — можно сразу несколько')), inp);
      return [tabs, zone, files.length ? h('div', {class: 'files'}, files.map(f => h('div', {class: 'f'},
        h('span', {}, f.state === 'ok' ? '✅' : f.state === 'err' ? '❌' : '⏳'), h('span', {class: 'ell grow'}, f.name), h('small', {class: f.state === 'err' ? 'bad' : 'faint'}, f.msg || '')))) : null];
    }
    if (tab === 'phrase') {
      return [tabs,
        h('label', {class: 'field'}, h('span', {}, opts.dota ? _t('Что сказать команде') : _t('Что сказать')),
          h('textarea', {class: 'input', rows: 2, autofocus: true, maxlength: 300, placeholder: opts.dota ? _t('Например: Осторожно, у них Рошан!') : _t('Например: Всем привет, я на связи!'), value: phrase, oninput: ev => { phrase = ev.target.value; }})),
        h('div', {class: 'mt'}, h('span', {class: 'flabel'}, _t('Значок')), h('div', {class: 'mt-s'}, emojiPicker(pEmoji, x => { pEmoji = x; }))),
        h('p', {class: 'hint mt'}, _t('🗣️ Фразу озвучит голос Windows (выбрать голос и скорость — в «Настройках»). ') + (opts.dota ? _t('Перед фразой прозвучит короткий сигнал.') : _t('Озвучка готовится один раз и потом играет мгновенно.')))];
    }
    const list = builtins.filter(b => b.group !== 'dota' || app.state.config.dota.enabled);
    const groups = [...new Set(list.map(b => b.group || _t('Звуки')))];
    const have = new Set(app.state.sounds.filter(s => s.kind === 'builtin').flatMap(s => s.files).map(f => f.replace('builtin:', '')));
    return [tabs, h('p', {class: 'hint', style: 'margin:0 0 12px'}, _t('Короткие звуки, созданные прямо в программе. ▶ — послушать самому, + — добавить на панель.')),
      groups.map(g => [h('div', {class: 'flabel', style: 'margin:10px 0 6px'}, BGROUP[g] || g),
        h('div', {class: 'builtins', style: 'max-height:none'}, list.filter(b => (b.group || _t('Звуки')) === g).map(b => h('div', {class: 'bi'},
          h('span', {class: 'be'}, b.emoji || '🔊'), h('div', {class: 'bt'}, h('div', {class: 'ell', title: b.label || b.id}, b.label || b.id), h('small', {}, fmtDur(b.duration))),
          h('button', {class: 'btn sm icon ghost', title: _t('Послушать самому'), onclick: () => playSound(b.id, 'monitor')}, '▶'),
          have.has(b.id) ? h('span', {class: 'btn sm icon ghost ok', title: _t('Уже на панели'), style: 'cursor:default'}, '✓')
            : h('button', {class: 'btn sm icon', title: _t('Добавить на панель'), onclick: async () => { if (await act('/api/sounds/builtin', {name: b.id}, {ok: '✅ ' + (b.label || b.id) + _t(' — на панели')})) d.update(); }}, '+'))))])];
  }, foot: () => {
    if (tab === 'rec') return rec.foot();
    if (tab === 'phrase') return [h('div', {class: 'grow'}),
      h('button', {class: 'btn', onclick: () => phrase.trim() ? act('/api/tts/say', {text: phrase, to: 'monitor'}) : toast(_t('Напишите фразу'), 'err')}, _t('🎧 Послушать')),
      h('button', {class: 'btn primary', onclick: async () => {
        if (!phrase.trim()) { toast(_t('Напишите фразу'), 'err'); return; }
        if (await act('/api/sounds/phrase', {text: phrase, emoji: pEmoji, dota: !!opts.dota}, {ok: _t('✅ Фраза добавлена')})) d.close();
      }}, _t('Добавить'))];
    return [h('div', {class: 'grow'}), h('button', {class: 'btn', onclick: () => d.close()}, _t('Готово'))];
  }});
  if (opts.dota) tab = 'phrase', d.update();
  async function upload(list) {
    if (!list.length) return;
    const items = list.map(f => ({name: f.name, state: '…', msg: _t('читаю…')}));
    files = files.concat(items); d.update();
    await importFiles(list, (i, state, msg) => { items[i].state = state; items[i].msg = msg; if (!d.closed) d.update(); });
  }
}

// ---------- запись с микрофона ----------
const clock = sec => Math.floor(sec / 60) + ':' + String(Math.floor(sec % 60)).padStart(2, '0');
// recorder — вкладка «Запись»: записать → обрезать → сохранить звуком на панели. dlg() — окно.
function recorder(dlg) {
  const r = {phase: 'idle', wave: null, ed: null, name: '', effect: false, poll: 0, busy: false, max: 60};
  const time = h('b', {class: 'rec-time'}, '0:00');
  const bar = h('i', {});
  const upd = () => { const d = dlg(); if (d && !d.closed) d.update(); };
  async function start() {
    if (r.busy) return;
    r.busy = true;
    try { const st = await api('/api/rec/start', {}); r.max = st.max || 60; } catch (e) { toast(e.message, 'err'); r.busy = false; return; }
    r.busy = false;
    r.phase = 'rec';
    time.textContent = '0:00'; bar.style.width = '0';
    upd();
    r.poll = setInterval(async () => {
      let st;
      try { st = await api('/api/rec'); } catch (e) { return; }
      if (r.phase !== 'rec') return;
      time.textContent = clock(st.sec || 0);
      bar.style.width = Math.min(100, (st.sec || 0) / r.max * 100) + '%';
      if (st.full || !st.active) stop();
    }, 100);
  }
  async function stop() {
    clearInterval(r.poll); r.poll = 0;
    if (r.phase !== 'rec' || r.busy) return;
    r.busy = true;
    let w;
    try { w = await api('/api/rec/stop', {}); } catch (e) { toast(e.message, 'err'); r.phase = 'idle'; r.busy = false; upd(); return; }
    r.busy = false;
    r.wave = w;
    // сразу без тишины по краям — её почти всегда нужно срезать
    r.ed = trimEditor({wave: {...w, start: w.sound[0], end: w.sound[1]},
      play: (a, b) => api('/api/rec/play', {start: a, end: b, effect: r.effect}),
      stop: () => api('/api/play', {id: 'preview', stop: true}).catch(() => {})});
    r.phase = 'done';
    upd();
  }
  async function reset() {
    clearInterval(r.poll); r.poll = 0;
    if (r.ed) r.ed.stop();
    const was = r.phase;
    r.phase = 'idle'; r.wave = r.ed = null;
    if (was !== 'idle' && was !== 'saved') await api('/api/rec/cancel', {}).catch(() => {});
  }
  const micOff = () => !!(app.state.config.mic || {}).mute;
  function body() {
    if (r.phase === 'rec') {
      const btn = h('button', {class: 'rec-btn on', title: _t('Остановить запись (пробел)'), onclick: stop}, h('i'));
      setTimeout(() => btn.focus());
      return h('div', {class: 'rec-box on'}, btn,
        h('div', {class: 'grow'},
          h('div', {class: 'row', style: 'gap:8px'}, h('span', {class: 'rec-dot'}), h('b', {}, _t('Идёт запись')), time, h('span', {class: 'faint'}, _t('из ') + clock(r.max))),
          h('div', {class: 'rec-prog mt-s'}, bar),
          h('div', {class: 'mt-s'}, meter('mic_in', {big: true, title: _t('Громкость микрофона')}))));
    }
    if (r.phase === 'done') {
      const fx = presetTitle();
      return [
        r.wave.silent ? hint(_t('🤫 Похоже, микрофон молчал. Проверьте его на странице «Голос» и запишите ещё раз.'), 'warn') : null,
        r.ed.el,
        h('div', {class: 'form mt'},
          h('label', {class: 'field'}, h('span', {}, _t('Название')),
            h('input', {class: 'input', value: r.name, maxlength: 40, placeholder: _t('Например: Мой клич'), oninput: ev => { r.name = ev.target.value; }})),
          fx ? h('div', {class: 'col', style: 'justify-content:flex-end'},
            toggle(r.effect, v => { r.effect = v; }, {label: _tf('С эффектом голоса «{0}»', fx), title: _t('Записать так, как вас слышит команда: тембр и текущий эффект')})) : null),
        h('p', {class: 'hint mt'}, _t('💡 Тишину по краям уже срезали — подвиньте ручки, если нужно. ▶ — послушать самому.'))];
    }
    const off = micOff();
    const btn = h('button', {class: 'rec-btn', title: _t('Начать запись'), disabled: off, onclick: start}, h('i'));
    if (!off) setTimeout(() => btn.isConnected && btn.focus());
    return [
      h('div', {class: 'rec-box'}, btn,
        h('div', {class: 'grow'}, h('b', {}, _t('Запишите звук с микрофона')),
          h('p', {class: 'hint', style: 'margin:4px 0 10px'}, _t('Фразу, клич, смех — до ') + clock(r.max).replace(/^0:/, '') + _t(' секунд. Шумоподавление уже работает, эффект голоса можно добавить после записи. Тишину по краям программа срежет сама.')),
          meter('mic_in', {big: true, title: _t('Громкость микрофона')}))),
      off ? hint([_t('🔇 Микрофон выключен. '), h('button', {class: 'btn sm primary', style: 'margin-left:6px', onclick: async () => {
        await set({mic_mute: false}, {ok: _t('🎙️ Микрофон включён')}); upd();
      }}, _t('Включить'))], 'warn') : null];
  }
  function foot() {
    if (r.phase !== 'done') return [h('div', {class: 'grow'}), h('button', {class: 'btn', onclick: () => dlg().close()}, _t('Закрыть'))];
    return [
      h('button', {class: 'btn ghost', onclick: async () => { await reset(); start(); }}, _t('🎙️ Записать заново')),
      h('div', {class: 'grow'}),
      h('button', {class: 'btn', onclick: () => dlg().close()}, _t('Отмена')),
      h('button', {class: 'btn primary', onclick: async () => {
        const [a, b] = r.ed.get();
        const name = r.name.trim();
        const res = await act('/api/rec/save', {name, start: a, end: b, effect: r.effect}, {ok: '✅ ' + (name ? '«' + name + '»' : _t('Запись')) + _t(' — на панели')});
        if (res && res.id) { r.ed.stop(); r.phase = 'saved'; dlg().close(); }
      }}, _t('Сохранить звук'))];
  }
  return {body, foot, reset};
}

// importFiles — добавить файлы: WAV и MP3 — как есть, остальное (и нечитаемые) — перекодировать в WAV прямо в окне.
// ids — сюда попадут id добавленных звуков.
export async function importFiles(list, progress = () => {}, ids = []) {
  let added = 0, failed = 0;
  for (let i = 0; i < list.length; i++) {
    const f = list[i];
    const name = f.name.replace(/\.[^.]+$/, '');
    try {
      if (f.size > 48 << 20) throw new Error(_t('файл больше 48 МБ'));
      let blob = f, fname = f.name;
      const ext = (f.name.match(/\.[^.]+$/) || [''])[0].toLowerCase();
      let ok = false;
      if (ext === '.wav' || ext === '.mp3') {
        const v = await api('/api/sounds/validate?ext=' + ext, f).catch(() => ({ok: false}));
        ok = v.ok;
      }
      if (!ok) { progress(i, '…', _t('перекодирую…')); blob = await toWAV(f); fname = name + '.wav'; }
      progress(i, '…', _t('добавляю…'));
      const fd = new FormData();
      fd.append('files', blob, fname);
      const r = await api('/api/sounds/import', fd);
      if (r.ids) ids.push(...r.ids);
      if (r.state) setState(r.state);
      added++;
      progress(i, 'ok', _t('добавлен'));
    } catch (e) {
      failed++;
      progress(i, 'err', e.message);
    }
  }
  if (added) toast(_t('✅ Добавлено звуков: ') + added, 'ok');
  if (failed && !added) toast(_t('Не удалось добавить: ') + failed, 'err');
  return added;
}
async function toWAV(file) {
  const buf = await file.arrayBuffer();
  const ctx = new (window.OfflineAudioContext || window.webkitOfflineAudioContext)(1, 1, 48000);
  let audio;
  try { audio = await ctx.decodeAudioData(buf); } catch (e) { throw new Error(_t('формат не поддерживается')); }
  const ch = Math.min(2, audio.numberOfChannels), sr = audio.sampleRate, n = audio.length;
  if (n / sr > 600) throw new Error(_t('длиннее 10 минут'));
  const dv = new DataView(new ArrayBuffer(44 + n * ch * 2));
  const w = (o, s) => { for (let i = 0; i < s.length; i++) dv.setUint8(o + i, s.charCodeAt(i)); };
  w(0, 'RIFF'); dv.setUint32(4, 36 + n * ch * 2, true); w(8, 'WAVE'); w(12, 'fmt '); dv.setUint32(16, 16, true);
  dv.setUint16(20, 1, true); dv.setUint16(22, ch, true); dv.setUint32(24, sr, true); dv.setUint32(28, sr * ch * 2, true);
  dv.setUint16(32, ch * 2, true); dv.setUint16(34, 16, true); w(36, 'data'); dv.setUint32(40, n * ch * 2, true);
  const chans = Array.from({length: ch}, (_, c) => audio.getChannelData(c));
  let o = 44;
  for (let i = 0; i < n; i++) for (let c = 0; c < ch; c++) { const v = Math.max(-1, Math.min(1, chans[c][i])); dv.setInt16(o, v < 0 ? v * 32768 : v * 32767, true); o += 2; }
  return new Blob([dv.buffer], {type: 'audio/wav'});
}
