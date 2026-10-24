// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package registry

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/registryauth"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"strings"
)

type PlatformObservation struct{ Digest, ConfigDigest, Version, AuthState string }

// PlatformImage resolves both current immutable and candidate refs at the same
// platform-manifest level. A changed index on another platform is not an update.
func (s *ContainerRegistryService) PlatformImage(ctx context.Context, raw string, platform v1.Platform) (PlatformObservation, error) {
	out := PlatformObservation{AuthState: "anonymous"}
	if err := guard.ValidateReference(raw); err != nil {
		return out, err
	}
	if platform.OS == "" || platform.Architecture == "" {
		return out, imageDTO.Failure("update.check-unavailable", 503)
	}
	refs := []name.Option{name.WeakValidation}
	opts := []remote.Option{remote.WithContext(ctx), remote.WithPlatform(platform), remote.WithTransport(s.distributionHTTPClient.Transport)}
	// A nil transport uses the library default, preserving TLS and registry auth.
	if s.distributionHTTPClient.Transport == nil {
		opts = opts[:2]
	}
	parsed, err := name.ParseReference(raw, refs...)
	if err != nil {
		return out, imageDTO.Failure("image.invalid-reference", 400)
	}
	registries, err := s.GetEnabledRegistries(ctx)
	if err != nil {
		return out, imageDTO.Failure("registry.unavailable", 503)
	}
	for i := range registries {
		reg := &registries[i]
		if !registryauth.IsRegistryMatch(reg.URL, parsed.Context().RegistryStr()) {
			continue
		}
		if reg.Insecure {
			refs = append(refs, name.Insecure)
			parsed, err = name.ParseReference(raw, refs...)
			if err != nil {
				return out, imageDTO.Failure("image.invalid-reference", 400)
			}
		}
		if reg.Token != "" || reg.AWSSecretAccessKey != "" {
			out.AuthState = "unknown"
			if err = settings.ValidateCredentialEncryption(); err != nil {
				return out, err
			}
			cred, e := s.credentialForRegistryInternal(ctx, reg)
			if e != nil {
				return out, imageDTO.Failure("registry.credential-unavailable", 503)
			}
			if cred != nil {
				opts = append(opts, remote.WithAuth(authn.FromConfig(authn.AuthConfig{Username: cred.Username, Password: cred.Token})))
				out.AuthState = "credential"
			}
		}
		break
	}
	img, err := remote.Image(parsed, opts...)
	if err != nil {
		code := "registry.unavailable"
		status := 503
		var transportErr *transport.Error
		if errors.As(err, &transportErr) {
			if transportErr.StatusCode == 401 || transportErr.StatusCode == 403 {
				if out.AuthState == "anonymous" {
					out.AuthState = "authentication-required"
				} else {
					out.AuthState = "authentication-failed"
				}
				code = "registry.authentication-failed"
				status = 401
			}
			if transportErr.StatusCode == 429 {
				code = "registry.rate-limited"
				status = 429
			}
		}
		return out, imageDTO.Failure(code, status)
	}
	digest, err := img.Digest()
	if err != nil {
		return out, imageDTO.Failure("update.check-unavailable", 503)
	}
	configDigest, err := img.ConfigName()
	if err != nil {
		return out, imageDTO.Failure("update.check-unavailable", 503)
	}
	cfg, err := img.ConfigFile()
	if err != nil || cfg == nil || cfg.OS != platform.OS || cfg.Architecture != platform.Architecture || cfg.Variant != platform.Variant {
		return out, imageDTO.Failure("update.check-unavailable", 503)
	}
	out.Digest = digest.String()
	out.ConfigDigest = configDigest.String()
	out.Version = strings.TrimSpace(cfg.Config.Labels["org.opencontainers.image.version"])
	return out, nil
}
