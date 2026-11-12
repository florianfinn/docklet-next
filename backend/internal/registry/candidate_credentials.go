// Source: docklet-next/issues/438
// Revision: 57e835f7cce5778fd3d164298e5b488e11abb71d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original registry-scoped temporary scanner credential seam.

package registry

import (
	"context"

	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/registryauth"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/vuln"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dockerregistry "github.com/moby/moby/api/types/registry"
)

// CandidateDockerConfig resolves only the target registry's enabled credential,
// including ECR refresh. The caller owns private attempt storage and disposal.
func (s *ContainerRegistryService) CandidateDockerConfig(ctx context.Context, raw string) ([]byte, bool, error) {
	named, err := reference.ParseNormalizedNamed(raw)
	if err != nil {
		return nil, false, imageDTO.Failure("image.invalid-reference", 400)
	}
	registries, err := s.GetEnabledRegistries(ctx)
	if err != nil {
		return nil, false, imageDTO.Failure("registry.unavailable", 503)
	}
	host := reference.Domain(named)
	for i := range registries {
		reg := &registries[i]
		if !registryauth.IsRegistryMatch(reg.URL, host) {
			continue
		}
		credential, err := s.credentialForRegistryInternal(ctx, reg)
		if err != nil {
			return nil, false, imageDTO.Failure("registry.credential-unavailable", 503)
		}
		if credential == nil {
			return nil, reg.Insecure, nil
		}
		key := host
		if host == "docker.io" {
			key = "https://index.docker.io/v1/"
		}
		config, err := vuln.BuildDockerConfigJSON(map[string]dockerregistry.AuthConfig{key: {Username: credential.Username, Password: credential.Token}})
		return config, reg.Insecure, err
	}
	return nil, false, nil
}
