// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original shared ownership across restore, safety and consumer recovery.

package backup

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/notification/backupnotify"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/restore"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
)

func (s *Service) configureRestoreLifecycle() {
	s.lifecycle = &restore.Lifecycle{DB: s.deps.DB, Activity: s.deps.Activity, Notify: func(ctx context.Context, r backupnotify.Run) { s.notifier.Terminal(ctx, r) }}
	s.restore.SetLifecycle(s.lifecycle)
	s.lifecycle.Effects = restore.LifecycleEffects{
		Validate: func(ctx context.Context, r restore.StartRequest) error {
			if err := s.restore.ValidateAdmission(ctx, r); err != nil {
				return err
			}
			if r.Mode != "new" {
				_, err := s.listConsumers(ctx, r.VolumeName, true)
				return err
			}
			return nil
		},
		Admit: func(ctx context.Context, source, destination string) (context.Context, func(), error) {
			owned, owner, err := s.acquireOperation(ctx, source, destination)
			if err != nil {
				return ctx, nil, err
			}
			return owned, func() { owner.Release(owned) }, nil
		},
		Prepare: s.restore.PrepareRestore,
		Consumers: func(ctx context.Context, name string) ([]volume.StoppedContainer, error) {
			return s.listConsumers(ctx, name, true)
		},
		Stop: func(ctx context.Context, c volume.StoppedContainer, u user.Actor) error {
			return s.deps.StopContainer(ctx, c.ID, u)
		},
		Restart: s.restartConsumers, Safety: s.safetyBackupInternal,
		Write: s.restore.WriteRestore, CleanupNew: s.restore.CleanupNew, Fence: s.retainWriterFence, PruneSafety: s.pruneSafety,
	}
}
