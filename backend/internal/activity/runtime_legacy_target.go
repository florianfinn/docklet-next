// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Trusted legacy alias preparation stays outside canonical execution.

package activity

import (
	"context"
	"errors"
	"strings"
)

// PrepareRuntimeTarget accepts the legacy container lookup vocabulary only at
// the trusted compatibility boundary; Execute and receiver Execute stay canonical.
func (a *RuntimeServiceAdapter) PrepareRuntimeTarget(ctx context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
	if target.Kind == "container" && !runtimeFullContainerID(target.ID) {
		if a == nil || a.containers == nil {
			return RuntimeResolvedTarget{}, errors.New("runtime services unavailable")
		}
		inspected, err := a.containers.GetContainerByID(ctx, target.ID)
		if err != nil {
			return RuntimeResolvedTarget{}, err
		}
		if inspected == nil || !runtimeFullContainerID(inspected.ID) {
			return RuntimeResolvedTarget{}, errRuntimeInvalidInternal
		}
		target.ID = inspected.ID
	}
	return a.ResolveRuntimeTarget(ctx, target)
}

func runtimeLegacyTargetMatches(request RuntimeTarget, row *RuntimeReceipt) bool {
	if row.TargetKind == nil || row.TargetID == nil || request.Kind != *row.TargetKind {
		return false
	}
	if request.ID == *row.TargetID {
		return true
	}
	if request.Kind != "container" || request.ID == "" {
		return false
	}
	if row.TargetName != nil && strings.TrimPrefix(request.ID, "/") == strings.TrimPrefix(*row.TargetName, "/") {
		return true
	}
	return len(request.ID) < len(*row.TargetID) && strings.HasPrefix(*row.TargetID, request.ID)
}

// HeldCommand is read only. Admit rechecks the receipt and caller permissions
// before joining or returning minimal conflict information; no fresh effect is
// admitted using this possibly older identity evidence.
func (s *RuntimeService) HeldCommand(ctx context.Context, command RuntimeCommand) (RuntimeCommand, bool, error) {
	if err := s.checkInternal(); err != nil {
		return RuntimeCommand{}, false, err
	}
	var rows []RuntimeReceipt
	if err := s.activity.db.WithContext(ctx).Where("kind = 'receipt' AND environment_id = ? AND settled_at IS NULL", command.Store.EnvironmentID).Order("run_id").Find(&rows).Error; err != nil {
		return RuntimeCommand{}, false, &RuntimeStoreError{Cause: err}
	}
	for i := range rows {
		row := &rows[i]
		exact := runtimeLegacyTargetMatches(command.Target, row)
		parent := command.Target.Kind == "stack" && row.ParentProjectID != nil && *row.ParentProjectID == command.Target.ID
		if !exact && !parent {
			continue
		}
		command.Store = runtimeStoreFromRowInternal(row)
		if exact {
			command.Target = RuntimeTarget{Kind: *row.TargetKind, ID: *row.TargetID, Name: copyPtrInternal(row.TargetName), ParentProjectID: copyPtrInternal(row.ParentProjectID)}
		}
		return command, true, nil
	}
	return command, false, nil
}
