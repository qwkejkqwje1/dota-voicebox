// Пульт VoiceBox для телефона: плитки звуков, эффекты голоса, стоп и микрофон.
// Все адреса относительные: страница живёт по адресу /<ключ>/ на компьютере с VoiceBox.
(function () {
  'use strict';
  var $ = function (s) { return document.querySelector(s); };
  var state = null, live = null, tab = 'sounds', es = null, waitTimer = 0, refetchTimer = 0, toastTimer = 0;

  function api(path, body) {
    var opts = body === undefined ? {cache: 'no-store'} :
      {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body)};
    return fetch('api/' + path, opts).then(function (r) {
      if (r.status === 404) { var e = new Error('expired'); e.expired = true; throw e; }
      return r.json().then(function (j) {
        if (!r.ok) throw new Error(j.error || r.statusText);
        return j;
      });
    });
  }

  // подпись плитки: длинные слова не влезают в узкую плитку — шрифт поменьше, чтобы не рвать слово
  function label(text) {
    var s = el('span', 'lb', text);
    var long = 0;
    String(text || '').split(/\s+/).forEach(function (w) { long = Math.max(long, w.length); });
    if (long > 13) s.classList.add('xs'); else if (long > 10) s.classList.add('sm');
    return s;
  }

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }

  function hexRGB(hex) {
    var m = /^#?([0-9a-f]{6})$/i.exec(hex || '');
    if (!m) return null;
    var n = parseInt(m[1], 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  }

  function toast(text) {
    var t = $('#toast');
    t.textContent = text;
    t.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { t.hidden = true; }, 2600);
  }

  function buzz() { if (navigator.vibrate) try { navigator.vibrate(12); } catch (e) {} }

  function press(btn) {
    btn.classList.add('press');
    setTimeout(function () { btn.classList.remove('press'); }, 140);
    buzz();
  }

  // ---------- связь ----------

  function setConn(kind, text) {
    var c = $('#conn');
    c.className = 'conn-' + kind;
    c.querySelector('span').textContent = text;
  }

  function cover(show, kind) {
    var c = $('#cover');
    c.hidden = !show;
    if (!show) return;
    if (kind === 'expired') {
      $('#coverIcon').textContent = '🔒';
      $('#coverTitle').textContent = 'Ссылка устарела';
      $('#coverText').textContent = 'Пульт выключен или ссылку сменили. Откройте в VoiceBox страницу «Телефон» и отсканируйте QR-код ещё раз.';
    } else {
      $('#coverIcon').textContent = '📡';
      $('#coverTitle').textContent = 'Нет связи с компьютером';
      $('#coverText').textContent = 'Проверьте, что VoiceBox запущен, а телефон подключён к той же сети Wi-Fi, что и компьютер.';
    }
  }

  function connect() {
    if (es) es.close();
    setConn('wait', 'подключение…');
    es = new EventSource('api/events');
    es.onopen = function () {
      clearTimeout(waitTimer);
      setConn('live', 'на связи');
      cover(false);
    };
    es.addEventListener('live', function (e) {
      try { applyLive(JSON.parse(e.data)); } catch (err) {}
    });
    es.onerror = function () {
      setConn('wait', 'переподключение…');
      clearTimeout(waitTimer);
      waitTimer = setTimeout(check, es.readyState === 2 ? 300 : 3000);
    };
  }

  // check — связь пропала надолго: ссылка устарела или компьютер недоступен?
  function check() {
    if (es && es.readyState === 1) return;
    api('state').then(function (s) {
      state = s; render(); applyLive(s.live);
      if (!es || es.readyState === 2) connect();
    }, function (err) {
      if (err.expired) {
        setConn('off', 'ссылка устарела');
        cover(true, 'expired');
        if (es) es.close();
        return;
      }
      setConn('off', 'нет связи');
      cover(true, 'offline');
      clearTimeout(waitTimer);
      waitTimer = setTimeout(function () { if (!es || es.readyState === 2) connect(); else check(); }, 4000);
    });
  }

  function load() {
    return api('state').then(function (s) {
      state = s;
      render();
      applyLive(s.live);
    }, function (err) {
      if (err.expired) { cover(true, 'expired'); setConn('off', 'ссылка устарела'); }
    });
  }

  // ---------- отрисовка ----------

  function render() {
    if (!state) return;
    var effects = state.effects && state.effects.length > 0;
    $('#tabs').hidden = !effects;
    if (!effects) tab = 'sounds';
    var tabs = document.querySelectorAll('#tabs button');
    for (var i = 0; i < tabs.length; i++) tabs[i].classList.toggle('on', tabs[i].dataset.tab === tab);
    $('#mic').hidden = !state.show_mic;
    var main = $('#main');
    main.textContent = '';
    if (tab === 'effects') {
      var g = el('div', 'grid');
      state.effects.forEach(function (it) { g.appendChild(effectTile(it)); });
      main.appendChild(g);
    } else {
      if (state.sets && state.sets.length > 1) main.appendChild(setsRow(state.sets));
      var inSet = (state.sets || []).some(function (x) { return x.active && x.id; });
      if (!state.sounds.length) main.appendChild(el('div', 'empty', inSet ? 'В этом наборе пока нет звуков — выберите их в VoiceBox на компьютере.' : 'Звуков пока нет — добавьте их в VoiceBox на компьютере.'));
    }
    if (tab !== 'effects' && state.sounds.length) {
      groups(state.sounds).forEach(function (gr) {
        if (gr.name) main.appendChild(el('div', 'group', gr.name));
        var grid = el('div', 'grid');
        gr.items.forEach(function (it) { grid.appendChild(soundTile(it)); });
        main.appendChild(grid);
      });
    }
    paintLive();
  }

  // setsRow — наборы звуков: нажатие включает набор и на компьютере (панель, меню, колесо).
  function setsRow(sets) {
    var row = el('div', 'sets');
    sets.forEach(function (it) {
      var b = el('button', 'set' + (it.active ? ' on' : ''));
      b.type = 'button';
      b.appendChild(el('span', 'em', it.emoji || '🗂️'));
      b.appendChild(el('span', '', it.label));
      b.addEventListener('click', function () {
        if (it.active) return;
        press(b);
        api('set', {id: it.id}).then(function (l) { applyLive(l); load(); }, function (err) {
          if (err.expired) return check();
          toast('Не получилось: ' + err.message);
        });
      });
      row.appendChild(b);
    });
    return row;
  }

  // groups — разделы по категориям в порядке панели; звуки без категории — первыми, без заголовка.
  function groups(items) {
    var out = [], byName = {};
    items.forEach(function (it) {
      var name = it.group || '';
      if (!byName[name]) {
        byName[name] = {name: name, items: []};
        if (name === '') out.unshift(byName[name]); else out.push(byName[name]);
      }
      byName[name].items.push(it);
    });
    if (out.length === 1) out[0].name = '';
    return out;
  }

  function soundTile(it) {
    var b = el('button', 'tile');
    b.type = 'button';
    b.dataset.id = it.id;
    var rgb = hexRGB(it.color);
    if (rgb) {
      var c = rgb.join(',');
      b.style.setProperty('--c', 'rgb(' + c + ')');
      b.style.setProperty('--tint', 'rgba(' + c + ',.30)');
      b.style.setProperty('--glow', 'rgba(' + c + ',.5)');
    }
    if (it.pending) b.classList.add('pending');
    b.appendChild(el('span', 'em', it.emoji || '🔊'));
    b.appendChild(label(it.label));
    var eq = el('span', 'eq');
    eq.appendChild(el('i')); eq.appendChild(el('i')); eq.appendChild(el('i'));
    b.appendChild(eq);
    var pg = el('span', 'pg');
    pg.appendChild(el('i'));
    b.appendChild(pg);
    b.addEventListener('click', function () {
      press(b);
      api('play', {id: it.id}).then(applyLive, function (err) {
        if (err.expired) return check();
        toast('Не получилось: ' + err.message);
      });
    });
    return b;
  }

  function effectTile(it) {
    var b = el('button', 'tile fx');
    b.type = 'button';
    b.dataset.fx = it.id;
    b.appendChild(el('span', 'em', it.emoji || '🎛️'));
    b.appendChild(label(it.label));
    b.appendChild(el('span', 'state', ''));
    b.addEventListener('click', function () {
      press(b);
      api('preset', {id: it.id}).then(applyLive, function (err) {
        if (err.expired) return check();
        toast('Не получилось: ' + err.message);
      });
    });
    return b;
  }

  function applyLive(l) {
    if (!l) return;
    var prev = live;
    live = l;
    if (state && l.rev !== state.rev) { // на компьютере что-то поменяли — перечитать плитки
      clearTimeout(refetchTimer);
      refetchTimer = setTimeout(load, 150);
    }
    if (!prev || prev.preset !== l.preset || prev.mute !== l.mute || JSON.stringify(prev.playing) !== JSON.stringify(l.playing)) paintLive();
  }

  function paintLive() {
    if (!live || !state) return;
    var on = {};
    (live.playing || []).forEach(function (p) { on[p.id] = p; });
    var tiles = document.querySelectorAll('.tile[data-id]');
    for (var i = 0; i < tiles.length; i++) {
      var t = tiles[i], p = on[t.dataset.id];
      t.classList.toggle('on', !!p);
      var bar = t.querySelector('.pg i');
      if (p && bar) bar.style.width = (p.len > 0 ? Math.min(100, (p.pos + 0.25) / p.len * 100) : 100) + '%';
      else if (bar) bar.style.width = '0';
    }
    var fx = document.querySelectorAll('.tile[data-fx]');
    for (var j = 0; j < fx.length; j++) {
      var active = fx[j].dataset.fx === live.preset;
      fx[j].classList.toggle('on', active);
      fx[j].querySelector('.state').textContent = active ? 'включён' : '';
    }
    // текущий эффект — в шапке
    var chip = $('#fx'), cur = null;
    (state.effects || []).forEach(function (e) { if (e.id === live.preset) cur = e; });
    if (!cur && state.preset && state.preset.id === live.preset) cur = state.preset;
    chip.hidden = !cur;
    if (cur) {
      chip.querySelector('.e').textContent = cur.emoji || '🎙️';
      chip.querySelector('.t').textContent = cur.label;
    }
    var mic = $('#mic');
    mic.classList.toggle('muted', !!live.mute);
    mic.classList.toggle('live', !live.mute);
    mic.querySelector('.e').textContent = live.mute ? '🔇' : '🎙️';
    mic.querySelector('.t').textContent = live.mute ? 'Микрофон выключен' : 'Микрофон включён';
  }

  // ---------- кнопки ----------

  $('#tabs').addEventListener('click', function (e) {
    var b = e.target.closest('button');
    if (!b || b.dataset.tab === tab) return;
    tab = b.dataset.tab;
    render();
    window.scrollTo(0, 0);
  });
  $('#fx').addEventListener('click', function () {
    if (state && state.effects && state.effects.length) { tab = 'effects'; render(); window.scrollTo(0, 0); }
  });
  $('#stop').addEventListener('click', function () {
    press($('#stop'));
    api('stop', {}).then(applyLive, function (err) { if (err.expired) check(); else toast(err.message); });
  });
  $('#mic').addEventListener('click', function () {
    press($('#mic'));
    api('mic', {}).then(applyLive, function (err) { if (err.expired) check(); else toast(err.message); });
  });
  $('#retry').addEventListener('click', function () {
    cover(false);
    setConn('wait', 'подключение…');
    load().then(connect);
  });
  document.addEventListener('visibilitychange', function () {
    if (document.visibilityState === 'visible' && (!es || es.readyState !== 1)) { load(); connect(); }
  });

  load();
  connect();
})();
