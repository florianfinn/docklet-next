// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Actual registered middleware byte limits and joined slow network body.

package activity_test

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationRegisteredIngressBounds(t *testing.T) {
	token := uuid.NewString()
	returned := make(chan struct{}, 1)
	agent := activationApp(t, true, token, activationSetup{AdmissionBudget: 100 * time.Millisecond, BeforeRouter: func(router *echo.Echo) {
		router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c *echo.Context) error {
				defer func() {
					select {
					case returned <- struct{}{}:
					default:
					}
				}()
				return next(c)
			}
		})
	}})
	status, response := activationMetadataCall(t, agent, "/api/environments/0/runtime-registration", map[string]string{remenv.HeaderAgentToken: token}, []byte(strings.Repeat(" ", activitytypes.RuntimeMaxBodyBytes+1)))
	require.Equal(t, 413, status, string(response))
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("oversized actual handler did not join")
	}
	address := strings.TrimPrefix(agent.server.URL, "http://")
	connection, err := net.DialTimeout("tcp", address, 5*time.Second)
	require.NoError(t, err)
	defer connection.Close()
	require.NoError(t, connection.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = fmt.Fprintf(connection, "POST /api/environments/0/runtime-registration HTTP/1.1\r\nHost: example.invalid\r\nX-Arcane-Agent-Token: %s\r\nContent-Type: application/json\r\nContent-Length: 4096\r\n\r\n{\"managerId\":", token)
	require.NoError(t, err)
	reply, err := http.ReadResponse(bufio.NewReader(connection), nil)
	require.NoError(t, err)
	body, err := io.ReadAll(reply.Body)
	require.NoError(t, err)
	require.NoError(t, reply.Body.Close())
	require.Equal(t, 408, reply.StatusCode, string(body))
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("expired actual Echo/Huma handler did not join")
	}
	var count int64
	require.NoError(t, agent.services.db.Model(&activity.RuntimeExecutorBinding{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, agent.services.db.Model(&activity.RuntimeReceipt{}).Count(&count).Error)
	require.Zero(t, count)
	require.Empty(t, runtimeEffectEvents(agent.services.http.snapshot()))
}
