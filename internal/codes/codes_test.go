package codes

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/diag"
)

func TestEveryCodeHasAPageAndViceVersa(t *testing.T) {
	files, err := fs.Glob(pages, "explain/*.md")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range files {
		code := strings.TrimSuffix(filepath.Base(f), ".md")
		seen[code] = true
		if _, ok := catalogue[code]; !ok {
			t.Errorf("page %s has no catalogue entry", f)
		}
	}
	for code := range catalogue {
		if !seen[code] {
			t.Errorf("code %s has no page explain/%s.md", code, code)
		}
	}
}

func TestPagesStartWithCodeAndTitle(t *testing.T) {
	for _, code := range All() {
		page, err := Explain(code)
		if err != nil {
			t.Fatal(err)
		}
		want := "# " + code + ": " + catalogue[code].Title + "\n"
		if !strings.HasPrefix(page, want) {
			t.Errorf("%s page starts with %q, want %q", code, strings.SplitN(page, "\n", 2)[0], strings.TrimSpace(want))
		}
		if !strings.Contains(page, "## How to fix it") {
			t.Errorf("%s page has no \"How to fix it\" section", code)
		}
	}
}

func TestExplain(t *testing.T) {
	if _, err := Explain("pf110"); err != nil {
		t.Errorf("lower-case code: %v", err)
	}
	if _, err := Explain("PF999"); err == nil {
		t.Error("unknown code: no error")
	}
}

func TestNewUsesCatalogueSeverity(t *testing.T) {
	d := New("PF110", diag.Span{File: "f"}, "m", "h")
	if d.Severity != diag.Error || d.Code != "PF110" || d.Message != "m" || d.Hint != "h" {
		t.Errorf("New = %+v", d)
	}
	defer func() {
		if recover() == nil {
			t.Error("New with an unknown code didn't panic")
		}
	}()
	New("PF999", diag.Span{}, "", "")
}

// TestEveryEmittedCodeIsCatalogued scans the module's Go sources for "PFnnn"
// string literals, so a code can't be emitted without a catalogue entry.
func TestEveryEmittedCodeIsCatalogued(t *testing.T) {
	re := regexp.MustCompile(`"(PF\d{3})"`)
	found := 0
	root := os.DirFS(filepath.Join("..", ".."))
	err := fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) && path != "." {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			found++
			if _, ok := catalogue[m[1]]; !ok {
				t.Errorf("%s uses %s, which isn't in the catalogue", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found < len(catalogue) {
		t.Errorf("scan found %d code literals, fewer than the %d catalogue entries: is the walk reaching the sources?", found, len(catalogue))
	}
}
