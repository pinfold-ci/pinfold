package ir

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/load"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

var update = flag.Bool("update", false, "rewrite golden files")

func lowerFile(t *testing.T, path string) *Program {
	t.Helper()
	root, diags := load.ReadFile(path)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	s, diags := spec.Decode("example.yml", root)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	return Lower(s)
}

func lowerString(t *testing.T, src string) *Program {
	t.Helper()
	root, diags := load.Load("t.yml", []byte(src))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	s, diags := spec.Decode("t.yml", root)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	return Lower(s)
}

func TestExampleGolden(t *testing.T) {
	p := lowerFile(t, filepath.Join("..", "spec", "testdata", "example.yml"))
	got, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	golden := filepath.Join("testdata", "example.ir.json")
	if *update {
		if err := os.WriteFile(golden, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("IR differs from %s; run go test ./internal/ir -update and review the diff", golden)
	}
}

func TestEveryNodeHasSpanAndUniqueID(t *testing.T) {
	p := lowerFile(t, filepath.Join("..", "spec", "testdata", "example.yml"))
	seen := map[string]bool{}
	check := func(id string, s diag.Span) {
		if id == "" || seen[id] {
			t.Errorf("missing or duplicate ID %q", id)
		}
		seen[id] = true
		if s.Line == 0 || s.Col == 0 {
			t.Errorf("%s has no position", id)
		}
	}
	for _, w := range p.Workflows {
		check(w.ID, w.Span)
		for _, j := range w.Jobs {
			check(j.ID, j.Span)
			for _, s := range j.Steps {
				check(s.ID, s.Span)
			}
		}
	}
}

func TestZones(t *testing.T) {
	p := lowerFile(t, filepath.Join("..", "spec", "testdata", "example.yml"))
	want := map[string]Zone{
		"release/build":          ZoneFeeder,
		"release/publish":        ZonePrivileged,
		"release/github-release": ZonePrivileged,
		"triage/label":           ZonePrivileged,
	}
	for _, w := range p.Workflows {
		for _, j := range w.Jobs {
			if j.Zone != want[j.ID] {
				t.Errorf("%s zone = %s, want %s", j.ID, j.Zone, want[j.ID])
			}
		}
	}

	const head = "version: 0\nworkflows:\n  w:\n    on: { push: {} }\n    jobs:\n      j:\n        runner: x\n"
	tests := []struct {
		name string
		job  string
		want Zone
	}{
		{"read only", "        permissions: { contents: read }\n        steps: [{run: a}]\n", ZoneUnprivileged},
		{"write scope", "        permissions: { contents: write }\n        steps: [{run: a}]\n", ZonePrivileged},
		{"oidc", "        permissions: { id-token: write }\n        steps: [{run: a}]\n", ZonePrivileged},
		{"environment", "        environment: prod\n        steps: [{run: a}]\n", ZonePrivileged},
		{"secret in env", "        steps: [{run: a, env: {T: {secret: TOKEN}}}]\n", ZonePrivileged},
		{"secret in with", "        steps: [{uses: a/b@v1, with: {t: {secret: TOKEN}}}]\n", ZonePrivileged},
		{"scope set to none", "        permissions: { contents: none }\n        steps: [{run: a}]\n", ZoneUnprivileged},
	}
	for _, tt := range tests {
		j := lowerString(t, head+tt.job).Workflows[0].Jobs[0]
		if j.Zone != tt.want {
			t.Errorf("%s: zone = %s, want %s", tt.name, j.Zone, tt.want)
		}
	}
	// Any non-trusted trigger makes every job privileged, even with no permissions.
	j := lowerString(t, "version: 0\nworkflows:\n  w:\n    on: { push: {}, issues: {} }\n    jobs:\n      j: { runner: x, steps: [{run: a}] }\n").Workflows[0].Jobs[0]
	if j.Zone != ZonePrivileged {
		t.Errorf("mixed push+issues trigger: zone = %s, want privileged", j.Zone)
	}
}

func TestScopeTableMatchesSpec(t *testing.T) {
	var a, b []string
	for k := range ScopeResources {
		a = append(a, k)
	}
	for k := range spec.PermissionLevels {
		b = append(b, k)
	}
	slices.Sort(a)
	slices.Sort(b)
	if !slices.Equal(a, b) {
		t.Errorf("ScopeResources has %v, spec.PermissionLevels has %v", a, b)
	}
}
