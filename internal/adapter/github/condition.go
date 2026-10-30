package github

import (
	"strings"

	"github.com/pinfold-ci/pinfold/internal/ir"
)

// condition renders a structured condition as a GitHub expression. Values are
// always single-quoted string literals (” escaped), never spliced as code.
// maintainer is handled by a generated step, so it contributes nothing here.
func condition(c *ir.Condition) string {
	switch c.Kind {
	case "all", "any":
		subs := c.All
		op := " && "
		if c.Kind == "any" {
			subs, op = c.Any, " || "
		}
		var parts []string
		for _, s := range subs {
			if e := condition(s); e != "" {
				parts = append(parts, "("+e+")")
			}
		}
		return strings.Join(parts, op)
	case "event":
		return anyOf(c.Values, func(v string) string { return "github.event_name == " + lit(eventName(v)) })
	case "branch":
		return anyOf(c.Values, func(v string) string { return "(github.base_ref || github.ref_name) == " + lit(v) })
	case "tag":
		return anyOf(c.Values, func(v string) string {
			return "(startsWith(github.ref, 'refs/tags/') && github.ref_name == " + lit(v) + ")"
		})
	case "label":
		return anyOf(c.Values, func(v string) string {
			return "contains(github.event.issue.labels.*.name, " + lit(v) + ") || contains(github.event.pull_request.labels.*.name, " + lit(v) + ")"
		})
	case "actor":
		return anyOf(c.Values, func(v string) string { return "github.actor == " + lit(v) })
	case "association":
		return anyOf(c.Values, func(v string) string {
			return "(github.event.comment.author_association || github.event.review.author_association || github.event.issue.author_association || github.event.pull_request.author_association) == " + lit(v)
		})
	case "comment":
		return "startsWith(github.event.comment.body, " + lit(c.Values[0]) + ")"
	case "status":
		return map[string]string{"success": "success()", "failure": "failure()", "always": "always()", "not_cancelled": "!cancelled()"}[c.Values[0]]
	}
	return "" // maintainer
}

func eventName(v string) string {
	if v == "manual" {
		return "workflow_dispatch"
	}
	return v
}

func anyOf(vals []string, f func(string) string) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = f(v)
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, ") || (") + ")"
}

// lit quotes s as a GitHub expression string literal.
func lit(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func hasMaintainer(c *ir.Condition) bool {
	if c == nil {
		return false
	}
	if c.Kind == "maintainer" {
		return true
	}
	for _, s := range append(append([]*ir.Condition{}, c.All...), c.Any...) {
		if hasMaintainer(s) {
			return true
		}
	}
	return false
}
