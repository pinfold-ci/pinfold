# Pinfold

> **Status: pre-alpha.** Nothing here works yet. Don't use it in production. See [ROADMAP.md](ROADMAP.md).

**The compiler won't let a CI job both run untrusted code and hold your secrets.**

Pinfold compiles a small, declarative spec (`pinfold.yml`) into GitHub Actions workflows that are secure by construction. Its first target is the riskiest automation an open-source project runs: **releases** (PyPI trusted publishing, GitHub Releases) and **comment and label bots**.

- **Trust zones per job.** A job that holds secrets, OIDC or write permissions can't run your own `run:` steps, and a job that runs your code can't hold credentials. Violations are compile errors, not lint warnings.
- **Tainted by default.** Event data such as titles, branch names and comment text never reaches a `run:`, `script:` or `with:` value. It only arrives through `env:`.
- **Declared, checked permissions.** Each job gets exactly the permissions it declares plus what the compiler generates, and nothing else.
- **Pinned and locked.** Every action reference is a commit SHA recorded in `pinfold.lock`. If a tag moves, the next update or verification fails.
- **No runtime.** The generated workflows are plain, readable YAML and never need the `pinfold` binary to run. If you stop using Pinfold, keep the files.

## What is and isn't guaranteed

Pinfold separates what its verifier **guarantees**, what is **best-effort**, and what it **does not protect**. The full table is in the [design document, §2](docs/design/design-v0.1.md#2-claims-github-actions-output). In short, hand-written workflows, stolen credentials and a compromised runner are out of scope.

## Status and roadmap

v0.1 is being built in five milestones (front end → first output → guarantees → lock → launch). See [ROADMAP.md](ROADMAP.md).

## Contributing

Contributions are welcome once the core settles (milestone M3). Until then, issues and design feedback are the most useful. See [CONTRIBUTING.md](CONTRIBUTING.md). To report a security problem, see [SECURITY.md](SECURITY.md).

*Pinfold is developed with AI assistance under human direction; security-critical code is independently reviewed.*

## License

[Apache-2.0](LICENSE)
