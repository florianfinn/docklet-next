// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// ResolveTarget binds the exact feed version to its immutable manifest/index
// digest and verifies the release includes the supported linux/amd64 platform.
func ResolveTarget(ctx context.Context, repository, version string, transport http.RoundTripper) (string, error) {
	tag, err := name.NewTag(repository+":"+version, name.StrictValidation)
	if err != nil {
		return "", Refuse("update.invalid-target")
	}
	options := []remote.Option{remote.WithContext(ctx)}
	if transport != nil {
		options = append(options, remote.WithTransport(transport))
	}
	descriptor, err := remote.Get(tag, options...)
	if err != nil {
		return "", Refuse("update.check-unavailable")
	}
	supported := false
	if descriptor.MediaType.IsIndex() {
		index, err := descriptor.ImageIndex()
		if err != nil {
			return "", err
		}
		manifest, err := index.IndexManifest()
		if err != nil {
			return "", err
		}
		for _, item := range manifest.Manifests {
			if item.Platform != nil && item.Platform.OS == "linux" && item.Platform.Architecture == "amd64" {
				supported = true
			}
		}
	} else {
		image, err := descriptor.Image()
		if err != nil {
			return "", err
		}
		config, err := image.ConfigFile()
		if err != nil {
			return "", err
		}
		supported = config.OS == "linux" && config.Architecture == "amd64"
	}
	if !supported {
		return "", Refuse("update.check-unavailable")
	}
	return fmt.Sprintf("%s:%s@%s", repository, version, descriptor.Digest.String()), nil
}
