package taint

import (
	"strings"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/ir"
	"github.com/pinfold-ci/pinfold/internal/load"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

func TestTaintedTable(t *testing.T) {
	ctx := func(s string) spec.Value { return spec.Value{Kind: spec.Context, Text: s} }
	tests := []struct {
		v     spec.Value
		class ir.TriggerClass
		want  bool
	}{
		{spec.Value{Kind: spec.Literal, Text: "x"}, ir.Privileged, false},
		{spec.Value{Kind: spec.Secret, Text: "TOKEN"}, ir.Privileged, false},
		{spec.Value{Kind: spec.Var, Text: "REGION"}, ir.Trusted, true},
		{spec.Value{Kind: spec.Output, Text: "build.version"}, ir.Trusted, true},
		{ctx("issue.number"), ir.Privileged, false},
		{ctx("pr.number"), ir.Privileged, false},
		{ctx("repo.full_name"), ir.Privileged, false},
		{ctx("commit.sha"), ir.Trusted, false},
		{ctx("commit.sha"), ir.Privileged, true},
		{ctx("ref.name"), ir.Trusted, true},
		{ctx("actor"), ir.Trusted, true},
		{ctx("comment.body"), ir.Privileged, true},
		{ctx("issue.title"), ir.Privileged, true},
		{ctx("pr.head.ref"), ir.Privileged, true},
		{ctx("something.added.later"), ir.Trusted, true},
	}
	for _, tt := range tests {
		if got := Tainted(tt.v, tt.class); got != tt.want {
			t.Errorf("Tainted(%s %q, %s) = %v, want %v", tt.v.Kind, tt.v.Text, tt.class, got, tt.want)
		}
	}
}

func check(t *testing.T, src string) []string {
	t.Helper()
	root, diags := load.Load("t.yml", []byte(src))
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	s, diags := spec.Decode("t.yml", root)
	if diags.HasErrors() {
		t.Fatalf("decode: %v", diags)
	}
	var got []string
	for _, d := range Check(s, ir.Lower(s)) {
		got = append(got, d.Code)
	}
	return got
}

func TestCheck(t *testing.T) {
	head := "version: 0\nworkflows:\n  w:\n    on: { push: { branches: [main] } }\n    jobs:\n      j:\n        runner: x\n        steps:\n"
	tests := []struct {
		name, steps, want string
	}{
		{"tainted value in T_ env", "          - run: echo \"$T_REF\"\n            env: { T_REF: { context: ref.name } }\n", ""},
		{"tainted value in plain env", "          - run: echo \"$REF\"\n            env: { REF: { context: ref.name } }\n", "PF202"},
		{"lowercase t_ prefix", "          - run: echo\n            env: { t_ref: { context: ref.name } }\n", "PF202"},
		{"untainted value in plain env", "          - run: echo\n            env: { NUM: { context: run.id } }\n", ""},
		{"var in plain env", "          - run: echo\n            env: { REGION: { var: REGION } }\n", "PF202"},
		{"secret in plain env", "          - run: echo\n            env: { TOKEN: { secret: TOKEN } }\n", ""},
		{"tainted value in with", "          - uses: some/action@v1\n            with: { branch: { context: ref.name } }\n", "PF203"},
		{"output in with", "          - uses: some/action@v1\n            with: { version: { output: build.version } }\n", "PF203"},
		{"untainted value in with", "          - uses: some/action@v1\n            with: { run: { context: run.id } }\n", ""},
		{"secret to a secret input", "          - uses: actions/checkout@v7\n            with: { token: { secret: PAT } }\n", ""},
		{"secret to a non-secret input", "          - uses: actions/checkout@v7\n            with: { path: { secret: PAT } }\n", "PF204"},
		{"secret to an unknown action", "          - uses: some/action@v1\n            with: { token: { secret: PAT } }\n", "PF204"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(check(t, head+tt.steps), " "); got != tt.want {
				t.Errorf("codes = %q, want %q", got, tt.want)
			}
		})
	}
}
