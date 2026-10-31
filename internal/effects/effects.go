// Package effects holds the compiler-generated scripts for effects (design §5):
// each effect becomes one pinned actions/github-script step whose script is a
// literal, with every input read from the environment.
package effects

import _ "embed"

// Sanitizer is the JavaScript sanitiser inlined into every effect script.
//
//go:embed js/sanitize.js
var Sanitizer string
