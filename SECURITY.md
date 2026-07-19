# Security

tally is currently a local-first, client-only app with no server and no
credentials. Financial data (via a SimpleFIN access URL, a bearer credential)
and the server/sync surface that would handle it are deferred to a later phase
— see [epic #1](https://github.com/clarkbar-sys/tally/issues/1). The threat
model and secret-handling policy already written for that phase live in
[`docs/security/`](docs/security/) and apply once it's built:

- [Threat model](docs/security/threat-model.md)
- [Secret handling, fixture policy, rotation](docs/security/secrets.md)

## Reporting

This is a private, single-user project. If you are reading this after it has
gone public and have found a vulnerability, please report it privately to the
maintainer rather than opening a public issue.

## Ground rules (enforced by CI)

- No real credential or provider response is ever committed, even redacted.
  Fixtures are synthetic. `gitleaks` runs in the pre-push gate and in CI over
  full history.
- Secrets on the box live in systemd credentials, never in the environment or
  the repo. See [`docs/security/secrets.md`](docs/security/secrets.md).
