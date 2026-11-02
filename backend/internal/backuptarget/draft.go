// Source: docklet-next/issues/289
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backuptarget

import (
	"context"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"go.getarcane.app/sys/crypto"
)

// Draft never persists a target, key or check result.
func (s *Service) Draft(ctx context.Context, request dto.TargetDraftTest) (dto.TargetInput, error) {
	input, err := Validate(request.Target)
	if err != nil {
		return input, err
	}
	if input.HostID != nil && *input.HostID != request.ProbeHostID {
		return input, dto.NewProblem(422, "backup.target.probe-host-mismatch", nil)
	}
	if request.TargetID != "" {
		var row Target
		if err = s.deps.Store.DB.WithContext(ctx).First(&row, "id = ?", request.TargetID).Error; err != nil {
			return input, err
		}
		if !sameLocation(row.location(), input.TargetLocation) {
			return input, dto.NewProblem(422, "backup.target.immutable", nil)
		}
		if input.Secret == nil && row.SecretEnc != "" {
			secret, e := crypto.Decrypt(row.SecretEnc)
			if e != nil {
				return input, e
			}
			input.Secret = &secret
		}
	}
	return Validate(input)
}
func DraftTarget(input dto.TargetInput) dto.SyncedTarget {
	secret := ""
	if input.Secret != nil {
		secret = *input.Secret
	}
	return dto.SyncedTarget{TargetLocation: input.TargetLocation, Name: input.Name, Username: input.Username, Secret: secret, Options: input.Options}
}
