// Пульт с телефона: включить, отсканировать QR-код — и звуки, эффекты и микрофон кнопками на телефоне.
// Сам пульт — отдельный сервер в домашней сети (internal/ui/remote.go) и страница static/remote/.
import {app, h, api, toggle, seg, setrow, hint, toast, confirmDlg, onLive, reload, requestRender, select, plural} from '../core.js';

let st = null;       // GET /api/remote
let loading = null;
let sel = '';        // выбранный адрес, если сетей несколько
let lastPoll = 0;

function load() {
  if (!loading) {
    loading = api('/api/remote').then(r => { st = r; return r; }, e => { toast(e.message, 'err'); return null; }).finally(() => { loading = null; });
  }
  return loading;
}
// sig — что влияет на страницу целиком (телефоны обновляются отдельно, без перерисовки предпросмотра).
export function phoneSig() {
  const r = app.state && app.state.config.remote;
  return JSON.stringify([r, st && [st.running, st.port, st.error, st.local, (st.urls || []).map(u => u.url)], sel]);
}

async function setRemote(patch, ok) {
  const next = {...app.state.config.remote, ...patch};
  try {
    st = await api('/api/remote', {remote: next});
    if (ok) toast(ok, 'ok');
  } catch (e) {
    toast(e.message, 'err');
  }
  await reload();
}
async function call(path, body, ok) {
  try {
    const r = await api(path, body);
    if (r && r.settings) st = r;
    if (ok) toast(ok, 'ok');
    return true;
  } catch (e) {
    toast(e.message, 'err');
    return false;
  }
}

export default function phonePage() {
  const r = app.state.config.remote;
  if (!st) {
    load().then(() => requestRender());
    return h('div', {class: 'hint', style: 'padding:20px'}, 'Загрузка…');
  }
  onLive('status', () => {
    if (Date.now() - lastPoll < 2000) return;
    lastPoll = Date.now();
    const before = phoneSig();
    load().then(x => {
      if (!x) return;
      if (phoneSig() !== before) { requestRender(); return; }
      const el = document.getElementById('phDev');
      if (el) { const n = devicesCard(); if (n.dataset.sig !== el.dataset.sig) el.replaceWith(n); }
    });
  });
  const main = h('div', {class: 'card'},
    setrow('Пульт с телефона', 'Звуки, эффекты голоса и микрофон — кнопками на экране телефона. По домашнему Wi-Fi, без приложений и интернета.',
      toggle(r.enabled, v => setRemote({enabled: v}, v ? '📱 Пульт включён — отсканируйте QR-код' : 'Пульт выключен'), {big: true})),
    r.enabled && st.error ? hint(['⚠ ', st.error], 'warn') : null);
  if (!r.enabled) {
    return h('div', {class: 'grid2', style: 'grid-template-columns:minmax(380px,1.15fr) minmax(330px,.85fr)'},
      h('div', {class: 'stack'}, main, introCard()),
      h('div', {style: 'position:sticky;top:0'}, previewCard()));
  }
  return h('div', {class: 'grid2', style: 'grid-template-columns:minmax(380px,1.15fr) minmax(330px,.85fr)'},
    h('div', {class: 'stack'}, main, st.running ? qrCard() : null, devicesCard(), contentCard(r), accessCard(), helpCard(r)),
    h('div', {style: 'position:sticky;top:0'}, previewCard()));
}

function introCard() {
  const f = (t, s) => h('div', {}, h('b', {}, t), h('small', {}, s));
  return h('div', {class: 'card'}, h('h3', {}, 'Как это работает'),
    h('div', {class: 'feat'},
      f('📷 QR-код — и готово', 'Наведите камеру телефона на код — пульт откроется в браузере. Ставить ничего не нужно.'),
      f('⚡ Как клавиши', 'Нажали плитку — звук сразу в эфире. Видно, что играет, можно остановить.'),
      f('🏠 Только дома', 'Работает в вашей сети Wi-Fi. Из интернета пульт недоступен.'),
      f('🔒 По секретной ссылке', 'Без ссылки с ключом пульт не открыть. Ссылку можно в любой момент сменить.')));
}

// ---------- QR-код ----------
function copyText(text) {
  const done = () => toast('📋 Ссылка скопирована', 'ok');
  if (navigator.clipboard && window.isSecureContext) { navigator.clipboard.writeText(text).then(done, () => fallback()); return; }
  fallback();
  function fallback() {
    const t = h('textarea', {style: 'position:fixed;opacity:0'}, text);
    document.body.append(t);
    t.select();
    try { document.execCommand('copy'); done(); } catch (e) { toast('Не удалось скопировать', 'err'); }
    t.remove();
  }
}
function qrCard() {
  const urls = st.urls || [];
  if (!urls.length) {
    return h('div', {class: 'card'}, hint(['⚠ У компьютера нет адреса в домашней сети — телефону не к чему подключиться. ',
      'Подключите компьютер к Wi-Fi или кабелем к роутеру.'], 'warn'));
  }
  const u = urls.find(x => x.ip === sel) || urls[0];
  return h('div', {class: 'card', id: 'phQR'},
    h('div', {class: 'ph-qrrow'},
      h('div', {class: 'ph-qr', innerHTML: u.qr || '', title: u.url}),
      h('div', {class: 'ph-how'},
        h('h3', {}, 'Откройте на телефоне'),
        h('ol', {},
          h('li', {}, 'Подключите телефон к ', h('b', {}, 'тому же Wi-Fi'), ', что и компьютер.'),
          h('li', {}, 'Откройте камеру и наведите на код.'),
          h('li', {}, 'Нажмите на появившуюся ссылку — откроется пульт.')),
        (st.devices || []).length ? null : h('p', {class: 'hint', style: 'margin:8px 0 0'},
          'Windows может спросить, пускать ли VoiceBox в сеть, — нажмите «Разрешить доступ» (хватит частных сетей).'))),
    h('div', {class: 'row nowrap ph-url'},
      h('input', {class: 'input', readOnly: true, value: u.url, onclick: e => e.target.select(), title: 'Эту ссылку можно открыть и вручную'}),
      h('button', {class: 'btn', onclick: () => copyText(u.url)}, 'Копировать')),
    u.vpn ? h('div', {style: 'margin-top:10px'}, hint(['⚠ Похоже, это адрес VPN — с телефона по нему, скорее всего, не открыть. ',
      'Выключите VPN на компьютере или разрешите в его настройках доступ к локальной сети.'], 'warn')) : null,
    urls.length > 1 ? setrow('Сеть', 'Не открывается — попробуйте другой адрес',
      select(urls.map(x => [x.ip, x.ip + ' · ' + x.iface + (x.primary ? ' (основная)' : x.vpn ? ' (VPN — с телефона вряд ли откроется)' : '')]), u.ip, v => { sel = v; requestRender(); })) : null,
    h('p', {class: 'hint', style: 'margin:10px 0 0'}, '📌 Чтобы пульт был под рукой, добавьте его на главный экран: iPhone — «Поделиться» → «На экран „Домой“», Android — меню браузера (три точки) → «Добавить на главный экран».'));
}

// ---------- телефоны ----------
function ago(t) {
  const s = Math.max(0, (Date.now() - new Date(t).getTime()) / 1000);
  if (s < 60) return 'только что';
  if (s < 3600) { const m = Math.floor(s / 60); return m + ' ' + plural(m, 'минуту', 'минуты', 'минут') + ' назад'; }
  if (s < 86400) { const n = Math.floor(s / 3600); return n + ' ' + plural(n, 'час', 'часа', 'часов') + ' назад'; }
  return new Date(t).toLocaleDateString('ru-RU', {day: 'numeric', month: 'long'});
}
function devicesCard() {
  const list = st.devices || [];
  const on = list.filter(d => d.online).length;
  const rows = list.map(d => {
    const phone = /iPhone|iPad|Android/.test(d.name || '');
    return h('div', {class: 'ph-dev' + (d.online ? ' on' : '')},
      h('span', {class: 'ph-de'}, phone ? '📱' : '💻'),
      h('div', {class: 'grow', style: 'min-width:0'}, h('b', {class: 'ell', style: 'display:block'}, d.name || 'Устройство'), h('small', {}, d.ip)),
      h('span', {class: 'ph-st'}, d.online ? [h('i'), 'на связи'] : ago(d.seen)));
  });
  const el = h('div', {class: 'card', id: 'phDev'},
    h('div', {class: 'card-h'}, h('h3', {}, '📲 Телефоны'), h('div', {class: 'grow'}),
      on ? h('span', {class: 'badge ok'}, 'на связи: ' + on) : null),
    rows.length ? h('div', {class: 'ph-devs'}, rows)
      : h('p', {class: 'hint', style: 'margin:0'}, st.running ? 'Пока никто не подключался. Отсканируйте QR-код телефоном — он появится здесь.' : 'Пульт не запущен.'));
  el.dataset.sig = JSON.stringify(list.map(d => [d.ip, d.name, d.online, d.online ? '' : ago(d.seen)]));
  return el;
}

// ---------- что на пульте ----------
function contentCard(r) {
  return h('div', {class: 'card'}, h('h3', {}, '🎛️ Что на пульте'),
    setrow('Звуки', r.sounds === 'menu' ? 'Только звуки из быстрого меню, в том же порядке' : 'Все плитки со страницы «Звуки», по группам',
      seg([['pad', 'Все с панели'], ['menu', 'Как в быстром меню']], r.sounds || 'pad', v => setRemote({sounds: v}))),
    setrow('Вкладка «Голос»', 'Переключать эффекты голоса с телефона', toggle(r.show_effects, v => setRemote({show_effects: v}))),
    setrow('Кнопка микрофона', 'Выключать и включать микрофон с телефона', toggle(r.show_mic, v => setRemote({show_mic: v}))));
}

// ---------- доступ ----------
function accessCard() {
  return h('div', {class: 'card'}, h('h3', {}, '🔒 Доступ'),
    h('p', {class: 'hint', style: 'margin:0 0 10px'}, 'Пульт открывается только из вашей домашней сети и только по ссылке с секретным ключом — это как пароль. ',
      'Не показывайте QR-код на стриме и не пересылайте ссылку. Попала не туда — сделайте новую: старые ссылки и QR-коды сразу перестанут работать.'),
    h('button', {class: 'btn', onclick: async () => {
      if (!await confirmDlg('Старая ссылка и QR-код перестанут работать — на телефоне нужно будет отсканировать новый код.', {title: 'Сделать новую ссылку?', ok: 'Сделать новую'})) return;
      if (await call('/api/remote/reset', {}, '🔑 Новая ссылка готова — отсканируйте код ещё раз')) await reload();
    }}, '🔑 Новая ссылка'));
}

// ---------- помощь ----------
function helpCard(r) {
  const port = st.port || (app.state.config.ui.port || 3001) + 1;
  const portIn = h('input', {class: 'input sm', type: 'number', min: 1024, max: 65535, value: r.port || '', placeholder: String(port), style: 'width:96px'});
  const step = (n, title, text, ...btns) => h('div', {class: 'ph-fix'}, h('span', {class: 'wh-n'}, n),
    h('div', {class: 'grow'}, h('b', {}, title), h('small', {}, text), btns.length ? h('div', {class: 'row', style: 'margin-top:8px'}, btns) : null));
  return h('div', {class: 'card'}, h('h3', {}, '🛠️ Телефон не открывает пульт?'),
    step(1, 'Одна сеть', 'Телефон — в том же Wi-Fi, что и компьютер: не мобильный интернет и не гостевая сеть роутера. Компьютер может быть подключён к этому роутеру кабелем.'),
    step(2, 'Сеть Windows — «Частная»', 'В «Общедоступной» сети Windows не пускает к компьютеру другие устройства. Откройте свойства своей сети и выберите «Частная сеть».',
      h('button', {class: 'btn sm', onclick: () => call('/api/remote/open', {what: 'network'})}, 'Открыть параметры сети')),
    step(3, 'Брандмауэр', ['Кнопка добавит одно правило: только VoiceBox, только порт ' + port + ', только частные сети и только устройства из вашей сети. Windows попросит подтверждение. ',
      'Если когда-то нажали «Отмена» в окне брандмауэра — в списке программ поставьте у VoiceBox галочку «Частная».'],
      h('button', {class: 'btn sm', onclick: async () => {
        toast('Подтвердите в окне Windows…');
        await call('/api/remote/firewall', {}, '✅ Разрешено. Обновите страницу на телефоне');
      }}, '🛡️ Разрешить в брандмауэре'),
      h('button', {class: 'btn sm ghost', onclick: () => call('/api/remote/open', {what: 'firewall_apps'})}, 'Список программ')),
    step(4, 'VPN', 'VPN на компьютере или телефоне может не пускать в домашнюю сеть — на время выключите его.'),
    step(5, 'Порт', 'Если порт занят другой программой — поменяйте (1024–65535). Пусто — ' + ((app.state.config.ui.port || 3001) + 1) + '.',
      portIn, h('button', {class: 'btn sm', onclick: () => {
        const v = portIn.value.trim() === '' ? 0 : Math.round(+portIn.value);
        if (v && (v < 1024 || v > 65535)) { toast('Порт — число от 1024 до 65535', 'err'); return; }
        setRemote({port: v}, 'Порт пульта: ' + (v || 'автоматически') + '. QR-код обновился');
      }}, 'Сохранить')));
}

// ---------- предпросмотр ----------
function previewCard() {
  const ok = st && st.running && st.local;
  let frame = null;
  return h('div', {class: 'card', id: 'phPrev'},
    h('div', {class: 'card-h'}, h('h3', {}, 'Так выглядит на телефоне'), h('div', {class: 'grow'}),
      ok ? h('button', {class: 'btn sm ghost', title: 'Обновить', onclick: () => { if (frame) frame.src = st.local; }}, '↻') : null),
    h('div', {class: 'ph-frame'},
      ok ? h('iframe', {src: st.local, title: 'Пульт с телефона', ref: el => { frame = el; }})
        : h('div', {class: 'ph-off'}, h('div', {class: 'big'}, '📱'), h('b', {}, 'Пульт выключен'), h('small', {}, 'Включите — здесь появится то, что увидите на телефоне.'))),
    ok ? h('p', {class: 'hint', style: 'margin:10px 0 0;text-align:center'}, 'Это настоящий пульт: нажатия здесь тоже работают.') : null);
}
