package registry

import (
	"slices"
	"strings"
	"testing"

	"github.com/pinfold-ci/pinfold/internal/ir"
)

func TestEntriesAreComplete(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 8 {
		t.Errorf("registry has %d entries, want at least 8", len(all))
	}
	var resources []string
	for _, r := range ir.ScopeResources {
		resources = append(resources, r)
	}
	for name, e := range all {
		if !strings.HasPrefix(e.Reviewed.Source, "https://github.com/"+name+"/") {
			t.Errorf("%s: reviewed source %q isn't a link into the action's repository", name, e.Reviewed.Source)
		}
		if len(e.Reviewed.Commit) != 40 || e.Reviewed.Date == "" || e.Reviewed.Notes == "" {
			t.Errorf("%s: review needs a 40-hex commit, a date and notes", name)
		}
		if len(e.Majors) == 0 {
			t.Errorf("%s: no reviewed major versions", name)
		}
		if e.RestoresCache == "" {
			t.Errorf("%s: restores_cache must be never, always or an input name", name)
		}
		for _, req := range e.Requires {
			res, access, ok := strings.Cut(req, ":")
			if !ok || !slices.Contains(resources, res) || (access != "read" && access != "write") {
				t.Errorf("%s: invalid capability %q", name, req)
			}
		}
		if e.Container != nil && !strings.HasPrefix(e.Container.Digest, "sha256:") {
			t.Errorf("%s: container must be pinned by sha256 digest", name)
		}
		if e.PrivilegedOK && e.RestoresCache == "always" {
			t.Errorf("%s: an action that always restores a cache can't be privileged_ok", name)
		}
	}
}

func TestLookup(t *testing.T) {
	tests := []struct {
		uses            string
		known, reviewed bool
		privilegedOK    bool
	}{
		{"actions/checkout@v7", true, true, true},
		{"actions/checkout@v7.0.1", true, true, true},
		{"actions/checkout@v6", true, false, true},
		{"actions/github-script@v9", true, true, false},
		{"pypa/gh-action-pypi-publish@v1.14.3", true, true, false},
		{"evil/action@v1", false, false, false},
	}
	for _, tt := range tests {
		e, reviewed, err := Lookup(tt.uses)
		if err != nil {
			t.Fatal(err)
		}
		if (e != nil) != tt.known || reviewed != tt.reviewed || (e != nil && e.PrivilegedOK != tt.privilegedOK) {
			t.Errorf("Lookup(%q) = known %v reviewed %v, want known %v reviewed %v privileged_ok %v", tt.uses, e != nil, reviewed, tt.known, tt.reviewed, tt.privilegedOK)
		}
	}
}

func TestDigestIsStable(t *testing.T) {
	if a, b := Digest(), Digest(); a != b || !strings.HasPrefix(a, "sha256:") || len(a) != 71 {
		t.Errorf("Digest() = %q / %q", a, b)
	}
}
