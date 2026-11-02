# Changelog

All notable changes to this project are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Spec format (`version: 0`), loader and decoder with positioned diagnostics, codes and fix hints; `validate`, `schema`, `explain`, `init`.
- IR, job graph, deterministic emitter, GitHub Actions adapter; `ir` and `compile` (with `--check` for drift).
- Guarantees (design §2): trust zones, taint, declared and checked permissions, checkout hardening, cache rules, artifact digest binding, and an independent verifier of the generated output.
- Effects (`comment`, `add_labels`, `remove_labels`, `commit_status`, `release`) with a sanitiser, and the maintainer gate (with custom roles).
- Action registry with reviewed entries; PyPI trusted publishing through a digest-pinned container.
- Declared job outputs.
- Incident fixtures for real CI compromises.
- Project scaffolding: license, governance, security policy, support and release policy, hardened CI, CodeQL.
