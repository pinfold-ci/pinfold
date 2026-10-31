// Package verify re-reads generated workflows and checks the guarantees of
// design §2 against the YAML itself, independently of the adapter that wrote
// it. Any violation is a compiler bug (PF9xx).
package verify

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/ir"
)

// allowedInWith lists the only expressions a with: value may contain: untainted
// contexts (design §4) and secrets. Registry secret_inputs narrow secrets in M3.
var allowedInWith = regexp.MustCompile(`^(github\.event\.pull_request\.number|github\.event\.issue\.number|github\.run_id|github\.run_attempt|github\.event_name|github\.event\.pull_request\.author_association|github\.repository|github\.event\.repository\.default_branch|github\.sha|secrets\.[A-Za-z_][A-Za-z0-9_]*)$`)

var exprRE = regexp.MustCompile(`\$\{\{(.*?)\}\}`)

// Workflow checks one generated file against the IR workflow it came from.
func Workflow(file, content string, w *ir.Workflow) diag.List {
	v := &verifier{file: file}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		v.report("PF904", &doc, "generated workflow isn't a YAML mapping: %v", err)
		return v.diags
	}
	root := doc.Content[0]
	v.expressions(root, nil)

	perms := get(root, "permissions")
	if perms == nil || perms.Kind != yaml.MappingNode || len(perms.Content) != 0 {
		v.report("PF901", nodeOr(perms, root), "workflow permissions must be {}")
	}
	jobs := get(root, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		v.report("PF902", root, "generated workflow has no jobs mapping")
		return v.diags
	}
	want := map[string]*ir.Job{}
	for _, j := range w.Jobs {
		want[j.Name] = j
	}
	seen := map[string]bool{}
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		name, job := jobs.Content[i].Value, jobs.Content[i+1]
		seen[name] = true
		j := want[name]
		if j == nil {
			v.report("PF902", jobs.Content[i], "generated job %q isn't in the IR", name)
			continue
		}
		v.permissions(job, j)
		v.checkouts(job)
	}
	for name := range want {
		if !seen[name] {
			v.report("PF902", jobs, "IR job %q is missing from the generated workflow", name)
		}
	}
	v.diags.Sort()
	return v.diags
}

type verifier struct {
	file  string
	diags diag.List
}

func (v *verifier) report(code string, n *yaml.Node, format string, args ...any) {
	v.diags.Add(codes.New(code, diag.Span{File: v.file, Line: n.Line, Col: n.Column}, fmt.Sprintf(format, args...), "this is a Pinfold bug; please report it"))
}

// expressions walks the whole document. "${{" is allowed only in a step's env:
// values, and in with: values that use nothing but allowlisted expressions.
func (v *verifier) expressions(n *yaml.Node, path []string) {
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		// The emitter never writes anchors or aliases; one could hide a value from this walk.
		v.report("PF904", n, "anchor or alias in generated output")
		return
	}
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if strings.Contains(k.Value, "${{") {
				v.report("PF904", k, "expression in a key")
			}
			v.expressions(n.Content[i+1], append(slices.Clone(path), k.Value))
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			v.expressions(c, append(slices.Clone(path), "[]"))
		}
	case yaml.ScalarNode:
		if !strings.Contains(n.Value, "${{") {
			return
		}
		where := strings.Join(path, ".")
		switch {
		case inStep(path, "env"):
			return // the only place tainted values may appear (design §4)
		case inStep(path, "with"):
			rest := n.Value
			for _, m := range exprRE.FindAllStringSubmatch(n.Value, -1) {
				if !allowedInWith.MatchString(strings.TrimSpace(m[1])) {
					v.report("PF904", n, "expression ${{%s}} is not allowed in %s", m[1], where)
					return
				}
				rest = strings.Replace(rest, m[0], "", 1)
			}
			if strings.Contains(rest, "${{") {
				v.report("PF904", n, "unterminated expression in %s", where)
			}
		default:
			v.report("PF904", n, "expression in %s", where)
		}
	}
}

// inStep reports a path of the form jobs.<job>.steps.[].<key>....
func inStep(path []string, key string) bool {
	return len(path) >= 5 && path[0] == "jobs" && path[2] == "steps" && path[3] == "[]" && path[4] == key
}

func (v *verifier) permissions(job *yaml.Node, j *ir.Job) {
	want := map[string]string{}
	for scope, res := range ir.ScopeResources {
		for _, c := range j.Capabilities {
			if c.Resource == res && (want[scope] == "" || c.Access == ir.Write) {
				want[scope] = string(c.Access)
			}
		}
	}
	p := get(job, "permissions")
	if p == nil || p.Kind != yaml.MappingNode {
		v.report("PF902", job, "job %q has no permissions mapping", j.Name)
		return
	}
	got := map[string]string{}
	for i := 0; i+1 < len(p.Content); i += 2 {
		got[p.Content[i].Value] = p.Content[i+1].Value
	}
	if fmt.Sprint(sortedPairs(got)) != fmt.Sprint(sortedPairs(want)) {
		v.report("PF902", p, "job %q permissions are %v, want exactly %v", j.Name, sortedPairs(got), sortedPairs(want))
	}
}

func (v *verifier) checkouts(job *yaml.Node) {
	steps := get(job, "steps")
	if steps == nil {
		return
	}
	for _, s := range steps.Content {
		uses := get(s, "uses")
		if uses == nil || !strings.HasPrefix(uses.Value, "actions/checkout@") {
			continue
		}
		with := get(s, "with")
		if pc := get(with, "persist-credentials"); pc == nil || pc.Tag != "!!bool" || pc.Value != "false" {
			v.report("PF903", uses, "checkout without persist-credentials: false")
		}
		for _, k := range []string{"repository", "ref"} {
			if n := get(with, k); n != nil {
				v.report("PF903", n, "checkout overrides %s", k)
			}
		}
	}
}

func get(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func nodeOr(n, fallback *yaml.Node) *yaml.Node {
	if n != nil {
		return n
	}
	return fallback
}

func sortedPairs(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+":"+v)
	}
	slices.Sort(out)
	return out
}
