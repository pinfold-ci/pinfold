# Security policy

Pinfold makes security promises, so we take reports seriously.

## Reporting a vulnerability

**Please don't open a public issue.** Report privately through GitHub's [private vulnerability reporting](https://github.com/pinfold-ci/pinfold/security/advisories/new) ("Report a vulnerability" on the Security tab).

Useful reports include:
- the Pinfold version (`pinfold version`) and the input spec;
- the generated workflow and why it breaks a claim in [design §2](docs/design/design-v0.1.md#2-claims-github-actions-output);
- a proof of concept, if you have one.

## What counts as a vulnerability

- Generated output that violates a **✅ Guaranteed** claim (for example, a tainted value reaching a `run:` step, or a privileged job running user code) without `unsafe_run`.
- A way for a pull request to pass `pinfold-verify` while changing the lock, the spec or a generated file without the documented maintainer approval.
- Bugs in the lock resolver that accept a moved tag, a redirect, or a reference that doesn't match `pinfold.lock`.

Items listed as **best-effort** or **not protected** in design §2 are documented limits. Reports that show a limit is weaker than documented are still welcome.

## What to expect

We aim to acknowledge reports within **5 working days** and to agree on a disclosure timeline with you. Pinfold is pre-alpha and maintained by a small team, so please allow reasonable time for fixes. Reporters are credited in the advisory unless they prefer otherwise.

## Supported versions

Until v0.1, only the latest commit on `main` is supported.
