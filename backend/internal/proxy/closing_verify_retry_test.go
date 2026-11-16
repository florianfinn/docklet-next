// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing verify counterexamples (C-B2 retry control).

package proxy

import (
	"context"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"testing"
)

type verifyTargetRuntime struct {
	Runtime
	unavailable bool
}

func (r *verifyTargetRuntime) Target(ctx context.Context, i dto.Instance, p string) (Bytes, error) {
	if r.unavailable {
		return Bytes{}, errors.New("temporary archive failure")
	}
	return r.Runtime.Target(ctx, i, p)
}
func TestVerifyRetryDoesNotLeakActiveOrSourceLock(t *testing.T) {
	for _, stage := range []string{"check", "target"} {
		t.Run(stage, func(t *testing.T) {
			x := newFixture(t)
			x.a.afterCommit = func(p dto.Phase) error {
				if p == dto.Staging {
					return errCrash
				}
				return nil
			}
			_, err := x.a.Apply(t.Context(), x.request)
			require.ErrorIs(t, err, errCrash)
			r := &verifyTargetRuntime{Runtime: x.f, unavailable: stage == "target"}
			x.a, err = NewAgent(x.db.DB, r, Engines{dto.Traefik: x.e})
			require.NoError(t, err)
			if stage == "check" {
				x.f.checkErr = errors.New("temporary daemon failure")
			}
			for range 3 {
				got, err := x.a.Query(t.Context(), x.query())
				require.Error(t, err)
				require.Equal(t, dto.Pending, got.State)
				require.False(t, x.a.active[x.request.ID])
				j, err := findJournal(t.Context(), x.db.DB, x.request.ID)
				require.NoError(t, err)
				require.Nil(t, j.FinishedAt)
			}
			x.f.checkErr = nil
			r.unavailable = false
			got, err := x.a.Query(t.Context(), x.query())
			require.NoError(t, err)
			require.Equal(t, dto.Rejected, got.State)
			second := cloneRequest(x.request)
			second.ID = "retry-operation-two"
			got, err = x.a.Apply(t.Context(), second)
			require.NoError(t, err)
			require.Equal(t, dto.Verified, got.State)
		})
	}
}
