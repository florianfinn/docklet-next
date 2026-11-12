// Source: docklet-next/issues/248
// Revision: 5a4ecc67a7712349cb869574429d09f09a96e895
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original normalized image reference joins for update projections.

package details

import (
	"strings"

	"github.com/distribution/reference"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
)

// NormalizedImageRef names equivalent spellings (familiar, library, fully
// qualified, implied latest) alike; unparseable references stay literal.
func NormalizedImageRef(ref string) string {
	ref = strings.TrimSpace(ref)
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return ref
	}
	return reference.TagNameOnly(named).String()
}

// UpdateInfoForRef prefers the literal key and falls back to an equivalent
// spelling, so an observation stored under another spelling still joins.
func UpdateInfoForRef(byRef map[string]*imagetypes.UpdateInfo, ref string) *imagetypes.UpdateInfo {
	ref = strings.TrimSpace(ref)
	if info, ok := byRef[ref]; ok {
		return info
	}
	want := NormalizedImageRef(ref)
	for key, info := range byRef {
		if NormalizedImageRef(key) == want {
			return info
		}
	}
	return nil
}
