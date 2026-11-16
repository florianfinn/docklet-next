// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing verify 2 counterexamples (C-B2).

package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	p "github.com/florianfinn/docklet-next/backend/internal/proxy"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

type verify2Runtime struct {
	p.Runtime
	fail string
}

func (r *verify2Runtime) Stage(ctx context.Context, i p.Intent) error {
	if r.fail == "stage" {
		return errors.New("temporary stage failure")
	}
	return r.Runtime.Stage(ctx, i)
}
func (r *verify2Runtime) Publish(ctx context.Context, i p.Intent) error {
	if r.fail == "publish-before" {
		return errors.New("temporary publish failure")
	}
	if err := r.Runtime.Publish(ctx, i); err != nil {
		return err
	}
	if r.fail == "publish-after" {
		return errors.New("temporary publish response failure")
	}
	return nil
}

type verify2Engine struct {
	p.Engine
	fail string
}

func (e *verify2Engine) Prepare(ctx context.Context, q dto.ApplyRequest) (p.Intent, error) {
	if e.fail == "prepare" {
		return p.Intent{}, errors.New("temporary preparation failure")
	}
	return e.Engine.Prepare(ctx, q)
}

func TestReviewerVerify2TransientApplySettlesAcrossRealTransport(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, phase := range []string{"prepare", "stage", "validate", "publish-before", "publish-after"} {
			for _, lostQuery := range []bool{false, true} {
				name := mode + "/" + phase
				if lostQuery {
					name += "/query-unavailable"
				}
				t.Run(name, func(t *testing.T) {
					x, a, tr, q := proxyWireFixture(t, mode)
					a.Runtime = &verify2Runtime{Runtime: a.Runtime, fail: phase}
					if phase == "validate" {
						a.Engines[dto.Traefik].(*proxyWireEngine).f.validateErr = errors.New("temporary validation read failure")
					}
					a.Engines[dto.Traefik] = &verify2Engine{Engine: a.Engines[dto.Traefik], fail: phase}
					calls, queries := 0, 0
					unavailable := lostQuery
					x.agentIntercept = func(w http.ResponseWriter, r *http.Request, next http.Handler) {
						if strings.HasSuffix(r.URL.Path, "/apply") {
							calls++
						}
						if strings.Contains(r.URL.Path, "/operations/") {
							queries++
							if unavailable {
								http.Error(w, "temporary transport failure", 503)
								return
							}
						}
						next.ServeHTTP(w, r)
					}
					h := &p.Hub{DB: x.f.db.DB, Modules: x.manager, Transport: tr}
					got, err := h.Apply(t.Context(), q)
					require.Positive(t, queries, "Apply failure must trigger Query")
					want := dto.Rejected
					if phase == "publish-after" {
						want = dto.Verified
					}
					var held int64
					if lostQuery {
						require.Error(t, err)
						require.Equal(t, dto.Pending, got.State)
						require.NoError(t, x.f.db.Table("proxy_reservations").Count(&held).Error)
						require.EqualValues(t, 1, held)
						_, err = h.Apply(t.Context(), q)
						require.Error(t, err)
						require.Equal(t, 1, calls)
						unavailable = false
						got, err = h.Query(t.Context(), q.ID)
					}
					require.NoError(t, err)
					require.Equal(t, want, got.State)
					require.Empty(t, got.Reason)
					require.NoError(t, x.f.db.Table("proxy_reservations").Count(&held).Error)
					require.Zero(t, held)
					replay, err := h.Apply(t.Context(), q)
					require.NoError(t, err)
					require.Equal(t, want, replay.State)
					require.Equal(t, 1, calls, "a persisted operation must never be dispatched twice")
				})
			}
		}
	}
}
