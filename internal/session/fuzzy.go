package session

import "strings"

// Targets is everything a session can be fuzzy-matched on.
func (s Session) Targets() []string {
	out := []string{s.Name, s.Slug, s.Group}
	out = append(out, s.Tags...)
	out = append(out, s.Endpoint())
	if s.Description != "" {
		out = append(out, s.Description)
	}
	return out
}

func (s Session) Matches(query string) bool {
	for _, t := range s.Targets() {
		if t == "" {
			continue
		}
		if _, ok := fuzzy(query, t); ok {
			return true
		}
	}
	return false
}

// fuzzy is a subsequence match: every rune of pattern must appear in target in
// order. Case-insensitive. The score rewards adjacency, so a contiguous match
// ranks above a scattered one.
func fuzzy(pattern, target string) (int, bool) {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	target = strings.ToLower(target)
	if pattern == "" {
		return 0, true
	}

	score := 0
	adjacent := 0
	pi := 0
	pr := []rune(pattern)
	for _, tr := range target {
		if pi >= len(pr) {
			break
		}
		if tr == pr[pi] {
			score += 1 + adjacent
			adjacent++
			pi++
		} else {
			adjacent = 0
		}
	}
	return score, pi == len(pr)
}
