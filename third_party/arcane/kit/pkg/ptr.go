// Source: getarcaneapp/kit/pkg/ptr.go
// Revision: 9c61043fc52d1977d644f075d9c168335c3a6df7
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package kit

// FromPtr returns the pointed-to value, or T's zero value if ptr is nil.
func FromPtr[T any](ptr *T) T {
	if ptr == nil {
		var zero T
		return zero
	}
	return *ptr
}
