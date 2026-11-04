// Модуль Dota 2: включается одной кнопкой. Кнопки-оповещения, голосовые тайминги, Рошан, игровое время.
import {_t, _tf, EN} from '../i18n.js';
import {app, h, api, act, toast, onLive, kbd, bindKey, fmtTime, parseTime, slider, seg, toggle, setrow, sectionH, dialog,
  confirmDlg, promptDlg, select, playSound, clone, plural, hint, requestRender} from '../core.js';
import {checkRows, DOTA_CHECKS, autoSetup, loadSetup} from './check.js';
import {addDialog, editSound, importFiles} from './sounds.js';

const dotaCfg = () => clone(app.state.config.dota);
const saveDota = (d, opt) => act('/api/dota', {dota: d}, opt);

export default function dotaPage() {
  const d = app.state.config.dota;
  return d.enabled ? onView(d) : offView(d);
}

function offView(d) {
  return h('div', {class: 'stack'},
    h('div', {class: 'hero-card'},
      h('div', {class: 'hc-e'}, '🐲'),
      h('div', {class: 'grow'}, h('h2', {}, _t('Модуль Dota 2')),
        h('p', {}, _t('Помощник для игры с командой: кнопки «Ганк на миде!», голосовые напоминания о рунах и Рошане по игровому времени, автонастройка Dota 2. Пока модуль выключен, его кнопки и клавиши не мешают.'))),
      h('button', {class: 'btn primary lg', onclick: () => enable(d)}, _t('Включить модуль'))),
    h('div', {class: 'feat'},
      [[_t('🚨 Кнопки-оповещения'), _t('Ганк на миде, мид пропал, смоук, все на деф — одной клавишей, голосом для всей команды.')],
        [_t('⏰ Голосовые тайминги'), _t('Руна мудрости каждые 7 минут, руны силы, лотосы, стаки — предупредит заранее.')],
        [_t('🐲 Рошан'), _t('Напомнит, когда пропадёт аегис и когда Рошан может появиться снова.')],
        [_t('🛠️ Автонастройка'), _t('Подключение к игре (GSI), микрофон и кнопка голосового чата Dota 2 — сами.')]].map(([t, s]) => h('div', {}, h('b', {}, t), h('small', {}, s)))));
}
async function enable(d) {
  d = clone(d); d.enabled = true;
  if (await saveDota(d, {ok: _t('🐲 Модуль Dota 2 включён')})) { app.setup = null; loadSetup(); }
}

function onView(d) {
  const st = app.status;
  const callouts = (app.state.sounds || []).filter(s => s.dota && !s.hidden);
  const setupBad = app.setup && app.setup.checks.some(c => DOTA_CHECKS.includes(c.id) && c.id !== 'gsi_live' && !c.ok);
  const game = h('div', {});
  const paintGame = s => game.replaceChildren(...gameBox(s));
  paintGame(st);
  onLive('status', paintGame);
  const tiles = new Map();
  onLive('levels', lv => {
    const now = new Set((lv.playing || []).map(p => p.id));
    for (const [id, el] of tiles) el.classList.toggle('playing', now.has(id));
  });
  return h('div', {class: 'stack'},
    h('div', {class: 'hero-card', style: 'padding:14px 20px'},
      h('div', {class: 'hc-e', style: 'font-size:32px'}, '🐲'),
      h('div', {class: 'grow'}, h('h2', {style: 'font-size:17px'}, _t('Модуль Dota 2 включён')),
        h('p', {style: 'margin:2px 0 0'}, st.game && st.game.connected ? _t('Dota 2 на связи — тайминги идут по игровому времени.') : _t('Ждём Dota 2: время появится, когда начнётся матч.'))),
      toggle(true, async () => { const x = dotaCfg(); x.enabled = false; if (await saveDota(x, {ok: _t('Модуль Dota 2 выключен')})) loadSetup(); }, {big: true, title: _t('Выключить модуль')})),
    setupBad ? hint([_t('⚠️ Dota 2 настроена не полностью — без этого не будет игрового времени и микрофона в игре. '), h('button', {class: 'btn sm primary', style: 'margin-left:6px', onclick: autoSetup}, _t('✨ Настроить автоматически'))], 'warn') : null,
    h('div', {class: 'grid2'},
      h('div', {class: 'card'}, h('h3', {}, _t('⏱️ Игра')), game,
        h('div', {class: 'row mt', style: 'padding-top:12px;border-top:1px solid var(--line)'},
          h('button', {class: 'btn', title: _t('Без связи с игрой: нажмите в момент горна — тайминги пойдут от 0:00'), onclick: () => act('/api/clock', {mode: 'horn'}, {ok: _t('Отсчёт с 0:00')})}, _t('📯 Горн 0:00')),
          h('button', {class: 'btn', onclick: async () => { const t = await promptDlg(_t('Сколько сейчас на игровых часах?'), {placeholder: '12:34', ok: _t('Синхронизировать')}); if (t) act('/api/clock', {mode: 'sync', time: t}, {ok: _t('Время: ') + t}); }}, _t('🕐 Указать время')),
          h('button', {class: 'btn', onclick: () => act('/api/action', {action: 'rosh'}, {ok: _t('🐲 Таймер Рошана запущен')})}, _t('🐲 Рошан убит')),
          kbd((app.state.hotkeys || {}).rosh, {onclick: () => bindKey('rosh'), empty: _t('клавиша')}))),
      h('div', {class: 'card'}, h('div', {class: 'card-h'}, h('h3', {}, _t('🔗 Подключение к Dota 2')), h('div', {class: 'grow'}),
        h('button', {class: 'btn sm ghost', onclick: () => { app.setup = null; loadSetup(); }}, '🔄')),
        checkRows(c => DOTA_CHECKS.includes(c.id)),
        h('p', {class: 'hint', style: 'margin:10px 0 0'}, _t('💡 Хотите, чтобы голос шёл только по кнопке? «Голос → Когда вас слышно → Пока держу».')))),
    sectionH(_t('🚨 Кнопки-оповещения'), _t('слышит вся команда · клавиши работают прямо в игре'),
      h('button', {class: 'btn sm', onclick: () => addDialog('phrase', {dota: true})}, _t('＋ Своя кнопка'))),
    callouts.length ? h('div', {class: 'callouts'}, callouts.map(s => {
      const el = h('div', {class: 'callout', style: {'--c': s.color || '#ef4444'}, title: _t('Нажмите — услышат все. Правый клик — настроить.'),
        onclick: () => playSound(s.id), oncontextmenu: e => { e.preventDefault(); editSound(s.id); }},
      h('span', {class: 'ce'}, s.emoji || '📣'),
      h('div', {class: 'ct'}, h('b', {class: 'ell', style: 'display:block'}, s.title), kbd(s.hotkey, {onclick: () => bindKey('sound:' + s.id), empty: _t('+ клавиша')})),
      h('button', {class: 'btn sm icon ghost', title: _t('Настроить'), onclick: e => { e.stopPropagation(); editSound(s.id); }}, '✏️'));
      tiles.set(s.id, el);
      return el;
    })) : h('p', {class: 'hint'}, _t('Кнопок пока нет — добавьте свою.')),
    eventsCard(),
    remindersCard(d));
}

// ---------- звуки на события матча ----------
const EV_ICON = {victory: '🏆', defeat: '💀', game_start: '📯', pregame: '⏳', kill: '🗡️', death: '☠️', respawn: '🔄', streak_3: '🔥', streak_5: '🔥',
  streak_10: '🔥', low_hp: '🩸', level_6: '⭐', smoked: '💨', day: '☀️', night: '🌙', rosh_killed: '🐲'};
const EV_SUB = {victory: _t('Ваша команда выиграла — сразу, как появится экран победы'), defeat: _t('Ваша команда проиграла')};
const MAIN_EV = ['victory', 'defeat'];
let moreEv = false;
function eventsCard() {
  const evs = app.state.events || [], cur = app.state.config.events || {};
  const all = app.state.sounds || [], byId = new Map(all.map(s => [s.id, s]));
  const mine = all.filter(s => !s.hidden && s.kind !== 'builtin').sort((a, b) => a.title.localeCompare(b.title, 'ru'));
  const built = app.state.builtins || [];
  const row = ev => {
    const [id, bus0] = (cur[ev.id] || '').split('@');
    const snd = id ? byId.get(id) : null;
    const bus = (bus0 || (snd && snd.bus) || 'both') === 'monitor' ? 'monitor' : 'both';
    const to = id ? bus : MAIN_EV.includes(ev.id) ? 'both' : 'monitor'; // победу и поражение по умолчанию слышат все
    const save = (sound, t, ok) => act('/api/dota/events', {event: ev.id, sound, to: sound ? t : ''}, {ok});
    const opts = [['', _t('— без звука —')],
      {group: _t('Ваши звуки'), items: mine.map(s => [s.id, (s.emoji || '🔊') + ' ' + s.title])},
      {group: _t('Встроенные'), items: built.map(b => [b.id, (b.emoji || '🔊') + ' ' + (b.label || b.id)])}];
    if (id && !mine.some(s => s.id === id) && !built.some(b => b.id === id)) opts.splice(1, 0, [id, snd ? (snd.emoji || '🔊') + ' ' + snd.title : '⚠️ ' + id + _t(' (нет такого звука)')]);
    const file = h('input', {type: 'file', class: 'hidden', accept: 'audio/*,video/*,.mp3,.wav,.ogg,.flac,.m4a,.opus,.webm', onchange: async e => {
      const f = e.target.files[0];
      if (!f) return;
      const ids = [];
      if (await importFiles([f], undefined, ids) && ids[0]) save(ids[0], to, '✅ ' + ev.label + ': «' + f.name.replace(/\.[^.]+$/, '') + '»');
    }});
    return setrow((EV_ICON[ev.id] || '🔔') + ' ' + ev.label, EV_SUB[ev.id] || '',
      h('div', {class: 'row nowrap ev-ctl'},
        select(opts, id || '', v => save(v, to, v ? '✅ ' + ev.label + _t(': звук выбран') : ev.label + _t(': без звука')), {title: _t('Какой звук играет')}),
        id ? seg([['both', _t('📡 Все')], ['monitor', _t('🎧 Только я')]], bus, v => save(id, v), {cls: 'sm'}) : null,
        id ? h('button', {class: 'btn sm icon ghost', title: _t('Послушать самому'), onclick: () => playSound(id, 'monitor')}, '▶') : null,
        h('button', {class: 'btn sm icon ghost', title: _t('Загрузить свой файл (MP3, WAV, OGG…) и поставить сюда'), onclick: () => file.click()}, '📁'), file));
  };
  const rest = evs.filter(e => !MAIN_EV.includes(e.id)), restOn = rest.filter(e => cur[e.id]).length;
  return h('div', {},
    sectionH(_t('🏆 Звуки на события'), _t('играют сами по данным из игры')),
    h('div', {class: 'card'},
      evs.filter(e => MAIN_EV.includes(e.id)).map(row),
      h('div', {style: 'padding-top:8px'}, h('button', {class: 'btn sm ghost', onclick: () => { moreEv = !moreEv; requestRender(); }},
        (moreEv ? _t('− Скрыть другие события') : _t('＋ Другие события: убийство, смерть, Рошан, ночь…')) + (restOn ? _t(' · настроено: ') + restOn : ''))),
      moreEv ? rest.map(row) : null,
      h('p', {class: 'hint', style: 'margin:10px 0 0'}, _t('💡 Свой гимн — кнопка 📁 или любой звук с панели. Остановить — «⏹ Стоп» на панели звуков или клавиша «Остановить все звуки».'))));
}

function gameBox(st) {
  const g = (st && st.game) || {}, ci = g.clock_info || {};
  const src = {gsi: ['ok', _t('● данные из игры')], estimate: ['warn', _t('≈ по последним данным')], manual: ['warn', _t('ручное время')], idle: ['', _t('матч не идёт')]}[ci.source];
  if (!g.connected && ci.source !== 'manual' && !g.has_clock) {
    return [h('div', {class: 'clock faint'}, '--:--'), h('p', {class: 'hint', style: 'margin:6px 0 0'}, _t('Запустите Dota 2 — время появится само. Без связи с игрой нажмите «Горн» в момент начала матча.'))];
  }
  const up = (st.upcoming || []).slice(0, 5);
  return [
    h('div', {class: 'row', style: 'align-items:baseline;gap:12px'}, h('div', {class: 'clock' + (g.has_clock ? '' : ' faint')}, g.has_clock ? fmtTime(g.clock) : '–:––', g.paused ? ' ⏸' : ''),
      src ? h('span', {class: 'badge ' + (src[0] || 'dim')}, src[1]) : null,
      ci.source === 'manual' ? h('button', {class: 'btn sm ghost', onclick: () => act('/api/clock', {mode: 'clear'}, {ok: _t('Ручное время сброшено')})}, _t('Сбросить')) : null),
    h('div', {class: 'dim', style: 'font-size:13px'}, [g.hero ? _t('Герой: ') + g.hero : '', g.has_clock ? (g.daytime ? _t('☀️ день') : _t('🌙 ночь')) : ''].filter(Boolean).join(' · ')),
    up.length ? h('div', {class: 'up'}, up.map(u => [h('span', {}, (u.label || u.id) + ' ', h('span', {class: 'faint'}, _t('в ') + fmtTime(u.event_at))), h('b', {}, fmtTime(u.in)),
      h('div', {class: 'bar'}, h('i', {style: `width:${Math.max(2, Math.min(100, 100 - u.in / 1.2))}%`}))])) :
      h('p', {class: 'hint', style: 'margin:8px 0 0'}, _t('Ближайшие напоминания появятся здесь.'))];
}

// ---------- напоминания ----------
const every = n => !n ? '' : n % 60 === 0 ? (n === 60 ? _t('каждую минуту') : _t('каждые ') + n / 60 + ' ' + plural(n / 60, _t('минуту'), _t('минуты'), _t('минут'))) : _t('каждые ') + fmtTime(n);
function schedule(r) {
  let s;
  if (r.at && r.at.length) s = _t('в ') + r.at.map(fmtTime).join(', ');
  else { s = every(r.every); if (r.start && r.start !== r.every) s += _t(', с ') + fmtTime(r.start); if (r.until) s += _t(', до ') + fmtTime(r.until); }
  if (r.before) s += _tf(' · за {0} с', r.before);
  return s;
}
function remindersCard(d) {
  const list = app.state.reminders || [];
  const setRem = (fn, opt) => { const x = dotaCfg(); fn(x); return saveDota(x, opt); };
  return h('div', {},
    sectionH(_t('⏰ Голосовые напоминания'), _t('по игровому времени'), h('button', {class: 'btn sm', onclick: () => editReminder(null)}, _t('＋ Напоминание'))),
    h('div', {class: 'card'},
      list.map((r, i) => h('div', {class: 'rem' + (r.enabled ? '' : ' off')},
        toggle(r.enabled, v => setRem(x => { x.reminders[i].enabled = v; })),
        h('div', {class: 'rt', style: 'cursor:pointer', onclick: () => editReminder(i)}, h('b', {}, r.label || r.id), h('small', {}, schedule(r)),
          h('small', {}, r.phrase ? h('q', {}, r.phrase) : h('span', {class: 'faint'}, _t('только сигнал')), r.to === 'both' || (!r.to && d.voice_to === 'both') ? h('span', {class: 'faint'}, _t(' · слышит команда')) : null)),
        h('div', {class: 'row nowrap'},
          h('button', {class: 'btn sm icon ghost', title: _t('Прослушать'), onclick: () => act('/api/dota/test', {reminder: r})}, '▶'),
          h('button', {class: 'btn sm icon ghost', title: _t('Изменить'), onclick: () => editReminder(i)}, '✏️')))),
      !list.length ? h('p', {class: 'hint'}, _t('Напоминаний нет.')) : null,
      h('div', {style: 'border-top:1px solid var(--line);margin-top:6px;padding-top:4px'},
        setrow(_t('Кто слышит напоминания'), d.voice_to === 'both' ? _t('Вы и вся команда в голосовом чате') : _t('Только вы, в наушниках'),
          seg([['monitor', _t('🎧 Только я')], ['both', _t('📡 Я и команда')]], d.voice_to === 'both' ? 'both' : 'monitor', v => setRem(x => { x.voice_to = v; }))),
        setrow(_t('Громкость напоминаний'), '', h('div', {style: 'width:220px'}, slider({label: '', value: d.volume || 1, min: 0.2, max: 2, step: 0.05, fmt: v => Math.round(v * 100) + '%',
          onChange: v => setRem(x => { x.volume = v; }, {render: false})}))),
        setrow(_t('Рошан'), _t('После «Рошан убит»: когда пропадёт аегис и когда Рошан может появиться снова'), toggle(d.rosh, v => setRem(x => { x.rosh = v; }))))));
}

// Язык фразы — как в программе (phraseEn): русские буквы в названии — по-русски, латиница — по-английски, иначе — язык интерфейса
const phraseEn = s => /[а-яё]/i.test(s) ? false : /[a-z]/i.test(s) ? true : EN;
// inTime — «через 40 секунд» / «in 40 seconds» (как inTime в программе)
function inTime(sec, en) {
  const m = Math.floor(sec / 60), s = sec % 60;
  if (en) {
    const u = (n, w) => n + ' ' + w + (n === 1 ? '' : 's');
    return 'in ' + (m ? u(m, 'minute') + (s ? ' ' + u(s, 'second') : '') : u(s, 'second'));
  }
  const ru = (n, one, few, many) => { const a = n % 100, b = n % 10; return n + ' ' + (a > 10 && a < 20 ? many : b > 1 && b < 5 ? few : b === 1 ? one : many); };
  return 'через ' + (m ? (m === 1 ? 'минуту' : ru(m, 'минуту', 'минуты', 'минут')) + (s ? ' ' + ru(s, 'секунду', 'секунды', 'секунд') : '') : ru(s, 'секунду', 'секунды', 'секунд'));
}

function editReminder(i) {
  const d = dotaCfg();
  const isNew = i == null;
  const r = isNew ? {id: 'r' + Date.now().toString(36), label: '', enabled: true, start: 0, every: 300, before: 15} : clone(d.reminders[i]);
  let mode = r.at && r.at.length ? 'at' : 'every';
  const f = {label: r.label || '', start: r.start ? fmtTime(r.start) : '', every: r.every ? fmtTime(r.every) : '5:00', until: r.until ? fmtTime(r.until) : '',
    at: (r.at || []).map(fmtTime).join(', '), before: r.before || 0, say: r.say === '-' ? '' : (r.say || ''), silent: r.say === '-', sound: r.sound || '', to: r.to || ''};
  const signals = [['', _t('🔔 Колокольчик')], ['none', _t('Без сигнала')],
    {group: _t('Встроенные'), items: (app.state.builtins || []).map(b => ['builtin:' + b.id, (b.emoji || '') + ' ' + (b.label || b.id)])},
    {group: _t('Ваши звуки'), items: (app.state.sounds || []).filter(s => !s.hidden && s.on_pad).map(s => [s.id, (s.emoji || '') + ' ' + s.title])}];
  const build = () => {
    const x = {...r, label: f.label.trim(), before: +f.before || 0, sound: f.sound, to: f.to};
    delete x.at; delete x.start; delete x.every; delete x.until; delete x.phrase;
    if (mode === 'at') {
      x.at = f.at.split(/[,;\s]+/).map(parseTime).filter(v => v != null && v >= 0);
      if (!x.at.length) throw new Error(_t('Укажите время, например 2:00, 4:00'));
    } else {
      x.every = parseTime(f.every); x.start = parseTime(f.start) ?? x.every; const u = parseTime(f.until);
      if (!x.every || x.every < 10) throw new Error(_t('Интервал — не меньше 10 секунд'));
      if (u) x.until = u;
    }
    x.say = f.silent ? '-' : f.say.trim();
    if (!x.say) delete x.say;
    if (!x.label) throw new Error(_t('Дайте напоминанию название'));
    return x;
  };
  const auto = () => { const l = f.label.trim() || _t('Событие'); return l + (f.before > 0 ? ' ' + inTime(+f.before, phraseEn(l)) : ''); };
  const dlg = dialog({title: isNew ? _t('Новое напоминание') : _t('Напоминание'), body: () => [
    h('label', {class: 'field'}, h('span', {}, _t('Название')), h('input', {class: 'input', value: f.label, placeholder: _t('Например: Руна мудрости'), autofocus: true, oninput: e => { f.label = e.target.value; ph(); }})),
    h('div', {class: 'mt'}, seg([['every', _t('🔁 Повторять')], ['at', _t('📍 В определённое время')]], mode, v => { mode = v; dlg.update(); })),
    mode === 'every' ? h('div', {class: 'form mt', style: 'grid-template-columns:repeat(3, minmax(0, 1fr))'},
      h('label', {class: 'field'}, h('span', {}, _t('Каждые (м:сс)')), h('input', {class: 'input', value: f.every, placeholder: '7:00', oninput: e => { f.every = e.target.value; }})),
      h('label', {class: 'field'}, h('span', {}, _t('Первый раз в')), h('input', {class: 'input', value: f.start, placeholder: _t('как интервал'), oninput: e => { f.start = e.target.value; }})),
      h('label', {class: 'field'}, h('span', {}, _t('До (пусто — всю игру)')), h('input', {class: 'input', value: f.until, placeholder: '—', oninput: e => { f.until = e.target.value; }}))) :
      h('label', {class: 'field mt'}, h('span', {}, _t('Игровое время, через запятую')), h('input', {class: 'input', value: f.at, placeholder: '2:00, 4:00', oninput: e => { f.at = e.target.value; }})),
    h('div', {class: 'mt'}, slider({label: _t('Предупредить заранее'), value: f.before, min: 0, max: 60, step: 1, fmt: v => v ? _tf('за {0} с', v) : _t('в момент события'), onInput: v => { f.before = v; ph(); }})),
    h('label', {class: 'field mt'}, h('span', {}, _t('Фраза')), h('input', {class: 'input', id: 'remSay', value: f.say, placeholder: auto(), disabled: f.silent, oninput: e => { f.say = e.target.value; }})),
    f.silent ? null : h('div', {class: 'row mt-s', style: 'gap:6px'}, h('span', {class: 'hint'}, _t('{время} в фразе прозвучит как «через N секунд» (N — «Предупредить заранее»)')),
      h('button', {class: 'btn sm ghost', title: _t('Вставить {время} в конец фразы'), onclick: () => { f.say = (f.say.trim() + _t(' {время}')).trim(); dlg.update(); }}, _t('＋ {время}'))),
    h('div', {class: 'mt-s'}, toggle(f.silent, v => { f.silent = v; dlg.update(); }, {label: _t('Только сигнал, без фразы')})),
    h('div', {class: 'form mt'},
      h('label', {class: 'field'}, h('span', {}, _t('Сигнал перед фразой')), select(signals, f.sound, v => { f.sound = v; })),
      h('label', {class: 'field'}, h('span', {}, _t('Кто слышит')), select([['', _t('Как у всех напоминаний')], ['monitor', _t('🎧 Только я')], ['both', _t('📡 Я и команда')]], f.to, v => { f.to = v; })))],
  foot: () => [
    !isNew ? h('button', {class: 'btn danger', onclick: async () => {
      if (!await confirmDlg(_tf('Напоминание «{0}» будет удалено.', (r.label || r.id)), {title: _t('Удалить напоминание?'), ok: _t('Удалить'), danger: true})) return;
      d.reminders.splice(i, 1);
      if (await saveDota(d, {ok: _t('Удалено')})) dlg.close();
    }}, _t('🗑️ Удалить')) : null,
    h('div', {class: 'grow'}),
    h('button', {class: 'btn', onclick: () => { try { act('/api/dota/test', {reminder: build()}); } catch (e) { toast(e.message, 'err'); } }}, _t('▶ Прослушать')),
    h('button', {class: 'btn', onclick: () => dlg.close()}, _t('Отмена')),
    h('button', {class: 'btn primary', onclick: async () => {
      let x;
      try { x = build(); } catch (e) { toast(e.message, 'err'); return; }
      if (isNew) d.reminders.push(x); else d.reminders[i] = x;
      if (await saveDota(d, {ok: _t('Сохранено')})) dlg.close();
    }}, _t('Сохранить'))]});
  function ph() { const el = document.getElementById('remSay'); if (el) el.placeholder = auto(); }
}
