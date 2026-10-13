// Source: getarcaneapp/arcane/backend/buildables/disabled.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

//go:build !buildables

package buildables

// EnabledFeatures is empty when the buildables tag is not set.
var EnabledFeatures = ""

// HasBuildFeature always returns false when the buildables tag is not set.
//
//nolint:shimbad // build-tag stub: the real implementation is compiled under the buildables tag
func HasBuildFeature(feature string) bool {
	return false
}
