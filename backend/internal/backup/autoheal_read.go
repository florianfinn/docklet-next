// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package backup

import (
	"context"
	"errors"
	st "github.com/florianfinn/docklet-next/types/scheduler"
)

func (e *Engine) HeldForVolume(ctx context.Context, name string) (bool, error) {
	if e == nil || e.admission == nil {
		return true, errors.New("backup admission unavailable")
	}
	return e.admission.Held(ctx, st.AdmissionKey{Scope: VolumeAdmissionScope, ID: name})
}
