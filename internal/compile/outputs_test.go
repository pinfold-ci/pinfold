package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/verify"
)

// TestJobOutputs (design §4): a declared output is read from the job's own
// steps (the last one that sets it wins), the reader needs the producer, and
// the verifier accepts exactly that mapping.
func TestJobOutputs(t *testing.T) {
	src := `version: 0
workflows:
  release:
    on: { push: { tags: ["v*"] } }
    jobs:
      version:
        runner: ubuntu-24.04
        outputs: [tag]
        steps:
          - run: echo "tag=v1" >> "$GITHUB_OUTPUT"
          - name: override
            run: echo "tag=v2" >> "$GITHUB_OUTPUT"
      check:
        runner: ubuntu-24.04
        needs: [version]
        steps: [ { run: make check } ]
      publish:
        runner: ubuntu-24.04
        needs: [check]
        permissions: { contents: write }
        steps:
          - effect: release
            tag: { output: version.tag }
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
	for _, want := range []string{
		"      tag: '${{ steps.pinfold_step_2.outputs.tag || steps.pinfold_step_1.outputs.tag }}'\n",
		"      - id: pinfold_step_1\n",
		"      - name: override\n        id: pinfold_step_2\n",
		"T_TAG: '${{ needs.version.outputs.tag }}'",
		"    needs:\n      - check\n      - version\n",
	} {
		if !strings.Contains(f.Content, want) {
			t.Errorf("output lacks %q:\n%s", want, f.Content)
		}
	}
	good := "      tag: '${{ steps.pinfold_step_2.outputs.tag || steps.pinfold_step_1.outputs.tag }}'\n"
	for _, bad := range []string{
		"      tag: '${{ steps.pinfold_step_2.outputs.other }}'\n",
		"      tag: '${{ steps.pinfold_step_2.outputs.tag }}'\n      other: '${{ steps.pinfold_step_1.outputs.other }}'\n",
		"      tag: '${{ github.event.head_commit.message }}'\n",
	} {
		if d := verify.Workflow(f.Path, strings.Replace(f.Content, good, bad, 1), w); len(d) == 0 || d[0].Code != "PF904" {
			t.Errorf("verifier accepted %q: %v", bad, d)
		}
	}
}
