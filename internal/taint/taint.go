// Package taint enforces where untrusted values may flow (design §4): event
// data reaches steps only through T_* environment variables and compiler-
// generated conditions, never as text spliced into a run:, script: or with:.
package taint

import (
	"fmt"
	"regexp"
	"slices"
	"sort"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/effects"
	"github.com/pinfold-ci/pinfold/internal/ir"
	"github.com/pinfold-ci/pinfold/internal/registry"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

// untainted is the closed list of contexts that event authors can't control
// (design §4). Every other context, every output and every var is tainted.
var untainted = []string{
	"pr.number", "issue.number", "run.id", "run.attempt",
	"event.name", "pr.author_association",
	"repo.full_name", "repo.default_branch",
}

// trustedOnly are untainted only under trusted triggers (push, schedule, manual).
var trustedOnly = []string{"commit.sha"}

var taintedEnvName = regexp.MustCompile(`^T_[A-Z0-9_]+$`)

// Tainted reports whether v is untrusted in a workflow of the given class.
func Tainted(v spec.Value, class ir.TriggerClass) bool {
	switch v.Kind {
	case spec.Literal, spec.Secret:
		return false
	case spec.Context:
		if slices.Contains(untainted, v.Text) {
			return false
		}
		return class != ir.Trusted || !slices.Contains(trustedOnly, v.Text)
	default: // output, var, and anything added later
		return true
	}
}

// Check reports taint violations. p must be ir.Lower(s).
func Check(s *spec.Spec, p *ir.Program) diag.List {
	var diags diag.List
	add := func(code string, span diag.Span, hint, format string, args ...any) {
		diags.Add(codes.New(code, span, fmt.Sprintf(format, args...), hint))
	}
	for wi, w := range s.Workflows {
		class := p.Workflows[wi].Class
		for _, j := range w.Jobs {
			for _, st := range j.Steps {
				for _, k := range sortedKeys(st.Env) {
					v := st.Env[k]
					if Tainted(v, class) && !taintedEnvName.MatchString(k) {
						add("PF202", v.Span, fmt.Sprintf("rename it to T_%s, so it's clearly untrusted in the script", upper(k)),
							"untrusted value %s %q is passed in environment variable %q, which isn't named T_*", v.Kind, v.Text, k)
					}
				}
				checkWith(st, class, add)
				checkEffectSubject(j, st, class, add)
			}
		}
	}
	diags.Sort()
	return diags
}

func checkWith(st *spec.Step, class ir.TriggerClass, add func(string, diag.Span, string, string, ...any)) {
	if st.Kind != spec.StepUses {
		return
	}
	e, reviewed, _ := registry.Lookup(st.Uses)
	for _, k := range sortedKeys(st.With) {
		v := st.With[k]
		if Tainted(v, class) {
			add("PF203", v.Span, "actions can evaluate inputs as code; pass untrusted data to a run: step through a T_* environment variable instead",
				"untrusted value %s %q is passed to input %q of %s", v.Kind, v.Text, k, st.Uses)
			continue
		}
		if v.Kind == spec.Secret && (e == nil || !reviewed || !slices.Contains(e.SecretInputs, k)) {
			hint := "only inputs the registry lists as secret inputs may receive secrets"
			if e != nil && len(e.SecretInputs) > 0 {
				hint = fmt.Sprintf("secret inputs of this action: %v", e.SecretInputs)
			}
			add("PF204", v.Span, hint, "secret %q is passed to input %q of %s, which isn't a known secret input", v.Text, k, st.Uses)
		}
	}
}

// checkEffectSubject: under a trigger outsiders can cause, event data may
// only choose what a release or commit status acts on when a maintainer
// triggered the run. Otherwise any commenter could create tags and releases
// or forge a status check (security audit D3).
func checkEffectSubject(j *spec.Job, st *spec.Step, class ir.TriggerClass, add func(string, diag.Span, string, string, ...any)) {
	e := st.Effect
	if st.Kind != spec.StepEffect || e == nil || class != ir.Privileged || effects.RequiresMaintainer(j.If) {
		return
	}
	for _, f := range []struct {
		name string
		v    *spec.Value
	}{{"tag", e.Tag}, {"sha", e.SHA}} {
		if f.v != nil && Tainted(*f.v, class) {
			add("PF205", f.v.Span, "add { maintainer: true } under the job's if: all:, or write the value out",
				"effect %s in job %q takes its %s from untrusted %s %q, but the job doesn't require a maintainer", e.Name, j.Name, f.name, f.v.Kind, f.v.Text)
		}
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch {
		case c >= 'a' && c <= 'z':
			b[i] = c - 'a' + 'A'
		case (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'):
		default:
			b[i] = '_'
		}
	}
	return string(b)
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
