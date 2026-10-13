// Source: getarcaneapp/arcane/backend/buildables/buildables.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

//go:build buildables

package buildables

import "strings"

// EnabledFeatures is a comma-separated list of enabled buildable features.
var EnabledFeatures = ""

// HasBuildFeature returns true when the feature is present in EnabledFeatures.
func HasBuildFeature(feature string) bool {
	if feature == "" || EnabledFeatures == "" {
		return false
	}
	for value := range strings.SplitSeq(EnabledFeatures, ",") {
		if strings.EqualFold(strings.TrimSpace(value), feature) {
			return true
		}
	}
	return false
}
