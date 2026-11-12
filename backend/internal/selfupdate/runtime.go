// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker/compat"
)

func EmbeddedHelper(endpoint string) (Helper, error) {
	engine, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		return Helper{}, err
	}
	return Helper{
		Up: func(ctx context.Context, project *composetypes.Project, service string) error {
			effects := &effectClient{APIClient: engine}
			cli, err := command.NewDockerCli(command.WithAPIClient(compat.WrapDockerAPIClientForInspectCompatibility(effects)), command.WithCombinedStreams(io.Discard))
			if err != nil {
				return preEffectError{err}
			}
			options := flags.NewClientOptions()
			options.Hosts = []string{endpoint}
			if err = cli.Initialize(options); err != nil {
				return preEffectError{err}
			}
			svc, err := compose.NewComposeService(cli, compose.WithPrompt(func(string, bool) (bool, error) { return false, nil }), compose.WithOutputStream(io.Discard), compose.WithErrorStream(io.Discard))
			if err != nil {
				return preEffectError{err}
			}
			err = svc.Up(ctx, project, api.UpOptions{Create: api.CreateOptions{Services: []string{service}, IgnoreOrphans: true, RemoveOrphans: false, Recreate: api.RecreateDiverged, RecreateDependencies: api.RecreateNever, Inherit: true}, Start: api.StartOptions{Project: project, Services: []string{service}}})
			if err != nil && !effects.started.Load() {
				return preEffectError{err}
			}
			return err
		},
		Healthy: func(ctx context.Context, request Request) error {
			filters := client.Filters{}.Add("label", api.ProjectLabel+"="+request.Replay.Project).Add("label", api.ServiceLabel+"="+request.Replay.Service).Add("label", api.OneoffLabel+"=False")
			for ctx.Err() == nil {
				list, err := engine.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: filters})
				if err == nil {
					for _, item := range list.Items {
						if item.ID == request.OldContainerID {
							continue
						}
						response, err := engine.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
						if err != nil {
							continue
						}
						c := response.Container
						if c.Config == nil || c.Config.Image != request.TargetReference || c.State == nil {
							continue
						}
						if c.State.Health != nil {
							if c.State.Health.Status == container.Healthy {
								return nil
							}
							if c.State.Health.Status == container.Unhealthy {
								return errors.New("replacement is unhealthy")
							}
						}
						if c.State.Status == container.StateExited || c.State.Status == container.StateDead {
							return errors.New("replacement stopped")
						}
					}
				}
				timer := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
			return ctx.Err()
		},
	}, nil
}

// HelperMounts maps the Unix socket and data as the daemon sees them. The
// endpoint inside the detached helper is always /var/run/docker.sock.
func HelperMounts(current container.InspectResponse, endpoint, data string, replay Replay) ([]mount.Mount, error) {
	socket, ok := strings.CutPrefix(endpoint, "unix://")
	if !ok || socket == "" {
		return nil, Refuse(CodeLayout)
	}
	var socketMount, dataMount *mount.Mount
	for _, m := range current.Mounts {
		if m.Destination == socket && m.Type == mount.TypeBind && m.Source != "" {
			socketMount = &mount.Mount{Type: mount.TypeBind, Source: m.Source, Target: "/var/run/docker.sock"}
		}
		if m.Destination == data && m.RW {
			switch m.Type {
			case mount.TypeBind:
				dataMount = &mount.Mount{Type: m.Type, Source: m.Source, Target: HelperData}
			case mount.TypeVolume:
				if m.Name != "" {
					dataMount = &mount.Mount{Type: m.Type, Source: m.Name, Target: HelperData}
				}
			}
		}
	}
	if socketMount == nil || dataMount == nil {
		return nil, Refuse(CodeLayout)
	}
	return []mount.Mount{*socketMount, {Type: mount.TypeBind, Source: replay.WorkingDir, Target: replay.WorkingDir}, *dataMount}, nil
}

// HelperCreate runs the helper as root: the operator's env file and project
// directory need not be accessible to the application identity. The helper
// gives its own new files the original owners (files.go ownLike).
func HelperCreate(request Request, mounts []mount.Mount) client.ContainerCreateOptions {
	return client.ContainerCreateOptions{Name: HelperName(request.OpID), Config: &container.Config{
		Image: request.TargetReference, Cmd: []string{"self-update", "--op", request.OpID},
		Env:         []string{"DOCKER_HOST=unix:///var/run/docker.sock"},
		User:        "0:0",
		Labels:      map[string]string{"io.docklet.helper": "self-update", "io.docklet.operation": request.OpID},
		Healthcheck: &container.HealthConfig{Test: []string{"NONE"}},
	}, HostConfig: &container.HostConfig{Mounts: mounts, RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled}}}
}
