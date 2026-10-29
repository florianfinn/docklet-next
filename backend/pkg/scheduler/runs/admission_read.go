// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package runs

import (
	"context"
	"errors"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/italypaleale/francis/actor"
	kit "go.getarcane.app/kit/pkg"
)

// Held reads current-epoch ownership without acquiring or releasing a lease.
// Errors preserve uncertainty so callers cannot mistake an unreadable store for idle.
func (a *Admission) Held(ctx context.Context, key st.AdmissionKey) (bool, error) {
	if err := ctx.Err(); err != nil {
		return true, err
	}
	if a == nil || a.service == nil {
		return true, errors.New("admission snapshot unavailable")
	}
	id := kit.SHA256Hex(key.Scope + "\x00" + key.ID)
	a.mu.Lock()
	pending := len(a.pending[id]) > 0
	a.mu.Unlock()
	if pending {
		return true, nil
	}
	var state st.AdmissionState
	if err := a.service.GetState(ctx, admissionTypeInternal, id, &state); err != nil {
		if errors.Is(err, actor.ErrStateNotFound) {
			return false, nil
		}
		return true, err
	}
	return state.Epoch == a.epoch && state.Token != "", nil
}
