// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Resolve local roles without contacting a registry.

package identity

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/platformmatch"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func apiAtLeast(raw string, minor int) bool {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return false
	}
	major, e := strconv.Atoi(parts[0])
	m, f := strconv.Atoi(parts[1])
	return e == nil && f == nil && (major > 1 || major == 1 && m >= minor)
}
func imagePlatform(img image.InspectResponse) (Platform, error) {
	raw := img.Os + "/" + img.Architecture
	if img.Variant != "" {
		raw += "/" + img.Variant
	}
	return ParsePlatform(raw)
}
func descriptorPlatform(d *ocispec.Descriptor) (Platform, error) {
	if d == nil || d.Platform == nil {
		return Platform{}, unavailable()
	}
	raw := d.Platform.OS + "/" + d.Platform.Architecture
	if d.Platform.Variant != "" {
		raw += "/" + d.Platform.Variant
	}
	return ParsePlatform(raw)
}

// Resolve requires unqualified inspect for L. Qualified results can supply N/P
// only. Container descriptors take priority over the daemon's default platform.
func Resolve(ctx context.Context, c *client.Client, img image.InspectResponse, requested string, container *ocispec.Descriptor) (LocalImage, error) {
	id, err := ParseLocalID(img.ID)
	if err != nil {
		return LocalImage{}, err
	}
	local := LocalImage{ID: id}
	if img.Descriptor == nil {
		p, e := imagePlatform(img)
		if e != nil {
			return LocalImage{}, e
		}
		if requested == "" {
			want, e := daemonPlatform(ctx, c, img)
			if e != nil || !want.Matches(p) {
				return LocalImage{}, unavailable()
			}
		}
		if requested != "" {
			want, e := ParsePlatform(requested)
			if e != nil || !want.Matches(p) {
				return LocalImage{}, unavailable()
			}
		}
		local.Platform = p
		local.Key = NativeKey{id.value}
		return local, nil
	}
	if !apiAtLeast(c.ClientVersion(), 48) || img.Descriptor.Digest.String() != id.value {
		return LocalImage{}, unavailable()
	}
	local.containerd = true
	var p Platform
	if container != nil {
		p, err = descriptorPlatform(container)
	} else if requested != "" {
		p, err = ParsePlatform(requested)
	} else {
		p, err = daemonPlatform(ctx, c, img)
	}
	if err != nil {
		return LocalImage{}, err
	}
	if requested != "" {
		want, e := ParsePlatform(requested)
		if e != nil || !want.Matches(p) {
			return LocalImage{}, unavailable()
		}
	}
	local.Platform = p
	if container != nil {
		if !manifestDescriptor(container) {
			return LocalImage{}, unavailable()
		}
		local.Key = NativeKey{container.Digest.String()}
		return local, nil
	}
	if apiAtLeast(c.ClientVersion(), 49) {
		platform := p.OCI()
		qualified, e := c.ImageInspect(ctx, id.value, client.ImageInspectWithPlatform(&platform))
		if e != nil || !manifestDescriptor(qualified.Descriptor) || qualified.Descriptor.Digest.String() != qualified.ID {
			return LocalImage{}, unavailable()
		}
		qp, e := imagePlatform(qualified.InspectResponse)
		if e != nil || !qp.Matches(p) {
			return LocalImage{}, unavailable()
		}
		local.Key = NativeKey{qualified.Descriptor.Digest.String()}
		return local, nil
	}
	manifests, e := c.ImageInspect(ctx, id.value, client.ImageInspectWithManifests(true))
	if e != nil {
		return LocalImage{}, unavailable()
	}
	for _, child := range manifests.Manifests {
		if !child.Available || child.Kind != "image" || child.ImageData == nil {
			continue
		}
		d := child.Descriptor
		d.Platform = &child.ImageData.Platform
		cp, e := descriptorPlatform(&d)
		if e != nil || !cp.Matches(p) {
			continue
		}
		if local.Key.value != "" || !manifestDescriptor(&d) || child.ID != d.Digest.String() {
			return LocalImage{}, unavailable()
		}
		local.Key = NativeKey{d.Digest.String()}
	}
	if local.Key.value == "" {
		return LocalImage{}, unavailable()
	}
	return local, nil
}

func daemonPlatform(ctx context.Context, c *client.Client, img image.InspectResponse) (Platform, error) {
	info, e := c.Info(ctx, client.InfoOptions{})
	if e != nil {
		return Platform{}, unavailable()
	}
	p, err := ParsePlatform(info.Info.OSType + "/" + info.Info.Architecture)
	if err != nil {
		return Platform{}, err
	}
	if ip, e := imagePlatform(img); e == nil {
		// Info lacks the variant; use config evidence only for the same OS/arch.
		candidate, host := ip.OCI(), p.OCI()
		candidate.Variant, host.Variant = "", ""
		if platformmatch.Equal(candidate, host) {
			p = ip
		}
	}
	return p, nil
}

// ResolveContainer uses the actual container platform, including classic config
// platform evidence; a missing containerd platform must never become a default.
func ResolveContainer(ctx context.Context, c *client.Client, img image.InspectResponse, requested string, descriptor *ocispec.Descriptor) (LocalImage, error) {
	if requested == "" && descriptor == nil {
		if img.Descriptor != nil {
			return LocalImage{}, unavailable()
		}
		p, e := imagePlatform(img)
		if e != nil {
			return LocalImage{}, e
		}
		requested = p.String()
	}
	return Resolve(ctx, c, img, requested, descriptor)
}

func manifestDescriptor(d *ocispec.Descriptor) bool {
	return d != nil && digestPattern.MatchString(d.Digest.String()) && (d.MediaType == ocispec.MediaTypeImageManifest || d.MediaType == "application/vnd.docker.distribution.manifest.v2+json")
}
