// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original hub retire fence and exact accepted-set policy admission.

package backup

import (
	"cmp"
	"context"
	"gorm.io/gorm"
	"strings"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	dto "github.com/florianfinn/docklet-next/types/backup"
)

func (s *Service) validatePolicyTarget(ctx context.Context, id string) error {
	if id == "" || id == "local" {
		return nil
	}
	host, err := s.hostID(ctx)
	if err != nil {
		return err
	}
	_, err = s.targetStore.Resolve(ctx, id, host, true)
	return err
}
func (s *Service) aroundPolicyUpdate(environments *environment.EnvironmentService, cfg *config.Config) func(context.Context, *policies.UpdateVolumeBackupPolicyInput, func() error) error {
	return func(ctx context.Context, input *policies.UpdateVolumeBackupPolicyInput, next func() error) error {
		if len(input.Body.Policies) > 1 {
			return dto.NewProblem(422, "volume.backup.policy-single", nil)
		}
		agent := cfg != nil && (cfg.AgentMode || cfg.EdgeAgent)
		if agent {
			return next()
		}
		target := "local"
		if len(input.Body.Policies) > 0 {
			target = cmp.Or(input.Body.Policies[0].TargetID, "local")
		}
		return s.targets.WithActive(ctx, target, func() error {
			for attempt := 0; attempt < 2; attempt++ {
				if input.EnvironmentID == "0" {
					authority, version, err := s.targetStore.Metadata(ctx, false)
					if err != nil {
						return err
					}
					input.Body.Authority = authority
					input.Body.SetVersion = version
				} else {
					if environments == nil {
						return dto.NewProblem(503, "backup.target.sync-failed", nil)
					}
					receipt, err := backuptarget.SyncHost(ctx, s.deps.DB, input.EnvironmentID, environments.ProxyJSONRequest)
					if err != nil {
						return err
					}
					input.Body.Authority = receipt.Authority
					input.Body.SetVersion = receipt.SetVersion
				}
				err := next()
				if err == nil || attempt == 1 || !strings.Contains(err.Error(), "backup.target.stale-set") {
					return err
				}
			}
			return nil
		})
	}
}

func (s *Service) checkPolicySet(ctx context.Context, input *policies.UpdateVolumeBackupPolicyInput) error {
	authority, version, err := s.targetStore.Metadata(ctx, true)
	if err != nil {
		return err
	}
	if authority == "" || input.Body.Authority != authority || input.Body.SetVersion != version {
		return dto.NewProblem(409, "backup.target.stale-set", nil)
	}
	return nil
}

// RemovePolicies drops the backup schedules of a removed volume.
func (s *Service) RemovePolicies(ctx context.Context, volumeName string) {
	s.policies.Remove(ctx, volumeName)
}

// RenamePolicies moves a renamed volume's backup policies inside the caller's transaction.
func (s *Service) RenamePolicies(tx *gorm.DB, oldName, newName string) error {
	return s.policies.Rename(tx, oldName, newName)
}
