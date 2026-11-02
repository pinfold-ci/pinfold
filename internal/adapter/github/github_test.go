package github

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/pinfold-ci/pinfold/internal/ir"
	"github.com/pinfold-ci/pinfold/internal/load"
	"github.com/pinfold-ci/pinfold/internal/registry"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

var update = flag.Bool("update", false, "rewrite golden files")

func compileExample(t *testing.T) []File {
	t.Helper()
	root, diags := load.ReadFile(filepath.Join("..", "..", "spec", "testdata", "example.yml"))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	s, diags := spec.Decode("pinfold.yml", root)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	return Compile(ir.Lower(s), "pinfold.yml", "0000000000000000000000000000000000000000000000000000000000000000")
}

func TestGolden(t *testing.T) {
	files := compileExample(t)
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	for _, f := range files {
		golden := filepath.Join("testdata", filepath.Base(f.Path))
		if *update {
			if err := os.WriteFile(golden, []byte(f.Content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if f.Content != string(want) {
			t.Errorf("%s differs from %s; run go test ./internal/adapter/github -update and review the diff", f.Path, golden)
		}
	}
}

func TestDeterministic(t *testing.T) {
	first := compileExample(t)
	for range 10 {
		again := compileExample(t)
		for i := range first {
			if first[i] != again[i] {
				t.Fatalf("%s changed between compiles", first[i].Path)
			}
		}
	}
}

func TestOutputShape(t *testing.T) {
	for _, f := range compileExample(t) {
		if strings.Contains(f.Content, "\r") {
			t.Errorf("%s contains CR", f.Path)
		}
		var w struct {
			On          map[string]any `yaml:"on"`
			Permissions map[string]any `yaml:"permissions"`
			Jobs        map[string]struct {
				Permissions map[string]string `yaml:"permissions"`
				Steps       []map[string]any  `yaml:"steps"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal([]byte(f.Content), &w); err != nil {
			t.Fatalf("%s: %v", f.Path, err)
		}
		if len(w.On) == 0 {
			t.Errorf("%s: no triggers (was 'on' misread?)", f.Path)
		}
		if w.Permissions == nil || len(w.Permissions) != 0 {
			t.Errorf("%s: workflow permissions = %v, want {}", f.Path, w.Permissions)
		}
		for name, j := range w.Jobs {
			for _, s := range j.Steps {
				uses, _ := s["uses"].(string)
				if strings.HasPrefix(uses, "actions/checkout@") {
					with, _ := s["with"].(map[string]any)
					if with["persist-credentials"] != false {
						t.Errorf("%s job %s: checkout without persist-credentials: false", f.Path, name)
					}
				}
			}
		}
	}
}

func TestReleasePermissions(t *testing.T) {
	var w struct {
		Jobs map[string]struct {
			Needs       []string          `yaml:"needs"`
			Permissions map[string]string `yaml:"permissions"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(compileExample(t)[0].Content), &w); err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string]string{
		"build":          {"contents": "read"},
		"publish":        {"id-token": "write", "actions": "read"}, // actions: read is generated for artifact binding
		"github-release": {"contents": "write", "actions": "read"},
	}
	for job, perms := range want {
		got := w.Jobs[job].Permissions
		if len(got) != len(perms) {
			t.Errorf("%s permissions = %v, want %v", job, got, perms)
		}
		for k, v := range perms {
			if got[k] != v {
				t.Errorf("%s permissions = %v, want %v", job, got, perms)
			}
		}
	}
	// github-release consumes from build without needing it directly: the adapter adds it.
	if n := w.Jobs["github-release"].Needs; strings.Join(n, ",") != "publish,build" {
		t.Errorf("github-release needs = %v, want [publish build]", n)
	}
}

// TestContainerInputsAllSet: the PyPI image's script runs with set -u, so every
// input it reads must be set, empty or not, as GitHub does for a real action.
// The v0.0.1 live smoke run failed with "INPUT_PASSWORD: unbound variable".
func TestContainerInputsAllSet(t *testing.T) {
	var w struct {
		Jobs map[string]struct {
			Steps []struct {
				Uses string            `yaml:"uses"`
				Env  map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(compileExample(t)[0].Content), &w); err != nil {
		t.Fatal(err)
	}
	all, err := registry.All()
	if err != nil {
		t.Fatal(err)
	}
	e := all["pypa/gh-action-pypi-publish"]
	if e == nil || e.Container == nil {
		t.Fatal("registry entry for pypa/gh-action-pypi-publish has no container")
	}
	found := false
	for _, st := range w.Jobs["publish"].Steps {
		if !strings.HasPrefix(st.Uses, "docker://") {
			continue
		}
		found = true
		for name, ci := range e.Container.Inputs {
			v, set := st.Env[ci.Env]
			if !set {
				t.Errorf("input %s: %s is not set", name, ci.Env)
			} else if v != ci.Default {
				t.Errorf("input %s: %s = %q, want the default %q", name, ci.Env, v, ci.Default)
			}
		}
		if len(st.Env) != len(e.Container.Inputs) {
			t.Errorf("container env has %d entries, want %d", len(st.Env), len(e.Container.Inputs))
		}
	}
	if !found {
		t.Fatal("no container step in the publish job")
	}
}

func TestConditionQuoting(t *testing.T) {
	c := &ir.Condition{Kind: "comment", Values: []string{"/label x' || true || '"}}
	got := condition(c)
	want := "startsWith(github.event.comment.body, '/label x'' || true || ''')"
	if got != want {
		t.Errorf("condition = %s, want %s", got, want)
	}
}

// TestBranchConditionGuard: a tag named like the branch must not match, and a
// pull request matches on its base branch (external audit).
func TestBranchConditionGuard(t *testing.T) {
	got := condition(&ir.Condition{Kind: "branch", Values: []string{"production"}})
	want := "github.base_ref == 'production' || (github.base_ref == '' && startsWith(github.ref, 'refs/heads/') && github.ref_name == 'production')"
	if got != want {
		t.Errorf("condition = %s, want %s", got, want)
	}
}

// TestActionlint runs actionlint on the goldens when ACTIONLINT names a binary
// (CI also lints them directly).
func TestActionlint(t *testing.T) {
	bin := os.Getenv("ACTIONLINT")
	if bin == "" {
		t.Skip("set ACTIONLINT to an actionlint binary to run")
	}
	files, _ := filepath.Glob(filepath.Join("testdata", "pinfold-*.yml"))
	out, err := exec.Command(bin, files...).CombinedOutput() // #nosec G204 -- test-only, binary named by the developer
	if err != nil {
		t.Errorf("actionlint: %v\n%s", err, out)
	}
}
