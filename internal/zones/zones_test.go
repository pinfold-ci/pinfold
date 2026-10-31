package zones

import (
	"strings"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/ir"
	"github.com/pinfold-ci/pinfold/internal/load"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

func check(t *testing.T, src string) []string {
	t.Helper()
	root, diags := load.Load("t.yml", []byte(src))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	s, diags := spec.Decode("t.yml", root)
	if diags.HasErrors() {
		t.Fatalf("decode: %v", diags)
	}
	var got []string
	for _, d := range Check(s, ir.Lower(s)) {
		got = append(got, d.Code)
	}
	return got
}

func wf(on, job string) string {
	return "version: 0\nworkflows:\n  w:\n    on: " + on + "\n    jobs:\n      j:\n        runner: ubuntu-24.04\n" + job
}

func TestZones(t *testing.T) {
	push := "{ push: { branches: [main] } }"
	comment := "{ issue_comment: { types: [created] } }"
	tests := []struct {
		name string
		src  string
		want string
	}{
		// PF301: user scripts in privileged jobs.
		{"run under issue_comment", wf(comment, "        steps: [ { run: make } ]\n"), "PF301"},
		{"run with a write scope", wf(push, "        permissions: { contents: write }\n        steps: [ { run: make } ]\n"), "PF301"},
		{"run with a secret", wf(push, "        steps: [ { run: ./deploy, env: { T: { secret: TOKEN } } } ]\n"), "PF301"},
		{"run with an environment", wf(push, "        environment: prod\n        steps: [ { run: make } ]\n"), "PF301"},
		{"run with OIDC", wf(push, "        permissions: { id-token: write }\n        steps: [ { run: make } ]\n"), "PF301"},
		{"run in an unprivileged job", wf(push, "        permissions: { contents: read }\n        steps: [ { run: make } ]\n"), ""},
		{"unsafe_run turns it off", "version: 0\nworkflows:\n  w:\n    on: " + comment + "\n    unsafe_run: true\n    jobs:\n      j:\n        runner: x\n        steps: [ { run: make } ]\n", "PF310"},
		{"unsafe_run keeps checkout hardening", "version: 0\nworkflows:\n  w:\n    on: " + comment + "\n    unsafe_run: true\n    jobs:\n      j:\n        runner: x\n        steps: [ { uses: actions/checkout@v7, with: { ref: refs/pull/1/head } } ]\n", "PF310 PF303"},
		{"unsafe_run keeps the cache rule", "version: 0\nworkflows:\n  w:\n    on: " + comment + "\n    unsafe_run: true\n    jobs:\n      j:\n        runner: x\n        steps: [ { uses: actions/setup-python@v7, with: { cache: pip } } ]\n", "PF310 PF302"},
		// PF304: actions in privileged jobs.
		{"unknown action", wf(comment, "        steps: [ { uses: evil/action@v1 } ]\n"), "PF304"},
		{"unreviewed major", wf(comment, "        steps: [ { uses: actions/checkout@v6 } ]\n"), "PF304"},
		{"github-script is not privileged_ok", wf(comment, "        steps: [ { uses: actions/github-script@v9 } ]\n"), "PF304"},
		{"setup-python is not privileged_ok", wf(comment, "        steps: [ { uses: actions/setup-python@v7 } ]\n"), "PF304"},
		{"unknown action in unprivileged job is fine", wf(push, "        steps: [ { uses: evil/action@v1 } ]\n"), ""},
		{"privileged_ok action is fine", wf(comment, "        steps: [ { uses: actions/upload-artifact@v7, with: { name: x, path: y } } ]\n"), ""},
		{"pinned container is fine", wf(push, "        permissions: { id-token: write }\n        steps: [ { uses: pypa/gh-action-pypi-publish@v1.14.3 } ]\n"), ""},
		// PF303: checkout overrides in a privileged context.
		{"checkout ref override", wf(comment, "        steps: [ { uses: actions/checkout@v7, with: { ref: refs/pull/1/head } } ]\n"), "PF303"},
		{"checkout repository override", wf(comment, "        steps: [ { uses: actions/checkout@v7, with: { repository: attacker/fork } } ]\n"), "PF303"},
		{"checkout unsafe PR opt-in", wf(comment, "        steps: [ { uses: actions/checkout@v7, with: { allow-unsafe-pr-checkout: true } } ]\n"), "PF303"},
		{"checkout persisting credentials", wf(comment, "        steps: [ { uses: actions/checkout@v7, with: { persist-credentials: true } } ]\n"), "PF303"},
		{"checkout ref override in unprivileged job", wf(push, "        steps: [ { uses: actions/checkout@v7, with: { ref: main } } ]\n"), ""},
		// PF302 / PF305: cache restore.
		{"cache in a privileged job", wf(push, "        permissions: { contents: write }\n        steps: [ { uses: actions/setup-python@v7, with: { cache: pip } } ]\n"), "PF302 PF304"},
		{"cache in an unprivileged job", wf(push, "        steps: [ { uses: actions/setup-python@v7, with: { cache: pip } } ]\n"), ""},
		// PF311 / PF313: pull_request_target.
		{"prt with a comment effect", wf("{ pull_request_target: { types: [opened] } }", "        permissions: { pull-requests: write }\n        steps:\n          - effect: comment\n            target: { context: pr.number }\n            body: thanks\n"), "PF311"},
		{"prt with a checkout", wf("{ pull_request_target: { types: [opened] } }", "        steps: [ { checkout: {} } ]\n"), "PF311 PF313"},
		{"prt with a run step", wf("{ pull_request_target: { types: [opened] } }", "        steps: [ { run: make } ]\n"), "PF311 PF301 PF313"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := check(t, tt.src)
			if strings.Join(got, " ") != tt.want {
				t.Errorf("codes = %v, want %q", got, tt.want)
			}
		})
	}
}

func TestFeederCache(t *testing.T) {
	src := `version: 0
workflows:
  release:
    on: { push: { tags: ["v*"] } }
    jobs:
      build:
        runner: x
        steps:
          - uses: actions/setup-python@v7
            with: { cache: pip }
          - run: python -m build
        produces: { dist: dist/ }
      publish:
        runner: x
        needs: [build]
        permissions: { id-token: write }
        consumes: { dist: { from: build, to: dist/ } }
        steps: [ { uses: pypa/gh-action-pypi-publish@v1.14.3 } ]
`
	if got := check(t, src); strings.Join(got, " ") != "PF305" {
		t.Errorf("codes = %v, want [PF305]", got)
	}
}

// TestFeederCacheBypasses holds the round-1 review findings (#32): a feeder may
// not run anything the registry can't show is cache-free.
func TestFeederCacheBypasses(t *testing.T) {
	feeder := func(step string) string {
		return `version: 0
workflows:
  release:
    on: { push: { tags: ["v*"] } }
    jobs:
      build:
        runner: x
        steps:
          - ` + step + `
          - run: python -m build
        produces: { dist: dist/ }
      publish:
        runner: x
        needs: [build]
        permissions: { id-token: write }
        consumes: { dist: { from: build, to: dist/ } }
        steps: [ { uses: pypa/gh-action-pypi-publish@v1.14.3 } ]
`
	}
	tests := []struct{ name, step, want string }{
		{"action not in the registry", "uses: actions/setup-go@v5", "PF305"},
		{"actions/cache in another case", "{ uses: Actions/Cache@v4, with: { path: x, key: k } }", "PF305"},
		{"cache input in another case", "{ uses: actions/setup-python@v7, with: { Cache: pip } }", "PF305"},
		{"unreviewed major with cache", "{ uses: actions/setup-python@v6, with: { cache: pip } }", "PF305"},
		{"registry action in another case", "{ uses: Actions/Setup-Python@v7 }", "PF304"},
		{"reviewed action without cache is fine", "{ uses: actions/setup-python@v7, with: { python-version: '3.13' } }", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(check(t, feeder(tt.step)), " "); got != tt.want {
				t.Errorf("codes = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFeederViaOutput(t *testing.T) {
	src := `version: 0
workflows:
  release:
    on: { push: { tags: ["v*"] } }
    jobs:
      version:
        runner: x
        steps:
          - uses: actions/setup-python@v7
            with: { cache: pip }
          - run: ./compute-version
      publish:
        runner: x
        needs: [version]
        permissions: { contents: write }
        steps:
          - effect: release
            tag: { output: version.tag }
`
	if got := check(t, src); strings.Join(got, " ") != "PF305" {
		t.Errorf("codes = %v, want [PF305]: a job whose output reaches a privileged job is a feeder", got)
	}
}

func TestTransitiveFeeder(t *testing.T) {
	src := `version: 0
workflows:
  release:
    on: { push: { tags: ["v*"] } }
    jobs:
      deps:
        runner: x
        steps:
          - uses: actions/setup-python@v7
            with: { cache: pip }
        produces: { wheels: wheels/ }
      build:
        runner: x
        needs: [deps]
        consumes: { wheels: { from: deps, to: wheels/ } }
        steps: [ { run: make } ]
        produces: { dist: dist/ }
      publish:
        runner: x
        needs: [build]
        permissions: { id-token: write }
        consumes: { dist: { from: build, to: dist/ } }
        steps: [ { uses: pypa/gh-action-pypi-publish@v1.14.3 } ]
`
	if got := check(t, src); strings.Join(got, " ") != "PF305" {
		t.Errorf("codes = %v, want [PF305]: deps feeds build, which feeds publish", got)
	}
}
