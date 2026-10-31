// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original session matrix and concurrent handle identity guards.

package containershell

import (
	"context"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

type audit struct {
	mu     sync.Mutex
	starts []activitylib.StartRequest
	ends   map[string]int
}

func (a *audit) StartActivity(_ context.Context, req activitylib.StartRequest) (*activity.Activity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.starts = append(a.starts, req)
	return &activity.Activity{ID: string(rune('a' + len(a.starts)))}, nil
}
func (a *audit) CompleteActivity(_ context.Context, id string, _ activity.Status, _ string, _ *string, _ ...string) (*activity.Activity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ends[id]++
	return &activity.Activity{ID: id}, nil
}
func TestQueryBeforeActivity(t *testing.T) {
	for _, raw := range []string{"shell=/bin/sh", "shell=", "cols=0", "rows=1001", "cols=hello", "shell=sh&shell=bash", "unexpected=1"} {
		a := &audit{ends: map[string]int{}}
		r := New(a)
		_, _, err := r.Accept(t.Context(), mustQuery(raw), &user.Actor{ID: "example-user"}, "example-host", "example-container")
		require.Error(t, err)
		require.Empty(t, a.starts)
		require.Empty(t, r.handles)
	}
	q, err := ParseQuery(mustQuery("shell=auto&cols=80&rows=24"))
	require.NoError(t, err)
	require.Equal(t, 80, *q.Cols)
}
func mustQuery(raw string) url.Values { q, _ := url.ParseQuery(raw); return q }
func TestSessionMatrixReleaseAndReplacementIdentity(t *testing.T) {
	for _, event := range []string{"upgrade-failure", "dial-failure", "EOF", "client-close", "container-stop", "revocation", "credential-change", "removal", "replacement", "shutdown"} {
		t.Run(event, func(t *testing.T) {
			a := &audit{ends: map[string]int{}}
			r := New(a)
			parent, cancel := context.WithCancel(t.Context())
			defer cancel()
			_, old, err := r.Accept(parent, url.Values{}, &user.Actor{ID: "example-user"}, "example-host", "example-container")
			require.NoError(t, err)
			var oldCloses, newCloses atomic.Int32
			old.SetCloser(func() { oldCloses.Add(1) })
			old.Close()
			_, replacement, err := r.Accept(t.Context(), url.Values{}, &user.Actor{ID: "example-user"}, "example-host", "example-container")
			require.NoError(t, err)
			replacement.SetCloser(func() { newCloses.Add(1) })
			var workers sync.WaitGroup
			for range 20 {
				workers.Go(func() { old.Close(); old.Release() })
			}
			workers.Wait()
			require.EqualValues(t, 1, oldCloses.Load())
			require.Zero(t, newCloses.Load())
			require.Len(t, r.handles, 1)
			require.Equal(t, 1, a.ends[old.activityID])
			require.Zero(t, a.ends[replacement.activityID])
			require.Equal(t, "example-user", a.starts[0].StartedBy.ID)
			require.Equal(t, "example-user", a.starts[1].StartedBy.ID)
			replacement.Release()
			require.Empty(t, r.handles)
			require.Equal(t, 1, a.ends[replacement.activityID])
		})
	}
}
func TestCloseByUserIsolationAndBeforeBind(t *testing.T) {
	r := New(nil)
	ctx, a, err := r.Accept(t.Context(), url.Values{}, &user.Actor{ID: "example-user"}, "example-host", "example-container")
	require.NoError(t, err)
	defer a.Release()
	other, b, err := r.Accept(t.Context(), url.Values{}, &user.Actor{ID: "example-other"}, "example-host", "example-container")
	require.NoError(t, err)
	defer b.Release()
	r.CloseByUser("example-user")
	require.Error(t, ctx.Err())
	require.NoError(t, other.Err())
	var closes atomic.Int32
	a.SetCloser(func() { closes.Add(1) })
	a.Close()
	require.EqualValues(t, 1, closes.Load())
}

func TestApplicationShutdownClosesOwnedSession(t *testing.T) {
	owner, stop := context.WithCancel(t.Context())
	r := New(nil, owner)
	ctx, h, err := r.Accept(t.Context(), url.Values{}, &user.Actor{ID: "example-user"}, "example-host", "example-container")
	require.NoError(t, err)
	closed := make(chan struct{})
	h.SetCloser(func() { close(closed) })
	stop()
	<-closed
	require.Error(t, ctx.Err())
	h.Release()
	require.Empty(t, r.handles)
}
