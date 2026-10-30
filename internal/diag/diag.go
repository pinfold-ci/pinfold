// Package diag defines compiler diagnostics: a stable code, a source position,
// a message and a fix hint.
package diag

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Severity is how serious a diagnostic is.
type Severity int

const (
	Error Severity = iota
	Warning
)

func (s Severity) String() string {
	if s == Warning {
		return "warning"
	}
	return "error"
}

// MarshalText encodes the severity as "error" or "warning".
func (s Severity) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText decodes "error" or "warning".
func (s *Severity) UnmarshalText(b []byte) error {
	switch string(b) {
	case "error":
		*s = Error
	case "warning":
		*s = Warning
	default:
		return fmt.Errorf("unknown severity %q", b)
	}
	return nil
}

// Span is a position in a source file. Line and Col are 1-based; zero means unknown.
type Span struct {
	File string `json:"file"`
	Line int    `json:"line,omitempty"`
	Col  int    `json:"col,omitempty"`
}

func (s Span) String() string {
	switch {
	case s.Line == 0:
		return s.File
	case s.Col == 0:
		return fmt.Sprintf("%s:%d", s.File, s.Line)
	default:
		return fmt.Sprintf("%s:%d:%d", s.File, s.Line, s.Col)
	}
}

// Diagnostic is one compiler message.
type Diagnostic struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Span     Span     `json:"span"`
	Message  string   `json:"message"`
	Hint     string   `json:"hint,omitempty"`
}

// List collects diagnostics.
type List []Diagnostic

// Add appends a diagnostic.
func (l *List) Add(d Diagnostic) { *l = append(*l, d) }

// HasErrors reports whether any diagnostic is an error.
func (l List) HasErrors() bool {
	return slices.ContainsFunc(l, func(d Diagnostic) bool { return d.Severity == Error })
}

// Sort orders diagnostics by file, line, column, code and message, so output is deterministic.
func (l List) Sort() {
	slices.SortStableFunc(l, func(a, b Diagnostic) int {
		return cmp.Or(
			cmp.Compare(a.Span.File, b.Span.File),
			cmp.Compare(a.Span.Line, b.Span.Line),
			cmp.Compare(a.Span.Col, b.Span.Col),
			cmp.Compare(a.Code, b.Code),
			cmp.Compare(a.Message, b.Message),
		)
	})
}

// WriteText renders diagnostics for humans, one per line, with an indented hint line.
func (l List) WriteText(w io.Writer) error {
	var b strings.Builder
	for _, d := range l {
		fmt.Fprintf(&b, "%s: %s %s: %s\n", d.Span, d.Severity, d.Code, d.Message)
		if d.Hint != "" {
			fmt.Fprintf(&b, "    hint: %s\n", d.Hint)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// WriteJSON renders diagnostics as a JSON array (an empty list is "[]").
func (l List) WriteJSON(w io.Writer) error {
	if l == nil {
		l = List{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(l)
}
