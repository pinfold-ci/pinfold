package effects

import (
	"fmt"
	"slices"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

// Check validates effect fields that depend on the rest of the job.
func Check(s *spec.Spec) diag.List {
	var diags diag.List
	for _, w := range s.Workflows {
		for _, j := range w.Jobs {
			for _, st := range j.Steps {
				e := st.Effect
				if st.Kind != spec.StepEffect || e == nil {
					continue
				}
				if e.Name == "comment" {
					for _, p := range Placeholders(e.Body) {
						if !slices.Contains(spec.Contexts, p) {
							diags.Add(codes.New("PF122", e.Span, fmt.Sprintf("comment body uses unknown placeholder {{ %s }}", p),
								"placeholders name contexts, e.g. {{ actor }} or {{ issue.number }}"))
						}
					}
				}
				if e.FromCommentArgs {
					if !hasCommentPrefix(j.If) {
						diags.Add(codes.New("PF122", e.Span, fmt.Sprintf("effect %s takes labels from the comment, but job %q has no comment.starts_with condition", e.Name, j.Name),
							`add if: { all: [ { comment: { starts_with: "/label " } }, { maintainer: true } ] }`))
					}
					if len(e.Allowed) == 0 {
						diags.Add(codes.New("PF122", e.Span, fmt.Sprintf("effect %s takes labels from the comment, so it needs an allowed list", e.Name),
							"add allowed: [ ... ] with the labels people may set"))
					}
				}
			}
		}
	}
	diags.Sort()
	return diags
}

func hasCommentPrefix(c *spec.Condition) bool {
	if c == nil {
		return false
	}
	if c.Kind == "comment" && len(c.Values) == 1 && c.Values[0] != "" {
		return true
	}
	return slices.ContainsFunc(append(append([]*spec.Condition{}, c.All...), c.Any...), hasCommentPrefix)
}
