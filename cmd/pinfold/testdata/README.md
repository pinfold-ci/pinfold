# CLI fixtures

- `valid/`: specs that must validate with no output (exit 0).
- `malformed/`: specs that must fail validation (exit 1). Each `NN-name.yml` has a `NN-name.golden` file with the exact `pinfold validate` text output. Regenerate with `make golden` and review the diff.

| Fixture | Codes | What it covers |
|---|---|---|
| 01, 02 | PF101 | YAML syntax error; tab indentation |
| 03, 04, 05 | PF102 | Anchors and aliases; merge keys; explicit tags |
| 06 | PF103 | Duplicate key |
| 07, 08 | PF104 | Two documents; empty file |
| 09 | PF105 | Not UTF-8 |
| 10, 11 | PF110 | `runs-on` instead of `runner`; misspelled `permissions` (a typo must not silently drop permissions) |
| 12 | PF111 | Missing required key |
| 13 | PF112 | Wrong type |
| 14 | PF113 | Invalid enum value |
| 15 | PF114 | Invalid job name |
| 16 | PF115 | Unsupported version |
| 17 | PF120 | `pull_request` (untrusted builds are v0.2) |
| 18 | PF121 | Step with two kinds |
| 19 | PF122 | Unknown effect |
| 20, 21 | PF123 | Bare SHA in `uses`; `docker://` action |
| 22 | PF124 | `needs` names an unknown job |
| 23 | PF125 | Unknown condition |
| 24, 25 | PF130 | `write-all`; `id-token: read` |
| 26 | PF201 | Incident shape: `issue_comment` body pasted into `run:` |
| 27 | PF201 | Incident shape: `/label x'; curl …` via a raw expression |
| 28 | PF113 | `maintainer` gate nested under `any:` |
| 29 | PF113 | Unknown context (`github.token`) |
| 30 | several | Many errors in one file, reported in position order |
| 31 | PF126 | `needs` cycle |
| 32 | PF127 | Artifact consumed without depending on its producer |
| 33 | PF128 | Artifact the producer doesn't produce |
