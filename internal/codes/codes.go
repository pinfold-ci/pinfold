// Package codes is the catalogue of diagnostic codes and their explanations.
package codes

import (
	"embed"
	"fmt"
	"slices"
	"strings"

	"github.com/pinfold-ci/pinfold/internal/diag"
)

//go:embed explain/*.md
var pages embed.FS

// Entry describes one diagnostic code.
type Entry struct {
	Severity diag.Severity
	Title    string
}

// Lookup returns the catalogue entry for code.
func Lookup(code string) (Entry, bool) {
	e, ok := catalogue[code]
	return e, ok
}

// All returns every code in sorted order.
func All() []string {
	out := make([]string, 0, len(catalogue))
	for c := range catalogue {
		out = append(out, c)
	}
	slices.Sort(out)
	return out
}

// Explain returns the Markdown explanation for code.
func Explain(code string) (string, error) {
	code = strings.ToUpper(code)
	if _, ok := catalogue[code]; !ok {
		return "", fmt.Errorf("unknown diagnostic code %q", code)
	}
	b, err := pages.ReadFile("explain/" + code + ".md")
	if err != nil {
		return "", fmt.Errorf("no explanation for %s: %w", code, err)
	}
	return string(b), nil
}

// New builds a diagnostic for code with the catalogue's severity. It panics on an
// unknown code: that is a compiler bug, and tests exercise every call site.
func New(code string, span diag.Span, msg, hint string) diag.Diagnostic {
	e, ok := catalogue[code]
	if !ok {
		panic("codes: unknown diagnostic code " + code)
	}
	return diag.Diagnostic{Code: code, Severity: e.Severity, Span: span, Message: msg, Hint: hint}
}
