// Source: docklet-next/issues/676
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original scheduled ownership lifetime regression coverage.

package backup

import (
	"testing"

	sharedbackup "github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestScheduledOwnershipIsReusedWithoutReleasingAdmission(t *testing.T) {
	engine := sharedbackup.NewEngine(t.Context(), newVolumeAdmissionForTestInternal(t), nil)
	locks := &utils.KeyedMutex{}
	s := NewService(Dependencies{Engine: engine, Locks: locks})
	owned, outer, err := s.acquireOperation(t.Context(), "data", "")
	require.NoError(t, err)
	defer outer.Release(owned)
	// This is the nested acquisition performed by CreateBackup after policy admission.
	_, inner, err := s.acquireOperation(owned, "data", "")
	require.NoError(t, err)
	inner.Release(owned)
	lease, admitted, err := engine.TryAcquireRun(t.Context(), sharedbackup.VolumeAdmissionScope, "data")
	require.NoError(t, err)
	if admitted {
		lease.Release(t.Context())
	}
	require.False(t, admitted, "nested completion released the policy's durable lease")
	unlock, ok := locks.TryLock("data")
	if ok {
		unlock()
	}
	require.False(t, ok)
	outer.Release(owned)
	lease, admitted, err = engine.TryAcquireRun(t.Context(), sharedbackup.VolumeAdmissionScope, "data")
	require.NoError(t, err)
	require.True(t, admitted)
	lease.Release(t.Context())
	unlock, ok = locks.TryLock("data")
	require.True(t, ok)
	unlock()
}
