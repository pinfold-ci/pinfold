package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUnprivilegedConsumerHasNoActionsRead (security audit): a consumer that
// holds no credentials and feeds nothing privileged downloads by name and gets
// no actions: read; a consumer that feeds a privileged job keeps the bind step.
func TestUnprivilegedConsumerHasNoActionsRead(t *testing.T) {
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
        consumes: { dist: { from: build, to: dist } }
        steps: [ { run: make test } ]
      package:
        runner: ubuntu-24.04
        needs: [build]
        consumes: { dist: { from: build, to: dist } }
        steps: [ { run: make package } ]
        produces: { pkg: pkg/ }
      release:
        runner: ubuntu-24.04
        needs: [package]
        permissions: { contents: write }
        consumes: { pkg: { from: package, to: pkg } }
        steps: [ { effect: release, tag: v1.0.0, files: [pkg/*] } ]
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
	job := func(name string) string {
		_, rest, _ := strings.Cut(out, "\n  "+name+":\n")
		if i := strings.Index(rest, "\n  ["); i >= 0 {
			rest = rest[:i]
		}
		end := len(rest)
		for _, other := range []string{"\n  build:\n", "\n  test:\n", "\n  package:\n", "\n  release:\n"} {
			if i := strings.Index(rest, other); i >= 0 && i < end {
				end = i
			}
		}
		return rest[:end]
	}
	test := job("test")
	if strings.Contains(test, "actions: read") || strings.Contains(test, "check artifact") || !strings.Contains(test, "name: dist") {
		t.Errorf("unprivileged consumer: want a download by name without actions: read or a bind step:\n%s", test)
	}
	for _, name := range []string{"package", "release"} {
		j := job(name)
		if !strings.Contains(j, "actions: read") || !strings.Contains(j, "check artifact") {
			t.Errorf("%s (feeder or privileged): want actions: read and a bind step:\n%s", name, j)
		}
	}
}
