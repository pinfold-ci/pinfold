// Package verify re-reads generated workflows and checks the guarantees of
// design §2 against the YAML itself, independently of the adapter that wrote
// it. Any violation is a compiler bug (PF9xx).
package verify

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/effects"
	"github.com/pinfold-ci/pinfold/internal/ir"
	"github.com/pinfold-ci/pinfold/internal/registry"
)

// allowedInWith lists the only expressions a with: value may contain: untainted
// contexts (design §4) and secrets. Registry secret_inputs narrow secrets in M3.
var allowedInWith = regexp.MustCompile(`^(github\.event\.pull_request\.number|github\.event\.issue\.number|github\.run_id|github\.run_attempt|github\.event_name|github\.event\.pull_request\.author_association|github\.repository|github\.event\.repository\.default_branch|github\.sha|secrets\.[A-Za-z_][A-Za-z0-9_]*)$`)

var exprRE = regexp.MustCompile(`\$\{\{(.*?)\}\}`)

// Keys the adapter writes at each level. Anything else (job container: or
// services:, a reusable-workflow uses:, workflow env: or defaults:, a step's
// continue-on-error:, ...) could change what runs, so it is a bug (PF904).
var (
	workflowKeys = []string{"name", "on", "permissions", "concurrency", "jobs"}
	jobKeys      = []string{"name", "needs", "if", "runs-on", "environment", "permissions", "timeout-minutes", "concurrency", "outputs", "steps"}
	stepKeys     = []string{"name", "id", "if", "uses", "run", "with", "env"}
)

// Env names a generated github-script step may receive (design §5), and the
// names a registry container step reads its inputs from (D-035).
var (
	scriptEnvRE    = regexp.MustCompile(`^(T|PINFOLD)_[A-Z0-9_]+$`)
	containerEnvRE = regexp.MustCompile(`^INPUT_[A-Z0-9_]+$`)
	// Any use of the secrets context (secrets.X, secrets['X'], toJSON(secrets))
	// or of the job token, inside an expression.
	credentialRE = regexp.MustCompile(`(?i)\bsecrets\b|\bgithub\s*(?:\.\s*token\b|\[\s*['"]token['"]\s*\])`)
)

// gateEnv is the exact env of the generated maintainer gate step.
var gateEnv = map[string]string{
	"T_ACTOR":            "${{ github.actor }}",
	"T_TRIGGERING_ACTOR": "${{ github.triggering_actor }}",
}

// bindOutputRE is the bind step's checked artifact id, used only as the
// artifact-ids input of the download step right after it (pinfold#63).
var bindOutputRE = regexp.MustCompile(`^\$\{\{ steps\.pinfold_bind_([a-z][a-z0-9-]{0,62})\.outputs\.id \}\}$`)

var artifactOutputRE = regexp.MustCompile(`^\$\{\{ steps\.pinfold_upload_[a-z][a-z0-9-]{0,62}\.outputs\.artifact-(id|digest) \}\}$`)

// Workflow checks one generated file against the IR workflow it came from.
func Workflow(file, content string, w *ir.Workflow) diag.List {
	v := &verifier{file: file, zones: map[string]ir.Zone{}, privilegedTrigger: w.Class == ir.Privileged}
	v.outputs = map[string][]string{}
	for _, j := range w.Jobs {
		v.zones[j.Name] = j.Zone
		v.outputs[j.Name] = j.Outputs
	}
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader([]byte(content)))
	if err := dec.Decode(&doc); err != nil || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		v.report("PF904", &doc, "generated workflow isn't a YAML mapping: %v", err)
		return v.diags
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		v.report("PF904", &doc, "generated workflow has more than one YAML document")
		return v.diags
	}
	root := doc.Content[0]
	v.keys(root, workflowKeys, "the workflow")
	v.expressions(root, nil)

	if w.UnsafeRun && !stamped(content) {
		v.report("PF909", root, "workflow sets unsafe_run but the generated file doesn't say so")
	}
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
		v.keys(job, jobKeys, fmt.Sprintf("job %q", name))
		v.steps(job, name)
		v.permissions(job, j)
		v.checkouts(job)
		v.containers(job)
		if j.Zone == ir.ZonePrivileged {
			v.scripts(job)
			if !w.UnsafeRun {
				v.declarative(job, j.Name)
			}
		}
		if j.Zone == ir.ZonePrivileged || j.Zone == ir.ZoneFeeder {
			v.runner(job, j, content)
			v.caches(job, j.Name, j.Zone)
			// A feeder's downloads reach the privileged job too (audit F1).
			v.bindings(job, j.Name)
		}
		if hasMaintainer(j.If) {
			v.gate(job, j.Name, j.If)
		}
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
	file              string
	zones             map[string]ir.Zone  // job name -> zone, from the IR
	privilegedTrigger bool                // the workflow runs under a privileged trigger
	outputs           map[string][]string // job name -> declared output names, from the IR
	diags             diag.List
}

// stamped reports whether the file's leading comment block carries the
// unsafe_run stamp the adapter writes (not merely the text somewhere).
func stamped(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "#") {
			return false
		}
		if strings.HasPrefix(line, "# pinfold:unsafe_run:") {
			return true
		}
	}
	return false
}

// keys reports any key of mapping m that isn't in allowed.
func (v *verifier) keys(m *yaml.Node, allowed []string, where string) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if k := m.Content[i]; !slices.Contains(allowed, k.Value) {
			v.report("PF904", k, "unexpected key %q in %s", k.Value, where)
		}
	}
}

// steps checks each step's keys and the shape of its env: a mapping, with
// only T_*/PINFOLD_* names for generated scripts and INPUT_* names for
// registry containers.
func (v *verifier) steps(job *yaml.Node, name string) {
	steps := get(job, "steps")
	if steps == nil {
		return
	}
	if steps.Kind != yaml.SequenceNode {
		v.report("PF904", steps, "job %q steps isn't a list", name)
		return
	}
	for _, s := range steps.Content {
		if s.Kind != yaml.MappingNode {
			v.report("PF904", s, "a step in job %q isn't a mapping", name)
			continue
		}
		v.keys(s, stepKeys, fmt.Sprintf("a step of job %q", name))
		if w := get(s, "with"); w != nil && w.Kind != yaml.MappingNode {
			v.report("PF904", w, "a step's with: in job %q isn't a mapping", name)
		}
		env := get(s, "env")
		if env == nil {
			continue
		}
		if env.Kind != yaml.MappingNode {
			v.report("PF904", env, "a step's env: in job %q isn't a mapping", name)
			continue
		}
		uses := ""
		if u := get(s, "uses"); u != nil {
			uses = u.Value
		}
		var re *regexp.Regexp
		switch {
		case strings.HasPrefix(uses, "actions/github-script@"):
			re = scriptEnvRE
		case strings.HasPrefix(uses, "docker://"):
			re = containerEnvRE
		}
		for i := 0; i+1 < len(env.Content); i += 2 {
			k, val := env.Content[i], env.Content[i+1]
			if re != nil && !re.MatchString(k.Value) {
				v.report("PF904", k, "env name %q isn't allowed on %s in job %q", k.Value, uses, name)
			}
			if strings.HasPrefix(uses, "docker://") && strings.Contains(val.Value, "${{") && !secretOnly(val.Value) {
				v.report("PF905", val, "container input %s in job %q may only be a literal or a secret", k.Value, name)
			}
		}
	}
}

// secretOnly reports a value that is exactly one secrets.* expression.
func secretOnly(s string) bool {
	m := exprRE.FindAllStringSubmatch(s, -1)
	return len(m) == 1 && m[0][0] == s && regexp.MustCompile(`^secrets\.[A-Za-z_][A-Za-z0-9_]*$`).MatchString(strings.TrimSpace(m[0][1]))
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
		if len(path) >= 2 && path[0] == "jobs" && v.zones[path[1]] != ir.ZonePrivileged && credentialIn(n.Value) {
			// A job that the IR doesn't treat as privileged must not receive credentials.
			v.report("PF904", n, "credential in %s, but job %q isn't privileged", where, path[1])
			return
		}
		switch {
		case inStep(path, "env") && len(path) == 6 && strings.HasPrefix(path[5], "T_"):
			return // the only place tainted values may appear (design §4)
		case inStep(path, "env") && len(path) == 6:
			v.withRule(n, where) // any other env name gets the with: rule
		case len(path) == 4 && path[0] == "jobs" && path[2] == "outputs":
			// The artifact id and digest a generated upload step reports, or a
			// declared output read from the job's own steps under its own name.
			if !artifactOutputRE.MatchString(n.Value) && !v.userOutput(path[1], path[3], n.Value) {
				v.report("PF904", n, "job output %s may only report a generated upload step's artifact id or digest, or a declared output", where)
			}
		case inStep(path, "with") && len(path) == 6 && path[5] == "artifact-ids" && bindOutputRE.MatchString(n.Value):
			return // bindings checks it names the bind step right before
		case inStep(path, "with"):
			v.withRule(n, where)
		default:
			v.report("PF904", n, "expression in %s", where)
		}
	}
}

// userOutputRE is a declared output: the same output of one or more of the
// job's own steps, joined with ||.
var userOutputRE = regexp.MustCompile(`^\$\{\{ steps\.pinfold_step_[0-9]+\.outputs\.([a-z][a-z0-9_-]{0,62})(?: \|\| steps\.pinfold_step_[0-9]+\.outputs\.([a-z][a-z0-9_-]{0,62}))* \}\}$`)
var userOutputPartRE = regexp.MustCompile(`steps\.pinfold_step_[0-9]+\.outputs\.([a-z][a-z0-9_-]{0,62})`)

// userOutput reports a job output that the IR declares, whose value reads
// only that output name from the job's own steps.
func (v *verifier) userOutput(job, key, value string) bool {
	if !slices.Contains(v.outputs[job], key) || !userOutputRE.MatchString(value) {
		return false
	}
	for _, m := range userOutputPartRE.FindAllStringSubmatch(value, -1) {
		if m[1] != key {
			return false
		}
	}
	return true
}

// credentialIn reports a credential reference inside any expression in s.
func credentialIn(s string) bool {
	for _, m := range exprRE.FindAllStringSubmatch(s, -1) {
		if credentialRE.MatchString(m[1]) {
			return true
		}
	}
	// An unterminated expression is reported elsewhere; check its tail too.
	if i := strings.LastIndex(s, "${{"); i >= 0 && !strings.Contains(s[i:], "}}") {
		return credentialRE.MatchString(s[i:])
	}
	return false
}

// withRule allows only allowlisted expressions in n. Under a privileged
// trigger github.sha isn't trusted (design §4: commit.sha is untainted only
// under trusted triggers), so it is refused there.
func (v *verifier) withRule(n *yaml.Node, where string) {
	rest := n.Value
	for _, m := range exprRE.FindAllStringSubmatch(n.Value, -1) {
		e := strings.TrimSpace(m[1])
		if !allowedInWith.MatchString(e) || (v.privilegedTrigger && e == "github.sha") {
			v.report("PF904", n, "expression ${{%s}} is not allowed in %s", m[1], where)
			return
		}
		rest = strings.Replace(rest, m[0], "", 1)
	}
	if strings.Contains(rest, "${{") {
		v.report("PF904", n, "unterminated expression in %s", where)
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
		if uses == nil || !strings.HasPrefix(strings.ToLower(uses.Value), "actions/checkout@") {
			continue
		}
		with := get(s, "with")
		if pc := getFold(with, "persist-credentials"); pc == nil || pc.Tag != "!!bool" || pc.Value != "false" {
			v.report("PF903", uses, "checkout without persist-credentials: false")
		}
		for _, k := range []string{"repository", "ref"} {
			if n := getFold(with, k); n != nil {
				v.report("PF903", n, "checkout overrides %s", k)
			}
		}
	}
}

// scripts checks that, in a privileged job, every actions/github-script step is
// a compiler-generated effect: marked as generated and running exactly one of
// the generated scripts.
func (v *verifier) scripts(job *yaml.Node) {
	steps := get(job, "steps")
	if steps == nil {
		return
	}
	known := effects.Scripts()
	for _, s := range steps.Content {
		uses := get(s, "uses")
		if uses == nil || !strings.HasPrefix(uses.Value, "actions/github-script@") {
			continue
		}
		marked := false
		for i := 0; i+1 < len(s.Content); i += 2 {
			if s.Content[i].Value == "name" && strings.Contains(s.Content[i].LineComment+s.Content[i+1].LineComment, "pinfold:generated ") {
				marked = true
			}
		}
		if !marked {
			v.report("PF906", uses, "github-script step in a privileged job isn't marked as generated")
			continue
		}
		script := get(get(s, "with"), "script")
		ok := false
		for _, k := range known {
			if script != nil && script.Value == k {
				ok = true
			}
		}
		if !ok {
			v.report("PF906", uses, "github-script step in a privileged job doesn't run a generated effect script")
		}
	}
}

// bindings checks that, in a privileged or feeder job, every artifact download is
// immediately preceded by a generated binding step for the same artifact.
func (v *verifier) bindings(job *yaml.Node, name string) {
	steps := get(job, "steps")
	if steps == nil {
		return
	}
	bind := effects.Scripts()[effects.Bind]
	for i, s := range steps.Content {
		uses := get(s, "uses")
		if uses == nil || !isDownloadArtifact(uses.Value) {
			continue
		}
		// The download fetches the artifact id that the bind step right before
		// it checked, never a name: a same-name artifact uploaded after the
		// check gets a new id (pinfold#63).
		ids := get(get(s, "with"), "artifact-ids")
		var artifact string
		ok := i > 0 && ids != nil
		if ok {
			m := bindOutputRE.FindStringSubmatch(ids.Value)
			ok = m != nil
			if ok {
				artifact = m[1]
			}
		}
		if ok {
			prev := steps.Content[i-1]
			script := get(get(prev, "with"), "script")
			bound := get(get(prev, "env"), "PINFOLD_NAME")
			id := get(prev, "id")
			ok = script != nil && script.Value == bind && bound != nil && bound.Value == artifact && id != nil && id.Value == "pinfold_bind_"+artifact
		}
		if !ok {
			v.report("PF908", uses, "job %q downloads an artifact without first checking it is the one its producer uploaded", name)
			continue
		}
		// The download must fetch exactly what the bind step checked: by that
		// id, from this run, failing on a digest mismatch.
		with := get(s, "with")
		for i := 0; i+1 < len(with.Content); i += 2 {
			if k := with.Content[i]; !slices.Contains([]string{"artifact-ids", "path", "digest-mismatch"}, k.Value) {
				v.report("PF908", k, "artifact download in job %q sets %q", name, k.Value)
			}
		}
		if dm := get(with, "digest-mismatch"); dm == nil || dm.Value != "error" {
			v.report("PF908", uses, "artifact download in job %q must set digest-mismatch: error", name)
		}
		prevEnv := get(steps.Content[i-1], "env")
		for j := 0; j+1 < len(prevEnv.Content); j += 2 {
			if k := prevEnv.Content[j].Value; !slices.Contains([]string{"PINFOLD_NAME", "T_EXPECTED_ID", "T_EXPECTED_DIGEST"}, k) {
				v.report("PF908", prevEnv.Content[j], "bind step in job %q sets unexpected env %q", name, k)
			}
		}
	}
}

// generatedUses are the action references the adapter itself emits.
var generatedUses = []string{
	"actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1",
	"actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9",
	"actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333",
	"actions/github-script@3a2844b7e9c422d3c10d287c895573f7108da1b3",
}

// declarative checks design §2 row 1 on the output: a privileged job has no
// run: step, and every action is generated, privileged_ok, or a registry
// container pinned by digest.
func (v *verifier) declarative(job *yaml.Node, name string) {
	steps := get(job, "steps")
	if steps == nil {
		return
	}
	for _, s := range steps.Content {
		if r := get(s, "run"); r != nil {
			v.report("PF909", r, "job %q holds credentials but has a run: step", name)
		}
		uses := get(s, "uses")
		if uses == nil || slices.Contains(generatedUses, uses.Value) || strings.HasPrefix(uses.Value, "docker://") {
			continue // generated, or checked by PF905
		}
		if e, reviewed, err := registry.Lookup(uses.Value); err != nil || e == nil || !reviewed || !e.PrivilegedOK {
			v.report("PF909", uses, "job %q holds credentials but uses %s, which isn't privileged_ok", name, uses.Value)
		}
	}
}

// caches checks design §2 row 5 on the output: no cache restore in a job that
// holds credentials or feeds one.
func (v *verifier) caches(job *yaml.Node, name string, zone ir.Zone) {
	steps := get(job, "steps")
	if steps == nil {
		return
	}
	for _, s := range steps.Content {
		uses := get(s, "uses")
		if uses == nil {
			continue
		}
		if slices.Contains(generatedUses, uses.Value) || strings.HasPrefix(uses.Value, "docker://") {
			continue // generated, or a container checked by PF905
		}
		// Default deny: only a reviewed registry action can be shown not to
		// restore a cache.
		restores := true
		if e, reviewed, err := registry.Lookup(uses.Value); err == nil && e != nil && reviewed {
			switch e.RestoresCache {
			case "always":
			case "never":
				restores = false
			default:
				restores = false
				if with := get(s, "with"); with != nil && with.Kind == yaml.MappingNode {
					for i := 0; i+1 < len(with.Content); i += 2 {
						k, in := with.Content[i], with.Content[i+1]
						if strings.EqualFold(k.Value, e.RestoresCache) && in.Value != "" && in.Value != "false" {
							restores = true
						}
					}
				}
			}
		}
		if restores {
			v.report("PF910", uses, "job %q (%s) restores a cache with %s", name, zone, uses.Value)
		}
	}
}

// gate checks that a job with a maintainer condition starts with the generated
// gate step and that every later step only runs when the gate passed.
func (v *verifier) gate(job *yaml.Node, name string, cond *ir.Condition) {
	steps := get(job, "steps")
	if steps == nil || len(steps.Content) == 0 {
		v.report("PF907", job, "job %q needs a maintainer gate but has no steps", name)
		return
	}
	first := steps.Content[0]
	if id := get(first, "id"); id == nil || id.Value != "pinfold_gate" {
		v.report("PF907", first, "job %q needs the maintainer gate as its first step", name)
		return
	}
	if script := get(get(first, "with"), "script"); script == nil || script.Value != effects.Scripts()[effects.Gate] {
		v.report("PF907", first, "job %q's maintainer gate doesn't run the generated gate script", name)
	}
	// Exactly the generated env: PINFOLD_ROLES or similar could widen who passes.
	env := get(first, "env")
	got := map[string]string{}
	if env != nil && env.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(env.Content); i += 2 {
			got[env.Content[i].Value] = env.Content[i+1].Value
		}
	}
	want := maps.Clone(gateEnv)
	if roles := ir.GateRolesJSON(cond); roles != "" {
		want["PINFOLD_ROLES"] = roles // the spec's custom roles (#56), nothing else
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		v.report("PF907", first, "job %q's maintainer gate env is %v, want exactly %v", name, got, want)
	}
	for _, s := range steps.Content[1:] {
		if cond := get(s, "if"); cond == nil || cond.Value != gateCondition {
			v.report("PF907", s, "a step in job %q runs without checking the maintainer gate", name)
		}
	}
}

const gateCondition = "steps.pinfold_gate.outputs.ok == 'true'"

func hasMaintainer(c *ir.Condition) bool {
	if c == nil {
		return false
	}
	if c.Kind == "maintainer" {
		return true
	}
	for _, sub := range append(append([]*ir.Condition{}, c.All...), c.Any...) {
		if hasMaintainer(sub) {
			return true
		}
	}
	return false
}

// containers checks that every docker:// step is a registry-listed image pinned by digest.
func (v *verifier) containers(job *yaml.Node) {
	steps := get(job, "steps")
	if steps == nil {
		return
	}
	all, err := registry.All()
	if err != nil {
		v.report("PF905", job, "registry unavailable: %v", err)
		return
	}
	for _, s := range steps.Content {
		uses := get(s, "uses")
		if uses == nil || !strings.HasPrefix(uses.Value, "docker://") {
			continue
		}
		ok := false
		for _, e := range all {
			if e.Container != nil && uses.Value == "docker://"+e.Container.Ref() {
				ok = true
			}
		}
		if !ok {
			v.report("PF905", uses, "container image %q isn't a registry-listed image pinned by digest", uses.Value)
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

// getFold is get with a case-insensitive key match: GitHub matches action
// input names without regard to case.
func getFold(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if strings.EqualFold(m.Content[i].Value, key) {
			return m.Content[i+1]
		}
	}
	return nil
}

// isDownloadArtifact matches actions/download-artifact in any case and with
// any version or sub-path: GitHub resolves owner and repository names without
// regard to case.
func isDownloadArtifact(uses string) bool {
	path, _, _ := strings.Cut(uses, "@")
	owner, rest, _ := strings.Cut(path, "/")
	repo, _, _ := strings.Cut(rest, "/")
	return strings.EqualFold(owner, "actions") && strings.EqualFold(repo, "download-artifact")
}

// runner checks that a feeder or privileged job runs on a fresh machine: a
// standard GitHub-hosted label, or a runner the spec declares ephemeral, which
// the header must then say (security audit D1).
func (v *verifier) runner(job *yaml.Node, j *ir.Job, content string) {
	r := get(job, "runs-on")
	if r == nil || r.Kind != yaml.ScalarNode {
		v.report("PF911", job, "job %q has no single runs-on label", j.Name)
		return
	}
	if slices.Contains(ir.GitHubHostedRunners, r.Value) {
		return
	}
	if !j.EphemeralRunner || !headerHas(content, "# pinfold:ephemeral_runner: job "+j.Name+" ") {
		v.report("PF911", r, "job %q is in the %s zone but runs on %q, which isn't a GitHub-hosted runner, and no ephemeral_runner assertion is stamped", j.Name, j.Zone, r.Value)
	}
}

// headerHas reports whether the file's leading comment block has a line
// starting with prefix.
func headerHas(content, prefix string) bool {
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(line, "#") {
			return false
		}
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
