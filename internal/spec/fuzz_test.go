package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/load"
)

// FuzzLoadDecode checks that any input either loads and decodes or produces
// diagnostics, never panics, and that every position lies inside the input.
func FuzzLoadDecode(f *testing.F) {
	seeds, _ := filepath.Glob(filepath.Join("..", "..", "cmd", "pinfold", "testdata", "*", "*.yml"))
	for _, s := range seeds {
		if b, err := os.ReadFile(s); err == nil {
			f.Add(b)
		}
	}
	f.Add([]byte(base + "        steps: [{run: a}]\n"))
	f.Fuzz(func(t *testing.T, src []byte) {
		text := strings.ReplaceAll(strings.ReplaceAll(string(src), "\r\n", "\n"), "\r", "\n")
		lines := strings.Count(text, "\n") + 1
		root, diags := load.Load("f.yml", src)
		if root != nil {
			_, more := Decode("f.yml", root)
			diags = append(diags, more...)
		}
		if root == nil && !diags.HasErrors() {
			t.Fatal("load returned no tree and no error")
		}
		for _, d := range diags {
			if d.Code == "" || d.Message == "" {
				t.Fatalf("diagnostic without code or message: %+v", d)
			}
			if d.Span.Line < 0 || d.Span.Line > lines || d.Span.Col < 0 {
				t.Fatalf("position %s outside the input (%d lines)", d.Span, lines)
			}
		}
	})
}
