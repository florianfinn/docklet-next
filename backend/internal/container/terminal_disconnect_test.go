// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original real Direct/Edge disconnect audit evidence for D5.

package container

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/containershell"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func shellAudit(t *testing.T, owners ...context.Context) (*shellObservedActivity, *containershell.Registry, func() activity.Activity) {
	t.Helper()
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "shell.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	audit := &shellObservedActivity{ActivityService: activity.NewActivityService(db, nil), completed: make(chan string, 4), counts: map[string]int{}}
	sessions := containershell.New(audit, owners...)
	return audit, sessions, func() activity.Activity {
		t.Helper()
		select {
		case id := <-audit.completed:
			var entry activity.Activity
			require.NoError(t, db.First(&entry, "id = ?", id).Error)
			require.NotNil(t, entry.EndedAt)
			require.NotNil(t, entry.DurationMs)
			require.Equal(t, "example-user", *entry.StartedByUserID)
			require.Empty(t, entry.Output)
			require.Zero(t, sessions.ActiveCount())
			audit.mu.Lock()
			count := audit.counts[id]
			audit.mu.Unlock()
			require.Equal(t, 1, count)
			return entry
		case <-time.After(8 * time.Second):
			t.Fatal("shell audit did not complete within drain bound")
			return activity.Activity{}
		}
	}
}

func TestShellDisconnectAuditRealDirectAndEdge(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		for _, ending := range []string{"browser-first", "close-control", "agent-drop"} {
			t.Run(transport+"/"+ending, func(t *testing.T) {
				_, sessions, completed := shellAudit(t)
				f := &shellDocker{orphan: true, agentSockets: make(chan *websocket.Conn, 1)}
				hub := shellTransportHub(t, transport, dockerTerminalAgent(t, f), sessions, middleware.EnvironmentProxySecurity{})
				browser := dialShellHub(t, hub)
				_, data := readShell(t, browser)
				require.Equal(t, "ready", string(data))
				agent := <-f.agentSockets
				switch ending {
				case "browser-first":
					require.NoError(t, browser.Close(websocket.StatusNormalClosure, ""))
				case "close-control":
					require.NoError(t, browser.Write(t.Context(), websocket.MessageText, wshub.TerminalCloseControl))
					exit := readShellExit(t, browser)
					require.Equal(t, "container.shell-orphaned", exit.Reason)
					_, _, err := browser.Read(t.Context())
					require.Equal(t, websocket.StatusPolicyViolation, websocket.CloseStatus(err))
				case "agent-drop":
					require.NoError(t, agent.CloseNow())
					exit := readShellExit(t, browser)
					require.Empty(t, exit.Reason)
					require.Nil(t, exit.Code)
					_, _, _ = browser.Read(t.Context())
				}
				entry := completed()
				if ending == "agent-drop" {
					require.Equal(t, "Shell ended", entry.LatestMessage)
					require.Equal(t, activityDTO.StatusCancelled, entry.Status)
					require.NotContains(t, entry.MessageCode, "container.shell-")
				} else {
					require.Equal(t, "container.shell-orphaned", entry.LatestMessage)
					require.Equal(t, activityDTO.StatusFailed, entry.Status)
					require.GreaterOrEqual(t, f.inspections.Load(), int32(2))
				}
			})
		}
	}
}

func TestShellExitDrainBoundRealDirectAndEdge(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			_, sessions, completed := shellAudit(t)
			received := make(chan struct{})
			agent := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := wshub.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				_ = conn.Write(r.Context(), websocket.MessageBinary, []byte("ready"))
				for {
					kind, data, err := conn.Read(context.WithoutCancel(r.Context()))
					if err != nil {
						return
					}
					if wshub.IsTerminalClose(kind, data) {
						close(received)
					}
				}
			})
			hub := shellTransportHub(t, transport, agent, sessions, middleware.EnvironmentProxySecurity{})
			browser := dialShellHub(t, hub)
			readShell(t, browser)
			started := time.Now()
			require.NoError(t, browser.Close(websocket.StatusNormalClosure, ""))
			<-received
			entry := completed()
			require.GreaterOrEqual(t, time.Since(started), 5*time.Second)
			require.Equal(t, "Shell ended", entry.LatestMessage)
			require.Equal(t, activityDTO.StatusCancelled, entry.Status)
			require.NotContains(t, entry.MessageCode, "container.shell-")
		})
	}
}

func TestShellDialFailureAuditRealDirectAndEdge(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			_, sessions, completed := shellAudit(t)
			hub := shellTransportHub(t, transport, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }), sessions, middleware.EnvironmentProxySecurity{})
			browser := dialShellHub(t, hub)
			exit := readShellExit(t, browser)
			require.Empty(t, exit.Reason)
			require.Nil(t, exit.Code)
			_, _, closeErr := browser.Read(t.Context())
			require.Contains(t, []websocket.StatusCode{websocket.StatusNormalClosure, websocket.StatusPolicyViolation}, websocket.CloseStatus(closeErr))
			var closed websocket.CloseError
			require.ErrorAs(t, closeErr, &closed)
			require.Empty(t, closed.Reason, "generic tunnel errors are not shell reasons")
			entry := completed()
			require.Equal(t, "Shell ended", entry.LatestMessage)
			require.Equal(t, activityDTO.StatusCancelled, entry.Status)
			require.NotContains(t, entry.MessageCode, "container.shell-")
		})
	}
}

func TestShellUpgradeFailureAuditRealDirectAndEdge(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			_, sessions, completed := shellAudit(t)
			hub := shellTransportHub(t, transport, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("failed browser upgrade reached agent")
				w.WriteHeader(503)
			}), sessions, middleware.EnvironmentProxySecurity{})
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, hub.URL+"/api/environments/example-host/ws/containers/example/terminal", nil)
			require.NoError(t, err)
			request.Header = http.Header{"Origin": {"https://hub.example.com"}, "Connection": {"Upgrade"}, "Upgrade": {"websocket"}, "Sec-WebSocket-Version": {"13"}}
			response, err := http.DefaultClient.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, 400, response.StatusCode)
			entry := completed()
			require.Equal(t, "Shell ended", entry.LatestMessage)
		})
	}
}

func TestShellShutdownAndRevocationRealDirectAndEdge(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		for _, cause := range []string{"shutdown", "revocation"} {
			t.Run(transport+"/"+cause, func(t *testing.T) {
				owner, stop := context.WithCancel(t.Context())
				defer stop()
				_, sessions, completed := shellAudit(t, owner)
				hub := shellTransportHub(t, transport, dockerTerminalAgent(t, &shellDocker{}), sessions, middleware.EnvironmentProxySecurity{})
				browser := dialShellHub(t, hub)
				readShell(t, browser)
				if cause == "shutdown" {
					stop()
				} else {
					sessions.CloseByUser("example-user")
				}
				exit := readShellExit(t, browser)
				require.Empty(t, exit.Reason)
				require.NotNil(t, exit.Code)
				_, _, _ = browser.Read(t.Context())
				entry := completed()
				require.Equal(t, "Shell ended", entry.LatestMessage)
			})
		}
	}
}
