// emoji.js — картинки эмодзи для меню поверх игры: окно рисует их цветным шрифтом Windows и отдаёт программе.
import {app, api} from './core.js';

const sent = new Set();
const EXTRA = ['⚠️', '🐲', '⏰', '🔇', '🎙️', '⏹️', '🔊', '🗣️', '🔔', '📯', '🎛️', '✅', '❌', '🔈', '🗂️', '😂'];
let busy = false;

export async function syncEmoji() {
  const st = app.state;
  if (!st || busy) return;
  const all = new Set(EXTRA);
  for (const s of st.sounds || []) if (s.emoji) all.add(s.emoji);
  for (const p of st.presets || []) if (p.emoji) all.add(p.emoji);
  for (const x of (st.config && st.config.sets) || []) if (x.emoji) all.add(x.emoji);
  for (const a of (st.voice && st.voice.ambiences) || []) if (a.emoji) all.add(a.emoji);
  const want = [...all].filter(e => !sent.has(e));
  if (!want.length) return;
  busy = true;
  try {
    const r = await api('/api/emoji/missing', {list: want});
    want.forEach(e => sent.add(e));
    for (const e of r.missing || []) {
      const png = await render(e);
      if (png) await api('/api/emoji?e=' + encodeURIComponent(e), png).catch(() => {});
    }
  } catch (e) { /* не критично: меню нарисует эмодзи своим шрифтом */ }
  busy = false;
}

const SIZE = 96;
function render(e) {
  const c = document.createElement('canvas');
  c.width = c.height = SIZE;
  const g = c.getContext('2d');
  g.font = Math.round(SIZE * 0.78) + 'px "Segoe UI Emoji", "Apple Color Emoji", "Noto Color Emoji", sans-serif';
  g.textBaseline = 'alphabetic';
  g.textAlign = 'left';
  const m = g.measureText(e);
  const w = m.actualBoundingBoxLeft + m.actualBoundingBoxRight, hh = m.actualBoundingBoxAscent + m.actualBoundingBoxDescent;
  if (!w || !hh) return null;
  const k = Math.min(1, (SIZE - 4) / Math.max(w, hh));
  if (k < 1) g.font = Math.floor(SIZE * 0.78 * k) + 'px "Segoe UI Emoji", "Apple Color Emoji", "Noto Color Emoji", sans-serif';
  const m2 = g.measureText(e);
  const x = SIZE / 2 - (m2.actualBoundingBoxRight - m2.actualBoundingBoxLeft) / 2;
  const y = SIZE / 2 + (m2.actualBoundingBoxAscent - m2.actualBoundingBoxDescent) / 2;
  g.fillStyle = '#ff00ff'; // цветной шрифт этот цвет не использует, а обычный (запасной) — красит им
  g.fillText(e, x, y);
  // пусто или нарисовано не цветным шрифтом — не отправляем, меню нарисует сама
  const d = g.getImageData(0, 0, SIZE, SIZE).data;
  let opaque = 0, magenta = 0;
  for (let i = 0; i < d.length; i += 16) {
    if (d[i + 3] > 40) { opaque++; if (d[i] > 180 && d[i + 1] < 90 && d[i + 2] > 180) magenta++; }
  }
  if (opaque < 10 || magenta > opaque * 0.4) return null;
  return new Promise(r => c.toBlob(r, 'image/png'));
}
