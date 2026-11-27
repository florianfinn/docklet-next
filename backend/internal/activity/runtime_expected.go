// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

package activity

import dto "github.com/florianfinn/docklet-next/types/updater"

func runtimeExpectedValid(items []dto.ExpectedCandidate) bool {
	if len(items) > dto.MaxLifecycleServices {
		return false
	}
	seen := map[string]bool{}
	for _, item := range items {
		if !runtimeTextInternal(item.Service, 256) || !dto.ValidManifest(item.Manifest) || seen[item.Service] {
			return false
		}
		seen[item.Service] = true
	}
	return true
}
