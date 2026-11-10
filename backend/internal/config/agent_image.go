// Source: docklet-next/issues/362
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original agent image resolution for onboarding, hints and image protection.

package config

import (
	"strings"

	productVersion "github.com/florianfinn/docklet-next/types/version"
)

// SourceAgentRepository is the registry-free agent image the deploy/hub
// Dockerfile agent target builds when no release default is stamped.
const SourceAgentRepository = "docklet-next-agent"

// AgentImageReference resolves the agent image for installation files, update
// hints and image protection: an operator DOCKLET_AGENT_IMAGE override (any
// reference, validated by the caller), else the release-stamped default, else
// the source-build fallback tagged with this build's version.
func AgentImageReference(override string) (image string, operator bool) {
	if image = strings.TrimSpace(override); image != "" {
		return image, true
	}
	if DefaultAgentImage != "" {
		return DefaultAgentImage, false
	}
	return SourceAgentRepository + ":" + sourceAgentTagInternal(), false
}

// AgentReleaseRepository is the repository whose versions the release feed
// describes: the stamped release repository's agent image, or the source-build
// repository for unstamped builds.
func AgentReleaseRepository() string {
	if DefaultAgentImage != "" {
		return ReleaseRepository + "-agent"
	}
	return SourceAgentRepository
}

func sourceAgentTagInternal() string {
	if parsed, ok := productVersion.ParseProduct(Version); ok {
		return parsed.String()
	}
	return "0.0.0"
}
