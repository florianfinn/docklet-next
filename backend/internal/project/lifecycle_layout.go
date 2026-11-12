// Source: docklet-next/issues/406
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original typed refusal for stack layouts updates cannot load.

package project

import (
	"context"
	"errors"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
)

// UnsupportedUpdateLayout returns the typed update refusal when err comes from a
// stack layout updates cannot load (a nested COMPOSE_FILE base), else nil.
func UnsupportedUpdateLayout(err error) error {
	if errors.Is(err, projects.ErrComposeFileNestedBase) {
		return lifecycleFailure("update.image-binding-unsupported")
	}
	return nil
}

// UpdateLayoutRefusal checks only the Compose selection, so update checks refuse
// such a stack before reading details that cannot resolve its services.
func (s *ProjectService) UpdateLayoutRefusal(ctx context.Context, id string) error {
	p, err := s.GetProjectFromDatabaseByID(ctx, id)
	if err != nil || p == nil {
		return nil
	}
	_, err = s.ResolveProjectComposeFile(ctx, p)
	return UnsupportedUpdateLayout(err)
}
