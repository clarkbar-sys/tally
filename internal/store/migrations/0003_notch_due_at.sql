-- Notch due dates (initiative #95, mirrors internal/web/static/app.js's `dueAt`).
-- A notch gains an optional due date: the day something is meant to be done by.
-- NULL means no due date, matching the client's `dueAt: null`. Stored as
-- RFC3339Nano UTC text like every other timestamp (see internal/store/time.go),
-- pinned by the client to the start of the chosen day. Forward-only ALTER so the
-- app-protocol schema stays in step with the client without rewriting 0002.
ALTER TABLE notches ADD COLUMN due_at TEXT;
