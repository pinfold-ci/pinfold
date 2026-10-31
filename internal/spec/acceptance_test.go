package spec

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"go.yaml.in/yaml/v3"
)

// guaranteeRows are the ✅ rows of design §2.
var guaranteeRows = []string{
	"G1-privileged-jobs-declarative", "G2-no-tainted-expressions", "G3-tainted-by-default", "G4-checkout-hardening",
	"G5-no-cache-with-credentials", "G6-artifact-digest-binding", "G7-exact-permissions", "G8-pinned-actions-match-lock",
}

// irConstructs derives the construct list from the decoder's own tables.
func irConstructs() []string {
	var out []string
	for t := range Triggers {
		out = append(out, "trigger."+t)
	}
	for _, k := range jobKeys {
		if k != "steps" {
			out = append(out, "job."+k)
		}
	}
	for _, k := range stepKinds {
		if k != "effect" {
			out = append(out, "step."+k)
		}
	}
	for e := range effectFields {
		out = append(out, "effect."+e)
	}
	out = append(out, "gate.maintainer")
	slices.Sort(out)
	return out
}

func TestAcceptanceFixturesCoverTheIR(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "ir-acceptance", "*.yml"))
	if len(files) != 3 {
		t.Fatalf("found %d platform files, want 3 (forgejo, gitlab, jenkins)", len(files))
	}
	allowed := []string{"unverified", "yes", "no", "partial"}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var fx struct {
			Platform   string
			Constructs map[string]struct{ Maps, Question string }
			Guarantees map[string]struct{ Holds, Note string }
		}
		if err := yaml.Unmarshal(raw, &fx); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		var got []string
		for k, c := range fx.Constructs {
			got = append(got, k)
			if !slices.Contains(allowed, c.Maps) || c.Question == "" {
				t.Errorf("%s: construct %s has maps=%q question=%q", f, k, c.Maps, c.Question)
			}
		}
		slices.Sort(got)
		if want := irConstructs(); !slices.Equal(got, want) {
			t.Errorf("%s constructs differ from the IR\n got: %v\nwant: %v", f, got, want)
		}
		var gs []string
		for k, g := range fx.Guarantees {
			gs = append(gs, k)
			if !slices.Contains(allowed, g.Holds) || g.Note == "" {
				t.Errorf("%s: guarantee %s has holds=%q note=%q", f, k, g.Holds, g.Note)
			}
		}
		slices.Sort(gs)
		if want := slices.Sorted(slices.Values(guaranteeRows)); !slices.Equal(gs, want) {
			t.Errorf("%s guarantees differ from design §2\n got: %v\nwant: %v", f, gs, want)
		}
	}
}
