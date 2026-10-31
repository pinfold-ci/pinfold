# zizmor: expected findings on generated workflows

Pinfold runs [zizmor](https://docs.zizmor.sh/) on its own generated workflows in CI, with the default persona, and fails on any finding not listed here. Each entry says why it's expected and what Pinfold does instead. The machine-readable list is `internal/adapter/github/testdata/zizmor.yml`.

| Audit | Where | Why it's expected | Pinfold's mitigation |
|---|---|---|---|
| `dangerous-triggers` | Workflows on privileged triggers, such as `issue_comment` (`pinfold-triage.yml`) | zizmor flags `issue_comment` because hand-written workflows on it are usually injectable. Comment bots need the trigger. | Every job under a privileged trigger is a privileged job: no user `run:` steps, no tainted `${{ }}` in `run:`/`with:`/`script:`, effects only (design §2, §4). The verifier re-checks the output (PF904). |
| `unpinned-uses` | User-written `uses:` steps (`pinfold-release.yml`) | **Temporary, until milestone M4.** User actions keep their tag and a `# pinfold:unlocked` comment until `pinfold lock` pins them. Actions Pinfold generates are already pinned by SHA. | From M4, every `uses:` is a commit SHA recorded in `pinfold.lock` (design §2, §7); this entry is then removed. |

## Pedantic persona

The pedantic persona also reports `concurrency-limits` (no workflow-level `concurrency:`). Pinfold doesn't add one by default, because cancelling or queueing runs changes behaviour (for example, a cancelled release). Specs set it per job with `concurrency:`.

## Expected when they arrive (M4)

- `dangerous-triggers` on the generated `pinfold-verify.yml` (`workflow_run`): it never executes pull-request content and reads the PR's files only as data (ADR 0001).
