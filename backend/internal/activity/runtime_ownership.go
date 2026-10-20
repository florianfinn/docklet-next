// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Receipt ownership protects generic activity writers during activation.

package activity

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// BindActivityOwner installs the sole runtime owner used by generic HTTP entry
// points. Production DI calls it once; receipt protection also works without it.
func (s *RuntimeService) BindActivityOwner() error {
	if err := s.checkInternal(); err != nil {
		return err
	}
	s.activity.runningMu.Lock()
	defer s.activity.runningMu.Unlock()
	if s.activity.runtime != nil && s.activity.runtime != s {
		return errRuntimeInvalidInternal
	}
	s.activity.runtime = s
	return nil
}

func (s *ActivityService) refuseRuntimeWriterInternal(ctx context.Context, id string) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&RuntimeReceipt{}).Where("run_id = ? AND kind = 'receipt'", id).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return ErrRunNotOwned
	}
	return nil
}

// Reference resolves durable ownership before remote fallback or fresh Docker
// inspection. Its caller must already have current activity access for this host.
func (s *RuntimeService) Reference(ctx context.Context, environmentID, runID string) (RuntimeRunReference, bool, error) {
	if err := s.checkInternal(); err != nil {
		return RuntimeRunReference{}, false, err
	}
	var row RuntimeReceipt
	err := s.activity.db.WithContext(ctx).First(&row, "environment_id = ? AND run_id = ? AND kind = 'receipt'", environmentID, runID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return RuntimeRunReference{}, false, nil
	}
	if err != nil {
		return RuntimeRunReference{}, false, &RuntimeStoreError{Cause: err}
	}
	return RuntimeRunReference{Store: runtimeStoreFromRowInternal(&row), RunID: row.RunID}, true, nil
}

// RetainedCommand restores the canonical parent/target and continuity tuple
// without contacting Docker. Caller identity is checked before any bound data.
func (s *RuntimeService) RetainedCommand(ctx context.Context, command RuntimeCommand) (RuntimeCommand, bool, error) {
	if err := s.checkInternal(); err != nil {
		return RuntimeCommand{}, false, err
	}
	key, err := runtimeUUIDInternal(command.Key)
	if err != nil {
		return RuntimeCommand{}, false, err
	}
	var binding RuntimeKeyBinding
	err = s.activity.db.WithContext(ctx).First(&binding, "environment_id = ? AND idempotency_key = ?", command.Store.EnvironmentID, key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return command, false, nil
	}
	if err != nil {
		return RuntimeCommand{}, false, &RuntimeStoreError{Cause: err}
	}
	if binding.CallerID != command.Actor.ID {
		return RuntimeCommand{}, false, &runtimeConflictInternal{}
	}
	var row RuntimeReceipt
	if err := s.activity.db.WithContext(ctx).First(&row, "run_id = ? AND kind = 'receipt'", binding.RunID).Error; err != nil {
		return RuntimeCommand{}, false, &RuntimeStoreError{Cause: err}
	}
	if row.TargetID == nil || row.TargetKind == nil || *row.TargetID != command.Target.ID || *row.TargetKind != command.Target.Kind {
		return RuntimeCommand{}, false, &runtimeConflictInternal{}
	}
	command.Store = runtimeStoreFromRowInternal(&row)
	command.Target = RuntimeTarget{Kind: *row.TargetKind, ID: *row.TargetID, Name: copyPtrInternal(row.TargetName), ParentProjectID: copyPtrInternal(row.ParentProjectID)}
	canonical, err := runtimeCanonicalizeInternal(command)
	if err != nil {
		return RuntimeCommand{}, false, err
	}
	if canonical.digest != binding.RequestDigest {
		return RuntimeCommand{}, false, &runtimeConflictInternal{}
	}
	return canonical.command, true, nil
}
