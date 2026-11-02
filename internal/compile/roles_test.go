package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/verify"
)

// TestCustomRoles (#56): custom roles reach the gate as PINFOLD_ROLES, and the
// verifier accepts exactly that list and nothing wider.
func TestCustomRoles(t *testing.T) {
	src := `version: 0
workflows:
  triage:
    on: { issue_comment: { types: [created] } }
    jobs:
      label:
        runner: ubuntu-24.04
        if: { all: [ { comment: { starts_with: "/label " } }, { maintainer: { custom_roles: [release-manager] } } ] }
        permissions: { issues: write }
        steps:
          - { effect: add_labels, target: { context: issue.number }, labels: { from_comment_args: true }, allowed: [bug] }
`
	p := filepath.Join(t.TempDir(), "pinfold.yml")
	if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	res, diags := Run(p)
	if res == nil {
		t.Fatalf("compile failed: %v", diags)
	}
	f, w := res.Files[0], res.Program.Workflows[0]
	want := `PINFOLD_ROLES: '["admin","maintain","write","release-manager"]'`
	if !strings.Contains(f.Content, want) {
		t.Fatalf("gate env lacks %s:\n%s", want, f.Content)
	}
	for _, bad := range []string{
		`PINFOLD_ROLES: '["admin","maintain","write","release-manager","read"]'`,
		`PINFOLD_ROLES: '["admin","maintain","write"]'`,
	} {
		d := verify.Workflow(f.Path, strings.Replace(f.Content, want, bad, 1), w)
		if len(d) == 0 || d[0].Code != "PF907" {
			t.Errorf("verifier accepted %s: %v", bad, d)
		}
	}
	// Without custom roles, the gate gets no PINFOLD_ROLES and the verifier refuses one.
	plain := strings.Replace(src, "{ maintainer: { custom_roles: [release-manager] } }", "{ maintainer: true }", 1)
	if err := os.WriteFile(p, []byte(plain), 0o600); err != nil {
		t.Fatal(err)
	}
	res, diags = Run(p)
	if res == nil {
		t.Fatalf("compile failed: %v", diags)
	}
	f, w = res.Files[0], res.Program.Workflows[0]
	if strings.Contains(f.Content, "PINFOLD_ROLES") {
		t.Error("PINFOLD_ROLES emitted without custom roles")
	}
	injected := strings.Replace(f.Content, "          T_ACTOR:", "          PINFOLD_ROLES: '[\"read\"]'\n          T_ACTOR:", 1)
	if injected == f.Content {
		t.Fatal("anchor not found")
	}
	if d := verify.Workflow(f.Path, injected, w); len(d) == 0 || d[0].Code != "PF907" {
		t.Errorf("verifier accepted an injected PINFOLD_ROLES: %v", d)
	}
}
