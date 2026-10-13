// Source: getarcaneapp/arcane/backend/pkg/utils/userctx/context.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package userctx

import (
	"context"

	"github.com/florianfinn/docklet-next/types/user"
)

// CurrentUserFromContext retrieves the actor for an authenticated request.
func CurrentUserFromContext(ctx context.Context) (*user.Actor, bool) {
	value := ctx.Value(user.CurrentUserContextKey{})
	if actor, ok := value.(*user.Actor); ok {
		return actor, true
	}
	provider, ok := value.(user.ActorProvider)
	if !ok {
		return nil, false
	}
	return provider.Actor(), true
}
