-- App-protocol persistence (initiative #95, S0 / #96). These tables back the
-- app -> proposal -> merge loop that today runs entirely in-memory in
-- internal/web/static/app.js: registered actors (apps) author proposals
-- ("tallies") whose typed diffs, once merged, write to tally's substrate
-- (notches + records). Column shapes follow app.js so S8's client swap is
-- mechanical.
--
-- Polymorphic, engine-owned payloads -- a proposal's `changes`, an event's
-- kind-specific fields, an app's `scopes`/`action`, a notch's `tags` -- are kept
-- as JSON text columns rather than a per-op relational schema: the engine (S1)
-- owns their semantics, the store just round-trips them. This keeps the change
-- vocabulary free to evolve without a migration per op.
--
-- Timestamps are RFC3339Nano UTC text, stamped by the store on write (same
-- convention as 0001_init; see internal/store/time.go).

-- apps: the registered actors. Matches pact's app.schema.json.
CREATE TABLE apps (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    kind         TEXT NOT NULL,                    -- 'you' | 'connected' | 'local'
    color        TEXT NOT NULL DEFAULT '',
    blurb        TEXT NOT NULL DEFAULT '',
    scopes       TEXT NOT NULL DEFAULT '[]',       -- JSON array of 'resource:verb'
    action       TEXT,                             -- JSON {label,verb} or NULL
    status       TEXT NOT NULL DEFAULT 'active',   -- 'active' | 'revoked'
    installed_at TEXT NOT NULL
);

-- notches: the issue-like substrate containers. A notch is never deleted; the
-- self-reference nests sub-notches under a parent.
CREATE TABLE notches (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL DEFAULT '',
    body       TEXT NOT NULL DEFAULT '',
    tags       TEXT NOT NULL DEFAULT '[]',         -- JSON array of label names
    parent_id  TEXT REFERENCES notches (id) ON DELETE CASCADE,
    status     TEXT NOT NULL DEFAULT 'open',       -- 'open' | 'done' | 'not_planned'
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_notches_parent ON notches (parent_id);
CREATE INDEX idx_notches_updated ON notches (updated_at DESC);

-- notch_events: a notch's append-only timeline (GitHub-issue style). `seq`
-- orders entries by append; `payload` holds the kind-specific fields.
CREATE TABLE notch_events (
    seq      INTEGER PRIMARY KEY AUTOINCREMENT,
    id       TEXT NOT NULL,
    notch_id TEXT NOT NULL REFERENCES notches (id) ON DELETE CASCADE,
    kind     TEXT NOT NULL,
    at       TEXT NOT NULL,
    payload  TEXT NOT NULL DEFAULT '{}'            -- JSON, kind-specific fields
);

CREATE INDEX idx_notch_events_notch ON notch_events (notch_id, seq);

-- proposals: the PR-like reviewable objects ("tallies"). `changes` is the typed
-- diff and `linked_notches` the notches the merge closes, both engine-owned JSON.
CREATE TABLE proposals (
    id             TEXT PRIMARY KEY,
    app_id         TEXT NOT NULL REFERENCES apps (id),   -- author
    title          TEXT NOT NULL DEFAULT '',
    body           TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'open',         -- 'open' | 'merged' | 'declined'
    changes        TEXT NOT NULL DEFAULT '[]',           -- JSON array (engine-owned diff)
    linked_notches TEXT NOT NULL DEFAULT '[]',           -- JSON array of notch ids
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    merged_at      TEXT                                  -- set once merged
);

CREATE INDEX idx_proposals_app ON proposals (app_id);
CREATE INDEX idx_proposals_updated ON proposals (updated_at DESC);

-- proposal_events: a proposal's append-only timeline, same shape as notch_events.
CREATE TABLE proposal_events (
    seq         INTEGER PRIMARY KEY AUTOINCREMENT,
    id          TEXT NOT NULL,
    proposal_id TEXT NOT NULL REFERENCES proposals (id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    at          TEXT NOT NULL,
    payload     TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_proposal_events_proposal ON proposal_events (proposal_id, seq);

-- records: tally's data substrate, admitted by merging a proposal. Provenance
-- travels with every row: `app_id` (which app admitted it) and `proposed_by`
-- (the proposal it came from -- app.js's `talliedFrom`).
CREATE TABLE records (
    id          TEXT PRIMARY KEY,
    dataset     TEXT NOT NULL,
    kind        TEXT NOT NULL DEFAULT 'text',         -- 'text' | 'blob'
    summary     TEXT NOT NULL DEFAULT '',             -- text records
    name        TEXT NOT NULL DEFAULT '',             -- blob records
    mime        TEXT NOT NULL DEFAULT '',
    size        INTEGER NOT NULL DEFAULT 0,
    blob_url    TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL DEFAULT '',             -- author display name
    app_id      TEXT NOT NULL REFERENCES apps (id),        -- provenance: admitting app
    proposed_by TEXT NOT NULL REFERENCES proposals (id),  -- provenance: source proposal
    at          TEXT NOT NULL
);

CREATE INDEX idx_records_dataset ON records (dataset, at DESC);
CREATE INDEX idx_records_proposal ON records (proposed_by);
