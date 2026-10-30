# AGENTS.md

Guidance for coding assistants and new contributors working in this repository.

## Commands

```bash
make test     # go test ./...
make lint     # go vet + golangci-lint
make golden   # regenerate golden files (review the diff!)
make check    # everything CI runs offline
```

## Layout

- `cmd/pinfold/`: the CLI entry point. Keep it thin.
- `internal/`: everything else. Nothing is a public Go API before v1.
- `testdata/` next to each package: fixtures and golden files.
- `docs/design/`: the design. `docs/adr/`: decision records.

## Conventions

- Go, standard library first. A new dependency needs a reason in the PR.
- Tests first. Security rules need a test that tries to break them.
- Diagnostics: every error has a stable code (`PFxxx`), a `file:line:col` position and a fix hint. Codes are never reused.
- Output must be deterministic: same input, same bytes.
- Conventional commits; one change per PR.
- Never weaken a guarantee in design §2 without an RFC.
