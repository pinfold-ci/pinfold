package github

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/pinfold-ci/pinfold/internal/effects"
	"github.com/pinfold-ci/pinfold/internal/emit"
	"github.com/pinfold-ci/pinfold/internal/ir"
	"github.com/pinfold-ci/pinfold/internal/spec"
	"github.com/pinfold-ci/pinfold/internal/taint"
)

// effectStep lowers an effect to one pinned actions/github-script step
// (design §5). The script is a compiler-generated literal; every input reaches
// it through env: untrusted values as T_*, configuration as PINFOLD_*.
func effectStep(w *ir.Workflow, j *ir.Job, s *ir.Step) emit.Map {
	e := s.Effect
	script, err := effects.Script(e.Name)
	if err != nil {
		panic(err) // the decoder only accepts known effects
	}
	env := map[string]ir.Value{}
	lit := func(k, v string) { env["PINFOLD_"+k] = ir.Value{Kind: spec.Literal, Text: v} }
	put := func(k string, v *ir.Value) {
		if v == nil {
			return
		}
		name := "PINFOLD_" + k
		if taint.Tainted(*v, w.Class) {
			name = "T_" + k
		}
		env[name] = *v
	}
	jsonLit := func(k string, v any) {
		b, _ := json.Marshal(v)
		lit(k, string(b))
	}
	switch e.Name {
	case "comment":
		put("TARGET", e.Target)
		lit("BODY", e.Body)
		if e.Sticky != "" {
			lit("STICKY", e.Sticky)
		}
		vars := effects.Placeholders(e.Body)
		if len(vars) > 0 {
			jsonLit("VARS", vars)
		}
		for i, name := range vars {
			v := ir.Value{Kind: spec.Context, Text: name}
			put(fmt.Sprintf("V%d", i), &v)
		}
	case "add_labels", "remove_labels":
		put("TARGET", e.Target)
		mode := "add"
		if e.Name == "remove_labels" {
			mode = "remove"
		}
		lit("MODE", mode)
		if e.FromCommentArgs {
			lit("FROM_COMMENT", "true")
			lit("PREFIX", commentPrefix(j.If))
			c := ir.Value{Kind: spec.Context, Text: "comment.body"}
			put("COMMENT", &c)
		} else {
			jsonLit("LABELS", e.Labels)
		}
		if len(e.Allowed) > 0 {
			jsonLit("ALLOWED", e.Allowed)
		}
		if e.Max > 0 {
			lit("MAX", strconv.Itoa(e.Max))
		}
	case "commit_status":
		put("SHA", e.SHA)
		lit("STATE", e.State)
		lit("CONTEXT", e.Context)
		if e.Description != "" {
			lit("DESCRIPTION", e.Description)
		}
	case "release":
		put("TAG", e.Tag)
		if len(e.Files) > 0 {
			jsonLit("FILES", e.Files)
		}
		lit("GENERATE_NOTES", strconv.FormatBool(e.GenerateNotes))
		lit("DRAFT", strconv.FormatBool(e.Draft))
		lit("PRERELEASE", strconv.FormatBool(e.Prerelease))
	}
	gs := generatedActions["script"]
	return emit.Map{
		{Key: "name", Value: emit.Str("pinfold: " + e.Name), Comment: generatedMarker + s.ID},
		{Key: "uses", Value: emit.Str(gs.ref), Comment: gs.tag},
		{Key: "env", Value: values(env)},
		{Key: "with", Value: emit.Map{{Key: "script", Value: emit.Str(script)}}},
	}
}

// commentPrefix returns the comment.starts_with value of a job condition, if any.
func commentPrefix(c *ir.Condition) string {
	if c == nil {
		return ""
	}
	if c.Kind == "comment" && len(c.Values) == 1 {
		return c.Values[0]
	}
	for _, sub := range append(append([]*ir.Condition{}, c.All...), c.Any...) {
		if p := commentPrefix(sub); p != "" {
			return p
		}
	}
	return ""
}
