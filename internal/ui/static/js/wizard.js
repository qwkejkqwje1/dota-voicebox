// wizard.js — мастер первого запуска: что это, устройства и кабель, Dota 2, программы, итог.
import {_t, EN} from './i18n.js';
import {app, h, api, act, set, toast, reload, dialog, select, kbd, cableRec, cableIn, shortDev, isVirt, clone, meter, go, hint, setrow, toggle} from './core.js';
import {fix, loadSetup, autoSetup} from './pages/check.js';
import {setShortcut, setAutostart, setLang} from './pages/settings.js';

let shown = false;
// maybeWizard — показать мастер один раз за запуск, если первичная настройка ещё не пройдена.
export function maybeWizard() {
  if (shown || !app.state || app.state.config.setup_done) return;
  openWizard();
}

const TITLES = [_t('Добро пожаловать в VoiceBox'), _t('Микрофон, наушники и кабель'), _t('Вы играете в Dota 2?'), _t('Выберите микрофон в программах'), _t('Всё готово')];

export function openWizard() {
  shown = true;
  let step = 0;
  let busy = false;
  const c = () => app.state.config;
  const keyOf = a => (app.state.hotkeys || {})[a] || '';
  const refresh = async () => { await reload(false); d.update(); };

  const dev = () => {
    const dv = app.state.devices || {capture: [], playback: []};
    const opt = (list, auto) => [['', auto]].concat((list || []).map(x => [x.name, x.name + (x.is_default ? _t(' · по умолчанию') : '')]));
    const real = l => (l || []).filter(x => !isVirt(x.name));
    const virt = l => (l || []).filter(x => isVirt(x.name));
    const play = dv.playback || [];
    const st = app.status || app.state.status;
    const cableOk = !!(dv.cable || c().devices.voice_out);
    return [
      h('p', {class: 'dim', style: 'margin:0 0 14px'}, _t('Проверьте, что выбраны нужные устройства. Обычно подходит «Авто».')),
      h('div', {class: 'col', style: 'gap:12px'},
        h('label', {class: 'field'}, h('span', {}, _t('🎙️ Ваш микрофон')),
          select(opt(real(dv.capture), _t('Авто — микрофон по умолчанию')), c().devices.mic, async v => { await set({mic: v}, {render: false}); refresh(); })),
        h('div', {class: 'row nowrap', style: 'gap:10px'}, h('span', {class: 'hint', style: 'white-space:nowrap'}, _t('Скажите что-нибудь:')), meter('mic_in', {big: true})),
        h('label', {class: 'field'}, h('span', {}, _t('🎧 Наушники — здесь звуки слышите вы')),
          h('div', {class: 'row nowrap'},
            select(opt(real(play), _t('Авто — устройство по умолчанию')), c().devices.monitor, async v => { await set({monitor: v}, {render: false}); refresh(); }),
            h('button', {class: 'btn', title: _t('Короткий сигнал только вам'), onclick: async () => {
              try { const r = await api('/api/test', {id: 'monitor', sec: 1}); toast(r.summary || _t('Сигнал отправлен'), r.verdict === 'fail' ? 'err' : 'ok'); } catch (e) { toast(e.message, 'err'); }
            }}, _t('🔔 Проверить')))),
        cableOk
          ? h('div', {class: 'hint box accent'}, _t('🔌 Виртуальный кабель найден: '), h('b', {}, shortDev(c().devices.voice_out || cableIn())),
            _t('. Всё, что идёт «в эфир», попадает в него, а программы слушают его другой конец — '), h('b', {}, shortDev(cableRec())), '.')
          : h('div', {class: 'hint box warn'},
            h('div', {}, h('b', {}, _t('Нужен виртуальный кабель. ')), _t('Это бесплатный драйвер VB-Cable: VoiceBox отправляет в него голос и звуки, а Discord и игра берут их как микрофон.')),
            h('div', {class: 'row mt-s'},
              h('button', {class: 'btn primary', disabled: busy, onclick: async () => { busy = true; d.update(); await fix('cable'); busy = false; await refresh(); }}, busy ? _t('Устанавливаю…') : _t('⬇️ Установить VB-Cable')),
              h('button', {class: 'btn ghost', onclick: () => act('/api/open', {what: 'cable'})}, _t('Сайт VB-Audio'))),
            h('label', {class: 'field mt-s'}, h('span', {}, _t('Уже есть другой кабель (Voicemeeter, «Line 1»)? Выберите его:')),
              select(opt(virt(play).concat(real(play)), _t('Не выбран')), c().devices.voice_out, async v => { await set({voice_out: v}, {render: false}); refresh(); }))),
        st && st.audio && !st.audio.running ? hint(_t('⚠ Звук не запущен: ') + (st.audio.error || _t('проверьте устройства')), 'warn') : null)];
  };

  const dota = () => {
    const on = c().dota.enabled;
    const pick = async v => {
      if (v !== on) await act('/api/dota', {dota: Object.assign(clone(c().dota), {enabled: v})}, {render: false});
      loadSetup();
      d.update();
    };
    return [
      h('p', {class: 'dim', style: 'margin:0 0 14px'}, _t('Для Dota 2 есть отдельный модуль. Его можно включить или выключить в любой момент на странице «Dota 2».')),
      h('div', {class: 'choice'},
        h('button', {type: 'button', class: on ? 'on' : '', onclick: () => pick(true)}, h('span', {class: 'ce'}, '🐲'), h('b', {}, _t('Да, играю')), h('small', {class: 'dim', style: 'display:block'}, _t('Включить модуль Dota 2'))),
        h('button', {type: 'button', class: on ? '' : 'on', onclick: () => pick(false)}, h('span', {class: 'ce'}, '🎧'), h('b', {}, _t('Нет / другие игры')), h('small', {class: 'dim', style: 'display:block'}, _t('Только звуки и голос')))),
      h('div', {class: 'feat three mt'},
        h('div', {}, h('b', {}, _t('🚨 Ганк на мид!')), h('small', {}, _t('Одна клавиша — тревога и голосовое предупреждение команде.'))),
        h('div', {}, h('b', {}, _t('⏰ Руны и таймеры')), h('small', {}, _t('Голосом заранее: руны силы и богатства, мудрость каждые 7 минут, лотосы.'))),
        h('div', {}, h('b', {}, _t('🛡️ Рошан')), h('small', {}, _t('Нажмите «Рошан убит» — VoiceBox напомнит про Аегис и возрождение.')))),
      on ? h('div', {class: 'hint box accent mt'},
        h('div', {}, _t('Чтобы таймеры шли по игровому времени, VoiceBox подключается к игре (Game State Integration). Это безопасно и разрешено Valve.')),
        h('div', {class: 'row mt-s'}, h('button', {class: 'btn sm primary', onclick: async () => { await autoSetup(); d.update(); }}, _t('⚙️ Подключить к игре')),
          h('span', {class: 'hint'}, _t('Можно сделать позже на странице «Dota 2».')))) : null];
  };

  const apps = () => {
    const rec = shortDev(cableRec());
    const step = (n, ...t) => h('div', {class: 'check'}, h('span', {class: 'ic'}, n), h('div', {style: 'min-width:0'}, t), h('span'));
    return [
      h('p', {class: 'dim', style: 'margin:0 0 12px'}, [_t('В каждой программе, где вас должны слышать, выберите микрофон '), h('b', {}, rec), _t('. Это нужно сделать один раз.')]),
      h('div', {class: 'card checks', style: 'padding:4px 16px'},
        step('💬', h('b', {}, 'Discord'), h('small', {}, [_t('Настройки → Голос и видео → Устройство ввода: '), h('b', {}, rec), _t('. Шумоподавление Krisp лучше выключить — оно может глушить звуки.')]),
          h('div', {class: 'mt-s'}, h('button', {class: 'btn sm', onclick: () => act('/api/open', {what: 'discord_voice'})}, _t('Открыть настройки Discord')))),
        c().dota.enabled ? step('🐲', h('b', {}, 'Dota 2'), h('small', {}, [_t('Настройки → Звук → Устройство ввода: '), h('b', {}, rec), '.'])) : null,
        step('🎥', h('b', {}, _t('OBS, TeamSpeak, другие игры')), h('small', {}, [_t('Источник звука / микрофон: '), h('b', {}, rec), '.']))),
      h('p', {class: 'hint mt'}, _t('Свой голос в этих программах вы не услышите — чтобы проверить, как звучите, есть кнопка «Слышать себя» на странице «Голос» и тесты на странице «Проверка».'))];
  };

  const done = () => {
    const row = (key, text) => h('div', {class: 'setrow'}, h('div', {class: 'txt'}, h('b', {}, text)), h('div', {class: 'ctl'}, kbd(key)));
    return [
      h('div', {class: 'empty', style: 'padding:6px 0 14px'}, h('div', {class: 'big'}, '🎉'), h('p', {style: 'margin:0'}, _t('VoiceBox работает в фоне — значок в трее рядом с часами. Закрытие окна программу не выключает.'))),
      h('div', {class: 'card', style: 'padding:4px 16px'},
        row(keyOf('menu'), _t('⚡ Быстрое меню поверх игры')),
        row(keyOf('stop'), _t('⏹ Остановить все звуки')),
        row(keyOf('ui'), _t('🪟 Открыть это окно')),
        c().dota.enabled && keyOf('sound:dota_gank') ? row(keyOf('sound:dota_gank'), _t('🚨 Ганк на мид!')) : null),
      h('div', {class: 'card mt', style: 'padding:4px 16px'},
        setrow(_t('📌 Ярлык в меню «Пуск»'), _t('Чтобы находить VoiceBox поиском Windows'), toggle(!!(app.state.shortcuts || {}).start, async v => { await setShortcut('start', v); d.update(); })),
        setrow(_t('🚀 Запускать вместе с Windows'), _t('В фоне, без окна'), toggle(!!app.state.autostart, async v => { await setAutostart(v); d.update(); }))),
      h('p', {class: 'hint mt'}, _t('Клавиши можно поменять в «Настройках», звуки — добавить перетаскиванием файлов прямо в окно.'))];
  };

  const welcome = () => [
    h('p', {style: 'margin:0 0 14px;font-size:15px'}, _t('Саундпад и эффекты голоса для Discord и игр. Звуки и ваш голос с эффектом идут в виртуальный микрофон — их слышат все, с кем вы говорите.')),
    h('div', {class: 'flow'}, h('span', {}, _t('🎙️ Микрофон')), h('em', {}, '→'), h('span', {class: 'acc'}, '🎛️ VoiceBox'), h('em', {}, '→'), h('span', {}, _t('🔌 Кабель')), h('em', {}, '→'), h('span', {}, _t('💬 Discord · 🎮 игра'))),
    h('div', {class: 'feat'},
      h('div', {}, h('b', {}, _t('🔊 Звуки')), h('small', {}, _t('Кнопки и горячие клавиши. Свои файлы, озвучка фраз, готовые звуки.'))),
      h('div', {}, h('b', {}, _t('📻 Эффекты голоса')), h('small', {}, _t('Рация, радио, робот, демон, мегафон и другие — с настройкой.'))),
      h('div', {}, h('b', {}, _t('⚡ Меню поверх игры')), h('small', {}, [_t('Нажмите '), kbd(keyOf('menu')), _t(' — выберите звук, не сворачивая игру.')])),
      h('div', {}, h('b', {}, _t('🐲 Dota 2 — по желанию')), h('small', {}, _t('Ганк на мид, голосовые таймеры рун, Рошан.')))),
    h('div', {class: 'row mt', style: 'gap:8px'}, h('span', {class: 'hint'}, '🌐 Язык · Language'),
      h('button', {class: 'btn sm' + (EN ? ' ghost' : ' primary'), onclick: () => EN && setLang('ru')}, 'Русский'),
      h('button', {class: 'btn sm' + (EN ? ' primary' : ' ghost'), onclick: () => !EN && setLang('en')}, 'English'))];

  const VIEWS = [welcome, dev, dota, apps, done];
  const last = VIEWS.length - 1;
  const goStep = n => { step = Math.max(0, Math.min(last, n)); d.title(TITLES[step]); d.update(); if (step === 1) reload(false).then(() => d.update()); };

  const d = dialog({
    cls: 'wizard', title: TITLES[0],
    onClose: () => { if (!c().setup_done) set({setup_done: true}, {render: true}); },
    body: () => [h('div', {class: 'steps'}, VIEWS.map((_, i) => h('i', {class: i <= step ? 'on' : ''}))), VIEWS[step]()],
    foot: () => [
      step === 0 ? h('button', {class: 'btn ghost', onclick: () => d.close()}, _t('Пропустить')) : h('button', {class: 'btn', onclick: () => goStep(step - 1)}, _t('← Назад')),
      h('div', {class: 'grow'}),
      h('span', {class: 'hint'}, (step + 1) + _t(' из ') + VIEWS.length),
      step < last ? h('button', {class: 'btn primary', autofocus: true, onclick: () => goStep(step + 1)}, step === 0 ? _t('Начать настройку') : _t('Далее →'))
        : h('button', {class: 'btn primary', onclick: () => { d.close(); go('sounds'); }}, _t('Начать пользоваться'))],
  });
}
