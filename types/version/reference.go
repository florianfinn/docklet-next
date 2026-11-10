// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original release reference predicate for Docklet's own image defaults.

package version

import (
	"regexp"
	"strings"
)

var (
	releaseRegistryHost = regexp.MustCompile(`^(?:localhost|[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+)(?::[0-9]{1,5})?$`)
	releasePathPart     = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$`)
	releaseDigest       = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

// ReleaseReference reports whether ref is a release pin
// registry[:port]/path:X.Y.Z[-pre]@sha256:<64 hex>. It applies to Docklet's own
// defaults only; operator overrides accept any valid reference.
func ReleaseReference(ref string) bool {
	named, digest, ok := strings.Cut(ref, "@")
	if !ok || !releaseDigest.MatchString(digest) {
		return false
	}
	slash := strings.LastIndex(named, "/")
	colon := strings.LastIndex(named, ":")
	if slash < 0 || colon < slash {
		return false
	}
	name, tag := named[:colon], named[colon+1:]
	parsed, ok := ParseProduct(tag)
	if !ok || strings.HasPrefix(tag, "v") || parsed.String() != tag {
		return false
	}
	parts := strings.Split(name, "/")
	if len(parts) < 2 || !releaseRegistryHost.MatchString(parts[0]) {
		return false
	}
	for _, part := range parts[1:] {
		if !releasePathPart.MatchString(part) {
			return false
		}
	}
	return true
}
