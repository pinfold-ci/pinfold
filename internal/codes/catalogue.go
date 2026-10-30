package codes

import "github.com/pinfold-ci/pinfold/internal/diag"

// catalogue lists every diagnostic code. Each entry needs a page in explain/ (enforced by tests).
var catalogue = map[string]Entry{
	"PF100": {Severity: diag.Error, Title: "Can't read the spec file"},
	"PF101": {Severity: diag.Error, Title: "YAML syntax error"},
	"PF102": {Severity: diag.Error, Title: "Forbidden YAML feature"},
	"PF103": {Severity: diag.Error, Title: "Duplicate key"},
	"PF104": {Severity: diag.Error, Title: "Exactly one YAML document required"},
	"PF105": {Severity: diag.Error, Title: "Encoding or size limit"},
	"PF110": {Severity: diag.Error, Title: "Unknown key"},
	"PF111": {Severity: diag.Error, Title: "Missing required key"},
	"PF112": {Severity: diag.Error, Title: "Wrong type"},
	"PF113": {Severity: diag.Error, Title: "Invalid value"},
	"PF114": {Severity: diag.Error, Title: "Invalid name"},
	"PF115": {Severity: diag.Error, Title: "Unsupported spec version"},
	"PF120": {Severity: diag.Error, Title: "Unknown or unsupported trigger"},
	"PF121": {Severity: diag.Error, Title: "A step needs exactly one kind"},
	"PF122": {Severity: diag.Error, Title: "Unknown effect or effect field"},
	"PF123": {Severity: diag.Error, Title: "Invalid action reference"},
	"PF124": {Severity: diag.Error, Title: "Reference to an unknown job"},
	"PF125": {Severity: diag.Error, Title: "Unknown condition"},
	"PF130": {Severity: diag.Error, Title: "Invalid permission"},
	"PF201": {Severity: diag.Error, Title: "`${{` isn't allowed in the spec"},
}
