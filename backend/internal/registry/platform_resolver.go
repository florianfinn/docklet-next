// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package registry

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/registryauth"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/stackimport"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"net/http"
)

// platformOptionsInternal is shared by update observations and content import.
// Only imports require an enabled registry match before the first HTTP request.
func (s *ContainerRegistryService) platformOptionsInternal(ctx context.Context, raw string, platform v1.Platform, configuredOnly bool, importTransport ...http.RoundTripper) (name.Reference, []remote.Option, string, error) {
	authState := "anonymous"
	refs := []name.Option{name.WeakValidation}
	opts := []remote.Option{remote.WithContext(ctx), remote.WithPlatform(platform), remote.WithTransport(s.distributionHTTPClient.Transport)}
	// A nil transport uses the library default, preserving TLS and registry auth.
	if s.distributionHTTPClient.Transport == nil {
		opts = opts[:2]
	}
	parsed, err := name.ParseReference(raw, refs...)
	if err != nil {
		return nil, nil, authState, imageDTO.Failure("image.invalid-reference", 400)
	}
	registries, err := s.GetEnabledRegistries(ctx)
	if err != nil {
		return nil, nil, authState, imageDTO.Failure("registry.unavailable", 503)
	}
	allowed := parsed.Context().RegistryStr() == name.DefaultRegistry
	for i := range registries {
		reg := &registries[i]
		if !registryauth.IsRegistryMatch(reg.URL, parsed.Context().RegistryStr()) {
			continue
		}
		allowed = true
		if reg.Insecure {
			refs = append(refs, name.Insecure)
			parsed, err = name.ParseReference(raw, refs...)
			if err != nil {
				return nil, nil, authState, imageDTO.Failure("image.invalid-reference", 400)
			}
		}
		if reg.Token != "" || reg.AWSSecretAccessKey != "" {
			authState = "unknown"
			if err = settings.ValidateCredentialEncryption(); err != nil {
				return nil, nil, authState, err
			}
			var cred *resolvedRegistryCredential
			var e error
			if configuredOnly && len(importTransport) > 0 {
				cred, e = s.importCredentialInternal(ctx, reg, importTransport[0])
			} else {
				cred, e = s.credentialForRegistryInternal(ctx, reg)
			}
			if e != nil {
				if configuredOnly {
					return nil, nil, authState, e
				}
				return nil, nil, authState, imageDTO.Failure("registry.credential-unavailable", 503)
			}
			if cred != nil {
				opts = append(opts, remote.WithAuth(authn.FromConfig(authn.AuthConfig{Username: cred.Username, Password: cred.Token})))
				authState = "credential"
			}
		}
		break
	}
	if configuredOnly && !allowed {
		return nil, nil, authState, dto.Failure("import.registry-not-configured", 400)
	}
	return parsed, opts, authState, nil
}
