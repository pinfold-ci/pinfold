# IR acceptance fixtures

Design §8: before Pinfold gets more adapters, each target platform records **which IR constructs it can't map** and **which design §2 guarantees weaken there**. This becomes the per-platform guarantee table in the v0.2 design.

- One file per platform: `forgejo.yml`, `gitlab.yml`, `jenkins.yml`.
- `constructs`: every trigger, job key, step kind, effect and the maintainer gate the IR has, each with `maps` and the question to answer.
- `guarantees`: every ✅ row of design §2, each with `holds` and a note.
- **Everything is `unverified` today.** Values become `yes`, `no` or `partial` only when checked against the platform's official documentation (and a live test where docs aren't enough), with the source linked.

`internal/spec/acceptance_test.go` fails if a file is missing a construct or guarantee, so adding a trigger, job key or effect forces every platform file to be updated. When an adapter exists, its tests will also fail on any construct marked `no` that a spec uses.
