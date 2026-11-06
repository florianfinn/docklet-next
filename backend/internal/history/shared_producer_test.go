// Source: docklet-next/issues/352
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original concurrent real-producer and caller-owned sample guard.

package history

import (
	"context"
	dto "github.com/florianfinn/docklet-next/types/history"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestM19SharedReadRealProducer(t *testing.T) {
	db := historyDB(t)
	fake := &historyDocker{id: "first", state: "running", started: "2026-11-03T10:00:00Z"}
	daemon := httptest.NewServer(http.HandlerFunc(fake.serve))
	defer daemon.Close()
	cli, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.41"), client.WithHTTPClient(daemon.Client()))
	require.NoError(t, err)
	defer cli.Close()
	producer := &Producer{db: db, getClient: func(context.Context) (*client.Client, error) { return cli, nil }, hostSample: func(context.Context) (dto.HostSample, error) { return fixtureSample().Host, nil }, inspectTimeout: time.Second}
	var wg sync.WaitGroup
	samples := make([]dto.Sample, 8)
	errs := make([]error, 8)
	start := make(chan struct{})
	for i := range samples {
		wg.Go(func() { <-start; samples[i], errs[i] = producer.Sample(t.Context()) })
	}
	close(start)
	wg.Wait()
	for i, sample := range samples {
		require.NoError(t, errs[i])
		require.Len(t, sample.Containers, 1)
		require.Equal(t, "first", sample.Containers[0].ID)
		require.Equal(t, "app", sample.Containers[0].Name)
		require.Equal(t, float64(100), sample.Containers[0].Stats.MemoryUsage)
	}
	copied := cloneSample(samples[0])
	copied.Containers[0].Name = "edited"
	copied.Containers[0].Stats.MemoryUsage = 1
	for _, sample := range samples {
		require.Equal(t, "app", sample.Containers[0].Name)
		require.Equal(t, float64(100), sample.Containers[0].Stats.MemoryUsage)
	}
}
