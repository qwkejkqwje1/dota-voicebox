// wizard.js — мастер первого запуска: что это, устройства и кабель, Dota 2, программы, итог.
import {app, h, api, act, set, toast, reload, dialog, select, kbd, cableRec, cableIn, shortDev, isVirt, clone, meter, go, hint} from './core.js';
import {fix, loadSetup, autoSetup} from './pages/check.js';

let shown = false;
// maybeWizard — показать мастер один раз за запуск, если первичная настройка ещё не пройдена.
export function maybeWizard() {
  if (shown || !app.state || app.state.config.setup_done) return;
  openWizard();
}

const TITLES = ['Добро пожаловать в VoiceBox', 'Микрофон, наушники и кабель', 'Вы играете в Dota 2?', 'Выберите микрофон в программах', 'Всё готово'];

export function openWizard() {
  shown = true;
  let step = 0;
  let busy = false;
  const c = () => app.state.config;
  const keyOf = a => (app.state.hotkeys || {})[a] || '';
  const refresh = async () => { await reload(false); d.update(); };

  const dev = () => {
    const dv = app.state.devices || {capture: [], playback: []};
    const opt = (list, auto) => [['', auto]].concat((list || []).map(x => [x.name, x.name + (x.is_default ? ' · по умолчанию' : '')]));
    const real = l => (l || []).filter(x => !isVirt(x.name));
    const virt = l => (l || []).filter(x => isVirt(x.name));
    const play = dv.playback || [];
    const st = app.status || app.state.status;
    const cableOk = !!(dv.cable || c().devices.voice_out);
    return [
      h('p', {class: 'dim', style: 'margin:0 0 14px'}, 'Проверьте, что выбраны нужные устройства. Обычно подходит «Авто».'),
      h('div', {class: 'col', style: 'gap:12px'},
        h('label', {class: 'field'}, h('span', {}, '🎙️ Ваш микрофон'),
          select(opt(real(dv.capture), 'Авто — микрофон по умолчанию'), c().devices.mic, async v => { await set({mic: v}, {render: false}); refresh(); })),
        h('div', {class: 'row nowrap', style: 'gap:10px'}, h('span', {class: 'hint', style: 'white-space:nowrap'}, 'Скажите что-нибудь:'), meter('mic_in', {big: true})),
        h('label', {class: 'field'}, h('span', {}, '🎧 Наушники — здесь звуки слышите вы'),
          h('div', {class: 'row nowrap'},
            select(opt(real(play), 'Авто — устройство по умолчанию'), c().devices.monitor, async v => { await set({monitor: v}, {render: false}); refresh(); }),
            h('button', {class: 'btn', title: 'Короткий сигнал только вам', onclick: async () => {
              try { const r = await api('/api/test', {id: 'monitor', sec: 1}); toast(r.summary || 'Сигнал отправлен', r.verdict === 'fail' ? 'err' : 'ok'); } catch (e) { toast(e.message, 'err'); }
            }}, '🔔 Проверить'))),
        cableOk
          ? h('div', {class: 'hint box accent'}, '🔌 Виртуальный кабель найден: ', h('b', {}, shortDev(c().devices.voice_out || cableIn())),
            '. Всё, что идёт «в эфир», попадает в него, а программы слушают его другой конец — ', h('b', {}, shortDev(cableRec())), '.')
          : h('div', {class: 'hint box warn'},
            h('div', {}, h('b', {}, 'Нужен виртуальный кабель. '), 'Это бесплатный драйвер VB-Cable: VoiceBox отправляет в него голос и звуки, а Discord и игра берут их как микрофон.'),
            h('div', {class: 'row mt-s'},
              h('button', {class: 'btn primary', disabled: busy, onclick: async () => { busy = true; d.update(); await fix('cable'); busy = false; await refresh(); }}, busy ? 'Устанавливаю…' : '⬇️ Установить VB-Cable'),
              h('button', {class: 'btn ghost', onclick: () => act('/api/open', {what: 'cable'})}, 'Сайт VB-Audio')),
            h('label', {class: 'field mt-s'}, h('span', {}, 'Уже есть другой кабель (Voicemeeter, «Line 1»)? Выберите его:'),
              select(opt(virt(play).concat(real(play)), 'Не выбран'), c().devices.voice_out, async v => { await set({voice_out: v}, {render: false}); refresh(); }))),
        st && st.audio && !st.audio.running ? hint('⚠ Звук не запущен: ' + (st.audio.error || 'проверьте устройства'), 'warn') : null)];
  };

  const dota = () => {
    const on = c().dota.enabled;
    const pick = async v => {
      if (v !== on) await act('/api/dota', {dota: Object.assign(clone(c().dota), {enabled: v})}, {render: false});
      loadSetup();
      d.update();
    };
    return [
      h('p', {class: 'dim', style: 'margin:0 0 14px'}, 'Для Dota 2 есть отдельный модуль. Его можно включить или выключить в любой момент на странице «Dota 2».'),
      h('div', {class: 'choice'},
        h('button', {type: 'button', class: on ? 'on' : '', onclick: () => pick(true)}, h('span', {class: 'ce'}, '🐲'), h('b', {}, 'Да, играю'), h('small', {class: 'dim', style: 'display:block'}, 'Включить модуль Dota 2')),
        h('button', {type: 'button', class: on ? '' : 'on', onclick: () => pick(false)}, h('span', {class: 'ce'}, '🎧'), h('b', {}, 'Нет / другие игры'), h('small', {class: 'dim', style: 'display:block'}, 'Только звуки и голос'))),
      h('div', {class: 'feat three mt'},
        h('div', {}, h('b', {}, '🚨 Ганк на мид!'), h('small', {}, 'Одна клавиша — тревога и голосовое предупреждение команде.')),
        h('div', {}, h('b', {}, '⏰ Руны и таймеры'), h('small', {}, 'Голосом заранее: руны силы и богатства, мудрость каждые 7 минут, лотосы.')),
        h('div', {}, h('b', {}, '🛡️ Рошан'), h('small', {}, 'Нажмите «Рошан убит» — VoiceBox напомнит про Аегис и возрождение.'))),
      on ? h('div', {class: 'hint box accent mt'},
        h('div', {}, 'Чтобы таймеры шли по игровому времени, VoiceBox подключается к игре (Game State Integration). Это безопасно и разрешено Valve.'),
        h('div', {class: 'row mt-s'}, h('button', {class: 'btn sm primary', onclick: async () => { await autoSetup(); d.update(); }}, '⚙️ Подключить к игре'),
          h('span', {class: 'hint'}, 'Можно сделать позже на странице «Dota 2».'))) : null];
  };

  const apps = () => {
    const rec = shortDev(cableRec());
    const step = (n, ...t) => h('div', {class: 'check'}, h('span', {class: 'ic'}, n), h('div', {style: 'min-width:0'}, t), h('span'));
    return [
      h('p', {class: 'dim', style: 'margin:0 0 12px'}, ['В каждой программе, где вас должны слышать, выберите микрофон ', h('b', {}, rec), '. Это нужно сделать один раз.']),
      h('div', {class: 'card checks', style: 'padding:4px 16px'},
        step('💬', h('b', {}, 'Discord'), h('small', {}, ['Настройки → Голос и видео → Устройство ввода: ', h('b', {}, rec), '. Шумоподавление Krisp лучше выключить — оно может глушить звуки.']),
          h('div', {class: 'mt-s'}, h('button', {class: 'btn sm', onclick: () => act('/api/open', {what: 'discord_voice'})}, 'Открыть настройки Discord'))),
        c().dota.enabled ? step('🐲', h('b', {}, 'Dota 2'), h('small', {}, ['Настройки → Звук → Устройство ввода: ', h('b', {}, rec), '.'])) : null,
        step('🎥', h('b', {}, 'OBS, TeamSpeak, другие игры'), h('small', {}, ['Источник звука / микрофон: ', h('b', {}, rec), '.']))),
      h('p', {class: 'hint mt'}, 'Свой голос в этих программах вы не услышите — чтобы проверить, как звучите, есть кнопка «Слышать себя» на странице «Голос» и тесты на странице «Проверка».')];
  };

  const done = () => {
    const row = (key, text) => h('div', {class: 'setrow'}, h('div', {class: 'txt'}, h('b', {}, text)), h('div', {class: 'ctl'}, kbd(key)));
    return [
      h('div', {class: 'empty', style: 'padding:6px 0 14px'}, h('div', {class: 'big'}, '🎉'), h('p', {style: 'margin:0'}, 'VoiceBox работает в фоне — значок в трее рядом с часами. Закрытие окна программу не выключает.')),
      h('div', {class: 'card', style: 'padding:4px 16px'},
        row(keyOf('menu'), '⚡ Быстрое меню поверх игры'),
        row(keyOf('stop'), '⏹ Остановить все звуки'),
        row(keyOf('ui'), '🪟 Открыть это окно'),
        c().dota.enabled && keyOf('sound:dota_gank') ? row(keyOf('sound:dota_gank'), '🚨 Ганк на мид!') : null),
      h('p', {class: 'hint mt'}, 'Клавиши можно поменять в «Настройках», звуки — добавить перетаскиванием файлов прямо в окно.')];
  };

  const welcome = () => [
    h('p', {style: 'margin:0 0 14px;font-size:15px'}, 'Саундпад и эффекты голоса для Discord и игр. Звуки и ваш голос с эффектом идут в виртуальный микрофон — их слышат все, с кем вы говорите.'),
    h('div', {class: 'flow'}, h('span', {}, '🎙️ Микрофон'), h('em', {}, '→'), h('span', {class: 'acc'}, '🎛️ VoiceBox'), h('em', {}, '→'), h('span', {}, '🔌 Кабель'), h('em', {}, '→'), h('span', {}, '💬 Discord · 🎮 игра')),
    h('div', {class: 'feat'},
      h('div', {}, h('b', {}, '🔊 Звуки'), h('small', {}, 'Кнопки и горячие клавиши. Свои файлы, озвучка фраз, готовые звуки.')),
      h('div', {}, h('b', {}, '📻 Эффекты голоса'), h('small', {}, 'Рация, радио, робот, демон, мегафон и другие — с настройкой.')),
      h('div', {}, h('b', {}, '⚡ Меню поверх игры'), h('small', {}, ['Нажмите ', kbd(keyOf('menu')), ' — выберите звук, не сворачивая игру.'])),
      h('div', {}, h('b', {}, '🐲 Dota 2 — по желанию'), h('small', {}, 'Ганк на мид, голосовые таймеры рун, Рошан.')))];

  const VIEWS = [welcome, dev, dota, apps, done];
  const last = VIEWS.length - 1;
  const goStep = n => { step = Math.max(0, Math.min(last, n)); d.title(TITLES[step]); d.update(); if (step === 1) reload(false).then(() => d.update()); };

  const d = dialog({
    cls: 'wizard', title: TITLES[0],
    onClose: () => { if (!c().setup_done) set({setup_done: true}, {render: true}); },
    body: () => [h('div', {class: 'steps'}, VIEWS.map((_, i) => h('i', {class: i <= step ? 'on' : ''}))), VIEWS[step]()],
    foot: () => [
      step === 0 ? h('button', {class: 'btn ghost', onclick: () => d.close()}, 'Пропустить') : h('button', {class: 'btn', onclick: () => goStep(step - 1)}, '← Назад'),
      h('div', {class: 'grow'}),
      h('span', {class: 'hint'}, (step + 1) + ' из ' + VIEWS.length),
      step < last ? h('button', {class: 'btn primary', autofocus: true, onclick: () => goStep(step + 1)}, step === 0 ? 'Начать настройку' : 'Далее →')
        : h('button', {class: 'btn primary', onclick: () => { d.close(); go('sounds'); }}, 'Начать пользоваться')],
  });
}
