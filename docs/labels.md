# Labels

Issues and pull requests use a small, fixed set of labels.

## Kind (`type/`)

| Label | Meaning |
|---|---|
| `type/bug` | Pinfold does something wrong: wrong output, a crash, a misleading message |
| `type/feature` | New behaviour |
| `type/security` | Hardening, or a fix to a security rule (public only after any embargo ends) |
| `type/docs` | Documentation only |
| `type/test` | Tests and fixtures only |
| `type/chore` | Maintenance: dependencies, tooling, CI |

## Area (`area/`)

Where the change lands: `area/spec` (format, loader, decoder, schema), `area/ir`, `area/graph`, `area/emit`, `area/adapter` (GitHub output), `area/zones`, `area/taint`, `area/permissions`, `area/effects` (effect scripts and sanitiser), `area/verify`, `area/registry`, `area/lock`, `area/cli`, `area/docs`, `area/ci`, `area/repo`.

## Priority

`priority/p0` (blocks a release or breaks a guarantee), `priority/p1` (next up), `priority/p2` (when there's time).

## Workflow

| Label | Meaning |
|---|---|
| `good first issue` | Self-contained, with pointers and acceptance criteria. A good place to start. |
| `help wanted` | Maintainers would welcome a contribution. |
| `needs-design` | Needs an agreed approach (often an RFC) before code. |
| `security` | Touches a guarantee in design §2. Changes get a maintainer's review. |
| `gate` | Needs a decision by the lead maintainer (scope, licensing, accounts, anything public). |
| `blocked` | Waiting on something outside the issue. |
| `question` | Better answered in Discussions; may be moved there. |
| `duplicate`, `invalid`, `wontfix` | Closed without a change; the closing comment says why. |
