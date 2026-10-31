// Package compile runs the whole pipeline: load, decode, graph checks,
// lowering, the GitHub adapter and the verifier. Nothing is returned for
// writing unless every stage passes.
package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pinfold-ci/pinfold/internal/adapter/github"
	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/graph"
	"github.com/pinfold-ci/pinfold/internal/ir"
	"github.com/pinfold-ci/pinfold/internal/load"
	"github.com/pinfold-ci/pinfold/internal/spec"
	"github.com/pinfold-ci/pinfold/internal/verify"
	"github.com/pinfold-ci/pinfold/internal/zones"
)

// Result is a successful compilation.
type Result struct {
	Program *ir.Program
	Files   []github.File // paths relative to the spec's directory
}

// Front loads, decodes and graph-checks the spec at path, then lowers it. It
// also returns the normalised spec bytes it parsed, for the header hash.
func Front(path string) (*ir.Program, []byte, diag.List) {
	src, diags := load.Read(path)
	if diags.HasErrors() {
		return nil, nil, diags
	}
	root, diags := load.Load(path, src)
	if diags.HasErrors() {
		return nil, nil, diags
	}
	s, diags := spec.Decode(path, root)
	if diags.HasErrors() {
		return nil, nil, diags
	}
	_, more := graph.Check(s)
	diags = append(diags, more...)
	if diags.HasErrors() {
		diags.Sort()
		return nil, nil, diags
	}
	p := ir.Lower(s)
	diags = append(diags, zones.Check(s, p)...)
	diags.Sort()
	if diags.HasErrors() {
		return nil, nil, diags
	}
	return p, load.Normalize(src), diags
}

// Run compiles the spec at path. Warnings may come back with a Result.
func Run(path string) (*Result, diag.List) {
	p, src, diags := Front(path)
	if p == nil {
		return nil, diags
	}
	sum := sha256.Sum256(src)
	files := github.Compile(p, filepath.Base(path), hex.EncodeToString(sum[:]))
	for i, f := range files {
		diags = append(diags, verify.Workflow(f.Path, f.Content, p.Workflows[i])...)
	}
	diags.Sort()
	if diags.HasErrors() {
		return nil, diags
	}
	return &Result{Program: p, Files: files}, diags
}

// Drift compares the result with the files on disk under dir and returns the
// paths that are missing or differ.
func Drift(dir string, r *Result) ([]string, error) {
	var out []string
	for _, f := range r.Files {
		got, err := os.ReadFile(filepath.Join(dir, f.Path)) // #nosec G304 -- paths are compiler-generated
		if os.IsNotExist(err) {
			out = append(out, f.Path+" (missing)")
			continue
		}
		if err != nil {
			return nil, err
		}
		if string(load.Normalize(got)) != f.Content {
			out = append(out, f.Path)
		}
	}
	return out, nil
}

// Write writes the result's files under dir.
func Write(dir string, r *Result) error {
	for _, f := range r.Files {
		p := filepath.Join(dir, f.Path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { // #nosec G301 -- workflow directories are normal repo directories
			return err
		}
		if err := os.WriteFile(p, []byte(f.Content), 0o644); err != nil { // #nosec G306 -- workflows are normal, readable repo files
			return fmt.Errorf("writing %s: %w", f.Path, err)
		}
	}
	return nil
}
