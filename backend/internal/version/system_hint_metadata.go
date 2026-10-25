// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Configured system references are reserved for authenticated hub reads.

package version

import dto "github.com/florianfinn/docklet-next/types/version"

func publicVersionInfo(info *dto.Info) *dto.Info {
	result := *info
	result.CurrentImageRef = ""
	result.ToolsImage = ""
	result.ToolsImageOverride = false
	return &result
}
