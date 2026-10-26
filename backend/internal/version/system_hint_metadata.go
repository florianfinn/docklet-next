// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
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
