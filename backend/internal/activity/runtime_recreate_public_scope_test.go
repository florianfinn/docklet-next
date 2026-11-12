// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original recreate exclusion through public admission and real held Direct/Edge receivers.

package activity_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRecreatePublicTransportsHoldContainerExclusion(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			hub := activationApp(t, false, "")
			executor, environment := hub, "0"
			if mode != "local" {
				executor = activationApp(t, true, uuid.NewString())
				environment = activationRemote(t, hub, executor, mode)
			}
			daemon := newRecreateHTTP(t)
			daemon.hold = "create"
			daemon.entered, daemon.release = make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(daemon.release) })
			defer unblock()
			executor.services.http.onCall = daemon.serve
			path := "/api/environments/" + environment + "/containers/" + runtimeContainerID
			status, body := activationCall(t, hub, path+"/recreate", uuid.NewString(), "respond-async", []byte(`{"edit":{}}`))
			require.Equal(t, 202, status, string(body))
			run := activationRunID(t, body)
			select {
			case <-daemon.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("recreate never reached held receiver effect")
			}
			status, body = activationCall(t, hub, path+"/restart", uuid.NewString(), "respond-async", nil)
			require.Equal(t, 409, status, string(body))
			require.Contains(t, string(body), "run.busy")
			require.Zero(t, daemon.counts()["restart"])
			unblock()
			activationJoinRun(t, hub, executor, environment, run)
			var row activity.RuntimeReceipt
			require.NoError(t, hub.services.db.First(&row, "run_id=?", run).Error)
			require.Equal(t, "container:"+runtimeContainerID, *row.GateScope)
		})
	}
}
func TestRecreatePublicBodyBoundsAndRetiredPaths(t *testing.T) {
	hub := activationApp(t, false, "")
	base := "/api/environments/0/containers/" + runtimeContainerID
	for _, body := range []string{`{"edit":{},"unknown":true}`, `{"edit":{"hostConfig":{"unknown":true}}}`, `{"edit":{"command":[1]}}`, strings.Repeat(" ", 64<<10) + `{"edit":{}}`} {
		status, response := activationCall(t, hub, base+"/recreate", uuid.NewString(), "respond-async", []byte(body))
		require.Contains(t, []int{400, 413}, status, string(response))
	}
	status, body := activationCall(t, hub, base+"/edit", "", "", []byte(`{}`))
	require.Equal(t, 409, status, string(body))
	require.Contains(t, string(body), "update.path-retired")
	status, body = activationCall(t, hub, base+"/redeploy", "", "", nil)
	require.Equal(t, 409, status, string(body))
	require.Contains(t, string(body), "update.path-retired")
	var count int64
	require.NoError(t, hub.services.db.Model(&activity.RuntimeReceipt{}).Count(&count).Error)
	require.Zero(t, count)
}
func TestRedeployRealTransportsReplacementResult(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			hub := activationApp(t, false, "")
			executor, environment := hub, "0"
			if mode != "local" {
				executor = activationApp(t, true, uuid.NewString())
				environment = activationRemote(t, hub, executor, mode)
			}
			daemon := newRecreateHTTP(t)
			executor.services.http.onCall = daemon.serve
			key := uuid.NewString()
			path := "/api/environments/" + environment + "/containers/" + runtimeContainerID + "/redeploy"
			status, body := activationCall(t, hub, path, key, "respond-async", nil)
			require.Equal(t, 202, status, string(body))
			run := activationRunID(t, body)
			activationJoinRun(t, hub, executor, environment, run)
			var row activity.RuntimeReceipt
			require.NoError(t, hub.services.db.First(&row, "run_id=?", run).Error)
			require.Equal(t, "success", *row.ExecutorState)
			require.NotNil(t, row.Result)
			require.Contains(t, *row.Result, recreateNewID)
			status, body = activationCall(t, hub, path, key, "respond-async", nil)
			require.Equal(t, 200, status, string(body))
			require.Contains(t, string(body), recreateNewID)
			require.Equal(t, 1, daemon.counts()["pull"])
			require.Equal(t, 1, daemon.counts()["create"])
		})
	}
}
