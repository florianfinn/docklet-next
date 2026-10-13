// Source: getarcaneapp/arcane/backend/internal/config/version.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package config

import "runtime"

var (
	Version          = "dev"
	Revision         = "unknown"
	BuildTime        = "unknown"
	NodeVersion      = "unknown"
	SvelteKitVersion = "unknown"
)

// ShortRevision returns the first 8 characters of the revision hash
func ShortRevision() string {
	if len(Revision) > 8 {
		return Revision[:8]
	}
	return Revision
}

// GoVersion returns the Go runtime version
func GoVersion() string {
	return runtime.Version()
}
