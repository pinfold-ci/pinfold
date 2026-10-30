# 0001. `pinfold-verify` runs as a `workflow_run` consumer, not on `pull_request_target`

- **Status:** accepted
- **Date:** 2026-10-30

## Context

`pinfold-verify` checks pull requests for lock tampering, edited generated files and security-relevant spec changes. It must run the *base branch's* copy of its workflow, so a pull request can't edit the check away. The obvious trigger for that, `pull_request_target`, is blocked by a GitHub default policy in public repositories from 2026-12-02 unless the repository opts back in ([GitHub docs: Securely using `pull_request_target`](https://docs.github.com/en/actions/reference/security/securely-using-pull_request_target)).

## Decision

Generate two files:

1. `pinfold-verify-pr.yml`: `on: pull_request` and `pull_request_review`, `permissions: {}`, a no-op job. A pull request runs its own copy, so this file is untrusted and nothing depends on its contents.
2. `pinfold-verify.yml`: `on: workflow_run` (`types: [requested]`) for the trigger above, plus `push` and `schedule`. GitHub always runs the default branch's copy. It reads `head_sha` from the event payload, fetches `refs/pull/<N>/head`, aborts unless the fetched commit equals `head_sha`, reads the files with `git show` as data (no checkout, nothing executed), and posts a `pinfold/verify` commit status on the head commit.

The required check is the `pinfold/verify` status, with GitHub Actions as the expected source.

## Consequences

- Works in public repositories without an event-policy opt-in.
- A pull request that deletes or renames the trigger never gets the status, so it can't merge (fails closed).
- Two files instead of one; both are in `owned_files`.
- zizmor reports `dangerous-triggers` for the `workflow_run` consumer. It's on the expected-findings list with this ADR as the justification.
- To be confirmed by live tests before M4: `workflow_run` fires for fork runs (including runs awaiting approval), and a `GITHUB_TOKEN` commit status on a fork head SHA satisfies the required check.
