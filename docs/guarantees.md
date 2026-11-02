# What enforces each guarantee

Design §2 lists what Pinfold **guarantees** for generated GitHub Actions workflows. Each guarantee is enforced twice: by compile-time checks on the spec, and by the **verifier**, which re-reads the generated YAML independently of the code that wrote it. A verifier finding (`PF9xx`) is always a Pinfold bug. Every row has tests that try to break it.

| # | Guarantee | Compile-time | Verifier (on the output) | Breaking tests |
|---|---|---|---|---|
| 1 | No job holding secrets, OIDC or write permissions runs a user `run:` step or an action that isn't `privileged_ok`, unless `unsafe_run` is set and stamped | PF301, PF304, PF310 | PF909 | `internal/zones`, `internal/verify` (row 1), incident fixtures |
| 2 | No tainted `${{ }}` in `run:`, `script:` or `with:` | PF201, PF202, PF203 | PF904 | `internal/taint`, `internal/verify`, fixtures 26, 27, 36, 37 |
| 3 | Contexts are tainted by default | taint table (`internal/taint`) | PF904 allowlist | `TestTaintedTable` |
| 4 | Checkouts never override `repository`/`ref` in privileged workflows; `persist-credentials: false` | PF303 | PF903 | `internal/zones`, `internal/verify` |
| 5 | No cache restore in a job that holds credentials or feeds one | PF302, PF305, PF307 (artifacts reach feeder and privileged jobs only through `consumes:`) | PF910, PF908 (feeder downloads) | `internal/zones` (feeders via artifacts, outputs, transitive), `internal/verify` (row 5), Ultralytics fixture, `TestArtifactDownloadsOnlyViaConsumes` |
| 6 | Artifacts consumed by a privileged job match the digest their producer recorded | generated bind step (#57); download by the checked artifact id (#63) | PF908 | bind behaviour tests, `TestArtifactBindingCantBeRemoved` |
| 7 | Workflow `permissions: {}`; each job's permissions equal the declared plus generated set | PF401, PF402 | PF901, PF902 | `internal/permissions`, `internal/verify` |
| 8 | Every `uses:` is a commit SHA matching `pinfold.lock` | lock (M4) | lock verifier (M4) | M4 |

All zone rows assume a fresh machine per job: feeder and privileged jobs run on a GitHub-hosted runner or one the spec declares `ephemeral_runner: true`, stamped in the header (PF315, verifier PF911, `TestRunnersAreFresh`).

Also verified on every output: container images are registry-listed and digest-pinned (PF905); github-script steps in privileged jobs run only generated effect scripts (PF906); the maintainer gate can't be removed or bypassed (PF907).

The verifier also refuses anything the adapter never writes, so a bug can't slip a new way to run code past the checks above (PF904): unknown keys at workflow, job or step level (for example job `container:`/`services:`, workflow `env:`/`defaults:`, `continue-on-error:`), a second YAML document, a step `env:` that isn't a mapping, env names other than `T_*`/`PINFOLD_*` on generated scripts or `INPUT_*` on containers, tainted values under any env name except `T_*`, and credentials (`secrets.*`, `github.token`) in jobs that aren't privileged. Artifact downloads must fetch exactly what the bind step checked (PF908), and the gate's env is fixed (PF907).

Best-effort items and what isn't protected are listed in design §2.
