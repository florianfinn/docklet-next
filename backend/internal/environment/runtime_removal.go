// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Runtime transaction hook for the existing environment removal boundary.

package environment

import (
	"context"
	"gorm.io/gorm"
)

// RuntimeRemoval owns the durable guard and tombstone in the same transaction
// as removing a registration. It is installed once by application construction.
type RuntimeRemoval func(context.Context, string, func(*gorm.DB) error) error

func (s *EnvironmentService) SetRuntimeRemoval(remove RuntimeRemoval) { s.runtimeRemoval = remove }
