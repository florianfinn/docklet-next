// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original exact-container recovery ownership for volume operations.

package backup

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func (s *Service) listConsumers(ctx context.Context, name string, refuseSelf bool) ([]volume.StoppedContainer, error) {
	cli, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	result, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	stopped := []volume.StoppedContainer{}
	for _, candidate := range result.Items {
		uses := false
		for _, m := range candidate.Mounts {
			if m.Name == name {
				uses = true
				break
			}
		}
		if !uses {
			continue
		}
		self := strings.EqualFold(candidate.Labels["com.getarcaneapp.arcane"], "true") || strings.EqualFold(candidate.Labels["com.getarcaneapp.arcane.agent"], "true")
		if refuseSelf && self {
			return nil, dto.NewProblem(409, "volume.restore.self-mounted", nil)
		}
		if self || candidate.Labels[libarcane.InternalResourceLabel] == "true" {
			continue
		}
		if candidate.State != container.StateRunning {
			continue
		}
		display := name
		if len(candidate.Names) > 0 {
			display = strings.TrimPrefix(candidate.Names[0], "/")
		}
		stopped = append(stopped, volume.StoppedContainer{ID: candidate.ID, Name: display})
	}
	return stopped, nil
}

// Recovery uses only the recorded ID. A replacement with the same name or
// Compose labels is a different consumer and cannot inherit this permission.
func (s *Service) restartConsumers(ctx context.Context, stopped []volume.StoppedContainer, actor user.Actor) error {
	if len(stopped) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cli, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	var failures []error
	for _, consumer := range stopped {
		if seen[consumer.ID] {
			continue
		}
		seen[consumer.ID] = true
		current, err := cli.ContainerInspect(ctx, consumer.ID, client.ContainerInspectOptions{})
		if errdefs.IsNotFound(err) {
			continue
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if current.Container.State != nil && current.Container.State.Running {
			continue
		}
		if s.deps.StartContainer == nil {
			failures = append(failures, errors.New("container service unavailable"))
			continue
		}
		if err = s.deps.StartContainer(ctx, consumer.ID, actor); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
