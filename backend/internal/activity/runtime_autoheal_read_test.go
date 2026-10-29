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
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAutoHealRuntimeOwnershipResolvesParentAndReadsLocalGates(t *testing.T) {
	for _, role := range []string{"hub", "agent"} {
		t.Run(role, func(t *testing.T) {
			f := b1Fixture(t)
			parent := "example-stack"
			f.resolver.resolve = func(_ context.Context, v RuntimeTarget) (RuntimeResolvedTarget, error) {
				if v.Kind == "container" {
					v.ParentProjectID = &parent
				}
				return RuntimeResolvedTarget{Target: v, ComposeManaged: v.Kind == "container"}, nil
			}
			held, e := f.service.HeldForContainer(t.Context(), "container", "compose-label")
			require.NoError(t, e)
			require.False(t, held)
			command := f.command()
			_, e = f.service.Admit(t.Context(), command)
			require.NoError(t, e)
			// Only the receipt remains active: no gate shortcut may mask parent resolution.
			require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("1=1").Update("status", dto.StatusSuccess).Error)
			held, e = f.service.HeldForContainer(t.Context(), "container", "compose-label")
			require.NoError(t, e)
			require.True(t, held)
			require.NoError(t, f.store.svc.db.Where("1=1").Delete(&RuntimeKeyBinding{}).Error)
			require.NoError(t, f.store.svc.db.Where("1=1").Delete(&RuntimeReceipt{}).Error)
			for _, scope := range []string{"container:container", "stack:" + parent} {
				for _, status := range []dto.Status{dto.StatusQueued, dto.StatusRunning} {
					a := Activity{ID: "policy-child", EnvironmentID: "0", Type: dto.TypeProjectDeploy, Status: status, GateScope: scope, GateKey: "policy-child"}
					require.NoError(t, f.store.svc.db.Create(&a).Error)
					held, e = f.service.HeldForContainer(t.Context(), "container", "compose-label")
					require.NoError(t, e)
					require.True(t, held)
					require.NoError(t, f.store.svc.db.Delete(&a).Error)
				}
			}
			f.resolver.resolve = func(context.Context, RuntimeTarget) (RuntimeResolvedTarget, error) {
				return RuntimeResolvedTarget{}, errors.New("read failed")
			}
			held, e = f.service.HeldForContainer(t.Context(), "container", "")
			require.True(t, held)
			require.Error(t, e)
			f.resolver.resolve = func(_ context.Context, v RuntimeTarget) (RuntimeResolvedTarget, error) {
				return RuntimeResolvedTarget{Target: v}, nil
			}
			require.NoError(t, f.store.svc.db.Migrator().DropTable(&RuntimeReceipt{}))
			held, e = f.service.HeldForContainer(t.Context(), "container", "")
			require.True(t, held)
			require.Error(t, e)
		})
	}
}
