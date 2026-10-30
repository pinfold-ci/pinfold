package graph

import (
	"slices"
	"strings"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/load"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

func check(t *testing.T, jobs string) (map[string][]string, []string) {
	t.Helper()
	src := "version: 0\nworkflows:\n  w:\n    on: { push: {} }\n    jobs:\n" + jobs
	root, diags := load.Load("t.yml", []byte(src))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	s, diags := spec.Decode("t.yml", root)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	orders, diags := Check(s)
	var got []string
	for _, d := range diags {
		got = append(got, d.Code+"@"+strings.TrimPrefix(d.Span.String(), "t.yml:"))
	}
	return orders, got
}

func TestOrder(t *testing.T) {
	orders, diags := check(t, `      c: { runner: x, needs: [b], steps: [{run: a}] }
      a: { runner: x, steps: [{run: a}] }
      b: { runner: x, needs: [a], steps: [{run: a}] }
      d: { runner: x, steps: [{run: a}] }
`)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	if got, want := orders["w"], []string{"a", "b", "c", "d"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestOrderIsStable(t *testing.T) {
	jobs := "      z: { runner: x, steps: [{run: a}] }\n      y: { runner: x, steps: [{run: a}] }\n      x: { runner: x, needs: [z, y], steps: [{run: a}] }\n"
	first, _ := check(t, jobs)
	for range 20 {
		again, _ := check(t, jobs)
		if !slices.Equal(first["w"], again["w"]) {
			t.Fatalf("order changed between runs: %v vs %v", first["w"], again["w"])
		}
	}
	if want := []string{"z", "y", "x"}; !slices.Equal(first["w"], want) {
		t.Errorf("order = %v, want %v (source order for ties)", first["w"], want)
	}
}

func TestDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		jobs string
		want []string
	}{
		{"two-job cycle", "      a: { runner: x, needs: [b], steps: [{run: a}] }\n      b: { runner: x, needs: [a], steps: [{run: a}] }\n", []string{"PF126@7:31"}},
		{"self cycle", "      a: { runner: x, needs: [a], steps: [{run: a}] }\n", []string{"PF126@6:31"}},
		{"consumes without needs", "      a: { runner: x, produces: {d: out/}, steps: [{run: a}] }\n      b: { runner: x, consumes: {d: {from: a, to: in/}}, steps: [{run: a}] }\n", []string{"PF127@7:44"}},
		{"consumes through transitive needs", "      a: { runner: x, produces: {d: out/}, steps: [{run: a}] }\n      m: { runner: x, needs: [a], steps: [{run: a}] }\n      b: { runner: x, needs: [m], consumes: {d: {from: a, to: in/}}, steps: [{run: a}] }\n", nil},
		{"artifact not produced", "      a: { runner: x, produces: {d: out/}, steps: [{run: a}] }\n      b: { runner: x, needs: [a], consumes: {e: {from: a, to: in/}}, steps: [{run: a}] }\n", []string{"PF128@7:46"}},
	}
	for _, tt := range tests {
		_, got := check(t, tt.jobs)
		if strings.Join(got, " ") != strings.Join(tt.want, " ") {
			t.Errorf("%s: diagnostics = %v, want %v", tt.name, got, tt.want)
		}
	}
}
