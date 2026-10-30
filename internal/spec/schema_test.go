package spec

import (
	"encoding/json"
	"slices"
	"testing"
)

type jsonSchema struct {
	Properties map[string]*jsonSchema `json:"properties"`
	Required   []string               `json:"required"`
	Defs       map[string]*jsonSchema `json:"$defs"`
	OneOf      []*jsonSchema          `json:"oneOf"`
	Enum       []string               `json:"enum"`
	Const      any                    `json:"const"`
	Items      *jsonSchema            `json:"items"`
}

func (s *jsonSchema) keys() []string {
	out := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func sorted(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}

func sameSet(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(sorted(got), sorted(want)) {
		t.Errorf("%s: schema has %v, decoder has %v", what, sorted(got), sorted(want))
	}
}

// TestSchemaMatchesDecoder fails when the decoder's key tables and the JSON
// Schema disagree, so editor hints can't drift from what the compiler accepts.
func TestSchemaMatchesDecoder(t *testing.T) {
	var s jsonSchema
	if err := json.Unmarshal(Schema, &s); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	d := s.Defs
	sameSet(t, "top-level keys", s.keys(), topKeys)
	sameSet(t, "top-level required", s.Required, topKeys)
	sameSet(t, "workflow keys", d["workflow"].keys(), workflowKeys)
	sameSet(t, "job keys", d["job"].keys(), jobKeys)
	sameSet(t, "condition keys", d["condition"].keys(), conditionKeys)

	on := d["workflow"].Properties["on"]
	sameSet(t, "triggers", on.keys(), sortedKeys(Triggers))
	for event, keys := range Triggers {
		sameSet(t, "trigger "+event, on.Properties[event].keys(), keys)
	}

	perms := d["job"].Properties["permissions"]
	sameSet(t, "permission scopes", perms.keys(), sortedKeys(PermissionLevels))
	for scope, levels := range PermissionLevels {
		sameSet(t, "levels of "+scope, perms.Properties[scope].Enum, levels)
	}

	cond := d["condition"].Properties
	sameSet(t, "status values", cond["status"].Enum, statusValues)
	sameSet(t, "associations", cond["association"].OneOf[0].Enum, associations)

	sameSet(t, "contexts", d["value"].OneOf[1].Properties["context"].Enum, Contexts)

	var kinds, effects []string
	for _, alt := range d["step"].OneOf {
		if e, ok := alt.Properties["effect"]; ok {
			name, _ := e.Const.(string)
			effects = append(effects, name)
			var fields []string
			for _, k := range alt.keys() {
				if k != "name" && k != "effect" {
					fields = append(fields, k)
				}
			}
			sameSet(t, "fields of effect "+name, fields, effectFields[name].allowed)
			sameSet(t, "required fields of effect "+name, slices.DeleteFunc(slices.Clone(alt.Required), func(k string) bool { return k == "effect" }), effectFields[name].required)
			if name == "commit_status" {
				sameSet(t, "commit states", alt.Properties["state"].Enum, commitStates)
			}
			continue
		}
		for _, k := range alt.keys() {
			if slices.Contains(stepKinds, k) {
				kinds = append(kinds, k)
				var extra []string
				for _, k2 := range alt.keys() {
					if k2 != k && !slices.Contains(stepCommon, k2) {
						extra = append(extra, k2)
					}
				}
				want := map[string][]string{"checkout": nil, "run": {"env"}, "uses": {"with"}}[k]
				sameSet(t, k+" step extra keys", extra, want)
			}
		}
	}
	sameSet(t, "step kinds", append(kinds, "effect"), stepKinds)
	sameSet(t, "effects", effects, sortedKeys(effectFields))
}
