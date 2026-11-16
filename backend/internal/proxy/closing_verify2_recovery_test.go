// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing verify 2 counterexamples (C-B2).

package proxy

import (
	"context"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"testing"
)

type verify2RecoverRuntime struct {
	Runtime
	targetErr, cleanupErr, rebindErr error
}

func (r *verify2RecoverRuntime) Target(ctx context.Context, i dto.Instance, p string) (Bytes, error) {
	if r.targetErr != nil {
		return Bytes{}, r.targetErr
	}
	return r.Runtime.Target(ctx, i, p)
}
func (r *verify2RecoverRuntime) Cleanup(ctx context.Context, i Intent) error {
	if r.cleanupErr != nil {
		return r.cleanupErr
	}
	return r.Runtime.Cleanup(ctx, i)
}
func (r *verify2RecoverRuntime) Rebind(ctx context.Context, i dto.Instance) (dto.Instance, error) {
	if r.rebindErr != nil {
		return i, r.rebindErr
	}
	return r.Runtime.Rebind(ctx, i)
}
func TestReviewerVerify2StartupTransientQueryAndRelease(t *testing.T) {
	for _, state := range []string{"old", "new", "third"} {
		t.Run(state, func(t *testing.T) {
			x := newFixture(t)
			x.a.afterCommit = func(p dto.Phase) error {
				if p == dto.Publishing {
					return errors.New("temporary publishing database response failure")
				}
				return nil
			}
			got, err := x.a.Apply(t.Context(), x.request)
			require.Error(t, err)
			require.Equal(t, dto.Pending, got.State)
			if state == "new" {
				x.f.files[x.request.Target] = x.e.new
			}
			if state == "third" {
				x.f.files[x.request.Target] = Snapshot([]byte("operator bytes"), true)
			}
			r := &verify2RecoverRuntime{Runtime: x.f, targetErr: errors.New("temporary target failure")}
			x.a, err = NewAgent(x.db.DB, r, Engines{dto.Traefik: x.e})
			require.NoError(t, err)
			for range 3 {
				require.NoError(t, x.a.Settle(t.Context()))
				j, err := findJournal(t.Context(), x.db.DB, x.request.ID)
				require.NoError(t, err)
				require.Equal(t, dto.Pending, j.State)
				require.Equal(t, dto.Publishing, j.Phase)
				require.Nil(t, j.FinishedAt)
				require.False(t, x.a.active[x.request.ID])
			}
			r.targetErr = nil
			require.NoError(t, x.a.Settle(t.Context()))
			got, err = x.a.Query(t.Context(), x.query())
			require.NoError(t, err)
			want := dto.Rejected
			if state == "new" {
				want = dto.Verified
			}
			if state == "third" {
				want = dto.RecoveryRequired
			}
			require.Equal(t, want, got.State)
			require.Zero(t, x.f.publications, "recovery observes actual bytes without repeating publication")
			if state != "third" {
				require.Empty(t, got.Reason)
				return
			}
			require.Equal(t, dto.TargetChanged, got.Reason)
			before := x.f.files[x.request.Target]
			release := dto.ReleaseRequest{Query: x.query(), Confirmed: before.FileState, Actor: "example-admin"}
			for _, failure := range []string{"rebind", "target", "cleanup"} {
				errRead := errors.New("temporary release failure")
				if failure == "rebind" {
					r.rebindErr = errRead
				}
				if failure == "target" {
					r.targetErr = errRead
				}
				if failure == "cleanup" {
					r.cleanupErr = errRead
				}
				got, err = x.a.Release(t.Context(), release)
				if failure == "cleanup" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				require.Equal(t, dto.RecoveryRequired, got.State)
				require.Equal(t, before, x.f.files[x.request.Target])
				j, err := findJournal(t.Context(), x.db.DB, x.request.ID)
				require.NoError(t, err)
				require.Nil(t, j.FinishedAt)
				r.rebindErr = nil
				r.targetErr = nil
				r.cleanupErr = nil
			}
			got, err = x.a.Release(t.Context(), release)
			require.NoError(t, err)
			require.Equal(t, dto.Released, got.State)
			require.Equal(t, before, x.f.files[x.request.Target])
		})
	}
}
