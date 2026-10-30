package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pinfold.yml")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"init", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}

	golden := filepath.Join("testdata", "init.golden")
	got := strings.ReplaceAll(stdout.String(), path, "PATH")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("init output differs from %s:\n%s", golden, got)
	}

	// The starter spec must validate cleanly.
	stdout.Reset()
	if code := run([]string{"validate", path}, &stdout, &stderr); code != 0 || stdout.Len() != 0 {
		t.Errorf("starter spec doesn't validate (exit %d):\n%s", code, stdout.String())
	}

	// A second init must not overwrite it.
	if err := os.WriteFile(path, []byte("mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := run([]string{"init", path}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "already exists") {
		t.Errorf("second init: exit %d, stderr %q", code, stderr.String())
	}
	if b, _ := os.ReadFile(path); string(b) != "mine\n" {
		t.Errorf("existing file was overwritten: %q", b)
	}

	if code := run([]string{"init", "a", "b"}, &stdout, &stderr); code != 2 {
		t.Errorf("two arguments: exit %d, want 2", code)
	}
}
