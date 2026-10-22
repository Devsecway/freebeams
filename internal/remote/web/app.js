// FreeBeams Remote: the phone side of internal/remote. No libraries and no
// external requests: everything comes from the FreeBeams computer.
//
// The page is a projection of the state FreeBeams pushes over a WebSocket
// (render() is idempotent and only reads `app.st`); taps send actions over the
// same socket (HTTP when the socket is down) and the next push shows their
// effect. The socket reconnects by itself with backoff, and at once when the
// page comes back to the foreground.
'use strict';

(function () {
  const $ = (id) => document.getElementById(id);
  const app = {
    session: null, // {name, device, permissions, search}
    st: null, // the last state pushed by FreeBeams
    tab: 'schedule',
    deck: 'live', // which deck the Slides tab shows
    deckChosen: false, // the operator picked a deck (else it follows what is live)
    ws: null,
    wsOpen: false,
    retry: 0,
    retryTimer: 0,
    nextId: 1,
    replies: new Map(),
    lastMessage: 0,
    pair: null, // {id, secret, name} while waiting for approval
    drawn: {}, // what render last built (lists are rebuilt only when they change)
  };
  const PERM_CONTROL = 2, PERM_SCHEDULE = 4;

  // ---- tiny helpers -------------------------------------------------------------
  function el(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function svg(markup) {
    const t = document.createElement('template');
    t.innerHTML = markup.trim();
    return t.content.firstChild;
  }
  const ICONS = {
    song: '<svg viewBox="0 0 24 24"><path d="M9 18V6l10-2v12"/><circle cx="6.5" cy="18" r="2.5"/><circle cx="16.5" cy="16" r="2.5"/></svg>',
    scripture: '<svg viewBox="0 0 24 24"><path d="M4 5.5C6.5 4 9.5 4 12 5.5v14C9.5 18 6.5 18 4 19.5z"/><path d="M20 5.5C17.5 4 14.5 4 12 5.5v14c2.5-1.5 5.5-1.5 8 0z"/></svg>',
    media: '<svg viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 16l5-5 4 4 3-3 6 6"/></svg>',
    presentation: '<svg viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="12" rx="1.5"/><path d="M12 16v4M8 20h8"/></svg>',
    play: '<svg viewBox="0 0 24 24"><path d="M8 5.5v13a1 1 0 0 0 1.5.86l10.5-6.5a1 1 0 0 0 0-1.72L9.5 4.64A1 1 0 0 0 8 5.5z"/></svg>',
  };
  function haptic() { if (navigator.vibrate) navigator.vibrate(8); }

  let toastTimer = 0;
  function toast(msg, isError) {
    const t = $('toast');
    t.textContent = msg;
    t.classList.toggle('error', !!isError);
    t.classList.add('show');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => t.classList.remove('show'), isError ? 3200 : 1800);
  }

  async function api(path, body) {
    const opts = { credentials: 'same-origin', headers: {} };
    if (body !== undefined) {
      opts.method = 'POST';
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    const res = await fetch(path, opts);
    let data = {};
    try { data = await res.json(); } catch (e) { /* not JSON */ }
    return { status: res.status, data };
  }

  function show(screen) {
    for (const id of ['boot', 'pair', 'wait', 'main']) $(id).hidden = id !== screen;
  }

  // ---- startup and pairing ----------------------------------------------------------
  async function boot() {
    let r;
    try {
      r = await api('/api/session');
    } catch (e) {
      $('boot-msg').textContent = 'Cannot reach FreeBeams. Is the computer on this Wi-Fi and Remote Control enabled?';
      show('boot');
      setTimeout(boot, 3000);
      return;
    }
    if (r.status === 200 && r.data.paired) {
      app.session = r.data;
      startRemote();
    } else {
      showPair();
    }
  }

  function guessName() {
    const ua = navigator.userAgent;
    if (/iPad/.test(ua) || (/Macintosh/.test(ua) && navigator.maxTouchPoints > 1)) return 'iPad';
    if (/iPhone/.test(ua)) return 'iPhone';
    if (/Android/.test(ua)) return /Mobile/.test(ua) ? 'Android phone' : 'Android tablet';
    return 'Browser';
  }
  function savedName() {
    try { return localStorage.getItem('fb.deviceName') || ''; } catch (e) { return ''; }
  }

  function showPair(message) {
    stopSocket();
    app.session = null;
    show('pair');
    $('pair-error').textContent = message || '';
    if (!$('devname').value) $('devname').value = savedName() || guessName();
    $('pair-btn').disabled = false;
    setTimeout(() => { if (!$('pair').hidden && !$('code').value) $('code').focus(); }, 50);
  }

  $('code').addEventListener('input', (e) => {
    e.target.value = e.target.value.replace(/\D/g, '').slice(0, 6);
    $('pair-error').textContent = '';
  });

  $('pair-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const code = $('code').value.trim(), name = $('devname').value.trim();
    if (code.length !== 6) { $('pair-error').textContent = 'Enter the 6-digit code shown in FreeBeams.'; $('code').focus(); return; }
    if (!name) { $('pair-error').textContent = 'Give this device a name the operator will recognise.'; $('devname').focus(); return; }
    try { localStorage.setItem('fb.deviceName', name); } catch (err) { /* private mode */ }
    $('pair-btn').disabled = true;
    let r;
    try {
      r = await api('/api/pair', { code, name });
    } catch (err) {
      $('pair-btn').disabled = false;
      $('pair-error').textContent = 'Cannot reach FreeBeams.';
      return;
    }
    $('pair-btn').disabled = false;
    if (r.status !== 200) {
      $('pair-error').textContent = r.data.error || 'Pairing failed.';
      if (r.status === 403) { $('code').value = ''; $('code').focus(); }
      return;
    }
    document.activeElement && document.activeElement.blur();
    app.pair = { id: r.data.id, secret: r.data.secret, name };
    $('wait-name').textContent = '“' + name + '”';
    show('wait');
    waitForApproval(app.pair);
  });

  async function waitForApproval(pair) {
    while (app.pair === pair) {
      let r;
      try {
        r = await api('/api/pair/wait', { id: pair.id, secret: pair.secret });
      } catch (e) {
        await new Promise((ok) => setTimeout(ok, 1500));
        continue;
      }
      if (app.pair !== pair) return;
      const status = r.data.status;
      if (status === 'pending') continue;
      app.pair = null;
      if (status === 'approved') {
        $('code').value = '';
        boot();
      } else {
        showPair(r.data.error || 'The request expired. Pair again.');
      }
      return;
    }
  }

  $('wait-cancel').addEventListener('click', () => {
    const pair = app.pair;
    app.pair = null;
    if (pair) api('/api/pair/cancel', { id: pair.id, secret: pair.secret }).catch(() => {});
    showPair();
  });

  // ---- the socket ------------------------------------------------------------------------
  function startRemote() {
    show('main');
    $('server-name').textContent = app.session.name || 'FreeBeams';
    $('sheet-device').textContent = 'Paired as “' + app.session.device.name + '”';
    document.querySelector('.tab[data-tab="songs"]').hidden = !(app.session.search && (app.session.permissions & PERM_SCHEDULE));
    connect();
    render();
  }

  function setConn(state, text) {
    const c = $('conn');
    c.dataset.state = state;
    c.lastChild.textContent = text;
    document.body.classList.toggle('offline', state !== 'online');
  }

  function connect() {
    if (app.ws || !app.session) return;
    clearTimeout(app.retryTimer);
    setConn('connecting', app.retry ? 'Reconnecting' : 'Connecting');
    let ws;
    try {
      ws = new WebSocket((location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/api/ws');
    } catch (e) {
      scheduleReconnect();
      return;
    }
    app.ws = ws;
    ws.onopen = () => {
      app.wsOpen = true;
      app.retry = 0;
      app.lastMessage = Date.now();
      setConn('online', 'Live');
    };
    ws.onmessage = (ev) => {
      app.lastMessage = Date.now();
      let m;
      try { m = JSON.parse(ev.data); } catch (e) { return; }
      if (m.type === 'state') {
        app.st = m.state;
        render();
      } else if (m.type === 'hello') {
        app.session = m.session;
      } else if (m.type === 'reply') {
        const cb = app.replies.get(m.id);
        if (cb) { app.replies.delete(m.id); cb(m); }
      } else if (m.type === 'revoked') {
        app.revoked = true;
      }
    };
    ws.onclose = (ev) => {
      if (app.ws !== ws) return;
      app.ws = null;
      app.wsOpen = false;
      for (const cb of app.replies.values()) cb({ ok: false, error: 'Connection lost' });
      app.replies.clear();
      if (app.revoked || ev.code === 1008) {
        app.revoked = false;
        showPair('This device was removed in FreeBeams. Pair again to continue.');
        return;
      }
      if (!app.session) return;
      setConn('connecting', 'Reconnecting');
      // A refused upgrade looks like any other close: ask whether we are still paired.
      api('/api/session').then((r) => {
        if (r.status === 401) showPair('This device is no longer paired. Pair again to continue.');
        else scheduleReconnect();
      }, () => { setConn('offline', 'Offline'); scheduleReconnect(); });
    };
  }

  function scheduleReconnect() {
    clearTimeout(app.retryTimer);
    const delay = Math.min(4000, 250 * Math.pow(2, app.retry)) * (0.8 + Math.random() * 0.4);
    app.retry = Math.min(app.retry + 1, 8);
    app.retryTimer = setTimeout(connect, delay);
  }

  function stopSocket() {
    clearTimeout(app.retryTimer);
    const ws = app.ws;
    app.ws = null;
    app.wsOpen = false;
    if (ws) ws.close();
  }

  // Browsers cannot send ping frames: a small ping every 15 s finds a dead
  // connection (Wi-Fi roaming, a phone waking up) and reconnects.
  setInterval(() => {
    if (!app.wsOpen) return;
    if (Date.now() - app.lastMessage > 35000) { app.ws.close(); return; }
    send('ping', null, true);
  }, 15000);

  function wake() {
    if (!app.session || document.hidden) return;
    if (!app.ws) { app.retry = 0; connect(); } else if (app.wsOpen) send('ping', null, true);
  }
  document.addEventListener('visibilitychange', wake);
  window.addEventListener('online', wake);
  window.addEventListener('pageshow', wake);

  // send runs a FreeBeams action: over the socket, or HTTP while it is down.
  function send(action, args, quiet) {
    if (!quiet) haptic();
    const done = (r) => { if (!r.ok && !quiet) toast(r.error || 'That did not work', true); return r; };
    if (app.wsOpen) {
      const id = app.nextId++;
      return new Promise((resolve) => {
        app.replies.set(id, (m) => resolve(done(m)));
        app.ws.send(JSON.stringify({ id, action, args: args || {} }));
      });
    }
    if (action === 'ping') return Promise.resolve({ ok: true });
    return api('/api/action/' + action, args || {}).then((r) => {
      if (r.status === 401) { showPair('This device is no longer paired.'); return { ok: false }; }
      return done({ ok: r.status === 200, error: r.data.error });
    }, () => done({ ok: false, error: 'Cannot reach FreeBeams' }));
  }

  // ---- render: state → page --------------------------------------------------------------
  function canControl() { return !!(app.session && (app.session.permissions & PERM_CONTROL)); }

  function render() {
    const st = app.st;
    if (!st || $('main').hidden) return;
    $('server-name').textContent = st.name || (app.session && app.session.name) || 'FreeBeams';
    $('sched-title').textContent = st.title ? 'Schedule: ' + st.title : '';
    renderLive(st);
    for (const b of document.querySelectorAll('.mode')) {
      b.setAttribute('aria-pressed', String(st.mode === b.dataset.mode));
      b.disabled = !canControl();
    }
    const live = st.live, hasLive = live.index >= 0;
    $('prev').disabled = !canControl() || !hasLive || live.index <= 0;
    $('next').disabled = !canControl() || !hasLive || live.index >= live.slides.length - 1;
    for (const t of document.querySelectorAll('.tab')) t.setAttribute('aria-selected', String(t.dataset.tab === app.tab));
    for (const p of ['schedule', 'slides', 'songs']) $('pane-' + p).hidden = p !== app.tab;
    renderSchedule(st);
    renderSlides(st);
    renderMusic(st);
  }

  // The background audio playlist: the current track and play / pause.
  function renderMusic(st) {
    const a = st.audio || { tracks: 0 };
    $('music').hidden = !a.tracks;
    if (!a.tracks) return;
    $('music').dataset.status = a.status;
    $('music-label').textContent = a.status === 'playing' ? 'Music playing' : a.status === 'paused' ? 'Music paused' : 'Music stopped';
    $('music-title').textContent = a.title || '';
    $('music-play').dataset.playing = String(a.status === 'playing');
    for (const id of ['music-prev', 'music-play', 'music-next']) $(id).disabled = !canControl();
  }

  function renderLive(st) {
    const live = st.live, box = $('live');
    let mode = 'none', badge = 'Nothing live';
    if (!st.output) { mode = 'off'; badge = 'Output off'; }
    else if (st.mode === 'black') { mode = 'black'; badge = 'Black'; }
    else if (st.mode === 'clear') { mode = 'clear'; badge = 'Clear'; }
    else if (st.mode === 'logo') { mode = 'logo'; badge = 'Logo'; }
    else if (live.index >= 0) { mode = 'live'; badge = 'Live'; }
    box.dataset.mode = mode;
    $('live-badge').textContent = badge;
    $('live-title').textContent = live.title || '';
    $('live-pos').textContent = live.index >= 0 ? (live.index + 1) + ' / ' + live.slides.length : '';
    $('live-label').textContent = live.index >= 0 ? live.label : '';
    const text = $('live-text');
    let body = live.index >= 0 ? live.text : '';
    if (live.index >= 0 && !body) {
      const kind = live.slides[live.index] && live.slides[live.index].kind;
      body = kind === 'media' ? 'Image / video' : kind === 'presentation' ? 'Presentation slide' : '(no text)';
    }
    text.classList.toggle('placeholder', live.index < 0);
    text.textContent = live.index >= 0 ? body : 'Pick an item in the schedule and send it live.';
  }

  function renderSchedule(st) {
    const key = JSON.stringify([st.schedule, st.selected, st.liveItem, canControl()]);
    if (app.drawn.schedule === key) return;
    app.drawn.schedule = key;
    const ul = $('schedule');
    ul.textContent = '';
    $('schedule-empty').hidden = st.schedule.length > 0;
    st.schedule.forEach((it, i) => {
      const li = el('li', 'row');
      if (i === st.selected) li.classList.add('selected');
      if (i === st.liveItem) li.classList.add('live');
      const icon = el('div', 'row-icon');
      icon.appendChild(svg(ICONS[it.kind] || ICONS.song));
      const body = el('div', 'row-body');
      const title = el('div', 'row-title');
      if (i === st.liveItem) title.appendChild(el('span', 'chip', 'LIVE'));
      else if (i === st.selected) title.appendChild(el('span', 'chip preview', 'PREVIEW'));
      title.appendChild(document.createTextNode(it.title));
      const parts = [];
      if (it.subtitle) parts.push(it.subtitle);
      parts.push(it.slides === 1 ? '1 slide' : it.slides + ' slides');
      body.append(title, el('div', 'row-sub', parts.join(' · ')));
      li.append(icon, body);
      if (canControl()) {
        const go = el('button', 'go');
        go.type = 'button';
        go.setAttribute('aria-label', 'Go live with ' + it.title);
        go.appendChild(svg(ICONS.play));
        go.addEventListener('click', (e) => { e.stopPropagation(); goLiveItem(i); });
        li.appendChild(go);
        pressable(li, () => previewItem(i), () => goLiveItem(i));
      }
      ul.appendChild(li);
    });
  }

  function previewItem(i) {
    send('preview-item', { index: i });
    app.deck = 'preview';
    app.deckChosen = true;
    app.tab = 'slides';
    render();
    $('pane-slides').parentElement.scrollTop = 0;
  }
  function goLiveItem(i) {
    send('live-item', { index: i });
    app.deck = 'live';
    app.deckChosen = false;
    app.tab = 'slides';
    render();
  }

  function renderSlides(st) {
    // The Slides tab follows what is live unless the operator picked Preview.
    if (!app.deckChosen) app.deck = st.live.index >= 0 || st.preview.slides.length === 0 ? 'live' : 'preview';
    const deck = app.deck === 'live' ? st.live : st.preview;
    for (const b of document.querySelectorAll('.seg-btn')) b.setAttribute('aria-pressed', String(b.dataset.deck === app.deck));
    $('deck-title').textContent = deck.title || '';
    $('deck-golive').hidden = app.deck !== 'preview' || deck.slides.length === 0 || !canControl();
    const sameAsLive = app.deck === 'preview' && st.preview.item >= 0 && st.preview.item === st.live.item;
    const key = JSON.stringify([app.deck, deck.title, deck.slides, deck.index, st.live.index, sameAsLive, canControl()]);
    if (app.drawn.slides === key) return;
    const deckChanged = !app.drawn.slidesDeck || app.drawn.slidesDeck !== app.deck + '\u0000' + deck.title;
    app.drawn.slides = key;
    app.drawn.slidesDeck = app.deck + '\u0000' + deck.title;
    const ol = $('slides');
    ol.textContent = '';
    const empty = $('slides-empty');
    empty.hidden = deck.slides.length > 0;
    empty.textContent = app.deck === 'live' ? 'Nothing is live yet.' : 'Tap an item in the schedule to preview it.';
    let current = null;
    deck.slides.forEach((s, i) => {
      const li = el('li', 'slide');
      const onAir = app.deck === 'live' ? i === deck.index : sameAsLive && i === st.live.index;
      if (onAir) li.classList.add('on-air');
      else if (app.deck === 'preview' && i === deck.index) li.classList.add('current');
      if (onAir || (app.deck === 'preview' && i === deck.index)) current = current || li;
      const body = el('div', 'slide-body');
      body.append(el('div', 'slide-label', s.label), el('div', 'slide-snippet', s.snippet || (s.kind === 'media' ? 'Image / video' : '')));
      li.append(el('div', 'slide-num', String(i + 1)), body);
      if (canControl()) {
        li.addEventListener('click', () => {
          if (app.deck === 'live') send('live-slide', { index: i });
          else { send('go-live', { slide: i }); app.deck = 'live'; app.deckChosen = false; }
        });
      }
      ol.appendChild(li);
    });
    if (current && !$('pane-slides').hidden) {
      if (deckChanged) $('pane-slides').parentElement.scrollTop = 0;
      current.scrollIntoView({ block: 'nearest', behavior: deckChanged ? 'auto' : 'smooth' });
    }
  }

  // pressable: tap → onTap, hold (550 ms) → onHold, without the iOS callout.
  function pressable(node, onTap, onHold) {
    let timer = 0, held = false, x = 0, y = 0;
    const cancel = () => { clearTimeout(timer); timer = 0; node.classList.remove('pressing'); };
    node.addEventListener('pointerdown', (e) => {
      if (e.button) return;
      held = false; x = e.clientX; y = e.clientY;
      node.classList.add('pressing');
      timer = setTimeout(() => { timer = 0; held = true; node.classList.remove('pressing'); haptic(); onHold(); }, 550);
    });
    node.addEventListener('pointermove', (e) => { if (timer && Math.hypot(e.clientX - x, e.clientY - y) > 10) cancel(); });
    node.addEventListener('pointerup', cancel);
    node.addEventListener('pointercancel', cancel);
    node.addEventListener('pointerleave', cancel);
    node.addEventListener('contextmenu', (e) => e.preventDefault());
    node.addEventListener('click', () => { if (held) { held = false; return; } onTap(); });
  }

  // ---- songs ------------------------------------------------------------------------------
  let songTimer = 0, songSeq = 0;
  async function searchSongs() {
    const q = $('song-q').value.trim(), seq = ++songSeq;
    let r;
    try { r = await api('/api/songs?q=' + encodeURIComponent(q)); } catch (e) { return; }
    if (seq !== songSeq) return;
    const ul = $('songs');
    ul.textContent = '';
    const songs = (r.data && r.data.songs) || [];
    $('songs-empty').hidden = songs.length > 0;
    $('songs-empty').textContent = q ? 'No songs match “' + q + '”.' : 'The song library is empty.';
    for (const s of songs.slice(0, 60)) {
      const li = el('li', 'row');
      const icon = el('div', 'row-icon');
      icon.appendChild(svg(ICONS.song));
      const body = el('div', 'row-body');
      body.append(el('div', 'row-title', s.title), el('div', 'row-sub', s.author || ''));
      const add = el('button', 'add', 'Add');
      add.type = 'button';
      add.addEventListener('click', async () => {
        add.disabled = true;
        const res = await send('add-song', { id: s.id });
        add.disabled = false;
        if (res.ok) { add.textContent = 'Added'; add.classList.add('done'); toast('Added “' + s.title + '” to the schedule'); }
      });
      li.append(icon, body, add);
      ul.appendChild(li);
    }
  }
  $('song-q').addEventListener('input', () => { clearTimeout(songTimer); songTimer = setTimeout(searchSongs, 250); });

  // ---- wiring -------------------------------------------------------------------------------
  for (const t of document.querySelectorAll('.tab')) {
    t.addEventListener('click', () => {
      app.tab = t.dataset.tab;
      render();
      if (app.tab === 'songs' && !$('songs').children.length) searchSongs();
    });
  }
  for (const b of document.querySelectorAll('.seg-btn')) {
    b.addEventListener('click', () => { app.deck = b.dataset.deck; app.deckChosen = true; render(); });
  }
  for (const b of document.querySelectorAll('.mode')) {
    b.addEventListener('click', () => {
      if (!app.st) return;
      send(b.dataset.mode, { on: app.st.mode !== b.dataset.mode });
    });
  }
  $('prev').addEventListener('click', () => send('prev'));
  $('music-play').addEventListener('click', () => send('audio-toggle'));
  $('music-prev').addEventListener('click', () => send('audio-prev'));
  $('music-next').addEventListener('click', () => send('audio-next'));
  $('next').addEventListener('click', () => send('next'));
  $('deck-golive').addEventListener('click', () => { send('go-live'); app.deck = 'live'; app.deckChosen = false; render(); });
  $('menu-btn').addEventListener('click', () => { $('sheet').hidden = false; });
  $('sheet-close').addEventListener('click', () => { $('sheet').hidden = true; });
  $('sheet').addEventListener('click', (e) => { if (e.target === $('sheet')) $('sheet').hidden = true; });
  $('forget').addEventListener('click', async () => {
    $('sheet').hidden = true;
    try { await api('/api/forget', {}); } catch (e) { /* unpaired anyway below */ }
    showPair('This device was forgotten. Pair again to use it.');
  });

  boot();
})();
