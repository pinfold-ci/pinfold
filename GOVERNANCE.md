# Governance

## Roles

- **Lead maintainer:** Muhammad Tauha ([@mtauha](https://github.com/mtauha)). Makes final decisions on scope, design and releases (a "benevolent dictator" model, appropriate while the project is small).
- **Maintainers:** none yet. A regular contributor who has landed several substantial, well-reviewed changes may be invited by the lead maintainer. Maintainers can review and merge PRs outside the security-critical paths listed in `.github/CODEOWNERS`.
- **Contributors:** anyone who opens an issue, a discussion or a PR.

## How decisions are made

- Day-to-day changes: through PR review.
- Significant design changes (taint, trust zones, verifier, lock, the spec language, guarantees): an **RFC** in GitHub Discussions, then a decision by the lead maintainer, recorded as an ADR in [`docs/adr/`](docs/adr/).
- Changes to what Pinfold **guarantees** (design §2) are always RFCs.

## Changes to this document

Changes to governance are made by PR and approved by the lead maintainer.
