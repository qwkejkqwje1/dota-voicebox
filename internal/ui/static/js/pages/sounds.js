// Звуки — панель плиток: клик — в эфир, 🎧 — послушать самому, правый клик — настроить.
import {app, h, api, act, set, toast, setState, requestRender, onLive, kbd, keyLabel, fmtDur, dialog, confirmDlg, captureKey, emojiPicker, colorPicker,
  slider, seg, select, toggle, playSound, empty, hotkeyOwner, actionLabel, bindKey, clone} from '../core.js';

let filter = 'all';
let query = '';

const KIND = {file: 'Файлы', phrase: 'Фразы', builtin: 'Встроенные'};
const BGROUP = {fun: '🎉 Весёлые', signal: '🔔 Сигналы', dota: '🐲 Dota 2', voice: '🗣️ Голос'};
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
      empty('🔊', 'Здесь будут ваши звуки', 'Перетащите в окно файлы MP3, WAV или OGG, добавьте фразу для озвучки или выберите встроенный звук. Нажатие на плитку — и звук слышат все в голосовом чате.',
        h('button', {class: 'btn primary lg', onclick: () => addDialog('file')}, '＋ Добавить звук'),
        h('button', {class: 'btn lg', onclick: () => addDialog('builtin')}, '✨ Встроенные звуки')));
  }
  const cur = curSet();
  const byId = new Map(all.map(s => [s.id, s]));
  const base = cur ? (cur.sounds || []).map(id => byId.get(id)).filter(Boolean) : all;
  const cats = [...new Set(base.map(s => s.category).filter(Boolean))];
  const filters = [['all', 'Все']];
  for (const k of Object.keys(KIND)) if (base.some(s => match(s, k))) filters.push([k, KIND[k]]);
  for (const c of cats) filters.push(['cat:' + c, c]);
  if (base.some(s => s.in_menu)) filters.push(['menu', '⚡ В меню']);
  if (!filters.some(f => f[0] === filter)) filter = 'all';
  const list = base.filter(s => match(s, filter));
  const canDrag = filter === 'all';

  const grid = h('div', {class: 'tiles'});
  const tiles = new Map();
  for (const s of list) { const t = tile(s, canDrag); tiles.set(s.id, t); grid.append(t); }
  const addTile = (title, sub, fn) => h('div', {class: 'tile add', tabindex: 0, onclick: fn, onkeydown: e => e.key === 'Enter' && fn()},
    h('div', {class: 'plus'}, '＋'), h('b', {}, title), h('small', {}, sub));
  grid.append(cur ? addTile('Звуки в набор', 'выбрать из всех звуков', () => setDialog(cur, {sounds: true}))
    : addTile('Добавить звук', 'файл, фраза или встроенный', () => addDialog('file')));
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

  const search = h('input', {class: 'input', type: 'search', placeholder: 'Найти звук…', value: query, oninput: e => { query = e.target.value; applySearch(); }});
  const applySearch = () => {
    const q = query.trim().toLowerCase();
    let shown = 0;
    for (const [id, el] of tiles) { const ok = !q || el.dataset.search.includes(q); el.classList.toggle('hidden', !ok); shown += ok; }
    nothing.classList.toggle('hidden', shown > 0 || !q);
  };
  const nothing = h('p', {class: 'hint hidden', style: 'padding:20px 4px'}, 'Ничего не нашлось.');
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
    ? empty(cur.emoji || '🗂️', 'В наборе «' + cur.name + '» пока нет звуков', 'Выберите звуки из уже добавленных или добавьте новые — они сразу попадут в этот набор.',
      h('button', {class: 'btn primary', onclick: () => setDialog(cur, {sounds: true})}, '☑️ Выбрать из всех звуков'),
      h('button', {class: 'btn', onclick: () => addDialog('file')}, '＋ Новый звук'),
      h('button', {class: 'btn ghost', onclick: () => act('/api/sets/active', {id: ''})}, '🎛️ Показать все звуки'))
    : [grid, nothing];
  return h('div', {},
    setsBar(cur, all),
    h('div', {class: 'pad-tools'},
      h('div', {class: 'search'}, search),
      filters.length > 1 ? h('div', {class: 'filters'}, filters.map(([v, l]) => h('button', {class: v === filter ? 'on' : '', onclick: () => { filter = v; requestRender(); }}, l))) : null,
      h('div', {class: 'grow'}),
      allSets().length ? null : h('button', {class: 'btn ghost', title: 'Свои плитки для каждой игры: включаются сами, когда игра на экране', onclick: () => setDialog()}, '🗂️ Наборы для игр'),
      h('button', {class: 'btn ghost', title: 'Остановить все звуки', onclick: () => act('/api/stop', {})}, '⏹ Стоп'),
      h('button', {class: 'btn primary', onclick: () => addDialog('file')}, '＋ Добавить')),
    body,
    h('p', {class: 'hint', style: 'margin-top:16px'}, cur
      ? '💡 Включён набор «' + cur.name + '»: эти звуки — на панели, в быстром меню, на колесе и в пульте. Новые звуки попадают в него сами. Плитки можно перетаскивать.'
      : '💡 Нажмите на плитку — звук услышат все в голосовом чате. 🎧 — послушать самому. Правый клик — настроить. Плитки можно перетаскивать.'));
}

// setNote — подсказка «включён набор» для страниц меню и колеса (null — набор не включён).
export function setNote(what) {
  const cur = curSet();
  if (!cur) return null;
  return h('div', {class: 'hint box accent row nowrap', style: 'margin-bottom:12px'},
    h('span', {class: 'grow'}, (cur.emoji || '🗂️') + ' Включён набор «' + cur.name + '» — ' + what),
    h('button', {class: 'btn sm', onclick: () => act('/api/sets/active', {id: ''}, {ok: '🎛️ Все звуки'})}, '🎛️ Все звуки'));
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
      title: st ? (g.length ? 'Игры: ' + g.join(', ') + (c.auto_set ? ' — включается сам, когда игра на экране' : '') + '\n' : '') + 'Правый клик — изменить набор' : 'Все звуки панели',
      onclick: () => { if (!on) act('/api/sets/active', {id}); },
      oncontextmenu: st ? e => { e.preventDefault(); setDialog(st); } : null},
    h('span', {class: 'pe'}, st ? st.emoji || '🗂️' : '🎛️'), h('span', {}, st ? st.name : 'Все звуки'), h('span', {class: 'auto'}, n + (st && g.length && c.auto_set ? ' · авто' : '')));
  };
  const hk = (app.state.hotkeys || {})['set:next'];
  return h('div', {class: 'sets-bar'},
    h('div', {class: 'opts grow'},
      chip(null, all.length),
      list.map(st => chip(st, (st.sounds || []).filter(id => have.has(id)).length)),
      h('button', {type: 'button', class: 'opt add', title: 'Новый набор звуков', onclick: () => setDialog()}, h('span', {class: 'pe'}, '＋'), h('span', {}, 'Набор'))),
    h('div', {class: 'row nowrap'},
      toggle(c.auto_set, v => set({auto_set: v}, {ok: v ? '🎮 Набор будет включаться сам, когда его игра на экране' : 'Наборы включаются только вручную'}),
        {label: 'Сам по игре', title: 'Включать набор, когда его игра на экране'}),
      kbd(hk, {empty: '⌨ клавиша', title: 'Клавиша «следующий набор» — переключать наборы прямо в игре', onclick: () => bindKey('set:next')}),
      cur ? h('button', {class: 'btn sm', title: 'Название, игры и звуки набора', onclick: () => setDialog(cur)}, '✏️ Изменить') : null));
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
  const d = dialog({title: orig ? 'Набор «' + orig.name + '»' : 'Новый набор звуков', wide: true, body: () => {
    const recent = (app.state.recent_apps || []).filter(x => !knownExe.has(x));
    const others = [...new Set(st.games.filter(x => !knownExe.has(x)).concat(recent))];
    const count = h('span', {class: 'flabel'});
    const paintCount = () => { count.textContent = 'Звуки набора · ' + st.sounds.filter(id => all.some(s => s.id === id)).length + ' из ' + all.length; };
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
      orig ? null : h('p', {class: 'hint', style: 'margin:0 0 14px'}, 'Набор — свои плитки для игры. Включите его, и панель, быстрое меню, колесо и пульт покажут только эти звуки. Набор может включаться сам, когда игра на экране.'),
      h('div', {class: 'row nowrap', style: 'gap:10px;align-items:flex-end'},
        h('button', {class: 'btn set-emo', title: 'Выбрать значок', onclick: () => { showEmoji = !showEmoji; d.update(); }}, st.emoji || '🗂️'),
        h('label', {class: 'field grow'}, h('span', {}, 'Название'),
          h('input', {class: 'input', value: st.name, maxlength: 40, autofocus: !orig, placeholder: 'Например: Dota 2 или «Тихий вечер»', oninput: e => { st.name = e.target.value; }}))),
      showEmoji ? h('div', {class: 'mt-s'}, emojiPicker(st.emoji, x => { st.emoji = x; showEmoji = false; d.update(); })) : null,
      h('div', {class: 'flabel mt'}, 'Игры — когда одна из них на экране, набор включается сам'),
      h('div', {class: 'opts mt-s'},
        known.map(g => h('button', {type: 'button', class: 'opt' + (gameOn(g) ? ' on' : ''), title: g.exe.join(', '), onclick: () => toggleGame(g)},
          h('span', {class: 'pe'}, g.emoji), h('span', {}, g.name))),
        others.map(x => h('button', {type: 'button', class: 'opt' + (has(x) ? ' on' : ''), title: has(x) ? 'Нажмите, чтобы убрать' : 'Недавно была на экране',
          onclick: () => { st.games = has(x) ? st.games.filter(y => y !== x) : st.games.concat(x); d.update(); }},
        h('span', {class: 'pe'}, '🖥️'), h('span', {}, x)))),
      h('div', {class: 'row mt-s'},
        h('input', {class: 'input', style: 'max-width:240px', placeholder: 'Другая игра: game.exe', value: custom,
          oninput: e => { custom = e.target.value; }, onkeydown: e => { if (e.key === 'Enter') { e.preventDefault(); addCustom(); } }}),
        h('button', {class: 'btn sm', onclick: addCustom}, 'Добавить'),
        h('span', {class: 'hint'}, 'Запустите игру — она появится в списке сама.')),
      st.games.length && !auto ? h('div', {class: 'row mt-s'}, toggle(false, v => set({auto_set: v}, {render: false}), {label: 'Включать наборы сами, когда игра на экране (сейчас выключено)'})) : null,
      h('div', {class: 'row mt', style: 'gap:6px'}, count, h('div', {class: 'grow'}),
        h('input', {class: 'input sm-input', type: 'search', placeholder: 'Найти…', value: q, oninput: e => { q = e.target.value; filterRows(); }}),
        h('button', {class: 'btn sm ghost', onclick: () => { st.sounds = st.sounds.concat(all.map(s => s.id).filter(id => !st.sounds.includes(id))); d.update(); }}, 'Все'),
        h('button', {class: 'btn sm ghost', onclick: () => { st.sounds = []; d.update(); }}, 'Ни одного')),
      all.length ? h('div', {class: 'spicks mt-s'}, rows) : h('p', {class: 'hint mt-s'}, 'Звуков пока нет — добавьте их на панель, и они появятся здесь.'),
    ];
  }, foot: () => [
    orig ? h('button', {class: 'btn danger', onclick: async () => {
      if (!await confirmDlg('Звуки останутся на панели — удалится только набор.', {title: 'Удалить набор «' + orig.name + '»?', ok: 'Удалить', danger: true})) return;
      if (await act('/api/sets/delete', {id: orig.id}, {ok: 'Набор удалён'})) d.close();
    }}, '🗑 Удалить') : null,
    h('div', {class: 'grow'}),
    h('button', {class: 'btn', onclick: () => d.close()}, 'Отмена'),
    h('button', {class: 'btn primary', onclick: save}, orig ? 'Сохранить' : 'Создать набор')]});
  if (opts.sounds) setTimeout(() => { const l = d.el.querySelector('.spicks'); if (l) l.scrollIntoView({block: 'nearest'}); });
  async function save() {
    const name = st.name.trim();
    if (!name) { toast('Назовите набор', 'err'); const i = d.el.querySelector('.dlg-b input.input'); if (i) i.focus(); return; }
    const r = await act('/api/sets/save', {set: {...st, name, emoji: st.emoji || '🗂️'}}, {render: !!orig, ok: orig ? 'Сохранено' : null});
    if (!r) return;
    if (!orig) await act('/api/sets/active', {id: r.id}, {ok: '✅ Набор «' + name + '» создан и включён'});
    d.close();
  }
}

function meta(s) {
  if (s.error) return [h('span', {}, '⚠ ' + s.error)];
  if (s.pending) return [h('span', {}, 'Готовлю голос…')];
  const out = [];
  if (s.duration) out.push(h('span', {}, fmtDur(s.duration)));
  if (s.kind === 'phrase' || s.text) out.push(h('span', {title: 'Фраза: ' + (s.text || '')}, '🗣️'));
  if (s.bus === 'monitor') out.push(h('span', {title: 'Слышите только вы'}, '🎧 только вам'));
  if (s.bus === 'voice') out.push(h('span', {title: 'Только в эфир, вы не слышите'}, '📡 в эфир'));
  if (s.in_menu) out.push(h('span', {title: 'Есть в быстром меню'}, '⚡'));
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
    h('button', {class: 't-act', title: 'Послушать самому (только в наушники)', onclick: e => { e.stopPropagation(); playSound(s.id, 'monitor'); }}, '🎧'),
    h('button', {class: 't-act', title: 'Настроить', onclick: e => { e.stopPropagation(); editSound(s.id); }}, '✏️')),
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
const MODES = [['toggle', 'Остановить звук'], ['interrupt', 'Запустить заново'], ['overlap', 'Наложить ещё один'], ['ignore', 'Ничего (дождаться конца)']];
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
  const d = dialog({title: 'Настройка звука', wide: true, body: () => {
    const prev = h('div', {class: 'tile', style: {'--c': e.color, width: '170px', cursor: 'default', flex: 'none'}},
      h('div', {class: 't-top'}, h('div', {class: 'emo'}, e.emoji), hotkey ? h('span', {class: 'hk'}, kbd(hotkey)) : null),
      h('div', {class: 't-title'}, e.label || 'Без названия'), h('div', {class: 't-meta'}, fmtDur(s.duration)));
    return [
      h('div', {class: 'row nowrap', style: 'align-items:flex-start;gap:18px'}, prev,
        h('div', {class: 'col grow', style: 'gap:12px'},
          h('label', {class: 'field'}, h('span', {}, 'Название'), h('input', {class: 'input', value: e.label, maxlength: 40, oninput: ev => { e.label = ev.target.value; prev.querySelector('.t-title').textContent = e.label || 'Без названия'; }})),
          h('div', {class: 'row'}, h('button', {class: 'btn sm', onclick: () => { showEmoji = !showEmoji; d.update(); }}, e.emoji + ' Значок'), colorPicker(e.color, c => { e.color = c; prev.style.setProperty('--c', c); })))),
      showEmoji ? h('div', {class: 'mt-s'}, emojiPicker(e.emoji, x => { e.emoji = x; showEmoji = false; d.update(); })) : null,
      s.text != null && s.text !== '' || s.kind === 'phrase' ? h('label', {class: 'field mt'}, h('span', {}, 'Что сказать (голос Windows)'),
        h('textarea', {class: 'input', rows: 2, value: text, oninput: ev => { text = ev.target.value; }})) : null,
      h('div', {class: 'form mt'},
        h('div', {class: 'col'}, h('span', {class: 'flabel'}, 'Кто слышит'),
          seg([['both', 'Все'], ['voice', 'Только эфир'], ['monitor', 'Только я']], e.bus, v => { e.bus = v; })),
        h('div', {class: 'col'}, h('span', {class: 'flabel'}, 'Клавиша'),
          h('div', {class: 'row'}, kbd(hotkey, {empty: 'не задана'}), h('button', {class: 'btn sm', onclick: async () => {
            const k = await captureKey({title: 'Клавиша для «' + (e.label || s.title) + '»', current: hotkey, action: 'sound:' + s.id});
            if (k !== null) { hotkey = k; d.update(); }
          }}, hotkey ? 'Изменить' : 'Назначить'))),
        slider({label: 'Громкость', value: e.volume, min: 0, max: 2, step: 0.05, fmt: v => Math.round(v * 100) + '%', onInput: v => { e.volume = v; }}),
        slider({label: 'Не чаще раза в', value: e.cooldown, min: 0, max: 30, step: 0.5, fmt: v => v ? String(v).replace('.', ',') + ' с' : 'без ограничений', onInput: v => { e.cooldown = v; }}),
        h('label', {class: 'field'}, h('span', {}, 'Повторное нажатие, пока звук играет'), select(MODES, e.mode, v => { e.mode = v; })),
        h('label', {class: 'field'}, h('span', {}, 'Группа (фильтр на панели)'),
          h('input', {class: 'input', value: e.category, list: 'vb-cats', placeholder: 'без группы', oninput: ev => { e.category = ev.target.value; }}),
          h('datalist', {id: 'vb-cats'}, cats.map(c => h('option', {value: c})))),
        h('div', {class: 'full'}, toggle(inMenu, v => { inMenu = v; }, {label: 'Показывать в быстром меню поверх игры'})),
        sets.length ? h('div', {class: 'full'}, h('span', {class: 'flabel'}, 'В каких наборах'),
          h('div', {class: 'opts mt-s'}, sets.map(x => h('button', {type: 'button', class: 'opt' + (inSets.includes(x.id) ? ' on' : ''), onclick: ev => {
            inSets = inSets.includes(x.id) ? inSets.filter(y => y !== x.id) : inSets.concat(x.id);
            ev.currentTarget.classList.toggle('on', inSets.includes(x.id));
          }}, h('span', {class: 'pe'}, x.emoji || '🗂️'), h('span', {}, x.name))))) : null),
      h('p', {class: 'hint mt', style: 'word-break:break-all'}, 'Источник: ', (s.files || []).map(f => f.replace(/^builtin:/, 'встроенный: ').replace(/^tts:/, 'фраза: ')).join(' + ') || s.id),
    ];
  }, foot: () => [
    h('button', {class: 'btn danger', onclick: async () => {
      if (!await confirmDlg('Плитка «' + s.title + '» будет удалена' + (s.kind === 'file' ? ', файл переместится в папку trash.' : '.'), {title: 'Удалить звук?', ok: 'Удалить', danger: true})) return;
      if (await act('/api/sounds/delete', {id: s.id}, {ok: 'Звук удалён'})) d.close();
    }}, '🗑 Удалить'),
    h('div', {class: 'grow'}),
    h('button', {class: 'btn', title: 'Только в наушники', onclick: () => playSound(s.id, 'monitor')}, '🎧 Послушать'),
    h('button', {class: 'btn', onclick: () => d.close()}, 'Отмена'),
    h('button', {class: 'btn primary', onclick: async () => {
      const edit = {...e, label: e.label.trim()};
      if (s.text != null && s.text !== '' || s.kind === 'phrase') { if (!text.trim()) { toast('Фраза не может быть пустой', 'err'); return; } if (text !== s.text) edit.text = text; }
      if (hotkey !== (s.hotkey || '')) edit.hotkey = hotkey;
      if (inMenu !== !!s.in_menu) edit.in_menu = inMenu;
      if (setsKey(inSets) !== sets0) edit.sets = inSets;
      if (await act('/api/sounds/update', {id: s.id, edit}, {ok: 'Сохранено'})) d.close();
    }}, 'Сохранить')]});
}

// ---------- добавление ----------
export function addDialog(tab = 'file', opts = {}) {
  let files = [];   // {name, state: '…'|'ok'|'err', msg}
  let phrase = '', pEmoji = opts.dota ? '📣' : '🗣️';
  const builtins = app.state.builtins || [];
  const d = dialog({title: opts.dota ? 'Новая кнопка-оповещение' : 'Добавить звук', wide: true, body: () => {
    const tabs = opts.dota ? null : h('div', {class: 'tabs'}, [['file', '📁 Файл'], ['phrase', '🗣️ Фраза'], ['builtin', '✨ Встроенные']].map(([v, l]) =>
      h('button', {class: tab === v ? 'on' : '', onclick: () => { tab = v; d.update(); }}, l)));
    if (tab === 'file') {
      const inp = h('input', {type: 'file', multiple: true, accept: 'audio/*,video/*,.mp3,.wav,.ogg,.flac,.m4a,.opus,.webm', class: 'hidden',
        onchange: ev => upload([...ev.target.files])});
      const zone = h('div', {class: 'drop', onclick: () => inp.click(),
        ondragover: ev => { ev.preventDefault(); zone.classList.add('over'); }, ondragleave: () => zone.classList.remove('over'),
        ondrop: ev => { ev.preventDefault(); ev.stopPropagation(); zone.classList.remove('over'); document.body.classList.remove('dropzone-over'); upload([...ev.dataTransfer.files]); }},
      h('div', {class: 'big'}, '🎵'), h('b', {}, 'Перетащите файлы сюда'), h('div', {class: 'hint mt-s'}, 'или нажмите, чтобы выбрать · MP3, WAV, OGG, FLAC, M4A — можно сразу несколько'), inp);
      return [tabs, zone, files.length ? h('div', {class: 'files'}, files.map(f => h('div', {class: 'f'},
        h('span', {}, f.state === 'ok' ? '✅' : f.state === 'err' ? '❌' : '⏳'), h('span', {class: 'ell grow'}, f.name), h('small', {class: f.state === 'err' ? 'bad' : 'faint'}, f.msg || '')))) : null];
    }
    if (tab === 'phrase') {
      return [tabs,
        h('label', {class: 'field'}, h('span', {}, opts.dota ? 'Что сказать команде' : 'Что сказать'),
          h('textarea', {class: 'input', rows: 2, autofocus: true, maxlength: 300, placeholder: opts.dota ? 'Например: Осторожно, у них Рошан!' : 'Например: Всем привет, я на связи!', value: phrase, oninput: ev => { phrase = ev.target.value; }})),
        h('div', {class: 'mt'}, h('span', {class: 'flabel'}, 'Значок'), h('div', {class: 'mt-s'}, emojiPicker(pEmoji, x => { pEmoji = x; }))),
        h('p', {class: 'hint mt'}, '🗣️ Фразу озвучит голос Windows (выбрать голос и скорость — в «Настройках»). ' + (opts.dota ? 'Перед фразой прозвучит короткий сигнал.' : 'Озвучка готовится один раз и потом играет мгновенно.'))];
    }
    const list = builtins.filter(b => b.group !== 'dota' || app.state.config.dota.enabled);
    const groups = [...new Set(list.map(b => b.group || 'Звуки'))];
    const have = new Set(app.state.sounds.filter(s => s.kind === 'builtin').flatMap(s => s.files).map(f => f.replace('builtin:', '')));
    return [tabs, h('p', {class: 'hint', style: 'margin:0 0 12px'}, 'Короткие звуки, созданные прямо в программе. ▶ — послушать самому, + — добавить на панель.'),
      groups.map(g => [h('div', {class: 'flabel', style: 'margin:10px 0 6px'}, BGROUP[g] || g),
        h('div', {class: 'builtins', style: 'max-height:none'}, list.filter(b => (b.group || 'Звуки') === g).map(b => h('div', {class: 'bi'},
          h('span', {class: 'be'}, b.emoji || '🔊'), h('div', {class: 'bt'}, h('div', {class: 'ell', title: b.label || b.id}, b.label || b.id), h('small', {}, fmtDur(b.duration))),
          h('button', {class: 'btn sm icon ghost', title: 'Послушать самому', onclick: () => playSound(b.id, 'monitor')}, '▶'),
          have.has(b.id) ? h('span', {class: 'btn sm icon ghost ok', title: 'Уже на панели', style: 'cursor:default'}, '✓')
            : h('button', {class: 'btn sm icon', title: 'Добавить на панель', onclick: async () => { if (await act('/api/sounds/builtin', {name: b.id}, {ok: '✅ ' + (b.label || b.id) + ' — на панели'})) d.update(); }}, '+'))))])];
  }, foot: () => {
    if (tab === 'phrase') return [h('div', {class: 'grow'}),
      h('button', {class: 'btn', onclick: () => phrase.trim() ? act('/api/tts/say', {text: phrase, to: 'monitor'}) : toast('Напишите фразу', 'err')}, '🎧 Послушать'),
      h('button', {class: 'btn primary', onclick: async () => {
        if (!phrase.trim()) { toast('Напишите фразу', 'err'); return; }
        if (await act('/api/sounds/phrase', {text: phrase, emoji: pEmoji, dota: !!opts.dota}, {ok: '✅ Фраза добавлена'})) d.close();
      }}, 'Добавить')];
    return [h('div', {class: 'grow'}), h('button', {class: 'btn', onclick: () => d.close()}, 'Готово')];
  }});
  if (opts.dota) tab = 'phrase', d.update();
  async function upload(list) {
    if (!list.length) return;
    const items = list.map(f => ({name: f.name, state: '…', msg: 'читаю…'}));
    files = files.concat(items); d.update();
    await importFiles(list, (i, state, msg) => { items[i].state = state; items[i].msg = msg; if (!d.closed) d.update(); });
  }
}

// importFiles — добавить файлы: WAV и MP3 — как есть, остальное (и нечитаемые) — перекодировать в WAV прямо в окне.
export async function importFiles(list, progress = () => {}) {
  let added = 0, failed = 0;
  for (let i = 0; i < list.length; i++) {
    const f = list[i];
    const name = f.name.replace(/\.[^.]+$/, '');
    try {
      if (f.size > 48 << 20) throw new Error('файл больше 48 МБ');
      let blob = f, fname = f.name;
      const ext = (f.name.match(/\.[^.]+$/) || [''])[0].toLowerCase();
      let ok = false;
      if (ext === '.wav' || ext === '.mp3') {
        const v = await api('/api/sounds/validate?ext=' + ext, f).catch(() => ({ok: false}));
        ok = v.ok;
      }
      if (!ok) { progress(i, '…', 'перекодирую…'); blob = await toWAV(f); fname = name + '.wav'; }
      progress(i, '…', 'добавляю…');
      const fd = new FormData();
      fd.append('files', blob, fname);
      const r = await api('/api/sounds/import', fd);
      if (r.state) setState(r.state);
      added++;
      progress(i, 'ok', 'добавлен');
    } catch (e) {
      failed++;
      progress(i, 'err', e.message);
    }
  }
  if (added) toast('✅ Добавлено звуков: ' + added, 'ok');
  if (failed && !added) toast('Не удалось добавить: ' + failed, 'err');
  return added;
}
async function toWAV(file) {
  const buf = await file.arrayBuffer();
  const ctx = new (window.OfflineAudioContext || window.webkitOfflineAudioContext)(1, 1, 48000);
  let audio;
  try { audio = await ctx.decodeAudioData(buf); } catch (e) { throw new Error('формат не поддерживается'); }
  const ch = Math.min(2, audio.numberOfChannels), sr = audio.sampleRate, n = audio.length;
  if (n / sr > 600) throw new Error('длиннее 10 минут');
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
