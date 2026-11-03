// main.js — оболочка интерфейса: меню слева, шапка, страницы, живые данные.
import {app, h, $, api, act, set, toast, setState, setRenderer, reload, clearLive, emitLive, paintMeters, keyLabel, confirmDlg,
  fmtTime, go, presetById, TOKEN, icon} from './core.js';
import soundsPage, {importFiles} from './pages/sounds.js';
import voicePage from './pages/voice.js';
import menuPage from './pages/menu.js';
import wheelPage from './pages/wheel.js';
import phonePage, {phoneSig} from './pages/phone.js';
import dotaPage from './pages/dota.js';
import settingsPage from './pages/settings.js';
import checkPage, {loadSetup} from './pages/check.js';
import scriptsPage from './pages/scripts.js';
import logPage from './pages/log.js';
import {maybeWizard} from './wizard.js';
import {syncEmoji} from './emoji.js';

const PAGES = [
  {id: 'sounds', icon: '🔊', label: 'Звуки', title: 'Звуки', render: soundsPage},
  {id: 'voice', icon: '🎙️', label: 'Голос', title: 'Голос и эффекты', render: voicePage},
  {id: 'menu', icon: '⚡', label: 'Меню в игре', title: 'Быстрое меню поверх игры', render: menuPage},
  {id: 'wheel', icon: '🎯', label: 'Колесо', title: 'Колесо звуков', render: wheelPage},
  {id: 'phone', icon: '📱', label: 'Телефон', title: 'Пульт с телефона', render: phonePage, sig: phoneSig},
  {id: 'dota', icon: '🐲', label: 'Dota 2', title: 'Модуль Dota 2', render: dotaPage},
  {id: 'settings', icon: '⚙️', label: 'Настройки', title: 'Настройки', render: settingsPage},
  {id: 'check', icon: '🩺', label: 'Проверка', title: 'Проверка и помощь', render: checkPage},
  {sep: 'Для опытных'},
  {id: 'scripts', icon: '📜', label: 'Скрипты', title: 'Скрипты', render: scriptsPage},
  {id: 'log', icon: '🧾', label: 'Журнал', title: 'Журнал', render: logPage},
];
const pageOf = id => PAGES.find(p => p.id === id) || PAGES[0];

// ---------- меню слева ----------
const nav = $('#nav');
function buildNav() {
  nav.replaceChildren(
    h('div', {class: 'brand'}, h('img', {src: 'icon.png', alt: ''}), h('div', {}, h('b', {}, 'VoiceBox'), h('small', {id: 'ver'}, ''))),
    ...PAGES.map(p => p.sep ? h('div', {class: 'navsep'}, p.sep) :
      h('button', {class: 'navbtn', 'data-page': p.id, title: p.label, onclick: () => navigate(p.id)},
        h('span', {class: 'ni'}, p.icon), h('span', {class: 'nl'}, p.label), h('span', {class: 'nb hidden'}))),
    h('div', {class: 'navfoot'},
      h('button', {class: 'navbtn', title: 'Закрыть VoiceBox полностью', onclick: quit}, h('span', {class: 'ni'}, icon('power', 17)), h('span', {class: 'nl'}, 'Выход'))));
}
function paintNav() {
  const s = app.state;
  $('#ver').textContent = s ? s.version : '';
  for (const b of nav.querySelectorAll('[data-page]')) {
    b.classList.toggle('on', b.dataset.page === app.page);
    const nb = b.querySelector('.nb');
    let badge = '';
    if (b.dataset.page === 'check' && app.setup && !app.setup.all_ok) badge = '!';
    if (b.dataset.page === 'dota' && s && !s.config.dota.enabled) badge = '';
    nb.textContent = badge;
    nb.classList.toggle('hidden', !badge);
  }
}
async function quit() {
  if (await confirmDlg('Звуки, эффекты голоса и быстрое меню перестанут работать до следующего запуска.', {title: 'Закрыть VoiceBox?', ok: 'Закрыть', danger: true})) {
    api('/api/quit', {}).catch(() => {});
    document.body.replaceChildren(h('div', {class: 'empty', style: 'margin-top:20vh'}, h('div', {class: 'big'}, '👋'), h('h2', {}, 'VoiceBox закрыт'), h('p', {}, 'Окно можно закрыть.')));
  }
}

// ---------- шапка ----------
let topSig = '';
function paintTop(force) {
  const s = app.state, st = app.status;
  $('#title').textContent = pageOf(app.page).title;
  if (!s || !st) return;
  const playing = (app.levels.playing || []).length > 0;
  const preset = presetById(st.preset);
  const g = st.game || {};
  const sig = JSON.stringify([app.offline, st.audio.running, st.audio.cable_found, st.mic_mute, st.talk, st.preset, playing,
    st.dota && g.has_clock ? [g.clock, g.paused] : 0, s.demo, preset && preset.title, st.set, st.denoise, (s.config.sets || []).length]);
  if (sig === topSig && !force) return;
  topSig = sig;
  const chips = [];
  if (app.offline) chips.push(h('span', {class: 'chip bad'}, '⚠ Нет связи с программой…'));
  if (s.demo) chips.push(h('span', {class: 'chip', title: 'VOICEBOX_DEMO: без настоящих звуковых устройств'}, '🧪 Демо'));
  if (!st.audio.running) chips.push(h('button', {class: 'chip bad', onclick: () => go('check'), title: st.audio.error || ''}, '⚠ Звук не запущен'));
  else if (!st.audio.cable_found) chips.push(h('button', {class: 'chip warn', onclick: () => go('check')}, '⚠ Нет виртуального кабеля'));
  if (st.dota && g.has_clock) chips.push(h('button', {class: 'chip live', onclick: () => go('dota'), title: 'Игровое время Dota 2'}, h('span', {class: 'dot'}), '🐲 ' + fmtTime(g.clock) + (g.paused ? ' ⏸' : '')));
  if (st.talk && st.talk.mode !== 'always') {
    const k = keyLabel(st.talk.key) || 'кнопка не задана';
    chips.push(h('button', {class: 'chip' + (st.talk.open ? ' live' : ''), onclick: () => go('voice'),
      title: st.talk.mode === 'ptt' ? 'Вас слышно, пока держите кнопку' : 'Нажмите кнопку — включить/выключить голос'},
      h('span', {class: 'dot'}), (st.talk.mode === 'ptt' ? '✊ ' : '🔘 ') + k));
  }
  if (playing) chips.push(h('button', {class: 'chip', onclick: () => act('/api/stop', {}), title: 'Остановить все звуки'}, '⏹ Стоп'));
  if (preset) chips.push(h('button', {class: 'chip', onclick: () => go('voice'), title: 'Эффект голоса'}, (preset.emoji || '🎛️') + ' ' + preset.title));
  const sets = s.config.sets || [];
  if (sets.length) {
    const cur = sets.find(x => x.id === st.set);
    chips.push(h('button', {class: 'chip', onclick: () => go('sounds'), title: 'Набор звуков: панель, меню и колесо'}, cur ? (cur.emoji || '🗂️') + ' ' + cur.name : '🎛️ Все звуки'));
  }
  chips.push(h('button', {class: 'chip' + (st.mic_mute ? ' muted' : ''), title: st.mic_mute ? 'Микрофон выключен — нажмите, чтобы включить' : 'Нажмите, чтобы выключить микрофон',
    onclick: () => set({mic_mute: !st.mic_mute}, {ok: st.mic_mute ? '🎙️ Микрофон включён' : '🔇 Микрофон выключен'})},
    st.mic_mute ? '🔇' : '🎙️', st.mic_mute ? 'Выключен' : h('span', {class: 'mini', 'data-meter': 'mic_in'}, h('i'))));
  $('#topbar').replaceChildren(...chips);
}

// ---------- страницы ----------
let pointerDown = false;
function guarded() {
  if (pointerDown) return true;
  if ($('#layer').children.length) return true;
  const a = document.activeElement;
  return !!(a && a.closest && a.closest('#page') && (a.tagName === 'TEXTAREA' || (a.tagName === 'INPUT' && /^(text|search|number)$/.test(a.type))));
}
let pageSig = null;
function render(enter) {
  if (!app.state) return;
  if (!enter && guarded()) { app.dirty = true; return; }
  app.dirty = false;
  // страница с sig перерисовывается, только когда меняется то, что она показывает (предпросмотр пульта — живой iframe)
  const pg = pageOf(app.page), sig = pg.sig ? pg.sig() : null;
  if (!enter && sig !== null && sig === pageSig) { paintNav(); paintTop(true); return; }
  pageSig = sig;
  clearLive();
  const el = $('#page');
  const scroll = el.scrollTop;
  let node;
  try { node = pageOf(app.page).render(); } catch (e) { console.error(e); node = h('div', {class: 'hint box warn'}, 'Ошибка страницы: ' + e.message); }
  el.replaceChildren(h('div', {class: 'page' + (enter ? ' enter' : '')}, node));
  el.scrollTop = enter ? 0 : scroll;
  paintNav();
  paintTop(true);
  paintMeters(app.levels);
}
setRenderer(render);
function navigate(id) {
  if (!PAGES.some(p => p.id === id)) id = 'sounds';
  app.page = id;
  if (location.hash !== '#' + id) history.replaceState(null, '', '?t=' + encodeURIComponent(TOKEN) + '#' + id);
  if (id === 'check' || id === 'dota') loadSetup();
  render(true);
}
window.addEventListener('vb-go', e => navigate(e.detail));
document.addEventListener('pointerdown', () => { pointerDown = true; }, true);
const released = () => { pointerDown = false; if (app.dirty) setTimeout(() => app.dirty && render(false), 50); };
document.addEventListener('pointerup', released, true);
document.addEventListener('pointercancel', released, true);
document.addEventListener('dragend', released, true);
document.addEventListener('drop', released, true);
document.addEventListener('focusout', () => setTimeout(() => { if (app.dirty) render(false); }, 80), true);
// боковые кнопки мыши не должны листать «назад/вперёд»
window.addEventListener('mouseup', e => { if (e.button === 3 || e.button === 4) e.preventDefault(); });
window.addEventListener('error', e => toast('Ошибка интерфейса: ' + e.message, 'err'));
window.addEventListener('unhandledrejection', e => toast('Ошибка интерфейса: ' + ((e.reason && e.reason.message) || e.reason), 'err'));

// ---------- файлы со всего окна ----------
let dragDepth = 0;
const hasFiles = e => e.dataTransfer && [...e.dataTransfer.types].includes('Files');
window.addEventListener('dragenter', e => { if (hasFiles(e)) { dragDepth++; document.body.classList.add('dropzone-over'); } });
window.addEventListener('dragleave', e => { if (hasFiles(e) && --dragDepth <= 0) { dragDepth = 0; document.body.classList.remove('dropzone-over'); } });
window.addEventListener('dragover', e => { if (hasFiles(e)) e.preventDefault(); });
window.addEventListener('drop', e => {
  if (!hasFiles(e)) return;
  e.preventDefault();
  dragDepth = 0; document.body.classList.remove('dropzone-over');
  const files = [...e.dataTransfer.files];
  if (files.length) { if (app.page !== 'sounds') navigate('sounds'); importFiles(files); }
});

// ---------- живые данные ----------
let reloadT = null;
function scheduleReload() {
  clearTimeout(reloadT);
  reloadT = setTimeout(async () => { await reload(); syncEmoji(); }, 250);
}
function connect() {
  const es = new EventSource('/api/events?t=' + encodeURIComponent(TOKEN));
  es.addEventListener('levels', e => {
    app.levels = JSON.parse(e.data);
    paintMeters(app.levels);
    emitLive('levels', app.levels);
    if (((app.levels.playing || []).length > 0) !== topPlaying) { topPlaying = !topPlaying; paintTop(); }
  });
  es.addEventListener('status', e => {
    const st = JSON.parse(e.data);
    const prev = app.status;
    app.status = st;
    if (app.offline) { app.offline = false; scheduleReload(); }
    if (app.state && st.rev !== app.state.status.rev) scheduleReload();
    paintTop();
    emitLive('status', st);
    if (prev && prev.preset !== st.preset && app.page === 'voice') render(false);
  });
  es.addEventListener('log', e => {
    const line = JSON.parse(e.data);
    if (app.state) { app.state.logs.push(line); if (app.state.logs.length > 3000) app.state.logs.splice(0, 500); }
    emitLive('log', line);
    const m = line.match(/🔔 (.*)$/);
    if (m) toast('🔔 ' + m[1]);
  });
  es.onerror = () => { if (!app.offline) { app.offline = true; paintTop(true); } };
}
let topPlaying = false;

// ---------- запуск ----------
buildNav();
(async () => {
  try {
    setState(await api('/api/state'), false);
  } catch (e) {
    $('#page').replaceChildren(h('div', {class: 'empty'}, h('div', {class: 'big'}, '⚠️'), h('h2', {}, 'Не удалось связаться с VoiceBox'), h('p', {}, e.message)));
    return;
  }
  const hp = location.hash.slice(1);
  navigate(PAGES.some(p => p.id === hp) ? hp : 'sounds');
  connect();
  loadSetup();
  syncEmoji();
  maybeWizard();
})();
