// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original same-instance protected-interval admission proof.

package proxy

import (
	"sync"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestP13SameInstanceDistinctOperationsStayOutsideProtectedInterval(t *testing.T) {
	x := newFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	type result struct {
		operation dto.Operation
		err       error
	}
	done := make(chan result, 1)
	x.e.observe = func() Observation {
		if x.e.observations == 1 {
			close(entered)
			<-release
		}
		return Observation{State: dto.Verified, Evidence: dto.Routing}
	}
	go func() { o, err := x.a.Apply(t.Context(), x.request); done <- result{o, err} }()
	<-entered
	second := cloneRequest(x.request)
	second.ID = "same-instance-second"
	second.Target = "other.yml"
	second.Expected = dto.FileState{}
	_, err := x.a.Apply(t.Context(), second)
	require.ErrorContains(t, err, "module-busy")
	require.Equal(t, 1, x.f.publications)
	require.Equal(t, 1, x.e.observations, "only the first operation is inside the protected interval")
	j, err := findJournal(t.Context(), x.db.DB, x.request.ID)
	require.NoError(t, err)
	require.Nil(t, j.FinishedAt)
	once.Do(func() { close(release) })
	first := <-done
	require.NoError(t, first.err)
	require.Equal(t, dto.Verified, first.operation.State)
	x.e.observe = nil
	second.GraphHashes, err = x.f.Graph(t.Context(), second.Instance)
	require.NoError(t, err)
	o, err := x.a.Apply(t.Context(), second)
	require.NoError(t, err)
	require.Equal(t, dto.Verified, o.State)
	require.Equal(t, 2, x.f.publications)
}
