package spec

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/load"
)

var update = flag.Bool("update", false, "rewrite golden files")

func decodeString(t *testing.T, src string) (*Spec, diag.List) {
	t.Helper()
	root, diags := load.Load("t.yml", []byte(src))
	if diags.HasErrors() {
		t.Fatalf("load failed: %v", diags)
	}
	return Decode("t.yml", root)
}

func TestExampleDecodes(t *testing.T) {
	root, diags := load.ReadFile(filepath.Join("testdata", "example.yml"))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	s, diags := Decode("example.yml", root)
	if len(diags) != 0 {
		var b strings.Builder
		_ = diags.WriteText(&b)
		t.Fatalf("unexpected diagnostics:\n%s", b.String())
	}
	got, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "example.golden.json")
	if *update {
		if err := os.WriteFile(golden, append(got, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got)+"\n" {
		t.Errorf("decoded spec differs from %s; run go test ./internal/spec -update and review the diff", golden)
	}
}

// base is a minimal valid spec; tests splice a job body into it.
const base = `version: 0
workflows:
  w:
    on: { push: { branches: [main] } }
    jobs:
      j:
        runner: ubuntu-24.04
`

func TestDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string // "code@line:col" for every diagnostic, in order
	}{
		{"valid minimal", base + "        steps:\n          - run: make\n", nil},
		{"unknown top-level key", "version: 0\nworkflow: {}\n", []string{"PF111@1:1", "PF110@2:1"}},
		{"missing version", "workflows:\n  w: {on: {manual: {}}, jobs: {j: {runner: x, steps: [{run: a}]}}}\n", []string{"PF111@1:1"}},
		{"unsupported version", "version: 1\nworkflows:\n  w: {on: {manual: {}}, jobs: {j: {runner: x, steps: [{run: a}]}}}\n", []string{"PF115@1:10"}},
		{"version as string", "version: \"0\"\nworkflows:\n  w: {on: {manual: {}}, jobs: {j: {runner: x, steps: [{run: a}]}}}\n", []string{"PF115@1:10"}},
		{"runs-on synonym", base + "        runs-on: x\n        steps: [{run: a}]\n", []string{"PF110@8:9"}},
		{"missing steps", base, []string{"PF111@7:9"}},
		{"empty steps", base + "        steps: []\n", []string{"PF111@8:16"}},
		{"bad workflow name", "version: 0\nworkflows:\n  Release_1: {on: {manual: {}}, jobs: {j: {runner: x, steps: [{run: a}]}}}\n", []string{"PF114@3:3"}},
		{"bad job name", "version: 0\nworkflows:\n  w: {on: {manual: {}}, jobs: {Build: {runner: x, steps: [{run: a}]}}}\n", []string{"PF114@3:32"}},
		{"pull_request unsupported", "version: 0\nworkflows:\n  w: {on: {pull_request: {}}, jobs: {j: {runner: x, steps: [{run: a}]}}}\n", []string{"PF120@3:12"}},
		{"workflow_dispatch", "version: 0\nworkflows:\n  w: {on: {workflow_dispatch: {}}, jobs: {j: {runner: x, steps: [{run: a}]}}}\n", []string{"PF120@3:12"}},
		{"schedule without cron", "version: 0\nworkflows:\n  w: {on: {schedule: {}}, jobs: {j: {runner: x, steps: [{run: a}]}}}\n", []string{"PF111@3:12"}},
		{"two step kinds", base + "        steps:\n          - run: a\n            uses: actions/checkout@v7\n", []string{"PF121@9:13"}},
		{"no step kind", base + "        steps:\n          - name: hi\n", []string{"PF121@9:13"}},
		{"with on run step", base + "        steps:\n          - run: a\n            with: {x: 1}\n", []string{"PF110@10:13"}},
		{"unknown effect", base + "        steps:\n          - effect: frobnicate\n            target: 1\n", []string{"PF122@9:21"}},
		{"field effect doesn't take", base + "        steps:\n          - effect: comment\n            target: { context: issue.number }\n            body: hi\n            max: 3\n", []string{"PF122@12:13"}},
		{"effect missing field", base + "        steps:\n          - effect: comment\n            body: hi\n", []string{"PF111@9:13"}},
		{"bad commit state", base + "        steps:\n          - effect: commit_status\n            sha: { context: commit.sha }\n            state: ok\n            context: ci\n", []string{"PF113@11:20"}},
		// External audit I5: a secret in an effect field would be published (as a tag, say).
		{"secret as a release tag", base + "        steps:\n          - effect: release\n            tag: { secret: PYPI_API_TOKEN }\n", []string{"PF112@10:18"}},
		{"secret as a status sha", base + "        steps:\n          - effect: commit_status\n            sha: { secret: X }\n            state: success\n            context: ci\n", []string{"PF112@10:18"}},
		{"secret as a comment target", base + "        steps:\n          - effect: comment\n            target: { secret: X }\n            body: hi\n", []string{"PF112@10:21"}},
		// External audit V-F: YAML reads True as a boolean; Pinfold read it as false.
		{"capitalised boolean", base + "        steps:\n          - effect: release\n            tag: v1\n            draft: True\n", []string{"PF112@11:20"}},
		{"docker uses", base + "        steps:\n          - uses: docker://alpine:3\n", []string{"PF123@9:19"}},
		{"local uses", base + "        steps:\n          - uses: ./my-action\n", []string{"PF123@9:19"}},
		{"sha uses", base + "        steps:\n          - uses: actions/checkout@0123456789abcdef0123456789abcdef01234567\n", []string{"PF123@9:19"}},
		{"no ref uses", base + "        steps:\n          - uses: actions/checkout\n", []string{"PF123@9:19"}},
		{"path traversal uses", base + "        steps:\n          - uses: actions/checkout/../x@v1\n", []string{"PF123@9:19"}},
		// External audit: branch refs, and downloads outside a workspace subdirectory.
		{"branch uses", base + "        steps:\n          - uses: some/action@main\n", []string{"PF123@9:19"}},
		{"branch uses, other case", base + "        steps:\n          - uses: some/action@Master\n", []string{"PF123@9:19"}},
		{"consumes to dot", base + "        consumes: { dist: { from: j, to: . } }\n        steps: [{run: a}]\n", []string{"PF113@8:42"}},
		{"consumes to parent", base + "        consumes: { dist: { from: j, to: ../x } }\n        steps: [{run: a}]\n", []string{"PF113@8:42"}},
		{"consumes to inner parent", base + "        consumes: { dist: { from: j, to: a/../../x } }\n        steps: [{run: a}]\n", []string{"PF113@8:42"}},
		{"consumes to absolute", base + "        consumes: { dist: { from: j, to: /tmp/x } }\n        steps: [{run: a}]\n", []string{"PF113@8:42"}},
		{"consumes to home", base + "        consumes: { dist: { from: j, to: ~/x } }\n        steps: [{run: a}]\n", []string{"PF113@8:42"}},
		{"consumes to backslash parent", base + "        consumes: { dist: { from: j, to: 'a\\..\\..\\x' } }\n        steps: [{run: a}]\n", []string{"PF113@8:42"}},
		{"consumes then late checkout", base + "        consumes: { dist: { from: j, to: dist } }\n        steps:\n          - run: a\n          - checkout: {}\n", []string{"PF129@11:13"}},
		{"unknown needs", base + "        needs: [bulid]\n        steps: [{run: a}]\n", []string{"PF124@8:17"}},
		{"unknown consumes from", base + "        consumes: { dist: { from: nope, to: d/ } }\n        steps: [{run: a}]\n", []string{"PF124@8:35"}},
		{"unknown condition", base + "        if: { evnt: push }\n        steps: [{run: a}]\n", []string{"PF125@8:15"}},
		{"two keys in condition", base + "        if: { event: push, branch: main }\n        steps: [{run: a}]\n", []string{"PF125@8:13"}},
		{"maintainer not under all", base + "        if: { maintainer: true }\n        steps: [{run: a}]\n", []string{"PF113@8:13"}},
		{"maintainer nested in any", base + "        if: { all: [ { any: [ { maintainer: true } ] } ] }\n        steps: [{run: a}]\n", []string{"PF113@8:31"}},
		{"bad association", base + "        if: { association: [OWNER, ADMIN] }\n        steps: [{run: a}]\n", []string{"PF113@8:36"}},
		{"unknown permission scope", base + "        permissions: { content: read }\n        steps: [{run: a}]\n", []string{"PF130@8:24"}},
		{"bad id-token level", base + "        permissions: { id-token: read }\n        steps: [{run: a}]\n", []string{"PF130@8:34"}},
		{"write-all", base + "        permissions: write-all\n        steps: [{run: a}]\n", []string{"PF130@8:22"}},
		{"expression in run", base + "        steps:\n          - run: echo ${{ github.event.issue.title }}\n", []string{"PF201@9:18"}},
		{"expression in key", base + "        steps:\n          - run: a\n            env: { \"${{ x }}\": y }\n", []string{"PF113@10:20", "PF201@10:20"}},
		{"unknown context", base + "        steps:\n          - run: a\n            env: { T_X: { context: github.token } }\n", []string{"PF113@10:36"}},
		{"value with two kinds", base + "        steps:\n          - run: a\n            env: { T_X: { context: actor, secret: S } }\n", []string{"PF112@10:25"}},
		{"over-long with key", base + "        steps:\n          - uses: a/b@v1\n            with: { " + strings.Repeat("k", 257) + ": v }\n", []string{"PF113@10:21"}},
		{"bad with key", base + "        steps:\n          - uses: a/b@v1\n            with: { \"a b\": v }\n", []string{"PF113@10:21"}},
		{"bool as runner", "version: 0\nworkflows:\n  w: {on: {manual: {}}, jobs: {j: {runner: true, steps: [{run: a}]}}}\n", []string{"PF112@3:44"}},
		{"steps not a list", base + "        steps: {run: a}\n", []string{"PF112@8:16"}},
		{"timeout out of range", base + "        timeout-minutes: 1000\n        steps: [{run: a}]\n", []string{"PF113@8:26"}},
		{"from_comment_args false", base + "        steps:\n          - effect: add_labels\n            target: 1\n            labels: { from_comment_args: false }\n            allowed: [bug]\n", []string{"PF113@11:42"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diags := decodeString(t, tt.src)
			var got []string
			for _, d := range diags {
				got = append(got, d.Code+"@"+strings.TrimPrefix(d.Span.String(), "t.yml:"))
			}
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Errorf("diagnostics = %v, want %v", got, tt.want)
				for _, d := range diags {
					t.Logf("  %s %s: %s (%s)", d.Span, d.Code, d.Message, d.Hint)
				}
			}
		})
	}
}

func TestSuggest(t *testing.T) {
	tests := []struct{ key, want string }{
		{"runs-on", `did you mean "runner"? (Pinfold's name for "runs-on")`},
		{"permssions", `did you mean "permissions"?`},
		{"zzzzzzzz", ""},
	}
	for _, tt := range tests {
		if got := suggest(tt.key, jobKeys); got != tt.want {
			t.Errorf("suggest(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}
