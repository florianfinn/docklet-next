// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Targeted hub evidence distinguishes in-flight delivery from lost answers.

package activity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationRemoteTargetedKnowledge(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			var executes atomic.Int32
			agent := activationApp(t, true, uuid.NewString(), activationSetup{BeforeRouter: func(router *echo.Echo) {
				router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
					return func(c *echo.Context) error {
						if strings.HasPrefix(c.Request().URL.Path, "/api/environments/0/runtime-runs") {
							if c.Request().Method == http.MethodPost && c.Request().URL.Path == "/api/environments/0/runtime-runs" {
								if executes.Add(1) == 1 {
									close(entered)
								}
								select {
								case <-release:
								case <-ctx.Done():
									return ctx.Err()
								}
							}
							return c.NoContent(http.StatusServiceUnavailable)
						}
						return next(c)
					}
				})
			}})
			hub := activationApp(t, false, "")
			env := activationRemote(t, hub, agent, transport)
			status, body := activationCall(t, hub, "/api/environments/"+env+"/containers/"+runtimeContainerID+"/start", uuid.NewString(), "respond-async", nil)
			require.Equal(t, 202, status, string(body))
			run := activationRunID(t, body)
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("execute did not reach actual transport")
			}
			detail := func() activitytypes.RuntimeHubDetail {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, hub.server.URL+"/api/environments/"+env+"/runtime-runs/"+run, nil)
				require.NoError(t, err)
				response, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				defer response.Body.Close()
				require.Equal(t, 200, response.StatusCode)
				var out struct {
					Data activitytypes.RuntimeHubDetail `json:"data"`
				}
				require.NoError(t, json.NewDecoder(response.Body).Decode(&out))
				return out.Data
			}
			pending := detail()
			require.Equal(t, activitytypes.RuntimeOutcomeUnknown, pending.Activity.OutcomeState)
			require.Equal(t, "pending", pending.Recovery.ExecutorState)
			require.Nil(t, pending.Recovery.UnknownReason, "no lost-answer warning while delivery is in flight")
			unblock()
			require.NoError(t, hub.remote.WaitDispatch(ctx, run))
			lost := detail()
			require.Equal(t, activitytypes.RuntimeOutcomeUnknown, lost.Activity.OutcomeState)
			require.Equal(t, "unknown", lost.Recovery.ExecutorState)
			require.Equal(t, new("transport"), lost.Recovery.UnknownReason)
			require.EqualValues(t, 1, executes.Load())
			require.Empty(t, runtimeEffectEvents(agent.services.http.snapshot()))
		})
	}
}
