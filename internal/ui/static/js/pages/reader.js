// «Читалка» — набранный текст читает голос персонажа (ехидный, мерзкий, демон…): слышит команда и/или вы.
// Тут же — чтение чата Dota 2 вслух, клавиши для игры и голос Windows.
import {_t, _tf} from '../i18n.js';
import {app, h, api, act, toast, kbd, bindKey, slider, seg, toggle, select, setrow, hint, clone, go, requestRender, cardH} from '../core.js';

const SAMPLE = _t('Ну что, погнали? Сейчас всех раскидаем!');
const TO = [['both', _t('👥 Команда и вы')], ['voice', _t('🎙️ Только команда')], ['monitor', _t('🎧 Только вы')]];
const HEARS = {both: _t('слышат команда и вы'), voice: _t('слышит только команда'), monitor: _t('слышите только вы')};
let text = '', busy = false, winVoices = null, first = true;

const rcfg = () => clone(app.state.config.reader);
const save = (r, opt = {}) => act('/api/reader', {reader: r}, opt);
const info = () => app.state.reader || {voices: [], history: []};
const voiceOf = id => info().voices.find(v => v.id === id) || info().voices[0] || {id: '', emoji: '🙂', title: '', desc: ''};
const hk = () => app.state.hotkeys || {};

// say — прочитать: to = '' (как выбрано), 'monitor' (только себе). Набранное после отправки команде стирается.
async function say(to, t, fromBox) {
  t = (t || '').trim();
  if (!t) { toast(_t('Напишите, что прочитать'), 'err'); return; }
  busy = true; requestRender();
  try {
    await api('/api/reader/say', {text: t, to});
    if (fromBox && to !== 'monitor') text = '';
  } catch (e) { toast(e.message, 'err'); }
  busy = false; requestRender();
}

async function pickVoice(id) {
  const r = rcfg();
  if (r.voice !== id) { r.voice = id; await save(r); }
  api('/api/reader/say', {text: text.trim() || SAMPLE, voice: id, preview: true}).catch(e => toast(e.message, 'err'));
}

export default function readerPage() {
  const r = rcfg();
  return h('div', {class: 'grid2', style: 'grid-template-columns:minmax(380px,1.15fr) minmax(360px,1fr)'},
    h('div', {class: 'stack'}, composer(r), voicesCard(r), historyCard()),
    h('div', {class: 'stack'}, howCard(), chatCard(r), soundCard(r)));
}

// ---------- что сказать ----------
function composer(r) {
  const inf = info(), v = voiceOf(r.voice);
  const cnt = h('span', {class: 'faint', style: 'font-size:12px'}, text.length ? text.length + ' / 300' : '');
  // страница перерисовывается и во время набора (новые фразы в «Недавних»): поле возвращает фокус и курсор
  const prev = document.activeElement, had = !!(prev && prev.dataset && prev.dataset.keep === 'reader');
  const sel = had ? [prev.selectionStart, prev.selectionEnd] : null;
  const ta = h('textarea', {class: 'input', rows: 3, maxlength: 300, value: text, 'data-keep': 'reader', style: 'width:100%;min-height:86px',
    placeholder: _tf('Напишите фразу — её прочитает голос «{0}»', v.title),
    oninput: e => { text = e.target.value; cnt.textContent = text.length ? text.length + ' / 300' : ''; },
    onkeydown: e => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); say(e.ctrlKey ? 'monitor' : '', ta.value, true); }
    }});
  if (had) queueMicrotask(() => { ta.focus(); ta.setSelectionRange(Math.min(sel[0], text.length), Math.min(sel[1], text.length)); });
  else if (first) { first = false; setTimeout(() => { if (app.page === 'reader' && document.activeElement === document.body) ta.focus(); }); }
  return h('div', {class: 'card'},
    cardH(_t('📝 Что сказать'), h('span', {class: 'chip', title: _t('Голос персонажа — ниже')}, v.emoji + ' ' + v.title)),
    !inf.can_say ? hint(_t('⚠ Голоса Windows недоступны — читать нечем. Загляните на страницу «Проверка».'), 'warn') : null,
    ta,
    h('div', {class: 'row', style: 'margin-top:4px'}, h('span', {class: 'hint'}, _t('Enter — сказать · Ctrl+Enter — себе · Shift+Enter — новая строка')),
      h('div', {class: 'grow'}), cnt),
    h('div', {class: 'row mt-s', style: 'gap:8px'},
      h('button', {class: 'btn primary', disabled: busy, onclick: () => say('', ta.value, true)}, busy ? _t('⏳ Читаю…') : _t('🗣️ Сказать')),
      h('button', {class: 'btn', disabled: busy, title: 'Ctrl+Enter', onclick: () => say('monitor', ta.value, true)}, _t('🎧 Послушать самому')),
      h('div', {class: 'grow'}),
      h('button', {class: 'btn icon ghost', disabled: !inf.can_repeat, title: _t('Повторить последнюю фразу'),
        onclick: () => api('/api/reader/repeat', {}).catch(e => toast(e.message, 'err'))}, '🔁'),
      h('button', {class: 'btn icon ghost', title: _t('Замолчать: оборвать фразу и очередь чата'), onclick: () => api('/api/reader/stop', {}).catch(() => {})}, '🤐')),
    h('div', {class: 'flabel', style: 'margin:14px 0 6px'}, _t('Кто слышит')),
    seg(TO, r.to, to => { const x = rcfg(); x.to = to; save(x, {ok: _t('Теперь ') + HEARS[to]}); }),
    r.to !== 'monitor' ? h('p', {class: 'hint', style: 'margin:8px 0 0'},
      _t('Команда слышит через виртуальный микрофон — как звуки с панели (с рацией VoiceBox сам зажмёт кнопку).')) : null);
}

// ---------- голоса ----------
function voicesCard(r) {
  return h('div', {class: 'card'},
    cardH(_t('🎭 Голос персонажа'), h('span', {class: 'hint'}, _t('Нажмите — выбрать и послушать (только вам)'))),
    h('div', {class: 'fx-grid'}, info().voices.map(v => h('button', {class: 'fx' + (v.id === r.voice ? ' on' : ''), title: v.desc, onclick: () => pickVoice(v.id)},
      h('span', {class: 'fx-e'}, v.emoji), h('span', {class: 'fx-t'}, h('span', {class: 'ell'}, v.title)), h('span', {class: 'fx-d'}, v.desc)))));
}

// ---------- недавние ----------
const SRC = {chat: _t('чат'), script: _t('скрипт')};
const hhmm = ms => new Date(ms).toLocaleTimeString('ru-RU', {hour: '2-digit', minute: '2-digit'});
function historyCard() {
  const list = info().history || [];
  if (!list.length) return null;
  return h('div', {class: 'card'}, cardH(_t('🕘 Недавние'), h('span', {class: 'hint'}, _t('Нажмите — прочитать ещё раз'))),
    h('div', {class: 'pick-list', style: 'max-height:340px'}, list.map(it => {
      const v = voiceOf(it.voice);
      return h('div', {class: 'pick', title: _t('Прочитать ещё раз текущим голосом'), onclick: () => say('', it.text)},
        h('span', {class: 'pe', title: v.title}, v.emoji), h('span', {class: 'ell grow'}, it.text),
        SRC[it.src] ? h('span', {class: 'badge dim'}, it.src === 'chat' && it.channel === 'team' ? _t('чат команды') : SRC[it.src]) : null,
        h('span', {class: 'faint', style: 'font-size:12px'}, hhmm(it.at)),
        h('button', {class: 'btn sm icon ghost', title: _t('В поле ввода'), onclick: e => { e.stopPropagation(); text = it.text; requestRender(); }}, '✏️'));
    })));
}

// ---------- в игре ----------
function howCard() {
  const k = hk().reader;
  const step = (n, title, txt) => h('div', {class: 'wh-step'}, h('span', {class: 'wh-n'}, n), h('div', {}, h('b', {}, title), txt ? h('span', {class: 'dim'}, ' — ' + txt) : null));
  const row = (a, title, sub) => setrow(title, sub, kbd(hk()[a], {onclick: () => bindKey(a), empty: _t('назначить')}));
  return h('div', {class: 'card'}, h('h3', {}, _t('🎮 В игре')),
    h('div', {class: 'wh-steps'},
      step(1, [_t('Нажмите '), k ? kbd(k) : h('b', {}, _t('клавишу «Читалки»'))], _t('поверх игры появится строка набора.')),
      step(2, _t('Напишите фразу'), _t('↑↓ или ← → выбирают голос.')),
      step(3, 'Enter', _t('прочитать. Ctrl+Enter — только себе, Esc — закрыть.'))),
    h('p', {class: 'hint', style: 'margin:10px 0 4px'}, _t('Ещё «Читалка» есть в быстром меню (Enter → наберите текст → «Прочитать голосом…») и в колесе звуков (сектор «Читалка»).')),
    row('reader', _t('Открыть «Читалку»'), _t('строка набора поверх игры')),
    row('reader:repeat', _t('Повторить фразу'), _t('последнюю, без нового синтеза')),
    row('reader:next', _t('Следующий голос'), _t('по кругу, с подсказкой поверх игры')),
    row('reader:stop', _t('Замолчать'), _t('оборвать фразу и очередь чата')));
}

// ---------- чат Dota 2 ----------
async function exampleScript() {
  try {
    const r = await api('/api/scripts/example', {name: 'chat_reader'});
    toast(r.exists ? _t('Скрипт chat_reader уже есть — открываю') : _t('Создан скрипт chat_reader. Он выключен — включите, когда настроите'), 'ok');
    app.openScript = 'chat_reader';
    go('scripts');
  } catch (e) { toast(e.message, 'err'); }
}
function chatCard(r) {
  const c = r.chat, inf = info(), dota = app.state.config.dota.enabled;
  const upd = (f, ok) => { const x = rcfg(); f(x.chat); save(x, {ok}); };
  const sw = (title, sub, key) => setrow(title, sub, toggle(c[key], v => upd(x => { x[key] = v; })));
  return h('div', {class: 'card'},
    setrow(_t('💬 Читать чат Dota 2 вслух'), _t('Сообщения из чата матча читает голос персонажа — не нужно отвлекаться на текст.'),
      toggle(c.enabled, v => upd(x => { x.enabled = v; }, v ? _t('💬 Чтение чата включено') : _t('Чтение чата выключено')), {big: true})),
    c.enabled && !dota ? hint([_t('⚠ Чат приходит через модуль Dota 2 — он выключен. '), h('button', {class: 'btn sm', onclick: () => go('dota')}, _t('Включить модуль'))], 'warn') : null,
    c.enabled ? [
      setrow(_t('Кто слышит'), _t('обычно только вы — команда и так видит чат'), select(TO, c.to, v => upd(x => { x.to = v; }, _t('Чат: ') + HEARS[v]))),
      setrow(_t('Голос'), '', select([['', _t('Как у «Читалки»')]].concat(inf.voices.map(v => [v.id, v.emoji + ' ' + v.title])), c.voice || '', v => upd(x => { x.voice = v; }))),
      sw(_t('Общий чат'), _t('сообщения всем'), 'all'),
      sw(_t('Чат команды'), _t('сообщения союзников'), 'team'),
      sw(_t('Свои сообщения'), '', 'own'),
      sw(_t('Фразы колеса чата'), _t('«Хорошо сыграно!», «Пропал!» и т. п.'), 'wheel'),
      slider({label: _t('Обрезать длинные сообщения после'), value: c.max_len, min: 20, max: 300, step: 10, fmt: v => v + _t(' символов'),
        onChange: v => upd(x => { x.max_len = v; })}),
      setrow(_t('Клавиша: чтение чата вкл/выкл'), '', kbd(hk().chat_read, {onclick: () => bindKey('chat_read'), empty: _t('назначить')})),
      inf.chat_read ? h('p', {class: 'hint', style: 'margin:6px 0 0'}, _t('Прочитано за сеанс: ') + inf.chat_read +
        (inf.chat_dropped ? _t(' · пропущено, когда писали слишком часто: ') + inf.chat_dropped : '')) : null] : null,
    h('p', {class: 'hint', style: 'margin:12px 0 0'}, _t('Хотите по-своему — союзников одним голосом, врагов другим? '),
      h('button', {class: 'btn sm ghost', onclick: exampleScript}, _t('📜 Пример скрипта'))));
}

// ---------- звучание ----------
function soundCard(r) {
  if (!winVoices) { winVoices = {voices: []}; api('/api/tts/voices').then(x => { winVoices = x; requestRender(); }).catch(() => {}); }
  const list = winVoices.voices || [];
  const upd = (f, opt) => { const x = rcfg(); f(x); save(x, opt); };
  return h('div', {class: 'card'}, h('h3', {}, _t('🎚️ Звучание')),
    h('div', {class: 'col', style: 'gap:12px'},
      h('label', {class: 'field'}, h('span', {}, _t('Голос Windows')),
        select([['', _t('Подбирать под персонажа')]].concat(list.map(v => [v.id, v.name + (v.lang ? ' · ' + v.lang : '')])), r.base || '',
          v => upd(x => { x.base = v; }, {ok: v ? _t('Все персонажи — этим голосом') : _t('Голос подбирается под персонажа')}))),
      h('p', {class: 'hint', style: 'margin:-4px 0 0'}, _t('Мужской или женский голос — под персонажа; английский текст читает английский голос, если он есть.')),
      slider({label: _t('Скорость'), value: r.rate || 0, min: -5, max: 5, step: 1, fmt: v => v === 0 ? _t('обычная') : (v > 0 ? _t('быстрее +') + v : _t('медленнее ') + v),
        onChange: v => upd(x => { x.rate = v; }, {render: false})}),
      slider({label: _t('Громкость'), value: r.volume || 1, min: 0.1, max: 2, step: 0.05, fmt: v => Math.round(v * 100) + '%',
        onChange: v => upd(x => { x.volume = v; }, {render: false})})));
}
