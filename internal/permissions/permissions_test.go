package permissions

import (
	"strings"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/ir"
	"github.com/pinfold-ci/pinfold/internal/load"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

func run(t *testing.T, job string) string {
	t.Helper()
	src := "version: 0\nworkflows:\n  w:\n    on: { push: { branches: [main] } }\n    jobs:\n      j:\n        runner: x\n" + job
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
	return strings.Join(got, " ")
}

func TestPermissions(t *testing.T) {
	comment := "          - effect: comment\n            target: { context: issue.number }\n            body: hi\n"
	tests := []struct{ name, job, want string }{
		{"checkout with contents read", "        permissions: { contents: read }\n        steps: [ { checkout: {} } ]\n", ""},
		{"checkout without permission", "        steps: [ { checkout: {} } ]\n", "PF401"},
		{"checkout under contents write", "        permissions: { contents: write }\n        steps: [ { checkout: {} } ]\n", "PF402"},
		{"comment with issues write", "        permissions: { issues: write }\n        steps:\n" + comment, ""},
		{"comment with pull-requests write", "        permissions: { pull-requests: write }\n        steps:\n" + comment, ""},
		{"comment with issues read", "        permissions: { issues: read }\n        steps:\n" + comment, "PF401"},
		{"comment with nothing", "        steps:\n" + comment, "PF401"},
		{"release needs contents write", "        permissions: { contents: read }\n        steps:\n          - effect: release\n            tag: { context: ref.name }\n", "PF401"},
		{"commit status", "        permissions: { statuses: write }\n        steps:\n          - effect: commit_status\n            sha: { context: commit.sha }\n            state: success\n            context: ci\n", ""},
		{"registry action requirement", "        permissions: { id-token: write }\n        steps: [ { uses: pypa/gh-action-pypi-publish@v1.14.3 } ]\n", ""},
		{"registry action missing OIDC", "        steps: [ { uses: pypa/gh-action-pypi-publish@v1.14.3 } ]\n", "PF401"},
		{"unused OIDC", "        permissions: { id-token: write }\n        steps: [ { run: make } ]\n", "PF402"},
		{"unused write", "        permissions: { packages: write }\n        steps: [ { run: make } ]\n", "PF402"},
		{"artifacts need nothing", "        steps: [ { run: make } ]\n        produces: { dist: dist/ }\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := run(t, tt.job); got != tt.want {
				t.Errorf("codes = %q, want %q", got, tt.want)
			}
		})
	}
}
