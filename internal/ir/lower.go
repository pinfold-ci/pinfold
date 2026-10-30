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
	return out
}

func lowerJob(w *Workflow, j *spec.Job) *Job {
	out := &Job{
		ID:             w.ID + "/" + j.Name,
		Span:           j.Span,
		Name:           j.Name,
		Runner:         j.Runner,
		If:             j.If,
		Environment:    j.Environment,
		TimeoutMinutes: j.TimeoutMinutes,
		Concurrency:    j.Concurrency,
		Produces:       j.Produces,
		Consumes:       j.Consumes,
	}
	for _, n := range j.Needs {
		out.Needs = append(out.Needs, n.Name)
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

// markFeeders marks unprivileged jobs whose artifacts a privileged job consumes.
func markFeeders(w *Workflow) {
	byName := map[string]*Job{}
	for _, j := range w.Jobs {
		byName[j.Name] = j
	}
	for _, j := range w.Jobs {
		if j.Zone != ZonePrivileged {
			continue
		}
		for _, c := range j.Consumes {
			if src := byName[c.From.Name]; src != nil && src.Zone == ZoneUnprivileged {
				src.Zone = ZoneFeeder
			}
		}
	}
}
