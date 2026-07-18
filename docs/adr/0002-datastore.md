# 0002 — Datastore

- **Status:** Accepted
- **Date:** 2026-07-18
- **Issue:** [#4](https://github.com/clarkbar-sys/tally/issues/4)
- **Supersedes:** —

## Context

Single user, modest volume — years of transactions is still small. The
annotation layer implies relational queries (tags across transactions, stories
spanning date ranges), but nothing beyond what an embedded relational engine
handles comfortably at this scale.

The hinge question from #4 is whether tally is ever multi-user. Decision: it
stays single-user (auth is deferred in [0003](README.md#deferred-auth-3) but the
epic frames tally as single-user "for now", and the family-shareable surface is
explicitly a *separate service*, not more users of this one).

## Options considered

- **SQLite** — one file, trivial backup, plenty fast at this scale.
- **Postgres** — already on the box; real migrations and concurrent access if
  tally ever grew past one user. Heavier, another service to run and back up.

## Decision

**SQLite**, one file on the box.

## Consequences

- **Backup is copying a file** — ties cleanly into the restic setup (#17), which
  matters because annotation is irreplaceable by definition.
- **Migrations:** a lightweight, versioned, forward-only migration runner (plain
  `.sql` files embedded via `embed`, applied at startup). No ORM.
- Enable **WAL mode** and a busy timeout — even single-user, the scheduled sync
  job (#10) and the web handler will touch the DB concurrently.
- The canonical model (#7) must keep **provider fields separate from tally-owned
  fields** (tags, notes, story refs) at the schema level, so a re-sync updates
  provider data without ever clobbering annotation. That separation is a schema
  concern this decision enables, resolved in #7.
- **Revisit only if** tally goes genuinely multi-user or concurrent write
  contention shows up — supersede with a Postgres ADR at that point.
