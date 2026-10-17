// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package version

import (
	"github.com/florianfinn/docklet-next/types/version"
	"time"
)

// The probe runs only on authenticated agent version reads, at most once per 30 seconds.
func (s *VersionService) cachedOnboardingChecks() *version.OnboardingChecks {
	if s.settingsService == nil {
		return nil
	}
	directory := s.settingsService.GetSettingsConfig().ProjectsDirectory.Value
	dockerHost := ""
	if s.dockerService != nil {
		dockerHost = s.dockerService.DockerHost()
	}
	s.onboardingMu.Lock()
	defer s.onboardingMu.Unlock()
	if s.onboardingCache == nil || s.onboardingCache.ProjectDirectory != directory || s.onboardingDockerHost != dockerHost || time.Since(s.onboardingCachedAt) >= 30*time.Second {
		s.onboardingCache = inspectOnboarding(directory, dockerHost)
		s.onboardingDockerHost = dockerHost
		s.onboardingCachedAt = time.Now()
	}
	checks := *s.onboardingCache
	return &checks
}
