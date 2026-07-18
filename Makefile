# tally — dev tasks. See CONTRIBUTING.md.

.PHONY: hooks fmt vet test build gitleaks lint ci

## Enable the committed pre-push gate (run once per clone).
hooks:
	git config core.hooksPath .githooks
	@echo "core.hooksPath -> .githooks"

fmt:
	gofmt -w .

vet:
	go vet ./...

test:
	go test -race ./...

build:
	go build ./...

gitleaks:
	gitleaks detect --config .gitleaks.toml --redact --no-banner

## Formatting check (non-mutating), as CI runs it.
lint:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "unformatted (run: make fmt):"; echo "$$unformatted"; exit 1; \
	fi

## Everything CI runs, locally.
ci: lint vet build test gitleaks
