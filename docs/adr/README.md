# Architecture Decision Records

Short, dated records of decisions that are expensive to reverse. One file per
decision, numbered in order. A record is immutable once `Accepted` — supersede
it with a new ADR rather than editing the old one.

| ADR | Decision | Status |
|-----|----------|--------|
| [0001](0001-language-and-stack.md) | Language & stack — Go + templ + HTMX | Accepted |
| [0002](0002-datastore.md) | Datastore — SQLite | Accepted |
| [0003](0003-tailnet-integration.md) | Tailnet integration — embed tsnet | Accepted |
| — | Auth model & access domain | **Deferred** — see [#3](https://github.com/clarkbar-sys/tally/issues/3) |

**Note:** [Epic #1](https://github.com/clarkbar-sys/tally/issues/1) pivoted v1
to a local-first, no-server build. 0001–0003 stay accepted as the answer for
*when* a server exists — they don't describe what v1 runs today.

## Deferred: auth (#3)

App-level auth is parked, not decided — it will get its own ADR when taken up.
The epic's hard constraint stands regardless of which option wins: tally must
**never share an access domain** with the family-shareable surface. The answer
to "a family invitee hits `tally.taileafxyz.net`" must be *"denied by a different
authz system"* — not a role check inside a shared one.

ADR-0003 already delivers the *interim* answer: tally is its own tagged tailnet
node, so the **tailnet ACL** (a different authz system) is the boundary today.
Full app-level auth still lands with #3. Nothing built before then may assume
shared auth.
