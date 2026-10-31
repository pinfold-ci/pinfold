// Package permissions checks that every job declares the token permissions its
// steps need, and flags write permissions nothing uses (design §6, pass ⑦).
// Permissions are declared, never inferred (D-008).
package permissions

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pinfold-ci/pinfold/internal/codes"
	"github.com/pinfold-ci/pinfold/internal/diag"
	"github.com/pinfold-ci/pinfold/internal/ir"
	"github.com/pinfold-ci/pinfold/internal/registry"
	"github.com/pinfold-ci/pinfold/internal/spec"
)

// need is a requirement satisfied by any one of its capabilities.
type need struct {
	anyOf []string // "resource:access"
	by    string   // what needs it, for diagnostics
	span  diag.Span
}

// effectNeeds lists the capabilities each effect needs. Comment and label
// endpoints accept either issues or pull-requests write (GitHub REST docs,
// "Permissions required for GitHub Apps", checked 2026-10-31).
var effectNeeds = map[string][]string{
	"comment":       {"issues:write", "pr:write"},
	"add_labels":    {"issues:write", "pr:write"},
	"remove_labels": {"issues:write", "pr:write"},
	"commit_status": {"commit-statuses:write"},
	"release":       {"repo.contents:write"},
}

// scopeOf maps an abstract resource back to its GitHub scope, for messages.
func scopeOf(cap string) string {
	res, access, _ := strings.Cut(cap, ":")
	for scope, r := range ir.ScopeResources {
		if r == res {
			return scope + ": " + access
		}
	}
	return cap
}

// Check reports missing (PF401) and unneeded write (PF402) permissions. p must be ir.Lower(s).
func Check(s *spec.Spec, p *ir.Program) diag.List {
	var diags diag.List
	for wi, w := range s.Workflows {
		for ji, j := range w.Jobs {
			check(j, p.Workflows[wi].Jobs[ji], &diags)
		}
	}
	diags.Sort()
	return diags
}

func check(j *spec.Job, ij *ir.Job, diags *diag.List) {
	var needs []need
	for _, st := range j.Steps {
		switch st.Kind {
		case spec.StepCheckout:
			needs = append(needs, need{[]string{"repo.contents:read"}, "checkout", st.Span})
		case spec.StepUses:
			if e, reviewed, _ := registry.Lookup(st.Uses); e != nil && reviewed {
				for _, r := range e.Requires {
					needs = append(needs, need{[]string{r}, st.Uses, st.Span})
				}
			}
		case spec.StepEffect:
			if st.Effect != nil {
				if caps, ok := effectNeeds[st.Effect.Name]; ok {
					needs = append(needs, need{caps, "effect " + st.Effect.Name, st.Span})
				}
			}
		}
	}
	// Same-run artifact upload and download need no token permission.

	has := func(cap string) bool {
		res, access, _ := strings.Cut(cap, ":")
		return slices.ContainsFunc(ij.Capabilities, func(c ir.Capability) bool {
			return c.Resource == res && (string(c.Access) == access || (access == "read" && c.Access == ir.Write))
		})
	}
	needsWrite := map[string]bool{} // resource -> a satisfied requirement needs write
	for _, n := range needs {
		ok := false
		for _, cap := range n.anyOf {
			if has(cap) {
				ok = true
				res, access, _ := strings.Cut(cap, ":")
				needsWrite[res] = needsWrite[res] || access == "write"
			}
		}
		if !ok {
			var opts []string
			for _, cap := range n.anyOf {
				opts = append(opts, scopeOf(cap))
			}
			diags.Add(codes.New("PF401", n.span, fmt.Sprintf("job %q is missing permission %s, needed by %s", j.Name, strings.Join(opts, " or "), n.by),
				fmt.Sprintf("add it to the job: permissions: { %s }", opts[0])))
		}
	}
	for _, p := range j.Permissions {
		// Design §4: an unprivileged job gets contents: read at most. Other
		// read scopes (packages, security-events, ...) would hand code that may
		// be attacker-controlled more to read than the design promises.
		if ij.Zone != ir.ZonePrivileged && p.Level == "read" && p.Scope != "contents" {
			diags.Add(codes.New("PF403", p.Span, fmt.Sprintf("job %q is unprivileged, so it may declare contents: read at most, not %s: read", j.Name, p.Scope),
				"remove it, or move the work that needs it to a privileged job (which can't run your scripts)"))
		}
		if p.Level == "write" && !needsWrite[ir.ScopeResources[p.Scope]] {
			diags.Add(codes.New("PF402", p.Span, fmt.Sprintf("job %q declares %s: write, but no step in it needs write access", j.Name, p.Scope),
				"remove it or lower it to read: every write permission (and an OIDC token) makes the job privileged"))
		}
	}
}
