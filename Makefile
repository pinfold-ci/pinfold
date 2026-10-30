GO ?= go
ACTIONLINT_VERSION := v1.7.12

.PHONY: test lint golden actionlint check

test:
	$(GO) test -race ./...

lint:
	$(GO) vet ./...
	golangci-lint run

golden:
	$(GO) test ./... -update

actionlint:
	$(GO) run github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION)

check: test lint actionlint
