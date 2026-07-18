## What & why

<!-- What does this change, and which issue does it close? e.g. "Closes #12" -->

## How

<!-- Notable implementation choices or tradeoffs; what a reviewer should look at first. -->

## Checklist

- [ ] `make ci` passes locally (gofmt, vet, build, test, gitleaks)
- [ ] No real credentials or provider responses committed — fixtures are synthetic ([secrets.md](../docs/security/secrets.md))
- [ ] Docs/ADRs updated if this changes a decision or the security posture
- [ ] If this touches ingest: provider re-sync never clobbers tally-owned annotation (#7)
