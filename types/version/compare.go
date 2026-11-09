// Source: docklet-next/issues/235
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original SemVer 2 precedence for hub and agent product versions.

package version

import (
	"strconv"
	"strings"
)

// Product is a parsed product version X.Y.Z[-pre][+build]. Build metadata is
// accepted and ignored for precedence, as SemVer 2 requires.
type Product struct {
	Major, Minor, Patch uint64
	Prerelease          []string
}

// Stable reports whether the version carries no prerelease identifiers.
func (p Product) Stable() bool { return len(p.Prerelease) == 0 }

// String renders the version without a "v" prefix and without build metadata.
func (p Product) String() string {
	out := strconv.FormatUint(p.Major, 10) + "." + strconv.FormatUint(p.Minor, 10) + "." + strconv.FormatUint(p.Patch, 10)
	if len(p.Prerelease) > 0 {
		out += "-" + strings.Join(p.Prerelease, ".")
	}
	return out
}

// ParseProduct accepts an optional "v" prefix. Only full SemVer 2 versions are
// valid: no shorthand, no leading zeros, no empty identifiers.
func ParseProduct(raw string) (Product, bool) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "v")
	raw, build, hasBuild := strings.Cut(raw, "+")
	if hasBuild && !validIdentifiersInternal(build, false) {
		return Product{}, false
	}
	core, pre, hasPre := strings.Cut(raw, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Product{}, false
	}
	var numbers [3]uint64
	for i, part := range parts {
		if !numericIdentifierInternal(part) {
			return Product{}, false
		}
		value, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return Product{}, false
		}
		numbers[i] = value
	}
	p := Product{Major: numbers[0], Minor: numbers[1], Patch: numbers[2]}
	if hasPre {
		if !validIdentifiersInternal(pre, true) {
			return Product{}, false
		}
		p.Prerelease = strings.Split(pre, ".")
	}
	return p, true
}

// CompareProduct orders two product versions by SemVer 2 precedence
// (1.0.0-rc.1 < 1.0.0-rc.2 < 1.0.0). ok is false when either is invalid.
func CompareProduct(a, b string) (int, bool) {
	pa, ok := ParseProduct(a)
	if !ok {
		return 0, false
	}
	pb, ok := ParseProduct(b)
	if !ok {
		return 0, false
	}
	return pa.Compare(pb), true
}

// Compare returns -1, 0 or 1.
func (p Product) Compare(o Product) int {
	for _, pair := range [][2]uint64{{p.Major, o.Major}, {p.Minor, o.Minor}, {p.Patch, o.Patch}} {
		if pair[0] != pair[1] {
			return order(pair[0] < pair[1])
		}
	}
	switch {
	case p.Stable() && o.Stable():
		return 0
	case p.Stable():
		return 1
	case o.Stable():
		return -1
	}
	for i := 0; i < len(p.Prerelease) && i < len(o.Prerelease); i++ {
		if c := compareIdentifierInternal(p.Prerelease[i], o.Prerelease[i]); c != 0 {
			return c
		}
	}
	if len(p.Prerelease) == len(o.Prerelease) {
		return 0
	}
	return order(len(p.Prerelease) < len(o.Prerelease))
}

func order(less bool) int {
	if less {
		return -1
	}
	return 1
}

func compareIdentifierInternal(a, b string) int {
	an, aNumeric := numericValueInternal(a)
	bn, bNumeric := numericValueInternal(b)
	switch {
	case aNumeric && bNumeric:
		if an == bn {
			return 0
		}
		return order(an < bn)
	case aNumeric:
		return -1
	case bNumeric:
		return 1
	case a == b:
		return 0
	}
	return order(a < b)
}

func numericValueInternal(id string) (uint64, bool) {
	if id == "" || strings.Trim(id, "0123456789") != "" {
		return 0, false
	}
	value, err := strconv.ParseUint(id, 10, 64)
	return value, err == nil
}

func numericIdentifierInternal(id string) bool {
	_, ok := numericValueInternal(id)
	return ok && (id == "0" || id[0] != '0')
}

// validIdentifiersInternal checks dot-separated identifiers; prerelease numeric
// identifiers must not have leading zeros, build identifiers may.
func validIdentifiersInternal(raw string, prerelease bool) bool {
	if raw == "" {
		return false
	}
	for _, id := range strings.Split(raw, ".") {
		if id == "" || strings.Trim(id, "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-") != "" {
			return false
		}
		if prerelease && strings.Trim(id, "0123456789") == "" && !numericIdentifierInternal(id) {
			return false
		}
	}
	return true
}
