package load

import (
	"fmt"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/pinfold-ci/pinfold/internal/diag"
)

// codesOf returns "code@line:col" for each diagnostic.
func codesOf(l diag.List) []string {
	var out []string
	for _, d := range l {
		pos := strings.TrimPrefix(strings.TrimPrefix(d.Span.String(), "t.yml"), ":")
		out = append(out, strings.TrimSuffix(d.Code+"@"+pos, "@"))
	}
	return out
}

func TestRejected(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // first diagnostic, "code@line:col"
	}{
		{"syntax error", "a: b\n  c: d\n", "PF101@2"},
		{"tab indentation", "a:\n\tb: c\n", "PF101@2"},
		{"anchor", "a: &x 1\nb: 2\n", "PF102@1:4"},
		{"alias to a missing anchor", "a: 1\nb: *x\n", "PF102"},
		{"anchor and alias", "a: &x 1\nb: *x\n", "PF102@1:4"},
		{"merge key", "base: {a: 1}\nother:\n  <<: {a: 2}\n", "PF102@3:3"},
		{"explicit tag", "a: !!str 1\n", "PF102@1:4"},
		{"custom tag", "a: !env HOME\n", "PF102@1:4"},
		{"duplicate key", "a: 1\nb: 2\na: 3\n", "PF103@3:1"},
		{"nested duplicate key", "w:\n  x: 1\n  x: 2\n", "PF103@3:3"},
		{"empty file", "", "PF104"},
		{"only comments", "# nothing\n", "PF104"},
		{"two documents", "a: 1\n---\nb: 2\n", "PF104@2:1"},
		{"not utf-8", "a: \xff\n", "PF105"},
		{"too large", "a: " + strings.Repeat("x", MaxSize) + "\n", "PF105"},
		{"top level is a list", "- a\n- b\n", "PF112@1:1"},
		{"top level is a scalar", "hello\n", "PF112@1:1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, diags := Load("t.yml", []byte(tt.src))
			got := codesOf(diags)
			if len(got) == 0 || got[0] != tt.want {
				t.Errorf("diagnostics = %v, want first %q", got, tt.want)
			}
			if !diags.HasErrors() {
				t.Error("no error reported")
			}
		})
	}
}

func TestAccepted(t *testing.T) {
	src := "version: 0\non:\n  push: {}\nlist: [a, \"b\", 'c']\n"
	root, diags := Load("t.yml", []byte(src))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", codesOf(diags))
	}
	if root.Kind != yaml.MappingNode || len(root.Content) != 6 {
		t.Fatalf("root = kind %v with %d children, want a mapping with 3 pairs", root.Kind, len(root.Content))
	}
	// "on" must stay a string key (YAML 1.1 would read it as true).
	if k := root.Content[2]; k.Value != "on" || k.Tag != "!!str" {
		t.Errorf("key = %q %s, want string \"on\"", k.Value, k.Tag)
	}
	if k := root.Content[2]; k.Line != 2 || k.Column != 1 {
		t.Errorf("position of on = %d:%d, want 2:1", k.Line, k.Column)
	}
}

func TestLineEndingsAndBOMDontChangeTheTree(t *testing.T) {
	lf := "a: 1\nb:\n  c: two\n"
	variants := map[string]string{
		"crlf":     strings.ReplaceAll(lf, "\n", "\r\n"),
		"bom":      "\ufeff" + lf,
		"bom+crlf": "\ufeff" + strings.ReplaceAll(lf, "\n", "\r\n"),
	}
	want, diags := Load("t.yml", []byte(lf))
	if len(diags) != 0 {
		t.Fatal(codesOf(diags))
	}
	wantDump := dump(want)
	for name, src := range variants {
		got, diags := Load("t.yml", []byte(src))
		if len(diags) != 0 {
			t.Errorf("%s: %v", name, codesOf(diags))
			continue
		}
		if d := dump(got); d != wantDump {
			t.Errorf("%s: tree\n%s\nwant\n%s", name, d, wantDump)
		}
	}
}

func dump(n *yaml.Node) string {
	var b strings.Builder
	var walk func(*yaml.Node, int)
	walk = func(n *yaml.Node, depth int) {
		b.WriteString(strings.Repeat("  ", depth))
		fmt.Fprintf(&b, "%s %q @%d:%d\n", n.Tag, n.Value, n.Line, n.Column)
		for _, c := range n.Content {
			walk(c, depth+1)
		}
	}
	walk(n, 0)
	return b.String()
}
