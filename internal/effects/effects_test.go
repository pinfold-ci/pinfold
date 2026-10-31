package effects

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestSanitizerProperties runs the JavaScript property tests with Node. CI
// runners have Node; locally the test is skipped if it isn't installed, unless
// PINFOLD_REQUIRE_NODE is set.
func TestSanitizerProperties(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		if os.Getenv("PINFOLD_REQUIRE_NODE") != "" || os.Getenv("CI") != "" {
			t.Fatal("node is required to test the effect sanitiser")
		}
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, "--test", "js/sanitize.test.js").CombinedOutput() // #nosec G204 -- fixed test command
	if err != nil {
		t.Fatalf("sanitiser property tests failed:\n%s", out)
	}
	if !strings.Contains(string(out), "# fail 0") {
		t.Fatalf("unexpected node test output:\n%s", out)
	}
}

func TestSanitizerIsASCIIAndHasNoExpressions(t *testing.T) {
	for i, r := range Sanitizer {
		if r > 127 {
			t.Fatalf("non-ASCII character %U at byte %d: write it as an escape", r, i)
		}
	}
	if strings.Contains(Sanitizer, "${{") {
		t.Fatal("sanitizer contains ${{, which GitHub would evaluate as an expression")
	}
}
