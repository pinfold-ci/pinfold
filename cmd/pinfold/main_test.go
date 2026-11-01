package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/codes"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"no args prints usage", nil, 2, "", "Usage:"},
		{"version", []string{"version"}, 0, "pinfold 0.0.0-dev\n", ""},
		{"help", []string{"help"}, 0, "Usage:", ""},
		{"explain", []string{"explain", "PF110"}, 0, "# PF110: Unknown key", ""},
		{"explain lower case", []string{"explain", "pf201"}, 0, "# PF201:", ""},
		{"explain unknown code", []string{"explain", "PF999"}, 2, "", `unknown diagnostic code "PF999"`},
		{"explain without code lists codes", []string{"explain"}, 0, "PF301  error    User script in a job that holds credentials\n", ""},
		{"explain list ends with usage", []string{"explain"}, 0, "Run \"pinfold explain <code>\" for the full page", ""},
		{"explain too many args", []string{"explain", "PF110", "PF111"}, 2, "", "usage: pinfold explain"},
		{"help shows the workflow", []string{"help"}, 0, "Getting started:\n  1. pinfold init", ""},
		{"help names the generated files", []string{"help"}, 0, ".github/workflows/pinfold-<name>.yml", ""},
		{"schema", []string{"schema"}, 0, `"$schema": "https://json-schema.org/draft/2020-12/schema"`, ""},
		{"schema with argument", []string{"schema", "x"}, 2, "", "usage: pinfold schema"},
		{"unknown command", []string{"frobnicate"}, 2, "", `unknown command "frobnicate"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

// TestExplainListsEveryCode keeps the "pinfold explain" list complete.
func TestExplainListsEveryCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// strings.Fields, not a slice literal: gosec G602 follows a literal
	// one-element slice through run into runIR and runInit and reports
	// their guarded args[0] as out of range.
	if code := run(strings.Fields("explain"), &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
	for _, c := range codes.All() {
		e, _ := codes.Lookup(c)
		if !strings.Contains(stdout.String(), c+"  ") || !strings.Contains(stdout.String(), e.Title) {
			t.Errorf("list is missing %s (%s)", c, e.Title)
		}
	}
}
