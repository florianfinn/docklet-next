// Source: getarcaneapp/arcane/backend/internal/user/helpers.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package user

import (
	"context"

	"github.com/florianfinn/docklet-next/types/user"
)

// CurrentUserFromContext retrieves the authenticated persistence model.
func CurrentUserFromContext(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(user.CurrentUserContextKey{}).(*User)
	return u, ok
}
