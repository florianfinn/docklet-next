//go:build !linux

// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package version

import "github.com/florianfinn/docklet-next/types/version"

func inspectOnboarding(projectDirectory, _ string) *version.OnboardingChecks {
	return &version.OnboardingChecks{ProjectDirectory: projectDirectory}
}
