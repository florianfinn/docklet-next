// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing review counterexamples (C-B2, C-N1); the
// rejected-bytes case also accepts a completed restore, which removes the new bytes.

package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"

	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	dockerinternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"testing"
)

type reviewerEpochRuntime struct {
	Runtime
	call, at int
}

func (r *reviewerEpochRuntime) Epoch(ctx context.Context, i dto.Instance) (bool, string, error) {
	r.call++
	if r.call == r.at {
		return false, "", errors.New("temporary daemon failure")
	}
	return r.Runtime.Epoch(ctx, i)
}
func TestClosingReviewerTransientEpochClassification(t *testing.T) {
	for _, at := range []int{1, 2} {
		t.Run(string(rune('0'+at)), func(t *testing.T) {
			x := newFixture(t)
			x.a.Runtime = &reviewerEpochRuntime{Runtime: x.f, at: at}
			got, err := x.a.Apply(t.Context(), x.request)
			require.NoError(t, err)
			require.Equal(t, 1, x.f.publications)
			require.NotEqual(t, dto.ContainerChanged, got.Reason, "an untyped daemon read failure is not an observed container change")
		})
	}
}
func TestClosingReviewerDuplicateReceiptAfterRecreate(t *testing.T) {
	x := newFixture(t)
	first, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err)
	x.f.checkErr = dto.Fail(dto.ContainerChanged, 409)
	again, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err, "same-digest receipt replay must not depend on current Docker identity")
	require.Equal(t, first, again)
	require.Equal(t, 1, x.f.publications)
}
func TestClosingReviewerTransientCheckDoesNotWedgePendingRecovery(t *testing.T) {
	x := newFixture(t)
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Staging {
			return errCrash
		}
		return nil
	}
	_, err := x.a.Apply(t.Context(), x.request)
	require.ErrorIs(t, err, errCrash)
	x.a, err = NewAgent(x.db.DB, x.f, Engines{dto.Traefik: x.e})
	require.NoError(t, err)
	x.f.checkErr = errors.New("temporary daemon failure")
	q := dto.Query{ID: x.request.ID, Instance: x.request.Instance, Generation: x.request.Generation, Registration: x.request.Registration}
	_, err = x.a.Query(t.Context(), q)
	require.Error(t, err, "a transient read must be retriable, rather than committing recovery-required")
}

type reviewerTargetRuntime struct{ Runtime }

func (r reviewerTargetRuntime) Target(context.Context, dto.Instance, string) (Bytes, error) {
	return Bytes{}, errors.New("temporary archive failure")
}
func TestClosingReviewerTransientTargetDoesNotBecomeTargetChanged(t *testing.T) {
	x := newFixture(t)
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Staging {
			return errCrash
		}
		return nil
	}
	_, err := x.a.Apply(t.Context(), x.request)
	require.ErrorIs(t, err, errCrash)
	x.a, err = NewAgent(x.db.DB, reviewerTargetRuntime{Runtime: x.f}, Engines{dto.Traefik: x.e})
	require.NoError(t, err)
	q := dto.Query{ID: x.request.ID, Instance: x.request.Instance, Generation: x.request.Generation, Registration: x.request.Registration}
	_, err = x.a.Query(t.Context(), q)
	require.Error(t, err, "an unreadable archive is not evidence of changed target bytes")
}
func TestClosingReviewerTransientFinalEpochDoesNotAbandonRejectedBytes(t *testing.T) {
	x := newFixture(t)
	x.a.Runtime = &reviewerEpochRuntime{Runtime: x.f, at: 2}
	x.e.observe = func() Observation { return Observation{Restore: true, Reason: dto.ProxyRejected} }
	got, err := x.a.Apply(t.Context(), x.request)
	// The requested restore runs first; never a final release with the rejected bytes on disk.
	require.NoError(t, err)
	require.Equal(t, dto.Restored, got.State)
	after, er := x.f.Target(t.Context(), x.request.Instance, x.request.Target)
	require.NoError(t, er)
	require.Equal(t, x.request.Expected, after.FileState)
}

// C-B2 sweep: the remaining Epoch/inspect readers keep transient failures as errors.
func TestClosingPreRestartTransientEpoch(t *testing.T) {
	x := newFixture(t)
	x.request.Instance.Kind, x.request.Acknowledgements.Restart, x.e.adminOff = dto.Caddy, true, true
	x.f.epochErr = errors.New("temporary daemon failure")
	_, err := x.a.Apply(t.Context(), x.request)
	require.Error(t, err, "nothing is written; the admission stays recoverable instead of a claimed container change")
	require.Zero(t, x.f.publications)
	x.f.epochErr = nil
	q := dto.Query{ID: x.request.ID, Instance: x.request.Instance, Generation: x.request.Generation, Registration: x.request.Registration}
	got, err := x.a.Query(t.Context(), q)
	require.NoError(t, err)
	require.Equal(t, dto.Rejected, got.State)
}

func TestClosingRestartRecoveryTransientEpoch(t *testing.T) {
	x := newFixture(t)
	x.request.Instance.Kind, x.request.Acknowledgements.Restart, x.e.adminOff = dto.Caddy, true, true
	x.a.afterCommit = func(p dto.Phase) error {
		if p == dto.Restarting {
			return errCrash
		}
		return nil
	}
	_, err := x.a.Apply(t.Context(), x.request)
	require.ErrorIs(t, err, errCrash)
	x.a, err = NewAgent(x.db.DB, x.f, Engines{dto.Caddy: x.e})
	require.NoError(t, err)
	x.f.epochErr = errors.New("temporary daemon failure")
	q := dto.Query{ID: x.request.ID, Instance: x.request.Instance, Generation: x.request.Generation, Registration: x.request.Registration}
	_, err = x.a.Query(t.Context(), q)
	require.Error(t, err, "an unreadable epoch after a restart is retried, not recovery-required")
}

func TestClosingRebindTransientInspect(t *testing.T) {
	x := newFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/_ping") {
			w.Header().Set("API-Version", "1.54")
			_, _ = w.Write([]byte("OK"))
			return
		}
		http.Error(w, `{"message":"daemon temporarily busy"}`, http.StatusServiceUnavailable)
	}))
	defer server.Close()
	d := &DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: server.URL}, nil), Settings: x.settings, DB: x.db}
	_, err := d.Rebind(t.Context(), x.request.Instance)
	require.Error(t, err)
	p, typed := errors.AsType[*dto.Problem](err)
	require.False(t, typed && p.Code == dto.ContainerChanged, "a daemon failure is not a container change")
}

func TestClosingEpochRemovedContainerIsChange(t *testing.T) {
	x := newFixture(t)
	d := dockerSeam(t, x, nil)
	_, _, err := d.Epoch(t.Context(), x.request.Instance)
	requireCode(t, err, dto.ContainerChanged)
}
