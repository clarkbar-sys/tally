# Threat model

Issue: [#6](https://github.com/clarkbar-sys/tally/issues/6). Operational
specifics live in [secrets.md](secrets.md).

This is a threat model for a **single-user, tailnet-only, self-hosted** ledger,
scoped to what actually threatens *this* app. It is deliberately not a generic
web-app checklist. It will need one revision when auth is decided
([#3](https://github.com/clarkbar-sys/tally/issues/3), currently deferred) — the
tailnet-perimeter assumption below is exactly what that ADR must justify.

## Assets, worst first

1. **The SimpleFIN access URL** — a bearer credential granting read access to
   real financial accounts. Compromise = someone else reads the accounts.
2. **The annotation layer** — tags, notes, stories. **Irreplaceable by
   definition**: transactions can be re-pulled from the provider, hand-written
   provenance cannot. Loss = losing the actual product.
3. **The financial data itself** — balances and transaction history. Sensitive
   even though re-derivable.
4. **The restic backup** — contains 1–3 in one place (#17).

## Trust boundaries

```
  SimpleFIN bridge  ──(access URL)──►  tally on the box  ──(tailscale serve)──►  Josh's tailnet devices
        │                                    │                                        │
   external SaaS                    the box (systemd,                          the perimeter this
   we don't control                 SQLite, secrets)                           model must justify
```

- **Tailnet perimeter.** v1 trusts it: on the tailnet ⇒ allowed to reach tally.
  This is an *assumption*, recorded here so #3 can accept or replace it. Hard
  rule from the epic: **never `funnel`, never public ingress** — `tailscale
  serve` only. Nothing here is exposed to the open internet.
- **Access-domain boundary (day-one constraint).** tally must **never share an
  access domain** with the family-shareable surface. A family invitee hitting
  `tally.taileafxyz.net` must be **denied by a different authz system**, not by
  a role check inside a shared one. Until #3 lands, nothing may assume shared
  auth, and tally must not be co-deployed behind the family surface's session.
- **The box.** Root on the box ⇒ game over (reads the credential, the DB, the
  backup password). Out of scope to defend against; in scope to not *widen*
  (hence `0600`, `LoadCredential`, no secret in env).
- **GitHub.** Private today, **public later** (#1). The repo boundary is a real
  threat surface: anything committed now is visible after the flip, including
  history. See "Repo goes public" below.

## Threats and mitigations

### T1 — Credential leaks into the repo (highest-likelihood, this is the one)
A SimpleFIN URL or Plaid token gets pasted into a fixture, a log snippet, a
commit message, or a debug `fmt.Println`.

- **gitleaks** in the pre-push gate and CI, with rules targeting the SimpleFIN
  URL shape and Plaid tokens specifically (`.gitleaks.toml`, wired by #5).
- Secret stored via `LoadCredential`, never in env → far less likely to surface
  in logs/dumps in the first place (secrets.md).
- **Synthetic-only fixture policy** — no real response ever committed, even
  redacted (secrets.md).
- Hardened `.gitignore` — `.env*`, `*.db`, `*.sqlite*`, dumps, exports.

### T2 — Repo goes public with a secret in history
The flip from private → public exposes *all* history at once.

- Pre-flip checklist in secrets.md (gitleaks green over full history, not just
  tip; fixtures verified synthetic).
- If a leak is found, rotation (secrets.md) **plus** history scrub
  (`git filter-repo`) before flipping — cheaper now than as a post-hoc audit.

### T3 — Credential compromise despite the above
Leak via a channel git can't see (screenshot, chat, a third-party log).

- Documented, dry-run **rotation procedure** (secrets.md): revoke at the bridge
  first (the URL *is* the credential), then re-provision on the box.

### T4 — Loss of the annotation layer
Disk failure, bad migration, accidental `DROP`.

- restic backup (#17), **restore actually tested** — not merely configured.
- Backup inherits asset #1 and #4's sensitivity: the restic repo is encrypted
  and its password is itself a secret (secrets.md). A backup you can't restore,
  or one that leaks, is its own incident.

### T5 — Shared access domain with the family surface
The epic's day-one architectural failure mode: reusing one auth so a family
invite can, by role, reach financial data.

- Deferred to #3, but constrained *now*: separate service, separate authz, no
  shared session. This model forbids co-deploying tally behind the family
  surface's auth before #3 resolves it explicitly.

### T6 — Stale data that looks current (integrity, not confidentiality)
Out of scope for *this* doc but noted so it isn't dropped: "stale financial data
that looks current is worse than an obvious error" (#12). Handled in the browse
phase, not here.

## Explicitly out of scope for v1

- Multi-user authz, RBAC (single-user; see [ADR-0002](../adr/0002-datastore.md)).
- Defending against a compromised box root.
- Network attacks from outside the tailnet (no public ingress by construction).
- Provider-side (SimpleFIN bridge) compromise — we can revoke and rotate, not
  prevent.

Revisit this model when #3 lands (perimeter/auth) and when Plaid is added (#8).
