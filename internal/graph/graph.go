// Package graph checks the job dependency graph of each workflow and orders
// its jobs deterministically.
package graph

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

// Check validates needs and consumes for every workflow and returns, per
// workflow name, the job names in a deterministic topological order: a job
// comes after everything it needs, and ties keep source order.
func Check(s *spec.Spec) (map[string][]string, diag.List) {
	var diags diag.List
	orders := map[string][]string{}
	for _, w := range s.Workflows {
		order, ok := topo(w, &diags)
		if ok {
			orders[w.Name] = order
			checkConsumes(w, &diags)
			checkOutputs(w, &diags)
		}
	}
	diags.Sort()
	return orders, diags
}

func topo(w *spec.Workflow, diags *diag.List) ([]string, bool) {
	jobs := map[string]*spec.Job{}
	for _, j := range w.Jobs {
		jobs[j.Name] = j
	}
	const (
		unvisited = iota
		visiting
		done
	)
	state := map[string]int{}
	var order []string
	var stack []string
	ok := true
	var visit func(j *spec.Job)
	visit = func(j *spec.Job) {
		state[j.Name] = visiting
		stack = append(stack, j.Name)
		for _, n := range j.Needs {
			dep := jobs[n.Name]
			if dep == nil {
				continue // PF124, reported by the decoder
			}
			switch state[n.Name] {
			case visiting:
				i := slices.Index(stack, n.Name)
				cycle := append(slices.Clone(stack[i:]), n.Name)
				diags.Add(codes.New("PF126", n.Span, fmt.Sprintf("workflow %q: jobs depend on each other in a cycle: %s", w.Name, strings.Join(cycle, " → ")), fmt.Sprintf("remove %q from the needs of job %q", n.Name, j.Name)))
				ok = false
			case unvisited:
				visit(dep)
			}
		}
		stack = stack[:len(stack)-1]
		state[j.Name] = done
		order = append(order, j.Name)
	}
	for _, j := range w.Jobs {
		if state[j.Name] == unvisited {
			visit(j)
		}
	}
	return order, ok
}

// checkConsumes reports artifacts consumed from a job that isn't a transitive
// dependency (PF127) or doesn't produce that artifact (PF128).
func checkConsumes(w *spec.Workflow, diags *diag.List) {
	jobs := map[string]*spec.Job{}
	for _, j := range w.Jobs {
		jobs[j.Name] = j
	}
	var deps func(name string, seen map[string]bool)
	deps = func(name string, seen map[string]bool) {
		for _, n := range jobs[name].Needs {
			if jobs[n.Name] != nil && !seen[n.Name] {
				seen[n.Name] = true
				deps(n.Name, seen)
			}
		}
	}
	for _, j := range w.Jobs {
		upstream := map[string]bool{}
		deps(j.Name, upstream)
		for _, c := range j.Consumes {
			src := jobs[c.From.Name]
			if src == nil {
				continue // PF124, reported by the decoder
			}
			if !upstream[src.Name] {
				diags.Add(codes.New("PF127", c.From.Span, fmt.Sprintf("job %q consumes %q from job %q, which it doesn't depend on", j.Name, c.Name, src.Name), fmt.Sprintf("add %q to the needs of job %q", src.Name, j.Name)))
				continue
			}
			if !slices.ContainsFunc(src.Produces, func(a spec.Artifact) bool { return a.Name == c.Name }) {
				diags.Add(codes.New("PF128", c.Span, fmt.Sprintf("job %q consumes %q, but job %q doesn't produce it", j.Name, c.Name, src.Name), ""))
			}
		}
	}
}

// checkOutputs reports { output: job.name } values that name a job this one
// doesn't depend on, or an output that job doesn't declare (PF131). Without
// the check the reference would compile to an always-empty value.
func checkOutputs(w *spec.Workflow, diags *diag.List) {
	jobs := map[string]*spec.Job{}
	for _, j := range w.Jobs {
		jobs[j.Name] = j
	}
	var deps func(name string, seen map[string]bool)
	deps = func(name string, seen map[string]bool) {
		for _, n := range jobs[name].Needs {
			if jobs[n.Name] != nil && !seen[n.Name] {
				seen[n.Name] = true
				deps(n.Name, seen)
			}
		}
	}
	for _, j := range w.Jobs {
		if len(j.Outputs) > 0 && !slices.ContainsFunc(j.Steps, func(s *spec.Step) bool { return s.Kind == spec.StepRun || s.Kind == spec.StepUses }) {
			diags.Add(codes.New("PF131", j.Outputs[0].Span, fmt.Sprintf("job %q declares outputs but has no run: or uses: step to set them", j.Name), "remove outputs:, or add the step that writes them"))
		}
		upstream := map[string]bool{}
		deps(j.Name, upstream)
		for _, v := range outputRefs(j) {
			src, name, _ := strings.Cut(v.Text, ".")
			switch from := jobs[src]; {
			case from == nil:
				diags.Add(codes.New("PF131", v.Span, fmt.Sprintf("job %q reads output %q of unknown job %q", j.Name, name, src), ""))
			case !upstream[src]:
				diags.Add(codes.New("PF131", v.Span, fmt.Sprintf("job %q reads output %q of job %q, which it doesn't depend on", j.Name, name, src), fmt.Sprintf("add %q to the needs of job %q", src, j.Name)))
			case !slices.ContainsFunc(from.Outputs, func(r spec.Ref) bool { return r.Name == name }):
				diags.Add(codes.New("PF131", v.Span, fmt.Sprintf("job %q reads output %q, but job %q doesn't declare it", j.Name, name, src), fmt.Sprintf("add outputs: [%s] to job %q, and have a step write %s=... to $GITHUB_OUTPUT", name, src, name)))
			}
		}
	}
}

// outputRefs returns every { output: ... } value in a job's steps.
func outputRefs(j *spec.Job) []spec.Value {
	var out []spec.Value
	add := func(v *spec.Value) {
		if v != nil && v.Kind == spec.Output {
			out = append(out, *v)
		}
	}
	for _, st := range j.Steps {
		for _, m := range []map[string]spec.Value{st.Env, st.With} {
			for _, k := range sortedKeys(m) {
				v := m[k]
				add(&v)
			}
		}
		if e := st.Effect; e != nil {
			add(e.Target)
			add(e.SHA)
			add(e.Tag)
		}
	}
	return out
}

func sortedKeys(m map[string]spec.Value) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
