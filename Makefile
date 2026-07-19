# tally — dev tasks. See CONTRIBUTING.md.

# Keep TEMPL_VERSION in sync with the github.com/a-h/templ require in go.mod.
TEMPL_VERSION := v0.3.977

.PHONY: hooks generate check-generate fmt vet test build gitleaks lint ci

## Enable the committed pre-push gate (run once per clone).
hooks:
	git config core.hooksPath .githooks
	@echo "core.hooksPath -> .githooks"

## Regenerate templ templates (*_templ.go). Run after editing any .templ file.
generate:
	go run github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION) generate

## Verify committed *_templ.go match the .templ sources (non-mutating in CI).
check-generate: generate
	@if ! git diff --quiet -- '*_templ.go'; then \
		echo "generated templ output is stale; run: make generate, then commit"; \
		git --no-pager diff --stat -- '*_templ.go'; exit 1; \
	fi

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
ci: check-generate lint vet build test gitleaks
