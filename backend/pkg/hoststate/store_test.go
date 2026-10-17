// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package hoststate

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSnapshotDisconnectReconnectAndRestart(t *testing.T) {
	store := &Store{}
	key := Key{HostID: "example-host", List: "containers"}
	data := []byte(`["first"]`)
	read := store.Put(key, data)
	data[0] = 'x'
	read.Data[0] = 'x'
	stale, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, `["first"]`, string(stale.Data))
	require.False(t, stale.ObservedAt.IsZero())
	other, ok := store.Get(Key{HostID: "other-host", List: "containers"})
	require.False(t, ok)
	require.Empty(t, other.Data)
	_, exists := (&Store{}).Get(key)
	require.False(t, exists)
	fresh := store.Put(key, []byte(`["reconnected"]`))
	require.False(t, fresh.ObservedAt.Before(stale.ObservedAt))
	got, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, `["reconnected"]`, string(got.Data))
	store.Forget(key.HostID)
	_, ok = store.Get(key)
	require.False(t, ok)
}
