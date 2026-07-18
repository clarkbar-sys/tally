# tally
Personal ledger with provenance — aggregate financial data, then annotate it over time. Tailnet-only.

A [tally stick](https://en.wikipedia.org/wiki/Tally_stick): notched wood, split so both parties hold half the record. Pull financial data in automatically, then let it be *annotated* — tagged, narrated, and threaded into stories that span months. Aggregation is table stakes; the annotation layer is the point.

## Stack

Go + [templ](https://templ.guide/) + HTMX, SQLite. See [`docs/adr/`](docs/adr/) for the decisions and why.

## Development

Requires Go 1.24+ and [gitleaks](https://github.com/gitleaks/gitleaks).

```sh
make hooks   # enable the pre-push gate (once per clone)
make ci      # gofmt, vet, build, test, gitleaks
```

See [CONTRIBUTING.md](CONTRIBUTING.md). Security posture: [`docs/security/`](docs/security/).

## License

[GPL-2.0-or-later](LICENSE).
