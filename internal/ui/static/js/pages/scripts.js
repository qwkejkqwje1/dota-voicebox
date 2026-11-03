// Скрипты — свои клавиши, комбинации и реакции на игру на JavaScript (для опытных).
import {app, h, api, toast, requestRender, confirmDlg, promptDlg, hint} from '../core.js';

const API_REF = [
  ['hotkey("Ctrl+F1", fn)', 'клавиша или сочетание', 'hotkey("Ctrl+F1", () => {\n  play("airhorn", { to: "team" });\n});\n'],
  ['combo("Num1 Num1", fn, {within: 400})', 'последовательность: двойное нажатие, «Num1 Num2»…', 'combo("Num1 Num1", () => {\n  say("Мид, ганк!", { to: "team" });\n}, { within: 400 });\n'],
  ['hold("Num3", 500, fn, onRelease)', 'удержание клавиши N мс; onRelease — при отпускании', 'hold("Num3", 500, () => {\n  play("drumroll", { to: "team" });\n}, () => stop("drumroll"));\n'],
  ['onKey("F9", (down) => …)', 'любое нажатие/отпускание клавиши', 'onKey("F9", (down) => {\n  if (down) log("F9 нажата");\n});\n'],
  ['at("6:00", fn, {before: 10})', 'разово по игровому времени (за N с до)', 'at("6:00", () => {\n  say("Руны через 10 секунд", { to: "me" });\n}, { before: 10 });\n'],
  ['every("7:00", fn, {from, until, before})', 'повторять по игровому времени', 'every("7:00", (t) => {\n  notify("Руна мудрости в " + fmt(t));\n}, { from: "7:00", before: 20 });\n'],
  ['on("death", fn)', 'события: kill, death, respawn, low_hp, night, day, rosh_killed, game_start, new_game, clock, state…', 'on("death", () => {\n  play("sad_trombone", { to: "me" });\n});\n'],
  ['after(5, fn) / repeat(2, fn) / cancel(id)', 'реальные секунды (не игровые)', 'const id = repeat(2, () => log("тик"));\nafter(10, () => cancel(id));\n'],
  ['play("airhorn", {to, volume})', 'звук: to = "team" (эфир + вы), "me" (только вы), "both"', 'play("airhorn", { to: "team", volume: 0.8 });\n'],
  ['say("текст", {to})', 'озвучить текст голосом Windows', 'say("Мид пропал!", { to: "team" });\n'],
  ['stop() / stop("airhorn")', 'остановить звуки', 'stop();\n'],
  ['preset("radio") / preset()', 'включить эффект голоса / узнать текущий', 'preset("radio");\n'],
  ['action("rosh")', 'любое действие: rosh, stop, menu, mic_mute, mic_monitor, clock_horn, preset:next…', 'action("rosh");\n'],
  ['notify("текст") / log(…)', 'уведомление / строка в вывод скрипта', 'notify("Готово");\n'],
  ['game.clock · game.hero · game.paused', 'игровое время (null — нет игры), герой, game.daytime, game.state, game.clockSource', 'if (game.clock !== null && game.clock > 20 * 60) {\n  log("поздняя игра");\n}\n'],
  ['game.raw', 'весь JSON от Dota: hero, player, abilities, items, buildings', 'on("state", () => {\n  const r = game.raw;\n  if (r && r.hero) log(r.hero.health);\n});\n'],
  ['time("6:00") / fmt(360)', 'перевод времени: строка ⇄ секунды', 'log(fmt(time("6:00") + 30));\n'],
];
const KW = new Set('const let var function return if else for while of in new true false null undefined break continue switch case default typeof'.split(' '));
const FN = new Set('hotkey combo hold onKey at every on after repeat cancel play say stop preset action notify log game time fmt'.split(' '));
function highlight(code) {
  const esc = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;');
  const re = /(\/\/[^\n]*|\/\*[\s\S]*?\*\/)|("(?:\\.|[^"\\\n])*"?|'(?:\\.|[^'\\\n])*'?|`(?:\\.|[^`\\])*`?)|(\b\d+(?:\.\d+)?\b)|([A-Za-z_$][\w$]*)/g;
  let out = '', last = 0, m;
  while ((m = re.exec(code))) {
    out += esc(code.slice(last, m.index));
    const t = esc(m[0]);
    if (m[1]) out += '<i class="c">' + t + '</i>';
    else if (m[2]) out += '<i class="s">' + t + '</i>';
    else if (m[3]) out += '<i class="n">' + t + '</i>';
    else if (KW.has(m[0])) out += '<i class="k">' + t + '</i>';
    else if (FN.has(m[0])) out += '<i class="f">' + t + '</i>';
    else out += t;
    last = re.lastIndex;
  }
  return out + esc(code.slice(last)) + '\n';
}

let list = null, cur = null; // cur = {name, code, saved, err}
async function load(open) {
  try { list = (await api('/api/scripts')).scripts || []; } catch (e) { toast(e.message, 'err'); list = []; }
  if (open || (!cur && list.length)) await openScript(open || (list.find(s => s.enabled) || list[0]).name, true);
  requestRender();
}
async function openScript(name, quiet) {
  if (cur && cur.code !== cur.saved && !await confirmDlg('В «' + cur.name + '» есть несохранённые изменения. Отбросить их?', {ok: 'Отбросить', danger: true})) return;
  try { const r = await api('/api/scripts/code?name=' + encodeURIComponent(name)); cur = {name, code: r.code, saved: r.code, err: null}; } catch (e) { toast(e.message, 'err'); }
  if (!quiet) requestRender();
}
async function save(enable) {
  if (!cur) return;
  try {
    const r = await api('/api/scripts', {name: cur.name, code: cur.code, enable}, {method: 'PUT'});
    cur.saved = cur.code; cur.err = r.compile && r.compile.error ? r.compile : null; list = r.scripts;
    toast(cur.err ? 'Сохранено, но есть ошибка в строке ' + (cur.err.line || '?') : 'Сохранено: ' + cur.name, cur.err ? 'err' : 'ok');
  } catch (e) { toast(e.message, 'err'); }
  requestRender();
}

function editor() {
  const gut = h('div', {class: 'gut'});
  const pre = h('pre', {class: 'hl'});
  const ta = h('textarea', {spellcheck: false, value: cur.code, wrap: 'off'});
  const errBox = h('div');
  const dirty = h('span', {class: 'warn', style: 'font-size:12.5px'});
  const paint = () => {
    const n = cur.code.split('\n').length, el = cur.err && cur.err.line;
    gut.replaceChildren(...Array.from({length: n}, (_, i) => h('div', {class: i + 1 === el ? 'errl' : ''}, i + 1)));
    pre.innerHTML = highlight(cur.code);
    dirty.textContent = cur.code !== cur.saved ? '● не сохранено' : '';
    errBox.replaceChildren(cur.err ? h('div', {class: 'errbox', onclick: () => gotoLine(cur.err.line)}, '⚠ ', cur.err.line ? 'Строка ' + cur.err.line + ': ' : '', cur.err.error) : '');
  };
  const gotoLine = n => { if (!n) return; const pos = cur.code.split('\n').slice(0, n - 1).join('\n').length + (n > 1 ? 1 : 0); ta.focus(); ta.setSelectionRange(pos, pos); ta.scrollTop = Math.max(0, (n - 5) * 18); };
  let chk = null;
  ta.addEventListener('input', () => {
    cur.code = ta.value; paint();
    clearTimeout(chk);
    chk = setTimeout(async () => { try { const r = await api('/api/scripts/check', {name: cur.name, code: cur.code}); cur.err = r.error ? r : null; paint(); } catch (e) { /* */ } }, 600);
  });
  ta.addEventListener('scroll', () => { pre.scrollTop = ta.scrollTop; pre.scrollLeft = ta.scrollLeft; gut.scrollTop = ta.scrollTop; });
  ta.addEventListener('keydown', e => {
    if (e.key === 'Tab') { e.preventDefault(); document.execCommand('insertText', false, '  '); }
    if ((e.ctrlKey || e.metaKey) && e.code === 'KeyS') { e.preventDefault(); save(); }
    if (e.key === 'Enter') {
      const before = ta.value.slice(0, ta.selectionStart), ind = (before.split('\n').pop().match(/^\s*/) || [''])[0];
      e.preventDefault(); document.execCommand('insertText', false, '\n' + ind + (/[{(\[]\s*$/.test(before) ? '  ' : ''));
    }
  });
  editor.insert = s => { ta.focus(); document.execCommand('insertText', false, s); };
  setTimeout(paint);
  return {el: h('div', {}, h('div', {class: 'ed'}, gut, h('div', {class: 'edw'}, pre, ta)), errBox), dirty};
}

export default function scriptsPage() {
  if (!list) { load(); return h('p', {class: 'hint'}, 'Загружаю скрипты…'); }
  const info = cur && list.find(s => s.name === cur.name);
  const items = list.map(s => h('div', {class: 'scitem' + (cur && cur.name === s.name ? ' on' : ''), onclick: () => openScript(s.name)},
    h('label', {class: 'sw', onclick: e => e.stopPropagation()}, h('input', {type: 'checkbox', checked: s.enabled, onchange: async e => {
      try { list = (await api('/api/scripts/enable', {name: s.name, on: e.target.checked})).scripts; requestRender(); } catch (er) { toast(er.message, 'err'); }
    }}), h('i')),
    h('div', {style: 'min-width:0'}, h('b', {class: 'ell', style: 'display:block'}, s.name), s.title ? h('small', {class: 'ell'}, s.title) : null,
      h('small', {class: s.error ? 'bad' : s.running ? 'ok' : ''}, s.error ? '⚠ ошибка' : s.running ? '● работает' : s.enabled ? 'не запущен' : 'выключен'))));
  const newBtn = h('button', {class: 'btn sm', onclick: async () => {
    const n = ((await promptDlg('Новый скрипт', {label: 'Имя (буквы, цифры, _ и -)', value: 'my_script', ok: 'Создать'})) || '').trim();
    if (!n) return;
    if (list.some(s => s.name === n)) { toast('Такой скрипт уже есть', 'err'); return; }
    cur = null;
    try {
      await api('/api/scripts', {name: n, code: '// ' + n + ': что делает скрипт\n\nhotkey("F9", () => {\n  play("ding", { to: "me" });\n  notify("Работает!");\n});\n', enable: true}, {method: 'PUT'});
      await load(n);
    } catch (e) { toast(e.message, 'err'); }
  }}, '＋ Новый');
  let ed = null;
  const main = !cur ? h('div', {class: 'card'}, h('p', {class: 'hint', style: 'margin:0'}, 'Создайте скрипт или выберите слева.')) : (ed = editor(), h('div', {class: 'card'},
    h('div', {class: 'card-h', style: 'flex-wrap:wrap;row-gap:8px'}, h('h3', {class: 'ell', style: 'min-width:0'}, cur.name + '.js'), ed.dirty, h('div', {class: 'grow'}),
      h('button', {class: 'btn sm primary', onclick: () => save()}, '💾 Сохранить'),
      h('button', {class: 'btn sm', title: 'Сохранить и перезапустить', onclick: async () => {
        await save();
        try { const r = await api('/api/scripts/run', {name: cur.name}); list = r.scripts; cur.err = r.compile && r.compile.error ? r.compile : null; toast(cur.err ? 'Ошибка запуска' : 'Перезапущен', cur.err ? 'err' : 'ok'); requestRender(); } catch (e) { toast(e.message, 'err'); }
      }}, '▶ Запустить'),
      h('button', {class: 'btn sm icon danger', title: 'Удалить', onclick: async () => {
        if (!await confirmDlg('Скрипт «' + cur.name + '» будет удалён.', {title: 'Удалить скрипт?', ok: 'Удалить', danger: true})) return;
        try { list = (await api('/api/scripts/delete', {name: cur.name})).scripts; cur = null; if (list.length) await openScript(list[0].name, true); requestRender(); } catch (e) { toast(e.message, 'err'); }
      }}, '🗑️')),
    ed.el,
    info && info.hooks && info.hooks.length ? h('div', {class: 'row mt-s'}, h('span', {class: 'hint'}, 'Слушает:'), info.hooks.map(k => h('span', {class: 'kbd', title: k.kind},
      (k.kind === 'every' ? 'каждые ' : k.kind === 'at' ? 'в ' : k.kind === 'on' ? 'событие ' : '') + k.what))) : null,
    h('div', {class: 'flabel', style: 'margin:14px 0 6px'}, 'Вывод скрипта'),
    h('div', {class: 'out'}, info && info.output && info.output.length ? info.output.join('\n') : h('span', {class: 'faint'}, 'log(...) и ошибки появятся здесь')),
    h('div', {class: 'row mt-s'}, h('button', {class: 'btn sm ghost', onclick: () => load()}, '🔄 Обновить'))));
  return h('div', {},
    hint('Скрипты на JavaScript: свои клавиши, комбинации, тайминги и реакции на события игры. Клавиши не отбираются у игры. Каждый скрипт работает отдельно — ошибка одного не ломает остальные.'),
    h('div', {class: 'scgrid mt'},
      h('div', {class: 'card'}, h('div', {class: 'card-h'}, h('h3', {}, 'Скрипты'), h('div', {class: 'grow'}), newBtn), items.length ? items : h('p', {class: 'hint'}, 'Скриптов пока нет.')),
      main,
      h('div', {class: 'card'}, h('h3', {}, 'Справка'), h('p', {class: 'hint', style: 'margin:-6px 0 8px'}, 'Нажмите — вставится пример.'),
        API_REF.map(([sig, desc, snip]) => h('div', {class: 'refi', onclick: () => editor.insert ? editor.insert(snip) : null}, h('code', {}, sig), h('small', {}, desc))))));
}
