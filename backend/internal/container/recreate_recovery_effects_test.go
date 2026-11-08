// Source: docklet-next/issues/367
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original recovery effect outage, per-call budget and client classification proofs.

package container

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	dc "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

// effectFixture prepares the "app" journal so recovery reaches one Docker
// effect: compensation rename, start or remove of the replacement, or the
// forward removal of the original.
func effectFixture(t *testing.T, effect string) *recoveryFixture {
	t.Helper()
	f := newRecoveryFixture(t, "app")
	run := f.runs["app"]
	switch effect {
	case "start":
		old := f.d.containers[updateOldID]
		old.Name, old.State.Running = "/app", false
		f.d.containers[updateOldID] = old
	case "remove-new", "remove-old":
		j := RecreateJournal{}
		require.NoError(t, f.db.First(&j, "run_id=?", run).Error)
		j.NewID, j.Phase = updateNewID, "created"
		if effect == "remove-old" {
			j.Phase = "started"
		}
		require.NoError(t, f.db.Save(&j).Error)
		f.d.containers[updateNewID] = dc.InspectResponse{ID: updateNewID, Name: "/app", Image: updateNewImage, State: &dc.State{Running: effect == "remove-old"}, Config: &dc.Config{Labels: map[string]string{"io.docklet.run": run}}}
	}
	return f
}

// effectServer answers the chosen effect with 503 while faulty is set.
func effectServer(t *testing.T, f *recoveryFixture, effect string, faulty *atomic.Bool, faults *atomic.Int32) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit := strings.HasSuffix(r.URL.Path, "/"+effect) || strings.HasPrefix(effect, "remove") && r.Method == http.MethodDelete
		if hit && faulty.Load() {
			faults.Add(1)
			http.Error(w, "daemon unavailable", http.StatusServiceUnavailable)
			return
		}
		f.d.serve(w, r)
	}))
	t.Cleanup(server.Close)
	c, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	f.s.dockerService = docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(c)
}

// An unavailable daemon during a recovery effect is transient like a failed
// inspect: the run stays held from its committed intent and the next start
// settles it once, without a pull or create.
func TestRecreateRecoveryEffectOutageStaysRetryable(t *testing.T) {
	for _, effect := range []string{"rename", "start", "remove-new", "remove-old"} {
		t.Run(effect, func(t *testing.T) {
			withRecoveryTiming(t, 10*time.Second, 100*time.Millisecond, time.Millisecond)
			f := effectFixture(t, effect)
			faulty, faults := &atomic.Bool{}, &atomic.Int32{}
			faulty.Store(true)
			effectServer(t, f, effect, faulty, faults)
			before := f.journalPhase(t, "app")

			require.NoError(t, f.rt.InitializeLocalStartup(t.Context(), f.store), "startup continues")
			require.Equal(t, int32(recreateRecoveryTiming.attempts), faults.Load(), "each pass retries the effect")
			row := f.receipt(t, "app")
			require.Nil(t, row.SettledAt, "the run stays held")
			require.Equal(t, "unknown", *row.ExecutorState)
			require.NotEqual(t, "ambiguous", f.journalPhase(t, "app"), "an outage is no observed ambiguity")
			t.Logf("phase %s -> %s", before, f.journalPhase(t, "app"))

			faulty.Store(false)
			require.NoError(t, f.rt.InitializeLocalStartup(t.Context(), f.store))
			settled := f.receipt(t, "app").SettledAt
			require.NotNil(t, settled, "the next start settles the run")
			require.Contains(t, []string{"rolled-back", "done"}, f.journalPhase(t, "app"))
			require.Zero(t, f.d.counts["pull"])
			require.Zero(t, f.d.counts["create"])
			require.NoError(t, f.rt.InitializeLocalStartup(t.Context(), f.store))
			require.True(t, settled.Equal(*f.receipt(t, "app").SettledAt), "settled only once")
		})
	}
}

// Every recovery Docker effect carries the per-call budget, not only inspects.
func TestRecreateRecoveryEffectsHaveThePerCallBudget(t *testing.T) {
	withRecoveryTiming(t, 10*time.Second, 100*time.Millisecond, time.Millisecond)
	f := effectFixture(t, "rename")
	var remaining atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(f.d.serve))
	t.Cleanup(server.Close)
	c, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"), client.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/rename") {
			if deadline, ok := r.Context().Deadline(); ok {
				remaining.Store(int64(time.Until(deadline)))
			}
		}
		return http.DefaultTransport.RoundTrip(r)
	})}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	f.s.dockerService = docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(c)
	_, _, err = f.s.RecoverRecreates(t.Context())
	require.NoError(t, err)
	got := time.Duration(remaining.Load())
	require.Positive(t, got, "the rename ran with a deadline")
	require.LessOrEqual(t, got, 150*time.Millisecond, "the rename has the per-call budget, not the whole recovery budget")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Creating the Docker client uses the same closed classifier: an unavailable
// daemon holds the run, a refused one fails startup.
func TestRecreateRecoveryClassifiesClientErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		fatal  bool
	}{{http.StatusServiceUnavailable, false}, {http.StatusForbidden, true}} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			withRecoveryTiming(t, 2*time.Second, 100*time.Millisecond, time.Millisecond)
			f := newRecoveryFixture(t, "app")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, http.StatusText(tc.status), tc.status)
			}))
			t.Cleanup(server.Close)
			f.s.dockerService = docker.NewDockerClientService(context.Background(), nil, &config.Config{DockerHost: server.URL}, nil)
			_, pending, err := f.s.RecoverRecreates(t.Context())
			if tc.fatal {
				require.Error(t, err, "an authorisation failure is no unavailable daemon")
				require.Nil(t, f.receipt(t, "app").SettledAt)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{f.runs["app"]}, pending)
		})
	}
}
