// SPDX-License-Identifier: GPL-2.0-or-later
//
// tally — local-first client app (epic #1).
//
// The whole product runs here in the browser. A "notch" is a generic
// container — a mark carved on your tally — that you edit much like a GitHub
// issue: a title, a Markdown description (where tasks live as `- [ ]` task
// lists, checked off inline), tags/labels, sub-notches (parent/child like
// GitHub sub-issues), and a running thread of comments for the paper trail.
//
// DEMO MODE: for now tally runs entirely in memory — no IndexedDB, no server,
// no network, no persistence of any kind. Edits stick as you navigate and
// re-render (the `notches` array below is the one source of truth), but a
// reload (F5) starts fresh from the seed data. A demo banner and Reset button
// make that ephemerality obvious. This keeps the app instantly runnable on
// localhost without the browser's storage layer ever getting in the way;
// durable persistence is a later concern.
//
// (A "tally" — resolving/tallying-up notches — is a later concept; v1 is just
// the notches and their attachments.)
//
// Data model (`notches`, an in-memory array keyed by id):
//   Notch { id, title, body, tags:[name],
//           events:[Event],
//           parentId:string|null, status:'open'|'done'|'not_planned',
//           createdAt, updatedAt }
//   Event { id, kind, at, ...payload }   kind ∈ opened | comment | labeled |
//           unlabeled | task | moved | sub | renamed | status | attachment
//
// LABELS: labels are a second array (`labels`), global across the whole app —
// not owned by any one notch. A notch's `tags` is just a list of label names;
// the name is the join key. Making a label called "bug" on one notch and
// later typing "bug" on another reuses the exact same color, and re-coloring
// a label (the popover's two color pickers) updates every notch showing it
// at once. Label { name, color:'red'|'amber'|…, bg:hex|null, fg:hex|null } —
// `color` is the auto-assigned palette swatch (theme-aware CSS); `bg`/`fg`
// are only set once a color picker has been touched, and then override the
// palette with a fixed, theme-independent color pair, GitHub-label style.
// `body` is the Markdown description; tasks are ordinary Markdown task-list
// items inside it (`- [ ]` / `- [x]`), not a separate structure — mirroring how
// GitHub issues carry their checklists. A sub-notch is an ordinary notch whose
// parentId points at its parent. Any notch can be re-parented after the fact —
// the detail view's sub-notches roll-up links an existing notch under the current
// one (and the header kebab moves a notch back to the top level) — the only
// restriction is that a notch can't be moved under itself or one of its own
// descendants, which would make a cycle. A notch is never deleted — it's closed
// as done or not planned (and can be reopened), same as a GitHub issue.
//
// EVENT LOG: every notch keeps an append-only `events` timeline — the detail
// view IS that timeline, GitHub-issue style. Labelling, unlabelling, ticking a
// task, commenting, moving, renaming and status changes all append an event, and
// nothing is ever removed: a "deleted" comment is only flagged `deleted:true`,
// so its tombstone stays in the record. Comments live solely as `comment` events
// (there is no separate comments array). The `opened` event anchors the top of
// the log and hosts the live Markdown description.
//
// ATTACHMENTS: files and photos attach to a notch as `attachment` events on that
// same timeline. In demo mode the bytes ride along in memory as a data: URL on
// the event (`{name, mime, size, dataUrl}`) — no server, no blob store — so a
// reload wipes them with everything else. Images preview inline (and open in a
// lightbox on click); anything else shows as a downloadable file chip. Like a
// comment, a "removed" attachment is only flagged `deleted:true`, never dropped.
// When a real backend lands, the dataUrl is the seam that becomes an object ref.

(() => {
  'use strict';

  // ---------- storage: demo mode ----------
  // There is no backing store. `notches` (declared just below) holds every
  // record for the life of the page; mutations write straight to it and the
  // "put" is a no-op. On boot we populate it from demoSeed(), and Reset — plus
  // any reload — returns to exactly that seed. When durable persistence lands,
  // this is the seam where a real backend (IndexedDB, a server, …) plugs in.

  // ---------- state ----------
  let notches = []; // in-memory source of truth, mirrored to IndexedDB
  let labels = []; // global label registry — see the LABELS note up top
  const now = () => Date.now();
  const uid = (p) => p + Date.now().toString(36) + Math.random().toString(36).slice(2, 7);
  const byId = (id) => notches.find((n) => n.id === id);

  const PALETTE = ['red', 'amber', 'green', 'blue', 'pink', 'cyan'];

  // ---------- labels: a single global, shared registry ----------
  const findLabel = (name) => labels.find((l) => l.name.toLowerCase() === String(name || '').toLowerCase());
  // ensureLabel finds a label by name (any case) or creates one with the next
  // palette color, round-robin over how many labels already exist — this is
  // the one place a new label comes into being.
  function ensureLabel(name) {
    const existing = findLabel(name);
    if (existing) return existing;
    const label = { name: name.trim(), color: PALETTE[labels.length % PALETTE.length], bg: null, fg: null };
    labels.push(label);
    return label;
  }
  // A hex color's ideal readable text color (WCAG relative-luminance split) —
  // the fallback when a label has a custom background but no custom text
  // color picked yet.
  function autoContrast(hex) {
    const m = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex || '');
    if (!m) return '#111318';
    const lin = (h) => { const c = parseInt(h, 16) / 255; return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4); };
    const L = 0.2126 * lin(m[1]) + 0.7152 * lin(m[2]) + 0.0722 * lin(m[3]);
    return L > 0.45 ? '#111318' : '#f5f7fa';
  }
  // A palette-name class for the default (theme-aware) look, or an inline
  // style pinning a custom bg/fg once the label's colors have been picked.
  function chipStyle(label) {
    if (!label || !label.bg) return '';
    const fg = label.fg || autoContrast(label.bg);
    return ` style="--lc:${esc(label.bg)}; background:${esc(label.bg)}; color:${esc(fg)}; border-color:color-mix(in srgb, ${esc(label.bg)} 60%, var(--line))"`;
  }
  function labChip(name) {
    const label = findLabel(name);
    if (!label) return `<span class="lab gray">${esc(name)}</span>`;
    const cls = label.bg ? 'lab custom' : `lab ${esc(label.color || 'gray')}`;
    return `<span class="${cls}"${chipStyle(label)}>${esc(label.name)}</span>`;
  }

  // Attachments. In demo mode a file's bytes live in memory as a data: URL, so a
  // generous-but-finite cap keeps a stray multi-hundred-MB drop from wedging the
  // tab. Only raster/vector image types preview inline; everything else is a
  // download chip. (SVG previews via <img>, where scripts never execute.)
  const MAX_ATTACH_BYTES = 25 * 1024 * 1024;
  const IMAGE_MIMES = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/svg+xml', 'image/avif', 'image/bmp']);
  const isImageMime = (mime) => IMAGE_MIMES.has(String(mime || '').toLowerCase());
  // A compact, human-readable byte count for the file meta line.
  function fmtBytes(bytes) {
    const b = Number(bytes) || 0;
    if (b < 1024) return `${b} B`;
    const units = ['KB', 'MB', 'GB'];
    let v = b / 1024, i = 0;
    while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
    return `${v < 10 ? v.toFixed(1) : Math.round(v)} ${units[i]}`;
  }
  // encodeURIComponent keeps the seed's inline SVG "photos" valid in both an
  // <img src> attribute and a download href without hand-encoding every glyph.
  const svgDataUrl = (svg) => 'data:image/svg+xml,' + encodeURIComponent(svg);
  const textDataUrl = (text) => 'data:text/plain;charset=utf-8,' + encodeURIComponent(text);

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

  // A readable "A / B / C" path for a notch, so nested targets in the link-existing
  // picker are distinguishable when titles repeat.
  function pathLabel(n) {
    return trail(n).concat(n).map((x) => x.title.trim() || 'untitled').join(' / ');
  }

  // Notches that can be linked as a child of n (the "link existing" picker on the
  // sub-notches roll-up): anything that isn't n itself, isn't already a direct
  // child, and isn't an ancestor of n — parenting n's own ancestor under it would
  // make a cycle. A deeper descendant may still be pulled straight up to a child.
  function linkTargets(n) {
    const blocked = new Set([n.id, ...trail(n).map((a) => a.id), ...childrenOf(n.id).map((c) => c.id)]);
    return sortByUpdated(notches.filter((x) => !blocked.has(x.id)));
  }

  // demoSeed builds the starting notches for a demo session — fresh ids and
  // timestamps each call, so Reset (and every reload) lands on the same shape.
  // It shows off the core moves: a task list, tags, a comment, and nesting.
  // A tiny inline "photo" and a text file so the attachment feature has
  // something to show the moment the demo loads — no external assets, all data:
  // URLs held in memory like any user upload.
  const DEMO_PHOTO = svgDataUrl(
    "<svg xmlns='http://www.w3.org/2000/svg' width='480' height='300' viewBox='0 0 480 300'>" +
    "<rect width='480' height='300' fill='skyblue'/>" +
    "<circle cx='384' cy='74' r='40' fill='gold'/>" +
    "<path d='M0 300 L150 140 L250 260 L340 120 L480 300 Z' fill='seagreen'/>" +
    "<path d='M0 300 L110 205 L215 300 Z' fill='darkgreen'/>" +
    "</svg>");
  const DEMO_NOTE = textDataUrl('tally — attachment demo\n\nDrop a photo or any file onto a notch;\nit rides along in memory until you reload.\n');

  // demoLabels seeds the global label registry — same shape ensureLabel builds
  // at runtime, just with fixed colors so the demo looks the same every load.
  function demoLabels() {
    return [
      { name: 'demo', color: 'blue', bg: null, fg: null },
      { name: 'errand', color: 'green', bg: null, fg: null },
    ];
  }

  function demoSeed() {
    const t = now();
    const mk = (o) => Object.assign(
      { id: uid('n_'), title: '', body: '', tags: [], events: [],
        parentId: null, status: 'open', createdAt: t, updatedAt: t }, o);
    return [
      mk({
        id: 'n_demo_welcome',
        title: 'Welcome to tally',
        body: [
          "This is a **demo** — everything lives in your browser's memory only.",
          'Edits stick as you click around, but reloading the page starts fresh.',
          '',
          'Try it:',
          '- [ ] Check off a task by clicking the box',
          '- [x] (this one is already done)',
          '- [ ] Open a notch and leave a comment',
          '- [ ] Attach a photo or file with the paperclip below',
          '',
          'Use **Reset demo** in the bar above to wipe back to this starting point.',
        ].join('\n'),
        tags: ['demo'],
        // A ready-made timeline so the event log has a story to tell on open.
        events: [
          { id: 'e_w1', kind: 'opened', at: t - 5000 },
          { id: 'e_w2', kind: 'labeled', at: t - 4000, name: 'demo' },
          { id: 'e_w3', kind: 'comment', at: t - 3000, body: 'Notches keep a permanent event log, like a GitHub issue — labels, comments, tasks, status. Nothing you do here ever disappears from the record.' },
          { id: 'e_w4', kind: 'comment', at: t - 2000, deleted: true, body: 'Even a deleted comment stays as a struck-through tombstone.' },
          { id: 'e_w5', kind: 'task', at: t - 1000, text: '(this one is already done)', done: true },
          { id: 'e_w6', kind: 'attachment', at: t - 800, name: 'sunset.svg', mime: 'image/svg+xml', size: 512, dataUrl: DEMO_PHOTO },
          { id: 'e_w7', kind: 'attachment', at: t - 400, name: 'about-attachments.txt', mime: 'text/plain', size: 132, dataUrl: DEMO_NOTE },
        ],
      }),
      mk({
        id: 'n_demo_sub',
        title: 'A sub-notch',
        body: 'Notches nest — this one lives under “Welcome to tally”.',
        parentId: 'n_demo_welcome',
        updatedAt: t - 1000,
        events: [{ id: 'e_s1', kind: 'opened', at: t - 1000 }],
      }),
      mk({
        id: 'n_demo_list',
        title: 'A quick checklist',
        body: '- [ ] Milk\n- [ ] Coffee\n- [x] Bread',
        tags: ['errand'],
        updatedAt: t - 2000,
        events: [
          { id: 'e_l1', kind: 'opened', at: t - 3000 },
          { id: 'e_l2', kind: 'labeled', at: t - 2500, name: 'errand' },
          { id: 'e_l3', kind: 'task', at: t - 2000, text: 'Bread', done: true },
        ],
      }),
    ];
  }

  function load() {
    labels = demoLabels();
    notches = demoSeed();
  }
  // persist records an edit. In demo mode there is nothing to write to — the
  // record already lives in `notches` by reference — so we just stamp the time
  // and re-render. Kept async so existing `persist(n).then(...)` callers work.
  function persist(n) {
    n.updatedAt = now();
    ticker();
    return Promise.resolve();
  }
  // logEvent appends one entry to a notch's timeline. It only mutates the array;
  // callers persist() afterwards to stamp the time and re-render. This is the one
  // seam every recorded action goes through, so the log stays append-only.
  function logEvent(n, kind, data) {
    if (!Array.isArray(n.events)) n.events = [];
    const ev = Object.assign({ id: uid('e_'), kind, at: now() }, data || {});
    n.events.push(ev);
    return ev;
  }
  function createNotch(title, parentId) {
    const t = now();
    const n = {
      id: uid('n_'), title: title.trim(), body: '', tags: [],
      events: [{ id: uid('e_'), kind: 'opened', at: t }],
      parentId: parentId || null, status: 'open', createdAt: t, updatedAt: t,
    };
    notches.push(n);
    // Creating a sub-notch is itself an event on the parent's timeline.
    if (parentId) { const p = byId(parentId); if (p) logEvent(p, 'sub', { subId: n.id, title: n.title }); }
    ticker();
    return Promise.resolve(n);
  }
  // linkChild re-parents an existing notch under `parent`, recording it on both
  // timelines (a `moved` on the child, a `sub` on the parent) exactly as creating
  // a fresh sub-notch does — this is the "link existing" half of Add sub-notch.
  async function linkChild(parent, childId) {
    const c = byId(childId);
    if (!c || c.id === parent.id || c.parentId === parent.id) return;
    c.parentId = parent.id;
    logEvent(c, 'moved', { toId: parent.id, toTitle: parent.title });
    logEvent(parent, 'sub', { subId: c.id, title: c.title });
    await persist(c);
    await persist(parent);
  }
  const notchStatus = (n) => n.status || 'open';
  async function setStatus(n, status) {
    n.status = status;
    logEvent(n, 'status', { status });
    await persist(n);
  }
  async function addComment(n, body) {
    const text = body.trim();
    if (!text) return;
    logEvent(n, 'comment', { body: text });
    await persist(n);
  }
  // "Deleting" a comment never removes it — it flags the event so the timeline
  // renders a struck-through tombstone. The record keeps its full history.
  async function deleteComment(n, id) {
    const ev = (n.events || []).find((e) => e.id === id && e.kind === 'comment');
    if (ev) ev.deleted = true;
    await persist(n);
  }

  // Read a File into a data: URL — the in-memory stand-in for a stored blob.
  function readFileDataURL(file) {
    return new Promise((resolve, reject) => {
      const r = new FileReader();
      r.onload = () => resolve(r.result);
      r.onerror = () => reject(r.error || new Error('read failed'));
      r.readAsDataURL(file);
    });
  }
  // addAttachments logs one `attachment` event per file. Oversized files are
  // skipped (their bytes would just sit in memory) and reported back so the UI
  // can tell the user which ones didn't take. Persist once at the end.
  async function addAttachments(n, files) {
    const skipped = [];
    for (const file of files) {
      if (!file) continue;
      if (file.size > MAX_ATTACH_BYTES) { skipped.push(file.name || 'file'); continue; }
      try {
        const dataUrl = await readFileDataURL(file);
        logEvent(n, 'attachment', {
          name: file.name || 'file',
          mime: file.type || 'application/octet-stream',
          size: file.size,
          dataUrl,
        });
      } catch (e) { skipped.push(file.name || 'file'); }
    }
    await persist(n);
    return skipped;
  }
  // "Removing" an attachment mirrors deleting a comment: it flags the event so
  // the timeline keeps a tombstone rather than losing the record.
  async function deleteAttachment(n, id) {
    const ev = (n.events || []).find((e) => e.id === id && e.kind === 'attachment');
    if (ev) ev.deleted = true;
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

  // taskInfo returns the {text, done} of the Nth task-list item (document order),
  // so a toggle can record which task it flipped in the event log. Counts exactly
  // as mdRender/toggleTask do.
  function taskInfo(body, index) {
    let n = 0;
    for (const line of String(body || '').split('\n')) {
      const m = TASK_RE.exec(line);
      if (!m) continue;
      if (n === index) return { text: m[5].trim(), done: m[2].toLowerCase() === 'x' };
      n++;
    }
    return null;
  }

  // Live (non-deleted) comments on a notch, oldest → newest.
  const liveComments = (n) => (n.events || []).filter((e) => e.kind === 'comment' && !e.deleted);
  // Live (non-deleted) attachments on a notch — feeds the card/roll-up counts.
  const liveAttachments = (n) => (n.events || []).filter((e) => e.kind === 'attachment' && !e.deleted);

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
      || n.tags.some((name) => name.toLowerCase().includes(t))
      || liveComments(n).some((c) => c.body.toLowerCase().includes(t));
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
    let done = 0, total = 0, files = 0;
    const tags = new Set();
    for (const n of notches) {
      const s = taskStats(n);
      done += s.done; total += s.total;
      files += liveAttachments(n).length;
      for (const name of n.tags) tags.add(name);
    }
    const open = notches.filter((n) => notchStatus(n) === 'open').length;
    el.innerHTML =
      `<span class="stat"><b>${open}</b> open</span>` +
      `<span class="stat"><b class="up">${done}</b> of <b>${total}</b> tasks done</span>` +
      `<span class="stat"><b>${tags.size}</b> tags</span>` +
      `<span class="stat"><b>${files}</b> file${files === 1 ? '' : 's'}</span>`;
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
    const comments = liveComments(n).length;
    const files = liveAttachments(n).length;
    const bits = [];
    if (kids) bits.push(`<span class="num">${kids}</span> sub-notch${kids === 1 ? '' : 'es'}`);
    if (s.total) bits.push(`<span class="num">${s.done}/${s.total}</span> task${s.total === 1 ? '' : 's'}`);
    if (comments) bits.push(`<span class="num">${comments}</span> comment${comments === 1 ? '' : 's'}`);
    if (files) bits.push(`<span class="num">${files}</span> file${files === 1 ? '' : 's'}`);
    if ((n.body || '').trim() && !s.total) bits.push('description');
    const meta = bits.length ? bits.join(' · ') : 'empty';
    const tags = statusLab(n) + n.tags.map((name) => labChip(name)).join('');
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
  let detailUI = { id: null, bodyTab: 'preview', subsCollapsed: true };
  function ensureDetailUI(n) {
    if (detailUI.id !== n.id) {
      detailUI = { id: n.id, bodyTab: (n.body || '').trim() ? 'preview' : 'write', subsCollapsed: true };
    }
    return detailUI;
  }
  function bodyTabFor(n) { return ensureDetailUI(n).bodyTab; }

  // Which detail popover to re-open after the next renderDetail (a full re-render
  // otherwise closes every menu). Set right before a menu-driven mutation so the
  // labels popover, say, stays open while you add several labels in a row; cleared
  // once applied so an unrelated re-render doesn't spuriously pop it back open.
  let detailMenu = null;
  function applyOpenMenu() {
    if (!detailMenu) return;
    const m = document.querySelector(`.menu[data-menu="${detailMenu}"]`);
    if (m) {
      m.classList.add('open');
      const t = m.querySelector('[data-menu-trigger]');
      if (t) t.setAttribute('aria-expanded', 'true');
    }
    detailMenu = null;
  }

  // The title as it stood when the detail view was last rendered — the baseline a
  // blur compares against to record a `renamed` event (and to skip the initial
  // naming of a fresh, still-untitled notch, which isn't a rename).
  let titleBaseline = '';

  // ---------- timeline ----------
  // The detail view is one append-only event log. Small events are one-liners on
  // a shared rail; comments and the opening description are full cards. Icons are
  // inline SVG (the app is offline, no icon font). Everything a notch records maps
  // to one of the kinds below.
  const svgIcon = (inner, sw) => `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="${sw || 2}" stroke-linecap="round" stroke-linejoin="round">${inner}</svg>`;
  const ICON = {
    pencil: svgIcon('<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>'),
    tag: svgIcon('<path d="M20.6 13.4 12 22l-9-9V4h9l8.6 8.6a2 2 0 0 1 0 2.8Z"/><circle cx="7.5" cy="7.5" r="1.4" fill="currentColor" stroke="none"/>'),
    untag: svgIcon('<path d="M20.6 13.4 12 22l-9-9V4h9l8.6 8.6a2 2 0 0 1 0 2.8Z"/><path d="M2 2l20 20"/>'),
    check: svgIcon('<path d="M4 12l5 5L20 6"/>', 2.4),
    box: svgIcon('<rect x="4" y="4" width="16" height="16" rx="2.5"/>'),
    comment: svgIcon('<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2Z"/>'),
    trash: svgIcon('<path d="M3 6h18"/><path d="M8 6V4h8v2"/><path d="M6 6l1 14h10l1-14"/>'),
    move: svgIcon('<path d="M15 10l5 5-5 5"/><path d="M4 4v7a4 4 0 0 0 4 4h12"/>'),
    branch: svgIcon('<path d="M6 3v12"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="6" r="3"/><path d="M18 9a9 9 0 0 1-9 9"/>'),
    done: svgIcon('<circle cx="12" cy="12" r="9"/><path d="M8 12l3 3 5-6"/>'),
    skip: svgIcon('<circle cx="12" cy="12" r="9"/><path d="M6 6l12 12"/>'),
    reopen: svgIcon('<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="3" fill="currentColor" stroke="none"/>'),
    dot: svgIcon('<circle cx="12" cy="12" r="6" fill="currentColor" stroke="none"/>'),
    // A horizontal kebab ("…") — the menu affordance on the detail header and the
    // labels row; and a chevron for the collapsible sub-notches roll-up.
    dots: svgIcon('<circle cx="5" cy="12" r="1.6" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.6" fill="currentColor" stroke="none"/><circle cx="19" cy="12" r="1.6" fill="currentColor" stroke="none"/>'),
    caret: svgIcon('<path d="M9 6l6 6-6 6"/>', 2.4),
    plus: svgIcon('<path d="M12 5v14"/><path d="M5 12h14"/>', 2.4),
    // Attachment affordances: a paperclip for the composer action, and image /
    // file / download glyphs for the attachment cards.
    clip: svgIcon('<path d="M21 10.5 11.5 20a5 5 0 0 1-7-7l9-9a3.3 3.3 0 0 1 4.7 4.7l-8.5 8.5a1.6 1.6 0 0 1-2.3-2.3l7.8-7.8"/>'),
    image: svgIcon('<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9.5" r="1.6"/><path d="M21 15l-5-5L5 21"/>'),
    file: svgIcon('<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8Z"/><path d="M14 3v5h5"/>'),
    download: svgIcon('<path d="M12 3v12"/><path d="M7 11l5 5 5-5"/><path d="M5 21h14"/>'),
  };

  function statePill(n) {
    const s = notchStatus(n);
    if (s === 'done') return `<span class="state done">${ICON.done} Done</span>`;
    if (s === 'not_planned') return `<span class="state np">${ICON.skip} Not planned</span>`;
    return `<span class="state open">${ICON.dot} Open</span>`;
  }

  // The parent's status words, for the chip label + its aria/title text.
  const STATUS_WORD = { open: 'open', done: 'done', not_planned: 'not planned' };

  // parentChip renders the parent notch inline in the detail header — a compact
  // link that carries the parent's own open/done/not-planned state (a coloured
  // status dot) and navigates to it on click. Replaces the old Parent picker.
  function parentChip(p) {
    const s = notchStatus(p);
    const cls = s === 'done' ? 'done' : s === 'not_planned' ? 'np' : 'open';
    const icon = s === 'done' ? ICON.done : s === 'not_planned' ? ICON.skip : ICON.dot;
    const title = p.title.trim() || 'untitled';
    return `<a class="parent-chip" href="#/n/${esc(p.id)}" title="Parent (${STATUS_WORD[s] || 'open'}): ${esc(title)}">` +
      `<span class="pdot ${cls}" aria-hidden="true">${icon}</span>` +
      `<span class="pttl">${esc(title)}</span></a>`;
  }

  // A one-line event on the rail: icon + "you <did something> · <when>".
  function evSmall(cls, icon, inner, at) {
    return `<div class="ev small ${cls}"><span class="icon">${icon}</span>` +
      `<span class="line"><b>you</b> ${inner} <span class="when">${fmtDate(at)}</span></span></div>`;
  }

  // A comment card — or, once deleted, its tombstone: the body stays, struck
  // through and dimmed, so the record is never actually lost.
  function commentEventHTML(ev) {
    const when = fmtDate(ev.at);
    if (ev.deleted) {
      return '<div class="ev card deleted danger">' +
        `<span class="icon">${ICON.trash}</span>` +
        '<div class="box"><div class="box-head">' +
        `<span class="tombstone">deleted comment</span><span class="when">${when}</span></div>` +
        `<div class="box-body md-body">${mdRender(ev.body)}</div></div></div>`;
    }
    return '<div class="ev card">' +
      `<span class="icon">${ICON.comment}</span>` +
      '<div class="box"><div class="box-head"><span><b>you</b> commented</span>' +
      `<span class="row" style="gap:8px;align-items:center"><span class="when">${when}</span>` +
      `<button class="x" type="button" data-del-comment="${esc(ev.id)}" aria-label="delete comment">&times;</button></span></div>` +
      `<div class="box-body md-body">${mdRender(ev.body)}</div></div></div>`;
  }

  // An attachment card: an image previews inline (a button that opens the
  // lightbox) with a download link; any other file is a single download chip.
  // Once removed it collapses to a tombstone, like a deleted comment.
  function attachmentEventHTML(ev) {
    const when = fmtDate(ev.at);
    const name = ev.name || 'file';
    if (ev.deleted) {
      return '<div class="ev card deleted danger">' +
        `<span class="icon">${ICON.trash}</span>` +
        '<div class="box"><div class="box-head">' +
        `<span class="tombstone">removed attachment</span><span class="when">${when}</span></div>` +
        `<div class="box-body md-body"><span class="attach-name">${esc(name)}</span></div></div></div>`;
    }
    const delBtn = `<button class="x" type="button" data-del-attach="${esc(ev.id)}" aria-label="remove attachment">&times;</button>`;
    if (isImageMime(ev.mime)) {
      return '<div class="ev card">' +
        `<span class="icon">${ICON.image}</span>` +
        '<div class="box"><div class="box-head">' +
        `<span><b>you</b> attached <b>${esc(name)}</b></span>` +
        `<span class="row" style="gap:8px;align-items:center"><span class="when">${when}</span>${delBtn}</span></div>` +
        '<div class="box-body attach-body">' +
        `<button type="button" class="attach-figure" data-attach-view="${esc(ev.id)}" aria-label="View ${esc(name)} full size">` +
        `<img src="${esc(ev.dataUrl)}" alt="${esc(name)}" loading="lazy"/></button>` +
        '<div class="attach-meta">' +
        `<span class="attach-size">${fmtBytes(ev.size)}</span>` +
        `<a class="attach-dl" href="${esc(ev.dataUrl)}" download="${esc(name)}">${ICON.download}<span>Download</span></a>` +
        '</div></div></div></div>';
    }
    return '<div class="ev card">' +
      `<span class="icon">${ICON.file}</span>` +
      '<div class="box"><div class="box-head">' +
      `<span><b>you</b> attached a file</span>` +
      `<span class="row" style="gap:8px;align-items:center"><span class="when">${when}</span>${delBtn}</span></div>` +
      '<div class="box-body attach-body">' +
      `<a class="attach-file" href="${esc(ev.dataUrl)}" download="${esc(name)}">` +
      `<span class="attach-ico">${ICON.file}</span>` +
      `<span class="attach-file-txt"><span class="attach-name">${esc(name)}</span>` +
      `<span class="attach-size">${fmtBytes(ev.size)} · download</span></span>${ICON.download}</a>` +
      '</div></div></div>';
  }

  function eventHTML(n, ev) {
    switch (ev.kind) {
      case 'comment': return commentEventHTML(ev);
      case 'attachment': return attachmentEventHTML(ev);
      case 'labeled': return evSmall('accent', ICON.tag, `added the ${labChip(ev.name)} label`, ev.at);
      case 'unlabeled': return evSmall('', ICON.untag, `removed the ${labChip(ev.name)} label`, ev.at);
      case 'task': return evSmall(ev.done ? 'good' : '', ev.done ? ICON.check : ICON.box,
        `${ev.done ? 'checked off' : 'unchecked'} <b>${esc(ev.text || 'a task')}</b>`, ev.at);
      case 'moved': {
        const to = ev.toId ? byId(ev.toId) : null;
        const label = ev.toId
          ? `moved this under <b>${esc((to ? to.title : ev.toTitle) || 'untitled')}</b>`
          : 'moved this to the top level';
        return evSmall('', ICON.move, label, ev.at);
      }
      case 'sub': {
        const child = byId(ev.subId);
        const title = (child ? child.title : ev.title) || 'untitled';
        return evSmall('', ICON.branch, `added the sub-notch <a href="#/n/${esc(ev.subId)}" class="ev-link">${esc(title)}</a>`, ev.at);
      }
      case 'renamed': return evSmall('', ICON.pencil, `renamed this to <b>${esc(ev.to || 'untitled')}</b>`, ev.at);
      case 'status': {
        if (ev.status === 'done') return evSmall('good', ICON.done, 'closed this notch as <b>done</b>', ev.at);
        if (ev.status === 'not_planned') return evSmall('', ICON.skip, 'closed this notch as <b>not planned</b>', ev.at);
        return evSmall('accent', ICON.reopen, 'reopened this notch', ev.at);
      }
      default: return '';
    }
  }

  // The opening card anchors the log and hosts the live description — a
  // Write/Preview Markdown editor, the way a GitHub issue body sits atop its
  // timeline. Tasks are interactive in Preview; progress rides in the card head.
  function openingCardHTML(n) {
    const opened = (n.events || []).find((e) => e.kind === 'opened');
    const when = fmtDate(opened ? opened.at : n.createdAt);
    const ts = taskStats(n);
    const pct = ts.total ? Math.round((ts.done / ts.total) * 100) : 0;
    const tab = bodyTabFor(n);
    const rendered = (n.body || '').trim()
      ? `<div class="md-body" id="body-preview">${mdRender(n.body)}</div>`
      : '<div class="md-body empty" id="body-preview"><span class="swatch-note">No description yet — switch to Write to add one. Tasks live here as Markdown: <code>- [ ] do a thing</code>.</span></div>';
    const bodyPane = tab === 'write'
      ? `<textarea class="textarea md-input" id="body" placeholder="Describe this notch in Markdown. Add tasks with - [ ] …">${esc(n.body || '')}</textarea>`
      : rendered;
    return '<div class="ev card accent opening">' +
      `<span class="icon">${ICON.pencil}</span>` +
      '<div class="box"><div class="box-head"><span><b>you</b> opened this notch</span>' +
      `<span class="row" style="gap:12px;align-items:center">${ts.total ? `<span class="when">${ts.done}/${ts.total} tasks</span>` : ''}<span class="when">${when}</span></span></div>` +
      '<div class="box-body stack">' +
      '<div class="seg" id="body-tabs" role="group" aria-label="Description editor">' +
      `<button type="button" data-tab="write" aria-pressed="${tab === 'write'}">Write</button>` +
      `<button type="button" data-tab="preview" aria-pressed="${tab === 'preview'}">Preview</button></div>` +
      `${ts.total && tab === 'preview' ? `<div class="progress"><div class="bar"><i style="width:${pct}%"></i></div><span class="pct">${pct}%</span></div>` : ''}` +
      `${bodyPane}</div></div></div>`;
  }

  // The detail header packs the state, the parent, and the actions kebab onto one
  // line: [state pill] · [parent chip → the parent] · […]. The kebab is the one
  // place status changes now (Reopen / Close as done / Close as not planned), and
  // — when the notch has a parent — where it can be moved back to the top level.
  function headerActions(n) {
    const status = notchStatus(n);
    const parent = n.parentId ? byId(n.parentId) : null;
    const items = [];
    if (status !== 'open') items.push(`<button class="menu-item" type="button" data-set-status="open">${ICON.reopen}<span>Reopen</span></button>`);
    if (status !== 'done') items.push(`<button class="menu-item good" type="button" data-set-status="done">${ICON.done}<span>Close as done</span></button>`);
    if (status !== 'not_planned') items.push(`<button class="menu-item" type="button" data-set-status="not_planned">${ICON.skip}<span>Close as not planned</span></button>`);
    if (parent) items.push('<div class="menu-sep"></div>' +
      `<button class="menu-item" type="button" id="move-top">${ICON.move}<span>Move to top level</span></button>`);
    const menu =
      '<div class="menu" data-menu="status">' +
      `<button class="menu-btn" type="button" data-menu-trigger aria-haspopup="true" aria-expanded="false" aria-label="Change status">${ICON.dots}</button>` +
      `<div class="menu-pop right"><div class="menu-title">Status</div>${items.join('')}</div></div>`;
    return `${statePill(n)}${parent ? parentChip(parent) : ''}${menu}`;
  }

  // A default hex to seed a label's color pickers before it has custom colors
  // of its own — the paper theme's take on each palette swatch, just as a
  // reasonable starting point for the picker (the rendered chip itself still
  // follows the active theme until a color is actually picked).
  const PALETTE_HEX = { red: '#c0392b', amber: '#9a6b18', green: '#3f8a55', blue: '#3a5f9e', pink: '#a4478f', cyan: '#2f8a86', gray: '#6f6144' };

  // Labels sit right above the title: the live chips read-only, with a kebab whose
  // popover both adds new labels and removes existing ones, and lets each
  // label's background/text color be repicked — a change here is global, so it
  // shows up on every notch carrying that label, not just this one. Edits keep
  // the popover open so several labels can be managed in one go.
  function labelsRow(n) {
    const chips = n.tags.map((name) => labChip(name)).join('');
    const popChips = n.tags.length
      ? n.tags.map((name) => {
          const label = findLabel(name) || ensureLabel(name);
          const bgVal = label.bg || PALETTE_HEX[label.color] || PALETTE_HEX.gray;
          const fgVal = label.fg || autoContrast(bgVal);
          return '<div class="menu-label-row">' +
            `<button class="menu-label" type="button" data-del-tag="${esc(label.name)}" aria-label="remove ${esc(label.name)} label">` +
            `${labChip(label.name)}<span class="x" aria-hidden="true">&times;</span></button>` +
            `<input class="lab-color" type="color" data-label-bg="${esc(label.name)}" value="${esc(bgVal)}" title="Background color" aria-label="Background color for ${esc(label.name)} label"/>` +
            `<input class="lab-color" type="color" data-label-fg="${esc(label.name)}" value="${esc(fgVal)}" title="Text color" aria-label="Text color for ${esc(label.name)} label"/>` +
            '</div>';
        }).join('')
      : '<span class="swatch-note">No labels yet.</span>';
    return '<div class="labels-row">' +
      `<div class="labels-chips">${chips || '<span class="swatch-note">No labels</span>'}</div>` +
      '<div class="menu" data-menu="labels">' +
      `<button class="menu-btn sm" type="button" data-menu-trigger aria-haspopup="true" aria-expanded="false" aria-label="Edit labels">${ICON.dots}</button>` +
      '<div class="menu-pop right"><div class="menu-title">Labels</div>' +
      `<div class="menu-labels">${popChips}</div>` +
      '<form class="row" id="tag-form" autocomplete="off" style="margin-top:8px">' +
      '<input class="input" id="tag-name" type="text" placeholder="add a label…" style="flex:1 1 6rem" maxlength="24"/>' +
      '<button class="btn ghost sm" type="submit">Add</button></form></div></div></div>';
  }

  // A sub-notch row condenses cardHTML's title/tags/meta into one table line —
  // sub-notches are a roll-up, not a second notch list, so each one only needs
  // to be scannable at a glance, not laid out like its own card.
  function subRowHTML(n) {
    const s = taskStats(n);
    const kids = childrenOf(n.id).length;
    const comments = liveComments(n).length;
    const files = liveAttachments(n).length;
    const bits = [];
    if (kids) bits.push(`${kids} sub-notch${kids === 1 ? '' : 'es'}`);
    if (s.total) bits.push(`${s.done}/${s.total} task${s.total === 1 ? '' : 's'}`);
    if (comments) bits.push(`${comments} comment${comments === 1 ? '' : 's'}`);
    if (files) bits.push(`${files} file${files === 1 ? '' : 's'}`);
    const meta = bits.length ? bits.join(' · ') : '—';
    const tags = statusLab(n) + n.tags.map((name) => labChip(name)).join('');
    return `
      <tr>
        <td class="subs-title">
          <a href="#/n/${esc(n.id)}">${n.title ? esc(n.title) : '<span class="untitled">untitled</span>'}</a>
          ${tags ? `<span class="subs-tags">${tags}</span>` : ''}
        </td>
        <td class="subs-meta">${meta}</td>
        <td class="subs-updated">${fmtDate(n.updatedAt)}</td>
      </tr>`;
  }

  // Sub-notches now live at the foot of the notch box as a collapsible roll-up
  // (the header toggles it) with an Add popover that either creates a new child or
  // links an existing notch under this one.
  function subsBlock(n) {
    const kids = childrenOf(n.id);
    const collapsed = ensureDetailUI(n).subsCollapsed;
    const list = kids.length
      ? `<div class="notch-list"><table class="subs-table"><tbody>${kids.map(subRowHTML).join('')}</tbody></table></div>`
      : '<span class="swatch-note">No sub-notches yet.</span>';
    const links = linkTargets(n);
    const linkForm = links.length
      ? '<div class="menu-sep"></div><div class="menu-title">Link an existing notch</div>' +
        '<form class="stack" id="link-form" autocomplete="off" style="gap:8px">' +
        `<select class="select" id="link-target">${links.map((t) => `<option value="${esc(t.id)}">${esc(pathLabel(t))}</option>`).join('')}</select>` +
        '<button class="btn ghost sm" type="submit">Link as sub-notch</button></form>'
      : '';
    return `<div class="subs${collapsed ? ' collapsed' : ''}">` +
      '<div class="subs-head">' +
      `<button class="subs-toggle" type="button" id="subs-toggle" aria-expanded="${!collapsed}">` +
      `<span class="caret" aria-hidden="true">${ICON.caret}</span><span>Sub-notches</span>` +
      `${kids.length ? `<span class="count num">${kids.length}</span>` : ''}</button>` +
      '<div class="menu" data-menu="subs">' +
      `<button class="btn ghost sm add-sub" type="button" data-menu-trigger aria-haspopup="true" aria-expanded="false">${ICON.plus}<span>Add</span></button>` +
      '<div class="menu-pop right"><div class="menu-title">New sub-notch</div>' +
      '<form class="stack" id="sub-form" autocomplete="off" style="gap:8px">' +
      '<input class="input" id="sub-title" type="text" placeholder="new sub-notch title…"/>' +
      '<button class="btn primary sm" type="submit">Create</button></form>' +
      `${linkForm}</div></div></div>` +
      `<div class="subs-body">${list}</div></div>`;
  }

  function renderDetail(n) {
    titleBaseline = n.title || '';

    const crumbs = [`<a href="#/" class="back">all notches</a>`]
      .concat(trail(n).map((a) => `<a href="#/n/${esc(a.id)}" class="back">${esc(a.title) || 'untitled'}</a>`))
      .join('<span class="crumb-sep"> / </span>');

    // The timeline: the opening card (with the live description) first, then every
    // other event in chronological order. Deleted comments stay as tombstones.
    const events = (n.events || []).slice().sort((a, b) => a.at - b.at);
    const rest = events.filter((e) => e.kind !== 'opened').map((e) => eventHTML(n, e)).join('');

    view().innerHTML = `
      <p class="lede">← ${crumbs}</p>

      <section class="section">
        <h2><span>Notch</span><span class="row head-actions" style="gap:8px">${headerActions(n)}</span></h2>
        <div class="section-body stack">
          ${labelsRow(n)}
          <label class="field"><span>Title</span><input class="input title-input" id="title" type="text" value="${esc(n.title)}" placeholder="Title"/></label>
          ${subsBlock(n)}
        </div>
      </section>

      <section class="section">
        <h2><span>Activity</span><span class="count num">${events.length}</span></h2>
        <div class="section-body">
          <div class="timeline">
            ${openingCardHTML(n)}
            ${rest}
          </div>
          <form class="composer" id="comment-form" autocomplete="off">
            <span class="icon">${ICON.comment}</span>
            <div class="composer-box">
              <textarea id="comment-text" placeholder="Leave a comment (Markdown supported)…"></textarea>
              <div class="composer-foot">
                <span class="swatch-note">Comments join the log below — nothing here is ever deleted.</span>
                <div class="composer-actions">
                  <input id="attach-input" class="visually-hidden" type="file" multiple aria-hidden="true" tabindex="-1"/>
                  <button class="btn ghost sm" id="attach-btn" type="button">${ICON.clip}<span>Attach</span></button>
                  <button class="btn primary sm" type="submit">Comment</button>
                </div>
              </div>
            </div>
          </form>
        </div>
      </section>`;

    applyOpenMenu();
  }

  // ---------- lightbox ----------
  // A bare-bones full-screen image viewer for attachment previews: one overlay
  // appended to <body>, dismissed by clicking anywhere or pressing Escape. No
  // state to track beyond "is one open" — opening a second closes the first.
  function onLightboxKey(e) { if (e.key === 'Escape') closeLightbox(); }
  function closeLightbox() {
    const ov = document.querySelector('.lightbox');
    if (ov) ov.remove();
    document.removeEventListener('keydown', onLightboxKey);
  }
  function openLightbox(url, name) {
    closeLightbox();
    const ov = document.createElement('div');
    ov.className = 'lightbox';
    ov.setAttribute('role', 'dialog');
    ov.setAttribute('aria-label', name || 'attachment');
    ov.innerHTML =
      `<figure class="lightbox-fig"><img src="${esc(url)}" alt="${esc(name || '')}"/>` +
      `<figcaption>${esc(name || '')}</figcaption></figure>` +
      '<button class="lightbox-x" type="button" aria-label="Close">&times;</button>';
    ov.addEventListener('click', closeLightbox);
    document.body.appendChild(ov);
    document.addEventListener('keydown', onLightboxKey);
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
      ensureDetailUI(parent).subsCollapsed = false; // reveal the new child
      createNotch(title, parent.id).then(() => renderDetail(parent)).catch(() => {});
      return;
    }
    if (f.id === 'link-form') {
      e.preventDefault();
      const parent = currentDetail(); if (!parent) return;
      const sel = document.getElementById('link-target');
      const id = sel && sel.value;
      if (!id) return;
      ensureDetailUI(parent).subsCollapsed = false; // reveal the linked child
      linkChild(parent, id).then(() => renderDetail(parent)).catch(() => {});
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
      if (!n.tags.some((t) => t.toLowerCase() === name.toLowerCase())) {
        const label = ensureLabel(name); // reuses the global label's color if it already exists
        n.tags.push(label.name);
        logEvent(n, 'labeled', { name: label.name });
        detailMenu = 'labels'; // keep the labels popover open for the next add
        persist(n).then(() => renderDetail(n));
      } else {
        box.value = '';
      }
      return;
    }
  }

  // Toggle a detail popover (status / labels / add-sub). Opening one closes any
  // other, so only a single menu is ever open; the document-level onDocClick
  // closes them on an outside click.
  function toggleMenu(trigger) {
    const menu = trigger.closest('.menu');
    if (!menu) return;
    const willOpen = !menu.classList.contains('open');
    document.querySelectorAll('.menu.open').forEach((m) => {
      m.classList.remove('open');
      const t = m.querySelector('[data-menu-trigger]');
      if (t) t.setAttribute('aria-expanded', 'false');
    });
    if (willOpen) menu.classList.add('open');
    trigger.setAttribute('aria-expanded', String(willOpen));
  }

  function onDocClick(e) {
    const open = document.querySelectorAll('.menu.open');
    if (!open.length) return;
    open.forEach((m) => {
      if (m.contains(e.target)) return;
      m.classList.remove('open');
      const t = m.querySelector('[data-menu-trigger]');
      if (t) t.setAttribute('aria-expanded', 'false');
    });
  }

  function onClick(e) {
    if (e.target.closest('.back')) return; // hash links navigate themselves

    const menuTrigger = e.target.closest('[data-menu-trigger]');
    if (menuTrigger) { e.preventDefault(); toggleMenu(menuTrigger); return; }

    // The Attach button proxies a click to the hidden file input; the picker's
    // change event (onChange) does the actual work.
    if (e.target.closest('#attach-btn')) {
      e.preventDefault();
      const input = document.getElementById('attach-input');
      if (input) input.click();
      return;
    }

    // Clicking an image preview opens it full-size in the lightbox. Look the
    // event up by id so the (potentially large) data URL never rides in the DOM
    // attribute.
    const attView = e.target.closest('[data-attach-view]');
    if (attView) {
      e.preventDefault();
      const n = currentDetail(); if (!n) return;
      const ev = (n.events || []).find((x) => x.id === attView.getAttribute('data-attach-view'));
      if (ev) openLightbox(ev.dataUrl, ev.name);
      return;
    }
    const delAtt = e.target.closest('[data-del-attach]');
    if (delAtt) {
      e.preventDefault();
      const n = currentDetail(); if (!n) return;
      deleteAttachment(n, delAtt.getAttribute('data-del-attach')).then(() => renderDetail(n));
      return;
    }

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
      ensureDetailUI(n).bodyTab = tabBtn.getAttribute('data-tab');
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
      n.tags = n.tags.filter((t) => t !== name);
      logEvent(n, 'unlabeled', { name });
      detailMenu = 'labels'; // keep the labels popover open after removing one
      persist(n).then(() => renderDetail(n));
      return;
    }
    // Status changes live in the header kebab now: one handler for all three.
    const setStat = e.target.closest('[data-set-status]');
    if (setStat) {
      e.preventDefault();
      const n = currentDetail(); if (!n) return;
      setStatus(n, setStat.getAttribute('data-set-status')).then(() => renderDetail(n));
      return;
    }
    if (e.target.closest('#move-top')) {
      e.preventDefault();
      const n = currentDetail(); if (!n) return;
      if (!n.parentId) return;
      n.parentId = null;
      logEvent(n, 'moved', { toId: null, toTitle: '' });
      persist(n).then(() => renderDetail(n));
      return;
    }
    if (e.target.closest('#subs-toggle')) {
      e.preventDefault();
      const n = currentDetail(); if (!n) return;
      const ui = ensureDetailUI(n);
      ui.subsCollapsed = !ui.subsCollapsed;
      renderDetail(n);
      return;
    }
  }

  function onChange(e) {
    // A label's color pickers write straight to the global registry — the
    // change is visible on every notch that carries this label, not just the
    // one whose popover is open, so this doesn't persist() any single notch.
    const bgInput = e.target.closest('[data-label-bg]');
    if (bgInput) {
      const label = findLabel(bgInput.getAttribute('data-label-bg'));
      if (!label) return;
      label.bg = bgInput.value;
      detailMenu = 'labels'; // keep the labels popover open
      const n = currentDetail();
      if (n) renderDetail(n);
      return;
    }
    const fgInput = e.target.closest('[data-label-fg]');
    if (fgInput) {
      const label = findLabel(fgInput.getAttribute('data-label-fg'));
      if (!label) return;
      label.fg = fgInput.value;
      detailMenu = 'labels'; // keep the labels popover open
      const n = currentDetail();
      if (n) renderDetail(n);
      return;
    }
    // Picking files in the hidden input attaches each to the current notch. Clear
    // the input's value so re-picking the same file fires change again.
    if (e.target.id === 'attach-input') {
      const n = currentDetail(); if (!n) return;
      const files = Array.from(e.target.files || []);
      e.target.value = '';
      if (!files.length) return;
      addAttachments(n, files).then((skipped) => {
        renderDetail(n);
        if (skipped && skipped.length) {
          alert(`Too large to attach in this demo (max ${fmtBytes(MAX_ATTACH_BYTES)}):\n${skipped.join('\n')}`);
        }
      });
      return;
    }
    // Blur/commit on the description textarea saves immediately, so a pending
    // debounced edit isn't lost when the next click re-renders the detail view.
    if (e.target.id === 'body') {
      const n = currentDetail(); if (!n) return;
      clearTimeout(bodyTimer);
      if (n.body !== e.target.value) { n.body = e.target.value; persist(n); }
      return;
    }
    // Renaming: a blur on the title commits the change and, when it actually
    // differs from the baseline (and the notch already had a name), records a
    // `renamed` event. The debounced live edit in onInput never logs — only this
    // commit does, so a rename lands once, not once per keystroke.
    if (e.target.id === 'title') {
      const n = currentDetail(); if (!n) return;
      clearTimeout(titleTimer);
      const v = e.target.value.trim();
      if (v === titleBaseline) { n.title = v; return; }
      n.title = v;
      if (titleBaseline) logEvent(n, 'renamed', { from: titleBaseline, to: v });
      titleBaseline = v;
      persist(n).then(() => renderDetail(n));
      return;
    }
    // Ticking a task checkbox in the rendered description flips the matching
    // `- [ ]` in the Markdown source and records which task changed.
    const taskBox = e.target.closest('.md-body input[data-task]');
    if (taskBox) {
      const n = currentDetail(); if (!n) return;
      const idx = parseInt(taskBox.getAttribute('data-task'), 10);
      const info = taskInfo(n.body, idx);
      n.body = toggleTask(n.body, idx);
      if (info) logEvent(n, 'task', { text: info.text, done: !info.done });
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

  // ---------- demo bar ----------
  // A slim banner under the header spelling out that nothing is saved, with a
  // Reset that drops back to the seed. Injected here (not in the page template)
  // so it stays wired to this file's demo behaviour.
  function mountDemoBar() {
    const anchor = document.getElementById('ticker');
    if (!anchor || document.querySelector('.demo-bar')) return;
    const bar = document.createElement('div');
    bar.className = 'demo-bar';
    bar.setAttribute('role', 'note');
    bar.innerHTML =
      '<span class="demo-bar-msg"><b>Demo</b> nothing is saved — reloading the page wipes your changes.</span>' +
      '<button class="btn ghost sm" id="demo-reset" type="button">Reset demo</button>';
    anchor.parentNode.insertBefore(bar, anchor);
    bar.querySelector('#demo-reset').addEventListener('click', resetDemo);
  }

  function resetDemo() {
    if (!confirm('Reset the demo? This clears your changes and restores the starting notches.')) return;
    labels = demoLabels();
    notches = demoSeed();
    ticker();
    if (location.hash) location.hash = ''; // detail view → list (fires route via hashchange)
    else route();
  }

  // ---------- boot ----------
  function boot() {
    initTheme();
    load();
    mountDemoBar();
    ticker();
    const mount = view();
    mount.addEventListener('submit', onSubmit);
    mount.addEventListener('click', onClick);
    mount.addEventListener('change', onChange);
    mount.addEventListener('input', onInput);
    document.addEventListener('click', onDocClick);
    window.addEventListener('hashchange', route);
    route();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
