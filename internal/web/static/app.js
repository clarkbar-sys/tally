// SPDX-License-Identifier: GPL-2.0-or-later
//
// tally — local-first client app (epic #1).
//
// The whole product runs here in the browser. A "notch" is a generic
// container — a mark carved on your tally — that you edit much like a GitHub
// issue: a title, a Markdown description (where tasks live as `- [ ]` task
// lists, checked off inline), tags/labels, sub-notches (parent/child like
// GitHub sub-issues), and a running thread of comments for the paper trail.
// Everything persists in IndexedDB: no server, no network, no auth, no sync.
// Reload is the test — it all survives, because it lives in this browser.
//
// (A "tally" — resolving/tallying-up notches — is a later concept; v1 is just
// the notches and their attachments.)
//
// Data model (one object store, `notches`, keyed by id):
//   Notch { id, title, body, tags:[{name,color}],
//           comments:[{id, body, createdAt}],
//           parentId:string|null, status:'open'|'done'|'not_planned',
//           createdAt, updatedAt }
// `body` is the Markdown description; tasks are ordinary Markdown task-list
// items inside it (`- [ ]` / `- [x]`), not a separate structure — mirroring how
// GitHub issues carry their checklists. A sub-notch is an ordinary notch whose
// parentId points at its parent. Any notch can be re-parented after the fact
// (the "Parent" picker on the detail view) to group existing notches — the only
// restriction is that a notch can't be moved under itself or one of its own
// descendants, which would make a cycle. A notch is never deleted — it's closed
// as done or not planned (and can be reopened), same as a GitHub issue.
//
// Migration: pre-v3 records carried a plain-text `note` and a structured
// `items` checklist. On upgrade `note` becomes `body`, and any checklist items
// are appended to `body` as a Markdown task list, so nothing is lost. Records
// from before `status` existed are treated as 'open'.

(() => {
  'use strict';

  // ---------- IndexedDB ----------
  const DB_NAME = 'tally';
  const DB_VERSION = 3;
  const STORE = 'notches';
  const OLD_STORE = 'tallies'; // v1 name, migrated forward
  let _db = null;

  // upgradeNotch brings one stored record up to the current shape in place. It's
  // idempotent and defensive so it can run over old `tallies` rows, over v2
  // `notches`, or as a belt-and-braces pass on load. The big move is v2 → v3:
  // the plain-text `note` becomes the Markdown `body`, and the structured
  // `items` checklist is folded into that body as a Markdown task list, so a
  // user's checklist survives the redesign as editable Markdown.
  function upgradeNotch(rec) {
    if (rec.parentId === undefined) rec.parentId = null;
    if (rec.status === undefined) rec.status = 'open';
    if (rec.body === undefined) rec.body = typeof rec.note === 'string' ? rec.note : '';
    if (Array.isArray(rec.items) && rec.items.length) {
      const list = rec.items
        .map((i) => `- [${i.done ? 'x' : ' '}] ${String(i.text || '').trim()}`)
        .join('\n');
      rec.body = rec.body.trim() ? rec.body.replace(/\s+$/, '') + '\n\n' + list : list;
    }
    if (!Array.isArray(rec.comments)) rec.comments = [];
    delete rec.note;
    delete rec.items;
    return rec;
  }

  function openDB() {
    return new Promise((resolve, reject) => {
      const req = indexedDB.open(DB_NAME, DB_VERSION);
      req.onupgradeneeded = (ev) => {
        const db = req.result;
        const upgradeTx = req.transaction;
        const oldVersion = ev.oldVersion;
        if (!db.objectStoreNames.contains(STORE)) {
          db.createObjectStore(STORE, { keyPath: 'id' });
        }
        // v1 → v2: carry old `tallies` forward as top-level notches, then drop
        // the old store. Existing local data is preserved, not wiped.
        if (db.objectStoreNames.contains(OLD_STORE)) {
          const src = upgradeTx.objectStore(OLD_STORE);
          const dest = upgradeTx.objectStore(STORE);
          src.openCursor().onsuccess = (e) => {
            const cur = e.target.result;
            if (cur) {
              dest.put(upgradeNotch(cur.value));
              cur.continue();
            } else {
              db.deleteObjectStore(OLD_STORE);
            }
          };
        } else if (oldVersion >= 1 && oldVersion < 3) {
          // v2 → v3: rewrite existing notches in place (note → body,
          // items → Markdown task list, add comments). Rows already migrated
          // above (from `tallies`) are skipped — this only runs when there was
          // no old store to carry forward.
          const store = upgradeTx.objectStore(STORE);
          store.openCursor().onsuccess = (e) => {
            const cur = e.target.result;
            if (!cur) return;
            cur.update(upgradeNotch(cur.value));
            cur.continue();
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
    // Normalize in memory too, so any record the upgrade path didn't touch
    // (e.g. written by an older tab mid-migration) still presents the new shape.
    notches = (await dbAll()).map(upgradeNotch);
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
      id: uid('n_'), title: title.trim(), body: '', tags: [], comments: [],
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
  async function addComment(n, body) {
    const text = body.trim();
    if (!text) return;
    if (!Array.isArray(n.comments)) n.comments = [];
    n.comments.push({ id: uid('c_'), body: text, createdAt: now() });
    await persist(n);
  }
  async function deleteComment(n, id) {
    n.comments = (n.comments || []).filter((c) => c.id !== id);
    await persist(n);
  }

  // ---------- helpers ----------
  const view = () => document.getElementById('view');
  const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  // ---------- Markdown ----------
  // A small, dependency-free Markdown renderer — the app is client-only and
  // offline (GitHub Pages), so it can't pull in a library. It covers the subset
  // that matters for a notch body: headings, task lists (interactive), bullet
  // and ordered lists, blockquotes, fenced/inline code, links, bold/italic, and
  // horizontal rules. Everything is HTML-escaped before any markup is added, so
  // rendering user text can never inject HTML.
  const TASK_RE = /^(\s*[-*+]\s+\[)([ xX])(\])(\s+)([\s\S]*)$/;

  function mdInline(s) {
    let t = esc(s);
    t = t.replace(/`([^`]+)`/g, (m, c) => `<code>${c}</code>`);
    // links [text](http(s)://url) — scheme-restricted so no javascript: URIs
    t = t.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g,
      (m, label, url) => `<a href="${url}" target="_blank" rel="noopener noreferrer">${label}</a>`);
    t = t.replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>');
    t = t.replace(/(^|[^*])\*([^*\n]+)\*/g, '$1<em>$2</em>');
    t = t.replace(/(^|[^\w_])_([^_\n]+)_(?!\w)/g, '$1<em>$2</em>');
    return t;
  }

  // mdRender turns a Markdown string into safe HTML. Task-list items carry a
  // data-task index (their position among all task items, in document order) so
  // clicking one can flip the matching `- [ ]` in the source — see toggleTask.
  function mdRender(src) {
    const lines = String(src || '').replace(/\r\n?/g, '\n').split('\n');
    const out = [];
    let i = 0, taskIndex = 0;
    const isList = (l) => /^\s*([-*+]|\d+[.)])\s+/.test(l);
    const isOrdered = (l) => /^\s*\d+[.)]\s+/.test(l);
    while (i < lines.length) {
      const line = lines[i];
      if (/^\s*$/.test(line)) { i++; continue; }
      if (/^```/.test(line)) {
        const buf = [];
        i++;
        while (i < lines.length && !/^```/.test(lines[i])) { buf.push(lines[i]); i++; }
        i++; // closing fence
        out.push(`<pre class="md-code"><code>${esc(buf.join('\n'))}</code></pre>`);
        continue;
      }
      const h = /^(#{1,6})\s+(.*)$/.exec(line);
      if (h) { const l = h[1].length; out.push(`<h${l} class="md-h md-h${l}">${mdInline(h[2])}</h${l}>`); i++; continue; }
      if (/^\s*([-*_])(\s*\1){2,}\s*$/.test(line)) { out.push('<hr class="md-hr"/>'); i++; continue; }
      if (/^\s*>\s?/.test(line)) {
        const buf = [];
        while (i < lines.length && /^\s*>\s?/.test(lines[i])) { buf.push(lines[i].replace(/^\s*>\s?/, '')); i++; }
        out.push(`<blockquote class="md-quote">${mdInline(buf.join(' '))}</blockquote>`);
        continue;
      }
      if (isList(line)) {
        const ordered = isOrdered(line);
        const items = [];
        while (i < lines.length && isList(lines[i]) && isOrdered(lines[i]) === ordered) {
          const body = lines[i].replace(/^\s*([-*+]|\d+[.)])\s+/, '');
          const task = !ordered && TASK_RE.exec(lines[i]);
          if (task) {
            const done = task[2].toLowerCase() === 'x';
            const idx = taskIndex++;
            items.push(
              `<li class="md-task"><label class="check strike md-check">` +
              `<input type="checkbox" data-task="${idx}"${done ? ' checked' : ''}/>` +
              `<span class="box">${CHECK_SVG}</span>` +
              `<span class="lbl">${mdInline(task[5])}</span></label></li>`);
          } else {
            items.push(`<li>${mdInline(body)}</li>`);
          }
          i++;
        }
        const tag = ordered ? 'ol' : 'ul';
        out.push(`<${tag} class="md-list${ordered ? ' md-ol' : ''}">${items.join('')}</${tag}>`);
        continue;
      }
      const para = [];
      while (i < lines.length && !/^\s*$/.test(lines[i]) && !isList(lines[i]) &&
             !/^(#{1,6})\s+/.test(lines[i]) && !/^\s*>\s?/.test(lines[i]) && !/^```/.test(lines[i])) {
        para.push(lines[i]); i++;
      }
      out.push(`<p>${mdInline(para.join('\n')).replace(/\n/g, '<br/>')}</p>`);
    }
    return out.join('\n');
  }

  // toggleTask flips the Nth task checkbox (document order) in a Markdown body,
  // returning the new body. Mirrors mdRender's task counting exactly.
  function toggleTask(body, index) {
    const lines = String(body || '').split('\n');
    let n = 0;
    for (let k = 0; k < lines.length; k++) {
      const m = TASK_RE.exec(lines[k]);
      if (!m) continue;
      if (n === index) {
        const checked = m[2].toLowerCase() === 'x';
        lines[k] = lines[k].replace(TASK_RE, (_, a, b, c, sp, rest) => a + (checked ? ' ' : 'x') + c + sp + rest);
        break;
      }
      n++;
    }
    return lines.join('\n');
  }

  // taskStats counts the Markdown task-list items in a notch body — the
  // successor to the old structured checklist, read straight from the text.
  function taskStats(n) {
    let total = 0, done = 0;
    for (const line of String(n.body || '').split('\n')) {
      const m = TASK_RE.exec(line);
      if (m) { total++; if (m[2].toLowerCase() === 'x') done++; }
    }
    return { total, done };
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
      || (n.body || '').toLowerCase().includes(t)
      || n.tags.some((g) => g.name.toLowerCase().includes(t))
      || (n.comments || []).some((c) => c.body.toLowerCase().includes(t));
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
      const s = taskStats(n);
      done += s.done; total += s.total;
      for (const g of n.tags) tags.add(g.name);
    }
    const open = notches.filter((n) => notchStatus(n) === 'open').length;
    el.innerHTML =
      `<span class="stat"><b>${open}</b> open</span>` +
      `<span class="stat"><b class="up">${done}</b> of <b>${total}</b> tasks done</span>` +
      `<span class="stat"><b>${tags.size}</b> tags</span>`;
  }

  // ---------- cards ----------
  function statusLab(n) {
    const status = notchStatus(n);
    if (status === 'done') return '<span class="lab green">done</span>';
    if (status === 'not_planned') return '<span class="lab gray">not planned</span>';
    return '';
  }

  function cardHTML(n) {
    const s = taskStats(n);
    const kids = childrenOf(n.id).length;
    const comments = (n.comments || []).length;
    const bits = [];
    if (kids) bits.push(`<span class="num">${kids}</span> sub-notch${kids === 1 ? '' : 'es'}`);
    if (s.total) bits.push(`<span class="num">${s.done}/${s.total}</span> task${s.total === 1 ? '' : 's'}`);
    if (comments) bits.push(`<span class="num">${comments}</span> comment${comments === 1 ? '' : 's'}`);
    if ((n.body || '').trim() && !s.total) bits.push('description');
    const meta = bits.length ? bits.join(' · ') : 'empty';
    const tags = statusLab(n) + n.tags.map((g) => `<span class="lab ${esc(g.color)}">${esc(g.name)}</span>`).join('');
    return `
      <a class="notch-card" href="#/n/${esc(n.id)}">
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
        <h2><span>Notches</span></h2>
        <div class="section-body stack">
          <label class="field"><span>Search</span><input class="input" id="search" type="search" value="${esc(DEFAULT_QUERY)}" placeholder="Filter by title, description, comment, or tag… (try is:open, is:closed)"/></label>
          <div class="row" style="align-items:center; flex-wrap:nowrap">
            <div class="filter" id="filter" role="group" aria-label="Filter by status" style="flex:1 1 auto">
              <button type="button" data-status="open" aria-pressed="true"><span class="fdot" aria-hidden="true"></span>Open <span class="n">0</span></button>
              <button type="button" data-status="closed" aria-pressed="false"><span class="fdot" aria-hidden="true"></span>Closed <span class="n">0</span></button>
            </div>
            <button class="btn primary sm" id="new-notch" type="button">New notch</button>
          </div>
          <div id="notch-list" class="stack"></div>
        </div>
      </section>`;
    renderCards(DEFAULT_QUERY);
  }

  function renderCards(q) {
    updateFilter(q);
    const list = document.getElementById('notch-list');
    if (!list) return;
    const rows = topLevel().filter((n) => matches(n, q));
    if (rows.length === 0) {
      list.className = 'stack';
      list.innerHTML = `<p class="swatch-note">${topLevel().length === 0 ? 'No notches yet — hit New notch to start.' : 'Nothing matches that search.'}</p>`;
      return;
    }
    // Rows live in one bordered container (GitHub's issue-list shape); the
    // .notch-list border/separators only make sense when there are rows.
    list.className = 'notch-list';
    list.innerHTML = rows.map(cardHTML).join('');
  }

  // The Open/Closed tabs are a shortcut over the same `is:` search token: they
  // reflect the current query's status and, on click, rewrite it (keeping any
  // free-text terms). Counts are of top-level notches, which is what the list
  // shows.
  function updateFilter(q) {
    const f = document.getElementById('filter');
    if (!f) return;
    const tops = topLevel();
    const openN = tops.filter((n) => notchStatus(n) === 'open').length;
    const { statuses } = parseQuery(q);
    const isClosed = statuses.some((s) => s === 'closed' || s === 'done' || s === 'not_planned');
    const isOpen = statuses.includes('open') && !isClosed;
    f.querySelector('[data-status="open"]').setAttribute('aria-pressed', String(isOpen));
    f.querySelector('[data-status="closed"]').setAttribute('aria-pressed', String(isClosed));
    f.querySelector('[data-status="open"] .n').textContent = openN;
    f.querySelector('[data-status="closed"] .n').textContent = tops.length - openN;
  }

  function setStatusFilter(status) {
    const input = document.getElementById('search');
    if (!input) return;
    const { text } = parseQuery(input.value);
    input.value = `is:${status}` + (text ? ' ' + text : '');
    renderCards(input.value);
  }

  // ---------- detail view ----------
  // Per-view UI state that isn't part of the saved notch: which tab the
  // description editor shows (Write vs Preview). Keyed by notch id so opening a
  // different notch resets it — a fresh/empty body opens in Write, an existing
  // one in Preview (rendered), the way GitHub shows a saved issue body.
  let detailUI = { id: null, bodyTab: 'preview' };
  function bodyTabFor(n) {
    if (detailUI.id !== n.id) {
      detailUI = { id: n.id, bodyTab: (n.body || '').trim() ? 'preview' : 'write' };
    }
    return detailUI.bodyTab;
  }

  function renderDetail(n) {
    const crumbs = [`<a href="#/" class="back">all notches</a>`]
      .concat(trail(n).map((a) => `<a href="#/n/${esc(a.id)}" class="back">${esc(a.title) || 'untitled'}</a>`))
      .join('<span class="crumb-sep"> / </span>');

    const kids = childrenOf(n.id);
    const subCards = kids.length
      ? `<div class="notch-list">${kids.map(cardHTML).join('')}</div>`
      : '<span class="swatch-note">No sub-notches.</span>';

    const tags = n.tags.map((g) => `
      <span class="chip tag-chip"><span class="lab ${esc(g.color)}">${esc(g.name)}</span>
        <button class="x" type="button" data-del-tag="${esc(g.name)}" aria-label="remove tag">&times;</button></span>`).join('');

    const status = notchStatus(n);
    const closeControls = status === 'open'
      ? `<span class="lab open">open</span>
         <button class="btn ghost sm" id="close-not-planned" type="button">Not planned</button>
         <button class="btn primary sm" id="close-done" type="button">Close as done</button>`
      : `${statusLab(n)}<button class="btn ghost sm" id="reopen" type="button">Reopen</button>`;

    // Description: a GitHub-style Write / Preview editor. Preview renders the
    // Markdown (tasks are interactive there); Write is the raw textarea, saved
    // as you type. Task progress rides in the header like an issue's checklist.
    const ts = taskStats(n);
    const pct = ts.total ? Math.round((ts.done / ts.total) * 100) : 0;
    const tab = bodyTabFor(n);
    const rendered = (n.body || '').trim()
      ? `<div class="md-body" id="body-preview">${mdRender(n.body)}</div>`
      : '<div class="md-body empty" id="body-preview"><span class="swatch-note">No description yet — switch to Write to add one. Tasks live here as Markdown: <code>- [ ] do a thing</code>.</span></div>';
    const bodyPane = tab === 'write'
      ? `<textarea class="textarea md-input" id="body" placeholder="Describe this notch in Markdown. Add tasks with - [ ] …">${esc(n.body || '')}</textarea>`
      : rendered;

    const comments = (n.comments || []).slice().sort((a, b) => a.createdAt - b.createdAt);
    const commentList = comments.length
      ? comments.map((c) => `
        <article class="comment" data-comment="${esc(c.id)}">
          <header class="comment-head">
            <span class="comment-when">${fmtDate(c.createdAt)}</span>
            <button class="x" type="button" data-del-comment="${esc(c.id)}" aria-label="delete comment">&times;</button>
          </header>
          <div class="md-body">${mdRender(c.body)}</div>
        </article>`).join('')
      : '<span class="swatch-note">No comments yet — add one below to keep a paper trail.</span>';

    view().innerHTML = `
      <p class="lede">← ${crumbs}</p>

      <section class="section">
        <h2><span>Notch</span><span class="row" style="gap:8px">${closeControls}</span></h2>
        <div class="section-body stack">
          <label class="field"><span>Title</span><input class="input title-input" id="title" type="text" value="${esc(n.title)}" placeholder="Title"/></label>
          <label class="field"><span>Parent</span>
            <select class="select" id="parent">
              <option value=""${n.parentId ? '' : ' selected'}>— top level —</option>
              ${moveTargets(n).map((t) => `<option value="${esc(t.id)}"${t.id === n.parentId ? ' selected' : ''}>${esc(pathLabel(t))}</option>`).join('')}
            </select>
          </label>
        </div>
      </section>

      <section class="section">
        <h2><span>Description</span>${ts.total ? `<span class="count num">${ts.done} of ${ts.total} tasks</span>` : ''}</h2>
        <div class="section-body stack">
          <div class="seg" id="body-tabs" role="group" aria-label="Description editor">
            <button type="button" data-tab="write" aria-pressed="${tab === 'write'}">Write</button>
            <button type="button" data-tab="preview" aria-pressed="${tab === 'preview'}">Preview</button>
          </div>
          ${ts.total && tab === 'preview' ? `<div class="progress"><div class="bar"><i style="width:${pct}%"></i></div><span class="pct">${pct}%</span></div>` : ''}
          ${bodyPane}
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
        <h2><span>Comments</span>${comments.length ? `<span class="count num">${comments.length}</span>` : ''}</h2>
        <div class="section-body stack">
          <div class="stack" id="comments">${commentList}</div>
          <form class="stack" id="comment-form" autocomplete="off">
            <textarea class="textarea" id="comment-text" placeholder="Leave a comment (Markdown supported)…"></textarea>
            <div class="row" style="justify-content:flex-end">
              <button class="btn primary sm" type="submit">Comment</button>
            </div>
          </form>
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
  let bodyTimer = null, titleTimer = null;

  function onSubmit(e) {
    const f = e.target;
    if (f.id === 'sub-form') {
      e.preventDefault();
      const parent = currentDetail(); if (!parent) return;
      const box = document.getElementById('sub-title');
      const title = box.value.trim();
      if (!title) return;
      createNotch(title, parent.id).then(() => renderDetail(parent)).catch(() => {});
      return;
    }
    if (f.id === 'comment-form') {
      e.preventDefault();
      const n = currentDetail(); if (!n) return;
      const box = document.getElementById('comment-text');
      const text = box.value.trim();
      if (!text) return;
      addComment(n, text).then(() => renderDetail(n));
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

    if (e.target.closest('#new-notch')) {
      e.preventDefault();
      createNotch('', null).then((n) => { location.hash = '#/n/' + n.id; }).catch(() => {});
      return;
    }

    const fbtn = e.target.closest('.filter button[data-status]');
    if (fbtn) {
      e.preventDefault();
      setStatusFilter(fbtn.getAttribute('data-status'));
      return;
    }

    // Description Write / Preview toggle. Flush any pending textarea edit into
    // the notch before switching, so Preview always shows the latest text.
    const tabBtn = e.target.closest('#body-tabs button[data-tab]');
    if (tabBtn) {
      const n = currentDetail(); if (!n) return;
      const ta = document.getElementById('body');
      detailUI = { id: n.id, bodyTab: tabBtn.getAttribute('data-tab') };
      if (ta && ta.value !== (n.body || '')) {
        clearTimeout(bodyTimer);
        n.body = ta.value;
        persist(n).then(() => renderDetail(n));
      } else {
        renderDetail(n);
      }
      return;
    }

    const delComment = e.target.closest('[data-del-comment]');
    if (delComment) {
      e.preventDefault();
      const n = currentDetail(); if (!n) return;
      deleteComment(n, delComment.getAttribute('data-del-comment')).then(() => renderDetail(n));
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
    // Blur/commit on the description textarea saves immediately, so a pending
    // debounced edit isn't lost when the next click re-renders the detail view.
    if (e.target.id === 'body') {
      const n = currentDetail(); if (!n) return;
      clearTimeout(bodyTimer);
      if (n.body !== e.target.value) { n.body = e.target.value; persist(n); }
      return;
    }
    if (e.target.id === 'parent') {
      const n = currentDetail(); if (!n) return;
      const val = e.target.value || null;
      if (val === (n.parentId || null)) return; // no-op reselect
      n.parentId = val;
      persist(n).then(() => renderDetail(n)); // re-render: breadcrumb + siblings move
      return;
    }
    // Ticking a task checkbox in the rendered description flips the matching
    // `- [ ]` in the Markdown source.
    const taskBox = e.target.closest('.md-body input[data-task]');
    if (taskBox) {
      const n = currentDetail(); if (!n) return;
      const idx = parseInt(taskBox.getAttribute('data-task'), 10);
      n.body = toggleTask(n.body, idx);
      persist(n).then(() => renderDetail(n));
    }
  }

  function onInput(e) {
    if (e.target.id === 'search') {
      renderCards(e.target.value);
      return;
    }
    if (e.target.id === 'body') {
      const n = currentDetail(); if (!n) return;
      clearTimeout(bodyTimer);
      bodyTimer = setTimeout(() => { n.body = e.target.value; persist(n); }, 350);
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
