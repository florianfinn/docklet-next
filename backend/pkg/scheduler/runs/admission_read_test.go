// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package runs

import (
	"context"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
	kit "go.getarcane.app/kit/pkg"
	"testing"
)

func TestAdmissionHeldIsReadOnlyAndPreservesUncertainty(t *testing.T) {
	runtime := francistest.New(t)
	a := NewAdmission(runtime.Service(), "current")
	require.NoError(t, a.Register(runtime))
	francistest.Start(t, runtime)
	key := st.AdmissionKey{Scope: "volume-backup", ID: "data"}
	held, err := a.Held(t.Context(), key)
	require.NoError(t, err)
	require.False(t, held)
	lease, ok, err := a.TryAcquire(t.Context(), key)
	require.NoError(t, err)
	require.True(t, ok)
	id := kit.SHA256Hex(key.Scope + "\x00" + key.ID)
	var before, after st.AdmissionState
	require.NoError(t, a.service.GetState(t.Context(), admissionTypeInternal, id, &before))
	held, err = a.Held(t.Context(), key)
	require.NoError(t, err)
	require.True(t, held)
	require.NoError(t, a.service.GetState(t.Context(), admissionTypeInternal, id, &after))
	require.Equal(t, before, after)
	next := NewAdmission(runtime.Service(), "next")
	held, err = next.Held(t.Context(), key)
	require.NoError(t, err)
	require.False(t, held, "old epochs do not own current leases")
	lease.Release(t.Context())
	held, err = a.Held(t.Context(), key)
	require.NoError(t, err)
	require.False(t, held)
	a.mu.Lock()
	a.pending[id] = map[string]st.AdmissionCommand{"pending": {Epoch: "current"}}
	a.mu.Unlock()
	held, err = a.Held(t.Context(), key)
	require.NoError(t, err)
	require.True(t, held)
	require.Len(t, a.pending[id], 1)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	held, err = a.Held(ctx, key)
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, held)
	var missing *Admission
	held, err = missing.Held(t.Context(), key)
	require.Error(t, err)
	require.True(t, held)
}
