// Package search implements the query engine for projection nodes.
package search

import "strings"

// Levenshtein computes the edit distance between two strings.
func Levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Use single-row DP for space efficiency
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)

	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(
				prev[j]+1,      // deletion
				curr[j-1]+1,    // insertion
				prev[j-1]+cost, // substitution
			)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

// Similarity returns a normalized similarity score (0.0 to 1.0).
func Similarity(a, b string) float64 {
	dist := Levenshtein(a, b)
	maxLen := max(len(a), len(b))
	if maxLen == 0 {
		return 1.0
	}
	return 1.0 - float64(dist)/float64(maxLen)
}

// Fuzzy returns true if pattern is a subsequence of str (case-insensitive with CamelCase boost).
func Fuzzy(str, pattern string) bool {
	if str == "" || pattern == "" {
		return false
	}

	i := 0 // pattern index
	j := 0 // str index

	for i < len(pattern) && j < len(str) {
		pc := pattern[i]
		sc := str[j]

		// CamelCase boost: uppercase pattern char must match uppercase str char
		if pc >= 'A' && pc <= 'Z' {
			if sc == pc {
				i++
				j++
				continue
			}
		} else {
			// Normal case-insensitive match
			if toLower(sc) == toLower(pc) {
				i++
				j++
				continue
			}
		}
		j++
	}

	return i == len(pattern)
}

// FuzzyScore returns a match quality score (0.0 = no match, higher = better).
func FuzzyScore(haystack, needle string) float64 {
	if haystack == "" || needle == "" {
		return 0
	}

	h := strings.ToLower(haystack)
	n := strings.ToLower(needle)

	// Exact match — highest
	if h == n {
		return 1.0
	}

	// Prefix match — very high
	if strings.HasPrefix(h, n) {
		return 0.9
	}

	// Subsequence check
	if !Fuzzy(haystack, needle) {
		return 0
	}

	// Score based on similarity
	return Similarity(h, n) * 0.8
}

// TypeComponentScore scores a type label against a search term using component matching.
func TypeComponentScore(typeLabel, needle string) float64 {
	if typeLabel == "" || needle == "" {
		return 0
	}
	components := strings.Split(strings.ToLower(typeLabel), ",")
	n := strings.ToLower(needle)
	var best float64
	for _, comp := range components {
		comp = strings.TrimSpace(comp)
		if comp == n {
			return 1.0
		}
		if strings.HasPrefix(comp, n) {
			if 0.9 > best {
				best = 0.9
			}
		}
	}
	return best
}

// DocScore scores free-text prose (such as a "doc" field) against a search
// term using substring containment rather than pure fuzzy subsequence
// matching, since Levenshtein-based similarity degrades quickly on long
// sentences and would rarely surface useful doc matches otherwise.
func DocScore(doc, needle string) float64 {
	if doc == "" || needle == "" {
		return 0
	}
	d := strings.ToLower(doc)
	n := strings.ToLower(needle)

	if d == n {
		return 1.0
	}
	if !strings.Contains(d, n) {
		return 0
	}
	// Whole-word containment scores higher than a mid-word substring match.
	for _, word := range strings.Fields(d) {
		word = strings.Trim(word, ".,;:!?()[]{}\"'")
		if word == n {
			return 0.85
		}
	}
	return 0.6
}

func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
