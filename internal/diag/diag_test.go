package diag

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSpanString(t *testing.T) {
	tests := []struct {
		span Span
		want string
	}{
		{Span{File: "pinfold.yml"}, "pinfold.yml"},
		{Span{File: "pinfold.yml", Line: 3}, "pinfold.yml:3"},
		{Span{File: "pinfold.yml", Line: 3, Col: 7}, "pinfold.yml:3:7"},
	}
	for _, tt := range tests {
		if got := tt.span.String(); got != tt.want {
			t.Errorf("Span%+v.String() = %q, want %q", tt.span, got, tt.want)
		}
	}
}

func TestSortIsDeterministic(t *testing.T) {
	l := List{
		{Code: "PF110", Span: Span{File: "b.yml", Line: 1, Col: 1}},
		{Code: "PF112", Span: Span{File: "a.yml", Line: 2, Col: 1}},
		{Code: "PF110", Span: Span{File: "a.yml", Line: 2, Col: 1}},
		{Code: "PF101", Span: Span{File: "a.yml", Line: 1, Col: 9}},
	}
	l.Sort()
	want := []string{"a.yml:1:9 PF101", "a.yml:2:1 PF110", "a.yml:2:1 PF112", "b.yml:1:1 PF110"}
	for i, d := range l {
		if got := d.Span.String() + " " + d.Code; got != want[i] {
			t.Errorf("position %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestHasErrors(t *testing.T) {
	if (List{{Severity: Warning}}).HasErrors() {
		t.Error("a list with only warnings reported errors")
	}
	if !(List{{Severity: Warning}, {Severity: Error}}).HasErrors() {
		t.Error("a list with an error reported none")
	}
}

func TestWriteText(t *testing.T) {
	l := List{
		{Code: "PF110", Severity: Error, Span: Span{File: "pinfold.yml", Line: 4, Col: 5}, Message: `unknown key "runs-on"`, Hint: `did you mean "runner"?`},
		{Code: "PF402", Severity: Warning, Span: Span{File: "pinfold.yml"}, Message: "write scope not needed"},
	}
	var buf bytes.Buffer
	if err := l.WriteText(&buf); err != nil {
		t.Fatal(err)
	}
	want := "pinfold.yml:4:5: error PF110: unknown key \"runs-on\"\n" +
		"    hint: did you mean \"runner\"?\n" +
		"pinfold.yml: warning PF402: write scope not needed\n"
	if buf.String() != want {
		t.Errorf("WriteText =\n%s\nwant\n%s", buf.String(), want)
	}
}

func TestWriteJSONRoundTrip(t *testing.T) {
	in := List{{Code: "PF103", Severity: Warning, Span: Span{File: "x.yml", Line: 2, Col: 3}, Message: "m", Hint: "h"}}
	var buf bytes.Buffer
	if err := in.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	var out List
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0] != in[0] {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
}

func TestWriteJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := List(nil).WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "[]\n" {
		t.Errorf("empty list = %q, want %q", buf.String(), "[]\n")
	}
}
