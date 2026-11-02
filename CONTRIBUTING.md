# Contributing to Pinfold

Thanks for your interest. Pinfold is pre-alpha: the spec language and the guarantees are settling, and the lock (milestone M4) is still being built. Useful contributions now: **issues, design feedback, incident fixtures, registry entries and docs**. Self-contained tasks are labelled [`good first issue`](https://github.com/pinfold-ci/pinfold/labels/good%20first%20issue).

Questions go to [Discussions](SUPPORT.md); labels are explained in [docs/labels.md](docs/labels.md); versioning and releases in [docs/releases.md](docs/releases.md). Maintainers are listed in [MAINTAINERS.md](MAINTAINERS.md).

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
- **Core changes** (taint, zones, verifier, lock, sanitiser) need an [RFC in GitHub Discussions](https://github.com/pinfold-ci/pinfold/discussions/categories/rfcs) first.

## Developer Certificate of Origin

Contributions are accepted under the [Developer Certificate of Origin](https://developercertificate.org/). Sign off each commit with `git commit -s`, which adds a `Signed-off-by:` line with your name and email. By signing off you certify that you wrote the change or otherwise have the right to submit it under the project's license (Apache-2.0).

The `dco` check on each pull request looks for a `Signed-off-by:` line with the **commit author's email** on every commit (merge commits excepted). If it fails, add the sign-off to every commit in your branch and force-push:

```bash
git rebase --signoff origin/main
git push --force-with-lease
```

For a single commit, `git commit --amend -s --no-edit` does the same. Commits by the maintainers listed in `.github/dco-exempt-authors` are exempt.

## AI-assisted contributions

AI-assisted contributions are welcome if you **disclose** the assistance in the PR description and you **understand and can explain** every line you submit. You are responsible for the change, not the tool. Undisclosed, unreviewed AI output will be closed.

Pinfold itself is developed with AI assistance under human direction; security-critical code is independently reviewed.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).
