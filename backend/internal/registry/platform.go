// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package registry

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/registryauth"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/platformmatch"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"log/slog"
	"strings"
	"sync"
)

type PlatformObservation struct{ Digest, ConfigDigest, Version, AuthState string }

// PlatformImage resolves candidate refs at their native platform-manifest
// level. A changed index on another platform is not an update.
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
	// A HEAD request does not count against registry pull allowances; an
	// unchanged top-level digest reuses the earlier platform observation.
	head, err := remote.Head(parsed, opts...)
	if err == nil {
		if cached, ok := platformObservations.get(platformCacheKeyInternal(parsed, head.Digest.String(), platform)); ok {
			cached.AuthState = out.AuthState
			return cached, nil
		}
	} else if registryProbeError(err, out.AuthState == "anonymous").Problem.Code == "registry.rate-limited" {
		return out, s.platformFailureInternal(ctx, raw, parsed, err, &out)
	}
	desc, err := remote.Get(parsed, opts...)
	var img v1.Image
	if err == nil {
		img, err = platformImageInternal(desc, platform)
	}
	if err != nil {
		return out, s.platformFailureInternal(ctx, raw, parsed, err, &out)
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
	if err != nil || cfg == nil || !platformmatch.Equal(ocispec.Platform{OS: cfg.OS, Architecture: cfg.Architecture, Variant: cfg.Variant}, ocispec.Platform{OS: platform.OS, Architecture: platform.Architecture, Variant: platform.Variant}) {
		return out, imageDTO.Failure("update.check-unavailable", 503)
	}
	out.Digest = digest.String()
	out.ConfigDigest = configDigest.String()
	out.Version = strings.TrimSpace(cfg.Config.Labels["org.opencontainers.image.version"])
	platformObservations.put(platformCacheKeyInternal(parsed, desc.Digest.String(), platform), out)
	return out, nil
}

// platformFailureInternal classifies a registry failure and logs the cause the
// neutral public code no longer carries.
func (s *ContainerRegistryService) platformFailureInternal(ctx context.Context, raw string, parsed name.Reference, err error, out *PlatformObservation) error {
	problem := registryProbeError(err, out.AuthState == "anonymous")
	if problem.Problem.Code == "registry.authentication-required" {
		out.AuthState = "authentication-required"
	}
	if problem.Problem.Code == "registry.authentication-failed" {
		out.AuthState = "authentication-failed"
	}
	slog.WarnContext(ctx, "Registry platform lookup failed", "reference", raw, "registry", parsed.Context().RegistryStr(), "code", problem.Problem.Code, "status", registryStatusInternal(err))
	return problem
}

// Top-level digests are content addresses, so a cached platform observation
// for one never goes stale. The bound only limits memory.
const platformCacheLimit = 4096

type platformCacheInternal struct {
	mu      sync.Mutex
	entries map[string]PlatformObservation
}

var platformObservations = &platformCacheInternal{entries: map[string]PlatformObservation{}}

func platformCacheKeyInternal(parsed name.Reference, digest string, platform v1.Platform) string {
	return parsed.Context().Name() + "@" + digest + "|" + platform.String()
}
func (c *platformCacheInternal) get(key string) (PlatformObservation, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out, ok := c.entries[key]
	return out, ok
}
func (c *platformCacheInternal) put(key string, out PlatformObservation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= platformCacheLimit {
		clear(c.entries)
	}
	c.entries[key] = out
}

// Registry libraries compare index variants literally. Select candidates with
// the same normalized strict matcher as local evidence, including nested indexes.
func platformImageInternal(desc *remote.Descriptor, wanted v1.Platform) (v1.Image, error) {
	if !desc.MediaType.IsIndex() {
		return desc.Image()
	}
	index, err := desc.ImageIndex()
	if err != nil {
		return nil, err
	}
	return platformIndexImageInternal(index, wanted, 0)
}
func platformIndexImageInternal(index v1.ImageIndex, wanted v1.Platform, depth int) (v1.Image, error) {
	fail := func() (v1.Image, error) { return nil, imageDTO.Failure("update.check-unavailable", 503) }
	if depth > 8 {
		return fail()
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, err
	}
	var selected v1.Image
	for _, child := range manifest.Manifests {
		if child.Platform != nil && !platformmatch.Equal(ocispec.Platform{OS: wanted.OS, Architecture: wanted.Architecture, Variant: wanted.Variant}, ocispec.Platform{OS: child.Platform.OS, Architecture: child.Platform.Architecture, Variant: child.Platform.Variant}) {
			continue
		}
		var candidate v1.Image
		if child.MediaType.IsIndex() {
			nested, e := index.ImageIndex(child.Digest)
			if e != nil {
				return nil, e
			}
			candidate, err = platformIndexImageInternal(nested, wanted, depth+1)
			if err != nil {
				continue
			}
		} else if child.Platform != nil && child.MediaType.IsImage() {
			candidate, err = index.Image(child.Digest)
		} else {
			continue
		}
		if err != nil {
			return nil, err
		}
		if selected != nil {
			return fail()
		}
		selected = candidate
	}
	if selected == nil {
		return fail()
	}
	return selected, nil
}
