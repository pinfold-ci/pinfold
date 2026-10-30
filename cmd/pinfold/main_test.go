package main

import (
	"bytes"
	"strings"
	"testing"
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
		{"explain without code", []string{"explain"}, 2, "", "usage: pinfold explain"},
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
