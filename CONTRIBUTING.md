# Contributing to Pinfold

Thanks for your interest. Pinfold is pre-alpha: the core (spec, IR, trust zones, taint, lock) is still changing quickly, so for now the most useful contributions are **issues, design feedback and incident fixtures**. Smaller self-contained tasks will be labelled `good first issue` from milestone M3.

## Development setup

You need Go (the version in `go.mod`) and `make`.

```bash
make test     # unit and golden tests
make lint     # go vet + golangci-lint
make check    # everything CI runs that works offline
```

Golden files live next to the tests in `testdata/`. To regenerate them after an intended change, run `make golden` and review the diff carefully.

## Pull requests

- **One change per PR.** Small PRs get reviewed quickly.
- **Tests first.** Every behaviour change comes with a test that fails without it. Security rules (zones, taint, permissions, verifier, lock) need a test that *tries to break* the rule.
- **Conventional commits:** `feat:`, `fix:`, `test:`, `docs:`, `ci:`, `chore:`, `refactor:`.
- **CI must be green.** `main` is protected; nothing is pushed to it directly.
- **Core changes** (taint, zones, verifier, lock, sanitiser) need an RFC in GitHub Discussions first.

## Developer Certificate of Origin

Contributions are accepted under the [Developer Certificate of Origin](https://developercertificate.org/). Sign off each commit with `git commit -s`, which adds a `Signed-off-by:` line with your name and email. By signing off you certify that you wrote the change or otherwise have the right to submit it under the project's license (Apache-2.0).

## AI-assisted contributions

AI-assisted contributions are welcome if you **disclose** the assistance in the PR description and you **understand and can explain** every line you submit. You are responsible for the change, not the tool. Undisclosed, unreviewed AI output will be closed.

Pinfold itself is developed with AI assistance under human direction; security-critical code is independently reviewed.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).
