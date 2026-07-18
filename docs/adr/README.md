# Architecture Decision Records

Short, dated records of decisions that are expensive to reverse. One file per
decision, numbered in order. A record is immutable once `Accepted` — supersede
it with a new ADR rather than editing the old one.

| ADR | Decision | Status |
|-----|----------|--------|
| [0001](0001-language-and-stack.md) | Language & stack — Go + templ + HTMX | Accepted |
| [0002](0002-datastore.md) | Datastore — SQLite | Accepted |
| 0003 | Auth model & access domain | **Deferred** — see [#3](https://github.com/clarkbar-sys/tally/issues/3) |

## Deferred: auth (#3)

Auth is parked, not decided. It blocks *deploy* (#16), not the early build, so
it resolves later. The epic's hard constraint stands regardless of which option
wins: tally must **never share an access domain** with the family-shareable
surface. The answer to "a family invitee hits `tally.taileafxyz.net`" must be
*"denied by a different authz system"* — not a role check inside a shared one.
Nothing built before 0003 lands may assume shared auth.
