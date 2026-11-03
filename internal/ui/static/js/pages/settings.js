// Настройки — устройства, громкость, кнопка голосового чата, синтез речи, клавиши, программа, обновления.
import {app, h, api, act, set, toast, reload, requestRender, kbd, keyLabel, bindKey, captureKey, actionLabel, slider, toggle, select, setrow,
  confirmDlg, isVirt, shortDev, cableRec, clone, hint, icon, soundById} from '../core.js';
import {fix} from './check.js';

let voices = null, upd = null, backups = null, ttsText = 'Привет! Это VoiceBox. Проверка голоса.';

export default function settingsPage() {
  const c = app.state.config;
  return h('div', {},
    h('div', {class: 'grid2'},
      h('div', {class: 'stack'}, devicesCard(c), volumeCard(c), pttCard(c), ttsCard(c)),
      h('div', {class: 'stack'}, hotkeysCard(c), programCard(c), updateCard(c), backupsCard())),
    h('div', {class: 'mt'}, sourcesCard(c)));
}

// ---------- устройства ----------
function devicesCard(c) {
  const dv = app.state.devices || {capture: [], playback: []};
  const opt = (list, auto) => [['', auto]].concat((list || []).map(d => [d.name, d.name + (d.is_default ? ' · по умолчанию' : '')]));
  const real = l => (l || []).filter(d => !isVirt(d.name));
  const virt = l => (l || []).filter(d => isVirt(d.name));
  const play = dv.playback || [];
  return h('div', {class: 'card'},
    h('div', {class: 'card-h'}, h('h3', {}, '🔈 Устройства'), h('div', {class: 'grow'}),
      h('button', {class: 'btn sm ghost', title: 'Обновить список устройств', onclick: async () => { try { app.state.devices = await api('/api/devices'); requestRender(); toast('Список обновлён'); } catch (e) { toast(e.message, 'err'); } }}, '🔄')),
    h('div', {class: 'col', style: 'gap:12px'},
      h('label', {class: 'field'}, h('span', {}, '🎙️ Микрофон'), select(opt(real(dv.capture), 'Авто — микрофон по умолчанию'), c.devices.mic, v => set({mic: v}, {ok: 'Микрофон выбран'}))),
      h('label', {class: 'field'}, h('span', {}, '🎧 Наушники (что слышите вы)'), select(opt(real(play), 'Авто — устройство по умолчанию'), c.devices.monitor, v => set({monitor: v}, {ok: 'Наушники выбраны'}))),
      h('label', {class: 'field'}, h('span', {}, '🔌 Виртуальный кабель (куда идёт эфир)'),
        select(opt(virt(play).concat(real(play)), 'Авто — найти кабель' + (dv.cable ? ' (' + shortDev(dv.cable) + ')' : '')), c.devices.voice_out, v => set({voice_out: v}, {ok: 'Кабель выбран'}))),
      dv.cable ? h('p', {class: 'hint', style: 'margin:0'}, ['📡 В Discord, Dota 2 и OBS выберите микрофон ', h('b', {}, shortDev(cableRec())), '.'])
        : hint(['Виртуальный кабель не найден. ', h('button', {class: 'btn sm', onclick: () => fix('cable')}, 'Установить VB-Cable'), ' или выберите свой кабель (например «Line 1») в списке выше.'], 'warn'),
      h('details', {}, h('summary', {class: 'hint', style: 'cursor:pointer'}, 'Дополнительно'),
        h('label', {class: 'field mt-s'}, h('span', {}, 'Другой конец кабеля (микрофон для программ)'),
          select(opt(dv.capture, 'Авто' + (dv.cable_rec ? ' (' + shortDev(dv.cable_rec) + ')' : '')), c.devices.cable_rec || '', v => set({cable_rec: v})))),
      dv.error ? h('p', {class: 'bad'}, dv.error) : null));
}

// ---------- громкость ----------
function volumeCard(c) {
  return h('div', {class: 'card'}, h('h3', {}, '🎚️ Громкость звуков'),
    h('div', {class: 'col', style: 'gap:12px'},
      slider({label: 'Звуки в эфире (что слышат другие)', value: c.sfx_volume || 1, min: 0, max: 2, step: 0.05, fmt: v => Math.round(v * 100) + '%', onChange: v => set({sfx_volume: v}, {render: false})}),
      slider({label: 'Звуки у вас в наушниках', value: c.monitor_volume, min: 0, max: 2, step: 0.05, fmt: v => Math.round(v * 100) + '%', onChange: v => set({monitor_volume: v}, {render: false})}),
      slider({label: 'Ваш голос, пока играет звук', value: c.ducking, min: 0, max: 1, step: 0.05, fmt: v => v >= 1 ? 'не приглушать' : Math.round(v * 100) + '%', onChange: v => set({ducking: v}, {render: false})})),
    setrow('Выравнивать громкость звуков', 'Тихие и громкие файлы звучат одинаково', toggle(c.normalize_sounds, v => set({normalize: v}))),
    setrow('Эффект голоса и на звуки', 'Например, звуки тоже звучат «по рации»', toggle(c.fx_on_sounds, v => set({fx_on_sounds: v}))));
}

// ---------- кнопка голосового чата ----------
function pttCard(c) {
  const dota = c.dota.enabled;
  return h('div', {class: 'card'}, h('h3', {}, '📻 Кнопка голосового чата игры'),
    h('p', {class: 'hint', style: 'margin:-4px 0 6px'}, 'Если в игре или Discord голос работает по кнопке (рация), VoiceBox сам зажмёт её, пока играет звук, — иначе вас не услышат.'),
    setrow('Кнопка', c.ptt.key ? 'Та же, что в игре' : 'Не задана — звуки слышно, только если войс включён',
      kbd(c.ptt.key, {onclick: async () => { const k = await captureKey({title: 'Кнопка голосового чата', single: true, current: c.ptt.key, hint: 'Нажмите ту же клавишу, что в игре (или боковую кнопку мыши).'}); if (k !== null) set({ptt_key: k}); }, empty: 'не задана'}),
      dota ? h('button', {class: 'btn sm', title: 'Прочитать бинд из настроек Dota 2', onclick: () => fix('ptt')}, 'Взять из Dota 2') : null),
    setrow('Зажимать, пока играет звук', '', toggle(c.ptt.auto, v => set({ptt_auto: v}))),
    dota ? setrow('Следить за биндом в Dota 2', 'Поменяли клавишу в игре — VoiceBox подхватит', toggle(c.ptt.auto_detect, v => set({ptt_auto_detect: v}))) : null,
    c.ptt.auto ? h('div', {class: 'grid2', style: 'grid-template-columns:1fr 1fr;gap:10px 22px;margin-top:8px'},
      slider({label: 'Нажать заранее', value: c.ptt.lead_ms, min: 0, max: 500, step: 10, fmt: v => v + ' мс', onChange: v => set({ptt_lead_ms: v}, {render: false})}),
      slider({label: 'Отпустить после', value: c.ptt.tail_ms, min: 0, max: 1000, step: 10, fmt: v => v + ' мс', onChange: v => set({ptt_tail_ms: v}, {render: false})})) : null);
}

// ---------- синтез речи ----------
function ttsCard(c) {
  if (!voices) { voices = {voices: []}; api('/api/tts/voices').then(r => { voices = r; requestRender(); }).catch(() => {}); }
  const st = app.status;
  const list = voices.voices || [];
  return h('div', {class: 'card'}, h('h3', {}, '🗣️ Синтез речи'),
    h('p', {class: 'hint', style: 'margin:-4px 0 10px'}, 'Голос Windows озвучивает фразы на плитках, оповещения и напоминания.'),
    h('div', {class: 'col', style: 'gap:12px'},
      h('label', {class: 'field'}, h('span', {}, 'Голос'), select([['', 'По умолчанию (русский, если есть)']].concat(list.map(v => [v.id, v.name + (v.lang ? ' · ' + v.lang : '')])), c.tts_voice || '', v => set({tts_voice: v}, {ok: 'Голос выбран — фразы переозвучиваются'}))),
      voices.error ? h('p', {class: 'bad', style: 'margin:0'}, voices.error) : null,
      slider({label: 'Скорость', value: c.tts_rate || 0, min: -5, max: 5, step: 1, fmt: v => v === 0 ? 'обычная' : (v > 0 ? 'быстрее +' + v : 'медленнее ' + v), onChange: v => set({tts_rate: v}, {render: false})}),
      h('div', {class: 'row nowrap'}, h('input', {class: 'input grow', value: ttsText, oninput: e => { ttsText = e.target.value; }}),
        h('button', {class: 'btn', onclick: () => act('/api/tts/say', {text: ttsText, to: 'monitor'})}, '🎧 Сказать')),
      st.tts_error ? hint(['⚠ ', st.tts_error, ' ', h('button', {class: 'btn sm', onclick: () => act('/api/tts/retry', {}, {ok: 'Пробую снова'})}, 'Повторить')], 'warn') : null,
      st.tts_busy ? h('p', {class: 'hint', style: 'margin:0'}, '⏳ Готовлю озвучку фраз…') : null));
}

// ---------- горячие клавиши ----------
function hotkeysCard(c) {
  const hk = app.state.hotkeys || {};
  const base = ['menu', 'say', 'wheel', 'stop', 'ui', 'mic_mute', 'mic_monitor', 'talk', 'denoise', 'bg', 'preset:next', 'preset:prev']
    .concat((c.sets || []).length ? ['set:next'] : [], c.dota.enabled ? ['rosh', 'clock_horn'] : []);
  // клавиши звуков, которых сейчас нет (например, кнопки Dota 2 при выключенном модуле), не показываем
  const live = a => !a.startsWith('sound:') || !!soundById(a.slice(6).split('@')[0]);
  const extra = Object.keys(hk).filter(a => !base.includes(a) && live(a)).sort((a, b) => actionLabel(a).localeCompare(actionLabel(b), 'ru'));
  const row = a => h('tr', {}, h('td', {}, actionLabel(a)), h('td', {style: 'text-align:right;white-space:nowrap'},
    kbd(hk[a], {onclick: () => bindKey(a), empty: 'назначить'}),
    hk[a] ? h('button', {class: 'btn sm icon ghost', title: 'Убрать клавишу', onclick: () => act('/api/hotkey', {action: a, key: ''}, {ok: 'Клавиша убрана'})}, icon('x', 14)) : h('span', {style: 'display:inline-block;width:28px'})));
  return h('div', {class: 'card'}, h('h3', {}, '⌨️ Горячие клавиши'),
    h('p', {class: 'hint', style: 'margin:-4px 0 8px'}, 'Работают везде, даже в игре на весь экран, и не отбираются у игры. Клавиши звуков и эффектов задаются на их плитках.'),
    h('table', {class: 't'}, base.map(row)),
    extra.length ? [h('div', {class: 'flabel', style: 'margin:14px 0 4px'}, 'Звуки и эффекты'), h('table', {class: 't'}, extra.map(row))] : null);
}

// ---------- программа ----------
function programCard(c) {
  return h('div', {class: 'card'}, h('h3', {}, '🖥️ Программа'),
    setrow('Запускать вместе с Windows', 'В фоне, значок появится возле часов', toggle(app.state.autostart, async v => {
      try { await api('/api/setup/autostart', {on: v}); await reload(); toast(v ? 'Автозапуск включён' : 'Автозапуск выключен', 'ok'); } catch (e) { toast(e.message, 'err'); }
    })),
    setrow('Крестик сворачивает в трей', 'Звуки и клавиши продолжают работать. Выход — в меню значка или кнопкой «Выход».', toggle(c.ui.keep_running_on_close, v => set({keep_running: v}))),
    setrow('Открывать окно при запуске', 'Иначе VoiceBox тихо стартует в трее', toggle(c.ui.open_on_start, v => set({open_on_start: v}))),
    h('div', {class: 'row mt'},
      h('button', {class: 'btn sm', onclick: () => act('/api/open', {what: 'sounds'})}, '📂 Папка звуков'),
      h('button', {class: 'btn sm', onclick: () => act('/api/open', {what: 'config'})}, '📂 Папка программы'),
      h('button', {class: 'btn sm ghost', onclick: () => act('/api/open', {what: 'repo'})}, 'GitHub'),
      h('button', {class: 'btn sm ghost', onclick: async () => { if (await confirmDlg('Снова показать мастер первого запуска?', {ok: 'Показать'})) { const {openWizard} = await import('../wizard.js'); openWizard(); } }}, '🪄 Мастер настройки')));
}

// ---------- обновления ----------
const UPD = {idle: 'Ещё не проверялось', checking: 'Проверяю…', uptodate: '✅ У вас последняя версия', available: '⬆ Доступна новая версия', downloading: '⬇ Скачиваю…', ready: 'Готово к установке', error: '⚠ Ошибка'};
async function updAct(path, msg) { toast(msg); try { upd = await api(path, {}); } catch (e) { toast(e.message, 'err'); } requestRender(); }
function updateCard(c) {
  if (!upd) { upd = (app.status && app.status.update) || {}; api('/api/update').then(r => { upd = r; requestRender(); }).catch(() => {}); }
  const u = upd;
  return h('div', {class: 'card'}, h('div', {class: 'card-h'}, h('h3', {}, '⬆️ Обновления'), h('div', {class: 'grow'}), h('span', {class: 'badge dim'}, app.state.version)),
    u.state ? h('p', {style: 'margin:0 0 6px'}, UPD[u.state] || u.state, u.latest && u.available ? ': ' + u.latest : '', u.state === 'downloading' && u.progress ? ' ' + Math.round(u.progress * 100) + '%' : '') : null,
    u.error ? h('p', {class: 'bad', style: 'margin:0 0 6px'}, u.error) : null,
    u.notes && u.available ? h('div', {class: 'out', style: 'max-height:120px;margin-bottom:8px'}, u.notes) : null,
    h('div', {class: 'row', style: 'margin-bottom:6px'},
      h('button', {class: 'btn sm', onclick: () => updAct('/api/update/check', 'Проверяю…')}, '🔄 Проверить'),
      u.available ? h('button', {class: 'btn sm primary', onclick: async () => { if (await confirmDlg('Установить ' + u.latest + '? Программа перезапустится.', {ok: 'Обновить'})) updAct('/api/update/install', 'Скачиваю…'); }}, '⬇ Обновить до ' + u.latest) : null,
      u.can_rollback && u.previous ? h('button', {class: 'btn sm ghost', onclick: async () => { if (await confirmDlg('Вернуть версию ' + u.previous + '?', {ok: 'Вернуть'})) updAct('/api/update/rollback', 'Откатываю…'); }}, '↩ Вернуть ' + u.previous) : null),
    setrow('Проверять автоматически', '', toggle(c.update.auto_check !== false, v => set({update_auto_check: v}))),
    setrow('Устанавливать сами', 'Не во время матча. Если новая версия не запустится — вернётся прежняя.', toggle(!!c.update.auto_install, v => set({update_auto_install: v}))));
}

// ---------- резервные копии ----------
// config-20261101-103650.003151136.json → «1 ноября, 10:36:50»
const MONTHS = ['января', 'февраля', 'марта', 'апреля', 'мая', 'июня', 'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'];
function backupLabel(n) {
  const m = /(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})/.exec(n);
  if (!m) return n;
  const now = new Date(), y = +m[1];
  return +m[3] + ' ' + MONTHS[+m[2] - 1] + (y !== now.getFullYear() ? ' ' + y : '') + ', ' + m[4] + ':' + m[5] + ':' + m[6];
}
function backupsCard() {
  if (!backups) { backups = []; api('/api/backups').then(r => { backups = r.backups || []; requestRender(); }).catch(() => {}); }
  return h('div', {class: 'card'}, h('h3', {}, '🗂️ Резервные копии настроек'),
    backups.length ? h('table', {class: 't'}, backups.slice(0, 5).map(b => { const n = b.name || b; return h('tr', {}, h('td', {class: 'dim'}, backupLabel(n)),
      h('td', {style: 'text-align:right'}, h('button', {class: 'btn sm', onclick: async () => {
        if (!await confirmDlg('Текущие настройки заменятся копией от ' + backupLabel(n) + '.', {title: 'Восстановить настройки?', ok: 'Восстановить'})) return;
        try { await api('/api/backups/restore', {name: n}); toast('Настройки восстановлены', 'ok'); backups = null; await reload(); } catch (e) { toast(e.message, 'err'); }
      }}, 'Восстановить'))); })) :
      h('p', {class: 'hint', style: 'margin:0'}, 'Копии создаются при каждом сохранении (хранятся 5 последних).'));
}

// ---------- дополнительные источники ----------
function sourcesCard(c) {
  const st = app.status.strips || [];
  const dv = app.state.devices || {};
  const save = (fn, render = true) => { const x = clone(app.state.config); x.sources = x.sources || []; fn(x); return act('/api/config', x, {method: 'PUT', render}); };
  const kind = h('select', {class: 'input sm'}, h('option', {value: 'loopback'}, '🔁 Звук устройства (музыка, браузер)'), h('option', {value: 'input'}, '🎚 Вход (второй микрофон, аудиоинтерфейс)'));
  const dev = h('select', {class: 'input sm', style: 'max-width:280px'});
  const fill = () => dev.replaceChildren(...((kind.value === 'loopback' ? dv.playback : dv.capture) || []).filter(d => !isVirt(d.name)).map(d => h('option', {value: d.name}, d.name)));
  kind.onchange = fill; fill();
  return h('details', {class: 'card', open: (c.sources || []).length > 0},
    h('summary', {style: 'cursor:pointer;list-style:none', class: 'row'}, h('h3', {}, '🎛️ Дополнительные источники'), h('span', {class: 'hint'}, 'для опытных: музыка или второй микрофон в эфир')),
    h('div', {class: 'mt'},
      (c.sources || []).map((src, i) => {
        const s = st.find(x => x.id === src.id) || {};
        const upd = k => v => save(x => { x.sources[i][k] = v; }, false);
        return h('div', {class: 'card', style: 'background:var(--card2);margin-bottom:10px'},
          h('div', {class: 'row'}, h('b', {}, (src.kind === 'loopback' ? '🔁 ' : '🎚 ') + (src.label || src.id)),
            h('span', {class: s.running ? 'faint' : 'warn', style: 'font-size:12px'}, s.running ? shortDev(s.device) : '⚠ ' + (s.error || 'не запущен')),
            h('div', {class: 'grow'}),
            h('div', {class: 'meter', 'data-meter': 'src_' + src.id, style: 'width:140px'}, h('i'), h('b')),
            toggle(!src.mute, v => save(x => { x.sources[i].mute = !v; }), {title: 'Включён'}),
            h('button', {class: 'btn sm icon ghost', title: 'Удалить', onclick: async () => { if (await confirmDlg('Удалить источник «' + (src.label || src.id) + '»?', {ok: 'Удалить', danger: true})) save(x => { x.sources.splice(i, 1); }); }}, '🗑')),
          h('div', {class: 'knobs mt-s'},
            slider({label: 'Громкость', value: src.gain, min: 0, max: 3, step: 0.05, fmt: v => Math.round(v * 100) + '%', onChange: upd('gain')}),
            slider({label: 'В эфир', value: src.to_air, min: 0, max: 1.5, step: 0.05, fmt: v => Math.round(v * 100) + '%', onChange: upd('to_air')}),
            slider({label: 'Вам в наушники', value: src.to_mon, min: 0, max: 1.5, step: 0.05, fmt: v => Math.round(v * 100) + '%', onChange: upd('to_mon')}),
            slider({label: 'Тише, когда вы говорите', value: src.duck || 0, min: 0, max: 1, step: 0.05, fmt: v => Math.round(v * 100) + '%', onChange: upd('duck')})));
      }),
      h('div', {class: 'row'}, kind, dev, h('button', {class: 'btn sm primary', onclick: () => {
        if (!dev.value) { toast('Выберите устройство', 'err'); return; }
        const label = shortDev(dev.value).slice(0, 30) || 'Источник';
        const lb = kind.value === 'loopback';
        save(x => x.sources.push({id: 'src' + Date.now().toString(36), label, kind: kind.value, device: dev.value, channel: 0, gain: 1, to_air: lb ? 0.6 : 1, to_mon: 0, duck: lb ? 0.5 : 0}));
      }}, '＋ Добавить'))));
}
