// Source: docklet-next/issues/419
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original failed-transaction and concurrent real sampler cache-isolation proof.

package history

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	dto "github.com/florianfinn/docklet-next/types/history"
	"github.com/stretchr/testify/require"
)

func TestM20SeedDoesNotPublishOnCommitFailure(t *testing.T) {
	s, host, now := samplerFixture(t)
	sample := fixtureSample()
	points := []restartPoint{{Minute: now.Unix(), Count: 2}}
	encoded, err := json.Marshal(points)
	require.NoError(t, err)
	condition, err := json.Marshal(dto.Condition{Kind: "container", ContainerID: "first", Name: "app", Condition: dto.Running})
	require.NoError(t, err)
	require.NoError(t, s.db.Create(&conditionRow{HostID: host.ID, Kind: "container", EntityID: "first", Revision: hostRevision(host), Condition: string(condition), Restarts: string(encoded)}).Error)
	// Cached cold conditions cause persisted evidence to seed on the next real sample.
	cached := map[string][]restartPoint{"kept": {{Minute: now.Unix() - 60, Count: 1}}}
	s.states[host.ID] = hostState{revision: hostRevision(host), restarts: cached}
	// A deferred FK fails at COMMIT, after all sampling/seed work has succeeded.
	require.NoError(t, s.db.Exec("CREATE TABLE seed_commit_guard (id INTEGER REFERENCES environments(id) DEFERRABLE INITIALLY DEFERRED)").Error)
	require.NoError(t, s.db.Exec("CREATE TRIGGER seed_commit_failure AFTER INSERT ON history_buckets BEGIN INSERT INTO seed_commit_guard VALUES (987654321); END").Error)
	s.fetch = func(context.Context, environment.Environment) (dto.Sample, error) { return sample, nil }
	require.ErrorContains(t, s.SampleHost(t.Context(), host, now), "FOREIGN KEY constraint failed")
	require.Equal(t, map[string][]restartPoint{"kept": {{Minute: now.Unix() - 60, Count: 1}}}, cached, "failed commit must not seed into a shared cached map")
	require.Empty(t, s.snapshots.read(host.ID).Items)
	require.NoError(t, s.db.Exec("DROP TRIGGER seed_commit_failure").Error)
	// The actual sampler and concurrent readers run under the race detector.
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 20 {
			_, err := s.Conditions(t.Context(), host.ID)
			require.NoError(t, err)
		}
	})
	wg.Go(func() {
		for i := range 20 {
			require.NoError(t, s.SampleHost(t.Context(), host, now.Add(time.Duration(i)*time.Minute)))
		}
	})
	wg.Wait()
	require.NotEmpty(t, s.snapshots.read(host.ID).Items)
	require.NotContains(t, cached, "first", "successful seeding also detaches the old reader's map")
}
