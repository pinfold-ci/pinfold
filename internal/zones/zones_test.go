package zones

import (
	"fmt"
	"slices"
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
		{"unsafe_run turns it off", "version: 0\nworkflows:\n  w:\n    on: " + comment + "\n    unsafe_run: true\n    jobs:\n      j:\n        runner: ubuntu-24.04\n        steps: [ { run: make } ]\n", "PF310"},
		{"unsafe_run keeps checkout hardening", "version: 0\nworkflows:\n  w:\n    on: " + comment + "\n    unsafe_run: true\n    jobs:\n      j:\n        runner: ubuntu-24.04\n        steps: [ { uses: actions/checkout@v7, with: { ref: refs/pull/1/head } } ]\n", "PF310 PF303"},
		{"unsafe_run keeps the cache rule", "version: 0\nworkflows:\n  w:\n    on: " + comment + "\n    unsafe_run: true\n    jobs:\n      j:\n        runner: ubuntu-24.04\n        steps: [ { uses: actions/setup-python@v7, with: { cache: pip } } ]\n", "PF310 PF302"},
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
        runner: ubuntu-24.04
        steps:
          - uses: actions/setup-python@v7
            with: { cache: pip }
          - run: python -m build
        produces: { dist: dist/ }
      publish:
        runner: ubuntu-24.04
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
        runner: ubuntu-24.04
        steps:
          - ` + step + `
          - run: python -m build
        produces: { dist: dist/ }
      publish:
        runner: ubuntu-24.04
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
        runner: ubuntu-24.04
        steps:
          - uses: actions/setup-python@v7
            with: { cache: pip }
          - run: ./compute-version
      publish:
        runner: ubuntu-24.04
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
        runner: ubuntu-24.04
        steps:
          - uses: actions/setup-python@v7
            with: { cache: pip }
        produces: { wheels: wheels/ }
      build:
        runner: ubuntu-24.04
        needs: [deps]
        consumes: { wheels: { from: deps, to: wheels/ } }
        steps: [ { run: make } ]
        produces: { dist: dist/ }
      publish:
        runner: ubuntu-24.04
        needs: [build]
        permissions: { id-token: write }
        consumes: { dist: { from: build, to: dist/ } }
        steps: [ { uses: pypa/gh-action-pypi-publish@v1.14.3 } ]
`
	if got := check(t, src); strings.Join(got, " ") != "PF305" {
		t.Errorf("codes = %v, want [PF305]: deps feeds build, which feeds publish", got)
	}
}

// Round-1 review (#32), finding A4: a gate that denies only skips its own job.
func TestNeedsGatedJob(t *testing.T) {
	src := `version: 0
workflows:
  triage:
    on: { issue_comment: { types: [created] } }
    jobs:
      check:
        runner: ubuntu-24.04
        if: { all: [ { maintainer: true } ] }
        permissions: { issues: write }
        steps:
          - { effect: add_labels, target: { context: issue.number }, labels: [triaged], allowed: [triaged] }
      ack:
        runner: ubuntu-24.04
        needs: [check]
        ` + "%s" + `
        permissions: { issues: write }
        steps:
          - { effect: comment, target: { context: issue.number }, body: "Triaged." }
`
	if got := strings.Join(check(t, fmt.Sprintf(src, "")), " "); got != "PF314" {
		t.Errorf("ungated dependant: codes = %q, want PF314", got)
	}
	if got := strings.Join(check(t, fmt.Sprintf(src, "if: { all: [ { maintainer: true } ] }")), " "); got != "" {
		t.Errorf("gated dependant: codes = %q, want none", got)
	}
}

// TestArtifactDownloadsOnlyViaConsumes holds security audit finding F1: a
// cache-restoring job could feed the release through a hand-written
// download-artifact step in a feeder, which the feeder analysis doesn't see.
// Feeder and privileged jobs must download artifacts with consumes: (PF307).
func TestArtifactDownloadsOnlyViaConsumes(t *testing.T) {
	audit := `version: 0
workflows:
  release:
    on: { push: { tags: ["v*"] } }
    jobs:
      deps:
        runner: ubuntu-24.04
        steps:
          - uses: actions/setup-python@v7
            with: { cache: pip }
          - run: pip download -d wheels -r requirements.txt
          - uses: actions/upload-artifact@v7
            with: { name: wheels, path: wheels/ }
      build:
        needs: [deps]
        runner: ubuntu-24.04
        steps:
          - uses: actions/download-artifact@v8
            with: { name: wheels, path: wheels/ }
          - run: python -m build
        produces: { dist: dist/ }
      publish:
        needs: [build]
        runner: ubuntu-24.04
        permissions: { id-token: write }
        consumes: { dist: { from: build, to: dist/ } }
        steps: [ { uses: pypa/gh-action-pypi-publish@v1.14.3 } ]
`
	if got := strings.Join(check(t, audit), " "); got != "PF307" {
		t.Errorf("audit F1 spec: codes = %q, want PF307", got)
	}
	// Other spellings of the same action.
	for _, uses := range []string{"Actions/Download-Artifact@v8", "actions/download-artifact@v4"} {
		src := strings.Replace(audit, "actions/download-artifact@v8", uses, 1)
		if got := check(t, src); !slices.Contains(got, "PF307") {
			t.Errorf("%s: codes = %v, want PF307 among them", uses, got)
		}
	}
	// A privileged job downloading by hand.
	priv := strings.Replace(audit, "        steps: [ { uses: pypa/gh-action-pypi-publish@v1.14.3 } ]\n",
		"        steps:\n          - uses: actions/download-artifact@v8\n            with: { name: wheels, path: wheels/ }\n          - uses: pypa/gh-action-pypi-publish@v1.14.3\n", 1)
	if got := check(t, priv); !slices.Contains(got, "PF307") {
		t.Errorf("privileged job: codes = %v, want PF307 among them", got)
	}
	// An unprivileged job that feeds nothing may download by hand.
	plain := `version: 0
workflows:
  ci:
    on: { push: { branches: [main] } }
    jobs:
      a:
        runner: ubuntu-24.04
        steps:
          - run: make
          - uses: actions/upload-artifact@v7
            with: { name: out, path: out/ }
      b:
        needs: [a]
        runner: ubuntu-24.04
        steps:
          - uses: actions/download-artifact@v8
            with: { name: out, path: out/ }
`
	if got := check(t, plain); len(got) != 0 {
		t.Errorf("unprivileged download: codes = %v, want none", got)
	}
}

// TestRunnersAreFresh holds security audit finding D1: trust zones assume a
// fresh machine per job. On a persistent runner, code from a feeder (or from
// any other workflow) can wait on the machine for the privileged job (PF315).
func TestRunnersAreFresh(t *testing.T) {
	spec := func(build, publish, extra string) string {
		return `version: 0
workflows:
  release:
    on: { push: { tags: ["v*"] } }
    jobs:
      lint:
        runner: self-hosted
        steps: [ { run: make lint } ]
      build:
        runner: ` + build + `
        steps: [ { run: python -m build } ]
        produces: { dist: dist/ }
      publish:
        runner: ` + publish + `
` + extra + `        needs: [build]
        permissions: { id-token: write }
        consumes: { dist: { from: build, to: dist/ } }
        steps: [ { uses: pypa/gh-action-pypi-publish@v1.14.3 } ]
`
	}
	tests := []struct{ name, src, want string }{
		{"hosted runners (an unprivileged job may self-host)", spec("ubuntu-24.04", "ubuntu-latest", ""), ""},
		{"audit D1: self-hosted feeder and publish", spec("self-hosted", "self-hosted", ""), "PF315 PF315"},
		{"self-hosted publish only", spec("ubuntu-24.04", "my-runner", ""), "PF315"},
		{"a look-alike label", spec("ubuntu-24.04", "Ubuntu-Latest", ""), "PF315"},
		{"declared ephemeral", spec("ubuntu-24.04", "linux-large", "        ephemeral_runner: true\n"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(check(t, tt.src), " "); got != tt.want {
				t.Errorf("codes = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestReviewTriggersAreMetadataOnly holds Mythos audit finding D1: on
// pull_request_review and pull_request_review_comment, GITHUB_SHA is the PR's
// merge commit, so a privileged checkout gets the PR author's files. Like
// pull_request_target, these workflows may only label and comment (PF313).
func TestReviewTriggersAreMetadataOnly(t *testing.T) {
	wf := func(on, steps string) string {
		return `version: 0
workflows:
  review:
    on: ` + on + `
    jobs:
      j:
        runner: ubuntu-24.04
        if: { all: [ { maintainer: true } ] }
        permissions: { contents: write, pull-requests: write }
        steps:
` + steps
	}
	publish := "          - checkout: {}\n          - effect: release\n            tag: v9.9.9\n            files: [\"dist/*\"]\n"
	label := "          - effect: add_labels\n            target: { context: pr.number }\n            labels: [reviewed]\n            allowed: [reviewed]\n"
	tests := []struct{ name, src, want string }{
		{"Mythos D1: review-triggered publish", wf("{ pull_request_review: { types: [submitted] } }", publish), "PF313 PF313"},
		{"review comment trigger with a checkout", wf("{ pull_request_review_comment: { types: [created] } }", "          - checkout: {}\n"), "PF313"},
		{"label bot on a review trigger is fine", wf("{ pull_request_review: { types: [submitted] } }", label), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(check(t, tt.src), " "); got != tt.want {
				t.Errorf("codes = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestUngatedDependantIsAnError holds Mythos audit finding D3: PF314 was a
// warning, so the workflow was written and a dependant of a gated job ran its
// privileged effects (here a release) for anyone who fired the trigger.
func TestUngatedDependantIsAnError(t *testing.T) {
	src := `version: 0
workflows:
  bot:
    on: { issue_comment: { types: [created] } }
    jobs:
      approve:
        runner: ubuntu-24.04
        if: { all: [ { comment: { starts_with: "/release" } }, { maintainer: true } ] }
        permissions: { issues: write }
        steps:
          - { effect: comment, target: { context: issue.number }, body: ok }
      publish:
        runner: ubuntu-24.04
        needs: [approve]
        permissions: { contents: write }
        steps:
          - { effect: release, tag: v9.9.9, generate_notes: true }
`
	root, _ := load.Load("t.yml", []byte(src))
	s, _ := spec.Decode("t.yml", root)
	diags := Check(s, ir.Lower(s))
	if len(diags) != 1 || diags[0].Code != "PF314" || !diags.HasErrors() {
		t.Errorf("diagnostics = %v, want one PF314 error", diags)
	}
}

// TestGatedCommentJobsNeedCreated (#64): a gated job on a comment trigger must
// declare types: [created]. Without types the job also runs when a comment is
// edited or deleted, by whoever did that (PF316). The gate script checks this
// again at run time.
func TestGatedCommentJobsNeedCreated(t *testing.T) {
	wf := func(on string) string {
		return `version: 0
workflows:
  triage:
    on: ` + on + `
    jobs:
      label:
        runner: ubuntu-24.04
        if: { all: [ { comment: { starts_with: "/label " } }, { maintainer: true } ] }
        permissions: { issues: write }
        steps:
          - { effect: add_labels, target: { context: issue.number }, labels: { from_comment_args: true }, allowed: [bug] }
`
	}
	tests := []struct{ on, want string }{
		{"{ issue_comment: { types: [created] } }", ""},
		{"{ issue_comment: {} }", "PF316"},
		{"{ issue_comment: { types: [created, edited] } }", "PF316"},
		{"{ issue_comment: { types: [deleted] } }", "PF316"},
		{"{ discussion_comment: {} }", "PF316"},
	}
	for _, tt := range tests {
		t.Run(tt.on, func(t *testing.T) {
			if got := strings.Join(check(t, wf(tt.on)), " "); got != tt.want {
				t.Errorf("codes = %q, want %q", got, tt.want)
			}
		})
	}
}
