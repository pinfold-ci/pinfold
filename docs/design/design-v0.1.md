# Pinfold v0.1: System Design (Draft 4, final for v0.1)

> **Status:** Draft 4.1 · 2026-10-30 · **frozen for v0.1 planning.** The design went through three rounds of adversarial review before this draft. Changes are proposed as RFCs in GitHub Discussions and recorded in [`docs/adr/`](../adr/).
> **Name:** **Pinfold**: CLI `pinfold`, GitHub org `pinfold-ci`
> **One-liner:** *The compiler won't let a CI job both run untrusted code and hold your secrets.*
> **v0.1 scope:** **GitHub Actions only.** Compiles **releases** (PyPI trusted publishing and GitHub Releases) and **label and comment bots**, plus the lock and its verification. Hand-written CI is left alone. **Handoff (PR result reports from forks) and Forgejo are the v0.2 headline.**

---

## 0. Principles

1. **Guarantees need a limited input language:** no templating, no embedded code, no raw `${{ }}`.
2. **The job is the security boundary.** Steps in a job share its token, its secrets, its files and the runner's memory, so trust is decided per job.
3. **Privileged jobs are declarative.** A job that holds credentials runs only **effects** (compiler-generated, input-sanitising operations) and pinned actions that the registry marks as running no repo-controlled code.
4. **Zero runtime.** Generated workflows never need the `pinfold` binary to run. Effects are readable YAML (a pinned `actions/github-script` with a literal, compiler-generated script). The only exception is `pinfold-verify.yml`, and if the project dies, deleting that one file leaves everything else working.
5. **Claim only what the core verifies.** Everything else is labelled best-effort or not protected.
6. **Adopt one workflow at a time.** `pinfold` owns the files listed in its lock and nothing else.

---

## 1. Positioning
| Tool | What it does | How `pinfold` differs |
|---|---|---|
| zizmor, actionlint, poutine | *Find* problems in hand-written workflows (zizmor can auto-fix some) | `pinfold` makes the dangerous ones impossible to write, then runs these linters on its own output |
| pinact, ratchet, StepSecurity, GitHub's SHA-pinning policy | Pin action references | `pinfold` pins as well, but pinning is basic hygiene everyone now expects, not the pitch |
| **gh-aw "safe outputs"** | The same pattern: a read-only job requests writes, and a separate privileged job checks and applies them | gh-aw is **built around an AI agent** (the no-AI "custom" engine is now only an unsupported sample), is GitHub-only, is in preview, and doesn't cover releases or OIDC. **`pinfold` is deterministic, has no runtime, covers releases, and its adapter design is meant to extend to other platforms** |

**Accepted risk:** GitHub could add a deterministic mode to gh-aw. Hedges: zero lock-in, release and OIDC coverage, trust-zone compile errors, and multi-platform support from v0.2. **Credited to gh-aw in the docs:** `max` limits on effects, comment sanitisation, label allowlists.

---

## 2. Claims (GitHub Actions output)

| Level | Claim | Enforced by |
|---|---|---|
| ✅ **Guaranteed** | No job holding secrets, OIDC or write permissions contains a user `run:` step, or an action not marked `privileged_ok` (meaning: *runs no repo-controlled code, and has no unpinned nested references*), unless `unsafe_run` is set, which is stamped in the header and turns these claims off | Zones + verifier |
| ✅ | **No tainted `${{ }}` in any `run:`, `script:` or `with:` value, with no exceptions.** Tainted values appear only in `env:` (named `T_*`) and in compiler-generated `if:` | Taint + verifier (checks every key) |
| ✅ | Contexts are **tainted by default**, including `ref.name` | Taint table |
| ✅ | Generated checkouts never override `repository`/`ref` in privileged workflows; `persist-credentials: false` is always emitted | Lowering + verifier |
| ✅ | No cache restore in any job that holds credentials **or whose artifacts or outputs feed one** | Zones + registry + verifier |
| ✅ | Artifacts consumed by a privileged job are checked against the **SHA-256 digest** recorded by the job that produced them | Generated steps |
| ✅ | Workflow `permissions: {}`; each job's permissions **equal** the declared set plus the generated set | Verifier |
| ✅ | Every `uses:` is a peeled commit SHA that matches `pinfold.lock`; moved exact tags fail `lock --update`/`--verify` | Lock + verifier |
| ⚠️ **Best-effort** | **`pinfold-verify.yml`** catches lock tampering, drift and security-relevant spec changes in PRs, but **only if** it's a *required* status check and `CODEOWNERS` covers `pinfold.yml`, `pinfold.lock` and `.github/workflows/`. Both are repo settings, which `pinfold init` prints as setup instructions · registry review is **per major version**, not per commit · lints on scripts in *unprivileged* jobs · build integrity beyond the digest binding (a compromised dependency inside your own build; a build script that fetches artifacts or caches by itself, e.g. `gh run download`, which the compiler can't see: feeder jobs may only receive artifacts through `consumes:`, PF307) | Documented prerequisites |
| ❌ **Not protected** | Stolen credentials pushing hand-written workflows · hand-written workflows (including new ones a PR adds) · malicious code already in an action when it's first locked · a compromised runner or platform | Documented |

**Assumption behind every zone claim: a fresh machine per job.** Feeder and privileged jobs must run on a standard GitHub-hosted runner, or on a runner the spec declares `ephemeral_runner: true` (a GitHub larger runner, or an ephemeral or just-in-time self-hosted runner). That assertion is stamped in the generated file's header, and anything else is PF315 (verifier PF911). On a persistent self-hosted runner, code from one job can act inside the next. A self-hosted runner registered under a GitHub-hosted label is an administrator's choice, covered by "a compromised runner or platform".

---

## 3. Requirements → milestone → test

| # | Requirement | M | Test |
|---|---|---|---|
| F1 | Parse and validate; diagnostics with `file:line:col` + code + fix hint; `validate`, `schema`, `explain` | M1 | 25 malformed fixtures |
| F2 | `init`: starter spec + **setup instructions** (required check, CODEOWNERS, Dependabot/Renovate presets) | M1 | golden output |
| F3 | Multiple workflows per spec → `.github/workflows/pinfold-<name>.yml` | M2 | golden |
| F4 | IR with `Span` + `ID` on every node; `pinfold ir`; `setup_instructions[]` in the adapter response | M2 | golden IR JSON |
| F5 | Deterministic emitter (LF, logical key order) + header | M2 | compile twice and compare bytes; CRLF input |
| F6 | Trust zones (`PF3xx`), including the "feeds a privileged job" cache rule | M3 | incident fixtures |
| F7 | Taint (`PF2xx`), tainted by default | M3 | one fixture per context class + bypass fixtures (`/label x'; curl…`, ref names with `;$\|`) |
| F8 | Declared + checked permissions (`PF4xx`) | M3 | fixtures |
| F9 | Checkout hardening (no overrides in privileged workflows, `persist-credentials: false`) | M3 | verifier fixtures |
| F10 | **Effects** lowered to github-script: `comment` (sticky), `add_labels`, `remove_labels`, `commit_status`, `release` | M3 | golden + **property tests for the sanitiser** + live smoke |
| F11 | `maintainer` gate step | M3 | unit (role values, non-200, triggering actor) + live smoke |
| F12 | Artifacts within trusted workflows with digest binding | M3 | golden + tamper fixture |
| F13 | Verifier (all §2 ✅ rows) | M2–M3 | one fixture per rule (introduce a deliberately broken adapter bug → `PF9xx`) |
| F14 | Lock: `lock`, `--update`, `--verify`, peeling, resolution rules, moved-tag detection, refusing redirects, optional `repo_id` (`PF5xx`) | M4 | local git server fixtures: annotated tags, moved exact tag, moving major tag, branch/tag clash, redirect |
| F15 | `check` (offline; exit 2 drift / 4 compiler too old; unused lock entries) | M4 | fixtures; a newer compiler against an older lock |
| F16 | Generated `pinfold-verify-pr.yml` + `pinfold-verify.yml` (`workflow_run` consumer, commit status) | M4 | golden + live smoke from a fork: tampered lock, edited generated file, deleted lock, PR adding `unsafe_run`, compiler upgrade PR, **PR deleting the trigger** |
| F17 | Renovate + Dependabot presets (ignore `pinfold-*.yml`); `PF520` fallback | M4 | config fixtures; V2 checked |
| F18 | Registry (about 8 actions) with `privileged_ok`, `secret_inputs`, `restores_cache`, `fetches_refs`, required scopes | M3 | schema test; every entry has a reviewed-source link |
| F19 | Distribution: binaries with checksums, `pinfold-ci/setup` action pinned by SHA (used **only** by `pinfold-verify.yml` and developers), pre-commit hook | M5 | release dry run |

**Out of scope for v0.1:** handoff (PR reports from forks) · Forgejo / GitLab / Jenkins adapters · automated lock-update PRs (need an App token) · npm, cargo and container publishing · comment commands that run PR code · general CI · matrix · reusable workflows · external adapters · MCP · `import`.

---

## 4. Trust model

### Triggers
| Class | Triggers | Rules |
|---|---|---|
| **Untrusted** | `pull_request` | Not compiled in v0.1 (handoff is v0.2) |
| **Trusted** | `push` (branches/tags), `schedule`, `manual` | User `run:` allowed in unprivileged jobs |
| **Privileged** | everything else (default-deny): `issue_comment`, `issues`, `pull_request_target`, `pull_request_review*`, `discussion*`, `workflow_run` | Every job is privileged |

`pull_request_target` is allowed only for **metadata jobs**, meaning an explicit allowlist: the effects `add_labels`, `remove_labels` and `comment`, and conditions on PR metadata. No checkout, and no registry action marked `fetches_refs`. Every `pull_request_target` workflow also gets **`PF311` (warning)**: GitHub blocks this trigger by default in public repositories from 2026-12-02 unless an Actions event policy allows it, so the job may never run. User-written `workflow_run` workflows are not compiled in v0.1 (`PF312`); the generated `pinfold-verify.yml` is the one documented `workflow_run` consumer, because the compiler writes it.

### Job zones
- **Privileged job:** declares any write scope, `id-token`, secrets or an `environment`, *or* runs under a privileged trigger. It may contain effects, `privileged_ok` actions and generated steps. Errors:
  - `PF301`: user `run:`.
  - `PF302`: cache restore.
  - `PF303`: checkout override.
  - `PF304`: an action that isn't `privileged_ok`.
- **Feeder job:** an unprivileged job whose artifacts or outputs a privileged job consumes. It may run user code, but **can't restore caches** (`PF305`). Its artifacts are bound by digest.
- **Unprivileged job:** trusted triggers only. `contents: read` at most.
- **Outputs** are written with the generated `pinfold_output NAME VALUE` helper (random heredoc delimiter). They are **always tainted** when consumed by a privileged job.

### Taint
- **Untainted, closed list:**
  - `pr.number`, `issue.number`, `run.id`, `run.attempt`;
  - `event.name`, `pr.author_association`;
  - `repo.full_name`, `repo.default_branch`;
  - `commit.sha` under trusted triggers.

  **Everything else is tainted**, including `ref.name`, `actor` and any context added later.
- Tainted values may go only to:
  - `env:` with names matching `^T_[A-Z0-9_]+$` (on `run:` steps in unprivileged jobs, or on generated effect steps);
  - generated `if:` expressions (with `'` escaped).

### Maintainer gate
`maintainer: true` (allowed only at the top level of a job's `if: all:`) becomes a **generated first step**:
- It calls `GET /repos/{repo}/collaborators/{triggering_actor}/permission`.
- It reads `role_name` (not the legacy `permission` field) and **allows only `admin`, `maintain` or `write`**, plus custom role names the spec lists explicitly. Any status other than 200, a bot or app actor, or a mismatch between `actor` and `triggering_actor` counts as a deny.
- It sets `outputs.ok`. Every later step has `if: steps.pinfold_gate.outputs.ok == 'true'`, so a non-maintainer gets a **neutral skip, not a red X**.

The gate is **required** when event data chooses what an effect acts on under a trigger outsiders can cause: labels taken from the comment (PF122), and a release `tag` or commit status `sha` from event data (PF205).

`association: [...]` is still available, documented as weaker.

---

## 5. Source spec

```yaml
version: 0

workflows:

  release:
    on: { push: { tags: ["v*"] } }
    jobs:
      build:                                   # feeder: user code OK, no cache restore
        runner: ubuntu-24.04
        permissions: { contents: read }
        steps:
          - checkout: {}
          - run: python -m pip install build && python -m build
        produces: { dist: dist/ }              # generated upload + sha256 digest output
      publish:                                 # privileged: declarative only
        needs: [build]
        runner: ubuntu-24.04
        environment: pypi
        permissions: { id-token: write }
        consumes: { dist: { from: build, to: dist/ } }   # generated download + digest check
        steps:
          - uses: pypa/gh-action-pypi-publish@v1.14.3    # registry: privileged_ok
      github-release:
        needs: [publish]
        runner: ubuntu-24.04
        permissions: { contents: write }
        consumes: { dist: { from: build, to: dist/ } }
        steps:
          - effect: release
            tag: { context: ref.name }          # tainted → reaches the script via env only
            files: ["dist/*"]
            generate_notes: true

  triage:
    on: { issue_comment: { types: [created] } }
    jobs:
      label:
        runner: ubuntu-24.04
        if:
          all:
            - { comment: { starts_with: "/label " } }
            - { maintainer: true }
        permissions: { issues: write }
        steps:
          - effect: add_labels
            target: { context: issue.number }
            labels: { from_comment_args: true }
            allowed: [bug, docs, enhancement, question]
            max: 3
          - effect: comment
            target: { context: issue.number }
            sticky: triage-ack
            body: "Labelled by @{{ actor }}."   # template over typed values; sanitised; @mention allowlisted to actor
```

### Spec rules
1. **Value kinds:** `literal | context | secret | var | output`. `${{` anywhere is `PF201`.
2. **Conditions:**
   - `event`, `branch` (base), `tag`, `label`, `comment.starts_with`, `maintainer`, `association`, `actor` (`PF322` warning on privileged jobs);
   - `status`: success, failure, always, not_cancelled.
3. **Permissions:** values are checked per scope (`id-token: write|none`; the others `read|write|none`). The scope table is **versioned data**.
4. **`uses` forms:** `owner/repo@tag` and `owner/repo/path@tag`. No bare SHAs, branches, `docker://`, or local `./` actions in privileged jobs.
5. **Escape hatch:** `unsafe_run: true` per workflow, stamped into the header, and flagged as a security-relevant change by `pinfold-verify`. Extension keys come from an allowlist; `timeout-minutes` and `concurrency` are first-class fields.

### Effects (lowering)
Each effect becomes **one pinned `actions/github-script` step**:
- **`script:`** is a **literal** generated by the compiler. It contains the effect's code plus the inlined sanitiser (about 150 lines of JS, version-stamped).
- **Every input** arrives through `env:` (`T_*` for tainted values, `PINFOLD_*` for config) and is read with `process.env`.
- **The step carries a `# pinfold:generated <effect-id>` marker,** which the verifier uses to tell generated steps from user steps.
- **The sanitiser:**
  - strips HTML comments and control characters;
  - neutralises `@mentions` and `#refs` unless allowlisted;
  - caps links (default 10) and bytes (default 16 KB);
  - checks labels against `allowed`;
  - checks tag names against `^[A-Za-z0-9._/-]{1,128}$` before calling the API.
- **Sticky comments** are found by a marker comment **and** author `github-actions[bot]`. Anything else with the marker is ignored. If the comment isn't found it's created; a 404 or 403 on update falls back to creating a new comment.
- **Required scopes:** each effect has a fixed set, added to the generated permission set and listed in diagnostics.

---

## 6. Core passes
```
① parse → ② schema → ③ lower (Span+ID) → ④ graph
→ ⑤ zones (PF3xx) → ⑥ taint (PF2xx) → ⑦ permissions (PF4xx) → ⑧ pins (PF5xx)
→ GitHub adapter → ⑨ verify (PF9xx) → write / compare
```
- **⑦ Permissions:**
  - `PF401` (error): a needed scope isn't declared (effects, registry actions, `checkout` → `contents: read`).
  - `PF402` (warning): a write scope that nothing needs.
  - `PF404` (warning): `ghcr.io` images.
- **The registry is advisory** (it can cause errors but never grants anything). It ships inside the release and is pinned by digest. It starts with about 8 actions: `actions/checkout`, `actions/github-script`, `actions/upload-artifact`, `actions/download-artifact`, `pypa/gh-action-pypi-publish`, `actions/setup-python`, `actions/attest` (replacing `attest-build-provenance`, which is now a wrapper around it), and one docs/pages action. Baseline majors (V3): github-script v9, checkout v7, upload-artifact v7, download-artifact v8, setup-python v7. Only entries marked `privileged_ok` and with no unpinned nested references can be used in privileged jobs.
- **⑨ Verifier:** checks every ✅ row in §2 against the generated YAML. `with:` may contain literals, untainted values, registry `secret_inputs`, and compiler contexts (`github.token` on generated steps, `runner.temp`). **It never contains tainted values.** Any violation is an internal error.

---

## 7. Lock and `pinfold-verify.yml`

```yaml
# pinfold.lock — generated; commit it.
lock_version: 1
compiler_min: "0.1.0"
registry_digest: sha256:…
owned_files:                                  # every file pinfold generates; verify protects exactly these
  - .github/workflows/pinfold-release.yml
  - .github/workflows/pinfold-triage.yml
  - .github/workflows/pinfold-verify.yml
actions:
  "github.com/actions/checkout@v4":
    floating: v4                              # moves by design, never flagged
    resolved_tag: v4.2.2                      # moved-tag detection applies here
    commit: <40-hex, peeled ^{}>
    tag_object: <40-hex | null>
    repo_id: 197814629                        # optional, recorded when a token is available (repojacking check)
```

**Resolution:**
- `@vN` → the highest `vN.x.y` that peels to the same commit. No exact tag → `PF503` warning. A branch and tag with the same name → `PF504`.
- Refuse redirects (`PF505`); with no token, `repo_id` isn't checked (`PF506` warning).
- A moved `resolved_tag` → `PF510` in `--update` and `--verify`. Overriding it needs a human (`--accept-moved <key>`).

**`check` (offline):**
- LF-normalised comparison; unused lock entries are an error.
- Exit 2 = drift. Exit 4 = compiler older than `compiler_min`. A newer compiler passes when its output matches.
- The header hashes the spec plus the action entries, not the compiler version.

**`pinfold-verify.yml` (generated, two files):** it does **not** use `pull_request_target`, which GitHub blocks by default in public repositories from 2026-12-02.
- **`pinfold-verify-pr.yml` (the trigger):** `on: pull_request` (opened, synchronize, reopened) and `pull_request_review` (submitted, dismissed); `permissions: {}`; one job with no checkout and one no-op step. It exists only to start the consumer. Its content is **untrusted** (a PR runs its own copy), so nothing depends on what it does.
- **`pinfold-verify.yml` (the consumer):** `on: workflow_run` (`workflows: [pinfold-verify-pr]`, `types: [requested]`), plus `push` to the default branch and a weekly `schedule`. Not path-filtered. GitHub always runs the **default branch's** copy of a `workflow_run` workflow, so a PR can't change the consumer.
- **The required check is a commit status**, `pinfold/verify`, that the consumer posts on the PR's head SHA (`statuses: write`). A fork's `pull_request` run has a read-only token and can't post statuses. If a PR deletes or renames the trigger, the consumer never runs and the required status never appears, so **the PR can't merge (fails closed)**. `init` tells the user to require `pinfold/verify` with **GitHub Actions as the expected source**.
- **Which PR and which commit:** the consumer takes `head_sha` from the `workflow_run` payload (set by GitHub, not by the PR's workflow), finds the open PR whose head is that SHA (**zero or several matching PRs → failure**, so an approval on one PR can't be borrowed by another with the same commit), posts `pending`, then runs `git fetch --no-tags --depth=1 origin refs/pull/<N>/head` and **aborts if the fetched commit isn't `head_sha`** (the PR moved, and a newer run will report). Files are read with `git show <sha>:<path>` into a data directory. **Nothing from the PR is checked out into the workspace or executed.**
- **Which compiler runs it:** `pinfold-ci/setup` pinned by SHA in the **default branch's** copy of the consumer. It installs the compiler version named in the PR's lock, verified against a checksum list embedded in that setup action, and **only accepts versions ≥ the base branch's**, so a PR can't downgrade it.
- **Permissions:** `contents: read`, `pull-requests: read` (find the PR, read reviews), `statuses: write`. No secrets.
- **Fails if:**
  - a lock entry fails `--verify`;
  - an owned file differs from what the PR's spec and lock compile to;
  - an owned file was deleted, or the lock was removed;
  - **there's a security-relevant spec change:** `unsafe_run` added, a new privileged trigger, a new non-registry action, a widened `allowed` list, or new permission scopes. These pass only once a maintainer has approved the PR (a review from someone with ≥ `maintain`, checked through the API). An approval fires `pull_request_review`, which re-runs the check.
- **Push and schedule runs** verify the default branch itself (a normal checkout is fine there, because the code is trusted) and fail the run on drift.
- **Prerequisites (printed by `init`):** make `pinfold/verify` a required status check (source: GitHub Actions), and add `CODEOWNERS` for the spec, the lock and `.github/workflows/`. Fork PRs from first-time contributors need a maintainer to approve the workflow run before the check appears.
- **If the project dies:** delete both files. Everything else keeps working.
- **zizmor expected findings:** `dangerous-triggers` on the consumer (`workflow_run`), justified because it never executes PR content.

**Bots:**
- **Renovate preset:** `ignorePaths: [".github/workflows/pinfold-*.yml"]`.
- **Dependabot preset:** `exclude-paths` for the same glob, if V2 confirms it works for `github-actions`. Otherwise `check` fails with `PF520`.
- Hand-written workflows keep their normal bot updates.
- Updating `pinfold.lock` is a local `pinfold lock --update` in v0.1. Automated PRs come in v0.2.

---

## 8. IR and adapter boundary (unchanged in principle)
- `Workflows[]`; every node has a `Span` and `ID`. `Job.Zone` and `Job.Permissions` are owned by the core, serialised, and checked by the verifier.
- **Abstract capabilities** in the IR (`repo.contents.read`, `pr.write`, `oidc`, …); the GitHub adapter maps them to scopes.
- **Effects are IR nodes.** Each adapter lowers them to its platform's API: github-script on GitHub, and API calls on Forgejo and GitLab in v0.2.
- **IR acceptance fixtures** for Forgejo, GitLab and Jenkins *fail* when a construct can't be mapped. Each fixture also records **which §2 guarantees weaken on that platform**; this becomes the per-platform guarantee table in the v0.2 design. The fixtures live in [`testdata/ir-acceptance/`](../../testdata/ir-acceptance/).

---

## 9. Facts to verify
| # | Fact | Before |
|---|---|---|
| V1 | The `collaborators/{user}/permission` API (`role_name`) is readable with the CI token and the declared scopes | M3 |
| V2 | Dependabot `exclude-paths` works for the `github-actions` ecosystem (feature reported in the 2025-08-26 changelog) | M4 |
| V3 | Whether `actions/github-script` is acceptable as the effect runtime: the default token input and the Node version on hosted runners | M3 |
| V4 | ~~`pull_request_target` reads the PR's files through the API~~ → superseded: the consumer uses the documented `git fetch origin refs/pull/<N>/head`; live test that it works from a `workflow_run` job with `contents: read` | M4 |
| V5 | PyPI trusted publishing works with the generated `environment` + `id-token` layout | M3 |
| V6 | The status of gh-aw's deterministic/custom engine, checked again for the launch positioning | M5 |
| V7 | Live: `workflow_run` `requested` fires for fork PR runs (including runs awaiting first-time-contributor approval); a commit status posted with `GITHUB_TOKEN` on a fork head SHA satisfies a required check whose expected source is GitHub Actions | M4 |
| V8 | Live: the collaborator-permission API works under `permissions: {}` and for bot actors; re-run actor behaviour (open part of V1) | M3 |
| V9 | Live: Dependabot security updates and `exclude-paths` (open part of V2) | M4 |

---

## 10. Milestones (one developer, evenings and weekends, about 3–4 months)
| Milestone | Contents | Exit criterion |
|---|---|---|
| **M1** Front end (about 2–3 wks) | spec, loader, schema, diagnostics, `validate`/`schema`/`explain`/`init` | 25 malformed fixtures |
| **M2** First output (about 3 wks) | IR, graph, emitter, GitHub adapter, verifier skeleton, `ir`/`compile`, IR acceptance fixtures | `release` compiles; actionlint clean |
| **M3** Guarantees (about 4–5 wks) | zones, taint, permissions, effects + sanitiser, maintainer gate, artifact digests, registry (about 8 entries) | Incident and bypass fixtures fail correctly; the `release` (TestPyPI) and `triage` live smoke tests pass |
| **M4** Lock (about 3 wks) | lock/update/verify, `check`, `pinfold-verify.yml`, bot presets | All five `pinfold-verify` live attack scenarios are caught |
| **M5** Launch (about 2 wks) | docs, `explain` pages, binaries + setup action, pre-commit, launch post | Public release |

**Linters on the output:** actionlint clean. zizmor clean apart from an **expected-findings list with a written justification for each** (e.g. `dangerous-triggers` on `pinfold-verify.yml`).

**v0.2 headline:** **Handoff** (PR result reports from forks, an earlier draft; to be revisited in a v0.2 RFC) + **Forgejo adapter** (after verifying its event support, actions URL, `permissions:` enforcement and the `.github` fallback) + automated lock-update PRs + npm, cargo and container publishing.

---

## 11. Open questions
1. License (Apache-2.0 recommended). 2. Registry governance for `privileged_ok` (who reviews it, how it's audited). 3. Should `pinfold-verify.yml` also be offered as an org-level ruleset-required workflow (v0.2)?

---

## Appendix: changes from Draft 3
- **Scope (Option A):** handoff moved to v0.2; `pinfold-update.yml` dropped; v0.1 publishing limited to PyPI and GitHub Releases; registry cut from about 30 actions to about 8.
- **Effects** are lowered to pinned `actions/github-script` with literal scripts and **env-only inputs**, so there's no `pinfold` runtime and the injection hole is closed. The verifier's rule against tainted `${{ }}` in `run:`, `script:` and `with:` now has no exceptions.
- **`pinfold-verify.yml`:** fetches the spec; protects `owned_files`; flags security-relevant changes for maintainer approval; uses the compiler version from the PR's lock (≥ the base branch's); also runs on push; its prerequisites are listed under best-effort.
- **`privileged_ok`** defined as "runs no repo-controlled code and has no unpinned nested references". Registry review is honestly labelled per major version.
- **Feeder jobs:** no cache restore; artifacts bound by SHA-256 digest.
- **`ref.name` is always tainted.**
- **`maintainer` is a gate step** that fails closed, uses `role_name` and `triggering_actor`, and skips neutrally.
- **Sticky comments** match marker plus bot author; metadata-only `pull_request_target` uses an explicit allowlist.
- The requirement → milestone → test table is complete, including **property tests for the sanitiser**.
- The Dependabot `exclude-paths` preset is subject to V2.

## Appendix: changes in Draft 4.1
- **`pinfold-verify.yml` no longer uses `pull_request_target`** (blocked by default in public repos from 2026-12-02). It is now a read-only `pull_request` trigger file plus a `workflow_run` consumer that posts the required `pinfold/verify` commit status. A PR deleting the trigger fails closed. PR files are fetched with the documented `refs/pull/<N>/head` ref and checked against the payload's `head_sha`.
- **Metadata-only `pull_request_target` jobs stay in scope** (no scope change) but always get the `PF311` warning. User-written `workflow_run` workflows are rejected with `PF312`.
- Verification results folded in: `role_name` allow-list, github-script v9, `actions/attest`, pypi-publish v1.14.3, baseline action majors. V4 superseded; V7–V9 added as live tests.
