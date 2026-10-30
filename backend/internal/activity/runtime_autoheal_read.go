// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package activity

import (
	"context"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/activity"
)

// HeldForContainer resolves the persisted parent before applying the admission predicate.
// It reads receipts and gates on this executing host, including agent-local stores.
func (s *RuntimeService) HeldForContainer(ctx context.Context, containerID, projectID string) (bool, error) {
	if s == nil || s.activity == nil || s.activity.db == nil || s.resolver == nil {
		return true, errors.New("runtime snapshot unavailable")
	}
	resolved, err := s.resolver.ResolveRuntimeTarget(ctx, RuntimeTarget{Kind: "container", ID: containerID})
	if err != nil {
		return true, err
	}
	if resolved.Target.ID != containerID {
		return true, errors.New("container identity changed")
	}
	if resolved.ComposeManaged && resolved.Target.ParentProjectID == nil {
		return true, errors.New("parent identity unresolved")
	}
	if resolved.Target.ParentProjectID != nil {
		projectID = *resolved.Target.ParentProjectID
	}
	scopes := []string{"container:" + containerID}
	predicate := "gate_scope = ?"
	args := []any{scopes[0]}
	if projectID != "" {
		scopes = append(scopes, "stack:"+projectID)
		predicate += " OR (target_kind='stack' AND target_id=?)"
		args = append(args, projectID)
	}
	var count int64
	if err := s.activity.db.WithContext(ctx).Model(&RuntimeReceipt{}).Where("kind='receipt' AND environment_id='0' AND settled_at IS NULL").Where("("+predicate+")", args...).Count(&count).Error; err != nil {
		return true, err
	}
	if count > 0 {
		return true, nil
	}
	if err := s.activity.db.WithContext(ctx).Model(&Activity{}).Where("environment_id='0' AND gate_scope IN ? AND status IN ?", scopes, []dto.Status{dto.StatusQueued, dto.StatusRunning}).Count(&count).Error; err != nil {
		return true, err
	}
	return count > 0, nil
}
