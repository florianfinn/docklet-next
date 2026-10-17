// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original in-flight transport revocation tests.

package environment

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

type agentBlockingBody struct{ ctx context.Context }

func (b *agentBlockingBody) Read([]byte) (int, error) { <-b.ctx.Done(); return 0, b.ctx.Err() }
func (b *agentBlockingBody) Close() error             { return nil }

func TestAgentRevocationEndsInflightStreams(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, action := range []string{"rotate", "remove"} {
			t.Run(mode+"/"+action, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				f := newAgentFixture(t, mode, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
				entered := make(chan struct{}, 1)
				if mode == "direct" {
					original := f.service.httpClient.Transport
					f.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
						if r.URL.Path == "/api/environments/0/containers" {
							entered <- struct{}{}
							return &http.Response{StatusCode: 200, Header: http.Header{}, Body: &agentBlockingBody{ctx: r.Context()}, Request: r}, nil
						}
						return original.RoundTrip(r)
					})
				} else {
					original := f.conn.send
					f.conn.send = func(m *edge.TunnelMessage) {
						if m.Type == edge.MessageTypeCommandRequest && m.Path == "/api/environments/0/containers" {
							require.Equal(t, f.token, m.Headers[edge.HeaderAgentToken])
							entered <- struct{}{}
							return
						}
						original(m)
					}
				}
				finished := make(chan error, 1)
				go func() {
					_, err := f.service.ExecuteRemoteRequest(ctx, f.id, "GET", "/api/environments/0/containers", nil)
					finished <- err
				}()
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("stream did not start")
				}
				if action == "remove" {
					require.NoError(t, f.service.DeleteEnvironment(ctx, f.id, nil, nil))
				} else {
					keys := apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
					f.service.apiKeys = keys
					env, err := f.service.GetEnvironmentByID(ctx, f.id)
					require.NoError(t, err)
					_, err = f.service.RegenerateEnvironmentApiKey(ctx, env, "operator", "operator")
					require.NoError(t, err)
				}
				select {
				case err := <-finished:
					require.Error(t, err)
				case <-ctx.Done():
					t.Fatal("revoked stream did not terminate")
				}
			})
		}
	}
}
