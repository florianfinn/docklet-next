// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original lifecycle evidence with real connection loss and the real agent-online notifier.

package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	p "github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func waitDone(t *testing.T, what string, c <-chan struct{}) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(10 * time.Second):
		require.Failf(t, "not observed", "%s", what)
	}
}

func proxyHubRecord(t *testing.T, x *m27ModuleFixture) p.Record {
	t.Helper()
	var row p.Record
	require.NoError(t, x.f.db.Order("created_at DESC").First(&row).Error)
	return row
}

// L1/L2 with real connection loss, Direct and Edge. "caller": the browser's TCP
// connection to the hub is closed mid-request; the hub's own follow-up still records
// the verified result. "agent-link": the hub-to-agent connection (Direct TCP or the
// Edge tunnel) is closed mid-request; the agent still finishes and the hub settles.
// Each loss is observed on the receiving side before the admitted phase continues.
func TestM27ProxyRealConnectionLossAfterAdmission(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, cell := range []string{"caller", "agent-link"} {
			t.Run(mode+"/"+cell, func(t *testing.T) {
				x, a, tr, q := proxyWireFixture(t, mode)
				f := a.Runtime.(*proxyWireRuntime)
				f.inside, f.resume = make(chan struct{}), make(chan struct{})
				raw, err := json.Marshal(q.Instance)
				require.NoError(t, err)
				require.NoError(t, x.f.db.Exec("INSERT INTO proxy_instances(id,environment_id,data) VALUES (?,?,?)", q.Instance.ID, q.Instance.EnvironmentID, raw).Error)
				m := &p.Module{Hub: &p.Hub{DB: x.f.db.DB, Modules: x.manager, Transport: tr}, Modules: x.manager, Settings: x.hubSettings, Transport: tr}
				m.RegisterRoutes(x.hubAPI, false)
				agentLost := make(chan struct{})
				x.agentIntercept = func(w http.ResponseWriter, r *http.Request, next http.Handler) {
					if strings.HasSuffix(r.URL.Path, "/apply") {
						go func() { <-r.Context().Done(); close(agentLost) }()
					}
					next.ServeHTTP(w, r)
				}
				hubLost, hubDone := make(chan struct{}), make(chan struct{})
				hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodPost {
						defer close(hubDone)
						go func() { <-r.Context().Done(); close(hubLost) }()
					}
					x.hub.ServeHTTP(w, r)
				}))
				defer hub.Close()
				input, err := json.Marshal(dto.OperationRequest{Target: q.Target, Expected: q.Expected, GraphHashes: q.GraphHashes, Change: q.Change})
				require.NoError(t, err)
				caller, cancelCaller := context.WithCancel(t.Context())
				defer cancelCaller()
				request, err := http.NewRequestWithContext(caller, http.MethodPost, hub.URL+"/api/modules/proxy/instances/"+q.Instance.ID+"/operations", strings.NewReader(string(input)))
				require.NoError(t, err)
				request.Header.Set("Content-Type", "application/json")
				request.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: x.f.tokens.BrowserToken})
				sent := make(chan error, 1)
				go func() {
					response, err := hub.Client().Do(request)
					if err == nil {
						_ = response.Body.Close()
					}
					sent <- err
				}()
				waitDone(t, "admitted work reaching Validate", f.inside)
				if cell == "caller" {
					cancelCaller()
					waitDone(t, "the hub request context ending with the caller connection", hubLost)
				} else if mode == "direct" {
					x.agentServer.CloseClientConnections()
					waitDone(t, "the agent request context ending with the Direct connection", agentLost)
				} else {
					require.NoError(t, x.tunnel.CloseWithReason("example connection loss"))
					waitDone(t, "the agent request context ending with the Edge tunnel", agentLost)
				}
				close(f.resume)
				waitDone(t, "the hub handler returning", hubDone)
				if cell == "caller" {
					require.Error(t, <-sent)
					// The hub's own detached follow-up recorded the result; nothing else queried.
					require.Equal(t, dto.Verified, proxyHubRecord(t, x).State)
				} else {
					id := proxyHubRecord(t, x).ID
					require.Eventually(t, func() bool {
						var state dto.State
						return a.DB.Table("proxy_journal").Select("state").Where("id = ?", id).Scan(&state).Error == nil && state == dto.Verified
					}, 10*time.Second, 50*time.Millisecond, "the agent finishes after losing its connection")
					require.Eventually(t, func() bool {
						o, err := m.Hub.Query(t.Context(), id)
						return err == nil && o.State == dto.Verified
					}, 15*time.Second, 100*time.Millisecond)
				}
				f.mu.Lock()
				defer f.mu.Unlock()
				require.Equal(t, "new", string(f.target.Content))
			})
		}
	}
}

// L2 production wiring: NewModule and Start subscribe the reconciler to the real
// agent-online notifier; an authenticated online transition settles a pending record.
func TestM27ProxyReconcilerFollowsRealAgentOnline(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x, _, tr, q := proxyWireFixture(t, mode)
			var down atomic.Bool
			down.Store(true)
			var queries atomic.Int32
			x.agentIntercept = func(w http.ResponseWriter, r *http.Request, next http.Handler) {
				if strings.HasSuffix(r.URL.Path, "/apply") {
					next.ServeHTTP(httptest.NewRecorder(), r)
					w.WriteHeader(503)
					return
				}
				if strings.Contains(r.URL.Path, "/modules/proxy/") {
					// The answer is decided on arrival, so a request counted while down is
					// refused however long it takes afterwards.
					refuse := down.Load()
					queries.Add(1)
					if refuse {
						w.WriteHeader(503)
						return
					}
				}
				next.ServeHTTP(w, r)
			}
			h := &p.Hub{DB: x.f.db.DB, Modules: x.manager, Transport: tr}
			// The follow-up cannot reach the agent, so the record stays unfinished.
			_, _ = h.Apply(t.Context(), q)
			require.NotEqual(t, dto.Verified, proxyHubRecord(t, x).State)
			m, err := p.NewModule(x.f.db, &config.Config{}, x.manager, x.hubSettings, nil, nil, x.hosts)
			require.NoError(t, err)
			before := queries.Load()
			require.NoError(t, m.Start(t.Context()))
			t.Cleanup(func() { require.NoError(t, m.Stop(context.Background())) })
			require.Eventually(t, func() bool { return queries.Load() > before }, 10*time.Second, 20*time.Millisecond, "the startup pass queries and is refused")
			require.NotEqual(t, dto.Verified, proxyHubRecord(t, x).State)
			down.Store(false)
			// Each round makes the hub see the agent come back. The startup query was refused,
			// and the periodic interval is longer than this test, so only the notifier can
			// start the pass that settles the record.
			require.Eventually(t, func() bool {
				require.NoError(t, x.f.db.Exec("UPDATE environments SET status = 'offline' WHERE id = ?", "example").Error)
				_, _ = x.hosts.CheckAgentCompatibility(t.Context(), "example")
				return proxyHubRecord(t, x).State == dto.Verified
			}, 10*time.Second, 200*time.Millisecond)
		})
	}
}
