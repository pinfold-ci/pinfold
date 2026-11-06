# Pinfold

[![ci](https://github.com/pinfold-ci/pinfold/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/pinfold-ci/pinfold/actions/workflows/ci.yml)
[![codeql](https://github.com/pinfold-ci/pinfold/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/pinfold-ci/pinfold/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/pinfold-ci/pinfold/badge)](https://scorecard.dev/viewer/?uri=github.com/pinfold-ci/pinfold)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/16383/badge)](https://www.bestpractices.dev/projects/16383)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

> **Status: pre-alpha.** The compiler and its guarantees work and are tested live, but the lock (`pinfold.lock`, milestone M4) isn't built yet and the spec format may still change. Don't use it in production yet. See [ROADMAP.md](ROADMAP.md).

**The compiler won't let a CI job both run untrusted code and hold your secrets.**

Pinfold compiles a small, declarative spec (`pinfold.yml`) into GitHub Actions workflows that are secure by construction. Its first target is the riskiest automation an open-source project runs: **releases** (PyPI trusted publishing, GitHub Releases) and **comment and label bots**.

- **Trust zones per job.** A job that holds secrets, OIDC or write permissions can't run your own `run:` steps, and a job that runs your code can't hold credentials. Violations are compile errors, not lint warnings.
- **Tainted by default.** Event data such as titles, branch names and comment text never reaches a `run:`, `script:` or `with:` value. It only arrives through `env:`.
- **Declared, checked permissions.** Each job gets exactly the permissions it declares plus what the compiler generates, and nothing else.
- **Pinned and locked.** Every action reference is a commit SHA recorded in `pinfold.lock`. If a tag moves, the next update or verification fails.
- **No runtime.** The generated workflows are plain, readable YAML and never need the `pinfold` binary to run. If you stop using Pinfold, keep the files.

## Quick start

You need Go (the version in [`go.mod`](go.mod)). There are no release binaries before v0.1.

```bash
go install github.com/pinfold-ci/pinfold/cmd/pinfold@latest
pinfold init                  # writes a starter pinfold.yml and prints the repo settings to turn on
pinfold validate pinfold.yml  # diagnostics with file:line:col, a code and a fix hint
pinfold compile pinfold.yml   # writes .github/workflows/pinfold-*.yml; commit those files
```

`pinfold compile --check` fails if the committed workflows are out of date, and `pinfold explain PF301` explains any code (`pinfold explain` lists them all).

Here is the classic mistake, building and publishing in one job:

```yaml
publish:
  runner: ubuntu-24.04
  environment: pypi
  permissions: { contents: read, id-token: write }
  steps:
    - checkout: {}
    - run: pip install build && python -m build
    - uses: pypa/gh-action-pypi-publish@v1.14.3
```

```
pinfold.yml:12:13: error PF301: job "publish" holds credentials (id-token: write, environment pypi), so it can't run a user script
    hint: move the script to an unprivileged job that produces an artifact, and consume it here; or use an effect
```

The fix is two jobs. The build has no credentials; the publish job only receives the artifact, checked against the digest the build recorded:

```yaml
build:
  runner: ubuntu-24.04
  permissions: { contents: read }
  steps:
    - checkout: {}
    - run: pip install build && python -m build
  produces: { dist: dist/ }
publish:
  needs: [build]
  runner: ubuntu-24.04
  environment: pypi
  permissions: { id-token: write }
  consumes: { dist: { from: build, to: dist/ } }
  steps:
    - uses: pypa/gh-action-pypi-publish@v1.14.3
```

More examples: [`docs/design/design-v0.1.md` §5](docs/design/design-v0.1.md#5-source-spec) and the [incident fixtures](cmd/pinfold/testdata/incidents/), each a real CI compromise that Pinfold rejects.

## What is and isn't guaranteed

Pinfold separates what its verifier **guarantees**, what is **best-effort**, and what it **does not protect**. The full table is in the [design document, §2](docs/design/design-v0.1.md#2-claims-github-actions-output), and [`docs/guarantees.md`](docs/guarantees.md) shows which checks and tests enforce each guarantee. In short, hand-written workflows, stolen credentials and a compromised runner are out of scope.

## Status and roadmap

v0.1 is being built in five milestones (front end → first output → guarantees → lock → launch). See [ROADMAP.md](ROADMAP.md).

## Contributing

Contributions are welcome. Good places to start:

- issues labelled [`good first issue`](https://github.com/pinfold-ci/pinfold/labels/good%20first%20issue): registry entries, incident fixtures, explain-page examples;
- questions and ideas in [Discussions](https://github.com/pinfold-ci/pinfold/discussions); design changes start as an RFC there;
- trying to **break a guarantee**. If you manage, report it privately ([SECURITY.md](SECURITY.md)).

Read [CONTRIBUTING.md](CONTRIBUTING.md) first: tests first, one change per PR, and a DCO sign-off (`git commit -s`) on every commit. Where to get help: [SUPPORT.md](SUPPORT.md).

*Pinfold is developed with AI assistance under human direction; security-critical code is independently reviewed.*

## License

[Apache-2.0](LICENSE)
