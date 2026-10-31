# Roadmap

Dates are targets and are re-estimated at the end of each milestone.

## v0.1: privileged workflows on GitHub Actions

| Milestone | Contents | Exit criterion | Target |
|---|---|---|---|
| **M1** Front end ✅ | Spec format, loader, schema, diagnostics (`file:line:col`, codes, fix hints), `validate`, `schema`, `explain`, `init` | 25 malformed fixtures produce the right diagnostics | done Oct 2026 |
| **M2** First output ✅ | IR, job graph, deterministic emitter, GitHub adapter, verifier skeleton, `ir`, `compile` | The `release` example compiles; actionlint is clean | done Oct 2026 |
| **M3** Guarantees | Trust zones, taint, permissions, effects and sanitiser, maintainer gate, artifact digests, action registry | Incident and bypass fixtures are rejected; live release (TestPyPI) and triage smoke tests pass | Jan 10, 2027 |
| **M4** Lock | `lock`, `--update`, `--verify`, `check`, generated `pinfold-verify` workflows, Renovate and Dependabot presets | All five live attack scenarios are caught by `pinfold-verify` | Feb 7, 2027 |
| **M5** Launch | Docs site, `explain` pages, signed binaries with provenance, setup action, pre-commit hook | Public v0.1 release | Feb 26, 2027 |

## v0.2 themes

- **Handoff:** reporting results of untrusted pull-request builds from forks, safely.
- **Forgejo adapter**, then other platforms.
- Automated lock-update pull requests.
- npm, cargo and container publishing.
