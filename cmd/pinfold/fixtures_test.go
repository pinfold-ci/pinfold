package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestMalformedFixtures runs "pinfold validate" on every testdata/malformed/*.yml
// and compares the text output with the .golden file next to it.
func TestMalformedFixtures(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "malformed", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 25 {
		t.Fatalf("found %d malformed fixtures, want at least 25 (M1 exit criterion)", len(files))
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run([]string{"validate", filepath.ToSlash(f)}, &stdout, &stderr); code != 1 {
				t.Errorf("exit code = %d, want 1; stderr: %s", code, stderr.String())
			}
			golden := strings.TrimSuffix(f, ".yml") + ".golden"
			if *update {
				if err := os.WriteFile(golden, stdout.Bytes(), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run go test ./cmd/pinfold -update to create it)", err)
			}
			if stdout.String() != string(want) {
				t.Errorf("output differs from %s\ngot:\n%s\nwant:\n%s", golden, stdout.String(), want)
			}
		})
	}
}

func TestValidFixtures(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "valid", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"validate", f}, &stdout, &stderr); code != 0 || stdout.Len() != 0 {
			t.Errorf("%s: exit %d, output:\n%s%s", f, code, stdout.String(), stderr.String())
		}
	}
}

func TestValidateFlags(t *testing.T) {
	valid := filepath.Join("testdata", "valid", "design-example.yml")
	broken := filepath.Join("testdata", "malformed", "10-runs-on-instead-of-runner.yml")
	tests := []struct {
		args     []string
		wantCode int
		wantOut  string
	}{
		{[]string{"validate", "--format", "json", valid}, 0, "[]\n"},
		{[]string{"validate", "--format", "json", broken}, 1, `"code": "PF110"`},
		{[]string{"validate", "--format", "xml", valid}, 2, ""},
		{[]string{"validate", valid, broken}, 2, ""},
		{[]string{"validate", filepath.Join("testdata", "nope.yml")}, 1, "PF100"},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		code := run(tt.args, &stdout, &stderr)
		if code != tt.wantCode || !strings.Contains(stdout.String(), tt.wantOut) {
			t.Errorf("%v: exit %d, stdout %q; want exit %d containing %q", tt.args, code, stdout.String(), tt.wantCode, tt.wantOut)
		}
	}
}
