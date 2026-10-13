// Source: getarcaneapp/kit/pkg/utils/utils.go
// Revision: 9c61043fc52d1977d644f075d9c168335c3a6df7
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

// Package utils provides focused helpers for working with string slices
// whose entries may be blank.
package utils

import (
	"slices"
	"strings"

	kit "go.getarcane.app/kit/pkg"
)

// NormalizeSet trims every entry, drops blanks and duplicates, and returns
// the remaining entries sorted. The input slice is not modified.
func NormalizeSet(values []string) []string {
	out := kit.Unique(kit.TrimNonEmpty(values))
	slices.Sort(out)
	return out
}

// CountNonEmpty returns the number of entries that are not blank after
// trimming whitespace.
func CountNonEmpty(values []string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

// HasNonEmpty reports whether any entry is not blank after trimming
// whitespace.
func HasNonEmpty(values []string) bool {
	return CountNonEmpty(values) > 0
}
