// Source: getarcaneapp/kit/pkg/default.go
// Revision: 9c61043fc52d1977d644f075d9c168335c3a6df7
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package kit

// ParseOrDefault parses value using parse, falling back to defaultValue when empty or unparseable.
func ParseOrDefault[T any](value string, defaultValue T, parse func(string) (T, error)) T {
	if value == "" {
		return defaultValue
	}
	v, err := parse(value)
	return Ternary(err == nil, v, defaultValue)
}

// As returns value as a T, or fallback when it is nil or holds another type.
func As[T any](value any, fallback T) T {
	if v, ok := value.(T); ok {
		return v
	}
	return fallback
}
