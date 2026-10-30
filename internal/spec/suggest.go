package spec

import (
	"fmt"
	"slices"
)

// synonyms maps GitHub Actions spellings to the Pinfold key that replaces them.
var synonyms = map[string]string{
	"runs-on":           "runner",
	"workflow_dispatch": "manual",
	"timeout":           "timeout-minutes",
}

// suggest returns a "did you mean" hint for an unknown key, or "".
func suggest(key string, allowed []string) string {
	if s, ok := synonyms[key]; ok && slices.Contains(allowed, s) {
		return fmt.Sprintf("did you mean %q? (Pinfold's name for %q)", s, key)
	}
	best, bestDist := "", 3
	for _, a := range allowed {
		if d := distance(key, a); d < bestDist {
			best, bestDist = a, d
		}
	}
	if best == "" {
		return ""
	}
	return fmt.Sprintf("did you mean %q?", best)
}

// distance is the Levenshtein edit distance between a and b.
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
