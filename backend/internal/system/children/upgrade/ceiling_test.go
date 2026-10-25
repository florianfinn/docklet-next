// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Update-all ceiling and observed published candidate regression tests.

package upgrade

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	versionDTO "github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type capRoundTrip func(*http.Request) (*http.Response, error)

func (f capRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type capConn struct {
	send   func(*edge.TunnelMessage)
	mu     sync.Mutex
	closed bool
}

func (c *capConn) Send(m *edge.TunnelMessage) error      { c.send(m); return nil }
func (c *capConn) Receive() (*edge.TunnelMessage, error) { return nil, io.EOF }
func (c *capConn) IsExpectedReceiveError(error) bool     { return true }
func (c *capConn) Close() error                          { c.mu.Lock(); defer c.mu.Unlock(); c.closed = true; return nil }
func (c *capConn) IsClosed() bool                        { c.mu.Lock(); defer c.mu.Unlock(); return c.closed }
func (c *capConn) Transport() string                     { return edge.EdgeTransportWebSocket }

func TestUpdateAllAgentCeilingAndPublishedCandidate(t *testing.T) {
	saved := config.Version
	defer func() { config.Version = saved }()
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			for _, test := range []struct {
				name, hub, newest, feed, code, target string
				disabled                              bool
			}{
				{"above without published candidate", "2.0.0", "2.1.0", `[]`, "update.requires-hub-update", "", false},
				{"disabled", "2.0.0", "2.1.0", `[{"tag_name":"2.0.0","published_at":"2026-10-24T00:00:00Z"}]`, "update.requires-hub-update", "", true},
				{"blank", "2.0.0", "", `[]`, "update.invalid-target", "", false},
				{"unknown hub", "custom", "1.9.0", `[]`, "update.system-version-unknown", "", false},
				{"unknown target", "2.0.0", "custom", `[]`, "update.system-version-unknown", "", false},
				{"published cap", "2.0.0", "2.1.0", `[{"tag_name":"2.1.0","published_at":"2026-10-25T00:00:00Z"},{"tag_name":"1.9.1","published_at":"2026-10-24T00:00:00Z"},{"tag_name":"2.0.0","published_at":"2026-10-24T00:00:00Z"}]`, "", "2.0.0", false},
				{"no invented hub tag", "2.0.0", "2.1.0", `[{"tag_name":"2.1.0","published_at":"2026-10-25T00:00:00Z"},{"tag_name":"1.9.1","published_at":"2026-10-24T00:00:00Z"}]`, "", "1.9.1", false},
				{"under cap", "2.0.0", "1.9.1", `[]`, "", "1.9.1", false},
				{"no downgrade", "2.0.0", "1.8.0", `[]`, "update.invalid-target", "", false},
			} {
				t.Run(test.name, func(t *testing.T) {
					config.Version = test.hub
					g, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
					require.NoError(t, err)
					db := &database.DB{DB: g}
					require.NoError(t, db.AutoMigrate(&EnvironmentUpdateJob{}, &environment.Environment{}))
					var mu sync.Mutex
					var targets []string
					info := versionDTO.Info{CurrentVersion: "1.9.0", NewestVersion: test.newest, UpdateAvailable: true, AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}
					respond := func(method, path string, body []byte) (int, []byte) {
						if path == "/api/app-version" {
							data, e := json.Marshal(info)
							require.NoError(t, e)
							return 200, data
						}
						if method == "POST" && path == "/api/environments/0/system/upgrade" {
							var request TriggerUpgradeBody
							require.NoError(t, json.Unmarshal(body, &request))
							mu.Lock()
							targets = append(targets, request.TargetVersion)
							mu.Unlock()
							return 503, []byte(`{"error":"synthetic stop after observed effect"}`)
						}
						return 200, []byte(`{}`)
					}
					client := &http.Client{Transport: capRoundTrip(func(r *http.Request) (*http.Response, error) {
						var body []byte
						if r.Body != nil {
							var e error
							body, e = io.ReadAll(r.Body)
							require.NoError(t, e)
						}
						status, data := respond(r.Method, r.URL.Path, body)
						return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(data))), Header: http.Header{}, Request: r}, nil
					})}
					env := environment.NewEnvironmentService(db, client, nil, nil, nil, nil)
					token := "example"
					id := "example-cap-host"
					endpoint := "http://agent.example.com"
					if mode == "edge" {
						endpoint = "edge://example"
					}
					require.NoError(t, db.Create(&environment.Environment{ID: id, Name: "Example", ApiUrl: endpoint, IsEdge: mode == "edge", AccessToken: &token, ApiKeyID: new("example"), Enabled: true, Status: "pending"}).Error)
					if mode == "edge" {
						conn := &capConn{}
						tunnel := edge.NewAgentTunnelWithConn(id, conn)
						tunnel.APIKeyID = "example"
						tunnel.TokenHash = edge.AgentTokenHash(token)
						conn.send = func(m *edge.TunnelMessage) {
							if m.Type != edge.MessageTypeCommandRequest {
								return
							}
							status, data := respond(m.Method, m.Path, m.Body)
							pending, ok := tunnel.Pending.Load(m.ID)
							require.True(t, ok)
							pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeResponse, Status: status, Body: data}
						}
						edge.GetRegistry().Register(id, tunnel)
						defer edge.GetRegistry().Unregister(id)
						defer edge.ForgetPollState(id)
					}
					feedCalls := 0
					releaseClient := &http.Client{Transport: capRoundTrip(func(r *http.Request) (*http.Response, error) {
						feedCalls++
						require.Contains(t, r.URL.Path, "/releases")
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(test.feed)), Header: http.Header{}, Request: r}, nil
					})}
					service := NewService(db, nil, version.NewVersionService(releaseClient, test.disabled, test.hub, "", nil, nil, nil, nil), nil, nil, nil, nil)
					job := &EnvironmentUpdateJob{UserID: "operator", Username: "operator"}
					require.NoError(t, db.Create(job).Error)
					result := &EnvironmentUpdateResult{EnvironmentID: id}
					service.upgradeAgentInternal(context.Background(), env, id, job, result)
					if test.disabled {
						require.Zero(t, feedCalls, "disabled checks must not access the release feed")
					}
					mu.Lock()
					defer mu.Unlock()
					if test.code != "" {
						require.Equal(t, test.code, result.Error)
						require.Empty(t, targets, "refused target must never reach either transport")
					} else {
						require.Equal(t, []string{test.target}, targets)
						require.Equal(t, test.target, result.ToVersion)
					}
				})
			}
		})
	}
}
