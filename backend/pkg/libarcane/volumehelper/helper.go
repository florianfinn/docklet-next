// Source: getarcaneapp/arcane/backend/pkg/libarcane/volumehelper/helper.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package volumehelper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// DefaultToolsImage is pinned with the hub release. Issue #180 adds the digest
// after the first publication of this tools image.
const DefaultToolsImage = "ghcr.io/florianfinn/docklet-tools:0.1.0"

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

// ResolveHelperImage pulls an absent configured helper. Runtime images cannot
// supply ACFS and must never be substituted when that pull fails.
func ResolveHelperImage(ctx context.Context, dockerClient *client.Client, configured string) (string, error) {
	if dockerClient == nil {
		return "", errors.New("docker service unavailable")
	}
	toolsImage := ToolsImage(configured)
	if _, err := dockerClient.ImageInspect(ctx, toolsImage); err == nil {
		return toolsImage, nil
	} else if !errdefs.IsNotFound(err) {
		return "", fmt.Errorf("inspect helper image: %w", err)
	}
	pullReader, err := dockerClient.ImagePull(ctx, toolsImage, client.ImagePullOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to resolve helper image: %w", err)
	}
	if pullReader == nil {
		return "", errors.New("helper image pull returned no response body")
	}
	defer func() { _ = pullReader.Close() }()
	// Docker may return HTTP 200 then report failure inside its progress stream.
	decoder := json.NewDecoder(pullReader)
	for {
		var progress struct {
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if err := decoder.Decode(&progress); err == io.EOF {
			break
		} else if err != nil {
			return "", fmt.Errorf("read helper image pull response: %w", err)
		}
		if progress.Error != "" || progress.ErrorDetail.Message != "" {
			return "", errors.New("helper image pull failed")
		}
	}
	return toolsImage, nil
}
