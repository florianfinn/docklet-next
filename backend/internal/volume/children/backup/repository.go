// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original historical-destination resolver with AUTO-9 mount witness.

package backup

import (
	"context"
	"strings"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type admittedTargetKey struct{}

func (s *Service) repositoryFor(ctx context.Context, dockerClient *client.Client, destination string, readOnly bool) (backup.Repository, error) {
	if destination == "" || destination == "local" {
		repository, err := s.localRusticRepositoryInternal(ctx, dockerClient, readOnly)
		return s.observeRepository(repository, destination), err
	}
	hostID := "0"
	var host struct{ Value string }
	err := s.deps.DB.WithContext(ctx).Table("settings").Where("key = ?", "hubHostId").Scan(&host).Error
	if err != nil {
		return backup.Repository{}, err
	}
	if host.Value != "" {
		hostID = host.Value
	}
	target, frozen := ctx.Value(admittedTargetKey{}).(dto.SyncedTarget)
	if !frozen || target.ID != destination {
		target, err = s.targetStore.Resolve(ctx, destination, hostID, false)
	}
	if err != nil {
		return backup.Repository{}, err
	}
	plan := backuptarget.Plan(target, hostID, readOnly)
	if plan.Volume != nil {
		if _, err = dockerClient.VolumeCreate(ctx, *plan.Volume); err != nil {
			return backup.Repository{}, dto.NewProblem(503, "backup.target.mount-failed", err)
		}
	}
	repository := backup.Repository{ID: "target:" + target.ID + ":" + hostID, Environment: []string{"RUSTIC_REPOSITORY=" + plan.RepositoryPath}, Mounts: plan.Mounts, Password: target.RecoveryKey}
	if plan.Witness {
		repository.BeforeRun = func(ctx context.Context) error {
			image, err := s.deps.HelperImage(ctx, dockerClient)
			if err != nil {
				return err
			}
			created, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{Config: &container.Config{Image: image, Cmd: []string{"sleep", "infinity"}, Labels: rustic.OperationLabels(ctx)}, HostConfig: volumehelper.HostConfig(image, nil, plan.Mounts)})
			if err != nil {
				return dto.NewProblem(503, "backup.target.mount-failed", err)
			}
			defer waitForBackupHelperRemoval(ctx, dockerClient, created.ID)
			if _, err = dockerClient.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
				return dto.NewProblem(503, "backup.target.mount-failed", err)
			}
			output, _, err := s.deps.Exec(ctx, created.ID, "", []string{"stat", "-c", "%d", "/target", "/parent"})
			if err != nil {
				return dto.NewProblem(503, "backup.target.mount-failed", err)
			}
			return backuptarget.CheckWitness(strings.TrimSpace(output))
		}
	}
	return s.observeRepository(repository, destination), nil
}
