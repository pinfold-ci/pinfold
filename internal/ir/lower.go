package ir

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pinfold-ci/pinfold/internal/spec"
)

// trustedTriggers are the only events whose jobs may be unprivileged (design §4).
var trustedTriggers = map[string]bool{"push": true, "schedule": true, "manual": true}

// ScopeResources maps GitHub permission scopes to abstract resources. It is
// versioned data that mirrors spec.PermissionLevels.
var ScopeResources = map[string]string{
	"actions":              "ci.actions",
	"artifact-metadata":    "artifacts.metadata",
	"attestations":         "attestations",
	"checks":               "ci.checks",
	"code-quality":         "code-quality",
	"contents":             "repo.contents",
	"copilot-requests":     "copilot",
	"deployments":          "deployments",
	"discussions":          "discussions",
	"id-token":             "oidc",
	"issues":               "issues",
	"packages":             "packages",
	"pages":                "pages",
	"pull-requests":        "pr",
	"security-events":      "security-events",
	"statuses":             "commit-statuses",
	"vulnerability-alerts": "vulnerability-alerts",
}

// Lower builds the IR from a spec that decoded without errors.
func Lower(s *spec.Spec) *Program {
	p := &Program{}
	for _, w := range s.Workflows {
		p.Workflows = append(p.Workflows, lowerWorkflow(w))
	}
	return p
}

func lowerWorkflow(w *spec.Workflow) *Workflow {
	out := &Workflow{ID: w.Name, Span: w.Span, Class: Trusted, Triggers: w.On, UnsafeRun: w.UnsafeRun}
	for _, t := range w.On {
		if !trustedTriggers[t.Event] {
			out.Class = Privileged
		}
	}
	for _, j := range w.Jobs {
		out.Jobs = append(out.Jobs, lowerJob(out, j))
	}
	markFeeders(out)
	for _, j := range out.Jobs {
		addBindingCapability(j)
	}
	return out
}

// addBindingCapability gives a privileged or feeder job that consumes
// artifacts actions: read, so its generated bind step can list this run's
// artifacts and check each one's id and digest before the download (design
// §2, D-036). An unprivileged job that feeds nothing downloads by name
// without it: design §4 limits those jobs to contents: read (security audit).
func addBindingCapability(j *Job) {
	if len(j.Consumes) == 0 || j.Zone == ZoneUnprivileged {
		return
	}
	if slices.ContainsFunc(j.Capabilities, func(c Capability) bool { return c.Resource == "ci.actions" }) {
		return
	}
	j.Capabilities = append(j.Capabilities, Capability{Resource: "ci.actions", Access: Read, Source: "artifact binding"})
	slices.SortStableFunc(j.Capabilities, func(a, b Capability) int { return strings.Compare(a.Resource, b.Resource) })
}

func lowerJob(w *Workflow, j *spec.Job) *Job {
	out := &Job{
		ID:              w.ID + "/" + j.Name,
		Span:            j.Span,
		Name:            j.Name,
		Runner:          j.Runner,
		EphemeralRunner: j.EphemeralRunner,
		If:              j.If,
		Environment:     j.Environment,
		TimeoutMinutes:  j.TimeoutMinutes,
		Concurrency:     j.Concurrency,
		Produces:        j.Produces,
		Consumes:        j.Consumes,
	}
	for _, n := range j.Needs {
		out.Needs = append(out.Needs, n.Name)
	}
	for _, o := range j.Outputs {
		out.Outputs = append(out.Outputs, o.Name)
	}
	for _, p := range j.Permissions {
		if p.Level == "none" {
			continue
		}
		out.Capabilities = append(out.Capabilities, Capability{Resource: ScopeResources[p.Scope], Access: Access(p.Level), Source: "declared"})
	}
	usesSecret := false
	for i, s := range j.Steps {
		step := &Step{
			ID:   fmt.Sprintf("%s/step-%d", out.ID, i+1),
			Span: s.Span, Kind: StepKind(s.Kind), Name: s.Name,
			Run: s.Run, Env: s.Env, Uses: s.Uses, With: s.With, Effect: s.Effect,
		}
		for _, v := range s.Env {
			usesSecret = usesSecret || v.Kind == spec.Secret
		}
		for _, v := range s.With {
			usesSecret = usesSecret || v.Kind == spec.Secret
		}
		out.Steps = append(out.Steps, step)
	}
	slices.SortStableFunc(out.Capabilities, func(a, b Capability) int { return strings.Compare(a.Resource, b.Resource) })

	out.Zone = ZoneUnprivileged
	if w.Class == Privileged || usesSecret || out.Environment != "" || holdsCredential(out.Capabilities) {
		out.Zone = ZonePrivileged
	}
	return out
}

// holdsCredential reports a write capability or OIDC.
func holdsCredential(caps []Capability) bool {
	return slices.ContainsFunc(caps, func(c Capability) bool { return c.Access == Write || c.Resource == "oidc" })
}

// markFeeders marks every unprivileged job whose artifacts or outputs reach a
// privileged job, directly or through other feeders (design §4).
func markFeeders(w *Workflow) {
	byName := map[string]*Job{}
	for _, j := range w.Jobs {
		byName[j.Name] = j
	}
	for changed := true; changed; {
		changed = false
		for _, j := range w.Jobs {
			if j.Zone == ZoneUnprivileged {
				continue
			}
			for _, src := range upstream(j) {
				if s := byName[src]; s != nil && s.Zone == ZoneUnprivileged {
					s.Zone = ZoneFeeder
					changed = true
				}
			}
		}
	}
}

// upstream returns the jobs whose artifacts or outputs j consumes.
func upstream(j *Job) []string {
	var out []string
	for _, c := range j.Consumes {
		out = append(out, c.From.Name)
	}
	add := func(v *Value) {
		if v != nil && v.Kind == spec.Output {
			job, _, _ := strings.Cut(v.Text, ".")
			out = append(out, job)
		}
	}
	for _, s := range j.Steps {
		for _, m := range []map[string]Value{s.Env, s.With} {
			for _, v := range m {
				add(&v)
			}
		}
		if e := s.Effect; e != nil {
			add(e.Target)
			add(e.SHA)
			add(e.Tag)
		}
	}
	return out
}
