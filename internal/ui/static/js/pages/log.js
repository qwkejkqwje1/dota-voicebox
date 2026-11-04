// Журнал — что происходит в программе (для поиска проблем).
import {_t} from '../i18n.js';
import {app, h, onLive, toast} from '../core.js';

let q = '';
export default function logPage() {
  const box = h('div', {id: 'log'});
  const match = l => !q || l.toLowerCase().includes(q);
  const fill = () => { box.textContent = (app.state.logs || []).filter(match).join('\n'); box.scrollTop = box.scrollHeight; };
  onLive('log', line => {
    if (!match(line)) return;
    const atEnd = box.scrollTop + box.clientHeight >= box.scrollHeight - 30;
    box.append((box.textContent ? '\n' : '') + line);
    if (atEnd) box.scrollTop = box.scrollHeight;
  });
  setTimeout(fill);
  return h('div', {},
    h('div', {class: 'row', style: 'margin-bottom:12px'},
      h('div', {class: 'search'}, h('input', {class: 'input', type: 'search', placeholder: _t('Фильтр…'), value: q, oninput: e => { q = e.target.value.toLowerCase(); fill(); }})),
      h('div', {class: 'grow'}),
      h('button', {class: 'btn sm', onclick: () => { navigator.clipboard.writeText((app.state.logs || []).join('\n')).then(() => toast(_t('Журнал скопирован'), 'ok'), () => toast(_t('Не удалось скопировать'), 'err')); }}, _t('📋 Скопировать'))),
    box);
}
