// Настройки — устройства, громкость, кнопка голосового чата, синтез речи, клавиши, программа, обновления.
import {_t, _tf, EN} from '../i18n.js';
import {app, h, api, act, set, toast, reload, requestRender, kbd, keyLabel, bindKey, captureKey, actionLabel, slider, toggle, select, setrow,
  confirmDlg, isVirt, shortDev, cableRec, clone, hint, icon, soundById} from '../core.js';
import {fix} from './check.js';

let voices = null, upd = null, backups = null, ttsText = _t('Привет! Это VoiceBox. Проверка голоса.');

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
  const opt = (list, auto) => [['', auto]].concat((list || []).map(d => [d.name, d.name + (d.is_default ? _t(' · по умолчанию') : '')]));
  const real = l => (l || []).filter(d => !isVirt(d.name));
  const virt = l => (l || []).filter(d => isVirt(d.name));
  const play = dv.playback || [];
  return h('div', {class: 'card'},
    h('div', {class: 'card-h'}, h('h3', {}, _t('🔈 Устройства')), h('div', {class: 'grow'}),
      h('button', {class: 'btn sm ghost', title: _t('Обновить список устройств'), onclick: async () => { try { app.state.devices = await api('/api/devices'); requestRender(); toast(_t('Список обновлён')); } catch (e) { toast(e.message, 'err'); } }}, '🔄')),
    h('div', {class: 'col', style: 'gap:12px'},
      h('label', {class: 'field'}, h('span', {}, _t('🎙️ Микрофон')), select(opt(real(dv.capture), _t('Авто — микрофон по умолчанию')), c.devices.mic, v => set({mic: v}, {ok: _t('Микрофон выбран')}))),
      h('label', {class: 'field'}, h('span', {}, _t('🎧 Наушники (что слышите вы)')), select(opt(real(play), _t('Авто — устройство по умолчанию')), c.devices.monitor, v => set({monitor: v}, {ok: _t('Наушники выбраны')}))),
      h('label', {class: 'field'}, h('span', {}, _t('🔌 Виртуальный кабель (куда идёт эфир)')),
        select(opt(virt(play).concat(real(play)), _t('Авто — найти кабель') + (dv.cable ? ' (' + shortDev(dv.cable) + ')' : '')), c.devices.voice_out, v => set({voice_out: v}, {ok: _t('Кабель выбран')}))),
      dv.cable ? h('p', {class: 'hint', style: 'margin:0'}, [_t('📡 В Discord, Dota 2 и OBS выберите микрофон '), h('b', {}, shortDev(cableRec())), '.'])
        : hint([_t('Виртуальный кабель не найден. '), h('button', {class: 'btn sm', onclick: () => fix('cable')}, _t('Установить VB-Cable')), _t(' или выберите свой кабель (например «Line 1») в списке выше.')], 'warn'),
      h('details', {}, h('summary', {class: 'hint', style: 'cursor:pointer'}, _t('Дополнительно')),
        h('label', {class: 'field mt-s'}, h('span', {}, _t('Другой конец кабеля (микрофон для программ)')),
          select(opt(dv.capture, _t('Авто') + (dv.cable_rec ? ' (' + shortDev(dv.cable_rec) + ')' : '')), c.devices.cable_rec || '', v => set({cable_rec: v})))),
      dv.error ? h('p', {class: 'bad'}, dv.error) : null));
}

// ---------- громкость ----------
function volumeCard(c) {
  return h('div', {class: 'card'}, h('h3', {}, _t('🎚️ Громкость звуков')),
    h('div', {class: 'col', style: 'gap:12px'},
      slider({label: _t('Звуки в эфире (что слышат другие)'), value: c.sfx_volume || 1, min: 0, max: 2, step: 0.05, fmt: v => Math.round(v * 100) + '%', onChange: v => set({sfx_volume: v}, {render: false})}),
      slider({label: _t('Звуки у вас в наушниках'), value: c.monitor_volume, min: 0, max: 2, step: 0.05, fmt: v => Math.round(v * 100) + '%', onChange: v => set({monitor_volume: v}, {render: false})}),
      slider({label: _t('Ваш голос, пока играет звук'), value: c.ducking, min: 0, max: 1, step: 0.05, fmt: v => v >= 1 ? _t('не приглушать') : Math.round(v * 100) + '%', onChange: v => set({ducking: v}, {render: false})})),
    setrow(_t('Выравнивать громкость звуков'), _t('Тихие и громкие файлы звучат одинаково'), toggle(c.normalize_sounds, v => set({normalize: v}))),
    setrow(_t('Эффект голоса и на звуки'), _t('Например, звуки тоже звучат «по рации»'), toggle(c.fx_on_sounds, v => set({fx_on_sounds: v}))));
}

// ---------- кнопка голосового чата ----------
function pttCard(c) {
  const dota = c.dota.enabled;
  return h('div', {class: 'card'}, h('h3', {}, _t('📻 Кнопка голосового чата игры')),
    h('p', {class: 'hint', style: 'margin:-4px 0 6px'}, _t('Если в игре или Discord голос работает по кнопке (рация), VoiceBox сам зажмёт её, пока играет звук, — иначе вас не услышат.')),
    setrow(_t('Кнопка'), c.ptt.key ? _t('Та же, что в игре') : _t('Не задана — звуки слышно, только если войс включён'),
      kbd(c.ptt.key, {onclick: async () => { const k = await captureKey({title: _t('Кнопка голосового чата'), single: true, current: c.ptt.key, hint: _t('Нажмите ту же клавишу, что в игре (или боковую кнопку мыши).')}); if (k !== null) set({ptt_key: k}); }, empty: _t('не задана')}),
      dota ? h('button', {class: 'btn sm', title: _t('Прочитать бинд из настроек Dota 2'), onclick: () => fix('ptt')}, _t('Взять из Dota 2')) : null),
    setrow(_t('Зажимать, пока играет звук'), '', toggle(c.ptt.auto, v => set({ptt_auto: v}))),
    dota ? setrow(_t('Следить за биндом в Dota 2'), _t('Поменяли клавишу в игре — VoiceBox подхватит'), toggle(c.ptt.auto_detect, v => set({ptt_auto_detect: v}))) : null,
    c.ptt.auto ? h('div', {class: 'grid2', style: 'grid-template-columns:1fr 1fr;gap:10px 22px;margin-top:8px'},
      slider({label: _t('Нажать заранее'), value: c.ptt.lead_ms, min: 0, max: 500, step: 10, fmt: v => v + _t(' мс'), onChange: v => set({ptt_lead_ms: v}, {render: false})}),
      slider({label: _t('Отпустить после'), value: c.ptt.tail_ms, min: 0, max: 1000, step: 10, fmt: v => v + _t(' мс'), onChange: v => set({ptt_tail_ms: v}, {render: false})})) : null);
}

// ---------- синтез речи ----------
function ttsCard(c) {
  if (!voices) { voices = {voices: []}; api('/api/tts/voices').then(r => { voices = r; requestRender(); }).catch(() => {}); }
  const st = app.status;
  const list = voices.voices || [];
  return h('div', {class: 'card'}, h('h3', {}, _t('🗣️ Синтез речи')),
    h('p', {class: 'hint', style: 'margin:-4px 0 10px'}, _t('Голос Windows озвучивает фразы на плитках, оповещения и напоминания.')),
    h('div', {class: 'col', style: 'gap:12px'},
      h('label', {class: 'field'}, h('span', {}, _t('Голос')), select([['', _t('По умолчанию (русский, если есть)')]].concat(list.map(v => [v.id, v.name + (v.lang ? ' · ' + v.lang : '')])), c.tts_voice || '', v => set({tts_voice: v}, {ok: _t('Голос выбран — фразы переозвучиваются')}))),
      voices.error ? h('p', {class: 'bad', style: 'margin:0'}, voices.error) : null,
      slider({label: _t('Скорость'), value: c.tts_rate || 0, min: -5, max: 5, step: 1, fmt: v => v === 0 ? _t('обычная') : (v > 0 ? _t('быстрее +') + v : _t('медленнее ') + v), onChange: v => set({tts_rate: v}, {render: false})}),
      h('div', {class: 'row nowrap'}, h('input', {class: 'input grow', value: ttsText, oninput: e => { ttsText = e.target.value; }}),
        h('button', {class: 'btn', onclick: () => act('/api/tts/say', {text: ttsText, to: 'monitor'})}, _t('🎧 Сказать'))),
      st.tts_error ? hint(['⚠ ', st.tts_error, ' ', h('button', {class: 'btn sm', onclick: () => act('/api/tts/retry', {}, {ok: _t('Пробую снова')})}, _t('Повторить'))], 'warn') : null,
      st.tts_busy ? h('p', {class: 'hint', style: 'margin:0'}, _t('⏳ Готовлю озвучку фраз…')) : null));
}

// ---------- горячие клавиши ----------
function hotkeysCard(c) {
  const hk = app.state.hotkeys || {};
  const base = ['menu', 'say', 'wheel', 'reader', 'reader:repeat', 'stop', 'ui', 'mic_mute', 'mic_monitor', 'mic:next', 'talk', 'denoise', 'bg', 'preset:next', 'preset:prev']
    .concat((c.sets || []).length ? ['set:next'] : [], c.dota.enabled ? ['rosh', 'clock_horn', 'chat_read'] : []);
  // клавиши звуков, которых сейчас нет (например, кнопки Dota 2 при выключенном модуле), не показываем
  const live = a => !a.startsWith('sound:') || !!soundById(a.slice(6).split('@')[0]);
  const extra = Object.keys(hk).filter(a => !base.includes(a) && live(a)).sort((a, b) => actionLabel(a).localeCompare(actionLabel(b), 'ru'));
  const row = a => h('tr', {}, h('td', {}, actionLabel(a)), h('td', {style: 'text-align:right;white-space:nowrap'},
    kbd(hk[a], {onclick: () => bindKey(a), empty: _t('назначить')}),
    hk[a] ? h('button', {class: 'btn sm icon ghost', title: _t('Убрать клавишу'), onclick: () => act('/api/hotkey', {action: a, key: ''}, {ok: _t('Клавиша убрана')})}, icon('x', 14)) : h('span', {style: 'display:inline-block;width:28px'})));
  return h('div', {class: 'card'}, h('h3', {}, _t('⌨️ Горячие клавиши')),
    h('p', {class: 'hint', style: 'margin:-4px 0 8px'}, _t('Работают везде, даже в игре на весь экран, и не отбираются у игры. Клавиши звуков и эффектов задаются на их плитках.')),
    h('table', {class: 't'}, base.map(row)),
    extra.length ? [h('div', {class: 'flabel', style: 'margin:14px 0 4px'}, _t('Звуки и эффекты')), h('table', {class: 't'}, extra.map(row))] : null);
}

// ---------- программа ----------
export async function setShortcut(where, on) {
  try {
    await api('/api/setup/shortcut', {where, on}); await reload();
    toast(where === 'start' ? (on ? _t('VoiceBox есть в меню «Пуск»') : _t('Ярлык убран из «Пуска»')) : (on ? _t('Ярлык на рабочем столе') : _t('Ярлык убран с рабочего стола')), 'ok');
  } catch (e) { toast(e.message, 'err'); }
}
export async function setAutostart(on) {
  try { await api('/api/setup/autostart', {on}); await reload(); toast(on ? _t('Автозапуск включён') : _t('Автозапуск выключен'), 'ok'); } catch (e) { toast(e.message, 'err'); }
}
// setLang — язык интерфейса ("" — как в Windows): сохранить и перезагрузить окно
export async function setLang(v) {
  if (await set({ui_lang: v}, {render: false})) location.reload();
}

function programCard(c) {
  const sc = app.state.shortcuts || {};
  return h('div', {class: 'card'}, h('h3', {}, _t('🖥️ Программа')),
    setrow(EN ? 'Interface language' : 'Язык интерфейса · Interface language', '',
      select([['', _t('Как в Windows')], ['ru', 'Русский'], ['en', 'English']], c.ui_lang || '', setLang)),
    setrow(_t('Запускать вместе с Windows'), _t('В фоне, значок появится возле часов'), toggle(app.state.autostart, setAutostart)),
    setrow(_t('Ярлык в меню «Пуск»'), _t('VoiceBox находится поиском Windows, установка не нужна'), toggle(!!sc.start, v => setShortcut('start', v))),
    setrow(_t('Ярлык на рабочем столе'), '', toggle(!!sc.desktop, v => setShortcut('desktop', v))),
    setrow(_t('Крестик сворачивает в трей'), _t('Звуки и клавиши продолжают работать. Выход — в меню значка или кнопкой «Выход».'), toggle(c.ui.keep_running_on_close, v => set({keep_running: v}))),
    setrow(_t('Открывать окно при запуске'), _t('Иначе VoiceBox тихо стартует в трее'), toggle(c.ui.open_on_start, v => set({open_on_start: v}))),
    h('div', {class: 'row mt'},
      h('button', {class: 'btn sm', onclick: () => act('/api/open', {what: 'sounds'})}, _t('📂 Папка звуков')),
      h('button', {class: 'btn sm', onclick: () => act('/api/open', {what: 'config'})}, _t('📂 Папка программы')),
      h('button', {class: 'btn sm ghost', onclick: () => act('/api/open', {what: 'repo'})}, 'GitHub'),
      h('button', {class: 'btn sm ghost', onclick: async () => { if (await confirmDlg(_t('Снова показать мастер первого запуска?'), {ok: _t('Показать')})) { const {openWizard} = await import('../wizard.js'); openWizard(); } }}, _t('🪄 Мастер настройки'))));
}

// ---------- обновления ----------
const UPD = {idle: _t('Ещё не проверялось'), checking: _t('Проверяю…'), uptodate: _t('✅ У вас последняя версия'), available: _t('⬆ Доступна новая версия'), downloading: _t('⬇ Скачиваю…'), ready: _t('Готово к установке'), error: _t('⚠ Ошибка')};
async function updAct(path, msg) { toast(msg); try { upd = await api(path, {}); } catch (e) { toast(e.message, 'err'); } requestRender(); }
function updateCard(c) {
  if (!upd) { upd = (app.status && app.status.update) || {}; api('/api/update').then(r => { upd = r; requestRender(); }).catch(() => {}); }
  const u = upd;
  return h('div', {class: 'card'}, h('div', {class: 'card-h'}, h('h3', {}, _t('⬆️ Обновления')), h('div', {class: 'grow'}), h('span', {class: 'badge dim'}, app.state.version)),
    u.state ? h('p', {style: 'margin:0 0 6px'}, UPD[u.state] || u.state, u.latest && u.available ? ': ' + u.latest : '', u.state === 'downloading' && u.progress ? ' ' + Math.round(u.progress * 100) + '%' : '') : null,
    u.error ? h('p', {class: 'bad', style: 'margin:0 0 6px'}, u.error) : null,
    u.notes && u.available ? h('div', {class: 'out', style: 'max-height:120px;margin-bottom:8px'}, u.notes) : null,
    h('div', {class: 'row', style: 'margin-bottom:6px'},
      h('button', {class: 'btn sm', onclick: () => updAct('/api/update/check', _t('Проверяю…'))}, _t('🔄 Проверить')),
      u.available ? h('button', {class: 'btn sm primary', onclick: async () => { if (await confirmDlg(_t('Установить ') + u.latest + _t('? Программа перезапустится.'), {ok: _t('Обновить')})) updAct('/api/update/install', _t('Скачиваю…')); }}, _t('⬇ Обновить до ') + u.latest) : null,
      u.can_rollback && u.previous ? h('button', {class: 'btn sm ghost', onclick: async () => { if (await confirmDlg(_t('Вернуть версию ') + u.previous + '?', {ok: _t('Вернуть')})) updAct('/api/update/rollback', _t('Откатываю…')); }}, _t('↩ Вернуть ') + u.previous) : null),
    setrow(_t('Проверять автоматически'), '', toggle(c.update.auto_check !== false, v => set({update_auto_check: v}))),
    setrow(_t('Устанавливать сами'), _t('Не во время матча. Если новая версия не запустится — вернётся прежняя.'), toggle(!!c.update.auto_install, v => set({update_auto_install: v}))));
}

// ---------- резервные копии ----------
// config-20261101-103650.003151136.json → «1 ноября, 10:36:50»
const MONTHS = [_t('января'), _t('февраля'), _t('марта'), _t('апреля'), _t('мая'), _t('июня'), _t('июля'), _t('августа'), _t('сентября'), _t('октября'), _t('ноября'), _t('декабря')];
function backupLabel(n) {
  const m = /(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})/.exec(n);
  if (!m) return n;
  const now = new Date(), y = +m[1];
  return +m[3] + ' ' + MONTHS[+m[2] - 1] + (y !== now.getFullYear() ? ' ' + y : '') + ', ' + m[4] + ':' + m[5] + ':' + m[6];
}
function backupsCard() {
  if (!backups) { backups = []; api('/api/backups').then(r => { backups = r.backups || []; requestRender(); }).catch(() => {}); }
  return h('div', {class: 'card'}, h('h3', {}, _t('🗂️ Резервные копии настроек')),
    backups.length ? h('table', {class: 't'}, backups.slice(0, 5).map(b => { const n = b.name || b; return h('tr', {}, h('td', {class: 'dim'}, backupLabel(n)),
      h('td', {style: 'text-align:right'}, h('button', {class: 'btn sm', onclick: async () => {
        if (!await confirmDlg(_t('Текущие настройки заменятся копией от ') + backupLabel(n) + '.', {title: _t('Восстановить настройки?'), ok: _t('Восстановить')})) return;
        try { await api('/api/backups/restore', {name: n}); toast(_t('Настройки восстановлены'), 'ok'); backups = null; await reload(); } catch (e) { toast(e.message, 'err'); }
      }}, _t('Восстановить')))); })) :
      h('p', {class: 'hint', style: 'margin:0'}, _t('Копии создаются при каждом сохранении (хранятся 5 последних).')));
}

// ---------- дополнительные источники ----------
function sourcesCard(c) {
  const st = app.status.strips || [];
  const dv = app.state.devices || {};
  const save = (fn, render = true) => { const x = clone(app.state.config); x.sources = x.sources || []; fn(x); return act('/api/config', x, {method: 'PUT', render}); };
  const kind = h('select', {class: 'input sm'}, h('option', {value: 'loopback'}, _t('🔁 Звук устройства (музыка, браузер)')), h('option', {value: 'input'}, _t('🎚 Вход (второй микрофон, аудиоинтерфейс)')));
  const dev = h('select', {class: 'input sm', style: 'max-width:280px'});
  const fill = () => dev.replaceChildren(...((kind.value === 'loopback' ? dv.playback : dv.capture) || []).filter(d => !isVirt(d.name)).map(d => h('option', {value: d.name}, d.name)));
  kind.onchange = fill; fill();
  return h('details', {class: 'card', open: (c.sources || []).length > 0},
    h('summary', {style: 'cursor:pointer;list-style:none', class: 'row'}, h('h3', {}, _t('🎛️ Дополнительные источники')), h('span', {class: 'hint'}, _t('для опытных: музыка или второй микрофон в эфир'))),
    h('div', {class: 'mt'},
      (c.sources || []).map((src, i) => {
        const s = st.find(x => x.id === src.id) || {};
        const upd = k => v => save(x => { x.sources[i][k] = v; }, false);
        return h('div', {class: 'card', style: 'background:var(--card2);margin-bottom:10px'},
          h('div', {class: 'row'}, h('b', {}, (src.kind === 'loopback' ? '🔁 ' : '🎚 ') + (src.label || src.id)),
            h('span', {class: s.running ? 'faint' : 'warn', style: 'font-size:12px'}, s.running ? shortDev(s.device) : '⚠ ' + (s.error || _t('не запущен'))),
            h('div', {class: 'grow'}),
            h('div', {class: 'meter', 'data-meter': 'src_' + src.id, style: 'width:140px'}, h('i'), h('b')),
            toggle(!src.mute, v => save(x => { x.sources[i].mute = !v; }), {title: _t('Включён')}),
            h('button', {class: 'btn sm icon ghost', title: _t('Удалить'), onclick: async () => { if (await confirmDlg(_tf('Удалить источник «{0}»?', (src.label || src.id)), {ok: _t('Удалить'), danger: true})) save(x => { x.sources.splice(i, 1); }); }}, '🗑')),
          h('div', {class: 'knobs mt-s'},
            slider({label: _t('Громкость'), value: src.gain, min: 0, max: 3, step: 0.05, fmt: v => Math.round(v * 100) + '%', onChange: upd('gain')}),
            slider({label: _t('В эфир'), value: src.to_air, min: 0, max: 1.5, step: 0.05, fmt: v => Math.round(v * 100) + '%', onChange: upd('to_air')}),
            slider({label: _t('Вам в наушники'), value: src.to_mon, min: 0, max: 1.5, step: 0.05, fmt: v => Math.round(v * 100) + '%', onChange: upd('to_mon')}),
            slider({label: _t('Тише, когда вы говорите'), value: src.duck || 0, min: 0, max: 1, step: 0.05, fmt: v => Math.round(v * 100) + '%', onChange: upd('duck')})));
      }),
      h('div', {class: 'row'}, kind, dev, h('button', {class: 'btn sm primary', onclick: () => {
        if (!dev.value) { toast(_t('Выберите устройство'), 'err'); return; }
        const label = shortDev(dev.value).slice(0, 30) || _t('Источник');
        const lb = kind.value === 'loopback';
        save(x => x.sources.push({id: 'src' + Date.now().toString(36), label, kind: kind.value, device: dev.value, channel: 0, gain: 1, to_air: lb ? 0.6 : 1, to_mon: 0, duck: lb ? 0.5 : 0}));
      }}, _t('＋ Добавить')))));
}
