package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckoutBeforeDownloads: a consuming job's leading checkout is emitted
// before the bind and download steps, because checking out cleans the
// workspace (external audit). The verifier must accept that order.
func TestCheckoutBeforeDownloads(t *testing.T) {
	src := `version: 0
workflows:
  ci:
    on: { push: { branches: [main] } }
    jobs:
      build:
        runner: ubuntu-24.04
        steps: [ { run: make dist } ]
        produces: { dist: dist/ }
      test:
        runner: ubuntu-24.04
        needs: [build]
        permissions: { contents: read }
        consumes: { dist: { from: build, to: dist } }
        steps:
          - checkout: {}
          - run: make test
`
	p := filepath.Join(t.TempDir(), "pinfold.yml")
	if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	res, diags := Run(p)
	if res == nil {
		t.Fatalf("compile failed: %v", diags)
	}
	out := res.Files[0].Content
	_, job, _ := strings.Cut(out, "\n  test:\n")
	order := []string{"pinfold: checkout", "pinfold: check artifact dist", "pinfold: download dist", "make test"}
	last := -1
	for _, s := range order {
		i := strings.Index(job, s)
		if i <= last {
			t.Fatalf("%q is out of order (want %v) in:\n%s", s, order, job)
		}
		last = i
	}
}
