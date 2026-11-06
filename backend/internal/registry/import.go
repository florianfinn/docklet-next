// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package registry

import (
	"bytes"
	"context"
	"errors"
	"go.getarcane.app/sys/crypto"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/platformmatch"
	dto "github.com/florianfinn/docklet-next/types/stackimport"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const importConfigLimit = 1 << 20

type ImportImage struct {
	Reference, Digest string
	Config            *v1.ConfigFile
}

// ResolveImportImage uses the same registry credentials and platform selection as
// PlatformImage, with bounded config bytes and no diagnostic text in its errors.
func (s *ContainerRegistryService) ResolveImportImage(ctx context.Context, raw string, platform v1.Platform, wrap func(http.RoundTripper) http.RoundTripper) (ImportImage, error) {
	fail := func(code string, status int) (ImportImage, error) { return ImportImage{}, dto.Failure(code, status) }
	if strings.TrimSpace(raw) != raw || guard.ValidateReference(raw) != nil || platform.OS == "" || platform.Architecture == "" {
		return fail("import.invalid-reference", 400)
	}
	transport := s.distributionHTTPClient.Transport
	if transport == nil {
		transport = remote.DefaultTransport
	}
	transport = wrap(transport)
	parsed, opts, _, err := s.platformOptionsInternal(ctx, raw, platform, true, transport)
	if err != nil {
		return ImportImage{}, importErrorInternal(err)
	}
	opts = append(opts, remote.WithTransport(transport), remote.WithRetryBackoff(remote.Backoff{Steps: 1}), remote.WithRetryPredicate(func(error) bool { return false }))
	puller, err := remote.NewPuller(opts...)
	if err != nil {
		return ImportImage{}, importErrorInternal(err)
	}
	desc, err := puller.Get(ctx, parsed)
	if err != nil {
		return ImportImage{}, importErrorInternal(err)
	}
	if !desc.MediaType.IsIndex() && desc.MediaType != types.OCIManifestSchema1 && desc.MediaType != types.DockerManifestSchema2 {
		return fail("import.source-unavailable", 502)
	}
	img, err := platformImageInternal(desc, platform, true)
	if err != nil {
		return ImportImage{}, importErrorInternal(err)
	}
	manifest, err := img.Manifest()
	if err != nil {
		return ImportImage{}, importErrorInternal(err)
	}
	config := manifest.Config
	if config.Size > importConfigLimit || len(config.Data) > importConfigLimit {
		return fail("import.too-large", 413)
	}
	var data []byte
	if config.Data != nil {
		data = config.Data
	} else {
		blob, err := puller.Layer(ctx, parsed.Context().Digest(config.Digest.String()))
		if err != nil {
			return ImportImage{}, importErrorInternal(err)
		}
		body, err := blob.Compressed()
		if err != nil {
			return ImportImage{}, importErrorInternal(err)
		}
		defer body.Close()
		data, err = io.ReadAll(io.LimitReader(body, importConfigLimit+1))
		if len(data) > importConfigLimit {
			return fail("import.too-large", 413)
		}
		if err != nil {
			return ImportImage{}, importErrorInternal(err)
		}
	}
	hash, _, err := v1.SHA256(bytes.NewReader(data))
	if err != nil || hash != config.Digest || int64(len(data)) != config.Size {
		return fail("import.source-unavailable", 502)
	}
	cfg, err := v1.ParseConfigFile(bytes.NewReader(data))
	if err != nil || cfg == nil {
		return fail("import.source-unavailable", 502)
	}
	if !platformmatch.Equal(ocispec.Platform{OS: cfg.OS, Architecture: cfg.Architecture, Variant: cfg.Variant}, ocispec.Platform{OS: platform.OS, Architecture: platform.Architecture, Variant: platform.Variant}) {
		return fail("import.source-unavailable", 502)
	}
	digest, err := img.Digest()
	if err != nil {
		return ImportImage{}, importErrorInternal(err)
	}
	return ImportImage{Reference: parsed.Name(), Digest: digest.String(), Config: cfg}, nil
}
func importErrorInternal(err error) error {
	if p, ok := errors.AsType[*dto.Problem](err); ok {
		return p
	}
	var network net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) && network.Timeout() {
		return dto.Failure("import.timeout", 504)
	}
	switch registryStatusInternal(err) {
	case 401, 403:
		return dto.Failure("import.unauthorized", 502)
	case 404:
		return dto.Failure("import.not-found", 404)
	}
	return dto.Failure("import.source-unavailable", 502)
}

// Imports never detach credential refresh from their budget or persist new tokens.
// Normal registry operations retain the existing shared refresh and token cache.
func (s *ContainerRegistryService) importCredentialInternal(ctx context.Context, reg *ContainerRegistry, transport http.RoundTripper) (*resolvedRegistryCredential, error) {
	if reg.RegistryType != RegistryTypeECR {
		return s.credentialForRegistryInternal(ctx, reg)
	}
	if reg.AWSAccessKeyID == "" && reg.AWSSecretAccessKey == "" {
		return nil, nil
	}
	if reg.ECRTokenGeneratedAt != nil && time.Since(reg.ECRTokenGeneratedAt.UTC()) < ecrTokenTTL && reg.ECRToken != "" {
		token, err := crypto.Decrypt(reg.ECRToken)
		if err == nil && strings.TrimSpace(token) != "" {
			return &resolvedRegistryCredential{Username: "AWS", Token: token}, nil
		}
	}
	token, err := requestECRTokenInternal(ctx, reg, &http.Client{Transport: transport})
	if err != nil {
		return nil, err
	}
	return &resolvedRegistryCredential{Username: token.username, Token: token.password}, nil
}
