// Source: docklet-next/issues/193
// Revision: fdbb8c1fae620d9f97a485983cdaabb3fa53bca2
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original lock retention and independent release coverage.

package operationlock

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRetainedOperationOwnsOriginalReadAndWriteLock(t *testing.T) {
	for _, write := range []bool{false, true} {
		t.Run(map[bool]string{false: "read", true: "write"}[write], func(t *testing.T) {
			locks := &utils.KeyedMutex{}
			lock := locks.RLock
			if write {
				lock = locks.Lock
			}
			ctx, release := WithContext(t.Context(), "data", lock("data"))
			first, second := Retain(ctx, "data"), Retain(ctx, "data")
			release()
			release()
			first()
			first()
			unlock, acquired := locks.TryLock("data")
			if acquired {
				unlock()
			}
			require.False(t, acquired, "remaining waiter retains the original lock")
			other, acquired := locks.TryLock("other")
			require.True(t, acquired)
			other()
			second()
			second()
			unlock, acquired = locks.TryLock("data")
			require.True(t, acquired, "last completion releases the original lock")
			unlock()
		})
	}
}

func TestUnrelatedContextDoesNotRetainOperation(t *testing.T) {
	released := false
	ctx, release := WithContext(t.Context(), "data", func() { released = true })
	other := Retain(ctx, "other")
	release()
	require.True(t, released)
	other()
	Retain(context.Background(), "data")()
}
