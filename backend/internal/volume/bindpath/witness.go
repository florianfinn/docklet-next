// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package bindpath

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"strings"
	"time"
)

type Exec func(context.Context, string, string, []string) (string, string, error)

// Witness uses a structured mount so a missing source is never created by Docker.
func Witness(ctx context.Context, cli *client.Client, image, source string, exec Exec) (string, error) {
	labels := volumehelper.Labels()
	labels["docklet.bind-witness"] = "true"
	labels["docklet.bind-key"] = volume.BindKey(source)
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{Image: image, Cmd: []string{"sleep", "infinity"}, NetworkDisabled: true, Labels: labels}, HostConfig: volumehelper.HostConfig(image, nil, []mount.Mount{{Type: mount.TypeBind, Source: source, Target: "/source", ReadOnly: true}})})
	if err != nil {
		code := "volume.helper-unavailable"
		status := 503
		text := strings.ToLower(err.Error())
		if strings.Contains(text, "bind source path does not exist") || strings.Contains(text, "no such file") {
			code = "volume.bind.source-missing"
			status = 404
		}
		return "", dto.NewProblem(status, code, err)
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_, _ = cli.ContainerRemove(clean, created.ID, volumehelper.RemoveOptions())
	}()
	if _, err = cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return "", dto.NewProblem(503, "volume.helper-unavailable", err)
	}
	out, _, err := exec(ctx, created.ID, "", []string{"sh", "-c", `if test -d /source; then printf directory; elif test -f /source; then printf file; else printf unsupported; fi`})
	if err != nil {
		return "", dto.NewProblem(503, "volume.helper-unavailable", err)
	}
	switch strings.TrimSpace(out) {
	case "directory":
		return "directory", nil
	case "file":
		return "file", nil
	case "unsupported":
		return "", dto.NewProblem(422, "volume.bind.unsupported-type", nil)
	default:
		return "", dto.NewProblem(503, "volume.helper-unavailable", errors.New("invalid witness response"))
	}
}
