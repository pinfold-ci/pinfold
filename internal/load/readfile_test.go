package load

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	if _, diags := ReadFile(filepath.Join(dir, "missing.yml")); len(diags) != 1 || diags[0].Code != "PF100" {
		t.Errorf("missing file: %v", codesOf(diags))
	}
	big := filepath.Join(dir, "big.yml")
	if err := os.WriteFile(big, []byte("a: "+strings.Repeat("x", MaxSize+10)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, diags := ReadFile(big); len(diags) != 1 || diags[0].Code != "PF105" {
		t.Errorf("big file: %v", codesOf(diags))
	}
	ok := filepath.Join(dir, "ok.yml")
	if err := os.WriteFile(ok, []byte("version: 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if root, diags := ReadFile(ok); len(diags) != 0 || root == nil {
		t.Errorf("valid file: %v", codesOf(diags))
	}
}
