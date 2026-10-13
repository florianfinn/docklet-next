// Source: getarcaneapp/arcane/types/search/profile.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package search

// Profile selects the metadata defaults and ranking rules for a search surface.
type Profile uint8

const (
	SettingsProfile Profile = iota + 1
	CustomizeProfile
)
