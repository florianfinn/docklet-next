// Source: docklet-next/issues/341
// Revision: 0c8a014eda72706e48d60c22e86526e7dfd15964
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: An observation belongs to the reference it was checked for.

package imageupdate

import (
	"strings"

	"go.getarcane.app/updater/refs"
)

// SameUpdateReference reports whether an observation stored for observed
// belongs to the configured reference. Spellings and pinned digests are
// compared by registry, repository and tag; another tag of the same image is
// a different reference.
func SameUpdateReference(observed, configured string) bool {
	normalize := func(raw string) string {
		raw, _, _ = strings.Cut(strings.TrimSpace(raw), "@")
		parts, err := refs.NormalizeReference(raw)
		if err != nil {
			return ""
		}
		return parts.NormalizedRef
	}
	left := normalize(observed)
	return left != "" && left == normalize(configured)
}
