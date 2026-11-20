// Source: getarcaneapp/arcane/backend/pkg/libarcane/volumehelper/helper.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; Complete tools image resolution without runtime fallback.

package volumehelper

import (
	"context"
	"strings"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// DefaultToolsImage pins the multi-platform tools image index for this release.
const DefaultToolsImage = "ghcr.io/florianfinn/docklet-tools:0.2.0@sha256:2b8e00f83ce199f9faab467572ac96979f80a1f738c12014b74f9c464e0aa132"

// ContainerLabel is retained so existing helper containers remain discoverable
// for cleanup across upgrades. Image selection never depends on this label.
const ContainerLabel = "com.getarcaneapp.volume-helper"

// ToolsImage selects one complete image reference, with the release default
// used only when no environment override or persisted setting supplies one.
func ToolsImage(configured string) string {
	if reference := strings.TrimSpace(configured); reference != "" {
		return reference
	}
	return DefaultToolsImage
}

func Labels() map[string]string {
	return map[string]string{libarcane.InternalResourceLabel: "true", ContainerLabel: "true"}
}

func RemoveOptions() client.ContainerRemoveOptions {
	return client.ContainerRemoveOptions{Force: true, RemoveVolumes: true}
}

// HostConfig applies the same mounts to every configured helper image.
func HostConfig(_ string, binds []string, mounts []mount.Mount) *container.HostConfig {
	return &container.HostConfig{Binds: binds, Mounts: mounts, AutoRemove: true}
}

// ResolveHelperImage shares the classified resolver with every tools-image consumer.
func ResolveHelperImage(ctx context.Context, dockerClient *client.Client, configured string) (string, error) {
	image, err := Resolve(ctx, dockerClient, configured, nil)
	return image.Ref, err
}
