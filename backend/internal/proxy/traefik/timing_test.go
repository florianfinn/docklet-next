// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original deadline tests at completed Docker response boundaries.

package traefik

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

type responseCloseEffect struct {
	io.ReadCloser
	once   sync.Once
	effect func()
}

func (r *responseCloseEffect) Close() error {
	err := r.ReadCloser.Close()
	r.once.Do(r.effect)
	return err
}

// The real SDK decodes and closes the response before returning to DockerRuntime.
// Effects here synchronise with completed transport work, never a request start.
type effectTransport struct {
	base   http.RoundTripper
	match  func(*http.Response) bool
	effect func()
}

func (r effectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.base.RoundTrip(req)
	if err == nil && r.match(resp) {
		resp.Body = &responseCloseEffect{ReadCloser: resp.Body, effect: r.effect}
	}
	return resp, err
}
func afterDockerResponseClose(t *testing.T, x *seam, match func(*http.Response) bool, effect func()) {
	t.Helper()
	old, err := x.d.Docker.GetClient(t.Context())
	require.NoError(t, err)
	transport := &http.Transport{}
	t.Cleanup(transport.CloseIdleConnections)
	next, err := client.New(client.WithHost(old.DaemonHost()), client.WithAPIVersion(old.ClientVersion()), client.WithHTTPClient(&http.Client{Transport: effectTransport{base: transport, match: match, effect: effect}}))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, next.Close()) })
	x.d.Docker = x.d.Docker.WithClient(next)
}

func execInspectResponse(r *http.Response) bool {
	return strings.Contains(r.Request.URL.Path, "/exec/") && strings.HasSuffix(r.Request.URL.Path, "/json")
}
func logResponse(r *http.Response) bool { return strings.HasSuffix(r.Request.URL.Path, "/logs") }

func TestProbeCompletionCannotAcceptEvidenceAfterItsBudget(t *testing.T) {
	for _, marked := range []bool{false, true} {
		for _, cancelBudget := range []bool{false, true} {
			x := fixture(t)
			in := x.prepare(x.request("edit-route"))
			if marked {
				require.NoError(t, x.e.Mark(t.Context(), in))
			}
			base := x.now
			var expired atomic.Bool
			x.e.now = func() time.Time {
				if expired.Load() {
					return base.Add(20 * time.Second)
				}
				return base
			}
			var cancel context.CancelFunc
			x.e.limit = func(parent context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
				ctx, stop := context.WithCancel(parent)
				cancel = stop
				return ctx, stop
			}
			var inspected atomic.Int64
			afterDockerResponseClose(t, x, execInspectResponse, func() {
				n := inspected.Add(1)
				if !marked && n == 1 || marked && n == 2 {
					if cancelBudget {
						cancel()
					} else {
						expired.Store(true)
					}
				}
			})
			x.probeFunc = func(args []string) hostio.ExecResult {
				if strings.Contains(strings.Join(args, " "), "docklet-check.invalid") {
					return response(502, "closed")
				}
				if marked {
					return response(200, "old")
				}
				return response(200, "changed")
			}
			got, err := x.e.Observe(t.Context(), in, marked)
			require.NoError(t, err)
			require.NotEqual(t, dto.Verified, got.State)
			if marked {
				require.Equal(t, dto.NotObservable, got.Reason)
			} else {
				require.True(t, got.NeedsMarker)
			}
			x.mu.Lock()
			logCalls := len(x.logSince)
			x.mu.Unlock()
			require.Equal(t, 1, logCalls, "no further Docker log request may start after completed evidence exhausted the budget")
		}
	}
}

func TestFinalLogCompletionCannotAcceptExpiredEvidence(t *testing.T) {
	for _, cancelBudget := range []bool{false, true} {
		x := fixture(t)
		in := x.prepare(x.request("edit-route"))
		base := x.now
		var expired atomic.Bool
		x.e.now = func() time.Time {
			if expired.Load() {
				return base.Add(20 * time.Second)
			}
			return base
		}
		var cancel context.CancelFunc
		x.e.limit = func(parent context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
			ctx, stop := context.WithCancel(parent)
			cancel = stop
			return ctx, stop
		}
		var logs atomic.Int64
		afterDockerResponseClose(t, x, logResponse, func() {
			if logs.Add(1) == 2 {
				if cancelBudget {
					cancel()
				} else {
					expired.Store(true)
				}
			}
		})
		x.probeFunc = func([]string) hostio.ExecResult { return response(200, "changed") }
		got, err := x.e.Observe(t.Context(), in, false)
		require.NoError(t, err)
		require.True(t, got.NeedsMarker)
		require.EqualValues(t, 2, logs.Load(), "expiration follows the completed final log fence")
	}
}

func TestInitialLogCompletionKeepsRecoveryReasonsAndUnresolvedRestore(t *testing.T) {
	for _, unresolved := range []bool{false, true} {
		x := fixture(t)
		r := x.request("edit-route")
		if unresolved {
			r.Change.Middlewares = []string{"missing@file"}
			r.Acknowledgements.UnresolvedReferences = []string{"missing@file"}
		}
		in := x.prepare(r)
		if !unresolved {
			// Persisted recovery input for a preparation marker probe that exceeded the
			// hostio.Exec child budget while its parent operation remained live.
			s, err := state(in)
			require.NoError(t, err)
			s.Marker.Reason = dto.ExecRunning
			s.Marker.Before = hostio.Response{}
			in.Engine, err = json.Marshal(s)
			require.NoError(t, err)
		}
		var cancel context.CancelFunc
		x.e.limit = func(parent context.Context, _ time.Duration) (context.Context, context.CancelFunc) {
			ctx, stop := context.WithCancel(parent)
			cancel = stop
			return ctx, stop
		}
		afterDockerResponseClose(t, x, logResponse, func() { cancel() })
		got, err := x.e.Observe(t.Context(), in, false)
		require.NoError(t, err)
		if unresolved {
			require.True(t, got.Restore)
			require.Equal(t, dto.ProxyRejected, got.Reason)
		} else {
			require.Equal(t, dto.AppliedUnverified, got.State)
			require.Equal(t, dto.ExecRunning, got.Reason)
		}
	}
}
