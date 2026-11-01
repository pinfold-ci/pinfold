package compile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/adapter/github"
)

// Mythos audit finding I1: a branch can commit .github/workflows/pinfold-*.yml
// (or a directory on the way) as a symlink. Running pinfold compile on that
// branch must not write through it.
func TestWriteRefusesSymlinks(t *testing.T) {
	res := &Result{Files: []github.File{{Path: ".github/workflows/pinfold-ci.yml", Content: "generated\n"}}}
	outside := func(t *testing.T) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "victim")
		if err := os.WriteFile(p, []byte("original\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	unchanged := func(t *testing.T, p string) {
		t.Helper()
		if b, _ := os.ReadFile(p); string(b) != "original\n" {
			t.Errorf("file outside the repository was changed: %q", b)
		}
	}

	t.Run("target file is a symlink", func(t *testing.T) {
		repo, victim := t.TempDir(), outside(t)
		if err := os.MkdirAll(filepath.Join(repo, ".github", "workflows"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(victim, filepath.Join(repo, ".github", "workflows", "pinfold-ci.yml")); err != nil {
			t.Fatal(err)
		}
		if err := Write(repo, res); err == nil {
			t.Error("Write followed a symlinked target without an error")
		}
		unchanged(t, victim)
	})

	t.Run("workflows directory is a symlink", func(t *testing.T) {
		repo, elsewhere := t.TempDir(), t.TempDir()
		if err := os.MkdirAll(filepath.Join(repo, ".github"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(elsewhere, filepath.Join(repo, ".github", "workflows")); err != nil {
			t.Fatal(err)
		}
		if err := Write(repo, res); err == nil {
			t.Error("Write followed a symlinked directory without an error")
		}
		if _, err := os.Stat(filepath.Join(elsewhere, "pinfold-ci.yml")); err == nil {
			t.Error("a file was written outside the repository")
		}
	})

	t.Run(".github is a symlink", func(t *testing.T) {
		repo, elsewhere := t.TempDir(), t.TempDir()
		if err := os.Symlink(elsewhere, filepath.Join(repo, ".github")); err != nil {
			t.Fatal(err)
		}
		if err := Write(repo, res); err == nil {
			t.Error("Write followed a symlinked .github without an error")
		}
		if _, err := os.Stat(filepath.Join(elsewhere, "workflows")); err == nil {
			t.Error("a directory was created outside the repository")
		}
	})

	t.Run("regular files are written and replaced", func(t *testing.T) {
		repo := t.TempDir()
		if err := Write(repo, res); err != nil {
			t.Fatal(err)
		}
		res2 := &Result{Files: []github.File{{Path: res.Files[0].Path, Content: "second\n"}}}
		if err := Write(repo, res2); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(repo, ".github", "workflows", "pinfold-ci.yml"))
		if err != nil || string(b) != "second\n" {
			t.Errorf("content = %q, %v; want the second write", b, err)
		}
		left, _ := filepath.Glob(filepath.Join(repo, ".github", "workflows", "*"))
		if len(left) != 1 {
			t.Errorf("files left in the workflows directory: %v", left)
		}
	})
}
