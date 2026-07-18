# Security

tally handles financial data and a bearer credential (the SimpleFIN access
URL). The threat model and secret-handling policy live in
[`docs/security/`](docs/security/):

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
