// Source: getarcaneapp/arcane/backend/internal/registry/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package registry

import (
	"context"

	"errors"
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"runtime"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
)

// ImageVersionLabel resolves the org.opencontainers.image.version label from the
// remote image config for a tag or digest reference, without pulling the image.
// Anonymous access only — this is used for Arcane's own public GHCR images.
func (s *ContainerRegistryService) ImageVersionLabel(ctx context.Context, imageRef string) (string, error) {
	imageRef = strings.TrimSpace(imageRef)
	if imageRef == "" {
		return "", errors.New("empty image reference")
	}

	label, found, err := s.labelCache.GetWithLoaders(imageRef, func(_ []string) (map[string]string, error) {
		loadCtx, cancel := context.WithTimeout(ctx, timeouts.DefaultRegistry)
		defer cancel()

		value, loadErr := fetchImageVersionLabelInternal(loadCtx, imageRef)
		if loadErr != nil {
			return nil, loadErr
		}
		return map[string]string{imageRef: value}, nil
	})
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("registry version label cache loader returned no label")
	}
	return label, nil
}

func fetchImageVersionLabelInternal(ctx context.Context, imageRef string) (string, error) {
	parsedRef, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("invalid image reference %q: %w", imageRef, err)
	}

	img, err := remote.Image(parsedRef,
		remote.WithContext(ctx),
		remote.WithPlatform(v1.Platform{OS: "linux", Architecture: runtime.GOARCH}))
	if err != nil {
		return "", fmt.Errorf("version label fetch failed for %s: %w", imageRef, err)
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		return "", fmt.Errorf("version label fetch failed for %s: %w", imageRef, err)
	}
	if cfg == nil || strings.TrimSpace(cfg.Config.Labels[ociImageVersionLabel]) == "" {
		return "", ErrNoVersionLabel
	}
	return strings.TrimSpace(cfg.Config.Labels[ociImageVersionLabel]), nil
}
