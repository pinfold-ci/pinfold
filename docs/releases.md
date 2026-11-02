# Releases and versioning

## Version numbers

Pinfold follows [Semantic Versioning](https://semver.org/). Before 1.0:

- **0.x.0** (minor) releases may contain breaking changes: a spec that compiled may be rejected, or the generated output may change. The [changelog](../CHANGELOG.md) lists every one under **Changed** or **Removed**, with what to do.
- **0.x.y** (patch) releases contain fixes only. A patch release may still reject a spec that an earlier patch accepted if that spec breaks a guarantee in design §2. Security comes first.

After 1.0, breaking changes need a new major version.

## What counts as the interface

- **The spec format.** `version: 0` is the only spec version until 1.0. A future breaking format change gets a new `version:` value, and the compiler explains how to migrate.
- **Diagnostic codes.** A released code (`PFnnn`) keeps its meaning. Codes may be added; a removed code is never reused.
- **Generated output.** Byte-for-byte identical output for the same spec and compiler version. Output can change between versions; `pinfold compile --check` (and, from M4, `pinfold check`) tells you when to recompile.
- **The lock file** (from M4). Its format is versioned like the spec.
- **CLI commands, flags and exit codes.**

Go packages under `internal/` are not an API.

## How releases are made (from v0.1)

- Releases are cut from `main` by the lead maintainer, from a signed tag.
- Binaries are built in CI with SHA-pinned actions, published with checksums and build provenance (milestone M5).
- Every release has a changelog entry. Security fixes are released with a GitHub security advisory.

## Supported versions

Until v0.1: the latest commit on `main`. After v0.1: the latest release. See [SECURITY.md](../SECURITY.md).
