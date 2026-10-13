// Source: getarcaneapp/kit/updater/digest/digest.go
// Revision: 119f68956cee6d5aba8a5463561aaf9be9943b65
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

// Package digest parses and canonicalizes the OCI content digests that identify
// an exact image, independent of the tag pointing at it.
package digest

import (
	"fmt"
	"strings"

	oci "github.com/opencontainers/go-digest"
)

// Normalize parses and canonicalizes an OCI digest.
func Normalize(value string) (string, error) {
	parsed, err := oci.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("invalid OCI digest %q: %w", value, err)
	}
	return parsed.String(), nil
}

// FromReferenceSuffix returns the digest from a name@digest reference. It
// reports false when ref carries no digest, or one that does not parse.
func FromReferenceSuffix(ref string) (string, bool) {
	_, digestValue, ok := strings.Cut(strings.TrimSpace(ref), "@")
	if !ok {
		return "", false
	}
	normalized, err := Normalize(digestValue)
	if err != nil {
		return "", false
	}
	return normalized, true
}
