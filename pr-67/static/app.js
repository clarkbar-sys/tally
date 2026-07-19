// SPDX-License-Identifier: GPL-2.0-or-later
//
// tally — local-first client app (epic #1).
//
// The whole product runs here in the browser. A "notch" is a generic
// container — a mark carved on your tally — that you attach things to: a note,
// a checklist, tags, and other notches (sub-notches, parent/child like GitHub
// sub-issues). Everything persists in IndexedDB: no server, no network, no
// auth, no sync. Reload is the test — it all survives, because it lives in this
// browser.
//
// (A "tally" — resolving/tallying-up notches — is a later concept; v1 is just
// the notches and their attachments.)
//
// Data model (one object store, `notches`, keyed by id):
//   Notch { id, title, note, tags:[{name,color}], items:[{id,text,done}],
//           parentId:string|null, status:'open'|'done'|'not_planned',
//           createdAt, updatedAt }
// A sub-notch is an ordinary notch whose parentId points at its parent. Notes,
// checklists, and tags are embedded in the notch document. Any notch can be
// re-parented after the fact (the "Parent" picker on the detail view) to group
// existing notches — the only restriction is that a notch can't be moved under
// itself or one of its own descendants, which would make a cycle. A notch is
// never deleted — it's closed as done or not planned (and can be reopened),
// same as a GitHub issue. Records from before `status` existed are treated as
// 'open'.

(() => {
  'use strict';

  // ---------- IndexedDB ----------
  const DB_NAME = 'tally';
  const DB_VERSION = 2;
  const STORE = 'notches';
  const OLD_STORE = 'tallies'; // v1 name, migrated forward
  let _db = null;

  function openDB() {
    return new Promise((resolve, reject) => {
      const req = indexedDB.open(DB_NAME, DB_VERSION);
      req.onupgradeneeded = () => {
        const db = req.result;
        const upgradeTx = req.transaction;
        if (!db.objectStoreNames.contains(STORE)) {
          db.createObjectStore(STORE, { keyPath: 'id' });
        }
        // v1 → v2: carry old `tallies` forward as top-level notches, then drop
        // the old store. Existing local data is preserved, not wiped.
        if (db.objectStoreNames.contains(OLD_STORE)) {
          const src = upgradeTx.objectStore(OLD_STORE);
          const dest = upgradeTx.objectStore(STORE);
          src.openCursor().onsuccess = (ev) => {
            const cur = ev.target.result;
            if (cur) {
              const rec = cur.value;
              if (rec.parentId === undefined) rec.parentId = null;
              dest.put(rec);
              cur.continue();
            } else {
              db.deleteObjectStore(OLD_STORE);
            }
          };
        }
      };
      // Another tab holds an older version open, blocking this upgrade. The
      // request stays pending; show a notice and let it resolve on its own once
      // the other tab closes — no reload needed. (Without this the promise would
      // hang forever and the app would never render — you couldn't make notches.)
      req.onblocked = () => notice(
        'tally is open in another tab (or an older version). Close the other tab — this page resumes automatically.');
      req.onsuccess = () => {
        const db = req.result;
        // If a newer version opens elsewhere later, yield so we never block it.
        db.onversionchange = () => {
          db.close();
          notice('A newer version of tally opened in another tab. Reload this page to continue.');
        };
        resolve(db);
      };
      req.onerror = () => reject(req.error || new Error('could not open IndexedDB'));
    });
  }

  // notice replaces the whole view with a message — for blocked/errored storage
  // states, so the screen is never silently blank.
  function notice(msg) {
    const v = view();
    if (v) v.innerHTML = `<p class="lede" style="padding:var(--sp-4)">${esc(msg)}</p>`;
  }

  // reportWriteError surfaces a failed persist instead of silently dropping it.
  function reportWriteError(err) {
    const m = err && err.message ? err.message : String(err);
    alert('tally could not save to local storage: ' + m);
  }

  function tx(mode, fn) {
    return new Promise((resolve, reject) => {
      const t = _db.transaction(STORE, mode);
      const store = t.objectStore(STORE);
      const out = fn(store);
      t.oncomplete = () => resolve(out && out.result !== undefined ? out.result : out);
      t.onerror = () => reject(t.error);
      t.onabort = () => reject(t.error);
    });
  }

  const dbAll = () => tx('readonly', (s) => s.getAll());
  const dbPut = (rec) => tx('readwrite', (s) => s.put(rec));

  // ---------- state ----------
  let notches = []; // in-memory source of truth, mirrored to IndexedDB
  const now = () => Date.now();
  const uid = (p) => p + Date.now().toString(36) + Math.random().toString(36).slice(2, 7);
  const byId = (id) => notches.find((n) => n.id === id);

  const PALETTE = ['red', 'amber', 'green', 'blue', 'pink', 'cyan'];

  const topLevel = () => sortByUpdated(notches.filter((n) => !n.parentId));
  const childrenOf = (id) => sortByUpdated(notches.filter((n) => n.parentId === id));
  const sortByUpdated = (arr) => [...arr].sort((a, b) => b.updatedAt - a.updatedAt);

  // ancestors of n, oldest → nearest (for the breadcrumb)
  function trail(n) {
    const out = [];
    const seen = new Set();
    let cur = n;
    while (cur && cur.parentId && !seen.has(cur.parentId)) {
      seen.add(cur.parentId);
      const p = byId(cur.parentId);
      if (!p) break;
      out.unshift(p);
      cur = p;
    }
    return out;
  }

  // n plus every notch beneath it (for the move guard)
  function subtree(n) {
    const out = [n];
    for (const c of notches.filter((x) => x.parentId === n.id)) out.push(...subtree(c));
    return out;
  }

  // A readable "A / B / C" path for a notch, so nested targets in the move
  // picker are distinguishable when titles repeat.
  function pathLabel(n) {
    return trail(n).concat(n).map((x) => x.title.trim() || 'untitled').join(' / ');
  }

  // Valid new parents for n: every notch that isn't n or one of n's own
  // descendants. Moving n under its own subtree would create a cycle.
  function moveTargets(n) {
    const blocked = new Set(subtree(n).map((x) => x.id));
    return sortByUpdated(notches.filter((x) => !blocked.has(x.id)));
  }

  async function load() {
    notches = await dbAll();
  }
  async function persist(n) {
    n.updatedAt = now();
    try {
      await dbPut(n);
    } catch (err) {
      reportWriteError(err);
      return;
    }
    ticker();
  }
  async function createNotch(title, parentId) {
    const n = {
      id: uid('n_'), title: title.trim(), note: '', tags: [], items: [],
      parentId: parentId || null, status: 'open', createdAt: now(), updatedAt: now(),
    };
    try {
      await dbPut(n); // persist first, then adopt into memory, so the two agree
    } catch (err) {
      reportWriteError(err);
      throw err;
    }
    notches.push(n);
    ticker();
    return n;
  }
  const notchStatus = (n) => n.status || 'open';
  async function setStatus(n, status) {
    n.status = status;
    await persist(n);
  }

  // ---------- helpers ----------
  const view = () => document.getElementById('view');
  const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  function itemStats(n) {
    return { total: n.items.length, done: n.items.filter((i) => i.done).length };
  }

  // parseQuery pulls `is:` status tokens (e.g. "is:open", "is:closed") out of a
  // search string, GitHub-issue-search style, leaving the rest as free text.
  // "closed" matches any non-open status, mirroring the open/closed split
  // notches otherwise present to the user.
  function parseQuery(q) {
    const tokens = (q || '').trim().split(/\s+/).filter(Boolean);
    const statuses = [];
    const text = [];
    for (const t of tokens) {
      const m = /^is:(open|closed|done|not_planned)$/i.exec(t);
      if (m) statuses.push(m[1].toLowerCase());
      else text.push(t);
    }
    return { statuses, text: text.join(' ') };
  }

  function matchesStatus(n, statuses) {
    if (!statuses.length) return true;
    const s = notchStatus(n);
    return statuses.some((st) => (st === 'closed' ? s !== 'open' : s === st));
  }

  function matches(n, q) {
    const { statuses, text } = parseQuery(q);
    if (!matchesStatus(n, statuses)) return false;
    if (!text) return true;
    const t = text.toLowerCase();
    return n.title.toLowerCase().includes(t)
      || (n.note || '').toLowerCase().includes(t)
      || n.tags.some((g) => g.name.toLowerCase().includes(t))
      || n.items.some((i) => i.text.toLowerCase().includes(t));
  }

  function fmtDate(ms) {
    const d = new Date(ms);
    const p = (x) => String(x).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
  }

  const CHECK_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12l5 5L20 6"/></svg>';

  // ---------- status ticker ----------
  function ticker() {
    const el = document.getElementById('ticker');
    if (!el) return;
    let done = 0, total = 0;
    const tags = new Set();
    for (const n of notches) {
      const s = itemStats(n);
      done += s.done; total += s.total;
      for (const g of n.tags) tags.add(g.name);
    }
    const open = notches.filter((n) => notchStatus(n) === 'open').length;
    el.innerHTML =
      `<span class="stat">NOTCHES <b>${open}</b></span>` +
      `<span class="stat">ITEMS <b class="up">${done}</b>/<b>${total}</b></span>` +
      `<span class="stat">TAGS <b>${tags.size}</b></span>`;
  }

  // ---------- cards ----------
  function statusLab(n) {
    const status = notchStatus(n);
    if (status === 'done') return '<span class="lab green">done</span>';
    if (status === 'not_planned') return '<span class="lab gray">not planned</span>';
    return '';
  }

  function cardHTML(n) {
    const s = itemStats(n);
    const kids = childrenOf(n.id).length;
    const bits = [];
    if (kids) bits.push(`<span class="num">${kids}</span> sub-notch${kids === 1 ? '' : 'es'}`);
    if (s.total) bits.push(`<span class="num">${s.done}/${s.total}</span> done`);
    if (n.note) bits.push('note');
    const meta = bits.length ? bits.join(' · ') : 'empty';
    const tags = statusLab(n) + n.tags.map((g) => `<span class="lab ${esc(g.color)}">${esc(g.name)}</span>`).join('');
    return `
      <a class="card notch-card" href="#/n/${esc(n.id)}">
        <div class="title">${n.title ? esc(n.title) : '<span class="untitled">untitled</span>'}</div>
        ${tags ? `<div class="row" style="margin-top:8px">${tags}</div>` : ''}
        <div class="meta">${meta} · updated ${fmtDate(n.updatedAt)}</div>
      </a>`;
  }

  // ---------- list view ----------
  const DEFAULT_QUERY = 'is:open';

  function renderList() {
    view().innerHTML = `
      <p class="lede">Your notches — a mark for anything. Make one, then attach notes, checklist items, tags, and sub-notches.</p>
      <section class="section">
        <h2><span>New notch</span></h2>
        <div class="section-body">
          <form class="row" id="new-form" autocomplete="off">
            <input class="input" id="new-title" type="text" placeholder="e.g. Kitchen renovation, Groceries, Trip to Lisbon" style="flex:1 1 12rem"/>
            <button class="btn primary" type="submit">Add notch</button>
          </form>
        </div>
      </section>
      <section class="section">
        <h2><span>Notches</span></h2>
        <div class="section-body stack">
          <label class="field"><span>Search</span><input class="input" id="search" type="search" value="${esc(DEFAULT_QUERY)}" placeholder="Filter by title, note, item, or tag… (try is:open, is:closed)"/></label>
          <div id="notch-list" class="stack"></div>
        </div>
      </section>`;
    renderCards(DEFAULT_QUERY);
    const title = document.getElementById('new-title');
    if (title) title.focus();
  }

  function renderCards(q) {
    const list = document.getElementById('notch-list');
    if (!list) return;
    const rows = topLevel().filter((n) => matches(n, q));
    if (rows.length === 0) {
      list.innerHTML = `<p class="swatch-note">${topLevel().length === 0 ? 'No notches yet — add one above.' : 'Nothing matches that search.'}</p>`;
      return;
    }
    list.innerHTML = rows.map(cardHTML).join('');
  }

  // ---------- detail view ----------
  function renderDetail(n) {
    const crumbs = [`<a href="#/" class="back">all notches</a>`]
      .concat(trail(n).map((a) => `<a href="#/n/${esc(a.id)}" class="back">${esc(a.title) || 'untitled'}</a>`))
      .join('<span class="crumb-sep"> / </span>');

    const kids = childrenOf(n.id);
    const subCards = kids.length
      ? kids.map(cardHTML).join('')
      : '<span class="swatch-note">No sub-notches.</span>';

    const items = n.items.map((i) => `
      <label class="check strike" data-item="${esc(i.id)}">
        <input type="checkbox" ${i.done ? 'checked' : ''}/>
        <span class="box">${CHECK_SVG}</span>
        <span class="lbl">${esc(i.text)}</span>
        <button class="x" type="button" data-del-item="${esc(i.id)}" aria-label="remove item">&times;</button>
      </label>`).join('');

    const tags = n.tags.map((g) => `
      <span class="chip tag-chip"><span class="lab ${esc(g.color)}">${esc(g.name)}</span>
        <button class="x" type="button" data-del-tag="${esc(g.name)}" aria-label="remove tag">&times;</button></span>`).join('');

    const status = notchStatus(n);
    const closeControls = status === 'open'
      ? `<button class="btn ghost sm" id="close-not-planned" type="button">Not planned</button>
         <button class="btn primary sm" id="close-done" type="button">Close as done</button>`
      : `${statusLab(n)}<button class="btn ghost sm" id="reopen" type="button">Reopen</button>`;

    view().innerHTML = `
      <p class="lede">← ${crumbs}</p>

      <section class="section">
        <h2><span>Notch</span><span class="row" style="gap:8px">${closeControls}</span></h2>
        <div class="section-body stack">
          <label class="field"><span>Title</span><input class="input" id="title" type="text" value="${esc(n.title)}" placeholder="Untitled"/></label>
          <label class="field"><span>Parent</span>
            <select class="select" id="parent">
              <option value=""${n.parentId ? '' : ' selected'}>— top level —</option>
              ${moveTargets(n).map((t) => `<option value="${esc(t.id)}"${t.id === n.parentId ? ' selected' : ''}>${esc(pathLabel(t))}</option>`).join('')}
            </select>
          </label>
        </div>
      </section>

      <section class="section">
        <h2><span>Sub-notches</span></h2>
        <div class="section-body stack">
          <div class="stack" id="subs">${subCards}</div>
          <form class="row" id="sub-form" autocomplete="off">
            <input class="input" id="sub-title" type="text" placeholder="add a sub-notch…" style="flex:1 1 12rem"/>
            <button class="btn ghost sm" type="submit">Add sub-notch</button>
          </form>
        </div>
      </section>

      <section class="section">
        <h2><span>Tags</span></h2>
        <div class="section-body stack">
          <div class="row" id="tags">${tags || '<span class="swatch-note">No tags.</span>'}</div>
          <form class="row" id="tag-form" autocomplete="off">
            <input class="input" id="tag-name" type="text" placeholder="add a tag…" style="flex:1 1 8rem" maxlength="24"/>
            <button class="btn ghost sm" type="submit">Add tag</button>
          </form>
        </div>
      </section>

      <section class="section">
        <h2><span>Checklist</span></h2>
        <div class="section-body stack">
          <div class="stack" id="items">${items || '<span class="swatch-note">No items.</span>'}</div>
          <form class="row" id="item-form" autocomplete="off">
            <input class="input" id="item-text" type="text" placeholder="add an item…" style="flex:1 1 12rem"/>
            <button class="btn primary sm" type="submit">Add item</button>
          </form>
        </div>
      </section>

      <section class="section">
        <h2><span>Note</span></h2>
        <div class="section-body">
          <textarea class="textarea" id="note" placeholder="Write anything down…">${esc(n.note)}</textarea>
        </div>
      </section>`;
  }

  // ---------- router ----------
  function route() {
    const m = location.hash.match(/^#\/n\/(.+)$/);
    if (m) {
      const n = byId(m[1]);
      if (n) { renderDetail(n); return; }
      location.hash = '#/';
      return;
    }
    renderList();
  }

  function currentDetail() {
    const m = location.hash.match(/^#\/n\/(.+)$/);
    return m ? byId(m[1]) : null;
  }

  // ---------- events (delegated on #view) ----------
  let noteTimer = null, titleTimer = null;

  function onSubmit(e) {
    const f = e.target;
    if (f.id === 'new-form') {
      e.preventDefault();
      const input = document.getElementById('new-title');
      const title = input.value.trim();
      if (!title) return;
      createNotch(title, null).then((n) => { location.hash = '#/n/' + n.id; }).catch(() => {});
      return;
    }
    if (f.id === 'sub-form') {
      e.preventDefault();
      const parent = currentDetail(); if (!parent) return;
      const box = document.getElementById('sub-title');
      const title = box.value.trim();
      if (!title) return;
      createNotch(title, parent.id).then(() => renderDetail(parent)).catch(() => {});
      return;
    }
    if (f.id === 'item-form') {
      e.preventDefault();
      const n = currentDetail(); if (!n) return;
      const box = document.getElementById('item-text');
      const text = box.value.trim();
      if (!text) return;
      n.items.push({ id: uid('i_'), text, done: false });
      persist(n).then(() => renderDetail(n));
      return;
    }
    if (f.id === 'tag-form') {
      e.preventDefault();
      const n = currentDetail(); if (!n) return;
      const box = document.getElementById('tag-name');
      const name = box.value.trim();
      if (!name) return;
      if (!n.tags.some((g) => g.name.toLowerCase() === name.toLowerCase())) {
        n.tags.push({ name, color: PALETTE[n.tags.length % PALETTE.length] });
        persist(n).then(() => renderDetail(n));
      } else {
        box.value = '';
      }
      return;
    }
  }

  function onClick(e) {
    if (e.target.closest('.back')) return; // hash links navigate themselves

    const delItem = e.target.closest('[data-del-item]');
    if (delItem) {
      e.preventDefault();
      const n = currentDetail(); if (!n) return;
      const id = delItem.getAttribute('data-del-item');
      n.items = n.items.filter((i) => i.id !== id);
      persist(n).then(() => renderDetail(n));
      return;
    }
    const delTag = e.target.closest('[data-del-tag]');
    if (delTag) {
      e.preventDefault();
      const n = currentDetail(); if (!n) return;
      const name = delTag.getAttribute('data-del-tag');
      n.tags = n.tags.filter((g) => g.name !== name);
      persist(n).then(() => renderDetail(n));
      return;
    }
    if (e.target.id === 'close-done') {
      const n = currentDetail(); if (!n) return;
      setStatus(n, 'done').then(() => renderDetail(n));
      return;
    }
    if (e.target.id === 'close-not-planned') {
      const n = currentDetail(); if (!n) return;
      setStatus(n, 'not_planned').then(() => renderDetail(n));
      return;
    }
    if (e.target.id === 'reopen') {
      const n = currentDetail(); if (!n) return;
      setStatus(n, 'open').then(() => renderDetail(n));
      return;
    }
  }

  function onChange(e) {
    if (e.target.id === 'parent') {
      const n = currentDetail(); if (!n) return;
      const val = e.target.value || null;
      if (val === (n.parentId || null)) return; // no-op reselect
      n.parentId = val;
      persist(n).then(() => renderDetail(n)); // re-render: breadcrumb + siblings move
      return;
    }
    const item = e.target.closest('[data-item]');
    if (item && e.target.matches('input[type=checkbox]')) {
      const n = currentDetail(); if (!n) return;
      const id = item.getAttribute('data-item');
      const i = n.items.find((x) => x.id === id);
      if (i) { i.done = e.target.checked; persist(n); }
    }
  }

  function onInput(e) {
    if (e.target.id === 'search') {
      renderCards(e.target.value);
      return;
    }
    if (e.target.id === 'note') {
      const n = currentDetail(); if (!n) return;
      clearTimeout(noteTimer);
      noteTimer = setTimeout(() => { n.note = e.target.value; persist(n); }, 350);
      return;
    }
    if (e.target.id === 'title') {
      const n = currentDetail(); if (!n) return;
      clearTimeout(titleTimer);
      titleTimer = setTimeout(() => { n.title = e.target.value.trim(); persist(n); }, 350);
      return;
    }
  }

  // ---------- theme ----------
  // Themes are pure CSS (a [data-theme] token block in app.css); switching is
  // just setting data-theme on <html>. The choice is a per-browser preference
  // (tally is local-first — no account), stored in localStorage. An inline
  // script in <head> applies it before first paint; this wires the picker and
  // keeps the browser-chrome meta tags in step with the active theme.
  const THEME_KEY = 'tally.theme';
  const DEFAULT_THEME = 'paper';

  function setMetaContent(name, value) {
    let m = document.querySelector(`meta[name="${name}"]`);
    if (!m) { m = document.createElement('meta'); m.setAttribute('name', name); document.head.appendChild(m); }
    m.setAttribute('content', value);
  }

  // Read the active theme's own tokens so <meta theme-color>/color-scheme match
  // it — no per-theme table to maintain, it just follows the CSS.
  function syncThemeMeta() {
    const cs = getComputedStyle(document.documentElement);
    const bg = cs.getPropertyValue('--bg').trim();
    const scheme = (cs.colorScheme || cs.getPropertyValue('color-scheme') || '').trim();
    if (bg) setMetaContent('theme-color', bg);
    if (scheme) setMetaContent('color-scheme', scheme);
  }

  function applyTheme(id, persist) {
    document.documentElement.setAttribute('data-theme', id);
    if (persist) { try { localStorage.setItem(THEME_KEY, id); } catch (e) {} }
    syncThemeMeta();
  }

  function initTheme() {
    let stored = null;
    try { stored = localStorage.getItem(THEME_KEY); } catch (e) {}
    // The inline head script already set data-theme from storage; fall back to
    // that, then the default. Don't persist on load — only an explicit pick.
    const current = stored || document.documentElement.getAttribute('data-theme') || DEFAULT_THEME;
    applyTheme(current, false);
    const sel = document.getElementById('theme-select');
    if (sel) {
      sel.value = current;
      sel.addEventListener('change', () => applyTheme(sel.value, true));
    }
  }

  // ---------- boot ----------
  async function boot() {
    initTheme(); // independent of IndexedDB — theming works even if the DB can't open
    try {
      _db = await openDB();
      await load();
    } catch (err) {
      view().innerHTML = `<p class="lede" style="padding:var(--sp-4)">Couldn't open local storage: ${esc(err && err.message)}. tally needs IndexedDB (private-mode browsers may block it).</p>`;
      return;
    }
    ticker();
    const mount = view();
    mount.addEventListener('submit', onSubmit);
    mount.addEventListener('click', onClick);
    mount.addEventListener('change', onChange);
    mount.addEventListener('input', onInput);
    window.addEventListener('hashchange', route);
    route();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
