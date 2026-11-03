// Обрезка звука: волна с ручками начала и конца, время, «убрать тишину», «послушать».
import {app, h, api, act, toast, dialog} from './core.js';

const MIN = 0.05; // самый короткий кусок, сек

// 83,25 → «1:23,25»; 3,5 → «3,50»
export const fmtT = t => {
  t = Math.max(0, t);
  const m = Math.floor(t / 60), s = (t - m * 60).toFixed(2).replace('.', ',');
  return m ? m + ':' + s.padStart(5, '0') : s;
};
export const parseT = str => {
  const p = String(str).trim().replace(',', '.').split(':');
  const v = p.length > 1 ? (+p[0]) * 60 + (+p[1]) : +p[0];
  return Number.isFinite(v) ? v : null;
};
const fmtLen = t => t.toFixed(2).replace('.', ',') + ' с';

// trimEditor — o: {wave: {duration, peaks, start, end, sound: [s, e]}, play(start, end), stop()}.
// Возвращает {el, get: () => [start, end]}. Элемент создаётся один раз и переживает перерисовки окна.
export function trimEditor(o) {
  const w = o.wave, dur = w.duration;
  let s = w.start || 0, e = w.end && w.end < dur ? w.end : dur, raf = 0;
  const cv = h('canvas');
  const shL = h('div', {class: 'wave-sh'}), shR = h('div', {class: 'wave-sh r'});
  const ph = h('div', {class: 'wave-ph hidden'});
  const hs = h('div', {class: 'wave-h', tabindex: 0, role: 'slider', 'aria-label': 'Начало', title: 'Начало: тяните мышью или ← → (Shift — шаг больше)'});
  const he = h('div', {class: 'wave-h', tabindex: 0, role: 'slider', 'aria-label': 'Конец', title: 'Конец: тяните мышью или ← → (Shift — шаг больше)'});
  const box = h('div', {class: 'wave'}, cv, shL, shR, ph, hs, he);
  const inS = h('input', {class: 'input', value: fmtT(s), onchange: ev => setTime('s', ev.target.value)});
  const inE = h('input', {class: 'input', value: fmtT(e), onchange: ev => setTime('e', ev.target.value)});
  const len = h('b', {});
  const playBtn = h('button', {class: 'btn sm primary play-btn', title: 'Послушать выбранный кусок (слышите только вы)', onclick: () => raf ? stop() : play()}, '▶ Послушать');
  const el = h('div', {class: 'trim'}, box,
    h('div', {class: 'trim-row'},
      h('label', {class: 'field'}, h('span', {}, 'Начало'), inS),
      h('label', {class: 'field'}, h('span', {}, 'Конец'), inE),
      h('div', {class: 'trim-len'}, h('span', {class: 'flabel'}, 'Останется'), len),
      h('div', {class: 'grow'}),
      h('button', {class: 'btn sm', title: 'Срезать тишину в начале и в конце', onclick: () => set(w.sound[0], w.sound[1])}, '✂️ Убрать тишину'),
      h('button', {class: 'btn sm ghost', title: 'Вернуть весь звук', onclick: () => set(0, dur)}, '↔ Весь'),
      playBtn));

  function set(a, b) {
    a = Math.max(0, Math.min(a, dur - MIN));
    b = Math.max(a + MIN, Math.min(b, dur));
    s = Math.round(a * 1000) / 1000; e = Math.round(b * 1000) / 1000;
    paint();
  }
  const moveS = t => set(Math.min(t, e - MIN), e);
  const moveE = t => set(s, Math.max(t, s + MIN));
  function setTime(which, str) {
    const v = parseT(str);
    if (v == null) { toast('Время — в секундах, например 1,25 или 1:05,50', 'err'); paint(); return; }
    if (which === 's') moveS(v); else moveE(v);
  }
  function paint() {
    const p = t => (t / dur * 100) + '%';
    shL.style.width = p(s);
    shR.style.left = p(e);
    hs.style.left = p(s);
    he.style.left = p(e);
    inS.value = fmtT(s); inE.value = fmtT(e);
    len.textContent = fmtLen(e - s);
  }
  function draw() {
    const W = box.clientWidth, H = box.clientHeight;
    if (!W || !H) return;
    const dpr = window.devicePixelRatio || 1;
    cv.width = Math.round(W * dpr); cv.height = Math.round(H * dpr);
    const g = cv.getContext('2d');
    g.setTransform(dpr, 0, 0, dpr, 0, 0);
    g.clearRect(0, 0, W, H);
    const pk = w.peaks || [], n = pk.length;
    let top = 0;
    for (const v of pk) top = Math.max(top, v);
    g.fillStyle = getComputedStyle(box).getPropertyValue('--wave').trim() || '#a78bfa';
    const mid = H / 2, amp = H / 2 - 4;
    for (let x = 0; x < W; x++) {
      const i0 = Math.floor(x / W * n), i1 = Math.max(i0 + 1, Math.floor((x + 1) / W * n));
      let m = 0;
      for (let i = i0; i < i1 && i < n; i++) m = Math.max(m, pk[i]);
      const hh = Math.max(0.5, top ? m / top * amp : 0);
      g.fillRect(x, mid - hh, 1, hh * 2);
    }
  }
  // мышь: тянуть ручку; клик по волне — ближайшая ручка прыгает туда
  let drag = null;
  const at = ev => { const r = box.getBoundingClientRect(); return Math.max(0, Math.min(1, (ev.clientX - r.left) / r.width)) * dur; };
  const move = t => drag === hs ? moveS(t) : moveE(t);
  box.addEventListener('pointerdown', ev => {
    if (ev.button !== 0) return;
    const t = at(ev);
    drag = ev.target === hs || ev.target === he ? ev.target : (Math.abs(t - s) <= Math.abs(t - e) ? hs : he);
    box.setPointerCapture(ev.pointerId);
    drag.focus();
    move(t);
    ev.preventDefault();
  });
  box.addEventListener('pointermove', ev => { if (drag) move(at(ev)); });
  const up = () => { drag = null; };
  box.addEventListener('pointerup', up);
  box.addEventListener('pointercancel', up);
  for (const hd of [hs, he]) hd.addEventListener('keydown', ev => {
    const step = ev.shiftKey ? 0.1 : 0.01;
    const d = ev.key === 'ArrowLeft' ? -step : ev.key === 'ArrowRight' ? step : 0;
    if (!d) return;
    ev.preventDefault();
    if (hd === hs) moveS(s + d); else moveE(e + d);
  });
  new ResizeObserver(draw).observe(box);

  async function play() {
    stop(false);
    try { await o.play(s, e); } catch (er) { toast(er.message, 'err'); return; }
    const t0 = performance.now(), a = s, b = e;
    ph.classList.remove('hidden');
    playBtn.textContent = '⏹ Стоп';
    const tick = () => {
      const t = a + (performance.now() - t0) / 1000;
      if (t >= b || !el.isConnected) { stop(false); return; }
      ph.style.left = (t / dur * 100) + '%';
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
  }
  function stop(send = true) {
    if (raf) cancelAnimationFrame(raf);
    raf = 0;
    ph.classList.add('hidden');
    playBtn.textContent = '▶ Послушать';
    if (send) o.stop();
  }
  paint();
  return {el, get: () => [s, e], stop: () => stop()};
}

const stopPreview = () => api('/api/play', {id: 'preview', stop: true}).catch(() => {});

// можно ли обрезать звук: один файл (не встроенный и не фраза)
export const canTrim = s => s && s.kind === 'file' && (s.files || []).length === 1;

// trimDialog — обрезать звук с панели. Файл не меняется: обрезка хранится в настройках звука.
// done() — после сохранения или сброса обрезки.
export async function trimDialog(s, done = () => {}) {
  let w;
  try { w = await api('/api/sounds/wave?id=' + encodeURIComponent(s.id)); } catch (e) { toast(e.message, 'err'); return; }
  const trimmed = w.start > 0 || w.end < w.duration;
  const ed = trimEditor({wave: w, play: (a, b) => api('/api/sounds/wave/play', {id: s.id, start: a, end: b}), stop: stopPreview});
  const d = dialog({title: '✂️ Обрезать «' + s.title + '»', wide: true,
    onClose: () => { ed.stop(); api('/api/sounds/wave/done', {}).catch(() => {}); },
    body: () => [
      h('p', {class: 'hint', style: 'margin:0 0 12px'}, 'Перетащите ручки или нажмите на волну. «Убрать тишину» срежет тихие края сама. Файл не меняется — обрезку можно сбросить в любой момент.'),
      ed.el],
    foot: () => [
      trimmed ? h('button', {class: 'btn ghost', onclick: async () => {
        if (await act('/api/sounds/trim', {id: s.id, start: 0, end: 0}, {ok: 'Обрезка сброшена — звук целиком'})) { d.close(); done(); }
      }}, '↺ Без обрезки') : null,
      h('div', {class: 'grow'}),
      h('button', {class: 'btn', onclick: () => d.close()}, 'Отмена'),
      h('button', {class: 'btn primary', onclick: async () => {
        const [a, b] = ed.get();
        if (await act('/api/sounds/trim', {id: s.id, start: a, end: b}, {ok: '✂️ Звук обрезан: ' + fmtLen(b - a)})) { d.close(); done(); }
      }}, 'Сохранить')]});
  return d;
}

// presetTitle — название текущего эффекта голоса ('' — эффекта нет)
export function presetTitle() {
  const id = app.status && app.status.preset;
  if (!id || id === 'clean') return '';
  const p = (app.state.presets || []).find(x => x.id === id);
  return p ? p.title : id;
}
