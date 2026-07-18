# Secret handling

Issue: [#6](https://github.com/clarkbar-sys/tally/issues/6). Read alongside the
[threat model](threat-model.md).

## What counts as a secret here

| Secret | Shape | Why it's dangerous |
|--------|-------|--------------------|
| **SimpleFIN access URL** | `https://<user>:<pass>@<host>.simplefin.org/simplefin` | A **bearer credential** — this one string grants read access to account data. No second factor, no rotation-on-use. |
| SimpleFIN setup token | base64 of a one-time claim URL | Exchanged once for the access URL; still sensitive until claimed. |
| Plaid tokens (future) | `access-<env>-<uuid>`, client secret | Same bearer-credential problem for the Plaid adapter (#8). |
| restic repo password | passphrase | Decrypts the backup, which contains the whole datastore (#17). |

The access URL is exactly the kind of value that ends up pasted into a test
fixture or a debug log. Everything below exists to make that impossible by
policy, not just by care.

## Where secrets live on the box — **decision**

**systemd credentials (`LoadCredential=`), backed by a root-owned `0600` file
outside the repo tree.** Not process environment variables.

```ini
# tally.service (sketch — full unit lands in #16)
[Service]
LoadCredential=simplefin-access-url:/etc/tally/simplefin-access-url
# tally reads it from $CREDENTIALS_DIRECTORY/simplefin-access-url at startup.
```

```
/etc/tally/simplefin-access-url   root:root  0600   # source of truth
```

Rationale, and why **not** an env var:

- Environment variables leak. They're readable via `/proc/<pid>/environ`, show
  up in crash dumps and error reporters, and get inherited by child processes.
  For a bearer credential that's an unnecessary blast radius.
- `LoadCredential=` places the secret in a tmpfs directory that only the service
  can read, referenced by `$CREDENTIALS_DIRECTORY` — never in the environment,
  never on disk unencrypted at rest beyond the source file.
- No secrets manager: a single-user box with one credential doesn't justify
  Vault/etc. The systemd primitive is the right weight.

Non-secret config (DB path, listen address, sync interval) may use environment
or flags — see `.env.example` when scaffolding lands (#5). **Secrets never go in
`.env`.** `.env*` is gitignored regardless.

> This is the one genuine decision inside #6. It's documented and reversible;
> flag it if you'd rather a plain `0600` env-file `EnvironmentFile=` instead —
> I chose `LoadCredential` for the smaller leak surface.

## Test fixture policy — **synthetic, always**

**No real API response is ever committed, even redacted.** Redaction is
unreliable (a missed field, an ID that correlates back) and a redacted-but-real
fixture normalises committing real data.

- Every fixture under `testdata/` is hand-authored or generator-produced
  **synthetic** data.
- Any credential-shaped value in a fixture or doc is an obvious placeholder:
  `<username>`, `EXAMPLE`, `REDACTED`, `access-sandbox-00000000-0000-0000-0000-000000000000`.
  These are allowlisted in `.gitleaks.toml`; a value that isn't a recognised
  placeholder trips the gate.
- Verifying the SimpleFIN adapter against the **real** API (#9) is done locally
  against a live credential and its output is **never** saved into the repo.

## Rotation — when a token leaks (write this down *before* you need it)

Assume any leaked SimpleFIN access URL is compromised the moment it lands
somewhere it shouldn't (git history, a log, a screenshot, a chat).

1. **Revoke at the source.** In the SimpleFIN bridge, delete/deauthorize the
   access that issued the URL. The URL is the credential — invalidating it at
   the bridge is the only thing that actually stops access. Rotating the file on
   the box does nothing until the old URL is dead upstream.
2. **Re-provision.** Generate a new setup token, claim it for a fresh access
   URL, write it to `/etc/tally/simplefin-access-url` (`0600`), and
   `systemctl restart tally` (a `daemon-reload` + explicit `restart` — per #16,
   `enable --now` does **not** restart a running service).
3. **If it reached git history:** rotating is step 1. Then treat the history as
   contaminated — a plain revert does not remove it. Scrub with
   `git filter-repo` (or BFG) and force-push, and rotate anything else that
   shared that commit. This is far cheaper before the repo goes public than
   after.
4. **Record it.** Note what leaked, where, and when in the incident trail so the
   public-visibility flip (below) can be signed off honestly.

## Before this repo goes public

Visibility is **private today, public later once secret handling is proven**
(epic #1). All of the following must be true *before* the flip, and each is
cheaper now than as a history audit later:

- [ ] gitleaks gate green across **full history**, not just the tip
      (`gitleaks detect --config .gitleaks.toml`).
- [ ] No real fixture ever committed (policy above, enforced by review).
- [ ] `.gitignore` covers `.env*`, `*.db`/`*.sqlite*`, dumps, exports.
- [ ] Rotation procedure above has been dry-run at least once.
