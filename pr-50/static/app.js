// SPDX-License-Identifier: GPL-2.0-or-later
//
// tally — local-first client app (epic #1).
//
// The whole product runs here in the browser: a generic "tally" (a Trello-card
// container) with attachments — a note, a checklist, and tags — persisted in
// IndexedDB. No server, no network, no auth, no sync. Reload is the test:
// everything survives it because it lives in this browser's IndexedDB.
//
// Data model (one object store, `tallies`, keyed by id):
//   Tally { id, title, note, tags:[{name,color}], items:[{id,text,done}],
//           createdAt, updatedAt }
// Attachments are embedded in the tally document — the simplest thing that
// supports create/read/update/delete of a tally and of its attachments.

(() => {
  'use strict';

  // ---------- IndexedDB ----------
  const DB_NAME = 'tally';
  const STORE = 'tallies';
  let _db = null;

  function openDB() {
    return new Promise((resolve, reject) => {
      const req = indexedDB.open(DB_NAME, 1);
      req.onupgradeneeded = () => {
        const db = req.result;
        if (!db.objectStoreNames.contains(STORE)) {
          const os = db.createObjectStore(STORE, { keyPath: 'id' });
          os.createIndex('updatedAt', 'updatedAt');
        }
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error);
    });
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
  const dbDel = (id) => tx('readwrite', (s) => s.delete(id));

  // ---------- state ----------
  let tallies = []; // in-memory source of truth, mirrored to IndexedDB
  const now = () => Date.now();
  const uid = (p) => p + Date.now().toString(36) + Math.random().toString(36).slice(2, 7);
  const byId = (id) => tallies.find((t) => t.id === id);

  const PALETTE = ['red', 'amber', 'green', 'blue', 'pink', 'cyan'];

  async function load() {
    tallies = await dbAll();
  }
  async function persist(t) {
    t.updatedAt = now();
    await dbPut(t);
    ticker();
  }
  async function createTally(title) {
    const t = {
      id: uid('t_'), title: title.trim(), note: '', tags: [], items: [],
      createdAt: now(), updatedAt: now(),
    };
    tallies.push(t);
    await dbPut(t);
    ticker();
    return t;
  }
  async function removeTally(t) {
    tallies = tallies.filter((x) => x !== t);
    await dbDel(t.id);
    ticker();
  }

  // ---------- helpers ----------
  const view = () => document.getElementById('view');
  const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  const sortedTallies = () => [...tallies].sort((a, b) => b.updatedAt - a.updatedAt);

  function itemStats(t) {
    const total = t.items.length;
    const done = t.items.filter((i) => i.done).length;
    return { total, done };
  }

  function matches(t, q) {
    if (!q) return true;
    q = q.toLowerCase();
    if (t.title.toLowerCase().includes(q)) return true;
    if ((t.note || '').toLowerCase().includes(q)) return true;
    if (t.tags.some((tag) => tag.name.toLowerCase().includes(q))) return true;
    if (t.items.some((i) => i.text.toLowerCase().includes(q))) return true;
    return false;
  }

  // ---------- status ticker ----------
  function ticker() {
    const el = document.getElementById('ticker');
    if (!el) return;
    let done = 0, total = 0, tags = new Set();
    for (const t of tallies) {
      const s = itemStats(t);
      done += s.done; total += s.total;
      for (const g of t.tags) tags.add(g.name);
    }
    el.innerHTML =
      `<span class="stat">TALLIES <b>${tallies.length}</b></span>` +
      `<span class="stat">ITEMS <b class="up">${done}</b>/<b>${total}</b></span>` +
      `<span class="stat">TAGS <b>${tags.size}</b></span>`;
  }

  // ---------- list view ----------
  function renderList() {
    view().innerHTML = `
      <p class="lede">Your tallies — a container for anything. Make one, then attach notes, checklist items, and tags.</p>
      <section class="section">
        <h2><span>New tally</span></h2>
        <div class="section-body">
          <form class="row" id="new-form" autocomplete="off">
            <input class="input" id="new-title" type="text" placeholder="e.g. Kitchen renovation, Groceries, Trip to Lisbon" style="flex:1 1 12rem"/>
            <button class="btn primary" type="submit">Add tally</button>
          </form>
        </div>
      </section>
      <section class="section">
        <h2><span>Tallies</span></h2>
        <div class="section-body stack">
          <label class="field"><span>Search</span><input class="input" id="search" type="search" placeholder="Filter by title, note, item, or tag…"/></label>
          <div id="tally-list" class="stack"></div>
        </div>
      </section>`;
    renderCards('');
    const title = document.getElementById('new-title');
    if (title) title.focus();
  }

  function renderCards(q) {
    const list = document.getElementById('tally-list');
    if (!list) return;
    const rows = sortedTallies().filter((t) => matches(t, q));
    if (rows.length === 0) {
      list.innerHTML = `<p class="swatch-note">${tallies.length === 0 ? 'No tallies yet — add one above.' : 'Nothing matches that search.'}</p>`;
      return;
    }
    list.innerHTML = rows.map(cardHTML).join('');
  }

  function cardHTML(t) {
    const s = itemStats(t);
    const bits = [];
    if (s.total) bits.push(`<span class="num">${s.done}/${s.total}</span> done`);
    if (t.note) bits.push('note');
    const meta = bits.length ? bits.join(' · ') : 'empty';
    const tags = t.tags.map((g) => `<span class="lab ${esc(g.color)}">${esc(g.name)}</span>`).join('');
    return `
      <a class="card tally-card" href="#/t/${esc(t.id)}">
        <div class="title">${esc(t.title) || '<span class="untitled">untitled</span>'}</div>
        ${tags ? `<div class="row" style="margin-top:8px">${tags}</div>` : ''}
        <div class="meta">${meta} · updated ${fmtDate(t.updatedAt)}</div>
      </a>`;
  }

  function fmtDate(ms) {
    const d = new Date(ms);
    const p = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
  }

  // ---------- detail view ----------
  function renderDetail(t) {
    const items = t.items.map((i) => `
      <label class="check strike" data-item="${esc(i.id)}">
        <input type="checkbox" ${i.done ? 'checked' : ''}/>
        <span class="box">${CHECK_SVG}</span>
        <span class="lbl">${esc(i.text)}</span>
        <button class="x" type="button" data-del-item="${esc(i.id)}" aria-label="remove item">&times;</button>
      </label>`).join('');

    const tags = t.tags.map((g) => `
      <span class="chip tag-chip"><span class="lab ${esc(g.color)}">${esc(g.name)}</span>
        <button class="x" type="button" data-del-tag="${esc(g.name)}" aria-label="remove tag">&times;</button></span>`).join('');

    view().innerHTML = `
      <p class="lede"><a href="#/" class="back">← all tallies</a></p>

      <section class="section">
        <h2><span>Tally</span><button class="btn danger sm" id="delete" type="button">Delete</button></h2>
        <div class="section-body stack">
          <label class="field"><span>Title</span><input class="input" id="title" type="text" value="${esc(t.title)}" placeholder="Untitled"/></label>
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
          <textarea class="textarea" id="note" placeholder="Write anything down…">${esc(t.note)}</textarea>
        </div>
      </section>`;
  }

  const CHECK_SVG = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12l5 5L20 6"/></svg>';

  // ---------- router ----------
  function route() {
    const m = location.hash.match(/^#\/t\/(.+)$/);
    if (m) {
      const t = byId(m[1]);
      if (t) { renderDetail(t); return; }
      location.hash = '#/'; // unknown id → back to list
      return;
    }
    renderList();
  }

  // ---------- events (delegated on #view) ----------
  let noteTimer = null, titleTimer = null;

  function currentDetail() {
    const m = location.hash.match(/^#\/t\/(.+)$/);
    return m ? byId(m[1]) : null;
  }

  function onSubmit(e) {
    const f = e.target;
    if (f.id === 'new-form') {
      e.preventDefault();
      const input = document.getElementById('new-title');
      const title = input.value.trim();
      if (!title) return;
      createTally(title).then((t) => { location.hash = '#/t/' + t.id; });
      return;
    }
    if (f.id === 'item-form') {
      e.preventDefault();
      const t = currentDetail(); if (!t) return;
      const box = document.getElementById('item-text');
      const text = box.value.trim();
      if (!text) return;
      t.items.push({ id: uid('i_'), text, done: false });
      persist(t).then(() => renderDetail(t));
      return;
    }
    if (f.id === 'tag-form') {
      e.preventDefault();
      const t = currentDetail(); if (!t) return;
      const box = document.getElementById('tag-name');
      const name = box.value.trim();
      if (!name) return;
      if (!t.tags.some((g) => g.name.toLowerCase() === name.toLowerCase())) {
        t.tags.push({ name, color: PALETTE[t.tags.length % PALETTE.length] });
        persist(t).then(() => renderDetail(t));
      } else {
        box.value = '';
      }
      return;
    }
  }

  function onClick(e) {
    const back = e.target.closest('.back');
    if (back) return; // hash link handles it

    const delItem = e.target.closest('[data-del-item]');
    if (delItem) {
      e.preventDefault();
      const t = currentDetail(); if (!t) return;
      const id = delItem.getAttribute('data-del-item');
      t.items = t.items.filter((i) => i.id !== id);
      persist(t).then(() => renderDetail(t));
      return;
    }
    const delTag = e.target.closest('[data-del-tag]');
    if (delTag) {
      e.preventDefault();
      const t = currentDetail(); if (!t) return;
      const name = delTag.getAttribute('data-del-tag');
      t.tags = t.tags.filter((g) => g.name !== name);
      persist(t).then(() => renderDetail(t));
      return;
    }
    if (e.target.id === 'delete') {
      const t = currentDetail(); if (!t) return;
      if (confirm('Delete this tally? This cannot be undone.')) {
        removeTally(t).then(() => { location.hash = '#/'; });
      }
      return;
    }
  }

  function onChange(e) {
    const item = e.target.closest('[data-item]');
    if (item && e.target.matches('input[type=checkbox]')) {
      const t = currentDetail(); if (!t) return;
      const id = item.getAttribute('data-item');
      const i = t.items.find((x) => x.id === id);
      if (i) { i.done = e.target.checked; persist(t); }
      return;
    }
  }

  function onInput(e) {
    if (e.target.id === 'search') {
      renderCards(e.target.value);
      return;
    }
    if (e.target.id === 'note') {
      const t = currentDetail(); if (!t) return;
      clearTimeout(noteTimer);
      noteTimer = setTimeout(() => { t.note = e.target.value; persist(t); }, 350);
      return;
    }
    if (e.target.id === 'title') {
      const t = currentDetail(); if (!t) return;
      clearTimeout(titleTimer);
      titleTimer = setTimeout(() => { t.title = e.target.value.trim(); persist(t); }, 350);
      return;
    }
  }

  // ---------- boot ----------
  async function boot() {
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
