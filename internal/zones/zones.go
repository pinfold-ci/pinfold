// Package zones enforces the trust-zone rules of design §4: what a job may
// contain given the credentials it holds. It also checks registry-driven
// constraints on actions.
package zones

import (
	"fmt"
	"slices"
	"sort"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/registry"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

// Check reports trust-zone violations in a decoded spec.
func Check(s *spec.Spec) diag.List {
	var diags diag.List
	for _, w := range s.Workflows {
		for _, j := range w.Jobs {
			for _, st := range j.Steps {
				if st.Kind == spec.StepUses {
					checkContainerInputs(st, &diags)
				}
			}
		}
	}
	diags.Sort()
	return diags
}

// checkContainerInputs rejects with: inputs that a container-lowered action
// (D-035) doesn't map, so nothing the user wrote is silently dropped.
func checkContainerInputs(st *spec.Step, diags *diag.List) {
	e, reviewed, err := registry.Lookup(st.Uses)
	if err != nil || e == nil || !reviewed || e.Container == nil {
		return
	}
	var allowed []string
	for k := range e.Container.Inputs {
		allowed = append(allowed, k)
	}
	sort.Strings(allowed)
	var keys []string
	for k := range st.With {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := st.With[k]
		if !slices.Contains(allowed, k) {
			diags.Add(codes.New("PF306", v.Span, fmt.Sprintf("input %q isn't supported for %s, which Pinfold runs as a digest-pinned container", k, st.Uses), fmt.Sprintf("supported inputs: %v", allowed)))
			continue
		}
		// The image runs with the job's credentials (OIDC for PyPI). Event data
		// must not steer it, e.g. a repository-url taken from an issue title.
		if v.Kind != spec.Literal && v.Kind != spec.Secret {
			diags.Add(codes.New("PF306", v.Span, fmt.Sprintf("input %q of %s must be a literal or a secret, not a %s", k, st.Uses, v.Kind), "write the value out, or use { secret: NAME }"))
		}
	}
}
