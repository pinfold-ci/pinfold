// Package zones enforces the trust-zone rules of design §4: what a job may
// contain given the credentials it holds. It also checks registry-driven
// constraints on actions.
package zones

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/ir"
	"github.com/pinfold-ci/pinfold/internal/registry"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

// metadataEffects are the only steps allowed in a pull_request_target workflow (design §4).
var metadataEffects = []string{"add_labels", "remove_labels", "comment"}

// Check reports trust-zone violations. p must be ir.Lower(s).
func Check(s *spec.Spec, p *ir.Program) diag.List {
	c := &checker{}
	for wi, w := range s.Workflows {
		c.workflow(w, p.Workflows[wi])
	}
	c.diags.Sort()
	return c.diags
}

type checker struct {
	diags diag.List
}

func (c *checker) add(code string, span diag.Span, hint, format string, args ...any) {
	c.diags.Add(codes.New(code, span, fmt.Sprintf(format, args...), hint))
}

func (c *checker) workflow(w *spec.Workflow, iw *ir.Workflow) {
	metadataEvent := "" // the trigger that limits this workflow to metadata jobs
	for _, t := range w.On {
		if slices.Contains(metadataOnlyEvents, t.Event) && metadataEvent == "" {
			metadataEvent = t.Event
		}
		if t.Event == "pull_request_target" {
			c.add("PF311", t.Span, "keep this workflow for private repositories or ones with an Actions event policy that allows pull_request_target",
				"GitHub blocks pull_request_target by default in public repositories from 2026-12-02, so this workflow may never run")
		}
	}
	if w.UnsafeRun {
		c.add("PF310", w.Span, "remove unsafe_run once the job no longer needs it",
			"workflow %q sets unsafe_run: Pinfold's trust-zone guarantees are off for it, and the generated file says so", w.Name)
	}
	gated := map[string]bool{}
	for _, j := range w.Jobs {
		gated[j.Name] = maintainerGated(j.If)
	}
	// A gated job on a comment trigger must run only for new comments: the
	// gate checks the user who acted, and an edit or deletion of someone
	// else's comment would pass their text to the job (#64).
	for _, t := range w.On {
		if !slices.Contains(commentEvents, t.Event) || slices.Equal(t.Types, []string{"created"}) {
			continue
		}
		for _, j := range w.Jobs {
			if gated[j.Name] {
				c.add("PF316", t.Span, "add types: [created] to the trigger",
					"job %q is behind the maintainer gate, but %s runs it for %s, not only for new comments", j.Name, t.Event, typesWhat(t.Types))
				break
			}
		}
	}
	for _, j := range w.Jobs {
		for _, n := range j.Needs {
			if gated[n.Name] && !gated[j.Name] {
				c.add("PF314", n.Span, "add { maintainer: true } to this job's if: as well",
					"job %q needs %q, which is behind the maintainer gate; a denied gate only skips that job's steps, so %q would still run for anyone", j.Name, n.Name, j.Name)
			}
		}
	}
	for ji, j := range w.Jobs {
		zone := iw.Jobs[ji].Zone
		// Zones assume a fresh machine per job (security audit D1): on a
		// persistent runner, code from a feeder or any other workflow can
		// wait on the machine for the privileged job.
		if (zone == ir.ZoneFeeder || zone == ir.ZonePrivileged) && !ir.FreshRunner(iw.Jobs[ji]) {
			c.add("PF315", j.Span, "use a GitHub-hosted runner (e.g. ubuntu-24.04), or add ephemeral_runner: true if this runner is a fresh machine for every job",
				"job %q %s, but runs on %q, which isn't a standard GitHub-hosted runner, so Pinfold can't assume a fresh machine", j.Name, zoneWhat(zone), j.Runner)
		}
		for _, st := range j.Steps {
			if metadataEvent != "" {
				c.metadataOnly(w, metadataEvent, st)
			}
			c.step(w, iw, j, zone, st)
		}
	}
}

// maintainerGated reports a job whose if: mentions maintainer: true anywhere,
// which is what makes the adapter generate the gate (verifier PF907).
func maintainerGated(c *spec.Condition) bool {
	if c == nil {
		return false
	}
	if c.Kind == "maintainer" && c.Flag {
		return true
	}
	return slices.ContainsFunc(c.All, maintainerGated) || slices.ContainsFunc(c.Any, maintainerGated)
}

// metadataOnlyEvents run with the base repository's credentials while the
// pull request's content is in reach: pull_request_target by design, and the
// two review events because their GITHUB_SHA is the PR's merge commit, so a
// default checkout gets the PR author's files (Mythos audit D1). Workflows on
// them may only label and comment.
var metadataOnlyEvents = []string{"pull_request_target", "pull_request_review", "pull_request_review_comment"}

// metadataOnly enforces the metadata allowlist: label and comment effects only.
func (c *checker) metadataOnly(w *spec.Workflow, event string, st *spec.Step) {
	if st.Kind == spec.StepEffect && st.Effect != nil && slices.Contains(metadataEffects, st.Effect.Name) {
		return
	}
	what := string(st.Kind)
	if st.Kind == spec.StepEffect && st.Effect != nil {
		what = "effect " + st.Effect.Name
	}
	c.add("PF313", st.Span, event+" workflows may only label and comment, using the add_labels, remove_labels and comment effects",
		"workflow %q runs on %s, which doesn't allow a %s step", w.Name, event, what)
}

func (c *checker) step(w *spec.Workflow, iw *ir.Workflow, j *spec.Job, zone ir.Zone, st *spec.Step) {
	privileged := zone == ir.ZonePrivileged
	guarded := privileged && !w.UnsafeRun // unsafe_run turns the zone guarantees off (design §2)
	switch st.Kind {
	case spec.StepRun:
		if guarded {
			c.add("PF301", st.Span, "move the script to an unprivileged job that produces an artifact, and consume it here; or use an effect",
				"job %q holds credentials (%s), so it can't run a user script", j.Name, why(iw, j))
		}
	case spec.StepUses:
		c.uses(w, iw, j, zone, guarded, st)
	}
}

func (c *checker) uses(w *spec.Workflow, iw *ir.Workflow, j *spec.Job, zone ir.Zone, guarded bool, st *spec.Step) {
	// A hand-written download is invisible to the feeder analysis, so whatever
	// produced the artifact (a cache-restoring job, say) would reach the
	// privileged job unchecked (security audit F1). consumes: makes the
	// producer a feeder and binds the artifact by id and digest.
	if (zone == ir.ZoneFeeder || zone == ir.ZonePrivileged) && isDownloadArtifact(st.Uses) {
		c.add("PF307", st.Span, "declare it with consumes: { <name>: { from: <job>, to: <path> } } so Pinfold can check the producing job and the artifact",
			"job %q feeds or is a privileged job, so it can only download artifacts with consumes:", j.Name)
	}
	e, reviewed, err := registry.Lookup(st.Uses)
	if err != nil {
		c.add("PF304", st.Span, "", "can't check %s: %v", st.Uses, err)
		return
	}
	if e != nil && reviewed && e.Container != nil {
		c.containerInputs(st, e)
	}
	if guarded {
		switch {
		case e == nil:
			c.add("PF304", st.Span, "only reviewed actions run in jobs that hold credentials; see the registry, or move this step to an unprivileged job",
				"job %q holds credentials (%s), and %s isn't in Pinfold's action registry", j.Name, why(iw, j), st.Uses)
		case !reviewed:
			c.add("PF304", st.Span, fmt.Sprintf("reviewed versions: %s", strings.Join(e.Majors, ", ")),
				"job %q holds credentials (%s), and this major version of %s hasn't been reviewed", j.Name, why(iw, j), st.Uses)
		case !e.PrivilegedOK && e.Container == nil:
			c.add("PF304", st.Span, e.Reviewed.Notes,
				"job %q holds credentials (%s), and %s isn't safe there", j.Name, why(iw, j), st.Uses)
		}
	}
	if e == nil || !reviewed {
		// The registry can't say whether this action restores a cache, so in
		// zones where the cache rule applies it is refused (default deny). In a
		// guarded privileged job PF304 has already said so.
		if !guarded {
			c.cacheRule(j, zone, st, "isn't a reviewed registry action, so Pinfold can't show it doesn't restore a cache", true)
		}
		return
	}
	// Checkout hardening and the cache rule are separate §2 rows: unsafe_run
	// doesn't turn them off.
	privilegedContext := zone == ir.ZonePrivileged || iw.Class == ir.Privileged
	if privilegedContext {
		for _, k := range sortedKeys(st.With) {
			if slices.ContainsFunc(e.ForbiddenInPrivileged, func(f string) bool { return strings.EqualFold(f, k) }) {
				c.add("PF303", st.With[k].Span, "remove it; in privileged workflows Pinfold only checks out the trusted ref, without persisted credentials",
					"%s input %q isn't allowed in a privileged context", st.Uses, k)
			}
		}
	}
	if restores(e, st) {
		c.cacheRule(j, zone, st, "restores a cache", false)
	}
}

// cacheRule reports a step that restores (or may restore) a cache in a job
// where design §2 forbids it.
func (c *checker) cacheRule(j *spec.Job, zone ir.Zone, st *spec.Step, what string, unknown bool) {
	privHint, feedHint := "remove caching from this job; caches can be poisoned by less trusted runs",
		"remove caching: a poisoned cache here would reach the privileged job through the artifact"
	if unknown {
		privHint = "use a reviewed registry action, or move this step to a job that doesn't hold credentials"
		feedHint = "use a reviewed registry action, or move this step to a job whose output doesn't reach a privileged job"
	}
	switch zone {
	case ir.ZonePrivileged:
		c.add("PF302", st.Span, privHint, "job %q holds credentials, and %s %s", j.Name, st.Uses, what)
	case ir.ZoneFeeder:
		c.add("PF305", st.Span, feedHint, "job %q feeds a privileged job, and %s %s", j.Name, st.Uses, what)
	}
}

// restores reports whether this use of the action restores a cache.
func restores(e *registry.Entry, st *spec.Step) bool {
	switch e.RestoresCache {
	case "never":
		return false
	case "always":
		return true
	default:
		// Input names are case-insensitive on GitHub, so Cache: counts as cache:.
		for k, v := range st.With {
			if strings.EqualFold(k, e.RestoresCache) && v.Text != "" && v.Text != "false" {
				return true
			}
		}
		return false
	}
}

// containerInputs rejects with: inputs that a container-lowered action (D-035)
// doesn't map, and any value that isn't a literal or a secret.
func (c *checker) containerInputs(st *spec.Step, e *registry.Entry) {
	var allowed []string
	for k := range e.Container.Inputs {
		allowed = append(allowed, k)
	}
	sort.Strings(allowed)
	for _, k := range sortedKeys(st.With) {
		v := st.With[k]
		if !slices.Contains(allowed, k) {
			c.add("PF306", v.Span, fmt.Sprintf("supported inputs: %v", allowed), "input %q isn't supported for %s, which Pinfold runs as a digest-pinned container", k, st.Uses)
			continue
		}
		// The image runs with the job's credentials (OIDC for PyPI). Event data
		// must not steer it, e.g. a repository-url taken from an issue title.
		if v.Kind != spec.Literal && v.Kind != spec.Secret {
			c.add("PF306", v.Span, "write the value out, or use { secret: NAME }", "input %q of %s must be a literal or a secret, not a %s", k, st.Uses, v.Kind)
		}
	}
}

// why explains what makes a job privileged, for diagnostics.
func why(iw *ir.Workflow, j *spec.Job) string {
	var reasons []string
	if iw.Class == ir.Privileged {
		reasons = append(reasons, "privileged trigger")
	}
	for _, p := range j.Permissions {
		if p.Level == "write" {
			reasons = append(reasons, p.Scope+": write")
		}
	}
	if j.Environment != "" {
		reasons = append(reasons, "environment "+j.Environment)
	}
	for _, st := range j.Steps {
		for _, m := range []map[string]spec.Value{st.Env, st.With} {
			for _, v := range m {
				if v.Kind == spec.Secret && !slices.Contains(reasons, "secrets") {
					reasons = append(reasons, "secrets")
				}
			}
		}
	}
	if len(reasons) == 0 {
		return "credentials"
	}
	return strings.Join(reasons, ", ")
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// isDownloadArtifact reports actions/download-artifact in any case, version or
// sub-path (owner and repository names are case-insensitive on GitHub).
func isDownloadArtifact(uses string) bool {
	path, _, _ := strings.Cut(uses, "@")
	owner, rest, _ := strings.Cut(path, "/")
	repo, _, _ := strings.Cut(rest, "/")
	return strings.EqualFold(owner, "actions") && strings.EqualFold(repo, "download-artifact")
}

func zoneWhat(z ir.Zone) string {
	if z == ir.ZonePrivileged {
		return "holds credentials"
	}
	return "feeds a privileged job"
}

// commentEvents carry a comment written by someone who may not be the user
// that triggered the run (an edit or a deletion).
var commentEvents = []string{"issue_comment", "pull_request_review_comment", "discussion_comment"}

func typesWhat(types []string) string {
	if len(types) == 0 {
		return "every activity type (created, edited and deleted)"
	}
	return "types " + strings.Join(types, ", ")
}
