# Incident fixtures

Each file reproduces the **workflow shape** behind a real CI/CD compromise, written as a Pinfold spec, and must be rejected. The `.golden` files hold the exact `pinfold validate` output.

| Fixture | Incident | Rejected by |
|---|---|---|
| `ultralytics-cache-poisoning` | Ultralytics (Dec 2024): a poisoned Actions cache reached the PyPI release build | PF305 |
| `pwn-request` | The "pwn request" pattern: `pull_request_target` + checkout of the fork's head + build | PF303, PF301, PF313 (+ PF311 warning) |
| `nx-pr-title-injection` | Nx "s1ngularity" (Aug 2025): PR title injected into a script under `pull_request_target` | PF301, PF313 (+ PF311, PF402 warnings) |
| `tj-actions-unreviewed-action` | tj-actions/changed-files (Mar 2025): moved tags pointed at a secret-dumping commit | PF304 (+ PF402 warning; moved tags: lock, M4) |
| `review-triggered-publish` | Mythos audit D1 (Nov 2026): on `pull_request_review` the checkout is the PR's merge commit, so a privileged release would ship the PR author's files | PF313 |
| `comment-triggered-build` | Comment-triggered bots that build with write access | PF301 (+ PF402 warning) |

The `issue_comment` and `/label x'; curl …` injection shapes are in `../malformed/` (26, 27): they use raw `${{ }}`, which is rejected before trust zones run (PF201).
