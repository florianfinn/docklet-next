// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original global administrator lookup.

package user

import (
	"context"
	"errors"
)

func (s *UserService) GlobalAdministrator(ctx context.Context) (*User, error) {
	if s == nil || s.roleService == nil {
		return nil, errors.New("administrator service unavailable")
	}
	var users []User
	if err := s.db.WithContext(ctx).Order("created_at, id").Find(&users).Error; err != nil {
		return nil, err
	}
	for _, candidate := range users {
		if candidate.DisabledAt != nil {
			continue
		}
		permissions, err := s.roleService.ResolvePermissions(ctx, candidate.ID)
		if err != nil {
			return nil, err
		}
		if permissions != nil && permissions.IsGlobalAdmin() {
			return &candidate, nil
		}
	}
	return nil, errors.New("global administrator unavailable")
}
