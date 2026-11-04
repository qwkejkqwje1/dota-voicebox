// Проверка — состояние, автонастройка, тесты звука, подсказки для Discord и других программ.
import {_t, _tf} from '../i18n.js';
import {app, h, api, act, set, toast, reload, requestRender, confirmDlg, select, cableRec, cableIn, shortDev, isVirt, sectionH, presetById} from '../core.js';

export const DOTA_CHECKS = ['dota', 'dota_input', 'gsi', 'launch', 'ptt', 'gsi_live'];

let loading = null;
export function loadSetup() {
  if (loading) return loading;
  loading = api('/api/setup').then(r => { app.setup = r; requestRender(); }).catch(e => toast(e.message, 'err')).finally(() => { loading = null; });
  return loading;
}
export async function fix(id) {
  const ch = app.setup && app.setup.checks.find(c => c.id === id);
  if (id === 'launch' && ch && /перезапущен|Steam/.test(ch.detail || '') &&
    !await confirmDlg(_t('Чтобы добавить параметр запуска Dota 2, Steam будет закрыт и открыт снова.'), {title: _t('Перезапустить Steam?'), ok: _t('Перезапустить')})) return;
  toast(_t('Выполняю…'));
  try { app.setup = await api('/api/setup/fix', {id}); toast(_t('Готово'), 'ok'); } catch (e) { toast(e.message, 'err'); }
  await reload();
  loadSetup();
}
export async function autoSetup() {
  const dota = app.state.config.dota.enabled;
  const launch = app.setup && app.setup.checks.find(c => c.id === 'launch' && !c.ok);
  const steam = dota && launch ? await confirmDlg(_t('Если нужно добавить параметр запуска Dota 2, Steam будет перезапущен. Разрешить?'), {title: _t('Автонастройка'), ok: _t('Разрешить'), cancel: _t('Без перезапуска')}) : false;
  toast(_t('Настраиваю…'));
  try {
    const r = await api('/api/setup/auto', {allow_steam_restart: steam});
    app.setup = r.setup;
    (r.report || []).forEach(x => toast(x));
  } catch (e) { toast(e.message, 'err'); }
  await reload();
}
const icon = ch => ch.ok ? '✅' : ch.id === 'gsi_live' ? '⏳' : ch.fix ? '⚠️' : '❌';
export function checkRows(filter) {
  if (!app.setup) { loadSetup(); return h('p', {class: 'hint'}, _t('Проверяю…')); }
  const list = app.setup.checks.filter(filter);
  return h('div', {class: 'checks'}, list.map(ch => h('div', {class: 'check'},
    h('span', {class: 'ic'}, icon(ch)),
    h('div', {style: 'min-width:0'}, h('b', {}, ch.title), ch.detail ? h('small', {}, ch.detail) : null, ch.manual ? h('small', {class: 'warn'}, ch.manual) : null),
    ch.fix && !ch.ok ? h('button', {class: 'btn sm primary', onclick: () => fix(ch.fix)}, ch.fix_label || _t('Исправить')) : h('span'))));
}

// ---------- тесты ----------
const TESTS = [
  ['monitor', '🎧', _t('Наушники'), _t('Короткий сигнал только вам — проверка вывода в наушники.'), _t('Проверить')],
  ['mic', '🎤', _t('Микрофон'), _t('Скажите что-нибудь 3 секунды обычным голосом. Покажет громкость, шум и подскажет усиление.'), _t('Записать 3 с')],
  ['cable', '🔌', _t('Виртуальный кабель'), _t('Пропускает тон через кабель и слушает его с другой стороны: уровень и задержка.'), _t('Проверить')],
  ['voice', '🎛️', _t('Эффект голоса'), _t('Запишите фразу — услышите её без эффекта, а затем с выбранным эффектом.'), _t('Записать 3 с')],
  ['team', '👂', _t('Как меня слышат'), _t('Записывает ровно то, что уходит в кабель (голос + эффект + звуки), и проигрывает вам.'), _t('Записать 4 с')],
  ['ptt', '⌨️', _t('Кнопка голосового чата'), _t('Зажмёт кнопку голосового чата на 1,5 с — в игре загорится значок микрофона.'), _t('Нажать')],
];
const TR = {};
let running = null, testPreset = '', wizard = null;
async function runTest(id) {
  if (running) return;
  running = id; TR[id] = null; requestRender();
  try { TR[id] = await api('/api/test', {id, sec: id === 'team' ? 4 : 3, preset: testPreset || app.status.preset}); }
  catch (e) { TR[id] = {verdict: 'fail', summary: e.message}; }
  running = null; requestRender();
}
async function runAll() {
  const order = TESTS.map(t => t[0]).filter(id => id !== 'ptt' || app.state.config.ptt.key);
  wizard = {i: 0, n: order.length, cur: ''};
  for (const id of order) {
    wizard.cur = id; requestRender();
    if (['mic', 'voice', 'team'].includes(id)) { toast(_t('🎤 Сейчас запись — говорите обычным голосом')); await new Promise(r => setTimeout(r, 900)); }
    await runTest(id);
    wizard.i++;
    if (TR[id] && TR[id].verdict === 'fail' && ['monitor', 'cable'].includes(id)) { toast(_t('Сначала исправьте: ') + TESTS.find(t => t[0] === id)[2], 'err'); break; }
  }
  wizard = null; requestRender();
}
function stats(s) {
  if (!s) return null;
  return h('div', {class: 'stats'},
    [[_t('Речь'), s.speech_db], [_t('Фон'), s.noise_db], [_t('Пик'), s.peak_db]].map(([l, v]) => h('div', {}, h('small', {}, l), h('b', {}, Math.round(v) + _t(' дБ')),
      h('div', {class: 'meter', style: 'height:5px;margin-top:4px'}, h('i', {style: `width:${Math.max(0, Math.min(100, (v + 60) / 60 * 100))}%;transition:none`})))),
    s.clip_pct > 0 ? h('div', {}, h('small', {}, _t('Перегруз')), h('b', {class: s.clip_pct > 0.1 ? 'bad' : ''}, s.clip_pct + '%')) : null);
}
function testsCard() {
  const done = TESTS.filter(t => TR[t[0]]);
  const cnt = v => done.filter(t => TR[t[0]].verdict === v).length;
  const V = {ok: '✅', warn: '⚠️', fail: '❌'};
  return h('div', {},
    sectionH(_t('Тесты звука'), _t('идут по-настоящему через ваши устройства'), 
      wizard ? h('span', {class: 'chip warn'}, `⏳ ${wizard.i + 1}/${wizard.n}`) : done.length ? h('span', {class: 'hint'}, _tf("{0} ок · {1} с замечаниями · {2} ошибок", cnt('ok'), cnt('warn'), cnt('fail'))) : null,
      h('button', {class: 'btn primary', disabled: !!running, onclick: runAll}, _t('▶ Проверить всё'))),
    h('div', {class: 'grid3'}, TESTS.map(([id, ic, title, desc, btn]) => {
      const r = TR[id], busy = running === id;
      if (id === 'ptt' && !app.state.config.ptt.key) desc = _t('Сначала задайте кнопку голосового чата в «Настройках».');
      return h('div', {class: 'card test ' + (r ? r.verdict : '')},
        h('div', {class: 'row nowrap'}, h('span', {style: 'font-size:20px'}, ic), h('b', {style: 'font-size:15px'}, title), h('div', {class: 'grow'}), r ? h('span', {}, V[r.verdict] || '') : null),
        h('p', {class: 'hint', style: 'margin:6px 0 10px'}, desc),
        id === 'voice' ? h('div', {style: 'margin-bottom:10px'}, select((app.state.presets || []).map(p => [p.id, (p.emoji || '') + ' ' + p.title]), testPreset || app.status.preset, v => { testPreset = v; }, {cls: 'input sm'})) : null,
        h('button', {class: 'btn' + (busy ? ' on' : ''), disabled: !!running || (id === 'ptt' && !app.state.config.ptt.key), onclick: () => runTest(id)},
          busy ? (['mic', 'voice', 'team'].includes(id) ? _t('● Запись… говорите') : _t('⏳ Проверяю…')) : btn),
        r ? h('div', {class: 'mt-s'}, h('b', {}, r.summary), stats(r.stats),
          (r.details || []).map(d => h('div', {class: 'hint'}, '• ' + d)),
          r.ask ? h('div', {class: 'hint box mt-s'}, r.ask) : null,
          r.suggest && r.suggest.mic_gain ? h('button', {class: 'btn sm mt-s', onclick: () => set({mic_gain: r.suggest.mic_gain}, {ok: _t('Громкость микрофона: ') + Math.round(r.suggest.mic_gain * 100) + '%'})}, _t('✔ Применить громкость ') + Math.round(r.suggest.mic_gain * 100) + '%') : null) : null);
    })));
}

// ---------- программы ----------
async function capture() { return ((await api('/api/apps/capture')).sessions || []); }
function appCheck(re, name) {
  const out = h('span', {class: 'hint'});
  return h('div', {class: 'row'}, h('button', {class: 'btn sm', onclick: async () => {
    out.textContent = '…';
    try {
      const l = (await capture()).filter(x => re.test(x.process));
      const on = l.find(x => isVirt(x.device));
      if (on) { out.className = 'ok'; out.textContent = _tf("✅ {0} слушает {1}{2}", name, shortDev(on.device), on.active ? '' : _t(' (сейчас не пишет)')); }
      else if (l.length) { out.className = 'bad'; out.textContent = _tf("❌ {0} слушает «{1}» — выберите «{2}»", name, shortDev(l[0].device), shortDev(cableRec())); }
      else { out.className = 'warn'; out.textContent = _tf("⚠ {0} сейчас не открыл микрофон — зайдите в голосовой канал и проверьте снова", name); }
    } catch (e) { out.className = 'bad'; out.textContent = e.message; }
  }}, _t('🔍 Проверить ') + name), out);
}
function listeners() {
  const box = h('div', {}, h('p', {class: 'hint', style: 'margin:0'}, _t('Нажмите «Обновить», чтобы увидеть, какие программы сейчас слушают микрофоны.')));
  const load = async () => {
    try {
      const l = await capture();
      box.replaceChildren(l.length ? h('table', {class: 't'}, h('tr', {}, h('th', {}, _t('Программа')), h('th', {}, _t('Устройство')), h('th', {}, '')),
        l.map(x => h('tr', {}, h('td', {}, x.process || ('PID ' + x.pid)), h('td', {class: isVirt(x.device) ? 'ok' : ''}, x.device), h('td', {class: 'faint'}, x.active ? _t('● пишет') : '')))) :
        h('p', {class: 'hint', style: 'margin:0'}, _t('Сейчас ни одна программа не открыла микрофон.')));
    } catch (e) { box.replaceChildren(h('p', {class: 'bad'}, e.message)); }
  };
  return h('div', {class: 'card'}, h('div', {class: 'card-h'}, h('h3', {}, _t('Кто слушает микрофоны')), h('div', {class: 'grow'}), h('button', {class: 'btn sm', onclick: load}, _t('🔄 Обновить'))),
    h('p', {class: 'hint', style: 'margin:0 0 10px'}, _t('Зелёным — программы, которые получают звук VoiceBox. Если нужная программа слушает настоящий микрофон, эффекты и звуки до неё не дойдут.')), box);
}
function guide() {
  const step = (n, title, text, extra) => h('div', {class: 'check'}, h('span', {class: 'ic', style: 'font-weight:700;color:var(--accent2)'}, n), h('div', {}, h('b', {}, title), h('small', {}, text)), extra || h('span'));
  return h('div', {class: 'grid2'},
    h('div', {class: 'card'}, h('h3', {}, _t('💬 Discord — настройка за минуту')),
      h('div', {class: 'checks'},
        step('1', _t('Устройство ввода: ') + shortDev(cableRec()), _t('Discord → Настройки → Голос и видео → «Устройство ввода». Так Discord услышит голос с эффектом и звуки.'),
          h('button', {class: 'btn sm primary', onclick: () => act('/api/open', {what: 'discord_voice'})}, _t('Открыть'))),
        step('2', _t('Устройство вывода: ваши наушники'), _t('Не выбирайте ') + shortDev(cableIn()) + _t(' — иначе собеседники услышат сами себя.')),
        step('3', _t('Шумоподавление Discord — выключить'), _t('Krisp и эхоподавление режут эффекты и звуки. Шум уберёт порог в VoiceBox («Голос»).')),
        step('4', _t('Чувствительность ввода — вручную'), _t('Выключите «Автоматически определять» и поставьте порог около −50 дБ, иначе тихие звуки обрежутся.')),
        step('5', _t('Режим рации (по желанию)'), _t('Если в Discord включена «Рация», задайте ту же клавишу в «Настройки → Кнопка голосового чата» — VoiceBox сам нажмёт её, когда играет звук.'))),
      h('div', {class: 'mt'}, appCheck(/discord/i, 'Discord'))),
    h('div', {class: 'stack'},
      h('div', {class: 'card'}, h('h3', {}, _t('Другие программы')),
        h('p', {class: 'hint', style: 'margin:0 0 10px'}, [_t('Принцип один: в программе выберите микрофон '), h('b', {}, shortDev(cableRec())), '.']),
        h('table', {class: 't'}, [['TeamSpeak', _t('Настройки → Захват → Устройство захвата')], ['Telegram', _t('Настройки → Звонки → Микрофон')], ['OBS', _t('Источник «Захват входного аудиопотока» → ') + shortDev(cableRec())],
          [_t('CS2 и другие игры'), _t('Windows → Параметры → Звук → Ввод (игра берёт системный микрофон)')], ['Dota 2', _t('Настройки → Звук → Устройство ввода → ') + shortDev(cableRec())]].map(([a, b]) => h('tr', {}, h('td', {}, h('b', {}, a)), h('td', {class: 'dim'}, b))))),
      listeners()));
}

export default function checkPage() {
  const dota = app.state.config.dota.enabled;
  const s = app.setup;
  return h('div', {},
    h('div', {class: 'grid2'},
      h('div', {class: 'card'},
        h('div', {class: 'card-h'}, h('h3', {}, _t('Состояние')), s ? h('span', {class: 'badge ' + (s.all_ok ? 'ok' : 'warn')}, s.all_ok ? _t('всё в порядке') : _t('есть что исправить')) : null),
        s && s.busy ? h('div', {class: 'hint box mt-s'}, '⏳ ' + s.busy) : null,
        checkRows(c => !DOTA_CHECKS.includes(c.id)),
        dota ? [h('div', {class: 'flabel', style: 'margin:12px 0 2px'}, 'Dota 2'), checkRows(c => DOTA_CHECKS.includes(c.id))] : null,
        h('div', {class: 'row mt'},
          h('button', {class: 'btn primary', onclick: autoSetup}, _t('✨ Настроить автоматически')),
          h('button', {class: 'btn', onclick: () => { app.setup = null; loadSetup(); }}, _t('🔄 Проверить снова')))),
      h('div', {class: 'card'}, h('h3', {}, _t('Как это работает')),
        h('div', {class: 'checks'},
          [['🎙️', _t('Микрофон → VoiceBox'), _t('Ваш голос проходит через выбранный эффект.')],
            ['🔊', _t('Звуки — туда же'), _t('Плитки, фразы и оповещения подмешиваются к голосу.')],
            ['🔌', _t('Всё — в виртуальный кабель'), _tf('VoiceBox пишет в «{0}».', shortDev(cableIn()))],
            ['💬', _t('Discord и игры слушают кабель'), _tf('В них микрофон — «{0}».', shortDev(cableRec()))]].map(([i, t, d]) =>
            h('div', {class: 'check'}, h('span', {class: 'ic'}, i), h('div', {}, h('b', {}, t), h('small', {}, d)), h('span')))),
        h('div', {class: 'row mt'},
          h('button', {class: 'btn', onclick: async () => { try { const r = await api('/api/diag/bundle', {report: TR}); toast(_t('Сохранено: ') + r.file, 'ok'); } catch (e) { toast(e.message, 'err'); } },
            title: _t('Журнал, настройки, устройства и результаты тестов — приложите к сообщению об ошибке')}, _t('📦 Диагностический пакет')),
          h('button', {class: 'btn ghost', onclick: () => act('/api/open', {what: 'repo'})}, 'GitHub')))),
    testsCard(),
    sectionH(_t('Discord и другие программы')),
    guide());
}
