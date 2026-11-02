package spec

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/diag"
)

// maxKeyLen bounds env: and with: keys, well under YAML's 1024-character limit on keys.
const maxKeyLen = 256

var (
	nameRE      = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	envNameRE   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	inputNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
	outputRE    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}\.[a-z][a-z0-9_-]{0,62}$`)
	usesRE      = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}(?:/[A-Za-z0-9._/-]+)?@[A-Za-z0-9._/+-]{1,128}$`)
	hexSHARE    = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
)

// branchRefs are ref names that are branches in practice. Telling a branch
// from a tag needs the remote; pinfold lock (M4) does that for every ref, and
// this list catches the common case at validate time (external audit).
var branchRefs = []string{"main", "master", "develop", "development", "dev", "trunk", "head", "latest", "nightly", "next", "canary", "edge"}

// Decode turns the loader's top-level mapping into a Spec. It reports every
// problem it finds rather than stopping at the first; the Spec is only
// meaningful when the returned list has no errors.
func Decode(file string, root *yaml.Node) (*Spec, diag.List) {
	d := &decoder{file: file}
	d.scanExpressions(root)
	s := d.spec(root)
	d.diags.Sort()
	return s, d.diags
}

type decoder struct {
	file  string
	diags diag.List
}

func (d *decoder) span(n *yaml.Node) diag.Span {
	return diag.Span{File: d.file, Line: n.Line, Col: n.Column}
}

func (d *decoder) report(code string, n *yaml.Node, hint, format string, args ...any) {
	d.diags.Add(codes.New(code, d.span(n), fmt.Sprintf(format, args...), hint))
}

// scanExpressions reports every "${{" in any key or value (spec rule 1).
func (d *decoder) scanExpressions(n *yaml.Node) {
	if n.Kind == yaml.ScalarNode && strings.Contains(n.Value, "${{") {
		d.report("PF201", n, `use a typed value such as { context: issue.number } or { secret: NAME }`, "GitHub expressions (${{ ... }}) aren't allowed in the spec")
	}
	for _, c := range n.Content {
		d.scanExpressions(c)
	}
}

// field is one key/value pair of a mapping.
type field struct {
	key   string
	kNode *yaml.Node
	val   *yaml.Node
}

// fields checks that n is a mapping with only allowed keys and all required
// keys, and returns its pairs in source order. unknownCode is the code used
// for keys outside allowed (PF110 almost everywhere).
func (d *decoder) fields(n *yaml.Node, what string, allowed, required []string, unknownCode string) []field {
	if !d.isKind(n, yaml.MappingNode, what) {
		return nil
	}
	var out []field
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if !slices.Contains(allowed, k.Value) {
			d.report(unknownCode, k, suggest(k.Value, allowed), "unknown key %q in %s", k.Value, what)
			if s, ok := synonyms[k.Value]; ok {
				seen[s] = true // already reported as "did you mean"; don't also say it's missing
			}
			continue
		}
		seen[k.Value] = true
		out = append(out, field{k.Value, k, v})
	}
	for _, r := range required {
		if !seen[r] {
			d.report("PF111", n, "", "%s is missing required key %q", what, r)
		}
	}
	return out
}

var kindNames = map[yaml.Kind]string{yaml.MappingNode: "a mapping", yaml.SequenceNode: "a list", yaml.ScalarNode: "a single value"}

func (d *decoder) isKind(n *yaml.Node, k yaml.Kind, what string) bool {
	if n.Kind == k && n.Tag != "!!null" {
		return true
	}
	if k == yaml.MappingNode && n.Kind == yaml.ScalarNode && n.Tag == "!!null" {
		return true // "key:" with no value is an empty mapping
	}
	got := kindNames[n.Kind]
	if n.Tag == "!!null" {
		got = "empty"
	}
	d.report("PF112", n, "", "%s must be %s, not %s", what, kindNames[k], got)
	return false
}

// str decodes a non-empty string. Numbers are accepted as their text; booleans and nulls are not.
func (d *decoder) str(n *yaml.Node, what string) (string, bool) {
	if n.Kind != yaml.ScalarNode || n.Tag == "!!null" || n.Tag == "!!bool" || n.Value == "" {
		got := kindNames[n.Kind]
		switch {
		case n.Tag == "!!bool":
			got = "a boolean"
		case n.Tag == "!!null" || n.Value == "":
			got = "empty"
		}
		d.report("PF112", n, "quote the value if it is meant to be text", "%s must be a non-empty string, not %s", what, got)
		return "", false
	}
	return n.Value, true
}

func (d *decoder) boolean(n *yaml.Node, what string) (bool, bool) {
	// Only the lowercase spellings: YAML also reads True and TRUE as booleans,
	// which this compared as false (external audit V-F).
	if n.Kind != yaml.ScalarNode || n.Tag != "!!bool" || (n.Value != "true" && n.Value != "false") {
		d.report("PF112", n, "use true or false", "%s must be true or false", what)
		return false, false
	}
	return n.Value == "true", true
}

func (d *decoder) integer(n *yaml.Node, what string, lo, hi int) (int, bool) {
	v, err := strconv.Atoi(n.Value)
	if n.Kind != yaml.ScalarNode || n.Tag != "!!int" || err != nil {
		d.report("PF112", n, "", "%s must be a whole number", what)
		return 0, false
	}
	if v < lo || v > hi {
		d.report("PF113", n, "", "%s must be between %d and %d, not %d", what, lo, hi, v)
		return 0, false
	}
	return v, true
}

// strList decodes a list of strings. If scalarOK, a single string counts as a one-item list.
func (d *decoder) strList(n *yaml.Node, what string, scalarOK bool) []string {
	if scalarOK && n.Kind == yaml.ScalarNode {
		if s, ok := d.str(n, what); ok {
			return []string{s}
		}
		return nil
	}
	if !d.isKind(n, yaml.SequenceNode, what) {
		return nil
	}
	if len(n.Content) == 0 {
		d.report("PF111", n, "", "%s must not be empty", what)
	}
	var out []string
	for _, c := range n.Content {
		if s, ok := d.str(c, what+" entry"); ok {
			out = append(out, s)
		}
	}
	return out
}

func (d *decoder) oneOf(n *yaml.Node, what, v string, allowed []string) bool {
	if slices.Contains(allowed, v) {
		return true
	}
	d.report("PF113", n, "allowed values: "+strings.Join(allowed, ", "), "%q is not a valid %s", v, what)
	return false
}

func (d *decoder) name(n *yaml.Node, what string) string {
	if !nameRE.MatchString(n.Value) {
		d.report("PF114", n, "use lowercase letters, digits and hyphens, starting with a letter (e.g. build-docs)", "invalid %s name %q", what, n.Value)
	}
	return n.Value
}

func (d *decoder) spec(root *yaml.Node) *Spec {
	s := &Spec{Version: -1}
	for _, f := range d.fields(root, "the spec", topKeys, topKeys, "PF110") {
		switch f.key {
		case "version":
			if f.val.Kind != yaml.ScalarNode || f.val.Tag != "!!int" || f.val.Value != strconv.Itoa(SupportedVersion) {
				d.report("PF115", f.val, fmt.Sprintf("set \"version: %d\"", SupportedVersion), "unsupported spec version %q", f.val.Value)
				continue
			}
			s.Version = SupportedVersion
		case "workflows":
			if !d.isKind(f.val, yaml.MappingNode, "workflows") {
				continue
			}
			if len(f.val.Content) == 0 {
				d.report("PF111", f.val, "", "workflows must define at least one workflow")
			}
			for i := 0; i+1 < len(f.val.Content); i += 2 {
				s.Workflows = append(s.Workflows, d.workflow(f.val.Content[i], f.val.Content[i+1]))
			}
		}
	}
	return s
}

func (d *decoder) workflow(k, v *yaml.Node) *Workflow {
	w := &Workflow{Name: d.name(k, "workflow"), Span: d.span(k)}
	what := fmt.Sprintf("workflow %q", w.Name)
	var jobsNode *yaml.Node
	for _, f := range d.fields(v, what, workflowKeys, []string{"on", "jobs"}, "PF110") {
		switch f.key {
		case "on":
			w.On = d.triggers(f.val, what)
		case "jobs":
			jobsNode = f.val
		case "unsafe_run":
			w.UnsafeRun, _ = d.boolean(f.val, "unsafe_run")
		}
	}
	if jobsNode == nil || !d.isKind(jobsNode, yaml.MappingNode, what+" jobs") {
		return w
	}
	if len(jobsNode.Content) == 0 {
		d.report("PF111", jobsNode, "", "%s must define at least one job", what)
	}
	for i := 0; i+1 < len(jobsNode.Content); i += 2 {
		w.Jobs = append(w.Jobs, d.job(jobsNode.Content[i], jobsNode.Content[i+1]))
	}
	d.checkRefs(w)
	return w
}

func (d *decoder) triggers(n *yaml.Node, what string) []*Trigger {
	if !d.isKind(n, yaml.MappingNode, what+" on") {
		return nil
	}
	if len(n.Content) == 0 {
		d.report("PF111", n, "", "%s must have at least one trigger", what)
	}
	var out []*Trigger
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		keys, ok := Triggers[k.Value]
		if !ok {
			if k.Value == "workflow_run" {
				d.report("PF312", k, "keep this workflow hand-written for now", "user-written workflow_run workflows aren't supported")
				continue
			}
			hint := unsupportedTriggers[k.Value]
			if hint == "" {
				hint = suggest(k.Value, sortedKeys(Triggers))
			}
			d.report("PF120", k, hint, "trigger %q is not supported", k.Value)
			continue
		}
		t := &Trigger{Event: k.Value, Span: d.span(k)}
		for _, f := range d.fields(v, fmt.Sprintf("trigger %q", k.Value), keys, nil, "PF110") {
			switch f.key {
			case "branches":
				t.Branches = d.strList(f.val, "branches", false)
			case "tags":
				t.Tags = d.strList(f.val, "tags", false)
			case "paths":
				t.Paths = d.strList(f.val, "paths", false)
			case "types":
				t.Types = d.strList(f.val, "types", false)
			case "cron":
				t.Crons = d.strList(f.val, "cron", true)
			}
		}
		if t.Event == "schedule" && len(t.Crons) == 0 {
			d.report("PF111", k, `add cron: "0 3 * * 1"`, "schedule needs at least one cron expression")
		}
		out = append(out, t)
	}
	return out
}

func (d *decoder) job(k, v *yaml.Node) *Job {
	j := &Job{Name: d.name(k, "job"), Span: d.span(k)}
	what := fmt.Sprintf("job %q", j.Name)
	for _, f := range d.fields(v, what, jobKeys, []string{"runner", "steps"}, "PF110") {
		switch f.key {
		case "runner":
			j.Runner, _ = d.str(f.val, "runner")
		case "needs":
			for i, s := range d.strList(f.val, "needs", true) {
				n := f.val
				if f.val.Kind == yaml.SequenceNode {
					n = f.val.Content[i]
				}
				j.Needs = append(j.Needs, Ref{s, d.span(n)})
			}
		case "if":
			j.If = d.condition(f.val, true)
		case "permissions":
			j.Permissions = d.permissions(f.val)
		case "ephemeral_runner":
			j.EphemeralRunner, _ = d.boolean(f.val, "ephemeral_runner")
		case "environment":
			j.Environment, _ = d.str(f.val, "environment")
		case "timeout-minutes":
			j.TimeoutMinutes, _ = d.integer(f.val, "timeout-minutes", 1, 360)
		case "concurrency":
			j.Concurrency = d.concurrency(f.val)
		case "steps":
			if !d.isKind(f.val, yaml.SequenceNode, "steps") {
				continue
			}
			if len(f.val.Content) == 0 {
				d.report("PF111", f.val, "", "%s must have at least one step", what)
			}
			for _, s := range f.val.Content {
				j.Steps = append(j.Steps, d.step(s))
			}
		case "produces":
			j.Produces = d.produces(f.val)
		case "consumes":
			j.Consumes = d.consumes(f.val)
		}
	}
	return j
}

func (d *decoder) permissions(n *yaml.Node) []Permission {
	if n.Kind == yaml.ScalarNode && (n.Value == "read-all" || n.Value == "write-all") {
		d.report("PF130", n, "list each scope you need, e.g. { contents: read }", "%q is not accepted; permissions must be declared per scope", n.Value)
		return nil
	}
	if !d.isKind(n, yaml.MappingNode, "permissions") {
		return nil
	}
	var out []Permission
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		levels, ok := PermissionLevels[k.Value]
		if !ok {
			d.report("PF130", k, suggest(k.Value, sortedKeys(PermissionLevels)), "unknown permission scope %q", k.Value)
			continue
		}
		if v.Kind != yaml.ScalarNode || !slices.Contains(levels, v.Value) {
			d.report("PF130", v, "allowed values: "+strings.Join(levels, ", "), "invalid level %q for permission %q", v.Value, k.Value)
			continue
		}
		out = append(out, Permission{Scope: k.Value, Level: v.Value, Span: d.span(k)})
	}
	return out
}

func (d *decoder) concurrency(n *yaml.Node) *Concurrency {
	c := &Concurrency{}
	for _, f := range d.fields(n, "concurrency", []string{"group", "cancel-in-progress"}, []string{"group"}, "PF110") {
		switch f.key {
		case "group":
			c.Group, _ = d.str(f.val, "concurrency group")
		case "cancel-in-progress":
			c.CancelInProgress, _ = d.boolean(f.val, "cancel-in-progress")
		}
	}
	return c
}

func (d *decoder) produces(n *yaml.Node) []Artifact {
	if !d.isKind(n, yaml.MappingNode, "produces") {
		return nil
	}
	var out []Artifact
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		path, _ := d.str(v, "artifact path")
		out = append(out, Artifact{Name: d.name(k, "artifact"), Path: path, Span: d.span(k)})
	}
	return out
}

func (d *decoder) consumes(n *yaml.Node) []Consume {
	if !d.isKind(n, yaml.MappingNode, "consumes") {
		return nil
	}
	var out []Consume
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		c := Consume{Name: d.name(k, "artifact"), Span: d.span(k)}
		for _, f := range d.fields(v, fmt.Sprintf("consumes %q", k.Value), []string{"from", "to"}, []string{"from", "to"}, "PF110") {
			switch f.key {
			case "from":
				name, _ := d.str(f.val, "from")
				c.From = Ref{name, d.span(f.val)}
			case "to":
				c.To, _ = d.str(f.val, "to")
				d.workspacePath(f.val, c.To, "to")
			}
		}
		out = append(out, c)
	}
	return out
}

// workspacePath rejects a download directory outside a subdirectory of the
// workspace: ".", absolute paths, and paths that climb out with "..". A
// download there could overwrite the checked-out repository or files the
// runner uses (external audit).
func (d *decoder) workspacePath(n *yaml.Node, p, what string) {
	if p == "" {
		return
	}
	slashed := strings.ReplaceAll(p, `\`, "/")
	clean := path.Clean(slashed)
	switch {
	case strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) || strings.HasPrefix(p, "~") || (len(p) > 1 && p[1] == ':'):
		d.report("PF113", n, "use a path relative to the workspace, e.g. dist", "%s must be a relative path, not %q", what, p)
	case clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || slices.Contains(strings.Split(slashed, "/"), ".."):
		d.report("PF113", n, "use a subdirectory of the workspace, e.g. dist", "%s must be a subdirectory of the workspace, not %q", what, p)
	}
}

// checkRefs reports needs and consumes entries that name jobs missing from the workflow.
func (d *decoder) checkRefs(w *Workflow) {
	names := map[string]bool{}
	for _, j := range w.Jobs {
		names[j.Name] = true
	}
	var all []string
	for _, j := range w.Jobs {
		all = append(all, j.Name)
	}
	check := func(r Ref, what string) {
		if r.Name != "" && !names[r.Name] {
			d.diags.Add(codes.New("PF124", r.Span, fmt.Sprintf("%s refers to unknown job %q", what, r.Name), suggest(r.Name, all)))
		}
	}
	for _, j := range w.Jobs {
		for _, r := range j.Needs {
			check(r, fmt.Sprintf("job %q: needs", j.Name))
		}
		for _, c := range j.Consumes {
			check(c.From, fmt.Sprintf("job %q: consumes %q from", j.Name, c.Name))
		}
		// Downloads run after a leading checkout and before every other step;
		// a later checkout would clean the workspace and delete them.
		if len(j.Consumes) > 0 {
			for i, st := range j.Steps {
				if i > 0 && st.Kind == StepCheckout {
					d.diags.Add(codes.New("PF129", st.Span, fmt.Sprintf("job %q consumes artifacts, so its checkout must be the first step", j.Name),
						"move the checkout to the top of the job; Pinfold downloads the artifacts right after it"))
				}
			}
		}
	}
}

func (d *decoder) step(n *yaml.Node) *Step {
	s := &Step{Span: d.span(n)}
	if !d.isKind(n, yaml.MappingNode, "a step") {
		return s
	}
	var kinds []string
	for i := 0; i+1 < len(n.Content); i += 2 {
		if k := n.Content[i].Value; slices.Contains(stepKinds, k) {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) != 1 {
		msg := "a step has none of checkout, run, uses or effect"
		if len(kinds) > 1 {
			msg = "a step has more than one of " + strings.Join(kinds, ", ")
		}
		d.report("PF121", n, "split it into separate steps, one kind each", "%s", msg)
		return s
	}
	s.Kind = StepKind(kinds[0])
	allowed := append(slices.Clone(stepCommon), kinds[0])
	unknown := "PF110"
	switch s.Kind {
	case StepRun:
		allowed = append(allowed, "env")
	case StepUses:
		allowed = append(allowed, "with")
	case StepEffect:
		allowed, unknown = d.effectKeys(n, allowed)
	}
	for _, f := range d.fields(n, fmt.Sprintf("%s step", s.Kind), allowed, nil, unknown) {
		switch f.key {
		case "name":
			s.Name, _ = d.str(f.val, "step name")
		case "checkout":
			d.fields(f.val, "checkout", nil, nil, "PF110")
		case "run":
			s.Run, _ = d.str(f.val, "run")
		case "env":
			s.Env = d.valueMap(f.val, "env", true)
		case "uses":
			s.Uses = d.uses(f.val)
		case "with":
			s.With = d.valueMap(f.val, "with", false)
		case "effect":
			s.Effect = d.effect(n, f)
		}
	}
	return s
}

// effectKeys returns the keys an effect step accepts. Unknown keys get PF122.
func (d *decoder) effectKeys(n *yaml.Node, base []string) ([]string, string) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == "effect" {
			if t, ok := effectFields[n.Content[i+1].Value]; ok {
				return append(base, t.allowed...), "PF122"
			}
			// Unknown effect: reported once by effect(); don't also flag each field.
			var all []string
			for j := 0; j+1 < len(n.Content); j += 2 {
				all = append(all, n.Content[j].Value)
			}
			return all, "PF122"
		}
	}
	return base, "PF122"
}

func (d *decoder) effect(step *yaml.Node, ef field) *Effect {
	name, ok := d.str(ef.val, "effect")
	if !ok {
		return nil
	}
	table, ok := effectFields[name]
	if !ok {
		d.report("PF122", ef.val, "effects: "+strings.Join(sortedKeys(effectFields), ", "), "unknown effect %q", name)
		return nil
	}
	e := &Effect{Name: name, Span: d.span(ef.kNode)}
	present := map[string]bool{}
	for i := 0; i+1 < len(step.Content); i += 2 {
		k, v := step.Content[i], step.Content[i+1]
		if !slices.Contains(table.allowed, k.Value) {
			continue // reported by fields()
		}
		present[k.Value] = true
		switch k.Value {
		case "target":
			e.Target = d.effectValue(v, "target")
		case "body":
			e.Body, _ = d.str(v, "body")
		case "sticky":
			e.Sticky = d.name(v, "sticky marker")
		case "labels":
			e.Labels, e.FromCommentArgs = d.labels(v)
		case "allowed":
			e.Allowed = d.strList(v, "allowed", false)
		case "max":
			e.Max, _ = d.integer(v, "max", 1, 100)
		case "sha":
			e.SHA = d.effectValue(v, "sha")
		case "state":
			if st, ok := d.str(v, "state"); ok && d.oneOf(v, "commit state", st, commitStates) {
				e.State = st
			}
		case "context":
			e.Context, _ = d.str(v, "context")
		case "description":
			e.Description, _ = d.str(v, "description")
		case "tag":
			e.Tag = d.effectValue(v, "tag")
		case "files":
			e.Files = d.strList(v, "files", false)
		case "generate_notes":
			e.GenerateNotes, _ = d.boolean(v, "generate_notes")
		case "draft":
			e.Draft, _ = d.boolean(v, "draft")
		case "prerelease":
			e.Prerelease, _ = d.boolean(v, "prerelease")
		}
	}
	for _, r := range table.required {
		if !present[r] {
			d.report("PF111", step, "", "effect %q is missing required key %q", name, r)
		}
	}
	return e
}

// labels decodes either a list of literal labels or { from_comment_args: true }.
func (d *decoder) labels(n *yaml.Node) ([]string, bool) {
	if n.Kind == yaml.MappingNode {
		fs := d.fields(n, "labels", []string{"from_comment_args"}, []string{"from_comment_args"}, "PF110")
		if len(fs) == 0 {
			return nil, false
		}
		if v, ok := d.boolean(fs[0].val, "from_comment_args"); ok && !v {
			d.report("PF113", fs[0].val, "remove it and list labels instead", "from_comment_args must be true when set")
		}
		return nil, true
	}
	return d.strList(n, "labels", false), false
}

func (d *decoder) uses(n *yaml.Node) string {
	ref, ok := d.str(n, "uses")
	if !ok {
		return ""
	}
	hint := "use owner/repo@tag or owner/repo/path@tag, e.g. actions/checkout@v7"
	switch {
	case strings.HasPrefix(ref, "docker://"):
		d.report("PF123", n, hint, "docker:// actions are not supported")
	case strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../"):
		d.report("PF123", n, hint, "local actions (%s) are not supported", ref)
	case !usesRE.MatchString(ref) || strings.Contains(ref, ".."):
		d.report("PF123", n, hint, "invalid action reference %q", ref)
	case slices.Contains(branchRefs, strings.ToLower(ref[strings.LastIndexByte(ref, '@')+1:])):
		d.report("PF123", n, "reference a release tag, e.g. @v4; pinfold lock pins it to a commit", "branch and moving references are not accepted (%s)", ref)
	case hexSHARE.MatchString(ref[strings.LastIndexByte(ref, '@')+1:]):
		d.report("PF123", n, "reference a release tag; pinfold lock pins it to a commit and records it in pinfold.lock", "bare commit SHAs are not accepted (%s)", ref)
	default:
		return ref
	}
	return ""
}

// valueMap decodes env: or with: mappings of typed values.
func (d *decoder) valueMap(n *yaml.Node, what string, envNames bool) map[string]Value {
	if !d.isKind(n, yaml.MappingNode, what) {
		return nil
	}
	out := map[string]Value{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if len(k.Value) > maxKeyLen {
			d.report("PF113", k, "", "%s key is longer than %d characters", what, maxKeyLen)
			continue
		}
		if envNames && !envNameRE.MatchString(k.Value) {
			d.report("PF113", k, "use letters, digits and underscores, not starting with a digit", "invalid environment variable name %q", k.Value)
			continue
		}
		if !envNames && !inputNameRE.MatchString(k.Value) {
			d.report("PF113", k, "use letters, digits, underscores and hyphens", "invalid action input name %q", k.Value)
			continue
		}
		if val := d.value(v, fmt.Sprintf("%s %q", what, k.Value)); val != nil {
			out[k.Value] = *val
		}
	}
	return out
}

// value decodes a literal or a { context | secret | var | output: name } reference.
func (d *decoder) value(n *yaml.Node, what string) *Value {
	if n.Kind == yaml.ScalarNode && n.Tag != "!!null" {
		return &Value{Kind: Literal, Text: n.Value, Span: d.span(n)}
	}
	if n.Tag == "!!null" {
		d.report("PF112", n, "e.g. { context: issue.number }", "%s must not be empty", what)
		return nil
	}
	kinds := []string{string(Context), string(Secret), string(Var), string(Output)}
	fs := d.fields(n, what, kinds, nil, "PF110")
	if n.Kind == yaml.MappingNode && len(n.Content) != 2 {
		d.report("PF112", n, "e.g. { context: issue.number }", "%s must be a literal or exactly one of context, secret, var or output", what)
		return nil
	}
	if len(fs) != 1 {
		return nil
	}
	f := fs[0]
	name, ok := d.str(f.val, f.key+" name")
	if !ok {
		return nil
	}
	switch ValueKind(f.key) {
	case Context:
		if !slices.Contains(Contexts, name) {
			d.report("PF113", f.val, suggest(name, Contexts), "unknown context %q", name)
			return nil
		}
	case Secret, Var:
		if !envNameRE.MatchString(name) {
			d.report("PF113", f.val, "use letters, digits and underscores", "invalid %s name %q", f.key, name)
			return nil
		}
	case Output:
		if !outputRE.MatchString(name) {
			d.report("PF113", f.val, "write it as job.output, e.g. build.version", "invalid output reference %q", name)
			return nil
		}
	}
	return &Value{Kind: ValueKind(f.key), Text: name, Span: d.span(f.val)}
}

// condition decodes a structured if:. top is true for the job's own if:.
func (d *decoder) condition(n *yaml.Node, top bool) *Condition {
	c := &Condition{Span: d.span(n)}
	if !d.isKind(n, yaml.MappingNode, "a condition") {
		return c
	}
	if len(n.Content) != 2 {
		d.report("PF125", n, "combine several conditions with all: or any:", "a condition must have exactly one key")
		return c
	}
	k, v := n.Content[0], n.Content[1]
	c.Kind = k.Value
	switch k.Value {
	case "all", "any":
		if !d.isKind(v, yaml.SequenceNode, k.Value) {
			return c
		}
		if len(v.Content) == 0 {
			d.report("PF111", v, "", "%s must list at least one condition", k.Value)
		}
		for _, item := range v.Content {
			sub := d.condition(item, false)
			if sub.Kind == "maintainer" && (!top || k.Value != "all") {
				d.report("PF113", item, "put maintainer: true directly under the job's if: all:", "maintainer is only allowed at the top level of a job's if: all:")
			}
			if k.Value == "all" {
				c.All = append(c.All, sub)
			} else {
				c.Any = append(c.Any, sub)
			}
		}
	case "event", "branch", "tag", "label", "actor":
		c.Values = d.strList(v, k.Value, true)
		if k.Value == "event" {
			for i, e := range c.Values {
				if _, ok := Triggers[e]; !ok {
					d.report("PF113", itemAt(v, i), suggest(e, sortedKeys(Triggers)), "unknown event %q", e)
				}
			}
		}
	case "association":
		c.Values = d.strList(v, k.Value, true)
		for i, a := range c.Values {
			d.oneOf(itemAt(v, i), "author association", a, associations)
		}
	case "status":
		if s, ok := d.str(v, "status"); ok && d.oneOf(v, "status", s, statusValues) {
			c.Values = []string{s}
		}
	case "comment":
		for _, f := range d.fields(v, "comment condition", []string{"starts_with"}, []string{"starts_with"}, "PF110") {
			if s, ok := d.str(f.val, "starts_with"); ok {
				c.Values = []string{s}
			}
		}
	case "maintainer":
		if v.Kind == yaml.MappingNode {
			c.Roles = d.customRoles(v)
		} else if b, ok := d.boolean(v, "maintainer"); ok && !b {
			d.report("PF113", v, "remove the condition instead", "maintainer can only be true")
		}
		c.Flag = true
		if top {
			d.report("PF113", n, "write if: { all: [ { maintainer: true }, ... ] }", "maintainer is only allowed at the top level of a job's if: all:")
		}
	default:
		d.report("PF125", k, suggest(k.Value, conditionKeys), "unknown condition %q", k.Value)
	}
	return c
}

// GateRoles are the repository roles the maintainer gate always allows (design §4).
var GateRoles = []string{"admin", "maintain", "write"}

var roleNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,99}$`)

// customRoles decodes maintainer: { custom_roles: [...] } (#56). The gate
// matches role_name exactly, so names are checked for shape only; read and
// triage are refused because nearly anyone can hold them.
func (d *decoder) customRoles(n *yaml.Node) []string {
	var out []string
	for _, f := range d.fields(n, "maintainer", []string{"custom_roles"}, []string{"custom_roles"}, "PF110") {
		if !d.isKind(f.val, yaml.SequenceNode, "custom_roles") {
			continue
		}
		if len(f.val.Content) == 0 {
			d.report("PF111", f.val, "write maintainer: true instead", "custom_roles must list at least one role")
		}
		for _, item := range f.val.Content {
			r, ok := d.str(item, "a custom role")
			switch {
			case !ok:
			case strings.EqualFold(r, "read") || strings.EqualFold(r, "triage"):
				d.report("PF113", item, "list only custom roles meant to approve; admin, maintain and write are always allowed", "role %q can't pass the maintainer gate: anyone with that access could", r)
			case slices.ContainsFunc(GateRoles, func(g string) bool { return strings.EqualFold(g, r) }):
				d.report("PF113", item, "remove it", "role %q is always allowed", r)
			case !roleNameRE.MatchString(r):
				d.report("PF113", item, "use the role's name as shown in the organisation's repository roles", "invalid role name %q", r)
			case slices.Contains(out, r):
				d.report("PF113", item, "remove the duplicate", "role %q is listed twice", r)
			default:
				out = append(out, r)
			}
		}
	}
	return out
}

// itemAt returns the i-th entry of a list node, or the node itself for a scalar shorthand.
func itemAt(n *yaml.Node, i int) *yaml.Node {
	if n.Kind == yaml.SequenceNode && i < len(n.Content) {
		return n.Content[i]
	}
	return n
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// effectValue decodes an effect's target, sha or tag. A secret there would be
// written into a public tag, status or API call, so only literals, contexts
// and outputs are accepted (external audit I5).
func (d *decoder) effectValue(n *yaml.Node, what string) *Value {
	v := d.value(n, what)
	if v != nil && v.Kind == Secret {
		d.report("PF112", n, "use a literal, a context or an output", "%s can't be a secret: the effect would publish it", what)
		return nil
	}
	return v
}
